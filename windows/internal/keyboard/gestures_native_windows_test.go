//go:build windows

package keyboard

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"tapdeck/internal/input"
)

// Explicit opt-in: these tests briefly operate the actual interactive desktop.
// Ordinary builds run the deterministic fixtures and never open Task View.
type nativeMouseEvent struct {
	message uint32
	flags   uint16
	delta   int16
}
type gestureTestWindow struct {
	hwnd   win.HWND
	mu     sync.Mutex
	events []nativeMouseEvent
	done   chan struct{}
}

func gestureWindow(t *testing.T, suffix string) *gestureTestWindow {
	t.Helper()
	f := &gestureTestWindow{done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(f.done)
		name, _ := windows.UTF16PtrFromString(fmt.Sprintf("TapDeckGestureTest%d%s%d", os.Getpid(), suffix, time.Now().UnixNano()))
		title, _ := windows.UTF16PtrFromString("TapDeck gesture validation (temporary)")
		proc := syscall.NewCallback(func(hwnd win.HWND, msg uint32, w, l uintptr) uintptr {
			switch msg {
			case win.WM_LBUTTONDOWN, win.WM_LBUTTONUP, win.WM_LBUTTONDBLCLK, win.WM_MOUSEWHEEL:
				f.mu.Lock()
				f.events = append(f.events, nativeMouseEvent{msg, uint16(w), int16(w >> 16)})
				f.mu.Unlock()
				return 0
			case win.WM_CLOSE:
				win.DestroyWindow(hwnd)
				return 0
			case win.WM_DESTROY:
				win.PostQuitMessage(0)
				return 0
			}
			return win.DefWindowProc(hwnd, msg, w, l)
		})
		class := win.WNDCLASSEX{Style: win.CS_DBLCLKS, LpfnWndProc: proc, HInstance: win.GetModuleHandle(nil), LpszClassName: name, HbrBackground: win.HBRUSH(win.COLOR_WINDOW + 1)}
		class.CbSize = uint32(unsafe.Sizeof(class))
		if win.RegisterClassEx(&class) == 0 {
			ready <- fmt.Errorf("RegisterClassEx failed")
			return
		}
		defer win.UnregisterClass(name)
		f.hwnd = win.CreateWindowEx(0, name, title, win.WS_OVERLAPPEDWINDOW, 200, 200, 600, 400, 0, 0, class.HInstance, nil)
		if f.hwnd == 0 {
			ready <- fmt.Errorf("CreateWindowEx failed")
			return
		}
		win.ShowWindow(f.hwnd, win.SW_SHOW)
		ready <- nil
		var message win.MSG
		for win.GetMessage(&message, 0, 0, 0) > 0 {
			win.TranslateMessage(&message)
			win.DispatchMessage(&message)
		}
	}()
	if err := <-ready; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { win.PostMessage(f.hwnd, win.WM_CLOSE, 0, 0); <-f.done })
	return f
}
func (f *gestureTestWindow) snapshot() []nativeMouseEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]nativeMouseEvent(nil), f.events...)
}
func (f *gestureTestWindow) clear() { f.mu.Lock(); f.events = nil; f.mu.Unlock() }
func nativeAwait(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func nativeOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv("TAPDECK_NATIVE_GESTURE_TEST") != "1" {
		t.Skip("requires isolated interactive Windows acceptance")
	}
	if modifiers := input.CurrentModifiers(); modifiers != (input.Modifiers{}) {
		t.Fatal("release keyboard modifiers before native validation")
	}
}
func focusGestureWindow(t *testing.T, f *gestureTestWindow) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	foregroundThread := win.GetWindowThreadProcessId(win.GetForegroundWindow(), nil)
	currentThread := windows.GetCurrentThreadId()
	if foregroundThread != 0 && foregroundThread != currentThread {
		attached, _, _ := windowUser32.NewProc("AttachThreadInput").Call(uintptr(currentThread), uintptr(foregroundThread), 1)
		if attached != 0 {
			defer windowUser32.NewProc("AttachThreadInput").Call(uintptr(currentThread), uintptr(foregroundThread), 0)
		}
	}
	win.ShowWindow(f.hwnd, win.SW_RESTORE)
	win.SetForegroundWindow(f.hwnd)
	point := win.POINT{X: 120, Y: 120}
	win.ClientToScreen(f.hwnd, &point)
	win.SetCursorPos(point.X, point.Y)
	if win.GetForegroundWindow() != f.hwnd {
		// Windows can deny SetForegroundWindow to a background test runner.
		// Activate only our temporary window through a real, targeted click.
		win.SetWindowPos(f.hwnd, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
		time.Sleep(150 * time.Millisecond)
		win.SetCursorPos(point.X, point.Y)
		mouse := input.New()
		if err := mouse.Button("left", true); err != nil {
			t.Fatal(err)
		}
		if err := mouse.Button("left", false); err != nil {
			t.Fatal(err)
		}
		win.SetWindowPos(f.hwnd, win.HWND_NOTOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
	}
	nativeAwait(t, fmt.Sprintf("test window could not take focus (foreground %x, class %s)", win.GetForegroundWindow(), windowClass(uintptr(win.GetForegroundWindow()))), func() bool { return win.GetForegroundWindow() == f.hwnd })
	time.Sleep(time.Duration(input.DoubleClickTime()+50) * time.Millisecond)
	f.clear()
}
func TestNativeDoubleClickTimingAndZoom(t *testing.T) {
	nativeOptIn(t)
	foreground := win.GetForegroundWindow()
	var cursor win.POINT
	win.GetCursorPos(&cursor)
	t.Cleanup(func() { win.SetForegroundWindow(foreground); win.SetCursorPos(cursor.X, cursor.Y) })
	f := gestureWindow(t, "mouse")
	focusGestureWindow(t, f)
	c := input.New()
	t.Cleanup(c.ReleaseAll)
	button := func(down bool) {
		t.Helper()
		if err := c.Button("left", down); err != nil {
			t.Fatal(err)
		}
	}
	button(true)
	nativeAwait(t, "first PC mouse-down missing", func() bool { return len(f.snapshot()) == 1 })
	time.Sleep(40 * time.Millisecond)
	if got := f.snapshot(); !reflect.DeepEqual(got, []nativeMouseEvent{{message: win.WM_LBUTTONDOWN, flags: win.MK_LBUTTON}}) {
		t.Fatal("double click happened while the second finger was held", got)
	}
	button(false)
	button(true)
	button(false)
	nativeAwait(t, "double-click sequence missing", func() bool { return len(f.snapshot()) == 4 })
	got := f.snapshot()
	want := []uint32{win.WM_LBUTTONDOWN, win.WM_LBUTTONUP, win.WM_LBUTTONDBLCLK, win.WM_LBUTTONUP}
	for i, event := range got {
		if event.message != want[i] {
			t.Fatal("wrong mouse sequence", got)
		}
	}
	t.Log("second finger DOWN: only WM_LBUTTONDOWN; second finger UP: UP, DBLCLK, UP")
	time.Sleep(time.Duration(input.DoubleClickTime()+50) * time.Millisecond)
	f.clear()
	button(true)
	time.Sleep(320 * time.Millisecond)
	button(false)
	nativeAwait(t, "held click release missing", func() bool { return len(f.snapshot()) == 2 })
	for _, event := range f.snapshot() {
		if event.message == win.WM_LBUTTONDBLCLK {
			t.Fatal("held tap double clicked")
		}
	}
	f.clear()
	if err := c.ZoomSteps(1, false); err != nil {
		t.Fatal(err)
	}
	if err := c.ZoomSteps(-1, false); err != nil {
		t.Fatal(err)
	}
	nativeAwait(t, "zoom wheels missing", func() bool { return len(f.snapshot()) == 2 })
	got = f.snapshot()
	if got[0].flags&win.MK_CONTROL == 0 || got[1].flags&win.MK_CONTROL == 0 || got[0].delta != 120 || got[1].delta != -120 {
		t.Fatal("Ctrl+wheel not received", got)
	}
	if input.CurrentModifiers().Ctrl {
		t.Fatal("temporary Ctrl was not released")
	}
	if err := c.Hold("RightCtrl", true); err != nil {
		t.Fatal(err)
	}
	if err := c.ZoomSteps(1, true); err != nil {
		t.Fatal(err)
	}
	if !input.CurrentModifiers().Ctrl {
		t.Fatal("existing Ctrl was released")
	}
	if err := c.Hold("RightCtrl", false); err != nil {
		t.Fatal(err)
	}
	t.Log("native wheel received +/-120 with MK_CONTROL; temporary Ctrl released, existing RightCtrl preserved")
}
func TestNativeThreeFingerDesktopAndTaskView(t *testing.T) {
	nativeOptIn(t)
	foreground := win.GetForegroundWindow()
	t.Cleanup(func() { win.SetForegroundWindow(foreground) })
	f := gestureWindow(t, "windows")
	minimized := gestureWindow(t, "minimized")
	win.ShowWindow(minimized.hwnd, win.SW_MINIMIZE)
	focusGestureWindow(t, f)
	soft := input.New()
	e := &Engine{mode: "sendinput", hidKeys: map[uint16]int{}, owners: map[uint16]keyOwner{}, soft: soft, holds: map[string][]string{}, voices: map[string]voice{}}
	t.Cleanup(e.Close)
	t.Cleanup(func() {
		if e.navigator != nil {
			obs := e.navigator.observer.Observe()
			if obs.TaskView || shellView(obs.Foreground) {
				_ = soft.Chord("Esc")
				time.Sleep(250 * time.Millisecond)
			}
			if obs.Desktop {
				_ = soft.Chord("LeftWin+D")
				time.Sleep(250 * time.Millisecond)
			}
		}
	})
	run := func(direction string) {
		t.Helper()
		if err := e.gesture(context.Background(), direction); err != nil {
			t.Fatalf("gesture %s: %v; foreground class %s", direction, err, windowClass(uintptr(win.GetForegroundWindow())))
		}
		time.Sleep(300 * time.Millisecond)
		t.Logf("%s: %+v class=%s", direction, e.navigator.observer.Observe(), windowClass(uintptr(win.GetForegroundWindow())))
	}
	assertMode := func(want windowMode) {
		t.Helper()
		obs := e.navigator.observer.Observe()
		actual := windowNormal
		if obs.TaskView {
			actual = windowTaskView
		} else if obs.Desktop {
			actual = windowDesktop
		}
		if actual != want {
			t.Fatalf("wrong native window mode: want %v got %+v, class=%s", want, obs, windowClass(obs.Foreground))
		}
	}
	// Recognize Task View opened outside TapDeck, before the observer exists.
	if err := soft.Chord("LeftWin+Tab"); err != nil {
		t.Fatal(err)
	}
	nativeAwait(t, "external Task View did not open", func() bool { return windowClass(uintptr(win.GetForegroundWindow())) == "XamlExplorerHostIslandWindow" })
	time.Sleep(450 * time.Millisecond)
	run("down")
	assertMode(windowNormal)
	nativeAwait(t, "external Task View did not return to original window", func() bool { return win.GetForegroundWindow() == f.hwnd })
	run("up")
	assertMode(windowTaskView)
	run("up")
	assertMode(windowTaskView)
	run("down")
	assertMode(windowNormal)
	nativeAwait(t, "Task View did not return to original window", func() bool { return win.GetForegroundWindow() == f.hwnd })
	run("down")
	assertMode(windowDesktop)
	run("down")
	assertMode(windowDesktop)
	run("up")
	assertMode(windowNormal)
	nativeAwait(t, "desktop did not restore the original window", func() bool { return win.GetForegroundWindow() == f.hwnd })
	if !win.IsIconic(minimized.hwnd) {
		t.Fatal("pre-minimized window was incorrectly restored")
	}
	run("up")
	assertMode(windowTaskView)
	if err := soft.Chord("Esc"); err != nil {
		t.Fatal(err)
	}
	nativeAwait(t, "external Esc did not close Task View", func() bool { return win.GetForegroundWindow() == f.hwnd && !e.navigator.observer.Observe().TaskView })
	time.Sleep(300 * time.Millisecond)
	run("up")
	assertMode(windowTaskView)
	run("down")
	assertMode(windowNormal)
	t.Log("Task View and desktop transitions, repeated gesture no-ops, external Esc resync, pre-minimized window preservation passed")
}
