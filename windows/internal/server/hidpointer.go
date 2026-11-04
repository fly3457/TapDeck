package server

import (
	"fmt"
	"sync"
	"time"

	"tapdeck/internal/hidkeyboard"
	"tapdeck/internal/video"
)

// PointerClicker 以硬件级输入在显示器坐标处点击（视频模式点哪到哪）。
type PointerClicker interface {
	Click(d video.Display, x, y int32, right bool) error
	// Move 只移动光标，保持当前按住的键（按住时即拖拽）。
	Move(d video.Display, x, y int32) error
	// Down / Up 按住与松开左键（视频模式双击后拖拽）。
	Down(d video.Display, x, y int32) error
	Up(d video.Display, x, y int32) error
	// Release 松开所有按住的键（会话断开）。
	Release()
}

// HIDPointer 通过 FakerInput 虚拟 HID 绝对坐标鼠标点击。Windows 把它当作真实鼠标：
// 没有“注入”标记、不受 UIPI 限制，输入法候选框等忽略 SendInput 点击的窗口也能点中。
// 实测 HID 绝对坐标映射到主显示器；(0,0) 被视为“无位置”而忽略，最小值取 1。
type HIDPointer struct {
	mu   sync.Mutex
	dev  *hidkeyboard.Device
	held byte
	x, y uint16
}

func NewHIDPointer() *HIDPointer { return &HIDPointer{} }

func (h *HIDPointer) Move(d video.Display, x, y int32) error {
	hx, hy, err := hidPosition(d, x, y)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.send(h.held, hx, hy)
}

func (h *HIDPointer) Down(d video.Display, x, y int32) error {
	hx, hy, err := hidPosition(d, x, y)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	// 与单击相同：先移入并悬停，再按下。
	if err = h.send(0, hx, hy); err != nil {
		return err
	}
	pointerPause(pointerHoverDelay)
	if err = h.send(0, hx, hy); err != nil {
		return err
	}
	if err = h.send(hidkeyboard.MouseLeft, hx, hy); err != nil {
		return err
	}
	h.held = hidkeyboard.MouseLeft
	return nil
}

func (h *HIDPointer) Up(d video.Display, x, y int32) error {
	hx, hy, err := hidPosition(d, x, y)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.held = 0
	return h.send(0, hx, hy)
}

func (h *HIDPointer) Release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.held != 0 && h.dev != nil {
		h.held = 0
		_ = h.send(0, h.x, h.y)
	}
}

// hidPosition 把主显示器像素换算为 HID 绝对坐标（1–32767）。
func hidPosition(d video.Display, x, y int32) (uint16, uint16, error) {
	if d.Left != 0 || d.Top != 0 || d.Width < 2 || d.Height < 2 {
		return 0, 0, fmt.Errorf("虚拟鼠标只能定位主显示器")
	}
	scale := func(v int32, size int) uint16 {
		n := (int64(v)*hidkeyboard.AbsMax + int64(size-1)/2) / int64(size-1)
		return uint16(min(max(n, 1), hidkeyboard.AbsMax))
	}
	return scale(x, d.Width), scale(y, d.Height), nil
}

// send 写一个 HID 鼠标报告；设备不可用或写入失败时关闭句柄，下次重新打开。调用方持有 h.mu。
func (h *HIDPointer) send(buttons byte, hx, hy uint16) error {
	if h.dev == nil {
		dev, err := hidkeyboard.Open()
		if err != nil {
			return err
		}
		h.dev = dev
	}
	if err := h.dev.Mouse(buttons, hx, hy); err != nil {
		h.dev.Close()
		h.dev = nil
		h.held = 0
		return err
	}
	h.x, h.y = hx, hy
	return nil
}

func (h *HIDPointer) Click(d video.Display, x, y int32, right bool) error {
	hx, hy, err := hidPosition(d, x, y)
	if err != nil {
		return err
	}
	button := hidkeyboard.MouseLeft
	if right {
		button = hidkeyboard.MouseRight
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	// 与 SendInput 路径相同的节奏：先移入并悬停，再按下、按住、抬起。
	for _, s := range []struct {
		buttons byte
		pause   time.Duration
	}{{0, pointerHoverDelay}, {0, 0}, {button, pointerPressDuration}, {0, 0}} {
		if err = h.send(s.buttons, hx, hy); err != nil {
			return err
		}
		if s.pause > 0 {
			pointerPause(s.pause)
		}
	}
	return nil
}

func (h *HIDPointer) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.dev != nil {
		h.dev.Close()
		h.dev = nil
	}
}
