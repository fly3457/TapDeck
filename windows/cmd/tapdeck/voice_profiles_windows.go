package main

import (
	"fmt"
	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"strings"
	"tapdeck/internal/config"
)

type voiceProfileEditor struct {
	enabled                *walk.CheckBox
	name                   *walk.LineEdit
	mode                   *walk.ComboBox
	hold, start, stop      *walk.LineEdit
	holdPanel, togglePanel *walk.Composite
}

func (e *voiceProfileEditor) showMode() {
	if e.mode == nil || e.holdPanel == nil || e.togglePanel == nil {
		return
	}
	hold := e.mode.CurrentIndex() == 0
	e.holdPanel.SetVisible(hold)
	e.togglePanel.SetVisible(!hold)
}

func (e *voiceProfileEditor) value(id string) config.VoiceProfile {
	mode := "hold"
	if e.mode.CurrentIndex() == 1 {
		mode = "toggle"
	}
	return config.VoiceProfile{ID: id, Name: strings.TrimSpace(e.name.Text()), Enabled: e.enabled.Checked(), Mode: mode,
		HoldKey: e.hold.Text(), ToggleStartKey: e.start.Text(), ToggleStopKey: e.stop.Text()}
}

func voiceProfileWidgets(owner func() walk.Form, editors *[config.VoiceProfileCount]voiceProfileEditor, profiles []config.VoiceProfile) d.Widget {
	groups := make([]d.Widget, 0, len(profiles)+1)
	for i, p := range profiles {
		e := &editors[i]
		index := 0
		if p.Mode == "toggle" {
			index = 1
		}
		groups = append(groups, d.GroupBox{Title: fmt.Sprintf("语音配置 %d", i+1), Layout: d.VBox{}, Children: []d.Widget{
			d.Composite{Layout: d.HBox{MarginsZero: true}, Children: []d.Widget{
				d.CheckBox{AssignTo: &e.enabled, Text: "启用", Checked: p.Enabled, MaxSize: d.Size{Width: 60}},
				d.LineEdit{AssignTo: &e.name, Text: p.Name, CueBanner: "配置名称", StretchFactor: 1},
				d.ComboBox{AssignTo: &e.mode, Model: []string{"长按", "单击"}, CurrentIndex: index, MinSize: d.Size{Width: 80}, OnCurrentIndexChanged: e.showMode},
			}},
			d.Composite{AssignTo: &e.holdPanel, Visible: p.Mode == "hold", Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
				d.Label{Text: "触发热键（按住触发，松手释放）"}, d.Composite{Layout: d.HBox{MarginsZero: true}, Children: keyWidgets(owner, &e.hold, p.HoldKey)},
			}},
			d.Composite{AssignTo: &e.togglePanel, Visible: p.Mode == "toggle", Layout: d.VBox{MarginsZero: true}, Children: []d.Widget{
				d.Label{Text: "开始热键（单击开始录音）"}, d.Composite{Layout: d.HBox{MarginsZero: true}, Children: keyWidgets(owner, &e.start, p.ToggleStartKey)},
				d.Label{Text: "结束热键（再次单击停止）"}, d.Composite{Layout: d.HBox{MarginsZero: true}, Children: keyWidgets(owner, &e.stop, p.ToggleStopKey)},
			}},
		}})
	}
	groups = append(groups, d.VSpacer{})
	// Walk's fixed-horizontal scroll view does not grow with its parent. Keep the
	// growing layout and reserve room for the vertical bar inside the content.
	return d.ScrollView{Layout: d.VBox{Margins: d.Margins{Left: 4, Top: 4, Right: 24, Bottom: 4}}, Children: groups, MinSize: d.Size{Height: 130}, StretchFactor: 1}
}
