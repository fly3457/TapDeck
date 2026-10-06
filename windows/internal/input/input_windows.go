//go:build windows

package input

import (
	"fmt"
	"golang.org/x/sys/windows"
	"strconv"
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
type Controller struct {
	mu      sync.Mutex
	keys    map[uint16]int
	buttons map[string]bool
	inject  func(...nativeInput) error
}

func New() *Controller {
	return &Controller{keys: map[uint16]int{}, buttons: map[string]bool{}, inject: send}
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
	// wVk occupies the low 16 bits of DX, wScan the high 16 bits.
	// Side-specific modifiers use physical scan codes, independent of layout.
	scans := map[uint16]uint16{0xA0: 0x2A, 0xA1: 0x36, 0xA2: 0x1D, 0xA3: 0x1D, 0xA4: 0x38, 0xA5: 0x38}
	if scan, ok := scans[k]; ok {
		flags |= 8
		if k == 0xA3 || k == 0xA5 {
			flags |= 1
		}
		return nativeInput{Kind: 1, DX: int32(scan) << 16, DY: int32(flags)}
	}
	if k == 0x25 || k == 0x26 || k == 0x27 || k == 0x28 || k == 0x21 || k == 0x22 || k == 0x23 || k == 0x24 || k == 0x2D || k == 0x2E || k == 0x5B || k == 0x6F {
		flags |= 1
	}
	return nativeInput{Kind: 1, DX: int32(k), DY: int32(flags)}
}
func ParseChord(chord string) ([]uint16, error) {
	if strings.TrimSpace(chord) == "" {
		return nil, nil
	}
	result := []uint16{}
	seen := map[uint16]bool{}
	parts := strings.Split(strings.ToUpper(strings.ReplaceAll(chord, " ", "")), "+")
	names := map[string]uint16{"CTRL": 0xA2, "CONTROL": 0xA2, "SHIFT": 0xA0, "ALT": 0xA4,
		"LEFTCTRL": 0xA2, "LCTRL": 0xA2, "RIGHTCTRL": 0xA3, "RCTRL": 0xA3,
		"LEFTSHIFT": 0xA0, "LSHIFT": 0xA0, "RIGHTSHIFT": 0xA1, "RSHIFT": 0xA1,
		"LEFTALT": 0xA4, "LALT": 0xA4, "RIGHTALT": 0xA5, "RALT": 0xA5,
		"WIN": 0x5B, "ENTER": 0x0D, "RETURN": 0x0D, "ESC": 0x1B, "ESCAPE": 0x1B, "SPACE": 0x20, "TAB": 0x09, "BACKSPACE": 0x08, "DELETE": 0x2E, "INSERT": 0x2D, "HOME": 0x24, "END": 0x23, "PAGEUP": 0x21, "PAGEDOWN": 0x22, "LEFT": 0x25, "UP": 0x26, "RIGHT": 0x27, "DOWN": 0x28, "PLUS": 0xBB, "MINUS": 0xBD, "COMMA": 0xBC, "PERIOD": 0xBE}
	for _, p := range parts {
		k, ok := names[p]
		if !ok && len(p) == 1 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= '0' && p[0] <= '9')) {
			k = uint16(p[0])
			ok = true
		}
		if !ok && strings.HasPrefix(p, "F") {
			n, e := strconv.Atoi(p[1:])
			if e == nil && n >= 1 && n <= 24 {
				k = uint16(0x70 + n - 1)
				ok = true
			}
		}
		if !ok || seen[k] {
			return nil, fmt.Errorf("无效按键 %q", p)
		}
		seen[k] = true
		result = append(result, k)
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
	for _, k := range keys {
		if c.keys[k] == 0 {
			items = append(items, keyInput(k, false))
		}
	}
	for i := len(keys) - 1; i >= 0; i-- {
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
	return nil
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
	if down {
		for _, k := range keys {
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
	return nil
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
