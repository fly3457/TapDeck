//go:build windows

package input

import "fmt"

var asyncKeyState = user32.NewProc("GetAsyncKeyState")
var doubleClickTime = user32.NewProc("GetDoubleClickTime")

type Modifiers struct{ Ctrl, Shift, Alt, Win bool }

func CurrentModifiers() Modifiers {
	down := func(keys ...uint16) bool {
		for _, key := range keys {
			state, _, _ := asyncKeyState.Call(uintptr(key))
			if state&0x8000 != 0 {
				return true
			}
		}
		return false
	}
	return Modifiers{Ctrl: down(0xA2, 0xA3), Shift: down(0xA0, 0xA1), Alt: down(0xA4, 0xA5), Win: down(0x5B, 0x5C)}
}

func DoubleClickTime() uint32 {
	value, _, _ := doubleClickTime.Call()
	if value == 0 || value > 5000 {
		return 500
	}
	return uint32(value)
}

// ZoomSteps is one SendInput batch, so other mouse input cannot run between
// the temporary Ctrl press, wheel and release. Existing Ctrl belongs to its
// original owner and must never be released by a pinch.
func (c *Controller) ZoomSteps(steps int, ctrlHeld bool) error {
	if steps == 0 || steps < -4 || steps > 4 {
		return fmt.Errorf("无效缩放步进")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	temporary := !ctrlHeld && c.keys[0xA2] == 0 && c.keys[0xA3] == 0
	items := []nativeInput{}
	if temporary {
		items = append(items, keyInput(0xA2, false))
	}
	items = append(items, nativeInput{Data: uint32(int32(steps * 120)), Flags: 0x800})
	if temporary {
		items = append(items, keyInput(0xA2, true))
	}
	if err := c.inject(items...); err != nil {
		// SendInput can accept only the first part of the batch. Clean up only
		// our temporary modifier; held software/HID/physical Ctrl stays intact.
		if temporary {
			_ = c.inject(keyInput(0xA2, true))
		}
		return err
	}
	return nil
}
