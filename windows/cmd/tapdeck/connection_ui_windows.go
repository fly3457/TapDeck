package main

import (
	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

func connectionIntro(address **walk.LineEdit, qr **walk.ImageView, url string, copyURL, openURL func()) []d.Widget {
	return []d.Widget{
		d.Label{Text: "手机和电脑连接同一局域网。"},
		d.Composite{Layout: d.HBox{Spacing: 20}, Children: []d.Widget{
			d.ImageView{AssignTo: qr, MinSize: d.Size{Width: 200, Height: 200}, MaxSize: d.Size{Width: 200, Height: 200}, Mode: d.ImageViewModeIdeal},
			d.Composite{Layout: d.VBox{MarginsZero: true, Spacing: 14}, Children: []d.Widget{
				d.Label{Text: "连接手机", Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true}},
				d.TextLabel{Text: "1  扫码安装或打开 TapDeck\n已安装可在 App 内扫码填写网址。", MinSize: d.Size{Width: 200}},
				d.Label{Text: "2  在手机上点击“连接”"},
				d.TextLabel{Text: "3  核对电脑弹窗中的校验码\n一致后点击“允许连接”。", MinSize: d.Size{Width: 200}},
				d.VSpacer{},
			}},
		}},
		d.LineEdit{AssignTo: address, Text: url, ReadOnly: true},
		d.Composite{Layout: d.HBox{}, Children: []d.Widget{
			d.PushButton{Text: "复制网址", OnClicked: copyURL},
			d.PushButton{Text: "打开安装与配对网页", OnClicked: openURL}, d.HSpacer{},
		}},
		horizontalRule(),
	}
}

func updateSaveVisibility(tabs *walk.TabWidget, save *walk.PushButton) {
	if tabs != nil && save != nil {
		index := tabs.CurrentIndex()
		save.SetVisible(index >= 1 && index <= 3)
	}
}

func restoreSettingsWindow(mw *walk.MainWindow) {
	mw.Show()
	if win.IsIconic(mw.Handle()) {
		win.ShowWindow(mw.Handle(), win.SW_RESTORE)
	}
	mw.Activate()
}

// This pinned Walk version gives VSeparator horizontal growth flags and an
// SS_ETCHEDHORZ control; HSeparator incorrectly consumes vertical space.
func horizontalRule() d.VSeparator {
	return d.VSeparator{MinSize: d.Size{Height: 2}, MaxSize: d.Size{Height: 2}}
}
