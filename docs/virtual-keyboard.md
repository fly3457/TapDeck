# 虚拟键盘集成与验证

日期：2026-10-06 至 2026-10-07。Windows 0.3 与现有 Android 0.2 / 控制协议 v2 配合使用。

## 采用方案

`TapDeck.exe` 内置未修改的 FakerInput 0.1.1 x64 官方 MSI（1,089,536 字节）、MIT 许可证与版本清单。安装包 SHA-256：

```
4c0aefb7340051a91d606776243298b5cd1143ef5508bbae6800c474f9ed0840
```

MSI、驱动 DLL、CAT 的签名均有效，发布者为 Ryodigi Solutions LLC。驱动是 UMDF 用户态组件，使用 Windows 自带的 HID / WUDF 组件；没有修改 INF、关闭内存完整性、关闭 Secure Boot 或启用测试签名。官方 MSI 在本机返回 0，设备状态 OK，无需重启。签名检查不等于在所有 Windows 设备上已通过兼容性验收。

Go 客户端通过 SetupAPI 与 HID API 找到 VID `FE0F` / PID `00FF`、Usage Page `FF00` 的控制端点，检查 API v1 后发送原版 65 字节报告。无需 CGO 或额外客户端 DLL。HID 负责键盘，鼠标仍使用现有 SendInput。

键盘工作进程与接收端使用同一 EXE，以继承的匿名管道通信。点按保持 50 ms，持有键按引用计数管理，左右修饰键独立。自动模式下 F13–F24 使用软件发送；已持有的共用 Ctrl 保持原来的发送方式，避免回退快捷键释放语音热键。驱动写入失败不会把已发出的组合键重复发送到其他后端。

## 实测与验收边界

| 项目 | 结果 |
|---|---|
| 安装及安全设置 | 原版 MSI 安装成功，PnP 状态 OK，API v1；Secure Boot 与内存完整性保持启用 |
| 事件来源 | Raw Input 显示虚拟设备路径；关联的键盘钩子事件无 `LLKHF_INJECTED`，不是 SendInput 注入事件 |
| 豆包长按与免按 | 使用虚拟键盘发送右 Ctrl+M、右 Ctrl+L，用户现场确认两种模式均可触发 |
| 手机传音形成文字 | 用户选择稍后测试，本次未完成联合验收；不能据此声称最终识别效果已验收 |
| 100 次混合语音操作 | Tab8C / 新接收端通过，83.181 秒；覆盖拖动、取消、后台、断线重连，结束后采音停止 |
| 主进程强制结束 | 真实 Android 长按期间强制终止主进程，Ctrl+M 约 62 ms 内释放，工作进程退出；Android 断线后停止采音 |
| 六个左右修饰键 | 左右 Alt / Ctrl / Shift 的实际按下、释放均通过 |
| 共用 Ctrl | 长按 Ctrl+M 时发送 Ctrl+C 及 Ctrl+F24，Ctrl / M 保持按下，最终释放正确；HID 与软件回退两种情况通过 |
| 离线分发 | 从 EXE 解出原版 MSI，哈希一致，离线 Authenticode 验证通过；安装包、许可证均在 EXE 内 |
| 已有实例 | 重复启动显示“TapDeck 已运行”，未创建第二个接收端或键盘工作进程 |
| 配置与凭据 | 用户现有字段保留，配对文件哈希未变化，接收端重启后自动连接 |
| Go 验证 | `go test ./...` 与 `go vet ./...` 通过；覆盖跨后端引用计数、配置快照、准备取消、迟到就绪、重复停止、设备写入失败和管道断开清理 |

豆包此次设置为右 Ctrl，而原 TapDeck 配置中 `Ctrl` 表示左 Ctrl。升级保留用户配置，不自动替换热键。请按豆包设置填写 `RightCtrl+M`、`RightCtrl+L`、`RightCtrl+L`；也可在豆包中设置与 TapDeck 一致的左侧键。未开启豆包全局语音快捷键时，目标输入框需要先选择豆包输入法。

首次 60 秒并行鼠标 / 传音复测：RTT 中位数 11 ms、p95 29 ms、最大 68 ms；PC 有效鼠标收包至注入处理结束 p95 2.124 ms。第二次 60 秒复测的 RTT 中位数 12 ms、p95 27 ms、最大 54 ms；复测后 PC 最后 4,096 个样本的滚动 p95 4.223 ms。两次最大缓冲均为 6 帧，连接保持，结束采音停止。RTT 超出 20 ms 目标，此处不以历史版本结果代替本次测量。按用户要求已停止追加测试。

仍需人工完成：手机传音实际识别文字、Android 快捷键 / 圆球两种手势触发豆包、每种热键连续 10 轮豆包起停，以及真实复制 / 粘贴效果。完整音频延迟、其他设备、安装取消和要求重启场景也未实机验收。当前 100 次测试验证的是采音与键盘生命周期，并不代表豆包已识别 100 次。

## 使用与诊断

PC“快捷键”页选择自动 / HID / SendInput，保存后生效；录音或按键执行期间禁止切换。缺少驱动时显示安装入口；取消安装后键鼠可继续使用软件发送。修复入口重新运行原版 MSI 并检测设备，程序不自动重启电脑。正常退出不卸载驱动。

```powershell
.\dist\TapDeck-debug.exe --keyboard-status
.\dist\TapDeck-debug.exe --extract-keyboard-driver "$env:TEMP\TapDeckDriver"
.\dist\TapDeck-debug.exe --install-keyboard-driver
.\dist\TapDeck-hidprobe.exe --out "$env:TEMP\TapDeckHIDProbe"
```

HID 诊断窗口通过输出目录中的 `command.json` 接收动作，例如 `{"id":1,"action":"down","chord":"RightCtrl+M"}`，接着用更大的 id 发送 `up` 或 `release`。支持 `down` / `up` / `tap` / `release`。只有本诊断窗口在前台才允许发出按下动作，失去前台或诊断持有超过 8 秒自动释放；事件日志仅记录本测试窗口在前台时的事件。控制文件请使用完整 JSON，每次提高 id，关闭诊断程序会释放其持有键。

本机验证日志在 `.tools/hid-validation`，不会打包用户配置、凭据或测试输入内容。分发 ZIP 中包含 Windows GUI、控制台版、HID 诊断版、现有 Android APK、使用说明、协议与本验证记录。

上游参考：[官方发布](https://github.com/Ryochan7/FakerInput/releases/tag/v0.1.1)、[MIT 许可证](https://github.com/Ryochan7/FakerInput/blob/v0.1.1/LICENSE)、[接口实现](https://github.com/Ryochan7/FakerInputDll/blob/master/FakerInputDll/fakerinputclient.cpp)、[Microsoft 键盘钩子标记](https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-kbdllhookstruct)。上游仓库已归档，此版本固定分发，后续 Windows 兼容性需要继续实测。
