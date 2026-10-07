//go:build windows

package keyboard

import (
	"context"
	"fmt"
	"tapdeck/internal/input"
)

func validateGesture(action string) error {
	if action != "up" && action != "down" {
		return fmt.Errorf("无效三指手势")
	}
	return nil
}
func validateZoom(steps int) error {
	if steps == 0 || steps < -4 || steps > 4 {
		return fmt.Errorf("无效缩放步进")
	}
	return nil
}
func (e *Engine) currentModifiers() input.Modifiers {
	read := e.readModifiers
	if read == nil {
		read = input.CurrentModifiers
	}
	result := read()
	for key, owner := range e.owners {
		if owner.Count == 0 {
			continue
		}
		switch key {
		case 0xA2, 0xA3:
			result.Ctrl = true
		case 0xA0, 0xA1:
			result.Shift = true
		case 0xA4, 0xA5:
			result.Alt = true
		case 0x5B, 0x5C:
			result.Win = true
		}
	}
	return result
}
func (e *Engine) zoom(ctx context.Context, steps int) error {
	if err := validateZoom(steps); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if ctx.Err() != nil {
		return errCanceled
	}
	if e.fault != "" {
		return fmt.Errorf("%s", e.fault)
	}
	modifiers := e.currentModifiers()
	if modifiers.Alt || modifiers.Shift || modifiers.Win {
		return fmt.Errorf("请先松开 Alt、Shift 或 Win，再使用双指缩放")
	}
	sender, ok := e.soft.(interface{ ZoomSteps(int, bool) error })
	if !ok {
		return fmt.Errorf("缩放输入不可用")
	}
	return sender.ZoomSteps(steps, modifiers.Ctrl)
}
func (e *Engine) gesture(ctx context.Context, direction string) error {
	if err := validateGesture(direction); err != nil {
		return err
	}
	e.mu.Lock()
	if e.navigator == nil {
		observer, err := newNativeWindowObserver()
		if err != nil {
			e.mu.Unlock()
			return err
		}
		e.navigator = newWindowGestures(observer)
	}
	navigator := e.navigator
	e.mu.Unlock()
	return navigator.run(ctx, direction, func(chord string) error {
		e.mu.Lock()
		modifiers := e.currentModifiers()
		e.mu.Unlock()
		if modifiers.Ctrl || modifiers.Alt || modifiers.Shift || modifiers.Win {
			return fmt.Errorf("请先松开 Ctrl、Alt、Shift 或 Win，再使用三指手势")
		}
		return pulse(ctx, e, chord)
	})
}
