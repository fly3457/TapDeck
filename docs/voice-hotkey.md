# 豆包语音热键与虚拟键盘实测（2026-10-07）

本次会话在用户本机（Windows 11 Pro x64，豆包输入法 v0.9.1.22，TapDeck 0.3 + FakerInput 0.1.1）实测了“虚拟键盘无法激活豆包语音输入”这一问题。结论分三部分：能触发、不能触发的原因、以及本次修好的代码缺陷。

## 1. 结论

| 事项 | 实测结果 |
|---|---|
| FakerInput 虚拟键盘（HID）能否触发豆包语音 | **能**。在具备文本输入焦点的窗口（记事本编辑区）连续 8 次发送 `RightCtrl+M`，豆包语音界面 8 次全部出现，延迟约 1.2–1.6 s，松手后 1–2 s 内回到空闲 |
| SendInput 能否触发豆包语音 | **不能**。同一窗口连续 3 次软件注入 `RightCtrl+M` 全部无反应；像素差分显示 HID 触发时界面变化 52,644 采样点，SendInput 仅 491–628（时钟/光标噪声） |
| 左右修饰键是否有区别 | **有**。豆包设置显示“右 Ctrl + M / 右 Ctrl + L”，实测 `LeftCtrl+M` 不触发、`RightCtrl+M` 才触发 |
| 组合键录入对话框是否区分左右 | **修复前不区分**：按右 Ctrl+M 会被记录成 `Ctrl+M`（等于左 Ctrl）。已修复并验证 |
| 是否要求前台窗口有文本输入焦点 | **是**。在 TapDeck HID 诊断窗口（普通编辑框）里，实体键盘与虚拟键盘都触发不了；豆包自身日志里有 `[VHK] ... voice start was rejected by precheck` 一类分支。语音必须在真正的输入框获得焦点时触发 |

## 2. 根因

用户实际看到的配置是 `Ctrl+M` / `Ctrl+L`（不是豆包界面上写的“右 Ctrl”）。TapDeck 的按键表里 `Ctrl` 就是左 Ctrl：

```go
// windows/internal/input/input_windows.go
names := map[string]uint16{"CTRL": 0xA2, "CONTROL": 0xA2, ..., "RIGHTCTRL": 0xA3, "RCTRL": 0xA3, ...}
```

因此 TapDeck 一直在发左 Ctrl，而豆包只认右 Ctrl，表现为“虚拟键盘没反应”。同时实体键盘按右 Ctrl 可以触发。

配置为什么会变成 `Ctrl+M`：设置页的“录入”对话框用 walk 的修饰键状态拼接组合键，而 `walk.ModifiersDown()` 只认左 Alt/Ctrl/Shift：

```go
// walk/keyboard.go
const (ModShift Modifiers = 1 << iota; ModControl; ModAlt)
func ModifiersDown() Modifiers // 只用 ShiftDown/ControlDown/AltDown
```

`Modifiers` 的字符串表里只有 `"Shift"`、`"Ctrl"`、`"Alt"`，没有左右之分，`walk.KeyControl` 也不区分左右。所以用户按右 Ctrl+M 录入，得到的文本就是 `Ctrl+M`；而 `Ctrl` 在 TapDeck 里解析成左 Ctrl，两边语义并不一致。这是本次修掉的缺陷。

## 3. 代码修复

`windows/cmd/tapdeck/main_windows.go` 的组合键录入不再依赖 walk 的修饰键字符串，改为按左右分别读取 `GetAsyncKeyState(VK_LCONTROL/VK_RCONTROL/...)`，按 Ctrl → Shift → Alt 顺序拼接左右名称：

```go
var modifierNames = []struct{ Name string; VK uint16 }{
    {"LeftCtrl", 0xA2}, {"RightCtrl", 0xA3},
    {"LeftShift", 0xA0}, {"RightShift", 0xA1},
    {"LeftAlt", 0xA4}, {"RightAlt", 0xA5},
}
func chordWithModifiers(main string, down func(uint16) bool) string
```

对话框提示同步改为“先按住修饰键，再按主键，左右修饰键会分别记录”。单键选择下拉（左/右 Alt、左/右 Ctrl、左/右 Shift）本来就会写入 `RightCtrl` 这类名称，未受影响。

单元测试 `windows/cmd/tapdeck/chord_capture_test.go` 覆盖：右 Ctrl+M → `RightCtrl+M`、左 Ctrl+M → `LeftCtrl+M`、右 Shift+K → `RightShift+K`、无修饰键 → `Enter`、单选修饰键 → 空，并断言录制结果能被 `input.ParseChord` 解析成对应的左右 VK，且右 Ctrl 不会被折叠成通用 `Ctrl`。

端到端验证使用 `windows/cmd/chordprobe`（已归档到 `.tools/voice-tapdeck/`）：它打开与设置页相同的录入对话框，再用 FakerInput 虚拟键盘按下组合键，打印对话框实际记录的文本：

```
RightCtrl+M   -> recorded="RightCtrl+M"
LeftCtrl+M    -> recorded="LeftCtrl+M"
RightShift+K  -> recorded="RightShift+K"
```

修复前同样的路径记录为 `Ctrl+M`。

## 4. 现场排查手段

- `tools/voice-tapdeck/voiceprobe.exe --mode watch --hold 60s`：按 150 ms 采样，打印豆包语音界面（`ImeService.exe` 的 `OimeVoiceWaveWindow`）的出现/消失时间，用来判断某个操作是否真的触发了语音。
- `tools/voice-tapdeck/voiceprobe.exe --mode hid --chord RightCtrl+M --hold 500ms`：直接用虚拟键盘发送组合键，绕过手机与接收端，用于区分“发送链路问题”和“输入法不接受”问题。
- `tools/voice-tapdeck/chordprobe.exe --chord RightCtrl+M`：验证设置页录入对话框记录的左右键。
- 像素差分（`pixdiff`）：发送组合键前后整屏对比，量化“有没有出现语音界面”。
- `keybits` 一类的按键状态探针（本次临时使用）：确认 FakerInput 对六个修饰键的左右编码正确，`GetAsyncKeyState` 分别只报告 `RightCtrl`、`LeftCtrl`、`RightAlt`、`LeftAlt`、`RightShift`、`LeftShift`。

## 5. 使用注意

1. 豆包设置里是“右 Ctrl”时，TapDeck 的语音热键必须写 `RightCtrl+M`（长按）/ `RightCtrl+L`（免按开始与结束都填它）；只有“单键选择”或“录入”对话框才能正确带上左右，手输 `Ctrl+M` 等于左 Ctrl。
2. 触发时必须让真正的输入框获得焦点（记事本编辑区、浏览器搜索框、聊天输入框等）。TapDeck 自己的设置窗口、普通诊断窗口不算合格目标，此时连实体键盘也触发不了。
3. 长按模式：按住圆球达到阈值后 TapDeck 按下热键并保持，松手释放，与豆包“长按模式”对应。免按模式：开始与结束各发一次 `RightCtrl+L`，对应豆包“免按模式”的起停。
4. 本机 Alt 键硬件异常，实测中右 Alt 的触发结果不可复现，因此本记录不推荐用 Alt 作为豆包语音热键。
5. 本次仍属本机实测：语音识别成文字、免按模式连续起停、以及 30 分钟级长测未在本次会话中重做。
