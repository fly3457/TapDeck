//go:build windows

package main

import "tapdeck/internal/driver"

type keyboardDriverPrompt struct{ checked bool }

// Take is called only on the UI thread. Hidden startup, busy input, an ongoing
// installer or a detection failure must not consume the first visible check.
func (p *keyboardDriverPrompt) Take(visible, busy bool, state driver.State, err error) (title, message string) {
	if p.checked || !visible || busy || err != nil {
		return "", ""
	}
	if state != driver.Missing && state != driver.Unavailable && state != driver.Ready {
		return "", ""
	}
	p.checked = true
	switch state {
	case driver.Missing:
		return "安装虚拟键盘", "未检测到 FakerInput 虚拟键盘。部分输入法需要它才能响应语音热键。\n\nTapDeck 已内置原版签名安装包，可离线安装；Windows 会请求管理员授权。\n\n现在安装？也可稍后从“快捷键”页安装。"
	case driver.Unavailable:
		return "修复虚拟键盘", "检测到已安装 FakerInput，但当前虚拟键盘不可用。可先重新检测，或从“快捷键”页修复驱动。\n\n现在修复？Windows 会请求管理员授权。"
	}
	return "", ""
}
