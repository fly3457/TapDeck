//go:build windows

package hidkeyboard

import (
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestAbsoluteMouseMapping 实测 HID 绝对坐标映射到主显示器还是整个虚拟桌面。
// 会短暂移动本机光标并恢复，仅在 TAPDECK_HID_MOUSE_PROBE=1 时运行。
func TestAbsoluteMouseMapping(t *testing.T) {
	if os.Getenv("TAPDECK_HID_MOUSE_PROBE") != "1" {
		t.Skip("设置 TAPDECK_HID_MOUSE_PROBE=1 才实测")
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	user32.NewProc("SetProcessDpiAwarenessContext").Call(^uintptr(3))
	var orig, got struct{ X, Y int32 }
	pos := user32.NewProc("GetPhysicalCursorPos")
	pos.Call(uintptr(unsafe.Pointer(&orig)))
	d, err := Open()
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()
	defer user32.NewProc("SetPhysicalCursorPos").Call(uintptr(orig.X), uintptr(orig.Y))
	metric := func(i uintptr) int32 { v, _, _ := user32.NewProc("GetSystemMetrics").Call(i); return int32(v) }
	// 实测：(0,0) 被 Windows 视为“无位置”而忽略，(1,1) 才对应左上角像素。
	for _, p := range [][2]uint16{{AbsMax / 2, AbsMax / 2}, {0, 0}, {1, 1}, {AbsMax, AbsMax}} {
		if err = d.Mouse(0, p[0], p[1]); err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
		pos.Call(uintptr(unsafe.Pointer(&got)))
		t.Logf("hid (%d,%d) -> cursor (%d,%d)", p[0], p[1], got.X, got.Y)
	}
	t.Logf("primary %dx%d, virtual (%d,%d) %dx%d", metric(0), metric(1), metric(76), metric(77), metric(78), metric(79))
}
