# 重启后自动连接（2026-10-07）

目标：两端配对后，重开 App、重启接收端或电脑关机重启，可以自动恢复连接。0.3.16 起手机保存多个配对，启动只恢复当前选中的电脑；该电脑离线时继续重试，不自动改连其他电脑。手动切换见 [多 PC 配对与切换](multi-pc.md)。

## 1. Android 端：保存的配对会一直重试

原来的行为只在“连接成功过、然后断开”时才重连一次（固定 1.5 s，且要求已拿到 token）。开机顺序不对（手机先起、电脑还没进系统）时第一次连接就失败，之后就永远停在“连接失败”，需要手动重连。

现在的行为：

| 改动 | 说明 |
|---|---|
| 恢复选中电脑 | 启动读取已保存且选中的有效凭据；连接失败进入退避重试循环 |
| 退避间隔 | 1 s → 2 s → 4 s → 8 s → 15 s → 30 s，之后固定 30 s |
| 首次授权与撤销 | 0.3.16 只对持有凭据的目标自动重试；新配对失败、拒绝、到期或取消后等待用户操作，未完成的临时目标不在重启后恢复；撤销的记录保留并标记需重新配对 |
| 网络恢复提前重试 | 注册 `ConnectivityManager.registerDefaultNetworkCallback`，网络恢复时重置等待；正在连接／配对时不重复发起请求 |
| 回到前台提前重试 | `setForeground(true)` 时若在等待重连，提前下一次尝试 |
| 后台降低频率 | App 不在前台时重试间隔固定 30 s，避免耗电 |
| 连上后清零 | `ready` 时重置退避计数与提示 |
| 退避期间不发连接流量 | 每次尝试先探 `/api/pair-info`，失败按退避等待；只探选中电脑，不扫描其他已保存电脑 |

相关代码：`android/app/src/main/java/com/yuncii/tapdeck/TapClient.kt`（`scheduleReconnect`、`backoffDelay`、`wakeForReconnect`、`retryCount` / `retryAt`）。

## 2. PC 端：随 Windows 登录自动启动接收端

光靠 Android 重试还不够——电脑刚开机时接收端没运行，手机再重试也连不上。所以 PC 端新增“随 Windows 登录自动启动”：

- 注册表项：`HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 的 `TapDeck`
- 命令：`"<当前 EXE 路径>" --headless`（`--headless` 启动接收端并常驻托盘，但不弹出设置窗口）
- 设置入口：设置页「设置与状态」→ 勾选「随 Windows 登录自动启动接收端」，勾选状态与下方文字实时反映注册表真实值
- 命令行：`TapDeck.exe --autostart-on` / `--autostart-off`，输出 `{"autostart":true,"command":"..."}`
- 开机自启后若想改设置：左键单击（或右键菜单「打开设置」）托盘图标即可显示设置窗口。直接双击 `TapDeck.exe` 会弹「TapDeck 已运行」提示框，它提示的就是这个托盘图标。

相关代码：`windows/internal/autostart/autostart_windows.go`（含单元测试）、`windows/cmd/tapdeck/main_windows.go`（命令行开关与「设置与状态」页复选框）。

## 3. 实测

| 场景 | 结果 |
|---|---|
| 重开 Android App（PC 保持运行） | 启动后约 6 s 自动连上，无需任何操作 |
| 强制结束 PC 接收端后重新启动（Android 保持运行） | 接收端起来后约 8 s 自动恢复连接 |
| `TapDeck.exe --autostart-on` | 注册表写入 `"D:\...\dist\TapDeck.exe" --headless` |
| `TapDeck.exe --autostart-off` | 注册表项删除 |
| 模拟开机自启（`--headless` 启动接收端） | HTTP/WSS/UDP 正常监听，Android 在 2 s 内已连接；主窗口 `IsWindowVisible=false`（不弹窗），托盘图标创建成功（`NotifyIcon.SetVisible` 无错误，日志无「托盘图标创建失败」） |
| 设置页「设置与状态」 | 复选框已勾选，下方文字显示「开机自启：已开启 · "<路径>" --headless」 |
| 自动重连单元/集成测试 | `go test ./...` 全部通过（含 `internal/autostart` 往返测试，测试结束会恢复原有注册表状态） |

## 4. 使用提示

- 开机自启写的是“当前 EXE 的绝对路径”。改用另一个版本文件名或移动到别的目录后，请在新版设置页取消再重新勾选一次（或重跑 `--autostart-on`）。构建保留不带版本号的兼容副本，可继续用于固定路径。
- 自启使用 `--headless`：登录后不弹设置窗口，但托盘有图标（左键单击或右键菜单「打开设置」显示窗口，右键「退出」结束进程）。
- 手机端首次配对可从系统扫码打开网页唤起 App，或在 App 扫码填写／手动输入网址；须由 PC 核对并允许。Android 0.3.16 收到 `pairing_revoked` 仅清除匹配电脑的令牌，保留备注和语音选择并停止重试，需要用户选择该电脑重新配对。
- 想在手机上用快捷方式一键连接，也可以在连接页填写网址后保存（等价于首次配对）。
