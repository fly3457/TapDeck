//go:build windows

package driver

import "testing"

func TestDetectRecognizesExactFakerInputHardwareIdentity(t *testing.T) {
	for _, tc := range []struct {
		ids  []string
		want bool
	}{
		{nil, false},
		{[]string{`root\FakerInput`}, true},
		{[]string{"another device", `ROOT\FAKERINPUT`}, true},
		{[]string{`root\FakerInputOther`, `USB\FakerInput`}, false},
	} {
		if got := isFakerInput(tc.ids); got != tc.want {
			t.Fatalf("%v: got %v, want %v", tc.ids, got, tc.want)
		}
	}
	if state, err := Detect(true); err != nil || state != Ready {
		t.Fatalf("available HID incorrectly classified: %s, %v", state, err)
	}
}
