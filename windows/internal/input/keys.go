//go:build windows

package input

import (
	"fmt"
	"strconv"
	"strings"
)

// key 是本项目内部唯一的按键定义：名称、Windows 虚拟键码、物理扫描码与是否
// 扩展键。解析组合键、注入按键与设置页录入都使用这张表，避免出现“能录入却
// 保存不了”或“保存了却发不出去”的不一致。
//
// scan 为 0 时按虚拟键码发送；aliasOnly 表示该名称只能用于解析（例如
// NumpadEnter 与小键盘回车共用 VK_RETURN），不会作为反查名称返回给设置界面。
type key struct {
	name      string
	vk        uint16
	scan      uint16
	ext       bool
	aliasOnly bool
	// vkOnly 表示该键只按虚拟键码发送。音量与媒体键由 Windows 的媒体键处理
	// 逻辑响应，只发扫描码时系统不会执行音量 / 播放动作（已实测）。
	vkOnly bool
}

var keys = []key{
	// 左右修饰键按物理扫描码发送，避免依赖键盘布局。
	{name: "LeftCtrl", vk: 0xA2, scan: 0x1D},
	{name: "RightCtrl", vk: 0xA3, scan: 0x1D, ext: true},
	{name: "LeftShift", vk: 0xA0, scan: 0x2A},
	{name: "RightShift", vk: 0xA1, scan: 0x36},
	{name: "LeftAlt", vk: 0xA4, scan: 0x38},
	{name: "RightAlt", vk: 0xA5, scan: 0x38, ext: true},
	{name: "LeftWin", vk: 0x5B, scan: 0x5B, ext: true},
	{name: "RightWin", vk: 0x5C, scan: 0x5C, ext: true},

	{name: "Enter", vk: 0x0D, scan: 0x1C},
	{name: "NumpadEnter", vk: 0x0D, scan: 0x1C, ext: true, aliasOnly: true},
	{name: "Esc", vk: 0x1B, scan: 0x01},
	{name: "Backspace", vk: 0x08, scan: 0x0E},
	{name: "Tab", vk: 0x09, scan: 0x0F},
	{name: "Space", vk: 0x20, scan: 0x39},

	{name: "Insert", vk: 0x2D, scan: 0x52, ext: true},
	{name: "Delete", vk: 0x2E, scan: 0x53, ext: true},
	{name: "Home", vk: 0x24, scan: 0x47, ext: true},
	{name: "End", vk: 0x23, scan: 0x4F, ext: true},
	{name: "PageUp", vk: 0x21, scan: 0x49, ext: true},
	{name: "PageDown", vk: 0x22, scan: 0x51, ext: true},
	{name: "Up", vk: 0x26, scan: 0x48, ext: true},
	{name: "Down", vk: 0x28, scan: 0x50, ext: true},
	{name: "Left", vk: 0x25, scan: 0x4B, ext: true},
	{name: "Right", vk: 0x27, scan: 0x4D, ext: true},
	{name: "PrintScreen", vk: 0x2C, scan: 0x37, ext: true},
	{name: "ScrollLock", vk: 0x91, scan: 0x46},
	{name: "Pause", vk: 0x13, scan: 0x45},

	{name: "VolumeMute", vk: 0xAD, scan: 0x20, ext: true, vkOnly: true},
	{name: "VolumeDown", vk: 0xAE, scan: 0x2E, ext: true, vkOnly: true},
	{name: "VolumeUp", vk: 0xAF, scan: 0x30, ext: true, vkOnly: true},
	{name: "MediaNextTrack", vk: 0xB0, scan: 0x19, ext: true, vkOnly: true},
	{name: "MediaPrevTrack", vk: 0xB1, scan: 0x10, ext: true, vkOnly: true},
	{name: "MediaStop", vk: 0xB2, scan: 0x24, ext: true, vkOnly: true},
	{name: "MediaPlayPause", vk: 0xB3, scan: 0x22, ext: true, vkOnly: true},

	{name: "Numpad0", vk: 0x60, scan: 0x52},
	{name: "Numpad1", vk: 0x61, scan: 0x4F},
	{name: "Numpad2", vk: 0x62, scan: 0x50},
	{name: "Numpad3", vk: 0x63, scan: 0x51},
	{name: "Numpad4", vk: 0x64, scan: 0x4B},
	{name: "Numpad5", vk: 0x65, scan: 0x4C},
	{name: "Numpad6", vk: 0x66, scan: 0x4D},
	{name: "Numpad7", vk: 0x67, scan: 0x47},
	{name: "Numpad8", vk: 0x68, scan: 0x48},
	{name: "Numpad9", vk: 0x69, scan: 0x49},
	{name: "NumpadMultiply", vk: 0x6A, scan: 0x37},
	{name: "NumpadAdd", vk: 0x6B, scan: 0x4E},
	{name: "NumpadSubtract", vk: 0x6D, scan: 0x4A},
	{name: "NumpadDecimal", vk: 0x6E, scan: 0x53},
	{name: "NumpadDivide", vk: 0x6F, scan: 0x35, ext: true},

	{name: "Semicolon", vk: 0xBA, scan: 0x27},
	{name: "Plus", vk: 0xBB, scan: 0x0D},
	{name: "Comma", vk: 0xBC, scan: 0x33},
	{name: "Minus", vk: 0xBD, scan: 0x0C},
	{name: "Period", vk: 0xBE, scan: 0x34},
	{name: "Slash", vk: 0xBF, scan: 0x35},
	{name: "Backquote", vk: 0xC0, scan: 0x29},
	{name: "LeftBracket", vk: 0xDB, scan: 0x1A},
	{name: "Backslash", vk: 0xDC, scan: 0x2B},
	{name: "RightBracket", vk: 0xDD, scan: 0x1B},
	{name: "Quote", vk: 0xDE, scan: 0x28},
}

// 名称与别名都按去除空格、大写后的形式匹配。
var keyAliases = map[string]string{
	"CTRL": "LeftCtrl", "CONTROL": "LeftCtrl", "LCTRL": "LeftCtrl", "LEFTCTRL": "LeftCtrl",
	"RCTRL": "RightCtrl", "RIGHTCTRL": "RightCtrl",
	"SHIFT": "LeftShift", "LSHIFT": "LeftShift", "LEFTSHIFT": "LeftShift",
	"RSHIFT": "RightShift", "RIGHTSHIFT": "RightShift",
	"ALT": "LeftAlt", "LALT": "LeftAlt", "LEFTALT": "LeftAlt",
	"RALT": "RightAlt", "RIGHTALT": "RightAlt",
	"WIN": "LeftWin", "LWIN": "LeftWin", "LEFTWIN": "LeftWin",
	"RWIN": "RightWin", "RIGHTWIN": "RightWin",
	"RETURN": "Enter", "ENTER": "Enter", "NUMENTER": "NumpadEnter", "NUMPADENTER": "NumpadEnter",
	"ESC": "Esc", "ESCAPE": "Esc",
	"BACKSPACE": "Backspace", "BACK": "Backspace",
	"SPACE": "Space", "SPACEBAR": "Space", "TAB": "Tab",
	"DEL": "Delete", "DELETE": "Delete", "INS": "Insert", "INSERT": "Insert",
	"HOME": "Home", "END": "End",
	"PGUP": "PageUp", "PAGEUP": "PageUp", "PRIOR": "PageUp",
	"PGDN": "PageDown", "PAGEDOWN": "PageDown", "NEXT": "PageDown",
	"UP": "Up", "DOWN": "Down", "LEFT": "Left", "RIGHT": "Right",
	"ARROWUP": "Up", "ARROWDOWN": "Down", "ARROWLEFT": "Left", "ARROWRIGHT": "Right",
	"PRINTSCREEN": "PrintScreen", "PRTSC": "PrintScreen", "SNAPSHOT": "PrintScreen",
	"SCROLLLOCK": "ScrollLock", "SCROLL": "ScrollLock",
	"PAUSE": "Pause", "BREAK": "Pause",
	"VOLUMEUP": "VolumeUp", "VOLUP": "VolumeUp", "VOLUMEDOWN": "VolumeDown", "VOLDOWN": "VolumeDown",
	"VOLUMEMUTE": "VolumeMute", "MUTE": "VolumeMute",
	"MEDIANEXTTRACK": "MediaNextTrack", "NEXTTRACK": "MediaNextTrack",
	"MEDIAPREVTRACK": "MediaPrevTrack", "PREVTRACK": "MediaPrevTrack",
	"MEDIASTOP": "MediaStop", "MEDIAPLAYPAUSE": "MediaPlayPause", "PLAYPAUSE": "MediaPlayPause",
	"SEMICOLON": "Semicolon", "PLUS": "Plus", "MINUS": "Minus", "COMMA": "Comma",
	"PERIOD": "Period", "SLASH": "Slash", "BACKQUOTE": "Backquote",
	"LEFTBRACKET": "LeftBracket", "BACKSLASH": "Backslash", "RIGHTBRACKET": "RightBracket",
	"QUOTE": "Quote", "APOSTROPHE": "Quote",
}

var (
	keyByName = map[string]key{}
	keyByVK   = map[uint16]key{}
)

func init() {
	for _, k := range keys {
		keyByName[strings.ToUpper(k.name)] = k
		if !k.aliasOnly {
			if _, exists := keyByVK[k.vk]; !exists {
				keyByVK[k.vk] = k
			}
		}
	}
}

// KeyNames lists the names accepted by ParseChord, in table order.
func KeyNames() []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.name)
	}
	return out
}

// KeyName returns the canonical name for a Windows virtual key code. The
// settings dialog uses it so recorded keys always use a name the parser and the
// injection backends understand.
func KeyName(vk uint16) (string, bool) {
	if k, ok := keyByVK[vk]; ok {
		return k.name, true
	}
	if vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9' {
		return string(rune(vk)), true
	}
	if vk >= 0x70 && vk <= 0x87 {
		return fmt.Sprintf("F%d", vk-0x70+1), true
	}
	return "", false
}

// lookupKey resolves one chord part to its key definition.
func lookupKey(part string) (key, bool) {
	if k, ok := keyByName[part]; ok {
		return k, true
	}
	if name, ok := keyAliases[part]; ok {
		return keyByName[strings.ToUpper(name)], true
	}
	if len(part) == 1 && ((part[0] >= 'A' && part[0] <= 'Z') || (part[0] >= '0' && part[0] <= '9')) {
		return key{name: part, vk: uint16(part[0])}, true
	}
	if strings.HasPrefix(part, "F") {
		if n, e := strconv.Atoi(part[1:]); e == nil && n >= 1 && n <= 24 {
			return key{name: fmt.Sprintf("F%d", n), vk: uint16(0x70 + n - 1)}, true
		}
	}
	return key{}, false
}
