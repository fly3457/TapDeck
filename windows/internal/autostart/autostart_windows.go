//go:build windows

// Package autostart manages the per-user "run at sign-in" entry so the receiver
// is already listening when the phone reconnects after a reboot.
package autostart

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey  = `Software\Microsoft\Windows\CurrentVersion\Run`
	runName = "TapDeck"
)

// Command returns what the sign-in entry runs: the current executable with
// --headless, which starts the receiver and stops the console window from
// appearing at sign-in.
func Command() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`"%s" --headless`, exe), nil
}

// Enabled reports whether the current user has the sign-in entry, and returns
// the registered command so callers can show or repair it.
func Enabled() (bool, string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false, "", err
	}
	defer k.Close()
	value, _, err := k.GetStringValue(runName)
	if err == registry.ErrNotExist {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, value, nil
}

// Enable writes the sign-in entry for the current executable.
func Enable() error {
	command, err := Command()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runName, command)
}

// Disable removes the sign-in entry; a missing entry is not an error.
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()
	if err = k.DeleteValue(runName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// Set turns the sign-in entry on or off and reports whether it is enabled now.
func Set(enabled bool) (bool, error) {
	if enabled {
		if err := Enable(); err != nil {
			return false, err
		}
	} else if err := Disable(); err != nil {
		return false, err
	}
	on, _, err := Enabled()
	return on, err
}

// Summary is a short human-readable state for the settings window.
func Summary() string {
	on, value, err := Enabled()
	if err != nil {
		return "开机自启状态读取失败：" + err.Error()
	}
	if !on {
		return "开机自启：关闭（勾选后随 Windows 登录自动启动接收端）"
	}
	return "开机自启：已开启 · " + strings.TrimSpace(value)
}
