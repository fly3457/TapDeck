//go:build windows

// hidprobe is a local-only diagnostic. It sends keys exclusively while its own
// editable test window is foreground, and records both hook flags and Raw Input.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"tapdeck/internal/hidkeyboard"
	"tapdeck/internal/input"
)

var u = windows.NewLazySystemDLL("user32.dll")

type hookData struct {
	VK, Scan, Flags, Time uint32
	Extra                 uintptr
}
type rawDevice struct {
	Page, Usage uint16
	Flags       uint32
	Target      uintptr
}
type rawHeader struct {
	Type, Size     uint32
	Device, WParam uintptr
}
type rawKeyboard struct {
	Make, Flags, Reserved, VK uint16
	Message, Extra            uint32
}
type command struct {
	ID     int    `json:"id"`
	Action string `json:"action"`
	Chord  string `json:"chord"`
	MS     int    `json:"ms"`
}

func main() {
	dir := flag.String("out", ".", "diagnostic files directory")
	flag.Parse()
	if e := os.MkdirAll(*dir, 0700); e != nil {
		panic(e)
	}
	dev, e := hidkeyboard.Open()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	defer dev.Close()
	defer dev.Update(nil)
	info, _ := json.Marshal(map[string]any{"path": dev.Path, "api": dev.APIVersion, "device_version": dev.DeviceVersion})
	os.WriteFile(filepath.Join(*dir, "device.json"), info, 0600)
	f, e := os.OpenFile(filepath.Join(*dir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		panic(e)
	}
	defer f.Close()
	var logMu sync.Mutex
	var current atomic.Int64
	record := func(v map[string]any) {
		v["at"] = time.Now().Format(time.RFC3339Nano)
		v["command"] = current.Load()
		logMu.Lock()
		defer logMu.Unlock()
		_ = json.NewEncoder(f).Encode(v)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var mw *walk.MainWindow
	var edit *walk.TextEdit
	if e = (d.MainWindow{AssignTo: &mw, Title: "TapDeck HID 诊断 · 专用测试输入框", Size: d.Size{Width: 640, Height: 380}, Layout: d.VBox{}, Children: []d.Widget{d.Label{Text: "仅向此窗口发送诊断键；关闭窗口立即释放虚拟键盘。"}, d.TextEdit{AssignTo: &edit, VScroll: true}}}).Create(); e != nil {
		panic(e)
	}
	defer mw.Dispose()
	raw := rawDevice{Page: 1, Usage: 6, Flags: 0x100, Target: uintptr(mw.Handle())}
	if ok, _, er := u.NewProc("RegisterRawInputDevices").Call(uintptr(unsafe.Pointer(&raw)), 1, unsafe.Sizeof(raw)); ok == 0 {
		panic(er)
	}
	var previous uintptr
	windowProc := windows.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
		fg, _, _ := u.NewProc("GetForegroundWindow").Call()
		if msg == 0xFF && fg == uintptr(mw.Handle()) {
			var needed uint32
			u.NewProc("GetRawInputData").Call(lp, 0x10000003, 0, uintptr(unsafe.Pointer(&needed)), unsafe.Sizeof(rawHeader{}))
			if needed >= 40 && needed < 4096 {
				b := make([]byte, needed)
				n, _, _ := u.NewProc("GetRawInputData").Call(lp, 0x10000003, uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&needed)), unsafe.Sizeof(rawHeader{}))
				if n != ^uintptr(0) {
					h := (*rawHeader)(unsafe.Pointer(&b[0]))
					if h.Type == 1 {
						k := (*rawKeyboard)(unsafe.Pointer(&b[24]))
						var size uint32
						u.NewProc("GetRawInputDeviceInfoW").Call(h.Device, 0x20000007, 0, uintptr(unsafe.Pointer(&size)))
						name := make([]uint16, size+1)
						u.NewProc("GetRawInputDeviceInfoW").Call(h.Device, 0x20000007, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&size)))
						record(map[string]any{"source": "raw", "vk": k.VK, "scan": k.Make, "flags": k.Flags, "device": windows.UTF16ToString(name)})
					}
				}
			}
		}
		return win.CallWindowProc(previous, win.HWND(hwnd), msg, wp, lp)
	})
	previous = win.SetWindowLongPtr(mw.Handle(), win.GWL_WNDPROC, windowProc)
	defer win.SetWindowLongPtr(mw.Handle(), win.GWL_WNDPROC, previous)
	callback := windows.NewCallback(func(code int, wp uintptr, k *hookData) uintptr {
		fg, _, _ := u.NewProc("GetForegroundWindow").Call()
		if code >= 0 && fg == uintptr(mw.Handle()) {
			record(map[string]any{"source": "hook", "vk": k.VK, "scan": k.Scan, "flags": k.Flags, "injected": k.Flags&0x10 != 0, "message": wp})
		}
		r, _, _ := u.NewProc("CallNextHookEx").Call(0, uintptr(code), wp, uintptr(unsafe.Pointer(k)))
		return r
	})
	hook, _, er := u.NewProc("SetWindowsHookExW").Call(13, callback, 0, 0)
	if hook == 0 {
		panic(er)
	}
	defer u.NewProc("UnhookWindowsHookEx").Call(hook)
	quit := make(chan struct{})
	done := make(chan struct{})
	defer func() { close(quit); <-done }()
	go func() {
		defer close(done)
		last := 0
		keys := map[uint16]int{}
		var heldAt time.Time
		defer dev.Update(nil)
		for {
			select {
			case <-quit:
				return
			case <-time.After(20 * time.Millisecond):
			}
			fgNow, _, _ := u.NewProc("GetForegroundWindow").Call()
			if (fgNow != uintptr(mw.Handle()) || (!heldAt.IsZero() && time.Since(heldAt) > 8*time.Second)) && len(keys) > 0 {
				keys = map[uint16]int{}
				_ = dev.Update(nil)
				record(map[string]any{"source": "focus_lost_release"})
			}
			b, _ := os.ReadFile(filepath.Join(*dir, "command.json"))
			var c command
			if json.Unmarshal(b, &c) != nil || c.ID <= last {
				continue
			}
			last = c.ID
			current.Store(int64(c.ID))
			var err error
			ks, err := input.ParseChord(c.Chord)
			fg, _, _ := u.NewProc("GetForegroundWindow").Call()
			if c.Action != "release" && fg != uintptr(mw.Handle()) {
				err = fmt.Errorf("测试窗口未获得前台焦点")
			}
			if err == nil {
				switch c.Action {
				case "down":
					heldAt = time.Now()
					for _, k := range ks {
						keys[k]++
					}
					err = dev.Update(keys)
				case "up":
					for _, k := range ks {
						if keys[k] > 0 {
							keys[k]--
						}
					}
					err = dev.Update(keys)
				case "tap":
					for _, k := range ks {
						keys[k]++
					}
					err = dev.Update(keys)
					select {
					case <-quit:
						return
					case <-time.After(50 * time.Millisecond):
					}
					for _, k := range ks {
						keys[k]--
					}
					if e := dev.Update(keys); err == nil {
						err = e
					}
				case "release":
					keys = map[uint16]int{}
					err = dev.Update(keys)
				default:
					err = fmt.Errorf("未知诊断动作")
				}
			}
			status := map[string]any{"id": c.ID, "action": c.Action, "chord": c.Chord, "error": ""}
			if err != nil {
				status["error"] = err.Error()
				keys = map[uint16]int{}
				dev.Update(nil)
			}
			record(map[string]any{"source": "result", "result": status})
			out, _ := json.Marshal(status)
			os.WriteFile(filepath.Join(*dir, "result.json"), out, 0600)
		}
	}()
	mw.Show()
	mw.Activate()
	edit.SetFocus()
	mw.Run()
}
