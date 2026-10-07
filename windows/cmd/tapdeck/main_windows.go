//go:build windows

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/sys/windows"
	"image"
	"image/color"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"tapdeck/internal/audio"
	"tapdeck/internal/apkdist"
	"tapdeck/internal/autostart"
	"tapdeck/internal/config"
	"tapdeck/internal/driver"
	"tapdeck/internal/input"
	"tapdeck/internal/keyboard"
	"tapdeck/internal/server"
	"time"
	"unsafe"
)

func main() {
	worker := flag.Bool("keyboard-worker", false, "internal inherited-pipe keyboard worker")
	extractDriver := flag.String("extract-keyboard-driver", "", "extract bundled signed MSI to this directory")
	installDriver := flag.Bool("install-keyboard-driver", false, "install the bundled driver (Windows UAC)")
	keyboardStatus := flag.Bool("keyboard-status", false, "print virtual keyboard device status")
	headless := flag.Bool("headless", false, "启动接收端并与托盘常驻，但不弹出设置窗口（用于开机自启）")
	data := flag.String("data-dir", config.Directory(), "settings directory")
	list := flag.Bool("list-audio", false, "list WASAPI render endpoints")
	probe := flag.Int("audio-probe", 0, "capture only CABLE Output and print level metrics for N seconds")
	autostartOn := flag.Bool("autostart-on", false, "register the receiver to start at Windows sign-in, then exit")
	autostartOff := flag.Bool("autostart-off", false, "remove the sign-in startup entry, then exit")
	flag.Parse()
	if *autostartOn || *autostartOff {
		on, err := autostart.Set(*autostartOn)
		if err != nil {
			log.Fatalf("设置开机自启失败: %v", err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"autostart": on, "command": mustCommand()})
		return
	}
	if *worker {
		if e := keyboard.RunWorker(os.Stdin, os.Stdout); e != nil {
			log.Print(e)
		}
		return
	}
	if *extractDriver != "" {
		path, e := driver.Extract(*extractDriver)
		if e != nil {
			log.Fatal(e)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"msi": path, "sha256": driver.SHA256})
		return
	}
	if *installDriver {
		r, e := driver.Install(filepath.Join(*data, "driver", "0.1.1"), false)
		if e != nil {
			log.Fatal(e)
		}
		_ = json.NewEncoder(os.Stdout).Encode(r)
		return
	}
	if *keyboardStatus {
		engine := keyboard.NewEngine("auto")
		_ = json.NewEncoder(os.Stdout).Encode(engine.Status())
		return
	}
	if *probe > 0 {
		samples, err := audio.CaptureProbe(context.Background(), *probe)
		if err != nil {
			log.Fatal(err)
		}
		var power float64
		var peak int
		for _, sample := range samples {
			v := int(sample)
			peak = max(peak, int(math.Abs(float64(v))))
			power += float64(v) * float64(v)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"samples": len(samples), "peak": peak, "rms": math.Sqrt(power / float64(max(1, len(samples))))})
		return
	}
	if *list {
		ds, e := audio.Devices()
		if e != nil {
			log.Fatal(e)
		}
		for _, v := range ds {
			fmt.Printf("%s\t%s\n", v.ID, v.Name)
		}
		return
	}
	if e := os.MkdirAll(*data, 0700); e != nil {
		log.Fatal(e)
	}
	f, e := os.OpenFile(filepath.Join(*data, "tapdeck.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		log.Fatal(e)
	}
	defer f.Close()
	// GUI processes have no usable stderr handle; write the file first.
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	mutexName, _ := windows.UTF16PtrFromString("Local\\TapDeckReceiver")
	instance, e := windows.CreateMutex(nil, false, mutexName)
	if e == windows.ERROR_ALREADY_EXISTS {
		if !*headless {
			walk.MsgBox(nil, "TapDeck 已运行", "请从系统托盘打开现有接收端。", walk.MsgBoxIconInformation)
		}
		if instance != 0 {
			windows.CloseHandle(instance)
		}
		return
	}
	if e != nil {
		log.Fatal(e)
	}
	defer windows.CloseHandle(instance)
	s, e := server.New(*data)
	if e != nil {
		log.Fatal(e)
	}
	defer s.Stop()
	// 内置的 Android 安装包：配对网页会给出下载二维码。
	s.SetAPK(apkdist.Name(), apkdist.Bytes(), apkdist.SHA256())
	agent, e := keyboard.NewAgent(s.Config().KeyboardBackend)
	if e != nil {
		log.Fatal(e)
	}
	s.Input = agent
	defer func() { s.Stop(); agent.Close() }()
	if e = s.Start(); e != nil {
		log.Printf("接收启动失败: %v", e)
	}
	if *headless {
		// 与常规模式一样创建窗口与托盘图标，只是先隐藏设置窗口：
		// 开机自启不弹窗，但用户可以随时从托盘打开设置或退出。
		if e = window(s, *data, true); e != nil {
			log.Fatal(e)
		}
		return
	}
	if e = window(s, *data, false); e != nil {
		log.Fatal(e)
	}
}
// apkSummary 说明本次构建是否内置了 Android 安装包（配对网页据此决定显示二维码还是 Release 链接）。
func apkSummary() string {
	if apkdist.Available() {
		return fmt.Sprintf("内置 Android 安装包：%s（%s），配对网页可扫码下载", apkdist.Name(), apkdist.SizeText())
	}
	return "未内置 Android 安装包：配对网页只显示 GitHub Release 链接（先运行 scripts/build-android.ps1 再构建接收端）"
}

func open(path string) {
	p, _ := windows.UTF16PtrFromString(path)
	verb, _ := windows.UTF16PtrFromString("open")
	windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, 1)
}
func mustCommand() string {
	command, err := autostart.Command()
	if err != nil {
		return ""
	}
	return command
}

// autostartEnabled reports the current sign-in entry state for the settings tab.
func autostartEnabled() bool {
	on, _, err := autostart.Enabled()
	return err == nil && on
}

func window(s *server.Server, dir string, startHidden bool) error {
	// Window creation, the message loop and STA callbacks must share an OS
	// thread, even when COM enumeration yields before the first window.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var mw *walk.MainWindow
	var address *walk.LineEdit
	var status, audioStatus, pendingLabel, stats *walk.Label
	var qrView *walk.ImageView
	var devices *walk.ComboBox
	var backend *walk.ComboBox
	var keyboardLabel *walk.Label
	var driverButton *walk.PushButton
	var installing atomic.Bool
	var holdKey, toggleStartKey, toggleStopKey *walk.LineEdit
	var gain, sensitivity, delay, httpPort, wssPort, udpPort *walk.NumberEdit
	var natural *walk.CheckBox
	var autostartBox *walk.CheckBox
	var autostartLabel *walk.Label
	var labels, keys [config.ShortcutCount]*walk.LineEdit
	var enabled [config.ShortcutCount]*walk.CheckBox
	var pendingID string
	cfg := s.Config()
	backendIDs := []string{"auto", "hid", "sendinput"}
	backendNames := []string{"自动（优先虚拟键盘；不支持的键使用 SendInput）", "虚拟键盘（HID）", "软件按键（SendInput）"}
	selectedBackend := 0
	for i, id := range backendIDs {
		if cfg.KeyboardBackend == id {
			selectedBackend = i
		}
	}
	agent, _ := s.Input.(*keyboard.Agent)
	ds, _ := audio.Devices()
	deviceNames := []string{"自动选择 CABLE Input"}
	deviceIDs := []string{""}
	selectedDevice := 0
	for _, v := range ds {
		deviceNames = append(deviceNames, v.Name)
		deviceIDs = append(deviceIDs, v.ID)
		if v.ID == cfg.AudioDevice {
			selectedDevice = len(deviceNames) - 1
		}
	}
	var qrBitmap *walk.Bitmap
	refreshQR := func() {
		s.RefreshQR()
		q, e := qrcode.New(s.QRURI(), qrcode.Medium)
		if e != nil {
			return
		}
		b, e := walk.NewBitmapFromImage(q.Image(240))
		if e != nil {
			return
		}
		old := qrBitmap
		qrBitmap = b
		_ = qrView.SetImage(b)
		if old != nil {
			old.Dispose()
		}
	}
	shortcutRows := []d.Widget{d.Label{Text: "勾选 1–8 个快捷键。可直接编辑、录入组合键，或选择左右修饰键。"}}
	for i := 0; i < config.ShortcutCount; i++ {
		j := i
		row := []d.Widget{d.CheckBox{AssignTo: &enabled[j], Text: strconv.Itoa(j + 1), Checked: cfg.Shortcuts[j].Enabled}, d.LineEdit{AssignTo: &labels[j], Text: cfg.Shortcuts[j].Label, MinSize: d.Size{Width: 90}, MaxSize: d.Size{Width: 140}}}
		row = append(row, keyWidgets(func() walk.Form { return mw }, &keys[j], cfg.Shortcuts[j].Chord)...)
		shortcutRows = append(shortcutRows, d.Composite{Layout: d.HBox{}, Children: row})
	}
	save := func() {
		c := s.Config()
		for i := 0; i < config.ShortcutCount; i++ {
			c.Shortcuts[i] = config.Shortcut{Label: strings.TrimSpace(labels[i].Text()), Chord: strings.TrimSpace(keys[i].Text()), Enabled: enabled[i].Checked()}
		}
		c.HTTPPort = int(httpPort.Value())
		c.WSSPort = int(wssPort.Value())
		c.UDPPort = int(udpPort.Value())
		c.Gain = gain.Value()
		c.Sensitivity = sensitivity.Value()
		c.NaturalScroll = natural.Checked()
		c.KeyboardBackend = backendIDs[max(0, backend.CurrentIndex())]
		c.AudioDevice = deviceIDs[max(0, devices.CurrentIndex())]
		c.Voice.HoldKey = strings.TrimSpace(holdKey.Text())
		c.Voice.ToggleStartKey = strings.TrimSpace(toggleStartKey.Text())
		c.Voice.ToggleStopKey = strings.TrimSpace(toggleStopKey.Text())
		c.Voice.StopDelayMS = int(delay.Value())
		if e := s.Update(c); e != nil {
			walk.MsgBox(mw, "设置未保存", e.Error(), walk.MsgBoxIconError)
			return
		}
		_ = address.SetText(s.PairURL())
		refreshQR()
		walk.MsgBox(mw, "已保存", "配置已保存并同步到已连接设备。", walk.MsgBoxIconInformation)
	}
	err := (d.MainWindow{AssignTo: &mw, Title: "TapDeck · Windows 接收端", Size: d.Size{Width: 740, Height: 800}, MinSize: d.Size{Width: 680, Height: 700}, Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 9}, Layout: d.VBox{Margins: d.Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}}, Children: []d.Widget{
		d.Label{AssignTo: &status, Text: "正在启动…"},
		d.TabWidget{Pages: []d.TabPage{
			{Title: "连接", Layout: d.VBox{}, Children: []d.Widget{d.Label{Text: "在 Android 浏览器或 TapDeck 连接页输入下方网址。"}, d.LineEdit{AssignTo: &address, Text: s.PairURL(), ReadOnly: true}, d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.PushButton{Text: "复制网址", OnClicked: func() { _ = walk.Clipboard().SetText(s.PairURL()) }}, d.PushButton{Text: "打开配对网页", OnClicked: func() { open(s.PairURL()) }}, d.PushButton{Text: "刷新二维码", OnClicked: refreshQR}, d.PushButton{Text: "复制二维码配对信息", OnClicked: func() { _ = walk.Clipboard().SetText(s.QRURI()) }}}}, d.ImageView{AssignTo: &qrView, MinSize: d.Size{Width: 240, Height: 240}, MaxSize: d.Size{Width: 260, Height: 260}, Mode: d.ImageViewModeIdeal}, d.Label{AssignTo: &pendingLabel, Text: "等待配对请求"}, d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.PushButton{Text: "校验码一致，允许", OnClicked: func() { s.Approve(pendingID, true) }}, d.PushButton{Text: "拒绝", OnClicked: func() { s.Approve(pendingID, false) }}, d.PushButton{Text: "解除配对", OnClicked: func() {
				if walk.MsgBox(mw, "解除配对", "删除已配对设备并停止当前输入？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
					if e := s.Unpair(); e != nil {
						walk.MsgBox(mw, "解绑失败", e.Error(), walk.MsgBoxIconError)
					}
					refreshQR()
				}
			}}}}, d.VSpacer{}}},
			{Title: "快捷键", Layout: d.VBox{}, Children: append([]d.Widget{d.Label{Text: "键盘发送方式"}, d.ComboBox{AssignTo: &backend, Model: backendNames, CurrentIndex: selectedBackend}, d.Label{AssignTo: &keyboardLabel, Text: "正在检测虚拟键盘…"}, d.PushButton{AssignTo: &driverButton, Text: "安装 / 修复虚拟键盘", OnClicked: func() {
				if !installing.CompareAndSwap(false, true) {
					return
				}
				driverButton.SetEnabled(false)
				_ = keyboardLabel.SetText("正在安装原版签名驱动，请处理 Windows 授权提示…")
				go func() {
					repair := agent.KeyboardStatus().Driver != ""
					r, e := driver.Install(filepath.Join(dir, "driver", "0.1.1"), repair)
					if e == nil && !r.RestartRequired {
						e = agent.ConfigureBackend(s.Config().KeyboardBackend)
					}
					mw.Synchronize(func() {
						installing.Store(false)
						driverButton.SetEnabled(true)
						if e != nil {
							walk.MsgBox(mw, "驱动安装", e.Error(), walk.MsgBoxIconError)
						} else if r.RestartRequired {
							walk.MsgBox(mw, "需要重启", "驱动安装要求重启 Windows；TapDeck 不会自动重启电脑。", walk.MsgBoxIconInformation)
						} else {
							walk.MsgBox(mw, "安装完成", "虚拟键盘已重新检测。", walk.MsgBoxIconInformation)
						}
					})
				}()
			}}}, append(shortcutRows, d.VSpacer{})...)},
			{Title: "语音", Layout: d.VBox{}, Children: []d.Widget{d.Label{Text: "PC 软件选择 CABLE Output 为麦克风；TapDeck 输出到 CABLE Input。"}, d.ComboBox{AssignTo: &devices, Model: deviceNames, CurrentIndex: selectedDevice}, d.PushButton{Text: "刷新音频设备", OnClicked: func() {
				items, e := audio.Devices()
				if e != nil {
					walk.MsgBox(mw, "设备错误", e.Error(), walk.MsgBoxIconError)
					return
				}
				deviceNames = []string{"自动选择 CABLE Input"}
				deviceIDs = []string{""}
				for _, v := range items {
					deviceNames = append(deviceNames, v.Name)
					deviceIDs = append(deviceIDs, v.ID)
				}
				_ = devices.SetModel(deviceNames)
				_ = devices.SetCurrentIndex(0)
			}}, d.Label{AssignTo: &audioStatus, Text: "正在检查音频设备"}, d.Label{Text: "音量倍率（0–3）"}, d.NumberEdit{AssignTo: &gain, Value: cfg.Gain, MinValue: 0, MaxValue: 3, Decimals: 2, Increment: 0.1}, d.Label{Text: "两种手势同时可用；热键留空时仅传音。"}, d.Label{Text: "长按热键（圆球按住 300 ms，松手释放）"}, d.Composite{Layout: d.HBox{}, Children: keyWidgets(func() walk.Form { return mw }, &holdKey, cfg.Voice.HoldKey)}, d.Label{Text: "免按开始热键（双击圆球开始）"}, d.Composite{Layout: d.HBox{}, Children: keyWidgets(func() walk.Form { return mw }, &toggleStartKey, cfg.Voice.ToggleStartKey)}, d.Label{Text: "免按结束热键（单击圆球停止）"}, d.Composite{Layout: d.HBox{}, Children: keyWidgets(func() walk.Form { return mw }, &toggleStopKey, cfg.Voice.ToggleStopKey)}, d.Label{Text: "尾音结束延迟 ms"}, d.NumberEdit{AssignTo: &delay, Value: float64(cfg.Voice.StopDelayMS), MinValue: 0, MaxValue: 1000}, d.VSpacer{}}},
			{Title: "设置与状态", Layout: d.VBox{}, Children: []d.Widget{d.Label{Text: "HTTP / WSS / UDP 端口（修改后重启连接）"}, d.NumberEdit{AssignTo: &httpPort, Value: float64(cfg.HTTPPort), MinValue: 1024, MaxValue: 65535}, d.NumberEdit{AssignTo: &wssPort, Value: float64(cfg.WSSPort), MinValue: 1024, MaxValue: 65535}, d.NumberEdit{AssignTo: &udpPort, Value: float64(cfg.UDPPort), MinValue: 1024, MaxValue: 65535}, d.Label{Text: "触控灵敏度"}, d.NumberEdit{AssignTo: &sensitivity, Value: cfg.Sensitivity, MinValue: 0.1, MaxValue: 5, Decimals: 2, Increment: 0.1}, d.CheckBox{AssignTo: &natural, Text: "自然滚动", Checked: cfg.NaturalScroll}, d.CheckBox{AssignTo: &autostartBox, Text: "随 Windows 登录自动启动接收端", Checked: autostartEnabled(), OnCheckedChanged: func() {
				on, err := autostart.Set(autostartBox.Checked())
				if err != nil {
					walk.MsgBox(mw, "开机自启设置失败", err.Error(), walk.MsgBoxIconError)
				}
				autostartBox.SetChecked(on)
				_ = autostartLabel.SetText(autostart.Summary())
			}}, d.Label{AssignTo: &autostartLabel, Text: autostart.Summary()}, d.Label{Text: apkSummary()}, d.Label{AssignTo: &stats, Text: "等待数据"}, d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.PushButton{Text: "启动接收", OnClicked: func() {
				if e := s.Start(); e != nil {
					walk.MsgBox(mw, "启动失败", e.Error(), walk.MsgBoxIconError)
				}
			}}, d.PushButton{Text: "停止接收", OnClicked: s.Stop}, d.PushButton{Text: "打开日志", OnClicked: func() { open(filepath.Join(dir, "tapdeck.log")) }}}}, d.VSpacer{}}},
		}}, d.PushButton{Text: "保存并同步配置", OnClicked: save},
	}}).Create()
	if err != nil {
		return err
	}
	// Reapply PC-owned values after Walk's declarative initialization.
	for i := range enabled {
		enabled[i].SetChecked(cfg.Shortcuts[i].Enabled)
	}
	natural.SetChecked(cfg.NaturalScroll)
	defer mw.Dispose()
	defer func() {
		if qrBitmap != nil {
			qrBitmap.Dispose()
		}
	}()
	refreshQR()
	iconImage := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			v := color.RGBA{23, 92, 211, 255}
			if x > 7 && x < 25 && y > 7 && y < 25 {
				v = color.RGBA{255, 255, 255, 255}
			}
			iconImage.Set(x, y, v)
		}
	}
	icon, e := walk.NewIconFromImage(iconImage)
	if e != nil {
		return e
	}
	defer icon.Dispose()
	_ = mw.SetIcon(icon)
	tray, e := walk.NewNotifyIcon(mw)
	if e != nil {
		return e
	}
	defer tray.Dispose()
	_ = tray.SetIcon(icon)
	_ = tray.SetToolTip("TapDeck：点击打开设置")
	show := func() { mw.Show(); mw.Activate() }
	tray.MouseDown().Attach(func(x, y int, b walk.MouseButton) {
		if b == walk.LeftButton {
			show()
		}
	})
	a := walk.NewAction()
	_ = a.SetText("打开设置")
	a.Triggered().Attach(show)
	_ = tray.ContextMenu().Actions().Add(a)
	exit := walk.NewAction()
	_ = exit.SetText("退出")
	exit.Triggered().Attach(func() { s.Stop(); walk.App().Exit(0) })
	_ = tray.ContextMenu().Actions().Add(exit)
	if e = tray.SetVisible(true); e != nil {
		// 托盘创建失败时至少留下日志，避免“进程在跑但没有图标”无从排查。
		log.Printf("托盘图标创建失败: %v", e)
	}
	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) { *canceled = true; mw.Hide() })
	if startHidden {
		// --headless：开机自启不弹设置窗口，用户点击托盘图标再显示。
		mw.Hide()
		log.Printf("TapDeck %s（已常驻托盘，设置窗口未显示）", s.PairURL())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		var lastMetrics time.Time
		var uiPending atomic.Bool
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				v := s.Snapshot()
				kb := agent.KeyboardStatus()
				if time.Since(lastMetrics) >= 5*time.Second {
					metrics, _ := json.Marshal(map[string]any{"at": time.Now().Format(time.RFC3339), "connected": v.Device != "", "mouse_packets": v.MousePackets, "audio_packets": v.AudioPackets, "injection_p95_ms": v.InjectionP95MS, "buffered_frames": v.BufferedFrames, "max_buffered_frames": v.MaxBufferedFrames, "concealed_frames": v.Concealed, "audio_ready": v.AudioReady})
					_ = config.AtomicWrite(filepath.Join(dir, "runtime-stats.json"), metrics)
					lastMetrics = time.Now()
				}
				if !uiPending.CompareAndSwap(false, true) {
					continue
				}
				mw.Synchronize(func() {
					defer uiPending.Store(false)
					text := "接收已停止"
					if v.Running {
						text = "等待 Android 连接"
					}
					if v.Device != "" {
						text = "已连接：" + v.Device
					}
					_ = status.SetText(text)
					_ = keyboardLabel.SetText(fmt.Sprintf("实际发送：%s · 驱动：%s · API %d\n%s", kb.Actual, kb.Driver, kb.API, kb.Error))
					driverButton.SetEnabled(!kb.Busy && !installing.Load())
					_ = audioStatus.SetText(fmt.Sprintf("%s · 输入电平 %.0f%%", v.AudioStatus, v.Level*100))
					_ = stats.SetText(fmt.Sprintf("鼠标包 %d · 音频包 %d · 注入 p95 %.3f ms\n音频缓冲 %d/6 帧 · 历史最大 %d 帧 · 补静音帧 %d", v.MousePackets, v.AudioPackets, v.InjectionP95MS, v.BufferedFrames, v.MaxBufferedFrames, v.Concealed))
					pendingID = ""
					if len(v.Pending) > 0 {
						p := v.Pending[0]
						pendingID = p.ID
						_ = pendingLabel.SetText(p.Name + "\n核对校验码：\n" + p.Code)
					} else {
						_ = pendingLabel.SetText("等待配对请求")
					}
				})
			}
		}
	}()
	mw.Run()
	return nil
}
func keyWidgets(owner func() walk.Form, target **walk.LineEdit, text string) []d.Widget {
	var picker *walk.ComboBox
	names := []string{"单键选择…", "左 Alt", "右 Alt", "左 Ctrl", "右 Ctrl", "左 Shift", "右 Shift", "音量加", "音量减", "静音"}
	keys := []string{"LeftAlt", "RightAlt", "LeftCtrl", "RightCtrl", "LeftShift", "RightShift", "VolumeUp", "VolumeDown", "VolumeMute"}
	return []d.Widget{
		d.LineEdit{AssignTo: target, Text: text, MinSize: d.Size{Width: 150}, StretchFactor: 1},
		d.ComboBox{AssignTo: &picker, Model: names, CurrentIndex: 0, MinSize: d.Size{Width: 105}, MaxSize: d.Size{Width: 130}, OnCurrentIndexChanged: func() {
			i := picker.CurrentIndex()
			if i > 0 && *target != nil {
				_ = (*target).SetText(keys[i-1])
				_ = picker.SetCurrentIndex(0)
			}
		}},
		d.PushButton{Text: "录入", OnClicked: func() { captureChord(owner(), *target) }},
	}
}
func captureChord(owner walk.Form, target *walk.LineEdit) {
	var dlg *walk.Dialog
	var field *walk.LineEdit
	var chord string
	_ = (d.Dialog{AssignTo: &dlg, Title: "录入组合键", MinSize: d.Size{Width: 380, Height: 180}, Layout: d.VBox{}, Children: []d.Widget{d.Label{Text: "先按住修饰键，再按主键，左右修饰键会分别记录。单独 Alt/Ctrl/Shift 请用设置中的单键选择。"}, d.LineEdit{AssignTo: &field, ReadOnly: true, OnKeyDown: func(key walk.Key) {
		if key == walk.KeyControl || key == walk.KeyShift || key == walk.KeyMenu {
			return
		}
		chord = chordWithModifiers(walkKeyName(key), func(vk uint16) bool {
			down, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
			return int16(down) < 0
		})
		_ = field.SetText(chord)
	}}, d.PushButton{Text: "使用此组合键", OnClicked: func() { _ = target.SetText(chord); dlg.Accept() }}, d.PushButton{Text: "取消", OnClicked: func() { dlg.Cancel() }}}}).Create(owner)
	if dlg != nil {
		defer dlg.Dispose()
		field.SetFocus()
		dlg.Run()
	}
}

// walkKeyName maps a recorded key to the name the chord parser and the
// injection backends share. walk's own key names use a different vocabulary
// ("Back", "Escape", "Prior", "VolumeUp"), which previously produced hotkeys
// that the parser rejected, so special keys could not be saved at all.
func walkKeyName(k walk.Key) string {
	if name, ok := input.KeyName(uint16(k)); ok {
		return name
	}
	return ""
}

var procGetAsyncKeyState = windows.NewLazySystemDLL("user32.dll").NewProc("GetAsyncKeyState")

// modifiers lists the six side-specific modifiers in the order they are written
// into a chord. walk.ModifiersDown reports only the left Alt/Ctrl/Shift keys and
// cannot tell the two sides apart, so the sides are read directly.
var modifiers = []struct {
	Name string
	VK   uint16
}{
	{"LeftCtrl", 0xA2}, {"RightCtrl", 0xA3},
	{"LeftShift", 0xA0}, {"RightShift", 0xA1},
	{"LeftAlt", 0xA4}, {"RightAlt", 0xA5},
}

// chordWithModifiers builds the chord text for a main key while the given
// modifiers are held, so the recorded hotkey keeps the left/right side the user
// actually pressed.
func chordWithModifiers(main string, down func(uint16) bool) string {
	parts := []string{}
	for _, m := range modifiers {
		if down(m.VK) {
			parts = append(parts, m.Name)
		}
	}
	if main == "" {
		return ""
	}
	return strings.Join(append(parts, main), "+")
}
