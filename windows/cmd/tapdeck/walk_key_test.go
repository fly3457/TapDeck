//go:build windows

package main

import (
	"testing"

	"github.com/lxn/walk"
	"tapdeck/internal/input"
)

// Regression: the recording dialog used walk's own key names, which the chord
// parser does not accept ("Back", "Escape", "Prior", "Next", "VolumeUp"), so
// Backspace, Esc and the volume keys could never be saved.
func TestWalkKeysRecordToParsableNames(t *testing.T) {
	for _, name := range []string{"Backspace", "Esc", "VolumeUp", "VolumeDown", "VolumeMute", "MediaPlayPause", "PageUp", "PageDown", "Enter", "Delete", "Tab", "Space", "F9"} {
		keys, err := input.ParseChord(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(keys) != 1 {
			t.Fatalf("%s: %v", name, keys)
		}
		recorded := walkKeyName(walk.Key(keys[0]))
		if recorded != name {
			t.Fatalf("VK 0x%02X recorded as %q, want %q", keys[0], recorded, name)
		}
	}
}

func TestWalkKeyNameUnsupported(t *testing.T) {
	if name := walkKeyName(walk.Key(0xFF)); name != "" {
		t.Fatalf("unmapped key produced %q", name)
	}
}

// The recorded chord for a right-hand modifier must keep its side.
func TestWalkKeyChordKeepsSide(t *testing.T) {
	keys, err := input.ParseChord("RightCtrl+M")
	if err != nil {
		t.Fatal(err)
	}
	chord := chordWithModifiers(walkKeyName(walk.Key(keys[1])), func(vk uint16) bool { return vk == 0xA3 })
	if chord != "RightCtrl+M" {
		t.Fatalf("recorded %q, want RightCtrl+M", chord)
	}
}
