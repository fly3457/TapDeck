package main

import (
	"fmt"
	"strings"
	"tapdeck/internal/config"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
)

type voiceProfileEditor struct {
	enabled                *walk.CheckBox
	name                   *walk.LineEdit
	mode                   *walk.ComboBox
	hold, start, stop      *walk.LineEdit
	holdPanel, togglePanel *walk.Composite
	body                   *walk.Composite
	title                  *walk.Label
	fold                   *walk.PushButton
	expanded               bool
	number                 int
}

func (e *voiceProfileEditor) showMode() {
	if e.mode == nil || e.holdPanel == nil || e.togglePanel == nil {
		return
	}
	hold := e.mode.CurrentIndex() == 0
	e.holdPanel.SetVisible(hold)
	e.togglePanel.SetVisible(!hold)
}

func (e *voiceProfileEditor) updateTitle() {
	if e.name == nil || e.title == nil {
		return
	}
	name := strings.TrimSpace(e.name.Text())
	if name == "" {
		name = fmt.Sprintf("语音配置 %d", e.number)
	}
	_ = e.title.SetText(name)
}

func (e *voiceProfileEditor) setExpanded(expanded bool) {
	e.expanded = expanded
	if e.body == nil || e.fold == nil {
		return
	}
	e.body.SetVisible(expanded)
	arrow, hint := "›", "展开配置"
	if expanded {
		arrow, hint = "⌄", "收起配置"
	}
	_ = e.fold.SetText(arrow)
	_ = e.fold.SetToolTipText(hint)
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
		e.number, e.expanded = i+1, p.Enabled
		index := 0
		if p.Mode == "toggle" {
			index = 1
		}
		arrow, hint := "›", "展开配置"
		if p.Enabled {
			arrow, hint = "⌄", "收起配置"
		}
		groups = append(groups, d.Composite{Layout: d.VBox{Margins: d.Margins{Left: 6, Right: 6, Top: 4, Bottom: 6}}, Children: []d.Widget{
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.CheckBox{AssignTo: &e.enabled, Text: "启用", Checked: p.Enabled, MaxSize: d.Size{Width: 60}},
				d.Label{AssignTo: &e.title, Text: p.Name, Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 9, Bold: true}},
				d.HSpacer{},
				d.PushButton{AssignTo: &e.fold, Text: arrow, ToolTipText: hint, MinSize: d.Size{Width: 32}, MaxSize: d.Size{Width: 32}, OnClicked: func() { e.setExpanded(!e.expanded) }},
			}},
			d.Composite{AssignTo: &e.body, Visible: p.Enabled, Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
				d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
					voiceFieldLabel("名称"),
					d.LineEdit{AssignTo: &e.name, Text: p.Name, CueBanner: "配置名称", StretchFactor: 1, OnTextChanged: e.updateTitle},
					d.Label{Text: "类型"},
					d.ComboBox{AssignTo: &e.mode, Model: []string{"长按", "单击"}, CurrentIndex: index, MinSize: d.Size{Width: 80}, MaxSize: d.Size{Width: 90}, OnCurrentIndexChanged: e.showMode},
				}},
				d.Composite{AssignTo: &e.holdPanel, Visible: p.Mode == "hold", Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
					voiceKeyRow(owner, "触发热键", &e.hold, p.HoldKey),
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
