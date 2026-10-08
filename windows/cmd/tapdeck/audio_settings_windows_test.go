//go:build windows

package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestSoundInputSettingsOpensRecordingTabAndFallsBack(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		var calls [][2]string
		err := launchSoundInputSettings(func(file, args string) error {
			calls = append(calls, [2]string{file, args})
			if len(calls) == 1 && fallback {
				return errors.New("control panel unavailable")
			}
			return nil
		})
		if err != nil || filepath.Base(calls[0][0]) != "control.exe" || calls[0][1] != "mmsys.cpl,,1" {
			t.Fatalf("unexpected recording target: %v, %v", calls, err)
		}
		if fallback {
			if len(calls) != 2 || calls[1] != [2]string{"ms-settings:sound-defaultinputproperties", ""} {
				t.Fatalf("unexpected fallback: %v", calls)
			}
		} else if len(calls) != 1 {
			t.Fatal("opened more than one settings window")
		}
	}
	err := launchSoundInputSettings(func(string, string) error { return errors.New("cannot open") })
	if err == nil {
		t.Fatal("both launch failures were ignored")
	}
}
