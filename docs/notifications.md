# 电脑通知转发

从 Windows 接收端 0.5.0 / Android 0.5.0 起，电脑上的 Windows 系统通知会直接转发到已连接的手机。手机在前台时在顶部显示横幅，在后台或锁屏时以手机通知弹出；顶栏铃铛图标显示未读数，点击打开最近 100 条通知；视频模式右上角同样有铃铛（Android 0.5.1 起）。

## 原理

接收端调用 Windows 公开的 WinRT API `UserNotificationListener`，每秒读取一次通知中心，只转发新出现的通知：

- 不需要逐个软件对接，微信、Outlook、浏览器、Claude 桌面版等所有弹出 Windows 通知的应用都会转发。
- 接收端启动时已在通知中心的旧通知不转发。
- 转发的内容是应用名、标题、正文（最多 500 字）和时间，不包括通知中的图片和按钮。

前提是 Windows 允许应用读取通知：“设置 → 隐私和安全性 → 通知”中打开“允许应用访问你的通知”。接收端“通知”页显示当前权限状态，并提供直达按钮。

PC“通知”页还可以：

- 关闭转发（PC 端专用字段 `notify_disabled`，缺省为转发）。
- 填写不转发的应用名，名称与通知中心显示的一致，逗号分隔（字段 `notify_exclude`）。
- 发送一条测试通知。

> TapDeck 自己弹出的通知（测试通知、Agent 提醒）在 Windows 中没有应用名，手机上只显示标题。

## 手机在后台时

“连接与设置”中的“后台接收电脑通知”默认开启。开启后 App 运行一个 `connectedDevice` 类型的前台服务，并常驻一条低优先级的“后台连接”通知，保证进程不被回收。

App 进入后台时，手机发送 `link_mode background`，从此不再每 250 ms 发一次心跳：

- 改由 PC 每 20 秒发一次 `keepalive`，手机收到后回应。在两次 keepalive 之间，手机 CPU 可以休眠。
- PC 60 秒、手机 75 秒内收不到对方的任何消息，才判定断线。
- 断线后按后台间隔（30 秒）自动重连，Wi-Fi 变化时立即重试。
- 回到前台后恢复 250 ms 心跳。

每台 PC 保留最近 100 条通知，按“启动标识 + 序号”编号。手机重连后发 `notify_sync`，补收离线期间的通知。首次连接或 PC 重启后补发的旧通知只进入列表；只有 10 分钟内的通知才弹出手机通知，一次最多 5 条。

建议同时完成以下设置：

1. 首次连上支持通知转发的电脑时，允许 TapDeck 发送通知（Android 13+）。
2. 允许 TapDeck 后台运行（关闭电池优化）。首次连上支持通知转发的电脑时 App 会说明原因并打开系统弹窗，之后也可在“连接与设置”中点击按钮。**这一步是必需的**：实测努比亚 NX741J 不豁免电池优化时，即使有前台服务，进程也会在锁屏约 20 秒后被系统冻结（`isFrozen=true`），PC 随即判定心跳超时；豁免后进程不再被冻结。未豁免时“后台连接”通知会给出提示。
3. 努比亚、小米等机型还需要在系统设置中允许 TapDeck 自启动和后台运行。

关闭开关后前台服务停止，App 回到旧行为：在后台时不保持连接。

## Agent 完成提醒

Agent 只需弹一条 Windows 通知，就会经同一条通路同步到手机，不需要专门的协议。

- **Claude 桌面版、Codex 桌面版**等自身会弹 Windows 通知的应用：无需配置。
- **Claude Code / Codex 命令行**：通过 hook 调用 TapDeck 弹出通知。接收端每次启动都会写入稳定入口 `%LOCALAPPDATA%\TapDeck\bin\tapdeck-notify.cmd`，它指向当前接收端 EXE，hook 配置不随版本文件名变化。

### Claude Code

在 PC“通知”页点击“复制 Claude Code 钩子配置”，然后合并到 `%USERPROFILE%\.claude\settings.json` 的顶层。如果文件里已有 `hooks`，只把 `Stop` 与 `Notification` 两项合并进去：

```json
{
  "hooks": {
    "Stop": [{ "hooks": [{ "type": "command", "command": "\"C:/Users/<用户名>/AppData/Local/TapDeck/bin/tapdeck-notify.cmd\" --agent-hook claude" }] }],
    "Notification": [{ "hooks": [{ "type": "command", "command": "\"C:/Users/<用户名>/AppData/Local/TapDeck/bin/tapdeck-notify.cmd\" --agent-hook claude" }] }]
  }
}
```

两类事件生成的通知：

| 事件 | 通知标题 | 通知内容 |
|---|---|---|
| `Stop` | `Claude Code · <项目目录名>` | `任务完成：<最后一条回复的前 120 字>`。回复取自会话记录。 |
| `Notification` | `Claude Code · <项目目录名>` | Claude Code 的提示原文，例如需要你授权某个工具。 |

### Codex

点击“复制 Codex 配置”，然后写入 `%USERPROFILE%\.codex\config.toml` 的顶层：

```toml
notify = ["C:/Users/<用户名>/AppData/Local/TapDeck/bin/tapdeck-notify.cmd", "--agent-hook", "codex"]
```

Codex 每轮结束时以最后一个参数传入 JSON。TapDeck 取 `last-assistant-message` 的前 120 字作为通知内容；解析失败时显示“任务完成”。

### 命令行

接收端 EXE 支持以下命令行，执行完即退出，不启动接收端：

```powershell
TapDeck-0.5.0.exe --notify-title "构建完成" --notify-body "dist/0.5.0 已生成"
TapDeck-0.5.0.exe --agent-hook claude   # 从 stdin 读取 Claude Code hook JSON
TapDeck-0.5.0.exe --agent-hook codex '{"type":"agent-turn-complete", ...}'
```

通知以 AppUserModelID `Yuncii.TapDeck` 弹出。该身份登记在 `HKCU\Software\Classes\AppUserModelId\Yuncii.TapDeck`，不需要管理员权限。

## 范围与限制

- 只接收当前连接的那台 PC 的通知。
- 在手机上清除通知不会同步到 PC。
- PC 接收端未运行时产生的通知不补发。
- 协议细节见[控制协议](../protocol/README.md#notifications)，界面见 [Android 布局](android-ui.md#电脑通知)。
