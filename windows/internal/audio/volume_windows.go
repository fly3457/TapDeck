//go:build windows

package audio

import (
	"fmt"
	"runtime"
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

// VolumeControl resolves the current Windows default console playback endpoint
// for each operation. Enumeration order and TapDeck's CABLE output are unrelated
// to the speaker/headphone volume controlled by the system volume keys.
type VolumeControl struct {
	mu       sync.Mutex
	closed   bool
	endpoint func(func(*com) error) error
}

// NewVolumeControl activates IAudioEndpointVolume on the default render
// endpoint. A nil result means volume keys cannot be served on this machine.
func NewVolumeControl() (*VolumeControl, error) {
	v := &VolumeControl{endpoint: withDefaultVolume}
	if _, _, err := v.StepInfo(); err != nil {
		return nil, err
	}
	return v, nil
}

// COM creation, use and release stay on one initialized MTA thread. In
// particular, no interface is cached across Go goroutines or device switches.
func withDefaultVolume(action func(*com) error) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
			if code, ok := err.(*ole.OleError); !ok || code.Code() != 1 { // S_FALSE is also successful.
				done <- fmt.Errorf("初始化音量控制: %w", err)
				return
			}
		}
		defer ole.CoUninitialize()
		done <- onDefaultVolume(action)
	}()
	return <-done
}

func onDefaultVolume(action func(*com) error) error {
	e, err := enumerator()
	if err != nil {
		return fmt.Errorf("创建音频枚举器: %w", err)
	}
	defer release(e)
	vol, err := defaultVolume(e)
	if err != nil {
		return err
	}
	defer release(vol)
	return action(vol)
}

func defaultVolume(e *com) (*com, error) {
	var dev *com
	// IMMDeviceEnumerator::GetDefaultAudioEndpoint(eRender, eConsole).
	if err := call(e, 4, 0, 0, uintptr(unsafe.Pointer(&dev))); err != nil {
		return nil, fmt.Errorf("取系统默认播放设备失败: %w", err)
	}
	defer release(dev)
	var vol *com
	if err := call(dev, 3, uintptr(unsafe.Pointer(volumeIID)), 1, 0, uintptr(unsafe.Pointer(&vol))); err != nil {
		return nil, fmt.Errorf("Activate IAudioEndpointVolume: %w", err)
	}
	return vol, nil
}

func (v *VolumeControl) apply(action func(*com) error) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return fmt.Errorf("音量控制不可用")
	}
	return v.endpoint(action)
}

// Step raises or lowers the master volume by one system step.
func (v *VolumeControl) Step(up bool) error {
	slot := slotStepDown
	if up {
		slot = slotStepUp
	}
	return v.apply(func(vol *com) error { return call(vol, slot, 0) })
}

// ToggleMute flips the master mute state.
func (v *VolumeControl) ToggleMute() error {
	return v.apply(func(vol *com) error {
		var muted int32
		if err := call(vol, slotGetMute, uintptr(unsafe.Pointer(&muted))); err != nil {
			return err
		}
		next := int32(1)
		if muted != 0 {
			next = 0
		}
		return call(vol, slotSetMute, uintptr(next), 0)
	})
}

// Close stops accepting volume operations; every operation releases its own COM interface.
func (v *VolumeControl) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.closed = true
}

// Mute reports the current mute state; used by diagnostics and tests.
func (v *VolumeControl) Mute() (bool, error) {
	var muted int32
	err := v.apply(func(vol *com) error { return call(vol, slotGetMute, uintptr(unsafe.Pointer(&muted))) })
	return muted != 0, err
}

// Level reports the current master volume scalar.
func (v *VolumeControl) Level() (float32, error) {
	var level float32
	err := v.apply(func(vol *com) error { return call(vol, slotGetVolumeScalar, uintptr(unsafe.Pointer(&level))) })
	return level, err
}

// StepInfo reports the current step index and the total number of steps.
func (v *VolumeControl) StepInfo() (uint32, uint32, error) {
	var step, count uint32
	err := v.apply(func(vol *com) error {
		return call(vol, slotGetStepInfo, uintptr(unsafe.Pointer(&step)), uintptr(unsafe.Pointer(&count)))
	})
	return step, count, err
}
