package main

import (
	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"strconv"
	"tapdeck/internal/config"
)

func settingsSection(title string, stretch int, children ...d.Widget) d.GroupBox {
	return d.GroupBox{Title: title, Layout: d.VBox{}, StretchFactor: stretch, Children: children}
}

func shortcutSettingsWidgets(owner func() walk.Form, labels, keys *[config.ShortcutCount]*walk.LineEdit, enabled *[config.ShortcutCount]*walk.CheckBox, shortcuts []config.Shortcut) d.GroupBox {
	rows := []d.Widget{d.Label{Text: "勾选 1–8 个快捷键。可直接编辑、录入组合键，或选择左右修饰键。"}}
	for i, shortcut := range shortcuts {
		row := []d.Widget{
			d.CheckBox{AssignTo: &enabled[i], Text: strconv.Itoa(i + 1), Checked: shortcut.Enabled},
			d.LineEdit{AssignTo: &labels[i], Text: shortcut.Label, MinSize: d.Size{Width: 90}, MaxSize: d.Size{Width: 140}},
		}
		rows = append(rows, d.Composite{Layout: d.HBox{MarginsZero: true}, Children: append(row, keyWidgets(owner, &keys[i], shortcut.Chord)...)})
	}
	return settingsSection("快捷键设置", 1, append(rows, d.VSpacer{})...)
}

func keyboardEnvironmentWidgets(backend **walk.ComboBox, names []string, selected int, status **walk.Label, installButton **walk.PushButton, install, detect func()) d.GroupBox {
	return settingsSection("键盘环境", 0,
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.Label{Text: "键盘发送方式"},
			d.ComboBox{AssignTo: backend, Model: names, CurrentIndex: selected, StretchFactor: 1},
		}},
		d.Label{AssignTo: status, Text: "正在检测虚拟键盘…"},
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.PushButton{AssignTo: installButton, Text: "安装 / 修复虚拟键盘", OnClicked: install},
			d.PushButton{Text: "重新检测", OnClicked: detect},
			d.HSpacer{},
		}},
	)
}

type voiceEnvironmentOptions struct {
	cableStatus       **walk.TextLabel
	cableButton       **walk.PushButton
	level             **walk.Label
	devices           **walk.ComboBox
	gain              **walk.NumberEdit
	deviceNames       []string
	selectedDevice    int
	initialGain       float64
	install, detect   func()
	website, license  func()
	refresh, settings func()
}

func voiceEnvironmentWidgets(o voiceEnvironmentOptions) d.GroupBox {
	return settingsSection("语音输入环境", 0,
		voiceRoutingHint(),
		d.TextLabel{AssignTo: o.cableStatus, Text: "正在检测 VB-CABLE…", MinSize: d.Size{Width: 100}},
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.PushButton{AssignTo: o.cableButton, Text: "安装虚拟声卡", OnClicked: o.install},
			d.PushButton{Text: "重新检测", OnClicked: o.detect},
			d.PushButton{Text: "VB-Audio 官网", OnClicked: o.website},
			d.PushButton{Text: "原包许可", OnClicked: o.license},
			d.HSpacer{},
		}},
		d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
			d.Label{Text: "音量倍率（0–3）"},
			d.NumberEdit{AssignTo: o.gain, Value: o.initialGain, MinValue: 0, MaxValue: 3, Decimals: 2, Increment: 0.1},
			d.HSpacer{},
		}},
		audioDeviceRow(o.level, o.devices, o.deviceNames, o.selectedDevice, o.refresh, o.settings),
	)
}
