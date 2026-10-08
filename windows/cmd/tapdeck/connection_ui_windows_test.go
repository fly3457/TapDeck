package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"tapdeck/internal/server"
	"testing"
	"time"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	qrcode "github.com/skip2/go-qrcode"
)

func captureForm(form walk.Form, path string) error {
	bitmap, err := walk.NewBitmapForDPI(form.SizePixels(), form.DPI())
	if err != nil {
		return err
	}
	defer bitmap.Dispose()
	canvas, err := walk.NewCanvasFromImage(bitmap)
	if err != nil {
		return err
	}
	ok, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("PrintWindow").Call(uintptr(form.Handle()), uintptr(canvas.HDC()), 2)
	canvas.Dispose()
	if ok == 0 {
		return fmt.Errorf("PrintWindow failed")
	}
	img, err := bitmap.ToImage()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func findButton(container walk.Container, text string) *walk.PushButton {
	for i := 0; i < container.Children().Len(); i++ {
		widget := container.Children().At(i)
		if b, ok := widget.(*walk.PushButton); ok && b.Text() == text {
			return b
		}
		if child, ok := widget.(walk.Container); ok {
			if b := findButton(child, text); b != nil {
				return b
			}
		}
	}
	return nil
}

func TestConnectionAndPairingUI(t *testing.T) {
	out := os.Getenv("TAPDECK_UI_OUTPUT")
	if out == "" {
		t.Skip("set TAPDECK_UI_OUTPUT to exercise isolated native windows")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer useSystemDPIForTest()()
	for _, size := range []walk.Size{{Width: 740, Height: 800}, {Width: 680, Height: 700}} {
		var mw *walk.MainWindow
		var tabs *walk.TabWidget
		var save *walk.PushButton
		var address, draft *walk.LineEdit
		var qr *walk.ImageView
		intro := connectionIntro(&address, &qr, "http://192.168.1.25:41080/pair", func() {}, func() {})
		intro = append(intro,
			d.Label{Text: "已配对设备（同名设备请核对设备标识）"},
			d.TableView{Model: &pairedModel{items: []server.PairedDevice{{Name: "Android 手机", ID: "sample-device", Online: true}}}, MinSize: d.Size{Height: 140}, StretchFactor: 1,
				Columns: []d.TableViewColumn{{Title: "名称", Width: 130}, {Title: "状态", Width: 50}, {Title: "设备标识", Width: 210}, {Title: "最近连接", Width: 150}}},
			d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.PushButton{Text: "解除所选设备配对", Enabled: false}, d.PushButton{Text: "解除全部配对"}}})
		if err := (d.MainWindow{AssignTo: &mw, Title: "TapDeck · 连接界面验证", Size: d.Size{Width: size.Width, Height: size.Height},
			Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 9}, Layout: d.VBox{Margins: d.Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}}, Children: []d.Widget{
				d.Label{Text: "已连接：Android 手机"},
				d.TabWidget{AssignTo: &tabs, OnCurrentIndexChanged: func() { updateSaveVisibility(tabs, save) }, Pages: []d.TabPage{
					{Title: "连接", Layout: d.VBox{}, Children: intro},
					{Title: "快捷键", Layout: d.VBox{}, Children: []d.Widget{d.LineEdit{AssignTo: &draft, Text: "未保存的配置"}, d.VSpacer{}}},
					{Title: "语音", Layout: d.VBox{}}, {Title: "设置与状态", Layout: d.VBox{}}, {Title: "关于", Layout: d.VBox{}},
				}},
				d.PushButton{AssignTo: &save, Text: "保存并同步配置", Visible: false},
			}}).Create(); err != nil {
			t.Fatal(err)
		}
		code, _ := qrcode.New(address.Text(), qrcode.Medium)
		bitmap, err := walk.NewBitmapFromImage(code.Image(200))
		if err != nil {
			t.Fatal(err)
		}
		qr.SetImage(bitmap)
		placeTestWindow(mw, size)
		mw.Show()
		var decisions []string
		prompts := pairingPrompts{owner: mw, approve: func(id string, allow bool) { decisions = append(decisions, fmt.Sprintf("%s:%t", id, allow)) },
			showOwner: func() { mw.Show() }, present: func(dlg *walk.Dialog) {
				// Exercise real widgets without taking foreground focus or showing them onscreen.
				placeTestWindow(dlg, walk.Size{Width: 500, Height: 330})
			}}
		now := time.Now()
		request := func(id string) server.Pending {
			return server.Pending{ID: id, Name: "Android 手机", DeviceID: "device-" + id, Code: "1234 ABCD 5678 EF90 1234 ABCD 5678 EF90", Expires: now.Add(time.Minute)}
		}
		var failure error
		check := func(ok bool, message string) {
			if !ok && failure == nil {
				failure = fmt.Errorf("%s (active=%s decisions=%v)", message, prompts.activeID, decisions)
			}
		}
		capture := func(form walk.Form, label string) {
			t.Logf("capture %s hwnd=%x size=%v dpi=%d", label, form.Handle(), form.SizePixels(), form.DPI())
			if err := captureForm(form, filepath.Join(out, fmt.Sprintf("pc-%s-%dx%d-%ddpi.png", label, size.Width, size.Height, form.DPI()))); err != nil {
				if failure == nil {
					failure = fmt.Errorf("capture %s: %w (hwnd=%x)", label, err, form.Handle())
				}
			}
		}
		go func() {
			aborted := false
			step := func(fn func()) {
				if aborted {
					return
				}
				done := make(chan struct{})
				mw.Synchronize(func() { fn(); close(done) })
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					aborted = true
					mw.Synchronize(func() {
						failure = fmt.Errorf("native UI step timed out")
						if prompts.closeLive != nil {
							prompts.closeLive()
						}
						mw.Close()
					})
				}
				time.Sleep(200 * time.Millisecond)
			}
			time.Sleep(600 * time.Millisecond)
			step(func() {
				check(!save.Visible(), "connection save button visible")
				check(findButton(tabs.Pages().At(0), "刷新二维码") == nil, "obsolete QR refresh button")
				check(findButton(tabs.Pages().At(0), "拒绝") == nil, "default approval button")
				capture(mw, "connection")
				for i := 1; i <= 3; i++ {
					tabs.SetCurrentIndex(i)
					check(save.Visible(), "editable page has no save button")
				}
				tabs.SetCurrentIndex(4)
				check(!save.Visible() && draft.Text() == "未保存的配置", "about or page switch changed draft")
				mw.Hide()
				prompts.update(nil, now)
				check(!mw.Visible(), "idle prompt restored tray window")
				prompts.update([]server.Pending{request("a"), request("b")}, now)
				check(mw.Visible() && prompts.activeID == "a", "request did not restore hidden owner")
			})
			step(func() {
				if prompts.dialog == nil {
					failure = fmt.Errorf("missing dialog")
					return
				}
				capture(prompts.dialog, "pairing")
				first := prompts.dialog
				prompts.update([]server.Pending{request("a"), request("b")}, now)
				check(prompts.dialog == first, "duplicate dialog for same request")
				check(first.DefaultButton() == nil, "Enter grants pairing by default")
				findButton(first, "允许连接").SendMessage(win.BM_CLICK, 0, 0)
				check(fmt.Sprint(decisions) == "[a:true]", "allow submitted wrong request")
				prompts.update([]server.Pending{request("a"), request("b")}, now)
				check(prompts.activeID == "b", "queue did not advance")
			})
			step(func() {
				if prompts.dialog == nil {
					return
				}
				prompts.dialog.SendMessage(win.WM_CLOSE, 0, 0)
				check(fmt.Sprint(decisions) == "[a:true b:false]", "close did not reject once")
				prompts.update([]server.Pending{request("a"), request("b")}, now)
				check(prompts.dialog == nil, "handled requests reopened")
				prompts.update([]server.Pending{request("c")}, now)
			})
			step(func() {
				prompts.update([]server.Pending{request("c")}, now.Add(2*time.Minute))
				check(prompts.dialog == nil && len(decisions) == 2, "expiry submitted approval")
				prompts.update([]server.Pending{request("d")}, now)
			})
			step(func() {
				if prompts.dialog != nil {
					win.PostMessage(prompts.dialog.Handle(), win.WM_KEYDOWN, win.VK_ESCAPE, 0)
				}
			})
			step(func() {
				check(fmt.Sprint(decisions) == "[a:true b:false d:false]", "Escape binding did not reject")
				prompts.update([]server.Pending{request("e")}, now)
			})
			step(func() {
				prompts.update(nil, now)
				check(prompts.dialog == nil && len(decisions) == 3, "disconnect/stop did not close silently")
				tabs.SetCurrentIndex(0)
				mw.Close()
			})
		}()
		mw.Run()
		mw.Dispose()
		bitmap.Dispose()
		if failure != nil {
			t.Fatal(failure)
		}
	}
}

// Scope awareness to the test thread, without changing desktop display settings.
func useSystemDPIForTest() func() {
	proc := syscall.NewLazyDLL("user32.dll").NewProc("SetThreadDpiAwarenessContext")
	previous, _, _ := proc.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
	return func() {
		if previous != 0 {
			proc.Call(previous)
		}
	}
}

func placeTestWindow(form walk.Form, size walk.Size) {
	x, y := -12000, -12000
	if os.Getenv("TAPDECK_UI_MONITOR_DPI") == "1" {
		// Transparent, click-through, nonactivating windows let the real monitor
		// select its DPI while PrintWindow still captures the native widgets.
		style := win.GetWindowLong(form.Handle(), win.GWL_EXSTYLE)
		win.SetWindowLong(form.Handle(), win.GWL_EXSTYLE, style|win.WS_EX_LAYERED|win.WS_EX_TRANSPARENT|win.WS_EX_NOACTIVATE)
		syscall.NewLazyDLL("user32.dll").NewProc("SetLayeredWindowAttributes").Call(uintptr(form.Handle()), 0, 0, 2)
		x, y = 40, 40
		if value, err := strconv.Atoi(os.Getenv("TAPDECK_UI_MONITOR_X")); err == nil {
			x = value
		}
	}
	form.SetBounds(walk.Rectangle{X: x, Y: y, Width: size.Width, Height: size.Height})
}
