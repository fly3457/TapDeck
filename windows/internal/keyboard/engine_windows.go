//go:build windows

package keyboard

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"tapdeck/internal/audio"
	"tapdeck/internal/hidkeyboard"
	"tapdeck/internal/input"
)

// Engine owns all keyboard state in the child process. Mouse injection never
// shares this mutex or waits for a keyboard pulse.
type Engine struct {
	mu         sync.Mutex
	mode       string
	hid        *hidkeyboard.Device
	hidKeys    map[uint16]int
	owners     map[uint16]keyOwner
	soft       softwareKeyboard
	holds      map[string][]string
	writeHID   func(map[uint16]int) error
	voices     map[string]voice
	fault      string
	observe    func()
	recoverHID func()
	// volumeStatus describes the Core Audio endpoint used for the volume keys.
	volumeStatus string
}
type keyOwner struct {
	Count   int
	Backend string
}
type softwareKeyboard interface {
	HoldKeys([]uint16, bool) error
	ReleaseAll()
}
type voice struct {
	mode, start, stop string
	started           bool
}
type Status struct {
	Configured string `json:"configured"`
	Actual     string `json:"actual"`
	Driver     string `json:"driver"`
	API        uint32 `json:"api"`
	Ready      bool   `json:"ready"`
	Error      string `json:"error,omitempty"`
	WorkerPID  int    `json:"worker_pid,omitempty"`
	Busy       bool   `json:"busy,omitempty"`
	// Volume reports the audio endpoint used for the volume keys, which cannot
	// be served by key injection.
	Volume string `json:"volume,omitempty"`
}

func NewEngine(mode string) *Engine {
	e := &Engine{mode: normalize(mode), hidKeys: map[uint16]int{}, owners: map[uint16]keyOwner{}, soft: input.New(), holds: map[string][]string{}, voices: map[string]voice{}}
	// Volume keys never go through key injection: Windows ignores injected
	// volume/media keys, so they are served by Core Audio instead.
	if v, err := audio.NewVolumeControl(); err == nil {
		if setter, ok := e.soft.(interface{ SetVolume(input.Volume) }); ok {
			setter.SetVolume(v)
		}
		if _, count, err := v.StepInfo(); err == nil {
			e.volumeStatus = fmt.Sprintf("Core Audio 默认播放设备，%d 级", count)
		} else {
			e.volumeStatus = "Core Audio 默认播放设备"
		}
	} else {
		e.volumeStatus = "不可用：" + err.Error()
	}
	e.reopen()
	e.recoverHID = func() {
		if d, er := hidkeyboard.Open(); er == nil {
			_ = d.Update(nil)
			d.Close()
		}
	}
	return e
}
func normalize(s string) string {
	if s == "" {
		return "auto"
	}
	return s
}
func (e *Engine) reopen() {
	if e.hid != nil {
		e.hid.Close()
		e.hid = nil
	}
	e.writeHID = nil
	if d, err := hidkeyboard.Open(); err == nil {
		e.hid = d
		e.writeHID = d.Update
	}
}
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := Status{Configured: e.mode, Actual: "sendinput", Ready: true, Volume: e.volumeStatus}
	if e.writeHID != nil {
		s.Driver = "FakerInput 0.1.1"
		s.API = 1
		if e.mode != "sendinput" {
			s.Actual = "hid"
		}
	}
	if e.mode == "hid" && e.writeHID == nil {
		s.Ready = false
		s.Error = "虚拟键盘不可用，请安装或修复驱动"
	}
	if e.mode == "auto" && e.writeHID == nil {
		s.Error = "未检测到虚拟键盘；当前使用 SendInput，豆包可能忽略软件按键"
	}
	if e.fault != "" {
		s.Ready = false
		s.Error = e.fault
	}
	return s
}
func Validate(chord, mode string) error {
	ks, err := input.ParseChord(chord)
	if err != nil {
		return err
	}
	if mode == "hid" {
		for _, k := range ks {
			if _, _, ok := hidkeyboard.Usage(k); !ok {
				return fmt.Errorf("虚拟键盘不支持 %s；请使用自动或软件发送", chord)
			}
		}
	}
	return nil
}
func (e *Engine) Configure(mode string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.voices) > 0 || len(e.holds) > 0 {
		return fmt.Errorf("录音或按键持有期间不能切换键盘发送方式")
	}
	mode = normalize(mode)
	if mode != "auto" && mode != "hid" && mode != "sendinput" {
		return fmt.Errorf("无效键盘发送方式")
	}
	e.mode = mode
	e.fault = ""
	e.reopen()
	return nil
}
func (e *Engine) route(ks []uint16) (string, error) {
	if e.fault != "" {
		return "", fmt.Errorf("%s", e.fault)
	}
	if e.mode == "sendinput" {
		return "sendinput", nil
	}
	supported := true
	for _, k := range ks {
		if _, _, ok := hidkeyboard.Usage(k); !ok {
			supported = false
		}
	}
	if supported && e.writeHID != nil {
		return "hid", nil
	}
	if e.mode == "auto" {
		return "sendinput", nil
	}
	return "", fmt.Errorf("HID 不可用或不支持此组合键")
}

// press returns the chosen route, pinned through release even if the device is
// lost. No failed HID chord is replayed using SendInput.
func (e *Engine) press(chord string) (string, error) {
	ks, err := input.ParseChord(chord)
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	route, err := e.route(ks)
	if err != nil {
		return "", err
	}
	nextOwners := map[uint16]keyOwner{}
	for k, v := range e.owners {
		nextOwners[k] = v
	}
	var softDown []uint16
	for _, k := range ks {
		v := nextOwners[k]
		if v.Count == 0 {
			v.Backend = route
			if route == "sendinput" {
				softDown = append(softDown, k)
			}
		}
		v.Count++
		nextOwners[k] = v
	}
	next := hidState(nextOwners)
	if _, err = hidkeyboard.Report(next); err != nil {
		return "", err
	}
	if !sameKeys(next, e.hidKeys) && e.writeHID != nil {
		if err = e.writeHID(next); err != nil {
			e.lostHID(err)
			return "", err
		}
	}
	if err = e.soft.HoldKeys(softDown, true); err != nil {
		if e.writeHID != nil {
			_ = e.writeHID(e.hidKeys)
		}
		return "", err
	}
	e.hidKeys = next
	e.owners = nextOwners
	return route, nil
}
func (e *Engine) release(chord, route string) error {
	ks, err := input.ParseChord(chord)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	var softUp []uint16
	for _, k := range ks {
		v := e.owners[k]
		if v.Count <= 0 {
			continue
		}
		v.Count--
		if v.Count == 0 {
			delete(e.owners, k)
			if v.Backend == "sendinput" {
				softUp = append(softUp, k)
			}
		} else {
			e.owners[k] = v
		}
	}
	next := hidState(e.owners)
	if !sameKeys(next, e.hidKeys) {
		if e.writeHID == nil {
			err = fmt.Errorf("虚拟键盘已经断开")
		} else if err = e.writeHID(next); err != nil {
			e.lostHID(err)
		}
	}
	if err == nil {
		e.hidKeys = next
	}
	if er := e.soft.HoldKeys(softUp, false); err == nil {
		err = er
	}
	return err
}
func hidState(owners map[uint16]keyOwner) map[uint16]int {
	out := map[uint16]int{}
	for k, v := range owners {
		if v.Count > 0 && v.Backend == "hid" {
			out[k] = v.Count
		}
	}
	return out
}
func sameKeys(a, b map[uint16]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, n := range a {
		if n > 0 && b[k] <= 0 {
			return false
		}
	}
	return true
}
func (e *Engine) lostHID(err error) {
	_ = e.writeHID(nil)
	if e.hid != nil {
		e.hid.Close()
		e.hid = nil
	}
	e.writeHID = nil
	e.fault = err.Error()
	e.hidKeys = map[uint16]int{}
	e.soft.ReleaseAll()
	e.owners = map[uint16]keyOwner{}
	e.holds = map[string][]string{}
	if e.recoverHID != nil {
		e.recoverHID()
	}
}
func (e *Engine) softwareKeys() []uint16 {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []uint16
	for k, v := range e.owners {
		if v.Count > 0 && v.Backend == "sendinput" {
			out = append(out, k)
		}
	}
	return out
}
func copyKeys(in map[uint16]int) map[uint16]int {
	out := map[uint16]int{}
	for k, n := range in {
		out[k] = n
	}
	return out
}
func (e *Engine) Hold(chord string, down bool) error {
	if chord == "" {
		return nil
	}
	ks, err := input.ParseChord(chord)
	if err != nil {
		return err
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	parts := []string{}
	for _, k := range ks {
		parts = append(parts, fmt.Sprintf("%X", k))
	}
	id := strings.Join(parts, "+")
	if down {
		route, err := e.press(chord)
		if err != nil {
			return err
		}
		e.mu.Lock()
		e.holds[id] = append(e.holds[id], route)
		e.mu.Unlock()
		return nil
	}
	e.mu.Lock()
	routes := e.holds[id]
	if len(routes) == 0 {
		e.mu.Unlock()
		return nil
	}
	route := routes[len(routes)-1]
	if len(routes) == 1 {
		delete(e.holds, id)
	} else {
		e.holds[id] = routes[:len(routes)-1]
	}
	e.mu.Unlock()
	return e.release(chord, route)
}
func (e *Engine) ClearKeys() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.writeHID != nil {
		_ = e.writeHID(nil)
	}
	e.hidKeys = map[uint16]int{}
	e.owners = map[uint16]keyOwner{}
	e.holds = map[string][]string{}
	e.soft.ReleaseAll()
}

// PressKey presses a chord and keeps it held until ReleaseKey is called, which
// is what the phone's full keyboard needs for held keys, modifiers and repeats.
func (e *Engine) PressKey(chord string) error { return e.Hold(chord, true) }

// ReleaseKey releases a chord previously pressed by PressKey.
func (e *Engine) ReleaseKey(chord string) error { return e.Hold(chord, false) }
func (e *Engine) Close() {
	e.ClearKeys()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.hid != nil {
		e.hid.Close()
		e.hid = nil
	}
}
