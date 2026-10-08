//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

const voiceRoutingText = "TapDeck 输出选择 CABLE Input；\n系统音频输入或目标输入法的麦克风选择 CABLE Output。"
const cableAttributionText = "VB-CABLE 来自 VB-Audio，是 donationware。"

func voiceRoutingHint() d.TextLabel {
	return d.TextLabel{Text: voiceRoutingText, TextColor: walk.RGB(200, 35, 35), MinSize: d.Size{Width: 100}}
}

// Start with the recording-device list, where users can select CABLE Output
// as the Windows default input. This only opens UI; it never changes a device.
func launchSoundInputSettings(launch func(string, string) error) error {
	err := launch(filepath.Join(os.Getenv("WINDIR"), "System32", "control.exe"), "mmsys.cpl,,1")
	if err == nil {
		return nil
	}
	return launch("ms-settings:sound-defaultinputproperties", "")
}

func shellOpen(file, parameters string) error {
	p, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	args, err := windows.UTF16PtrFromString(parameters)
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	result, _, _ := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(args)), 0, 1)
	if result <= 32 {
		return fmt.Errorf("无法打开系统音频输入设置（Windows 错误 %d）", result)
	}
	return nil
}
