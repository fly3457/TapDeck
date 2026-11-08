//go:build windows

package audio

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
)

// 视频模式把电脑声音转发到手机：固定 48 kHz、立体声、16 位小端 PCM，每 10 ms 一包。
const (
	LoopbackRate     = 48000
	LoopbackChannels = 2
	// LoopbackPacket 是一包的字节数：10 ms × 48000 × 2 声道 × 2 字节。
	LoopbackPacket = LoopbackRate / 100 * LoopbackChannels * 2
)

// ErrLoopbackVirtual 表示默认播放设备是 VB-CABLE：它播放的正是手机麦克风的声音，转发回手机会形成回声。
var ErrLoopbackVirtual = errors.New("电脑默认播放设备是虚拟声卡 CABLE Input，不转发电脑声音")

// Loopback 回环采集电脑默认播放设备正在输出的声音，按 LoopbackPacket 分包交给 emit，直到 ctx 结束。
// 默认播放设备变化（插拔耳机、切换输出）时自动换到新设备。回环采集取自系统混音、早于端点音量，
// 电脑静音或调低音量时手机仍能听到。没有声音播放时 Windows 不产生数据，emit 不会被调用。
func Loopback(ctx context.Context, emit func(pcm []byte)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return err
	}
	defer ole.CoUninitialize()
	for ctx.Err() == nil {
		id, name, err := defaultRender()
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToUpper(name), "CABLE INPUT") {
			return ErrLoopbackVirtual
		}
		if err = loopbackDevice(ctx, id, emit); err != nil {
			return err
		}
	}
	return nil
}

// defaultRender 返回默认播放设备（eRender、eConsole）的 ID 与名称。
func defaultRender() (string, string, error) {
	e, err := enumerator()
	if err != nil {
		return "", "", err
	}
	defer release(e)
	var d *com
	// IMMDeviceEnumerator::GetDefaultAudioEndpoint(eRender, eConsole)。
	if err = call(e, 4, 0, 0, uintptr(unsafe.Pointer(&d))); err != nil {
		return "", "", errors.New("没有可用的播放设备")
	}
	defer release(d)
	var p *uint16
	if err = call(d, 5, uintptr(unsafe.Pointer(&p))); err != nil {
		return "", "", err
	}
	id := stringFrom(p)
	freeTask.Call(uintptr(unsafe.Pointer(p)))
	name := id
	if ds, err := devices(0); err == nil {
		for _, dv := range ds {
			if dv.ID == id {
				name = dv.Name
			}
		}
	}
	return id, name, nil
}

// loopbackDevice 采集一个播放设备，默认设备换成别的设备时返回 nil，由 Loopback 重新选择。
func loopbackDevice(ctx context.Context, id string, emit func([]byte)) error {
	c, err := activate(id)
	if err != nil {
		return err
	}
	defer release(c)
	format := waveFormat{1, LoopbackChannels, LoopbackRate, LoopbackRate * LoopbackChannels * 2, LoopbackChannels * 2, 16, 0}
	// AUDCLNT_STREAMFLAGS_LOOPBACK | AUTOCONVERTPCM | SRC_DEFAULT_QUALITY：由系统转换成上面的格式；缓冲 100 ms。
	if err = call(c, 3, 0, 0x88020000, 1000000, 0, uintptr(unsafe.Pointer(&format)), 0); err != nil {
		return err
	}
	var capture *com
	if err = call(c, 14, uintptr(unsafe.Pointer(captureIID)), uintptr(unsafe.Pointer(&capture))); err != nil {
		return err
	}
	defer release(capture)
	if err = call(c, 10); err != nil {
		return err
	}
	defer call(c, 11)
	pending := make([]byte, 0, LoopbackPacket*4)
	lastCheck := time.Now()
	for ctx.Err() == nil {
		// 回环流不支持事件通知（旧系统），按 5 ms 轮询；每 2 秒检查默认设备是否已切换。
		if time.Since(lastCheck) > 2*time.Second {
			lastCheck = time.Now()
			if now, _, err := defaultRender(); err == nil && now != id {
				return nil
			}
		}
		var n uint32
		if err = call(capture, 5, uintptr(unsafe.Pointer(&n))); err != nil {
			return err
		}
		if n == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		var p *byte
		var flags uint32
		if err = call(capture, 3, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&flags)), 0, 0); err != nil {
			return err
		}
		size := int(n) * int(format.Block)
		if flags&2 != 0 { // AUDCLNT_BUFFERFLAGS_SILENT
			pending = append(pending, make([]byte, size)...)
		} else {
			pending = append(pending, unsafe.Slice(p, size)...)
		}
		if err = call(capture, 4, uintptr(n)); err != nil {
			return err
		}
		for len(pending) >= LoopbackPacket {
			emit(append([]byte(nil), pending[:LoopbackPacket]...))
			pending = append(pending[:0], pending[LoopbackPacket:]...)
		}
	}
	return nil
}
