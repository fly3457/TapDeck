package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"tapdeck/internal/config"
	"testing"
	"time"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
)

func TestKeyboardSectionsUI(t *testing.T) {
	out := os.Getenv("TAPDECK_UI_OUTPUT")
	if out == "" {
		t.Skip("set TAPDECK_UI_OUTPUT to render the offscreen keyboard page")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer useSystemDPIForTest()()
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	for _, size := range []walk.Size{{Width: 740, Height: 800}, {Width: 680, Height: 700}} {
		var mw *walk.MainWindow
		var backend *walk.ComboBox
		var status *walk.Label
		var install *walk.PushButton
		var labels, keys [config.ShortcutCount]*walk.LineEdit
		var enabled [config.ShortcutCount]*walk.CheckBox
		var environment, settings *walk.GroupBox
		environmentWidget := keyboardEnvironmentWidgets(&backend, []string{"自动（优先虚拟键盘；不支持的键使用 SendInput）", "虚拟键盘（HID）", "软件按键（SendInput）"}, 0, &status, &install, func() {}, func() {})
		environmentWidget.AssignTo = &environment
		settingsWidget := shortcutSettingsWidgets(func() walk.Form { return mw }, &labels, &keys, &enabled, config.Default().Shortcuts)
		settingsWidget.AssignTo = &settings
		if err := (d.MainWindow{AssignTo: &mw, Title: "TapDeck · 快捷键布局验证", Size: d.Size{Width: size.Width, Height: size.Height},
			Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 9}, Layout: d.VBox{Margins: d.Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}}, Children: []d.Widget{
				d.Label{Text: "已连接：布局验证设备"},
				d.TabWidget{Pages: []d.TabPage{{Title: "快捷键", Layout: d.VBox{}, Children: []d.Widget{environmentWidget, settingsWidget}}}},
				d.PushButton{Text: "保存并同步配置"},
			}}).Create(); err != nil {
			t.Fatal(err)
		}
		status.SetText("FakerInput 虚拟键盘已就绪\n实际发送：HID · 驱动：FakerInput 0.1.1 · API 1")
		placeTestWindow(mw, size)
		mw.Show()
		var failure error
		go func() {
			time.Sleep(600 * time.Millisecond)
			mw.Synchronize(func() {
				if err := checkHorizontalRow(backend.Parent().(*walk.Composite)); err != nil {
					failure = err
				}
				for i := range keys {
					if err := checkHorizontalRow(keys[i].Parent().(*walk.Composite)); err != nil {
						failure = err
					}
				}
				if environment.BoundsPixels().Y+environment.BoundsPixels().Height > settings.BoundsPixels().Y {
					failure = fmt.Errorf("keyboard sections overlap")
				}
				path := filepath.Join(out, fmt.Sprintf("pc-shortcuts-%dx%d-%ddpi.png", size.Width, size.Height, mw.DPI()))
				if err := captureForm(mw, path); err != nil {
					failure = err
				}
				t.Logf("%dx%d, DPI %d, environment %+v, settings %+v", size.Width, size.Height, mw.DPI(), environment.BoundsPixels(), settings.BoundsPixels())
				mw.Close()
			})
		}()
		mw.Run()
		mw.Dispose()
		if failure != nil {
			t.Fatal(failure)
		}
	}
}
