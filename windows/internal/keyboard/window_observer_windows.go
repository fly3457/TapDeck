//go:build windows

package keyboard

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var windowUser32 = windows.NewLazySystemDLL("user32.dll")
var setWindowEventHook = windowUser32.NewProc("SetWinEventHook")
var unhookWindowEvent = windowUser32.NewProc("UnhookWinEvent")
var postWindowThreadMessage = windowUser32.NewProc("PostThreadMessageW")

type nativeWindowObserver struct {
	changed    chan struct{}
	done       chan struct{}
	thread     atomic.Uint32
	taskWindow atomic.Uintptr
	closeOnce  sync.Once
}

func newNativeWindowObserver() (*nativeWindowObserver, error) {
	o := &nativeWindowObserver{changed: make(chan struct{}, 1), done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(o.done)
		var message win.MSG
		win.PeekMessage(&message, 0, 0, 0, 0) // Create the message queue before publishing its thread ID.
		o.thread.Store(windows.GetCurrentThreadId())
		callback := syscall.NewCallback(func(_ uintptr, event uint32, _ uintptr, object int32, _ int32, _ uint32, _ uint32) uintptr {
			if event == 3 || object == 0 {
				select {
				case o.changed <- struct{}{}:
				default:
				}
			}
			return 0
		})
		foreground, _, err := setWindowEventHook.Call(3, 3, 0, callback, 0, 0, 0)
		if foreground == 0 {
			ready <- fmt.Errorf("监听前台窗口失败: %v", err)
			return
		}
		defer unhookWindowEvent.Call(foreground)
		visibility, _, err := setWindowEventHook.Call(0x8001, 0x8003, 0, callback, 0, 0, 0)
		if visibility == 0 {
			ready <- fmt.Errorf("监听窗口关闭失败: %v", err)
			return
		}
		defer unhookWindowEvent.Call(visibility)
		ready <- nil
		for win.GetMessage(&message, 0, 0, 0) > 0 {
			win.TranslateMessage(&message)
			win.DispatchMessage(&message)
		}
	}()
	if err := <-ready; err != nil {
		<-o.done
		return nil, err
	}
	return o, nil
}
func windowClass(hwnd uintptr) string {
	var buffer [256]uint16
	n, err := win.GetClassName(win.HWND(hwnd), &buffer[0], len(buffer))
	if err != nil || n <= 0 {
		return ""
	}
	return windows.UTF16ToString(buffer[:n])
}
func shellView(hwnd uintptr) bool {
	class := windowClass(hwnd)
	if class == "MultitaskingViewFrame" || class == "TaskViewFrame" {
		return true
	}
	if class != "XamlExplorerHostIslandWindow" && class != "Windows.UI.Core.CoreWindow" {
		return false
	}
	var pid uint32
	win.GetWindowThreadProcessId(win.HWND(hwnd), &pid)
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	var buffer [1024]uint16
	length := uint32(len(buffer))
	if windows.QueryFullProcessImageName(process, 0, &buffer[0], &length) != nil {
		return false
	}
	name := strings.ToLower(filepath.Base(windows.UTF16ToString(buffer[:length])))
	return name == "explorer.exe" || name == "shellexperiencehost.exe"
}
func (o *nativeWindowObserver) Observe() windowObservation {
	hwnd := uintptr(win.GetForegroundWindow())
	class := windowClass(hwnd)
	task := o.taskWindow.Load()
	// Windows 11's Explorer Task View foreground uses this XAML host class.
	// Check the owning shell process so another application's XAML window
	// cannot be mistaken for it; this also recognizes a manually opened view.
	taskView := class == "MultitaskingViewFrame" || class == "TaskViewFrame" || (class == "XamlExplorerHostIslandWindow" && shellView(hwnd))
	if task != 0 {
		// Shell animation can briefly hide its still-foreground surface.
		// Do not discard the identity until it has actually left foreground.
		if hwnd == task || win.IsChild(win.HWND(task), win.HWND(hwnd)) {
			taskView = true
		} else if !win.IsWindowVisible(win.HWND(task)) {
			o.taskWindow.CompareAndSwap(task, 0)
		}
	}
	return windowObservation{Foreground: hwnd, Desktop: class == "Progman" || class == "WorkerW", TaskView: taskView}
}
func (o *nativeWindowObserver) After(ctx context.Context, target windowMode, before uintptr) (windowObservation, error) {
	started := time.Now()
	waitCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	var stableSince time.Time
	var stableWindow uintptr
	for {
		observation := o.Observe()
		// Capture the shell surface opened by OUR Win+Tab. Do not use a
		// localized window title or classify a regular Explorer folder as it.
		if target == windowTaskView && !observation.TaskView && observation.Foreground != 0 && observation.Foreground != before && !observation.Desktop && shellView(observation.Foreground) {
			o.taskWindow.Store(observation.Foreground)
			observation.TaskView = true
		}
		actual := windowNormal
		if observation.TaskView {
			actual = windowTaskView
		} else if observation.Desktop {
			actual = windowDesktop
		}
		if observation.Foreground != 0 && actual == target {
			if stableWindow != observation.Foreground {
				stableWindow, stableSince = observation.Foreground, time.Now()
			}
			// Win+Tab can replace its initial animation surface. Wait for a
			// stable foreground identity before retaining it for later gestures.
			if time.Since(stableSince) >= 150*time.Millisecond && time.Since(started) >= 450*time.Millisecond {
				return observation, nil
			}
		} else {
			stableWindow = 0
		}
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return o.Observe(), errCanceled
			}
			return o.Observe(), fmt.Errorf("Windows 窗口状态未改变，请松开修饰键后重试")
		case <-ticker.C:
		}
	}
}
func (o *nativeWindowObserver) Changes() <-chan struct{} { return o.changed }
func (o *nativeWindowObserver) Done() <-chan struct{}    { return o.done }
func (o *nativeWindowObserver) Close() {
	o.closeOnce.Do(func() {
		select {
		case <-o.done:
			return
		default:
		}
		postWindowThreadMessage.Call(uintptr(o.thread.Load()), 0x12, 0, 0)
		<-o.done
	})
}
