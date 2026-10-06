//go:build windows

package autostart

import (
	"strings"
	"testing"
)

func TestRunNameAndCommand(t *testing.T) {
	command, err := Command()
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if !strings.HasSuffix(command, "--headless") {
		t.Fatalf("sign-in command should start headless, got %q", command)
	}
	if !strings.HasPrefix(command, `"`) {
		t.Fatalf("executable path should be quoted, got %q", command)
	}
}

// The registry helpers must be usable without privileges: enabling, reading and
// disabling the per-user Run entry, leaving the machine as it was found.
func TestEnableDisableRoundTrip(t *testing.T) {
	before, beforeValue, err := Enabled()
	if err != nil {
		t.Skipf("Run key unavailable: %v", err)
	}
	defer func() {
		if before {
			if err := Enable(); err != nil {
				t.Errorf("restore enable: %v", err)
			}
			return
		}
		if err := Disable(); err != nil {
			t.Errorf("restore disable: %v", err)
		}
	}()

	if _, err := Set(true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	on, value, err := Enabled()
	if err != nil {
		t.Fatalf("read after enable: %v", err)
	}
	if !on || !strings.Contains(value, "--headless") {
		t.Fatalf("Enable left %v %q", on, value)
	}
	if summary := Summary(); !strings.Contains(summary, "已开启") {
		t.Fatalf("Summary after enable: %q", summary)
	}

	if _, err := Set(false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if on, _, err = Enabled(); err != nil || on {
		t.Fatalf("Disable left the entry enabled: %v %v", on, err)
	}
	// Disabling twice must stay harmless.
	if err := Disable(); err != nil {
		t.Fatalf("second disable: %v", err)
	}
	if beforeValue != "" && before && !strings.Contains(beforeValue, "--headless") {
		t.Logf("previous entry value was %q", beforeValue)
	}
}
