//go:build windows

package hidkeyboard

import "testing"

func TestKeyboardWireLayoutAndSides(t *testing.T) {
	r, e := Report(map[uint16]int{0xA3: 1, 'M': 1})
	if e != nil || r[0] != 0x40 || r[1] != 9 || r[2] != 1 || r[3] != 16 || r[4] != 0 || r[5] != 0x10 {
		t.Fatal(r, e)
	}
	mods := []uint16{0xA2, 0xA0, 0xA4, 0x5B, 0xA3, 0xA1, 0xA5, 0x5C}
	for i, k := range mods {
		_, m, ok := Usage(k)
		if !ok || m != 1<<i {
			t.Fatalf("modifier %X: %X", k, m)
		}
	}
	empty, e := Report(nil)
	if e != nil || empty != [65]byte{0x40, 9, 1} {
		t.Fatal("all-up report", empty, e)
	}
}
func TestDescriptorLimits(t *testing.T) {
	for k := uint16(0x7C); k <= 0x87; k++ {
		if _, _, ok := Usage(k); ok {
			t.Fatalf("F13-F24 must use fallback: %X", k)
		}
	}
	if _, e := Report(map[uint16]int{'A': 1, 'B': 1, 'C': 1, 'D': 1, 'E': 1, 'F': 1, 'G': 1}); e == nil {
		t.Fatal("rollover overflow accepted")
	}
	r, e := Report(map[uint16]int{'B': 1, 'A': 2, 0xA2: 2, 'C': 0})
	if e != nil || r[3] != 1 || r[5] != 4 || r[6] != 5 || r[7] != 0 {
		t.Fatal("reference count report", r, e)
	}
}
