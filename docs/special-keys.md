# 特殊按键

PC 快捷键支持手动填写、单键选择及“录入”。组合键用 `+` 连接，例如 `RightCtrl+L`、`Ctrl+Shift+Enter`。

## 支持范围

| 类别 | 按键 |
|---|---|
| 字母、数字 | A–Z、0–9、F1–F24 |
| 修饰键 | 左右 Ctrl、Shift、Alt、Win；省略左右时采用左键 |
| 编辑与导航 | Backspace、Esc、Tab、Space、Enter、Insert、Delete、Home、End、PageUp、PageDown、方向键 |
| 系统键 | PrintScreen、ScrollLock、Pause |
| 小键盘 | Numpad0–9、运算符、NumpadEnter |
| 标点 | 分号、引号、反引号、加减号、逗号、句号、斜线、反斜线、方括号 |
| 音量 | VolumeUp、VolumeDown、VolumeMute |
| 媒体 | MediaNextTrack、MediaPrevTrack、MediaStop、MediaPlayPause |

名称不区分大小写；`Back`、`Escape`、`Prior`、`Next`、`Return`、`VolUp` 等旧别名继续兼容。录入与发送共用 [keys.go](../windows/internal/input/keys.go) 的名称表。

全键盘的 `…` 使用 `Ellipsis`，由 Unicode 软件输入发送；自动模式会选择软件后端，强制 HID 模式会提示不支持。其他标点支持 HID。

## 按住与释放

快捷键按下即保持组合键，抬手释放；长按方向键、退格等可由 Windows 重复。断开连接、切换 PC 或撤销设备时，只取消并释放对应会话的输入。

单击语音录制中，第一次快捷键按下只结束录音；录音结束后再次按下才发送快捷键。开始录音会先释放手机仍按住的键。

## 音量与限制

音量加减及静音直接调用 Windows Core Audio，始终作用于**系统默认播放设备**，不跟随 TapDeck 的语音输出选择。每次按下只调整一步，需松手再按；没有默认设备时提示错误。自动、HID、软件三种发送方式行为一致。

媒体键可以保存和发送，但效果取决于播放器及系统，尚未完成播放器实测。`Ctrl+Alt+Del` 等受保护的系统操作不保证有效；Fn 和系统截图行为取决于固件及 Windows。

## 开发与诊断

在 `windows` 目录运行 `go test ./...` 和 `go vet ./...`。观察接收按键可运行 `go run ./cmd/keywatch -seconds 10`；音量诊断可运行 `go run ./cmd/volkeycheck -chord VolumeUp`，会改变默认设备音量。

当前结果见[验证记录](verification.md)，输入方式见[虚拟键盘](virtual-keyboard.md)。

<details>
<summary>历史按键与音量修复实测（2026-10-07）</summary>

## 0.3.2：跟随系统默认播放设备

音量加、减、静音每次操作都通过 `IMMDeviceEnumerator.GetDefaultAudioEndpoint(eRender, eConsole)` 取得当前默认播放设备；不使用枚举顺序，也不使用 TapDeck 语音输出所选的 CABLE Input。更换默认扬声器或耳机后，下一次操作跟随新的默认设备。COM 创建、调用与释放在同一条已初始化的线程完成，不缓存跨线程的设备接口。

音量键在自动、HID、SendInput 三种发送方式下都走 Core Audio。HID 的支持检查跳过音量键，其他按键仍使用原有后端；已经按住的 HID 修饰键不会因音量操作被提前释放。失败的音量操作会恢复持有计数，避免残留 Ctrl 等修饰键，下一次按下可以重试。

本机第一枚举设备与系统默认播放设备确实不同。使用独立的 Core Audio 读取核验新版软件输入控制器：`VolumeUp` 将默认音量从 `0.3310` 调到 `0.3600`，`VolumeDown` 调到 `0.3400`；其他端点的音量与静音均未变化。测试后精确恢复默认音量 `0.3310` 和全部原静音状态。日志为 `.tools/ui-validation/volume-0.3.2-verification.log`。此项验证了 Windows 控制器；新版手机到接收端的完整操作由用户真机验收。

单测覆盖默认设备选择及切换、设备临时消失后恢复、COM 引用释放、三种后端的音量路由、HID 修饰键持有，以及音量失败后的重试与清理。接口说明见 [Microsoft GetDefaultAudioEndpoint](https://learn.microsoft.com/en-us/windows/desktop/api/Mmdeviceapi/nf-mmdeviceapi-immdeviceenumerator-getdefaultaudioendpoint)。更新该修复需要退出旧托盘程序并运行 `dist/0.3.2/TapDeck.exe`。

## 0. 快捷键的按下语义（同日追加）

快捷键按钮改为「按下即按下」：

- 手指按下 → 手机发送 `shortcut_hold_start`（带 `slot`、`revision`、唯一 `hold` 编号）→ PC 保持组合键按下。
- 手指抬起 → 发送 `shortcut_hold_stop`（带同一个 `hold` 编号）→ PC 释放该组合键。
- 因此轻点＝一次完整的按下 / 抬起，按住＝组合键持续按下（Windows 会按键重复，方向键、退格等会连续生效；音量每次按下只执行一步，需松手再按）。
- 录制（单击语音输入）进行中按下任意快捷键时只结束录音，这一次不发送按键；开始录音时手机也会撤销仍按住的键。
- 会话断开、心跳超时、单设备解绑时 PC 取消并释放该会话持有的动作和按键，保留其他设备的按键。全局 `ReleaseAll()` 用于停止接收和退出；重复的 `hold_start`（同一编号）不会重复按下，未知编号的 `hold_stop` 被忽略。

实现位置：Android `MainActivity.ShortcutHold` + `Modifier.shortcutPress`（按钮自身不处理点击，避免消费抬手事件）、`TapClient.shortcutHoldStart/shortcutHoldStop`；PC `internal/server` 的 `shortcut_hold_start` / `shortcut_hold_stop` 分支与 `session.holds`、`internal/keyboard` 的 `HoldAsync`（`hold_async` / `hold_up` 消息）。单测：`internal/server/keyboard_test.go` 的 `TestShortcutHoldPressesAndReleases`。

实测：轻点「上」→ PC 日志 `hold down` 与释放成对出现；按住「上」3.5 秒 → 用 `GetAsyncKeyState` 观察，`Up` 键从按下的瞬间一直保持按下约 3.5 秒，松手时立即释放（`05:25:07.071 [Up]` → `05:25:10.575 []`），即长按等价于实体键盘的持续按下。

## 1. 根因：录入对话框与解析器用了两套按键名

TapDeck 里按键有两条链路：设置页“录入”对话框写入的文本，和注入后端解析的文本。修复前两者用不同的名称表：

- 录入对话框用 walk 的 `key.String()`，它给出 `"Back"`、`"Escape"`、`"Prior"`、`"Next"`、`"VolumeUp"`、`"Numpad5"` 等名称。
- 解析器 `input.ParseChord` 只认 `BACKSPACE`、`ESC`、`PAGEUP`、`PAGEDOWN`、`LEFT` 等大写短名，且**完全没有**音量 / 媒体键。

于是按“后退键”得到 `Back`、按 ESC 得到 `Escape`、按音量键得到 `VolumeUp`，保存时报“无效按键 …”；音量键更是任何写法都不被接受。只有手输 `Backspace`、`Esc` 这类短名才能保存，这也是“某些特殊键保存不了”的来源。

## 2. 修复：单一按键表

新增 `windows/internal/input/keys.go`，把按键定义集中成一张表（名称、虚拟键码、物理扫描码、是否扩展键、是否仅按虚拟键码发送）。解析、注入和设置页录入都改为查这张表：

- `ParseChord` 查表并支持别名（`Back`/`Escape`/`Prior`/`Next`/`Return`/`VolUp` 等历史写法继续可用）。
- `input.KeyName(vk)` 提供“虚拟键码 → 规范名称”的反查，录入对话框改用它，因此录进去的名称一定能被解析。
- `keyInput` 用表中的扫描码与扩展标记构造 SendInput 事件。
- 设置页“单键选择”增加音量加 / 音量减 / 静音三项。

表内包含：`Backspace`、`Esc`、`Tab`、`Space`、`Enter`、`Insert`、`Delete`、`Home`、`End`、`PageUp`、`PageDown`、四个方向键、`PrintScreen`、`ScrollLock`、`Pause`、`F1`–`F24`、小键盘 0–9 与运算符、标点键、六个左右修饰键、左右 Win，以及音量 / 媒体键。

### 键盘上没有的字符：`Ellipsis`（同日追加）

全键盘 m 键的长按键位是 `…`，Windows 键盘上没有这个按键，虚拟键盘（HID）无法表示。表中新增 `text` 字段与 `Ellipsis`（占位虚拟键码 `0xE000`，落在私有使用区，不会与真实按键冲突）：

- `keyInput` 对该键发送 `KEYEVENTF_UNICODE`（`wVk=0`、`wScan=码点`），按下与抬起各一次。
- `internal/keyboard` 的路由判定里它不属于 HID 支持的按键，因此「自动」模式会把整条组合键交给 SendInput；强制 HID 模式会明确报「虚拟键盘不支持」。
- 同时补齐了标点键的 HID usage（`Semicolon`/`Quote`/`Backquote`/`Slash`/`[`/`]`/`\`），否则含这些键的组合键会整体退到 SendInput——而豆包忽略 SendInput。

实测：`TestUnicodeOnlyKey`（Unicode 编码与反查）、`TestUnicodeOnlyKeyFallsBackToSendInput`（路由）、`TestPunctuationKeysUseHID`（标点键与 `0xE000` 的 HID 判定）。

单元测试：`internal/input/keys_windows_test.go`（全表往返、历史别名、非法键拒绝）与 `cmd/tapdeck/walk_key_test.go`（录入门槛：backspace / esc / 音量键等必须记录成可解析名称）。

## 3. 历史音量实现：本机注入无效，改用 Core Audio

本机音量加减与静音通过注入按键没有效果，曾对 `VK_VOLUME_MUTE` 逐一实测六种 SendInput 编码：

| 编码 | 结果 |
|---|---|
| 虚拟键码 | 系统音量不变 |
| 虚拟键码 + `KEYEVENTF_EXTENDEDKEY` | 系统音量不变 |
| 扫描码 | 系统音量不变 |
| 扫描码 + 扩展标记 | 系统音量不变 |
| 虚拟键码 + 扫描码 | 系统音量不变 |
| 虚拟键码 + 扫描码 + 扩展标记 | 系统音量不变 |

`SendInput` 均返回成功，但本机系统没有执行音量动作。因此 TapDeck 改为在软件发送路径上拦截音量键，直接调用 Core Audio 的 `IAudioEndpointVolume`（`internal/audio/volume_windows.go`），与系统音量面板使用同一套接口：

- 音量加 / 减用 `VolumeStepUp` / `VolumeStepDown`，与系统音量键的步进一致。
- 静音用 `SetMute` / `GetMute` 翻转。
- 不使用 `SetMasterVolumeLevelScalar`：它要求按 x64 ABI 在 XMM1 传 float，Go 的 `syscall` 无法传浮点参数（实测返回 `E_INVALIDARG`）。
- 接口槽位由本机实测确认：9 `GetMasterVolumeLevelScalar`、14 `SetMute`、15 `GetMute`、16 `GetVolumeStepInfo`、17 `VolumeStepUp`、18 `VolumeStepDown`。
- 按住不放时只执行一次音量动作，松手后再次触发才继续调整；没有 Windows 默认播放设备时给出明确错误。是否启用 CABLE 不影响其他默认扬声器或耳机的音量控制。

实测（`internal/audio/volume_windows_test.go`）：默认播放设备的音量在 `Step(true)` 后上升、`Step(false)` 后下降，静音状态可翻转并还原；测试结束恢复原状态。

`internal/input/volume_windows_test.go` 覆盖路由：音量键不会进入 SendInput、按住不重复、含修饰键的组合只注入修饰键、没有音量控制器时返回错误。


</details>
