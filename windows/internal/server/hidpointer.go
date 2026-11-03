package server

import (
	"fmt"
	"sync"

	"tapdeck/internal/hidkeyboard"
	"tapdeck/internal/video"
)

// PointerClicker 以硬件级输入在显示器坐标处点击（视频模式点哪到哪）。
type PointerClicker interface {
	Click(d video.Display, x, y int32, right bool) error
}

// HIDPointer 通过 FakerInput 虚拟 HID 绝对坐标鼠标点击。Windows 把它当作真实鼠标：
// 没有“注入”标记、不受 UIPI 限制，输入法候选框等忽略 SendInput 点击的窗口也能点中。
// 实测 HID 绝对坐标映射到主显示器；(0,0) 被视为“无位置”而忽略，最小值取 1。
type HIDPointer struct {
	mu  sync.Mutex
	dev *hidkeyboard.Device
}

func NewHIDPointer() *HIDPointer { return &HIDPointer{} }

func (h *HIDPointer) Click(d video.Display, x, y int32, right bool) error {
	if d.Left != 0 || d.Top != 0 || d.Width < 2 || d.Height < 2 {
		return fmt.Errorf("虚拟鼠标只能定位主显示器")
	}
	scale := func(v int32, size int) uint16 {
		n := (int64(v)*hidkeyboard.AbsMax + int64(size-1)/2) / int64(size-1)
		return uint16(min(max(n, 1), hidkeyboard.AbsMax))
	}
	hx, hy := scale(x, d.Width), scale(y, d.Height)
	button := hidkeyboard.MouseLeft
	if right {
		button = hidkeyboard.MouseRight
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.dev == nil {
		dev, err := hidkeyboard.Open()
		if err != nil {
			return err
		}
		h.dev = dev
	}
	// 与 SendInput 路径相同的节奏：先移入并悬停，再按下、按住、抬起。
	steps := []struct {
		buttons byte
		pause   bool
	}{{0, true}, {0, false}, {button, true}, {0, false}}
	for _, s := range steps {
		if err := h.dev.Mouse(s.buttons, hx, hy); err != nil {
			h.dev.Close()
			h.dev = nil
			return err
		}
		if s.pause {
			if s.buttons == 0 {
				pointerPause(pointerHoverDelay)
			} else {
				pointerPause(pointerPressDuration)
			}
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
