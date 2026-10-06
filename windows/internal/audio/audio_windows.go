//go:build windows

package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
	"math"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type com struct{ VT *[20]uintptr }

func call(o *com, index int, args ...uintptr) error {
	params := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.VT[index], params...)
	if int32(r) < 0 {
		return fmt.Errorf("WASAPI HRESULT 0x%08x", uint32(r))
	}
	return nil
}
func release(o *com) {
	if o != nil {
		_ = call(o, 2)
	}
}
func guid(s string) *ole.GUID { return ole.NewGUID(s) }

var enumClass = guid("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
var enumIID = guid("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
var clientIID = guid("{1CB9AD4C-DBFA-4c32-B178-C2F568A703B2}")
var renderIID = guid("{F294ACFC-3146-4483-A7BF-ADDCA7C260E2}")
var captureIID = guid("{C8ADBD64-E71E-48A0-A4DE-185C395CD317}")
var createInstance = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
var freeTask = windows.NewLazySystemDLL("ole32.dll").NewProc("CoTaskMemFree")

type propertyKey struct {
	Fmt ole.GUID
	PID uint32
}

var friendlyKey = propertyKey{*guid("{A45C254E-DF1C-4EFD-8020-67D146A850E0}"), 14}

type variant struct {
	Type     uint16
	Reserved [3]uint16
	Value    unsafe.Pointer
	Extra    uintptr
}

func enumerator() (*com, error) {
	var e *com
	r, _, _ := createInstance.Call(uintptr(unsafe.Pointer(enumClass)), 0, 1, uintptr(unsafe.Pointer(enumIID)), uintptr(unsafe.Pointer(&e)))
	if int32(r) < 0 {
		return nil, fmt.Errorf("创建音频枚举器 0x%x", r)
	}
	return e, nil
}
func stringFrom(p *uint16) string {
	if p == nil {
		return ""
	}
	return windows.UTF16PtrToString(p)
}
func devices(flow uint32) ([]Device, error) {
	e, err := enumerator()
	if err != nil {
		return nil, err
	}
	defer release(e)
	var coll *com
	if err = call(e, 3, uintptr(flow), 1, uintptr(unsafe.Pointer(&coll))); err != nil {
		return nil, err
	}
	defer release(coll)
	var n uint32
	if err = call(coll, 3, uintptr(unsafe.Pointer(&n))); err != nil {
		return nil, err
	}
	ds := []Device{}
	for i := uint32(0); i < n; i++ {
		var d *com
		if call(coll, 4, uintptr(i), uintptr(unsafe.Pointer(&d))) != nil {
			continue
		}
		var p *uint16
		if call(d, 5, uintptr(unsafe.Pointer(&p))) != nil {
			release(d)
			continue
		}
		id := stringFrom(p)
		freeTask.Call(uintptr(unsafe.Pointer(p)))
		var store *com
		name := id
		if call(d, 4, 0, uintptr(unsafe.Pointer(&store))) == nil {
			var v variant
			if call(store, 5, uintptr(unsafe.Pointer(&friendlyKey)), uintptr(unsafe.Pointer(&v))) == nil {
				if v.Type == 31 {
					name = stringFrom((*uint16)(v.Value))
				}
				windows.NewLazySystemDLL("ole32.dll").NewProc("PropVariantClear").Call(uintptr(unsafe.Pointer(&v)))
			}
			release(store)
		}
		release(d)
		ds = append(ds, Device{id, name})
	}
	return ds, nil
}
func Devices() ([]Device, error) {
	// Walk owns an STA UI thread. Enumerate on a separate MTA thread so the
	// refresh button works after the settings window has initialized COM.
	type result struct {
		items []Device
		err   error
	}
	done := make(chan result, 1)
	go func() {
		items, err := enumerateDevices()
		done <- result{items, err}
	}()
	v := <-done
	return v.items, v.err
}
func enumerateDevices() ([]Device, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if e := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); e != nil {
		return nil, e
	}
	defer ole.CoUninitialize()
	return devices(0)
}

type waveFormat struct {
	Tag         uint16
	Channels    uint16
	Rate        uint32
	BytesPerSec uint32
	Block       uint16
	Bits        uint16
	Extra       uint16
}

func activate(id string) (*com, error) {
	e, err := enumerator()
	if err != nil {
		return nil, err
	}
	defer release(e)
	var d *com
	p, _ := windows.UTF16PtrFromString(id)
	if err = call(e, 5, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&d))); err != nil {
		return nil, err
	}
	defer release(d)
	var c *com
	if err = call(d, 3, uintptr(unsafe.Pointer(clientIID)), 23, 0, uintptr(unsafe.Pointer(&c))); err != nil {
		return nil, err
	}
	return c, nil
}

type Engine struct {
	mu          sync.Mutex
	device      string
	gain        float64
	ready       bool
	status      string
	level       float64
	recording   uint64
	frames      map[uint64][]int16
	next        uint64
	started     bool
	first       time.Time
	end         time.Time
	onDrain     func()
	received    uint64
	concealed   uint64
	lastSample  int16
	maxBuffered int
}

func New() *Engine {
	return &Engine{gain: 1, frames: map[uint64][]int16{}, status: "正在检查虚拟声卡"}
}
func (e *Engine) Configure(id string, gain float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.device = id
	e.gain = gain
}
func (e *Engine) Status() (string, bool, float64, uint64, uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status, e.ready, e.level, e.received, e.concealed
}
func (e *Engine) Begin(id uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		return errors.New(e.status)
	}
	if e.recording != 0 {
		return errors.New("上一轮传音尚未结束")
	}
	e.recording = id
	e.frames = map[uint64][]int16{}
	e.started = false
	e.first = time.Time{}
	e.end = time.Time{}
	e.next = 0
	e.onDrain = nil
	return nil
}
func (e *Engine) Push(id, pos uint64, pcm []byte) {
	if len(pcm) != 960 || pos%480 != 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if id != e.recording || id == 0 || (!e.end.IsZero() && time.Now().After(e.end)) || (e.started && pos+480 <= e.next) {
		return
	}
	if e.first.IsZero() {
		e.first = time.Now()
		e.next = pos
	}
	if pos > e.next+480*5 {
		e.next = pos - 480*2
		for k := range e.frames {
			if k < e.next {
				delete(e.frames, k)
			}
		}
	}
	if len(e.frames) >= 6 {
		return
	}
	if _, exists := e.frames[pos]; exists {
		return
	}
	samples := make([]int16, 480)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	e.frames[pos] = samples
	e.maxBuffered = max(e.maxBuffered, len(e.frames))
	e.received++
}
func (e *Engine) BufferStats() (int, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.frames), e.maxBuffered
}
func (e *Engine) End(id uint64, delay time.Duration, done func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.recording != id {
		if done != nil {
			go done()
		}
		return
	}
	e.end = time.Now().Add(60 * time.Millisecond)
	e.onDrain = func() { time.AfterFunc(delay, done) }
}
func (e *Engine) Abort() {
	e.mu.Lock()
	e.recording = 0
	e.frames = map[uint64][]int16{}
	e.onDrain = nil
	e.level = 0
	e.mu.Unlock()
}
func (e *Engine) render(samples []int16) {
	e.mu.Lock()
	defer e.mu.Unlock()
	clear(samples)
	if e.recording == 0 {
		return
	}
	if !e.end.IsZero() && time.Now().After(e.end) {
		done := e.onDrain
		e.onDrain = nil
		e.recording = 0
		e.frames = map[uint64][]int16{}
		e.level = 0
		if done != nil {
			go done()
		}
		return
	}
	if e.first.IsZero() || time.Since(e.first) < 20*time.Millisecond {
		return
	}
	e.started = true
	// Small sample slips hold the queue near its target despite independent
	// Android/Windows clocks, without allowing latency to grow unbounded.
	var newest uint64
	for position := range e.frames {
		if position+480 > newest {
			newest = position + 480
		}
	}
	if newest > e.next+1920 {
		oldFrame := e.next / 480 * 480
		e.next++
		if e.next%480 == 0 {
			delete(e.frames, oldFrame)
		}
	}
	duplicate := newest > e.next && newest < e.next+720 && e.end.IsZero()
	var power float64
	for i := range samples {
		if duplicate && i == len(samples)-1 {
			samples[i] = e.lastSample
			power += float64(e.lastSample) * float64(e.lastSample)
			continue
		}
		framePos := e.next / 480 * 480
		frame, ok := e.frames[framePos]
		if ok {
			v := float64(frame[e.next%480]) * e.gain
			v = math.Max(-32768, math.Min(32767, v))
			samples[i] = int16(v)
			e.lastSample = samples[i]
			power += v * v
		} else {
			if e.next%480 == 0 {
				e.concealed++
			}
		}
		e.next++
		if e.next%480 == 0 {
			delete(e.frames, framePos)
		}
	}
	e.level = math.Sqrt(power/float64(max(1, len(samples)))) / 32768
}
func (e *Engine) setStatus(status string, ready bool) {
	e.mu.Lock()
	e.status = status
	e.ready = ready
	e.mu.Unlock()
}
func (e *Engine) Run(ctx context.Context) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		e.setStatus(err.Error(), false)
		return
	}
	defer ole.CoUninitialize()
	for ctx.Err() == nil {
		ds, err := devices(0)
		e.mu.Lock()
		wanted := e.device
		e.mu.Unlock()
		id := ""
		name := ""
		for _, d := range ds {
			if d.ID == wanted || (wanted == "" && strings.Contains(strings.ToUpper(d.Name), "CABLE INPUT")) {
				id = d.ID
				name = d.Name
				break
			}
		}
		if err != nil {
			e.setStatus(err.Error(), false)
		} else if id == "" {
			e.setStatus("未找到 CABLE Input；请安装 VB-CABLE 或选择输出设备", false)
		} else {
			err = e.runDevice(ctx, id, name, wanted)
			if err != nil {
				e.setStatus(err.Error(), false)
				e.Abort()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
func (e *Engine) runDevice(ctx context.Context, id, name, wanted string) error {
	c, err := activate(id)
	if err != nil {
		return err
	}
	defer release(c)
	format := waveFormat{1, 1, 48000, 96000, 2, 16, 0}
	if err = call(c, 3, 0, 0x88040000, 200000, 0, uintptr(unsafe.Pointer(&format)), 0); err != nil {
		return err
	}
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(event)
	if err = call(c, 13, uintptr(event)); err != nil {
		return err
	}
	var render *com
	if err = call(c, 14, uintptr(unsafe.Pointer(renderIID)), uintptr(unsafe.Pointer(&render))); err != nil {
		return err
	}
	defer release(render)
	var size uint32
	if err = call(c, 4, uintptr(unsafe.Pointer(&size))); err != nil {
		return err
	}
	if err = call(c, 10); err != nil {
		return err
	}
	defer call(c, 11)
	e.setStatus(name, true)
	for ctx.Err() == nil {
		e.mu.Lock()
		changed := e.device != wanted
		e.mu.Unlock()
		if changed {
			e.setStatus("正在切换音频设备", false)
			return nil
		}
		result, err := windows.WaitForSingleObject(event, 100)
		if err != nil {
			return err
		}
		if result != windows.WAIT_OBJECT_0 {
			continue
		}
		var padding uint32
		if err = call(c, 6, uintptr(unsafe.Pointer(&padding))); err != nil {
			return err
		}
		available := size - padding
		if available == 0 {
			continue
		}
		var p *int16
		if err = call(render, 3, uintptr(available), uintptr(unsafe.Pointer(&p))); err != nil {
			return err
		}
		e.render(unsafe.Slice(p, int(available)))
		if err = call(render, 4, uintptr(available), 0); err != nil {
			return err
		}
	}
	e.setStatus("已停止", false)
	return nil
}

// CaptureProbe records only a named virtual endpoint for synthetic end-to-end checks.
func CaptureProbe(ctx context.Context, seconds int) ([]int16, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return nil, err
	}
	defer ole.CoUninitialize()
	ds, err := devices(1)
	if err != nil {
		return nil, err
	}
	id := ""
	for _, d := range ds {
		if strings.Contains(strings.ToUpper(d.Name), "CABLE OUTPUT") {
			id = d.ID
			break
		}
	}
	if id == "" {
		return nil, errors.New("未找到 CABLE Output")
	}
	c, err := activate(id)
	if err != nil {
		return nil, err
	}
	defer release(c)
	f := waveFormat{1, 1, 48000, 96000, 2, 16, 0}
	if err = call(c, 3, 0, 0x88000000, 200000, 0, uintptr(unsafe.Pointer(&f)), 0); err != nil {
		return nil, err
	}
	var capture *com
	if err = call(c, 14, uintptr(unsafe.Pointer(captureIID)), uintptr(unsafe.Pointer(&capture))); err != nil {
		return nil, err
	}
	defer release(capture)
	if err = call(c, 10); err != nil {
		return nil, err
	}
	defer call(c, 11)
	out := []int16{}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		var n uint32
		if err = call(capture, 5, uintptr(unsafe.Pointer(&n))); err != nil {
			return nil, err
		}
		if n == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		var p *int16
		var flags uint32
		if err = call(capture, 3, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&flags)), 0, 0); err != nil {
			return nil, err
		}
		if flags&2 != 0 {
			out = append(out, make([]int16, n)...)
		} else {
			out = append(out, unsafe.Slice(p, int(n))...)
		}
		if err = call(capture, 4, uintptr(n)); err != nil {
			return nil, err
		}
	}
	return out, nil
}
