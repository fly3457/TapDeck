package main

import (
	"testing"

	"tapdeck/internal/input"
)

// held builds the modifier state used by chordWithModifiers.
func held(vks ...uint16) func(uint16) bool {
	set := map[uint16]bool{}
	for _, vk := range vks {
		set[vk] = true
	}
	return func(vk uint16) bool { return set[vk] }
}

func TestChordWithModifiersKeepsSide(t *testing.T) {
	cases := []struct {
		name string
		main string
		down func(uint16) bool
		want string
	}{
		{"right ctrl only", "M", held(0xA3), "RightCtrl+M"},
		{"left ctrl only", "M", held(0xA2), "LeftCtrl+M"},
		{"right alt only", "M", held(0xA5), "RightAlt+M"},
		{"left alt only", "M", held(0xA4), "LeftAlt+M"},
		{"right shift only", "L", held(0xA1), "RightShift+L"},
		{"both ctrl sides", "M", held(0xA2, 0xA3), "LeftCtrl+RightCtrl+M"},
		{"ctrl and shift", "F9", held(0xA3, 0xA0), "RightCtrl+LeftShift+F9"},
		{"no modifier", "Enter", held(), "Enter"},
		{"modifier only", "", held(0xA3), ""},
	}
	for _, c := range cases {
		if got := chordWithModifiers(c.main, c.down); got != c.want {
			t.Errorf("%s: chordWithModifiers = %q, want %q", c.name, got, c.want)
		}
	}
}

// The recorded text must be accepted by the chord parser that the injection
// backends use, and must map to the side-specific virtual key codes.
func TestRecordedChordsParse(t *testing.T) {
	cases := []struct {
		chord string
		want  []uint16
	}{
		{"RightCtrl+M", []uint16{0xA3, 'M'}},
		{"RightAlt+M", []uint16{0xA5, 'M'}},
		{"LeftCtrl+RightShift+K", []uint16{0xA2, 0xA1, 'K'}},
		{"Enter", []uint16{0x0D}},
	}
	for _, c := range cases {
		keys, err := input.ParseChord(c.chord)
		if err != nil {
			t.Fatalf("%s: %v", c.chord, err)
		}
		if len(keys) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.chord, keys, c.want)
		}
		for i := range keys {
			if keys[i] != c.want[i] {
				t.Fatalf("%s: got %v, want %v", c.chord, keys, c.want)
			}
		}
	}
}

// Regression: recording a right-hand chord must not collapse to the left key,
// which Doubao and other IMEs reject for their global voice hotkey.
func TestRightSideIsNotCollapsedToLeft(t *testing.T) {
	recorded := chordWithModifiers("M", held(0xA3))
	if recorded == "Ctrl+M" {
		t.Fatal("right Ctrl was recorded as the generic left Ctrl")
	}
	keys, err := input.ParseChord(recorded)
	if err != nil || len(keys) == 0 || keys[0] != 0xA3 {
		t.Fatalf("recorded %q parses to %v (%v), want RightCtrl first", recorded, keys, err)
	}
}
