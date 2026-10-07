//go:build windows

package input

import (
	"testing"
	"unsafe"
)

func TestSideModifiersAndReferenceOwnership(t *testing.T) {
	if unsafe.Sizeof(nativeInput{}) != 40 {
		t.Fatal("incorrect INPUT layout")
	}
	c := New()
	var events []nativeInput
	c.inject = func(items ...nativeInput) error { events = append(events, items...); return nil }
	if err := c.Hold("LeftCtrl", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Chord("Ctrl+C"); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[1].DX != 'C' || events[2].DX != 'C' {
		t.Fatal("shortcut released the held modifier", events)
	}
	if err := c.Hold("Ctrl", false); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[3].DY&2 == 0 {
		t.Fatal("alias did not release same key")
	}
	if _, err := ParseChord("Ctrl+LeftCtrl"); err == nil {
		t.Fatal("duplicate physical key allowed")
	}
}

func TestSixPhysicalModifierEncodings(t *testing.T) {
	for _, item := range []struct {
		name     string
		vk, scan uint16
		extended bool
	}{
		{"LeftAlt", 0xA4, 0x38, false}, {"RightAlt", 0xA5, 0x38, true},
		{"LeftCtrl", 0xA2, 0x1D, false}, {"RightCtrl", 0xA3, 0x1D, true},
		{"LeftShift", 0xA0, 0x2A, false}, {"RightShift", 0xA1, 0x36, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			keys, err := ParseChord(item.name)
			if err != nil || len(keys) != 1 || keys[0] != item.vk {
				t.Fatal(keys, err)
			}
			down, up := keyInput(keys[0], false), keyInput(keys[0], true)
			if down.Kind != 1 || down.DX != int32(item.scan)<<16 || down.DY&8 == 0 || (down.DY&1 != 0) != item.extended || up.DY != down.DY|2 {
				t.Fatal("incorrect scan code or extended flag", down, up)
			}
		})
	}
	if _, err := ParseChord("RightAlt+F9"); err != nil {
		t.Fatal(err)
	}
}

// 键盘上没有的字符（…）按 Unicode 字符发送：wVk=0、wScan=码点、KEYEVENTF_UNICODE。
func TestUnicodeOnlyKey(t *testing.T) {
	keys, err := ParseChord("Ellipsis")
	if err != nil || len(keys) != 1 || keys[0] != unicodeVK {
		t.Fatal(keys, err)
	}
	down, up := keyInput(keys[0], false), keyInput(keys[0], true)
	if down.Kind != 1 || down.DX != 0 || down.DY&0xFFFF != 4 || up.DY&0xFFFF != 6 {
		t.Fatal("incorrect unicode input", down, up)
	}
	if rune(uint16(down.DY>>16)) != '…' {
		t.Fatal("incorrect code point", down)
	}
	if name, ok := KeyName(unicodeVK); !ok || name != "Ellipsis" {
		t.Fatal(name, ok)
	}
}
