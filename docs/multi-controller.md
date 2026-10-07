# 配对确认与多控制端（2026-10-07）

两项改动：手机上不再需要点“校验码一致”；一台 PC 可以同时接最多 **5 个** Android 控制端。

## 1. 配对只由 PC 端确认

以前是双向确认：手机弹「核对配对校验码」对话框、点「与电脑一致」，PC 再点「校验码一致，允许」。现在只需 PC 端确认：

- 手机收到 `pair_challenge` 后直接进入「等待电脑允许连接」，对话框只显示校验码与「取消」，没有确认按钮；状态栏文案同 PC 端一致，方便肉眼核对。
- 接收端不再等待客户端的 `pair_confirm`：发完 `pair_challenge` 就等 PC 端允许。为兼容老版本 App（它仍会发这条消息并把等待 PC 允许当作确认后的动作），接收端收到 `pair_confirm` 直接忽略。
- 扫码配对（`qr_verified`）保持原样：二维码里的临时 secret 已经证明是同一台机器，PC 端不必再点一次。
- 对接协议没有变（仍是 v2），新旧组合都能工作：新接收端忽略老 App 的 `pair_confirm`，老接收端仍能收到新 App 自动发出的 `pair_confirm`。

## 2. 一台 PC 同时接最多 5 个控制端

- `Server.current` 换成 `Server.sessions map[uint64]*session`，上限 `server.MaxSessions = 5`。
- 名额检查放在配对之前：满了就直接回 `{"type":"error","code":"too_many_clients","reason":"接收端最多同时连接 5 个控制端，请先断开其中一个"}` 并断开，不再让新设备白跑一遍配对。
- 同一台设备重连时先替换掉它的旧会话，不会占两个名额（旧会话会被关闭）。
- UDP 鼠标 / 音频按包头里的会话 id 各自路由（`activeSession` 改为按 id 查表），键鼠注入共用同一套引擎：键盘按引用计数持有键，会话断开时只释放自己持有的键。
- 配置更新（“保存并同步配置”）推送给**所有**会话；解除配对、停止接收会把所有会话一起关闭。
- 某台设备的录音结束/断开只中断它自己的采音，不会打断另一台正在传音的设备（`Audio.Abort()` 只在它确实在录音时调用）。
- PC 设置窗口的状态行：1 台显示「已连接：名字」，多台显示「已连接 N 个控制端：名字、名字…」；配对区显示「等待配对请求（最多同时连接 5 个控制端）」，有待处理请求时显示「<名字> 请求连接（同时连接上限 5）+ 校验码」。

### 同时多台时的行为

- 多台可以同时移动鼠标、滚轮、按键；注入按到达顺序执行（没有仲裁，谁先到谁先生效）。
- 键盘修饰键 / 按住键按引用计数：A 按住 Ctrl 时 B 按 Ctrl+Z 不会把 A 的 Ctrl 释放掉。
- 多台同时录音会混音到同一个虚拟麦克风（CABLE Input），一般应只在一台上说话。

## 3. 诊断工具 `cmd/ctrlprobe`

手边只有一台手机时，用它假装多个控制端来验证名额与配对：

```powershell
# 全新设备：走配对流程，需要在 PC 设置窗口点「校验码一致，允许」
go run ./cmd/ctrlprobe -name Probe-A -seconds 60

# 已配对设备：配合 paired.json 里的哈希免配对（device 填 key，token 填明文）
go run ./cmd/ctrlprobe -device probe-1-device-id -token probetoken -seconds 60

# 只想知道能不能连上（收到 ready 或 error 就退出）
go run ./cmd/ctrlprobe -seconds 0
```

## 4. 实测

| 项目 | 结果 |
|---|---|
| 单台连接 | 手机（Sony XQ-DQ72）保持连接，状态行「已连接：Sony XQ-DQ72」 |
| 5 台同时在线 | 平板 + 4 个 `ctrlprobe`（各自不同设备编号、各自配对）→ 状态行「已连接 5 个控制端：Probe-1、Probe-2、Probe-3、Probe-4、Sony XQ-DQ72」，每个 `ctrlprobe` 各自拿到会话编号（如 `adccb393f8fe28b8`） |
| 第 6 台 | 立刻收到 `too_many_clients`：`被拒绝：接收端最多同时连接 5 个控制端，请先断开其中一个` |
| 释放名额 | 关闭其中一个会话后，新的控制端可以再配对接入 |
| 配对只需 PC 确认 | `ctrlprobe` 只发 `hello`、不发任何确认，PC 点「校验码一致，允许」后直接打印 `已建立会话：e3d00800147c89ad`；同机状态行变为「已连接 2 个控制端：PairLive、Sony XQ-DQ72」 |
| 配置广播 | `TestConfigUpdateReachesEverySession`：两个会话都收到 `config` |
| 路由 | `TestPacketsRouteToOwningSession`：未知会话 id 不匹配，两个 id 各自命中自己的会话 |
| 单元测试 | `TestMultipleControllersUpToLimit`、`TestSameDeviceReplacesItsSession`、`TestPairWithoutClientConfirmation` 等全部通过 |

## 5. 已知限制

- 上限是硬编码的 5（`server.MaxSessions`），改大需要重编接收端；界面里写死的文案也用它，所以会跟着变。
- 没有「踢掉某一台」的界面：要腾名额得让那台手机断开（或关闭它的 App），或者在 PC 上「解除配对」清空全部。
- 多台同时操作鼠标会互相抢；多台同时录音会混音。
- 手机端界面没有“我是第几台”的提示，只有连上/未连上。
