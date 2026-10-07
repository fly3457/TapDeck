//go:build windows

// Package hidkeyboard implements the unmodified FakerInput 0.1.1 HID API.
// Wire layouts follow the upstream MIT-licensed fakerinputcommon.h.
package hidkeyboard

import (
	"encoding/binary"
	"fmt"
	"sort"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	hid   = windows.NewLazySystemDLL("hid.dll")
	setup = windows.NewLazySystemDLL("setupapi.dll")
)

type interfaceData struct {
	Size     uint32
	GUID     windows.GUID
	Flags    uint32
	Reserved uintptr
}
type attributes struct {
	Size                     uint32
	Vendor, Product, Version uint16
}
type caps struct {
	Usage, UsagePage, InputLength, OutputLength, FeatureLength uint16
	Reserved                                                   [17]uint16
	LinkCollections, InputButtons, InputValues, InputIndices   uint16
	OutputButtons, OutputValues, OutputIndices                 uint16
	FeatureButtons, FeatureValues, FeatureIndices              uint16
}
type Device struct {
	control, method windows.Handle
	Path            string
	APIVersion      uint32
	DeviceVersion   uint16
}

func Open() (*Device, error) {
	var guid windows.GUID
	hid.NewProc("HidD_GetHidGuid").Call(uintptr(unsafe.Pointer(&guid)))
	h, _, err := setup.NewProc("SetupDiGetClassDevsW").Call(uintptr(unsafe.Pointer(&guid)), 0, 0, 0x12)
	if h == uintptr(windows.InvalidHandle) {
		return nil, err
	}
	defer setup.NewProc("SetupDiDestroyDeviceInfoList").Call(h)
	d := &Device{control: windows.InvalidHandle, method: windows.InvalidHandle}
	for i := uintptr(0); ; i++ {
		in := interfaceData{Size: uint32(unsafe.Sizeof(interfaceData{}))}
		ok, _, _ := setup.NewProc("SetupDiEnumDeviceInterfaces").Call(h, 0, uintptr(unsafe.Pointer(&guid)), i, uintptr(unsafe.Pointer(&in)))
		if ok == 0 {
			break
		}
		var needed uint32
		setup.NewProc("SetupDiGetDeviceInterfaceDetailW").Call(h, uintptr(unsafe.Pointer(&in)), 0, 0, uintptr(unsafe.Pointer(&needed)), 0)
		if needed < 8 || needed > 65536 {
			continue
		}
		buf := make([]byte, needed)
		binary.LittleEndian.PutUint32(buf, 8) // sizeof(SP_DEVICE_INTERFACE_DETAIL_DATA_W), x64
		ok, _, _ = setup.NewProc("SetupDiGetDeviceInterfaceDetailW").Call(h, uintptr(unsafe.Pointer(&in)), uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)), 0)
		if ok == 0 {
			continue
		}
		path := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(&buf[4])))
		p, _ := windows.UTF16PtrFromString(path)
		file, e := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if e != nil {
			continue
		}
		a := attributes{Size: uint32(unsafe.Sizeof(attributes{}))}
		ok, _, _ = hid.NewProc("HidD_GetAttributes").Call(uintptr(file), uintptr(unsafe.Pointer(&a)))
		if ok == 0 || a.Vendor != 0xFE0F || a.Product != 0x00FF {
			windows.CloseHandle(file)
			continue
		}
		var prep uintptr
		ok, _, _ = hid.NewProc("HidD_GetPreparsedData").Call(uintptr(file), uintptr(unsafe.Pointer(&prep)))
		if ok == 0 {
			windows.CloseHandle(file)
			continue
		}
		var c caps
		status, _, _ := hid.NewProc("HidP_GetCaps").Call(prep, uintptr(unsafe.Pointer(&c)))
		hid.NewProc("HidD_FreePreparsedData").Call(prep)
		if status != 0x00110000 || c.UsagePage != 0xff00 || c.OutputLength != 65 {
			windows.CloseHandle(file)
			continue
		}
		if c.Usage == 1 && d.control == windows.InvalidHandle {
			d.control, d.Path, d.DeviceVersion = file, path, a.Version
		} else if c.Usage == 2 && d.method == windows.InvalidHandle {
			d.method = file
		} else {
			windows.CloseHandle(file)
		}
	}
	if d.control == windows.InvalidHandle || d.method == windows.InvalidHandle {
		d.Close()
		return nil, fmt.Errorf("未找到 FakerInput 0.1.1 控制设备")
	}
	var version [65]byte
	version[0] = 0x41
	binary.LittleEndian.PutUint32(version[4:], 1) // native UINT32 alignment, not a packed report
	if e := write(d.method, version[:]); e != nil {
		d.Close()
		return nil, e
	}
	version = [65]byte{0x42}
	ok, _, e := hid.NewProc("HidD_GetFeature").Call(uintptr(d.method), uintptr(unsafe.Pointer(&version[0])), 65)
	if ok == 0 {
		d.Close()
		return nil, fmt.Errorf("FakerInput API 查询失败: %v", e)
	}
	d.APIVersion = binary.LittleEndian.Uint32(version[4:])
	if d.APIVersion != 1 {
		d.Close()
		return nil, fmt.Errorf("不支持 FakerInput API %d", d.APIVersion)
	}
	return d, nil
}
func write(h windows.Handle, data []byte) error {
	var n uint32
	if e := windows.WriteFile(h, data, &n, nil); e != nil {
		return fmt.Errorf("FakerInput 写入失败: %w", e)
	}
	if int(n) != len(data) {
		return fmt.Errorf("FakerInput 写入不完整: %d", n)
	}
	return nil
}
func (d *Device) Close() {
	if d.control != windows.InvalidHandle {
		windows.CloseHandle(d.control)
		d.control = windows.InvalidHandle
	}
	if d.method != windows.InvalidHandle {
		windows.CloseHandle(d.method)
		d.method = windows.InvalidHandle
	}
}

// Usage maps the app's Windows VK codes to USB keyboard usages supported by
// the driver's descriptor (0x00..0x65). F13..F24 are intentionally unsupported.
func Usage(vk uint16) (usage, modifier byte, ok bool) {
	mods := map[uint16]byte{0xA2: 1, 0xA0: 2, 0xA4: 4, 0x5B: 8, 0xA3: 16, 0xA1: 32, 0xA5: 64, 0x5C: 128}
	if m, found := mods[vk]; found {
		return 0, m, true
	}
	if vk >= 'A' && vk <= 'Z' {
		return byte(vk - 'A' + 4), 0, true
	}
	if vk >= '1' && vk <= '9' {
		return byte(vk - '1' + 0x1E), 0, true
	}
	if vk == '0' {
		return 0x27, 0, true
	}
	if vk >= 0x70 && vk <= 0x7B {
		return byte(vk - 0x70 + 0x3A), 0, true
	}
	keys := map[uint16]byte{0x0D: 0x28, 0x1B: 0x29, 0x08: 0x2A, 0x09: 0x2B, 0x20: 0x2C, 0xBD: 0x2D, 0xBB: 0x2E, 0xBC: 0x36, 0xBE: 0x37, 0x2D: 0x49, 0x24: 0x4A, 0x21: 0x4B, 0x2E: 0x4C, 0x23: 0x4D, 0x22: 0x4E, 0x27: 0x4F, 0x25: 0x50, 0x28: 0x51, 0x26: 0x52,
		// 标点键：全键盘的符号键要走虚拟键盘（豆包只认 HID），所以也要在这里认出来。
		0xDB: 0x2F, 0xDD: 0x30, 0xDC: 0x31, 0xBA: 0x33, 0xDE: 0x34, 0xC0: 0x35, 0xBF: 0x38}
	u, found := keys[vk]
	return u, 0, found
}
func Report(keys map[uint16]int) ([65]byte, error) {
	r := [65]byte{0x40, 9, 1}
	var codes []int
	for vk, n := range keys {
		if n <= 0 {
			continue
		}
		u, m, ok := Usage(vk)
		if !ok {
			return r, fmt.Errorf("HID 不支持按键 0x%X", vk)
		}
		r[3] |= m
		if u != 0 {
			codes = append(codes, int(u))
		}
	}
	if len(codes) > 6 {
		return r, fmt.Errorf("HID 最多同时按住六个普通键")
	}
	sort.Ints(codes)
	for i, u := range codes {
		r[5+i] = byte(u)
	}
	return r, nil
}
func (d *Device) Update(keys map[uint16]int) error {
	r, e := Report(keys)
	if e != nil {
		return e
	}
	return write(d.control, r[:])
}
