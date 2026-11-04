//go:build windows

// Package winrt 以纯 Go 系统调用访问少量 Windows 运行时（WinRT）接口：HSTRING、
// 激活工厂、vtable 调用、异步操作轮询与 IVectorView 遍历。只覆盖通知监听与 Toast
// 所需的最小子集，不依赖 cgo 或生成的投影代码。
package winrt

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	combase                       = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize              = combase.NewProc("RoInitialize")
	procRoGetActivationFactory    = combase.NewProc("RoGetActivationFactory")
	procRoActivateInstance        = combase.NewProc("RoActivateInstance")
	procWindowsCreateString       = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString       = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuffer = combase.NewProc("WindowsGetStringRawBuffer")

	iidIAsyncInfo = windows.GUID{Data1: 0x00000036, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

// HRESULT 是失败的 WinRT/COM 调用结果。
type HRESULT uint32

func (h HRESULT) Error() string { return fmt.Sprintf("HRESULT 0x%08x", uint32(h)) }

// Check 把负 HRESULT 转为错误。
func Check(hr uintptr) error {
	if int32(hr) < 0 {
		return HRESULT(uint32(hr))
	}
	return nil
}

// Init 以多线程单元（MTA）初始化当前 OS 线程。调用方应先 runtime.LockOSThread；
// 线程已按其他模式初始化（RPC_E_CHANGED_MODE）时沿用现有模式。
func Init() error {
	hr, _, _ := procRoInitialize.Call(1) // RO_INIT_MULTITHREADED
	if uint32(hr) == 0x80010106 {        // RPC_E_CHANGED_MODE
		return nil
	}
	return Check(hr)
}

// String 是 WinRT 的 HSTRING 句柄，零值表示空字符串。
type String uintptr

// NewString 按 UTF-16 码元长度创建 HSTRING（emoji 等代理对也正确）。
func NewString(s string) (String, error) {
	if s == "" {
		return 0, nil
	}
	u := windows.StringToUTF16(s)
	var h String
	hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	return h, Check(hr)
}

// Value 读取 HSTRING 的内容，不释放句柄。
func (h String) Value() string {
	if h == 0 {
		return ""
	}
	var n uint32
	p, _, _ := procWindowsGetStringRawBuffer.Call(uintptr(h), uintptr(unsafe.Pointer(&n)))
	if p == 0 || n == 0 {
		return ""
	}
	// 经由变量地址转换，避免 uintptr → unsafe.Pointer 的直接转换。
	return windows.UTF16ToString(unsafe.Slice(*(**uint16)(unsafe.Pointer(&p)), n))
}

// Delete 释放 HSTRING。
func (h String) Delete() {
	if h != 0 {
		procWindowsDeleteString.Call(uintptr(h))
	}
}

// Take 读取并释放调用方拥有的 HSTRING（WinRT 属性返回值）。
func (h String) Take() string {
	v := h.Value()
	h.Delete()
	return v
}

// Object 是 IInspectable 派生接口指针；vtable 前 6 项为 IUnknown 与 IInspectable。
type Object struct{ vtbl *[64]uintptr }

// Call 调用第 index 个虚函数。uintptrescapes 让以 uintptr(unsafe.Pointer(&x)) 传入的
// 输出参数分配在堆上，避免调用期间 goroutine 栈搬迁后系统写入失效地址。
//
//go:uintptrescapes
func (o *Object) Call(index int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[index], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

// Release 释放接口引用；nil 安全。
func (o *Object) Release() {
	if o != nil {
		o.Call(2)
	}
}

// Query 按 IID 查询另一个接口。
func (o *Object) Query(iid *windows.GUID) (*Object, error) {
	var out *Object
	if err := Check(o.Call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))); err != nil {
		return nil, err
	}
	return out, nil
}

// Get 调用只有一个指针输出参数的方法（属性读取、无参工厂方法等）。
func (o *Object) Get(index int, args ...uintptr) (*Object, error) {
	var out *Object
	if err := Check(o.Call(index, append(args, uintptr(unsafe.Pointer(&out)))...)); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("WinRT 方法 %d 返回空对象", index)
	}
	return out, nil
}

// GetString 调用返回 HSTRING 的方法并取出内容。
func (o *Object) GetString(index int, args ...uintptr) (string, error) {
	var h String
	if err := Check(o.Call(index, append(args, uintptr(unsafe.Pointer(&h)))...)); err != nil {
		return "", err
	}
	return h.Take(), nil
}

// Factory 取运行时类的激活工厂或静态接口。
func Factory(class string, iid *windows.GUID) (*Object, error) {
	name, err := NewString(class)
	if err != nil {
		return nil, err
	}
	defer name.Delete()
	var out *Object
	if hr, _, _ := procRoGetActivationFactory.Call(uintptr(name), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); Check(hr) != nil {
		return nil, fmt.Errorf("%s: %w", class, Check(hr))
	}
	return out, nil
}

// Activate 以默认构造函数创建运行时类实例，返回 IInspectable。
func Activate(class string) (*Object, error) {
	name, err := NewString(class)
	if err != nil {
		return nil, err
	}
	defer name.Delete()
	var out *Object
	if hr, _, _ := procRoActivateInstance.Call(uintptr(name), uintptr(unsafe.Pointer(&out))); Check(hr) != nil {
		return nil, fmt.Errorf("%s: %w", class, Check(hr))
	}
	return out, nil
}

// Await 轮询 IAsyncOperation 直到完成（不实现完成回调委托）。结果为值类型时由
// 调用方自行调用 GetResults（vtable 8），接口结果用 Result。
func Await(op *Object, timeout time.Duration) error {
	info, err := op.Query(&iidIAsyncInfo)
	if err != nil {
		return err
	}
	defer info.Release()
	deadline := time.Now().Add(timeout)
	for wait := time.Millisecond; ; wait = min(wait*2, 50*time.Millisecond) {
		var status int32
		if err := Check(info.Call(7, uintptr(unsafe.Pointer(&status)))); err != nil { // get_Status
			return err
		}
		switch status {
		case 1: // Completed
			return nil
		case 2: // Canceled
			return fmt.Errorf("WinRT 异步操作已取消")
		case 3: // Error
			var code int32
			info.Call(8, uintptr(unsafe.Pointer(&code))) // get_ErrorCode
			return HRESULT(uint32(code))
		}
		if time.Now().After(deadline) {
			info.Call(9) // Cancel
			return fmt.Errorf("WinRT 异步操作超时")
		}
		time.Sleep(wait)
	}
}

// Result 等待 IAsyncOperation<T>（T 为接口）完成并取出结果。
func Result(op *Object, timeout time.Duration) (*Object, error) {
	if err := Await(op, timeout); err != nil {
		return nil, err
	}
	return op.Get(8) // GetResults
}

// Items 把 IVectorView<T>（T 为接口）展开为对象切片；调用方负责逐个 Release。
func Items(view *Object) ([]*Object, error) {
	var n uint32
	if err := Check(view.Call(7, uintptr(unsafe.Pointer(&n)))); err != nil { // get_Size
		return nil, err
	}
	out := make([]*Object, 0, n)
	for i := uint32(0); i < n; i++ {
		item, err := view.Get(6, uintptr(i)) // GetAt
		if err != nil {
			for _, v := range out {
				v.Release()
			}
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// GUID 解析 "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" 形式的接口标识；格式错误直接 panic，
// 只用于包级常量。
func GUID(s string) windows.GUID {
	g, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic(err)
	}
	return g
}
