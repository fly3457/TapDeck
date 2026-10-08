# 自动重连与开机自启

配对后重开手机 App，只连接当前选中的 PC。目标离线时保留选择，不自动切到其他电脑；切换见[多 PC](multi-pc.md)。

## 手机重连

- 前台退避为 1、2、4、8、15、30 秒，之后保持 30 秒；后台使用 30 秒。
- 网络恢复或回到前台可提前重试，已有连接请求不重复发起。
- 只探测选中电脑，收到 `ready` 后重置计数。
- 首次配对失败、拒绝、取消或到期后等待用户操作；撤销授权保留“需重新配对”条目并停止重试。
- 临时配对不会在重启后恢复，忘记当前 PC 也停止重试。

## Windows 自启

PC“设置与状态”勾选“随 Windows 登录自动启动接收端”，登记当前 EXE 的绝对路径及 `--headless`。登录后常驻托盘，不弹设置；单击托盘或选择“打开设置”即可配置。

移动 EXE 或改用另一个版本文件名后，请取消并重新勾选，更新路径。也可使用：

```powershell
.\dist\0.3.18\TapDeck-0.3.18.exe --autostart-on
.\dist\0.3.18\TapDeck-0.3.18.exe --autostart-off
```

登记项位于当前用户 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\TapDeck`。不要求管理员权限；开机自启仍在用户登录后运行。

下方为历史本机结果；当前重连、凭据保留和测试范围见[验证记录](verification.md)。

<details>
<summary>2026-10-07 自动重连与自启实测</summary>

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

</details>
