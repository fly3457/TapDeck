//go:build windows

package input

import (
	"errors"
	"testing"
)

func TestZoomInjectsCtrlWheelReleaseInOneBatch(t *testing.T) {
	for _, steps := range []int{-4, -1, 1, 4} {
		c := New()
		var batches [][]nativeInput
		c.inject = func(items ...nativeInput) error {
			batches = append(batches, append([]nativeInput(nil), items...))
			return nil
		}
		if err := c.ZoomSteps(steps, false); err != nil {
			t.Fatal(err)
		}
		if len(batches) != 1 || len(batches[0]) != 3 {
			t.Fatal("zoom is not atomic", batches)
		}
		batch := batches[0]
		if batch[0] != keyInput(0xA2, false) || batch[2] != keyInput(0xA2, true) || batch[1].Flags != 0x800 || int32(batch[1].Data) != int32(steps*120) {
			t.Fatal("wrong zoom sequence", batch)
		}
		if len(c.keys) != 0 {
			t.Fatal("temporary Ctrl left a held key", c.keys)
		}
	}
}
func TestZoomKeepsExistingSoftwareHIDAndPhysicalCtrl(t *testing.T) {
	for _, external := range []bool{false, true} {
		c := New()
		if !external {
			c.keys[0xA3] = 2
		}
		var batch []nativeInput
		c.inject = func(items ...nativeInput) error { batch = append(batch, items...); return nil }
		if err := c.ZoomSteps(1, external); err != nil {
			t.Fatal(err)
		}
		if len(batch) != 1 || batch[0].Kind != 0 || batch[0].Flags != 0x800 {
			t.Fatal("released another Ctrl owner", batch)
		}
		if !external && c.keys[0xA3] != 2 {
			t.Fatal("changed another owner's count")
		}
	}
}
func TestZoomFailureCleansOnlyItsTemporaryModifier(t *testing.T) {
	for _, held := range []bool{false, true} {
		c := New()
		var batches [][]nativeInput
		c.inject = func(items ...nativeInput) error {
			batches = append(batches, append([]nativeInput(nil), items...))
			if len(batches) == 1 {
				return errors.New("partial SendInput")
			}
			return nil
		}
		if err := c.ZoomSteps(1, held); err == nil {
			t.Fatal("ignored failed input")
		}
		if held && len(batches) != 1 {
			t.Fatal("cleaned someone else's Ctrl", batches)
		}
		if !held && (len(batches) != 2 || len(batches[1]) != 1 || batches[1][0] != keyInput(0xA2, true)) {
			t.Fatal("temporary Ctrl was not cleaned", batches)
		}
		if err := c.ZoomSteps(-1, held); err != nil {
			t.Fatal("retry failed", err)
		}
	}
}
func TestZoomRejectsInvalidStepsBeforeInjection(t *testing.T) {
	c := New()
	c.inject = func(...nativeInput) error { t.Fatal("invalid zoom injected input"); return nil }
	for _, steps := range []int{0, -5, 5, 1000000} {
		if c.ZoomSteps(steps, false) == nil {
			t.Fatal("accepted invalid step", steps)
		}
	}
}
