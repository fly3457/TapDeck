package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"tapdeck/internal/config"
	"testing"
	"time"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// Opt-in native layout regression: creates only an offscreen editor, with no
// receiver, pairing, audio engine, key injection or changes to the user's config.
func TestVoiceProfileEditorUI(t *testing.T) {
	out := os.Getenv("TAPDECK_UI_OUTPUT")
	if out == "" {
		t.Skip("set TAPDECK_UI_OUTPUT to render the offscreen native editor")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer useSystemDPIForTest()()
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	for _, size := range []walk.Size{{Width: 740, Height: 800}, {Width: 680, Height: 700}} {
		var mw *walk.MainWindow
		var scroll *walk.ScrollView
		var editors [config.VoiceProfileCount]voiceProfileEditor
		profiles := config.DefaultVoiceProfiles()
		widget := voiceProfileWidgets(func() walk.Form { return mw }, &editors, profiles).(d.ScrollView)
		widget.AssignTo = &scroll
		if err := (d.MainWindow{AssignTo: &mw, Title: "TapDeck · 语音配置验证", Size: d.Size{Width: size.Width, Height: size.Height}, Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 9}, Layout: d.VBox{}, Children: []d.Widget{
			d.Label{Text: "已连接：布局验证设备"},
			d.TabWidget{Pages: []d.TabPage{{Title: "语音", Layout: d.VBox{}, Children: []d.Widget{
				voiceRoutingHint(),
				d.Label{Text: "VB-CABLE 可用：CABLE Input / CABLE Output 均已就绪"},
				d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.PushButton{Text: "安装虚拟声卡", Enabled: false}, d.PushButton{Text: "重新检测"}, d.PushButton{Text: "VB-Audio 官网"}, d.PushButton{Text: "原包许可"}}},
				d.Label{Text: cableAttributionText},
				audioDeviceRow(nil, []string{"自动选择 CABLE Input"}, 0, func() {}, func() {}),
				d.Label{Text: "CABLE Input (VB-Audio Virtual Cable) · 输入电平 0%"},
				d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.Label{Text: "音量倍率（0–3）"}, d.NumberEdit{Value: float64(1), MinValue: 0, MaxValue: 3, Decimals: 2}}},
				horizontalRule(), d.Label{Text: "名称：最多 8 个汉字 / 16 个英文字符。热键：留空仅传音。"}, widget,
				d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.Label{Text: "尾音结束延迟 ms"}, d.NumberEdit{Value: float64(200), MinValue: 0, MaxValue: 1000}}},
			}}}}, d.PushButton{Text: "保存并同步配置"},
		}}).Create(); err != nil {
			t.Fatal(err)
		}
		placeTestWindow(mw, size)
		mw.Show()
		var failure error
		capture := func(label string) {
			bitmap, err := walk.NewBitmapForDPI(mw.SizePixels(), mw.DPI())
			if err != nil {
				failure = err
				return
			}
			defer bitmap.Dispose()
			canvas, err := walk.NewCanvasFromImage(bitmap)
			if err != nil {
				failure = err
				return
			}
			ok, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("PrintWindow").Call(uintptr(mw.Handle()), uintptr(canvas.HDC()), 2)
			canvas.Dispose()
			if ok == 0 {
				failure = fmt.Errorf("PrintWindow failed")
				return
			}
			img, err := bitmap.ToImage()
			if err != nil {
				failure = err
				return
			}
			f, err := os.Create(filepath.Join(out, fmt.Sprintf("pc-voice-%dx%d-%ddpi-%s.png", size.Width, size.Height, mw.DPI(), label)))
			if err != nil {
				failure = err
				return
			}
			defer f.Close()
			if err := png.Encode(f, img); err != nil {
				failure = err
			}
		}
		go func() {
			time.Sleep(600 * time.Millisecond)
			mw.Synchronize(func() {
				capture("top")
				for i := range editors {
					if editors[i].body.Visible() != profiles[i].Enabled {
						failure = fmt.Errorf("default expansion %d", i)
					}
				}
				e := &editors[0]
				e.setExpanded(false)
				if e.body.Visible() || !e.enabled.Checked() {
					failure = fmt.Errorf("collapse changed enable state")
				}
				e.setExpanded(true)
				if e.holdPanel.Visible() || !e.togglePanel.Visible() {
					failure = fmt.Errorf("incorrect toggle visibility")
				}
				e.hold.SetText("F8")
				e.mode.SetCurrentIndex(0)
				if !e.holdPanel.Visible() || e.togglePanel.Visible() {
					failure = fmt.Errorf("incorrect hold visibility")
				}
				e.mode.SetCurrentIndex(1)
				p := e.value("voice-1")
				if p.HoldKey != "F8" || p.ToggleStartKey != "RightCtrl+L" || p.ToggleStopKey != "RightCtrl+L" {
					failure = fmt.Errorf("hidden keys lost: %+v", p)
				}
			})
			time.Sleep(200 * time.Millisecond)
			mw.Synchronize(func() {
				for i := range editors {
					e := &editors[i]
					if e.body.Visible() {
						panel := e.holdPanel
						if e.mode.CurrentIndex() == 1 {
							panel = e.togglePanel
						}
						for j := 0; j < panel.Children().Len(); j++ {
							row := panel.Children().At(j).(*walk.Composite)
							if err := checkHorizontalRow(row); err != nil {
								failure = err
							}
						}
					}
				}
				for i := 0; i < 10; i++ {
					scroll.SendMessage(win.WM_VSCROLL, win.SB_PAGEDOWN, 0)
				}
			})
			time.Sleep(200 * time.Millisecond)
			mw.Synchronize(func() {
				capture("defaults-bottom")
				for i := range editors {
					editors[i].name.SetText("八个汉字名称测试")
					editors[i].mode.SetCurrentIndex(1)
					editors[i].setExpanded(true)
				}
			})
			time.Sleep(200 * time.Millisecond)
			mw.Synchronize(func() {
				capture("bottom")
				for i := range editors {
					editors[i].setExpanded(false)
				}
			})
			time.Sleep(200 * time.Millisecond)
			mw.Synchronize(func() {
				capture("collapsed")
				for i := range editors {
					if editors[i].title.Text() != "八个汉字名称测试" {
						failure = fmt.Errorf("collapsed title did not update")
					}
				}
				t.Logf("%dx%d, DPI %d, scroll viewport %+v", size.Width, size.Height, mw.DPI(), scroll.SizePixels())
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

func checkHorizontalRow(row *walk.Composite) error {
	end := 0
	for i := 0; i < row.Children().Len(); i++ {
		b := row.Children().At(i).BoundsPixels()
		if b.Width <= 0 || b.X < end || b.X+b.Width > row.ClientBoundsPixels().Width+1 {
			return fmt.Errorf("horizontal form overflow: child %d %+v in %+v", i, b, row.ClientBoundsPixels())
		}
		end = b.X + b.Width
	}
	return nil
}
