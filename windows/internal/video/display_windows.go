//go:build windows

package video

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dxgi                     = windows.NewLazySystemDLL("dxgi.dll")
	procCreateDXGIFactory1   = dxgi.NewProc("CreateDXGIFactory1")
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procGetPhysicalCursorPos = user32.NewProc("GetPhysicalCursorPos")
	procGetCursorInfo        = user32.NewProc("GetCursorInfo")
	iidIDXGIFactory1         = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
)

type comObject struct{ vtbl *[32]uintptr }

func (o *comObject) call(index int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[index], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}
func (o *comObject) release() { o.call(2) }

type dxgiOutputDesc struct {
	DeviceName [32]uint16
	Left, Top  int32
	Right      int32
	Bottom     int32
	Attached   int32
	Rotation   uint32
	Monitor    uintptr
}

// PrimaryDisplay 返回桌面坐标原点所在的 DXGI 输出（Windows 主显示器）。
// 序号与 ffmpeg 的 d3d11va 设备序号、ddagrab output_idx 一致。
func PrimaryDisplay() (Display, error) {
	var factory *comObject
	if hr, _, _ := procCreateDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&factory))); int32(hr) < 0 || factory == nil {
		return Display{}, fmt.Errorf("无法创建 DXGI 工厂：0x%08x", uint32(hr))
	}
	defer factory.release()
	for a := 0; a < 16; a++ {
		var adapter *comObject
		if int32(factory.call(7, uintptr(a), uintptr(unsafe.Pointer(&adapter)))) < 0 || adapter == nil { // EnumAdapters
			break
		}
		for o := 0; o < 16; o++ {
			var output *comObject
			if int32(adapter.call(7, uintptr(o), uintptr(unsafe.Pointer(&output)))) < 0 || output == nil { // EnumOutputs
				break
			}
			var desc dxgiOutputDesc
			hr := output.call(7, uintptr(unsafe.Pointer(&desc))) // GetDesc
			output.release()
			if int32(hr) >= 0 && desc.Attached != 0 && desc.Left == 0 && desc.Top == 0 {
				adapter.release()
				return Display{Adapter: a, Output: o, Width: int(desc.Right - desc.Left), Height: int(desc.Bottom - desc.Top)}, nil
			}
		}
		adapter.release()
	}
	return Display{}, fmt.Errorf("未找到主显示器的 DXGI 输出")
}

type cursorInfo struct {
	Size   uint32
	Flags  uint32
	Cursor uintptr
	X, Y   int32
}

// CursorOn 读取物理像素光标位置，换算为相对 d 的坐标。
func CursorOn(d Display) (int, int, bool) {
	var p struct{ X, Y int32 }
	if r, _, _ := procGetPhysicalCursorPos.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		return 0, 0, false
	}
	info := cursorInfo{Size: uint32(unsafe.Sizeof(cursorInfo{}))}
	visible := true
	if r, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&info))); r != 0 {
		visible = info.Flags&1 != 0 // CURSOR_SHOWING
	}
	x, y := int(p.X)-d.Left, int(p.Y)-d.Top
	return x, y, visible && x >= 0 && y >= 0 && x < d.Width && y < d.Height
}
