//go:build windows

package main

import (
	"errors"
	"testing"

	"tapdeck/internal/driver"
)

func TestKeyboardDriverPromptMissingUnavailableAndReady(t *testing.T) {
	for _, tc := range []struct {
		state driver.State
		title string
	}{{driver.Missing, "安装虚拟键盘"}, {driver.Unavailable, "修复虚拟键盘"}, {driver.Ready, ""}} {
		t.Run(string(tc.state), func(t *testing.T) {
			var prompt keyboardDriverPrompt
			title, _ := prompt.Take(true, false, tc.state, nil)
			if title != tc.title {
				t.Fatalf("title = %q, want %q", title, tc.title)
			}
			if title, _ := prompt.Take(true, false, tc.state, nil); title != "" {
				t.Fatal("prompt repeated after dismissal")
			}
		})
	}
}

func TestKeyboardDriverPromptDefersUntilVisibleIdleAndDetected(t *testing.T) {
	for _, tc := range []struct {
		name    string
		visible bool
		busy    bool
		err     error
	}{{"hidden startup", false, false, nil}, {"active input or installer", true, true, nil}, {"detection failure", true, false, errors.New("access denied")}} {
		t.Run(tc.name, func(t *testing.T) {
			var prompt keyboardDriverPrompt
			if title, _ := prompt.Take(tc.visible, tc.busy, driver.Missing, tc.err); title != "" {
				t.Fatal("unsafe or hidden prompt")
			}
			if title, _ := prompt.Take(true, false, driver.Missing, nil); title != "安装虚拟键盘" {
				t.Fatal("first visible check was lost")
			}
		})
	}
}
