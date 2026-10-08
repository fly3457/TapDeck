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
	"strings"
	"sync/atomic"
	"tapdeck/internal/apkdist"
	"tapdeck/internal/audio"
	"tapdeck/internal/autostart"
	"tapdeck/internal/config"
	"tapdeck/internal/driver"
	"tapdeck/internal/input"
	"tapdeck/internal/keyboard"
	"tapdeck/internal/server"
	"tapdeck/internal/vbcable"
	"time"
	"unsafe"
)

// Official builds inject receiver.version from the root version.properties.
var appVersion = "dev"

func main() {
	worker := flag.Bool("keyboard-worker", false, "internal inherited-pipe keyboard worker")
	extractDriver := flag.String("extract-keyboard-driver", "", "extract bundled signed MSI to this directory")
	installDriver := flag.Bool("install-keyboard-driver", false, "install the bundled driver (Windows UAC)")
	keyboardStatus := flag.Bool("keyboard-status", false, "print virtual keyboard device status")
	cableStatus := flag.Bool("cable-status", false, "print VB-CABLE driver and endpoint status without installing")
	extractCable := flag.String("extract-cable", "", "extract and verify the original VB-CABLE pack without installing")
	apkInfo := flag.Bool("apk-info", false, "verify and print bundled Android APK metadata")
	showVersion := flag.Bool("version", false, "print TapDeck version")
	headless := flag.Bool("headless", false, "启动接收端并与托盘常驻，但不弹出设置窗口（用于开机自启）")
	data := flag.String("data-dir", config.Directory(), "settings directory")
	list := flag.Bool("list-audio", false, "list WASAPI render endpoints")
	probe := flag.Int("audio-probe", 0, "capture only CABLE Output and print level metrics for N seconds")
	autostartOn := flag.Bool("autostart-on", false, "register the receiver to start at Windows sign-in, then exit")
	autostartOff := flag.Bool("autostart-off", false, "remove the sign-in startup entry, then exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("TapDeck %s\n", appVersion)
		return
	}
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
		status := engine.Status()
		state, err := driver.Detect(status.Driver != "")
		detectionError := ""
		if err != nil {
			detectionError = err.Error()
		}
		_ = json.NewEncoder(os.Stdout).Encode(struct {
			keyboard.Status
			Installation   driver.State `json:"installation"`
			DetectionError string       `json:"detection_error,omitempty"`
		}{status, state, detectionError})
		return
	}
	if *cableStatus {
		status, err := vbcable.DetectAt(filepath.Join(*data, "vbcable", "Pack45"))
		if err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(status)
		return
	}
	if *extractCable != "" {
		path, err := vbcable.Extract(*extractCable)
		if err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"installer": path, "sha256": vbcable.SHA256})
		return
	}
	if *apkInfo {
		if err := apkdist.Verify(); err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"pc_version": appVersion, "version_name": apkdist.Version(), "version_code": apkdist.VersionCode(), "filename": apkdist.Name(), "sha256": apkdist.SHA256(), "bytes": len(apkdist.Bytes())})
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
	if err := apkdist.Verify(); err != nil {
		log.Fatal(err)
	}
	if err := vbcable.Verify(); err != nil {
		log.Fatal(err)
	}
	s, e := server.New(*data)
	if e != nil {
		log.Fatal(e)
	}
	defer s.Stop()
	// 内置的 Android 安装包：配对网页会给出下载二维码。
	s.SetAPK(apkdist.Name(), apkdist.Bytes(), apkdist.SHA256(), apkdist.Version())
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

// apkSummary describes the Android artifact embedded by the official build.
func apkSummary() string {
	return fmt.Sprintf("内置 Android %s（code %d，%s）\nSHA-256：%s", apkdist.Version(), apkdist.VersionCode(), apkdist.SizeText(), apkdist.SHA256())
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
	var inputTest *walk.TextEdit
	var status, audioStatus, stats *walk.Label
	var tabs *walk.TabWidget
	var saveButton *walk.PushButton
	var qrView *walk.ImageView
	var devices *walk.ComboBox
	var backend *walk.ComboBox
	var keyboardLabel *walk.Label
	var driverButton *walk.PushButton
	var installing atomic.Bool
	var voiceEditors [config.VoiceProfileCount]voiceProfileEditor
	var gain, delay, httpPort, wssPort, udpPort *walk.NumberEdit
	var natural *walk.CheckBox
	var autostartBox *walk.CheckBox
	var autostartLabel *walk.Label
	var labels, keys [config.ShortcutCount]*walk.LineEdit
	var enabled [config.ShortcutCount]*walk.CheckBox
	var prompts pairingPrompts
	var pairingWaiting bool
	var pairedTable *walk.TableView
	var unpairButton *walk.PushButton
	paired := &pairedModel{}
	var cableLabel *walk.TextLabel
	var cableButton *walk.PushButton
	var cableInstalling atomic.Bool
	cableManager := vbcable.NewManager()
	cableLicense, licenseErr := vbcable.License(filepath.Join(dir, "vbcable"))
	if licenseErr != nil {
		return licenseErr
	}
	cable, cableErr := vbcable.DetectAt(filepath.Join(dir, "vbcable", "Pack45"))
	var cablePrompted bool
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
	keyboardState, keyboardErr := driver.Detect(agent.KeyboardStatus().Driver != "")
	var driverPrompt keyboardDriverPrompt
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
		q, e := qrcode.New(s.QRURI(), qrcode.Medium)
		if e != nil {
			return
		}
		b, e := walk.NewBitmapFromImage(q.Image(200))
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
	refreshAudio := func() {
		items, err := audio.Devices()
		if err != nil {
			walk.MsgBox(mw, "设备错误", err.Error(), walk.MsgBoxIconError)
			return
		}
		selected := ""
		if i := devices.CurrentIndex(); i >= 0 && i < len(deviceIDs) {
			selected = deviceIDs[i]
		}
		deviceNames, deviceIDs = []string{"自动选择 CABLE Input"}, []string{""}
		index := 0
		for _, item := range items {
			deviceNames = append(deviceNames, item.Name)
			deviceIDs = append(deviceIDs, item.ID)
			if item.ID == selected {
				index = len(deviceIDs) - 1
			}
		}
		_ = devices.SetModel(deviceNames)
		_ = devices.SetCurrentIndex(index)
	}
	applyKeyboardStatus := func() {
		if installing.Load() {
			return
		}
		kb := agent.KeyboardStatus()
		text := keyboardState.Text()
		if keyboardErr != nil {
			text = "虚拟键盘检测失败：" + keyboardErr.Error()
		}
		_ = keyboardLabel.SetText(fmt.Sprintf("%s\n实际发送：%s · 驱动：%s · API %d\n%s", text, kb.Actual, kb.Driver, kb.API, kb.Error))
		driverButton.SetEnabled(!kb.Busy && keyboardErr == nil)
	}
	redetectKeyboard := func() {
		if installing.Load() {
			return
		}
		go func() {
			err := agent.ConfigureBackend(s.Config().KeyboardBackend)
			state, detectionErr := driver.Detect(agent.KeyboardStatus().Driver != "")
			mw.Synchronize(func() {
				keyboardState, keyboardErr = state, detectionErr
				applyKeyboardStatus()
				if err != nil {
					walk.MsgBox(mw, "虚拟键盘检测", err.Error(), walk.MsgBoxIconInformation)
				}
			})
		}()
	}
	installKeyboard := func() {
		if agent.KeyboardStatus().Busy || !installing.CompareAndSwap(false, true) {
			return
		}
		driverButton.SetEnabled(false)
		_ = keyboardLabel.SetText("正在安装原版签名驱动，请处理 Windows 授权提示…")
		go func() {
			state, err := driver.Detect(agent.KeyboardStatus().Driver != "")
			var result driver.InstallResult
			if err == nil {
				result, err = driver.Install(filepath.Join(dir, "driver", "0.1.1"), state != driver.Missing)
			}
			if err == nil && !result.RestartRequired {
				err = agent.ConfigureBackend(s.Config().KeyboardBackend)
			}
			state, detectionErr := driver.Detect(agent.KeyboardStatus().Driver != "")
			mw.Synchronize(func() {
				defer func() {
					installing.Store(false)
					applyKeyboardStatus()
				}()
				keyboardState, keyboardErr = state, detectionErr
				switch {
				case err != nil:
					walk.MsgBox(mw, "驱动安装", err.Error(), walk.MsgBoxIconInformation)
				case result.RestartRequired:
					walk.MsgBox(mw, "需要重启", "驱动安装要求重启 Windows；TapDeck 不会自动重启电脑。", walk.MsgBoxIconInformation)
				case detectionErr != nil:
					walk.MsgBox(mw, "检测失败", detectionErr.Error(), walk.MsgBoxIconError)
				case state != driver.Ready:
					walk.MsgBox(mw, "虚拟键盘尚未就绪", state.Text()+"。可稍后重新检测。", walk.MsgBoxIconInformation)
				default:
					walk.MsgBox(mw, "安装完成", "虚拟键盘已重新检测并可用。", walk.MsgBoxIconInformation)
				}
			})
		}()
	}
	applyCableStatus := func() {
		if cableInstalling.Load() {
			return
		}
		_ = cableLabel.SetText(cableStatusText(cable, cableErr))
		cableButton.SetEnabled(cableErr == nil && cable.State == vbcable.Missing && !cable.RestartRequired)
	}
	redetectCable := func() {
		go func() {
			v, err := vbcable.DetectAt(filepath.Join(dir, "vbcable", "Pack45"))
			mw.Synchronize(func() { cable, cableErr = v, err; applyCableStatus(); refreshAudio() })
		}()
	}
	installCable := func() {
		if !cableInstalling.CompareAndSwap(false, true) {
			return
		}
		cableButton.SetEnabled(false)
		_ = cableLabel.SetText("正在解包并启动 VB-Audio 官方安装向导，请处理管理员授权并在向导中完成安装…")
		go func() {
			r, err := cableManager.Install(filepath.Join(dir, "vbcable", "Pack45"))
			v, detectionErr := vbcable.DetectAt(filepath.Join(dir, "vbcable", "Pack45"))
			mw.Synchronize(func() {
				defer func() {
					cableInstalling.Store(false)
					applyCableStatus()
				}()
				cable, cableErr = v, detectionErr
				refreshAudio()
				switch {
				case err != nil:
					walk.MsgBox(mw, "VB-CABLE 安装", err.Error(), walk.MsgBoxIconInformation)
				case r.AlreadyInstalled:
					walk.MsgBox(mw, "已安装 VB-CABLE", r.Status.Text(), walk.MsgBoxIconInformation)
				case r.RestartRequired:
					walk.MsgBox(mw, "需要重启 Windows", "VB-Audio 要求安装后重启。请保存工作并自行安排重启；重启后在语音页重新检测。\n\nTapDeck 输出选择 CABLE Input；目标软件的麦克风选择 CABLE Output。", walk.MsgBoxIconInformation)
				}
			})
		}()
	}
	checkCableOnce := func() {
		if cablePrompted {
			return
		}
		cablePrompted = true
		if cableErr == nil && cable.State == vbcable.Missing && !cable.RestartRequired && walk.MsgBox(mw, "安装虚拟声卡", "语音传输需要 VB-Audio 的 VB-CABLE。TapDeck 已内置完整原包，可离线打开官方安装向导。\n\nVB-CABLE 是 donationware。安装需要管理员授权。\n\n现在打开安装向导？也可稍后从“语音”页安装。", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
			installCable()
		}
	}
	var prerequisitePromptActive bool
	checkPrerequisites := func() {
		if prerequisitePromptActive || pairingWaiting || prompts.dialog != nil || !mw.Visible() || !mw.Enabled() || installing.Load() || cableInstalling.Load() || agent.KeyboardStatus().Busy {
			return
		}
		prerequisitePromptActive = true
		defer func() { prerequisitePromptActive = false }()
		title, message := driverPrompt.Take(true, false, keyboardState, keyboardErr)
		if title != "" && walk.MsgBox(mw, title, message, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
			installKeyboard()
			return
		}
		checkCableOnce()
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
		c.Sensitivity = config.PointerBaseSensitivity
		c.NaturalScroll = natural.Checked()
		c.KeyboardBackend = backendIDs[max(0, backend.CurrentIndex())]
		c.AudioDevice = deviceIDs[max(0, devices.CurrentIndex())]
		for i := range voiceEditors {
			c.Voice.Profiles[i] = voiceEditors[i].value(c.Voice.Profiles[i].ID)
		}
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
		d.TabWidget{AssignTo: &tabs, OnCurrentIndexChanged: func() { updateSaveVisibility(tabs, saveButton) }, Pages: []d.TabPage{
			{Title: "连接", Layout: d.VBox{}, Children: append(connectionIntro(&address, &qrView, s.PairURL(), func() { _ = walk.Clipboard().SetText(s.PairURL()) }, func() { open(s.PairURL()) }), []d.Widget{
				d.Label{Text: "已配对设备（同名设备请核对设备标识）"},
				d.TableView{AssignTo: &pairedTable, Model: paired, MinSize: d.Size{Height: 140}, StretchFactor: 1, Columns: []d.TableViewColumn{{Title: "名称", Width: 130}, {Title: "状态", Width: 50}, {Title: "设备标识", Width: 210}, {Title: "最近连接", Width: 150}}, OnCurrentIndexChanged: func() {
					if unpairButton != nil {
						_, ok := paired.selected(pairedTable)
						unpairButton.SetEnabled(ok)
					}
				}},
				d.Composite{Layout: d.HBox{}, Children: []d.Widget{
					d.PushButton{AssignTo: &unpairButton, Text: "解除所选设备配对", Enabled: false, OnClicked: func() {
						v, ok := paired.selected(pairedTable)
						if !ok {
							return
						}
						if walk.MsgBox(mw, "解除设备配对", fmt.Sprintf("解除“%s”的配对？\n设备标识：%s\n此设备需要重新连接并由电脑允许。", v.Name, v.ID), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
							if err := s.UnpairDevice(v.ID); err != nil {
								walk.MsgBox(mw, "解绑失败", err.Error(), walk.MsgBoxIconError)
							}
						}
					}},
					d.PushButton{Text: "解除全部配对", OnClicked: func() {
						if walk.MsgBox(mw, "解除全部配对", "解除所有已配对设备，并断开全部控制端？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
							if err := s.Unpair(); err != nil {
								walk.MsgBox(mw, "解绑失败", err.Error(), walk.MsgBoxIconError)
							}
						}
					}},
				}},
			}...)},
			{Title: "快捷键", Layout: d.VBox{}, Children: []d.Widget{
				keyboardEnvironmentWidgets(&backend, backendNames, selectedBackend, &keyboardLabel, &driverButton, installKeyboard, redetectKeyboard),
				shortcutSettingsWidgets(func() walk.Form { return mw }, &labels, &keys, &enabled, cfg.Shortcuts),
			}},
			{Title: "语音", Layout: d.VBox{}, Children: []d.Widget{
				voiceEnvironmentWidgets(voiceEnvironmentOptions{
					cableStatus: &cableLabel, cableButton: &cableButton, level: &audioStatus, devices: &devices, gain: &gain,
					deviceNames: deviceNames, selectedDevice: selectedDevice, initialGain: cfg.Gain,
					install: installCable, detect: redetectCable, website: func() { open(vbcable.Website) }, license: func() { open(cableLicense) },
					refresh: refreshAudio, settings: func() {
						if err := launchSoundInputSettings(shellOpen); err != nil {
							walk.MsgBox(mw, "音频输入设置", err.Error(), walk.MsgBoxIconError)
						}
					},
				}),
				settingsSection("语音快捷键设置", 1,
					d.Label{Text: "名称：最多 8 个汉字 / 16 个英文字符。热键：留空仅传音。"},
					voiceProfileWidgets(func() walk.Form { return mw }, &voiceEditors, cfg.Voice.Profiles),
					d.Composite{Layout: d.HBox{}, Children: []d.Widget{d.Label{Text: "尾音结束延迟 ms"}, d.NumberEdit{AssignTo: &delay, Value: float64(cfg.Voice.StopDelayMS), MinValue: 0, MaxValue: 1000}}},
				),
			}},
			{Title: "设置与状态", Layout: d.VBox{}, Children: []d.Widget{d.Label{Text: "HTTP / WSS / UDP 端口（修改后重启连接）"}, d.NumberEdit{AssignTo: &httpPort, Value: float64(cfg.HTTPPort), MinValue: 1024, MaxValue: 65535}, d.NumberEdit{AssignTo: &wssPort, Value: float64(cfg.WSSPort), MinValue: 1024, MaxValue: 65535}, d.NumberEdit{AssignTo: &udpPort, Value: float64(cfg.UDPPort), MinValue: 1024, MaxValue: 65535}, d.Label{Text: "触控板灵敏度：在各 Android 设备触控板左上角设置（0.5–3 倍）"}, d.Label{Text: "手机震动开关：Android 顶部连接图标 → 连接与设置"}, d.CheckBox{AssignTo: &natural, Text: "自然滚动", Checked: cfg.NaturalScroll}, d.CheckBox{AssignTo: &autostartBox, Text: "随 Windows 登录自动启动接收端", Checked: autostartEnabled(), OnCheckedChanged: func() {
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
			{Title: "关于", Layout: d.VBox{}, Children: []d.Widget{
				d.Label{Text: "TapDeck", Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 20, Bold: true}},
				d.Label{Text: "Windows 接收端 · 版本 " + appVersion},
				d.Label{Text: "将 Android 手机变成电脑的触控板、快捷键面板、键盘和语音输入控制器。"},
				d.Label{Text: "支持 Windows 11 x64、Android 8 及以上；手机与电脑需处于同一局域网。"},
				d.Label{Text: "开源许可：MIT · 第三方组件保留各自许可"},
				d.PushButton{Text: "项目主页 · github.com/fly3457/TapDeck", OnClicked: func() { open("https://github.com/fly3457/TapDeck") }},
				d.Label{Text: apkSummary()},
				d.Label{Text: "手机输入测试", Font: d.Font{Family: "Microsoft YaHei UI", PointSize: 11, Bold: true}},
				d.Label{Text: "先点击下方输入框，再使用手机键盘或语音输入。语音文字由电脑当前输入法识别。"},
				d.TextEdit{AssignTo: &inputTest, Name: "phoneInputTest", VScroll: true, MinSize: d.Size{Height: 180}, StretchFactor: 1},
				d.Composite{Layout: d.HBox{}, Children: []d.Widget{
					d.PushButton{Text: "开始输入测试", OnClicked: func() { _ = inputTest.SetFocus() }},
					d.PushButton{Text: "清空", OnClicked: func() { _ = inputTest.SetText(""); _ = inputTest.SetFocus() }},
					d.HSpacer{},
				}},
			}},
		}}, d.PushButton{AssignTo: &saveButton, Text: "保存并同步配置", OnClicked: save, Visible: false},
	}}).Create()
	if err != nil {
		return err
	}
	// Reapply PC-owned values after Walk's declarative initialization.
	for i := range enabled {
		enabled[i].SetChecked(cfg.Shortcuts[i].Enabled)
	}
	for i := range voiceEditors {
		voiceEditors[i].syncEnabled()
	}
	natural.SetChecked(cfg.NaturalScroll)
	updateSaveVisibility(tabs, saveButton)
	defer mw.Dispose()
	defer func() {
		if qrBitmap != nil {
			qrBitmap.Dispose()
		}
	}()
	refreshQR()
	applyCableStatus()
	applyKeyboardStatus()
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
	showSettings := func() {
		if !mw.Visible() {
			for i := range voiceEditors {
				voiceEditors[i].syncEnabled()
			}
		}
		restoreSettingsWindow(mw)
	}
	prompts = pairingPrompts{owner: mw, approve: s.Approve, showOwner: showSettings}
	show := func() { showSettings(); checkPrerequisites() }
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
	if !startHidden {
		mw.Synchronize(checkPrerequisites)
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
				var cableUpdate bool
				var detectedCable vbcable.Status
				var detectionErr error
				var detectedKeyboard driver.State
				var keyboardDetectionErr error
				if time.Since(lastMetrics) >= 5*time.Second {
					detectedCable, detectionErr = vbcable.DetectAt(filepath.Join(dir, "vbcable", "Pack45"))
					detectedKeyboard, keyboardDetectionErr = driver.Detect(kb.Driver != "")
					cableUpdate = true
					metrics, _ := json.Marshal(map[string]any{"at": time.Now().Format(time.RFC3339), "connected": v.Device != "", "mouse_packets": v.MousePackets, "audio_packets": v.AudioPackets, "injection_p95_ms": v.InjectionP95MS, "buffered_frames": v.BufferedFrames, "max_buffered_frames": v.MaxBufferedFrames, "concealed_frames": v.Concealed, "audio_ready": v.AudioReady})
					_ = config.AtomicWrite(filepath.Join(dir, "runtime-stats.json"), metrics)
					lastMetrics = time.Now()
				}
				if !uiPending.CompareAndSwap(false, true) {
					continue
				}
				mw.Synchronize(func() {
					defer uiPending.Store(false)
					paired.update(pairedTable, v.PairedDevices)
					_, selected := paired.selected(pairedTable)
					unpairButton.SetEnabled(selected)
					if cableUpdate {
						cable, cableErr = detectedCable, detectionErr
						applyCableStatus()
						keyboardState, keyboardErr = detectedKeyboard, keyboardDetectionErr
					}
					text := "接收已停止"
					if v.Running {
						text = "等待 Android 连接"
					}
					switch {
					case len(v.Devices) == 1:
						text = "已连接：" + v.Devices[0]
					case len(v.Devices) > 1:
						text = fmt.Sprintf("已连接 %d 个控制端：%s", len(v.Devices), strings.Join(v.Devices, "、"))
					}
					_ = status.SetText(text)
					applyKeyboardStatus()
					_ = audioStatus.SetText(fmt.Sprintf("输入电平 %.0f%%", v.Level*100))
					_ = audioStatus.SetToolTipText(v.AudioStatus)
					_ = stats.SetText(fmt.Sprintf("鼠标包 %d · 音频包 %d · 注入 p95 %.3f ms\n音频缓冲 %d/6 帧 · 历史最大 %d 帧 · 补静音帧 %d", v.MousePackets, v.AudioPackets, v.InjectionP95MS, v.BufferedFrames, v.MaxBufferedFrames, v.Concealed))
					if address.Text() != v.URL {
						_ = address.SetText(v.URL)
						refreshQR()
					}
					pairingWaiting = len(v.Pending) > 0
					if !prerequisitePromptActive {
						prompts.update(v.Pending, time.Now())
					}
					checkPrerequisites()
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
