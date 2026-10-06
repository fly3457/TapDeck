//go:build windows

package input

import "testing"

type fakeVolume struct {
	steps  []bool
	mutes  int
	failed error
}

func (f *fakeVolume) Step(up bool) error {
	if f.failed != nil {
		return f.failed
	}
	f.steps = append(f.steps, up)
	return nil
}

func (f *fakeVolume) ToggleMute() error {
	if f.failed != nil {
		return f.failed
	}
	f.mutes++
	return nil
}

// Volume keys must be served by Core Audio and never injected as key events:
// Windows ignores injected volume keys.
func TestVolumeKeysUseVolumeControl(t *testing.T) {
	c := New()
	vol := &fakeVolume{}
	c.SetVolume(vol)
	var injected int
	c.inject = func(items ...nativeInput) error { injected += len(items); return nil }

	if err := c.Hold("VolumeUp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Chord("VolumeDown"); err != nil {
		t.Fatal(err)
	}
	if err := c.Chord("VolumeMute"); err != nil {
		t.Fatal(err)
	}
	if len(vol.steps) != 2 || vol.steps[0] != true || vol.steps[1] != false || vol.mutes != 1 {
		t.Fatalf("volume calls: steps=%v mutes=%d", vol.steps, vol.mutes)
	}
	if injected != 0 {
		t.Fatalf("volume keys were injected as key events: %d", injected)
	}
}

// Holding a volume key must not repeat the action on every pulse, and a second
// press after release must work again.
func TestVolumeKeyRepeatsOnlyAfterRelease(t *testing.T) {
	c := New()
	vol := &fakeVolume{}
	c.SetVolume(vol)
	c.inject = func(...nativeInput) error { return nil }

	if err := c.Hold("VolumeUp", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Hold("VolumeUp", true); err != nil {
		t.Fatal(err)
	}
	if len(vol.steps) != 1 {
		t.Fatalf("volume stepped %d times while held", len(vol.steps))
	}
	// The engine pulses down twice while the button is held, so release twice.
	if err := c.Hold("VolumeUp", false); err != nil {
		t.Fatal(err)
	}
	if err := c.Hold("VolumeUp", false); err != nil {
		t.Fatal(err)
	}
	if err := c.Hold("VolumeUp", true); err != nil {
		t.Fatal(err)
	}
	if len(vol.steps) != 2 {
		t.Fatalf("volume did not step again after release: %v", vol.steps)
	}
}

// Chords that mix a volume key with a normal key still inject the normal key.
func TestVolumeKeyWithModifier(t *testing.T) {
	c := New()
	vol := &fakeVolume{}
	c.SetVolume(vol)
	injected := []nativeInput{}
	c.inject = func(items ...nativeInput) error { injected = append(injected, items...); return nil }

	if err := c.Chord("Ctrl+VolumeUp"); err != nil {
		t.Fatal(err)
	}
	if len(vol.steps) != 1 || !vol.steps[0] {
		t.Fatalf("volume step missing: %v", vol.steps)
	}
	// Ctrl down, then Ctrl up: the volume key itself is not injected.
	if len(injected) != 2 {
		t.Fatalf("expected only the modifier to be injected, got %d", len(injected))
	}
}

// Without a volume controller the key reports a clear error instead of silently
// injecting a key Windows will ignore.
func TestVolumeKeyWithoutController(t *testing.T) {
	c := New()
	c.inject = func(...nativeInput) error { return nil }
	if err := c.Chord("VolumeUp"); err == nil {
		t.Fatal("expected an error when no volume controller is attached")
	}
}
