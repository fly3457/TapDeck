package main

import (
	"strings"
	"tapdeck/internal/config"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
)

const voiceHotkeyHintText = "热键：在手机端激活语音时触发，一般设置为PC端的语音输入法快捷键，留空则只传输音频。"

type voiceProfileEditor struct {
	enabled                *walk.CheckBox
	name                   *walk.LineEdit
	mode                   *walk.ComboBox
	hold, start, stop      *walk.LineEdit
	holdPanel, togglePanel *walk.Composite
	body                   *walk.Composite
}

func (e *voiceProfileEditor) showMode() {
	if e.mode == nil || e.holdPanel == nil || e.togglePanel == nil {
		return
	}
	hold := e.mode.CurrentIndex() == 0
	e.holdPanel.SetVisible(hold)
	e.togglePanel.SetVisible(!hold)
}

func (e *voiceProfileEditor) syncEnabled() {
	if e.body == nil || e.enabled == nil {
		return
	}
	e.body.SetVisible(e.enabled.Checked())
	e.showMode()
}

func (e *voiceProfileEditor) value(id string) config.VoiceProfile {
	mode := "hold"
	if e.mode.CurrentIndex() == 1 {
		mode = "toggle"
	}
	return config.VoiceProfile{ID: id, Name: strings.TrimSpace(e.name.Text()), Enabled: e.enabled.Checked(), Mode: mode,
		HoldKey: e.hold.Text(), ToggleStartKey: e.start.Text(), ToggleStopKey: e.stop.Text()}
}

func voiceFieldLabel(text string) d.Label {
	return d.Label{Text: text, MinSize: d.Size{Width: 66}, MaxSize: d.Size{Width: 66}}
}

func voiceKeyRow(owner func() walk.Form, label string, target **walk.LineEdit, value string) d.Composite {
	widgets := keyWidgets(owner, target, value)
	button := widgets[2].(d.PushButton)
	button.MinSize, button.MaxSize = d.Size{Width: 60}, d.Size{Width: 60}
	widgets[2] = button
	return d.Composite{Layout: d.HBox{MarginsZero: true}, Children: append([]d.Widget{voiceFieldLabel(label)}, widgets...)}
}

func voiceProfileWidgets(owner func() walk.Form, editors *[config.VoiceProfileCount]voiceProfileEditor, profiles []config.VoiceProfile) d.Widget {
	groups := make([]d.Widget, 0, len(profiles)+1)
	for i, p := range profiles {
		e := &editors[i]
		index := 0
		if p.Mode == "toggle" {
			index = 1
		}
		groups = append(groups, d.Composite{Layout: d.VBox{Margins: d.Margins{Left: 6, Right: 6, Top: 4, Bottom: 6}}, Children: []d.Widget{
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.CheckBox{AssignTo: &e.enabled, Text: "启用", Checked: p.Enabled, MinSize: d.Size{Width: 54}, MaxSize: d.Size{Width: 54}, OnCheckedChanged: e.syncEnabled},
				d.Label{Text: "名称"},
				d.LineEdit{AssignTo: &e.name, Text: p.Name, CueBanner: "配置名称", StretchFactor: 1},
				d.Label{Text: "类型"},
				d.ComboBox{AssignTo: &e.mode, Model: []string{"长按", "单击"}, CurrentIndex: index, MinSize: d.Size{Width: 80}, MaxSize: d.Size{Width: 90}, OnCurrentIndexChanged: e.showMode},
			}},
			d.Composite{AssignTo: &e.body, Visible: p.Enabled, Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
				d.Composite{AssignTo: &e.holdPanel, Visible: p.Mode == "hold", Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
					voiceKeyRow(owner, "长按热键", &e.hold, p.HoldKey),
				}},
				d.Composite{AssignTo: &e.togglePanel, Visible: p.Mode == "toggle", Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
					voiceKeyRow(owner, "开始热键", &e.start, p.ToggleStartKey),
					voiceKeyRow(owner, "结束热键", &e.stop, p.ToggleStopKey),
				}},
			}},
			horizontalRule(),
		}})
	}
	groups = append(groups, d.VSpacer{})
	// Reserve space for the vertical scrollbar without narrowing the content.
	return d.ScrollView{Layout: d.VBox{Margins: d.Margins{Left: 4, Top: 4, Right: 24, Bottom: 4}}, Children: groups, MinSize: d.Size{Height: 130}, StretchFactor: 1}
}
