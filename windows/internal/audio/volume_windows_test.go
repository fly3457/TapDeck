//go:build windows

package audio

import (
	"testing"

	ole "github.com/go-ole/go-ole"
)

// Volume keys cannot be injected as key events on this platform, so TapDeck
// drives the endpoint directly. This test verifies that the real controller
// moves the default render endpoint and restores it.
func TestVolumeControlChangesEndpoint(t *testing.T) {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		t.Skip(err)
	}
	v, err := NewVolumeControl()
	if err != nil {
		t.Skipf("no default render endpoint: %v", err)
	}
	defer v.Close()

	level, err := v.Level()
	if err != nil {
		t.Fatal(err)
	}
	muted, err := v.Mute()
	if err != nil {
		t.Fatal(err)
	}
	// Always leave the endpoint as it was found.
	defer func() {
		_ = v.Step(false)
		_ = v.Step(true)
		if muted {
			_ = v.ToggleMute()
		}
	}()

	if err := v.Step(true); err != nil {
		t.Fatal(err)
	}
	raised, err := v.Level()
	if err != nil {
		t.Fatal(err)
	}
	if raised <= level {
		t.Fatalf("volume did not rise: %.3f -> %.3f", level, raised)
	}
	if err := v.Step(false); err != nil {
		t.Fatal(err)
	}
	lowered, err := v.Level()
	if err != nil {
		t.Fatal(err)
	}
	if lowered >= raised {
		t.Fatalf("volume did not fall: %.3f -> %.3f", raised, lowered)
	}

	if err := v.ToggleMute(); err != nil {
		t.Fatal(err)
	}
	nowMuted, err := v.Mute()
	if err != nil {
		t.Fatal(err)
	}
	if nowMuted == muted {
		t.Fatalf("mute did not toggle: was %v, now %v", muted, nowMuted)
	}
	if err := v.ToggleMute(); err != nil {
		t.Fatal(err)
	}
	back, err := v.Mute()
	if err != nil {
		t.Fatal(err)
	}
	if back != muted {
		t.Fatalf("mute not restored: want %v, got %v", muted, back)
	}
}
