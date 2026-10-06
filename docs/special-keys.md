# 特殊按键：支持范围与音量键实现（2026-10-07）

本次处理“后退键 / ESC 键保存不了、音量加减键不能保存”的问题。根因、修复与实测如下。

## 0. 快捷键的按下语义（同日追加）

快捷键按钮改为「按下即按下」：

- 手指按下 → 手机发送 `shortcut_hold_start`（带 `slot`、`revision`、唯一 `hold` 编号）→ PC 保持组合键按下。
- 手指抬起 → 发送 `shortcut_hold_stop`（带同一个 `hold` 编号）→ PC 释放该组合键。
- 因此轻点＝一次完整的按下 / 抬起，按住＝组合键持续按下（Windows 会按键重复，方向键、音量键等都会连续生效）。
- 录制（单击语音输入）进行中按下任意快捷键时只结束录音，这一次不发送按键；开始录音时手机也会撤销仍按住的键。
- 会话断开、心跳超时、解除配对时 PC 调用 `ReleaseAll()` 释放所有按键；重复的 `hold_start`（同一编号）不会重复按下，未知编号的 `hold_stop` 被忽略。

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

单元测试：`internal/input/keys_windows_test.go`（全表往返、历史别名、非法键拒绝）与 `cmd/tapdeck/walk_key_test.go`（录入门槛：backspace / esc / 音量键等必须记录成可解析名称）。

## 3. 音量键：不能注入，改用 Core Audio

音量加减与静音**无法**通过注入按键实现。本机对 `VK_VOLUME_MUTE` 逐一实测了六种 SendInput 编码：

| 编码 | 结果 |
|---|---|
| 虚拟键码 | 系统音量不变 |
| 虚拟键码 + `KEYEVENTF_EXTENDEDKEY` | 系统音量不变 |
| 扫描码 | 系统音量不变 |
| 扫描码 + 扩展标记 | 系统音量不变 |
| 虚拟键码 + 扫描码 | 系统音量不变 |
| 虚拟键码 + 扫描码 + 扩展标记 | 系统音量不变 |

`SendInput` 均返回成功，但系统不会执行音量动作（Windows 忽略注入的音量 / 媒体键）。因此 TapDeck 改为在软件发送路径上拦截音量键，直接调用 Core Audio 的 `IAudioEndpointVolume`（`internal/audio/volume_windows.go`），与系统音量面板使用同一套接口：

- 音量加 / 减用 `VolumeStepUp` / `VolumeStepDown`，与系统音量键的步进一致。
- 静音用 `SetMute` / `GetMute` 翻转。
- 不使用 `SetMasterVolumeLevelScalar`：它要求按 x64 ABI 在 XMM1 传 float，Go 的 `syscall` 无法传浮点参数（实测返回 `E_INVALIDARG`）。
- 接口槽位由本机实测确认：9 `GetMasterVolumeLevelScalar`、14 `SetMute`、15 `GetMute`、16 `GetVolumeStepInfo`、17 `VolumeStepUp`、18 `VolumeStepDown`。
- 按住不放时只执行一次音量动作，松手后再次触发才继续调整；关闭虚拟声卡或没有播放设备时给出明确错误，不再发一个系统会忽略的按键。

实测（`internal/audio/volume_windows_test.go`）：默认播放设备的音量在 `Step(true)` 后上升、`Step(false)` 后下降，静音状态可翻转并还原；测试结束恢复原状态。

`internal/input/volume_windows_test.go` 覆盖路由：音量键不会进入 SendInput、按住不重复、含修饰键的组合只注入修饰键、没有音量控制器时返回错误。

## 4. 仍然受限的按键

- **媒体键**（下一曲 / 上一曲 / 停止 / 播放暂停）与音量键同样属于“注入无效”的一类，本机未接播放器实测，因此 TapDeck 目前只对音量键走 Core Audio；媒体键可以保存和发送，但实际效果取决于系统是否响应注入事件，需要人工确认。
- **`Ctrl+Alt+Del`**、**Win 组合键的部分系统级行为**受 Windows 保护，不承诺可用。
- **`Fn`、`PrintScreen` 的系统截图行为**由固件 / 系统接管，TapDeck 只负责发送按键本身。

## 5. 复现与验证命令

```powershell
cd windows
go test ./...            # 含按键表、录入名称、音量路由与 Core Audio 实测
go vet ./...
```

音量与静音验证需要本机存在播放设备；没有设备时 `NewVolumeControl` 返回“没有可用的播放设备”，音量键会明确报错而不是静默失效。
