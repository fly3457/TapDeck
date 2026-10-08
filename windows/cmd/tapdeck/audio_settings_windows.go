//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tapdeck/internal/vbcable"
	"unsafe"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

const voiceRoutingText = "TapDeck 输出选择 CABLE Input；系统音频输入或目标输入法的麦克风选择 CABLE Output。"
const cableAttributionText = "VB-CABLE 是 donationware 来自 VB-Audio。"

func cableStatusText(status vbcable.Status, err error) string {
	if err != nil {
		return "VB-CABLE 检测失败：" + err.Error() + "。" + cableAttributionText
	}
	return strings.Replace(status.Text(), "可用：", "可用；", 1) + "。" + cableAttributionText
}

func audioDeviceRow(level **walk.Label, devices **walk.ComboBox, names []string, selected int, refresh, settings func()) d.Composite {
	return d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
		d.Label{AssignTo: level, Text: "输入电平 0%", MinSize: d.Size{Width: 88}, MaxSize: d.Size{Width: 88}},
		d.ComboBox{AssignTo: devices, Model: names, CurrentIndex: selected, StretchFactor: 1, MinSize: d.Size{Width: 180}},
		// Allocate the fixed actions first so Walk gives the readonly selector
		// all remaining width (its ComboBox is growable but not greedy).
		d.Composite{Layout: d.HBox{MarginsZero: true}, MinSize: d.Size{Width: 262}, MaxSize: d.Size{Width: 262}, Children: []d.Widget{
			d.PushButton{Text: "刷新音频设备", OnClicked: refresh, MinSize: d.Size{Width: 115}, MaxSize: d.Size{Width: 115}},
			d.PushButton{Text: "系统音频输入设置", OnClicked: settings, MinSize: d.Size{Width: 135}, MaxSize: d.Size{Width: 135}},
			d.HSpacer{},
		}},
	}}
}

func voiceRoutingHint() d.Label {
	return d.Label{Text: voiceRoutingText, TextColor: walk.RGB(200, 35, 35)}
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
