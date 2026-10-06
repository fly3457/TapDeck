//go:build windows

package input

import (
	"fmt"
	"golang.org/x/sys/windows"
	"strings"
	"sync"
	"unsafe"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var sendInput = user32.NewProc("SendInput")

// INPUT has an 8-byte aligned union and is 40 bytes on Windows amd64.
type nativeInput struct {
	Kind  uint32
	Pad   uint32
	DX    int32
	DY    int32
	Data  uint32
	Flags uint32
	Time  uint32
	Pad2  uint32
	Extra uintptr
}
// Volume 由 Core Audio 实现（见 internal/audio）。音量 / 媒体键无法通过注入
// 按键实现：本机实测 SendInput 的六种编码都不会让 Windows 改变音量。
type Volume interface {
	Step(up bool) error
	ToggleMute() error
}

type Controller struct {
	mu      sync.Mutex
	keys    map[uint16]int
	buttons map[string]bool
	inject  func(...nativeInput) error
	volume  Volume
	// volumeDone 记录已经执行过音量动作的按键，避免长按期间每一步都重复调整。
	volumeDone map[uint16]bool
}

func New() *Controller {
	return &Controller{keys: map[uint16]int{}, buttons: map[string]bool{}, inject: send, volumeDone: map[uint16]bool{}}
}

// SetVolume attaches the Core Audio volume controller used for the volume keys.
func (c *Controller) SetVolume(v Volume) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.volume = v
}

// volumeKey reports whether the key is served by Core Audio instead of injection.
func volumeKey(k uint16) bool { return k == 0xAD || k == 0xAE || k == 0xAF }

// applyVolume runs the volume action for a freshly pressed volume key.
func (c *Controller) applyVolume(k uint16) error {
	if c.volume == nil {
		return fmt.Errorf("音量控制不可用")
	}
	if c.volumeDone[k] {
		return nil
	}
	c.volumeDone[k] = true
	switch k {
	case 0xAD:
		return c.volume.ToggleMute()
	case 0xAE:
		return c.volume.Step(false)
	case 0xAF:
		return c.volume.Step(true)
	}
	return nil
}
func send(items ...nativeInput) error {
	if len(items) == 0 {
		return nil
	}
	n, _, err := sendInput.Call(uintptr(len(items)), uintptr(unsafe.Pointer(&items[0])), unsafe.Sizeof(items[0]))
	if int(n) != len(items) {
		return fmt.Errorf("SendInput 失败（可能目标窗口权限更高）: %v", err)
	}
	return nil
}
func keyInput(k uint16, up bool) nativeInput {
	flags := uint32(0)
	if up {
		flags = 2
	}
	// The table carries the physical scan code for every key that has one; scan
	// codes keep side-specific modifiers and keyboard keys independent of layout.
	// Volume and media keys are vkOnly because the shell's media-key handling
	// ignores scan-code-only injection (measured on this machine).
	def, known := keyByVK[k]
	if known && def.scan != 0 && !def.vkOnly {
		flags |= 8 // KEYEVENTF_SCANCODE
		if def.ext {
			flags |= 1 // KEYEVENTF_EXTENDEDKEY
		}
		return nativeInput{Kind: 1, DX: int32(def.scan) << 16, DY: int32(flags)}
	}
	if known && def.ext {
		flags |= 1
	}
	// wVk occupies the low 16 bits of DX, wScan the high 16 bits.
	return nativeInput{Kind: 1, DX: int32(k), DY: int32(flags)}
}
func ParseChord(chord string) ([]uint16, error) {
	if strings.TrimSpace(chord) == "" {
		return nil, nil
	}
	result := []uint16{}
	seen := map[uint16]bool{}
	parts := strings.Split(strings.ToUpper(strings.ReplaceAll(chord, " ", "")), "+")
	for _, p := range parts {
		k, ok := lookupKey(p)
		if !ok || seen[k.vk] {
			return nil, fmt.Errorf("无效按键 %q", p)
		}
		seen[k.vk] = true
		result = append(result, k.vk)
	}
	if len(result) > 5 {
		return nil, fmt.Errorf("组合键最多五个按键")
	}
	return result, nil
}
func (c *Controller) Chord(chord string) error {
	keys, e := ParseChord(chord)
	if e != nil {
		return e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	items := []nativeInput{}
	var volumeErr error
	for _, k := range keys {
		if volumeKey(k) {
			// 音量键交给 Core Audio，不注入按键。
			if e := c.applyVolume(k); e != nil && volumeErr == nil {
				volumeErr = e
			}
			continue
		}
		if c.keys[k] == 0 {
			items = append(items, keyInput(k, false))
		}
	}
	for i := len(keys) - 1; i >= 0; i-- {
		if k := keys[i]; volumeKey(k) {
			delete(c.volumeDone, k)
			continue
		}
		if c.keys[keys[i]] == 0 {
			items = append(items, keyInput(keys[i], true))
		}
	}
	if e := c.inject(items...); e != nil {
		// SendInput may accept only a prefix; release keys owned by this chord.
		cleanup := []nativeInput{}
		for i := len(keys) - 1; i >= 0; i-- {
			if c.keys[keys[i]] == 0 {
				cleanup = append(cleanup, keyInput(keys[i], true))
			}
		}
		_ = c.inject(cleanup...)
		return e
	}
	return volumeErr
}
func (c *Controller) Hold(chord string, down bool) error {
	keys, e := ParseChord(chord)
	if e != nil {
		return e
	}
	return c.HoldKeys(keys, down)
}
func (c *Controller) HoldKeys(keys []uint16, down bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := map[uint16]int{}
	for k, n := range c.keys {
		previous[k] = n
	}
	items := []nativeInput{}
	var volumeErr error
	if down {
		for _, k := range keys {
			if volumeKey(k) {
				// 音量键不注入按键：按住期间只执行一次音量动作。
				c.keys[k]++
				if e := c.applyVolume(k); e != nil && volumeErr == nil {
					volumeErr = e
				}
				continue
			}
			if c.keys[k] == 0 {
				items = append(items, keyInput(k, false))
			}
			c.keys[k]++
		}
	} else {
		for i := len(keys) - 1; i >= 0; i-- {
			k := keys[i]
			if c.keys[k] > 0 {
				c.keys[k]--
				if c.keys[k] == 0 {
					delete(c.volumeDone, k)
					if volumeKey(k) {
						continue
					}
					items = append(items, keyInput(k, true))
				}
			}
		}
	}
	if e := c.inject(items...); e != nil {
		cleanup := []nativeInput{}
		for i := len(keys) - 1; i >= 0; i-- {
			k := keys[i]
			if !down || previous[k] == 0 {
				cleanup = append(cleanup, keyInput(k, true))
			}
		}
		_ = c.inject(cleanup...)
		if down {
			c.keys = previous
		}
		return e
	}
	return volumeErr
}
func (c *Controller) Move(dx, dy, sx, sy int32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := []nativeInput{}
	if dx != 0 || dy != 0 {
		items = append(items, nativeInput{DX: dx, DY: dy, Flags: 1})
	}
	if sy != 0 {
		items = append(items, nativeInput{Data: uint32(sy), Flags: 0x800})
	}
	if sx != 0 {
		items = append(items, nativeInput{Data: uint32(sx), Flags: 0x1000})
	}
	return c.inject(items...)
}
func (c *Controller) Button(button string, down bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var f uint32
	switch button {
	case "left":
		f = 2
	case "right":
		f = 8
	case "middle":
		f = 32
	default:
		return fmt.Errorf("无效鼠标按钮")
	}
	if !down {
		f *= 2
	}
	if c.buttons[button] == down {
		return nil
	}
	if e := c.inject(nativeInput{Flags: f}); e != nil {
		return e
	}
	c.buttons[button] = down
	return nil
}
func (c *Controller) ReleaseAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := []nativeInput{}
	for k, n := range c.keys {
		if n > 0 {
			items = append(items, keyInput(k, true))
		}
	}
	for b, down := range c.buttons {
		if down {
			f := uint32(4)
			if b == "right" {
				f = 16
			}
			if b == "middle" {
				f = 64
			}
			items = append(items, nativeInput{Flags: f})
		}
	}
	_ = c.inject(items...)
	c.keys = map[uint16]int{}
	c.buttons = map[string]bool{}
	c.volumeDone = map[uint16]bool{}
}

// ReleaseOwnedKeys is reserved for recovery after the keyboard child crashes.
// The parent supplies only the software keys reported as owned by that child.
func ReleaseOwnedKeys(keys []uint16) {
	items := []nativeInput{}
	for _, k := range keys {
		items = append(items, keyInput(k, true))
	}
	_ = send(items...)
}
