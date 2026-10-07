# 全键盘（2026-10-07）

顶部状态栏新增「全键盘」开关（与语音输入方式开关互斥显示在「连接」左侧）。打开后，下方的快捷键区与语音输入区整体隐藏，替换为一块全键盘；关闭后恢复原布局。

键盘区与原来的快捷键区 + 语音区**权重相同**（`Regions.SHORTCUTS + Regions.MICROPHONE = 0.50`）。两种模式下都只有一组下半区子视图可见，LinearLayout 按可见子视图的权重归一化，因此状态区 + 触控板 + 下半区始终吃满整个窗口高度，键盘关闭时底部不会留空。

## 界面

布局按上传的截图逐格排，每格承载两个键位：中间是**主键位**（短按），右上角是**副键位**（长按）。

| 行 | 副键位（长按） | 主键位（短按） |
|---|---|---|
| 1 | `1234567890` | `qwertyuiop` |
| 2 | `- / : ; ( ) ~ ' "` | `a s d f g h j k l` |
| 3 | `[Shift] @ - # & ? ! … [Backspace]` | `[Shift] z x c v b n m [Backspace]` |
| 4 | `[alt]`（共用） `[语音输入]` `[Shift+Enter]` `[Shift+Enter]`（共用） `[Enter]`（共用） | `[alt]` `.` `空格` `[Shift+Enter]` `[Enter]` |

第 4 行逐格：① `[alt]` 主副共用 ② `.`（长按＝语音输入键）③ `空格`（长按＝`Shift+Enter`）④ `[Shift+Enter]` 主副共用，长按连续 ⑤ `[Enter]` 主副共用，长按连续。

逐格对应关系：

- 1 行：`1↔q`、`2↔w`、`3↔e`、`4↔r`、`5↔t`、`6↔y`、`7↔u`、`8↔i`、`9↔o`、`0↔p`。
- 2 行（主键位按 QWERTY 中行 `asdfghjkl` 排列，逐格对应）：`a↔-`、`s↔/`、`d↔:`、`f↔;`、`g↔(`、`h↔)`、`j↔~`、`k↔'`、`l↔"`。
- 3 行：`[Shift]` 与 `[Backspace]` 主副键位共用；`z↔@`、`x↔-`、`c↔#`、`v↔&`、`b↔?`、`n↔!`、`m↔…`。
- 4 行：`[alt]`、`[Shift+Enter]`、`[Enter]` 主副键位共用（短按单次、长按连续）；`空格` 长按为 `Shift+Enter`；句点 `.` 长按等于「长按语音输入」，也就是按住说话、松手结束。

### 宽度（按屏幕宽度的百分比）

| 键 | 宽度 |
|---|---|
| 常规字母键、数字键、`.` | 8.5% |
| 键与键之间的间隙 | 1.5% |
| `[Shift]`、`[Backspace]`、`[Shift+Enter]` | 13.5% |
| `[alt]`、`[Enter]` | 18.5% |
| `[空格]` | 33.5% |

左右各留 0.75%，于是 1 / 3 / 4 行的总宽正好是 `键宽之和 + 间隙 = 98.5%` 加上两侧留白 = 100%（1 行 10×8.5+9×1.5、3 行 13.5+7×8.5+13.5+8×1.5、4 行 18.5+8.5+33.5+13.5+18.5+4×1.5）。2 行只有 9 键（76.5% + 12% = 88.5%），左对齐、不铺满整行。行高仍由 4 行均分键盘区高度。

实现：`KeyboardView` 用 `BoxWithConstraints` 取可用宽度，按键用 `Modifier.width(可用宽度 × 百分比)`，间隙与留白同样按百分比计算（`KEY_GAP_PERCENT` / `SIDE_PADDING_PERCENT`），不再用 Compose 权重分配宽度。

按键行为：

- **双键位键**（字母 / 数字 / 符号 / 空格 / 句点）：手指按下先发主键位，按住 400 ms 后改发副键位，松手释放当前生效的那个键位。因此短按＝主键位、长按＝副键位。
- **Shift**：短按＝单次大写（点亮，用一次后自动复位）；长按＝锁定大写（一直生效），再短按一次解除锁定。锁定状态下按字母发 `LeftShift+字母`。
- **Alt / 退格 / 回车 / Shift+Enter**：短按＝一次完整按键；长按＝真正按住不放，由 Windows 连续触发（退格连续删、回车连续换行、Shift+Enter 连续换行、Alt 可配合其它键）。
- **语音键（句点长按）**：值与「长按语音输入」相同——达到阈值后开始录音并保持 PC 端长按热键，松手结束。
- 键面点亮表示当前生效：Shift 单次 / 锁定、长按键按住、语音录音中。
- 退出全键盘、断开连接或切回普通布局时释放所有按键与修饰键。

## 协议与接收端

新增两个消息：

```json
{"type":"key_down","revision":12,"text":"LeftShift+A"}
{"type":"key_up","revision":12,"text":"LeftShift+A"}
```

- `text` 是组合键文本，由 `input.ParseChord` 解析，`revision` 用于丢弃过期配置下的按键。
- 接收端在 `session` 中校验后转给 `internal/keyboard` 的 `KeyAsync`，最终调用引擎的 `PressKey` / `ReleaseKey`（内部就是 `Hold`，组合键文本作为持有键的键）。
- 因此按下与抬起是两条独立命令：按住手机上的键就等于按住 PC 上的键（Windows 会重复），组合键（修饰键 + 普通键）也按同一段文本释放，不会残留。
- 会话结束、心跳超时或断开时 `ReleaseAll()` 释放全部按键；配置版本不一致时返回 `config` 与错误，按键被丢弃。

### 符号键走虚拟键盘

标点键（`Semicolon` / `Quote` / `Backquote` / `Slash` / `[` `]` `\`）此前不在 HID 映射表里，会被判为「虚拟键盘不支持」而整条组合键退到 SendInput——而豆包忽略 SendInput。现在这些按键在 `internal/hidkeyboard` 里补齐了 USB HID usage，2 / 3 行的 `- / : ; ( ) ~ ' " @ # & ? !` 都走虚拟键盘。

### `…` 只能按 Unicode 字符发送

Windows 键盘上没有 `…` 这个按键，虚拟键盘（HID）无法表示。按键表新增 `text` 字段与 `Ellipsis`（占位虚拟键码 `0xE000`）：注入时走 `KEYEVENTF_UNICODE`（`wVk=0`、`wScan=码点`），此时整条组合键自动走 SendInput。键盘发送方式选「虚拟键盘（HID）」时设置页会提示不支持 `Ellipsis`；「自动」与「软件按键」可用。

## 实测（ONYX Tab8C / Android 11，接收端 `dist/TapDeck.exe`）

用 `cmd/keywatch`（轮询 `GetAsyncKeyState`，与焦点窗口和输入法无关）观察 PC 实际收到的按键：

| 项目 | 结果 |
|---|---|
| 区域高度 | 关闭键盘：状态区 + 触控板 + 快捷键区 + 语音区占满 1404×1872；打开键盘：同一块下半区换成键盘，底部不再留空 |
| 键宽（截图逐像素量取，屏宽 1404） | 常规键 118 px＝8.4%、间隙 21 px＝1.5%、`[Shift]`/`[Backspace]` 188 px＝13.4%、`[alt]`/`[Enter]` 258 px＝18.4%、`[Shift+Enter]` 189 px＝13.5%、`[空格]` 469 px＝33.4%；1/3/4 行右边缘都在 x≈1390（两侧各留 10 px）；2 行 9 键止于 x≈1250，不铺满 |
| 字母 | 点 2 行 `a s j` → PC 收到 `A`、`S`、`J` |
| 双键位长按 | 按住 `k` 约 900 ms → `K` 抬起后按 `Quote`（`'`），松手释放 |
| Shift 长按锁定 | 按住 Shift（1.1 s）→ `LeftShift` 一直保持按下；随后按 `a` 只发 `LeftShift+A`，字母抬起后 `LeftShift` 不释放；再短按 Shift → `LeftShift` 释放 |
| 退格 | 轻点 → `Backspace` 按下 / 抬起 51 ms；按住 1.2 s → `Backspace` 一直保持按下，松手释放 |
| 空格 | 轻点 → `Space` 45 ms；按住 409 ms → 先 `Space`，到阈值后切换为 `Shift+Enter`，松手释放 |
| 回车 | 轻点 → `Enter` 56 ms |
| 语音键 | 长按句点（2 s）→ PC 端按住语音长按热键（本机配置 `RightAlt`）约 1.9 s；手机日志 `beginMic hold -> true` |
| 单元测试 | `go test ./...` 含 `TestKeyboardKeyStatesAreForwarded`、`TestUnicodeOnlyKeyFallsBackToSendInput`、`TestPunctuationKeysUseHID`、`TestUnicodeOnlyKey`；`gradlew testDebugUnitTest` 含 `KeyHoldTest` 8 项（双键位 / Shift / 长按键的键值序列） |

## 已知限制

- 键盘是精简布局（字母 + 符号 + 数字 + 少量功能键），没有做中文输入；中文仍需用 PC 端输入法，或继续用语音输入。PC 输入法处于中文模式时，字母会进入拼音组合，这与实体键盘一致。
- 布局固定 4 行，屏幕特别小的设备上按键会变小（字号固定，未随宽度缩放）。
- 上档字符（`@ # & ? !` 等）与 `:`、`"`、`~` 通过附加 `LeftShift` 发出，结果取决于 PC 的键盘布局；中文 / 美式布局下与预期一致。
- `…` 走 SendInput 的 Unicode 注入，目标窗口需要接受 `WM_CHAR`；表现为「按了没反应」时把键盘发送方式保持为「自动」。
- 「语音键」始终是长按说话（对应「长按语音输入」的键值），与状态栏里选择的语音输入方式无关。
