//go:build windows

package audio

import (
	"fmt"
	"sync"
	"unsafe"

	ole "github.com/go-ole/go-ole"
)

// 音量键不能靠注入按键实现：本机实测 SendInput 的六种编码（虚拟键码、扩展虚拟
// 键码、扫描码、扩展扫描码、两者组合）都不会让 Windows 改变音量，系统会忽略
// 注入的音量 / 媒体键。因此音量键改为直接调用 Core Audio 的
// IAudioEndpointVolume —— 与系统音量面板使用同一套接口。
var volumeIID = guid("{5CDF2C82-841E-4546-9722-0CF74078229A}")

// IAudioEndpointVolume 的 vtable 槽位（IUnknown 占 0–2，接口方法自 3 起，
// 本机实测确认）：9 GetMasterVolumeLevelScalar，14 SetMute，15 GetMute，
// 16 GetVolumeStepInfo，17 VolumeStepUp，18 VolumeStepDown。
//
// 音量增减只用 VolumeStepUp / VolumeStepDown：它们仅接收指针参数。而
// SetMasterVolumeLevelScalar 需要按 Windows x64 ABI 在 XMM1 传入 float，
// Go 的 syscall 无法传浮点参数（实测返回 E_INVALIDARG），因此不使用它。
const (
	slotGetVolumeScalar = 9
	slotSetMute         = 14
	slotGetMute         = 15
	slotGetStepInfo     = 16
	slotStepUp          = 17
	slotStepDown        = 18
)

// VolumeControl changes the volume and mute state of the default render
// endpoint (the first active render device, which is the one Windows uses).
type VolumeControl struct {
	mu    sync.Mutex
	ready bool
	vol   *com
}

// NewVolumeControl activates IAudioEndpointVolume on the default render
// endpoint. A nil result means volume keys cannot be served on this machine.
func NewVolumeControl() (*VolumeControl, error) {
	// S_FALSE 只表示本线程已经初始化过 COM，go-ole 会把它当成错误返回；
	// 这种情况下接口依然可用，所以不在此处直接失败。
	_ = ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	e, err := enumerator()
	if err != nil {
		return nil, fmt.Errorf("创建音频枚举器: %w", err)
	}
	defer release(e)
	var coll *com
	if err := call(e, 3, 0, 1, uintptr(unsafe.Pointer(&coll))); err != nil {
		return nil, fmt.Errorf("EnumAudioEndpoints: %w", err)
	}
	defer release(coll)
	var n uint32
	if err := call(coll, 3, uintptr(unsafe.Pointer(&n))); err != nil {
		return nil, fmt.Errorf("GetCount: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("没有可用的播放设备")
	}
	var dev *com
	if err := call(coll, 4, 0, uintptr(unsafe.Pointer(&dev))); err != nil {
		return nil, fmt.Errorf("取默认播放设备失败: %w", err)
	}
	defer release(dev)
	var vol *com
	if err := call(dev, 3, uintptr(unsafe.Pointer(volumeIID)), 1, 0, uintptr(unsafe.Pointer(&vol))); err != nil {
		return nil, fmt.Errorf("Activate IAudioEndpointVolume: %w", err)
	}
	return &VolumeControl{ready: true, vol: vol}, nil
}

// Step raises or lowers the master volume by one system step.
func (v *VolumeControl) Step(up bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.ready {
		return fmt.Errorf("音量控制不可用")
	}
	slot := slotStepDown
	if up {
		slot = slotStepUp
	}
	return call(v.vol, slot, 0)
}

// ToggleMute flips the master mute state.
func (v *VolumeControl) ToggleMute() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.ready {
		return fmt.Errorf("音量控制不可用")
	}
	var muted int32
	if err := call(v.vol, slotGetMute, uintptr(unsafe.Pointer(&muted))); err != nil {
		return err
	}
	next := int32(1)
	if muted != 0 {
		next = 0
	}
	return call(v.vol, slotSetMute, uintptr(next), 0)
}

// Close releases the COM interface.
func (v *VolumeControl) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.vol != nil {
		release(v.vol)
		v.vol = nil
	}
	v.ready = false
}

// Mute reports the current mute state; used by diagnostics and tests.
func (v *VolumeControl) Mute() (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.ready {
		return false, fmt.Errorf("音量控制不可用")
	}
	var muted int32
	if err := call(v.vol, slotGetMute, uintptr(unsafe.Pointer(&muted))); err != nil {
		return false, err
	}
	return muted != 0, nil
}

// Level reports the current master volume scalar.
func (v *VolumeControl) Level() (float32, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.ready {
		return 0, fmt.Errorf("音量控制不可用")
	}
	var level float32
	if err := call(v.vol, slotGetVolumeScalar, uintptr(unsafe.Pointer(&level))); err != nil {
		return 0, err
	}
	return level, nil
}

// StepInfo reports the current step index and the total number of steps.
func (v *VolumeControl) StepInfo() (uint32, uint32, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.ready {
		return 0, 0, fmt.Errorf("音量控制不可用")
	}
	var step, count uint32
	if err := call(v.vol, slotGetStepInfo, uintptr(unsafe.Pointer(&step)), uintptr(unsafe.Pointer(&count))); err != nil {
		return 0, 0, err
	}
	return step, count, nil
}
