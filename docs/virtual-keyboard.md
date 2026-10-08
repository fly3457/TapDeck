# 虚拟键盘 FakerInput

感谢 [Ryochan7 / FakerInput](https://github.com/Ryochan7/FakerInput) 提供虚拟 HID 键盘。TapDeck 内嵌未修改的 0.1.1 x64 官方 MSI，采用上游 MIT 许可证。

首次使用豆包等输入法的语音热键时，请在 PC“快捷键 → 键盘环境”安装。部分输入法不接受软件注入，虚拟键盘可帮助激活语音输入；不需要这类功能时可跳过。

## 安装与检测

“安装 / 修复虚拟键盘”启动原版 MSI，由 Windows 请求管理员授权；“重新检测”更新状态。后台自启不立即弹提示，正常打开窗口后缺失时提示一次；录音、按键或安装忙碌时延后。

| 状态 | 操作 |
|---|---|
| 未安装 | 按需安装 |
| 已安装但不可用 | 重新检测或修复 |
| 已就绪 | 显示驱动、API 和实际发送方式 |
| 检测失败 | 查看错误后重试，不当成未安装 |

“自动”优先 HID，不支持的键使用 SendInput；强制 HID 不支持 `…` 等 Unicode 键。鼠标仍使用 SendInput。取消安装可继续软件发送，退出 TapDeck 不卸载驱动。

## 原包与实现

安装包 SHA-256：

```text
4c0aefb7340051a91d606776243298b5cd1143ef5508bbae6800c474f9ed0840
```

[发布来源](https://github.com/Ryochan7/FakerInput/releases/tag/v0.1.1)及许可证、签名信息保留在 EXE 和[第三方声明](../THIRD_PARTY_NOTICES.md)。使用 UMDF / HID 原版接口，不需要关闭 Secure Boot、内存完整性或导入自签名证书。上游已归档，版本固定；不同系统的兼容性按实测确认。

独立键盘进程经继承管道接收动作，按会话持有和释放按键；管道断开也清理。接口依据 [FakerInputDll](https://github.com/Ryochan7/FakerInputDll)，Go 实现不额外分发其 DLL。

诊断命令：

```powershell
.\dist\0.3.18\TapDeck-debug-0.3.18.exe --keyboard-status
.\dist\0.3.18\TapDeck-debug-0.3.18.exe --extract-keyboard-driver "$env:TEMP\TapDeckDriver"
.\dist\0.3.18\TapDeck-hidprobe-0.3.18.exe --out "$env:TEMP\TapDeckHIDProbe"
```

HID 工具仅在自身窗口前台接受按下动作；失去前台或超过八秒自动释放。历史本机结果见下方，当前构建及干净系统待验收范围见[验证记录](verification.md)。

<details>
<summary>2026-10-06 至 10-08 本机虚拟键盘实测</summary>

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

2026-10-07 复测补充（详见 [豆包语音热键实测](voice-hotkey.md)）：本次定位到“虚拟键盘无法激活豆包语音”有两个原因。一是组合键录入对话框此前用 walk 的修饰键字符串拼接，左右不分，按右 Ctrl+M 会记录成 `Ctrl+M`（等于左 Ctrl），已改为按 `GetAsyncKeyState` 分别读取左右修饰键并补充单元测试；二是豆包只在真正获得焦点的输入框里接受语音热键，TapDeck 自己的窗口与普通诊断窗口连实体键盘也触发不了。修复后同机复测：虚拟键盘 `RightCtrl+M` 连续 8 次全部触发豆包语音，`LeftCtrl+M` 不触发，软件注入仍不触发。

首次 60 秒并行鼠标 / 传音复测：RTT 中位数 11 ms、p95 29 ms、最大 68 ms；PC 有效鼠标收包至注入处理结束 p95 2.124 ms。第二次 60 秒复测的 RTT 中位数 12 ms、p95 27 ms、最大 54 ms；复测后 PC 最后 4,096 个样本的滚动 p95 4.223 ms。两次最大缓冲均为 6 帧，连接保持，结束采音停止。RTT 超出 20 ms 目标，此处不以历史版本结果代替本次测量。按用户要求已停止追加测试。

仍需人工完成：手机传音实际识别文字、Android 快捷键 / 圆球两种手势触发豆包、每种热键连续 10 轮豆包起停，以及真实复制 / 粘贴效果。完整音频延迟、其他设备、安装取消和要求重启场景也未实机验收。当前 100 次测试验证的是采音与键盘生命周期，并不代表豆包已识别 100 次。

</details>
