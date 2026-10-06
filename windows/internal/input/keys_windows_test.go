//go:build windows

package input

import "testing"

// Every canonical name must be accepted by ParseChord and convert back to a VK.
func TestKeyTableRoundTrip(t *testing.T) {
	for _, k := range keys {
		got, err := ParseChord(k.name)
		if err != nil {
			t.Fatalf("%s: %v", k.name, err)
		}
		if len(got) != 1 || got[0] != k.vk {
			t.Fatalf("%s: got %v, want VK 0x%02X", k.name, got, k.vk)
		}
		if k.aliasOnly {
			continue
		}
		name, ok := KeyName(k.vk)
		if !ok || name != k.name {
			t.Fatalf("VK 0x%02X: KeyName = %q (%v), want %q", k.vk, name, ok, k.name)
		}
	}
}

// Special keys that could not be saved before the shared key table existed.
func TestSpecialKeysParse(t *testing.T) {
	cases := []struct {
		chord string
		want  uint16
	}{
		{"Backspace", 0x08},
		{"Esc", 0x1B},
		{"VolumeUp", 0xAF},
		{"VolumeDown", 0xAE},
		{"VolumeMute", 0xAD},
		{"MediaPlayPause", 0xB3},
		{"NumpadEnter", 0x0D},
		{"Numpad5", 0x65},
		{"PageUp", 0x21},
		{"PrintScreen", 0x2C},
		{"F13", 0x7C},
		{"F24", 0x87},
		{"Ctrl+Backspace", 0xA2},
	}
	for _, c := range cases {
		got, err := ParseChord(c.chord)
		if err != nil {
			t.Fatalf("%s: %v", c.chord, err)
		}
		if len(got) == 0 || got[0] != c.want {
			t.Fatalf("%s: got %v, want first VK 0x%02X", c.chord, got, c.want)
		}
	}
}

func TestKeyAliasesAndSpelling(t *testing.T) {
	cases := []struct {
		chord string
		want  []uint16
	}{
		{"escape", []uint16{0x1B}},
		{"Back", []uint16{0x08}},
		{"ctrl+m", []uint16{0xA2, 'M'}},
		{"Right Ctrl + M", []uint16{0xA3, 'M'}},
		{"volup", []uint16{0xAF}},
		{"Return", []uint16{0x0D}},
		{"Prior", []uint16{0x21}},
		{"Next", []uint16{0x22}},
	}
	for _, c := range cases {
		got, err := ParseChord(c.chord)
		if err != nil {
			t.Fatalf("%s: %v", c.chord, err)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.chord, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: got %v, want %v", c.chord, got, c.want)
			}
		}
	}
}

func TestInvalidKeysRejected(t *testing.T) {
	for _, chord := range []string{"Nope", "F25", "Ctrl+Ctrl", "Ctrl+LeftCtrl"} {
		if _, err := ParseChord(chord); err == nil {
			t.Fatalf("%s should be rejected", chord)
		}
	}
}
