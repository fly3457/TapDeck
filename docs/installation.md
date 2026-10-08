# TapDeck 0.3 安装、设备管理与虚拟声卡

## 安装与配对

PC“连接”页二维码内容是 `http://<PC 地址>:<HTTP 端口>/pair`。手机用系统扫码工具打开网页，下载内置 Android APK；安装后返回网页点击“打开 TapDeck 连接”。已安装用户也可在 App“连接与设备设置”的网址下方操作行右侧点扫码图标（与“忘记当前电脑”同排），扫描 PC 二维码自动填写网址，再点“连接”；仍支持手动输入。

App 内扫码使用 [ZXing Android Embedded](https://github.com/journeyapps/zxing-android-embedded)，仅在点击扫码时请求相机权限；相机硬件可选，无相机或拒绝权限时仍可手动输入。扫码只回填 PC 配对网址，取消或扫到非配对二维码时保留原值，不自动发起连接。所有首次配对都显示校验码，由 PC 核对并允许。旧链接的 `secret` 字段不会产生授权。控制协议仍为 v2，现有凭据和旧客户端的 `pair_confirm` 保持兼容。

## 发布构建

发布统一使用根目录的 `scripts/build-windows.ps1`，默认产物位于 `dist`。可传 `-OutputDirectory` 输出到独立目录，方便在已有接收端运行时验证新版本。

1. 从 `version.properties` 读取接收端和 Android 独立版本，运行版本规则测试，再调用 Android 构建和 Kotlin 单元测试。
2. 从 `android/app/build/outputs/apk/debug/app-debug.apk` 复制该次 Gradle 产物，不读取旧 `dist` APK。
3. 验证 Gradle `output-metadata.json` 与 APK 内部版本均匹配 Android 版本清单，对比复制前后的 SHA-256，生成内嵌版本清单。
4. 从接收端版本生成 Windows 清单与 VERSIONINFO 资源，运行 Go 测试、`go vet`，构建带版本号的 EXE，并校验文件版本及产品版本。
5. 从两个接收端 EXE 读取 `--apk-info`，核对实际内嵌 APK 版本、code、名称、大小与哈希，生成 `release-manifest.json` 和 `SHA256SUMS.txt`。APK 缺失、Android 失败、哈希或版本信息不一致均终止交付。

当前接收端为 `0.3.13`，Android `versionName=0.3.13`、`versionCode=16`，均来自根目录 `version.properties`，两端版本允许不同。保留 `com.yuncii.tapdeck` 及本机原有开发签名。APK 文件名为 `TapDeck-0.3.13.apk`；独立 Android 构建、Windows 交付目录和内嵌下载均使用 Android 版本命名，另保留 `TapDeck-debug.apk` 兼容副本。手机“连接与设备设置”标题下显示安装版本，网页和 PC 显示内嵌版本及完整 SHA-256。`TapDeck-debug.exe --version` 输出接收端版本，`--apk-info` 可分别核对两端版本、下载文件名和内嵌哈希。每次迭代的修改位置和固定步骤见 [版本管理](versioning.md)。

换构建机时必须保留相同签名密钥，才能覆盖已有安装。主界面说明见 [android-ui.md](android-ui.md)，本次单 EXE 输出至 `dist/0.3.13`。更新时退出旧托盘程序后启动新版 EXE，再覆盖安装 Android APK。连接不支持双指缩放和三指窗口操作的旧电脑端时，App 会提示新手势需要升级。

Windows 主程序为 `TapDeck-0.3.13.exe`，控制台诊断版为 `TapDeck-debug-0.3.13.exe`，HID 工具为 `TapDeck-hidprobe-0.3.13.exe`。三者的文件版本与产品版本均显示 `0.3.13`，Windows 数字版本为 `0.3.13.0`。构建仍保留 `TapDeck.exe`、`TapDeck-debug.exe` 和 `TapDeck-hidprobe.exe`，分别与带版本号的文件完全相同。资源生成器保留原有 Common Controls、DPI 和管理员权限清单。

## PC 关于与手机输入测试

PC“关于”页显示 Windows 版本、内置 Android 版本、应用用途、支持的平台、项目主页和许可说明，下方提供可滚动的多行文本输入框。“开始输入测试”让输入框获得焦点，再使用手机键盘或语音输入；“清空”清除测试文本并把焦点放回输入框。文本只用于当前窗口的临时测试，不写入配置。语音识别仍由电脑当前输入法完成，需按“语音”页配置热键与音频路由。

## 已配对设备

PC“连接”页列出名称、在线／离线状态、完整设备标识和最近连接时间。同名设备按 ID 区分。选中一行并点击“解除所选设备配对”，确认框显示目标名称和 ID；“解除全部配对”另有独立确认。

配对文件为 `paired.json` schema 2，记录令牌哈希、名称、配对时间及最近连接时间。旧 ID→哈希文件在首次迁移时原样备份为 `paired.v1.bak`；迁移失败不覆盖原文件。旧设备先显示“旧设备（标识末 8 位）”，重连后补齐名称和最近连接时间。UI 的设备快照不含令牌哈希。

解绑先持久化删除，成功后才撤销授权并关闭目标会话；保存失败保留授权。待处理请求被取消，建立会话前复核授权，以防解绑和重连并发。键盘工作进程追踪动作所属会话，取消该会话排队中的动作并释放它持有的按键及语音热键；鼠标按钮按会话计数。全局清理用于停止接收和退出。

Android 0.3 收到 `pairing_revoked` 后清除对应电脑凭据、停止重试，提示手动重新连接；再次配对仍需 PC 允许。音频继续单路录音，推送、停止和取消均核对录音归属。

## VB-CABLE 安装

EXE 内嵌未修改的官方 **VBCABLE_Driver_Pack45.zip**，固定 SHA-256：

```text
b950e39f01af1d04ea623c8f6d8eb9b6ea5c477c637295fabf20631c85116bfb
```

检测结合 PnP 的 `VBAudioVACWDM` / `VBAudioVACMME` 身份和活动 CABLE Input / Output 端点，分为未安装、已安装但不可用、可用。驱动存在而端点禁用或待重启时，不会当成缺失而重复运行安装向导。

首次正常打开设置且缺失时提示安装；开机自启只更新状态。语音页保留“安装虚拟声卡”“重新检测”、VB-Audio 官网及原包许可；移除单独的“捐赠 / 购买”按钮，用户可经官网找到捐赠和购买许可途径。安装会解出 ZIP 全部文件，验证 `VBCABLE_Setup_x64.exe` 和 `vbaudio_cable64_win10.cat` 的离线签名，管理员授权后打开官方交互向导。TapDeck 不静默接受原包许可、不修改驱动、不导入签名证书。

根据 [VB-Audio 官方安装说明](https://vb-audio.com/Cable/VBCABLE_ReferenceManual.pdf)，安装完成后必须重启 Windows。TapDeck 显示取消、失败、已安装或需要重启的结果，由用户自行安排重启。安装记录保留系统启动时间，重开 App 仍显示需要重启；通过 [Windows LastBootUpTime](https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-operatingsystem#lastbootuptime)确认系统启动时间已改变后再重新检测，两个端点可用才完成安装验收。

- TapDeck 输出选择 **CABLE Input**。
- 输入法、会议或录音软件的麦克风选择 **CABLE Output**。
- TapDeck 不自动更改 Windows 默认麦克风。

VB-CABLE 来自 [VB-Audio / Vincent Burel](https://vb-audio.com/Cable/)，是 donationware，欢迎[捐赠或购买](https://vb-audio.com/Services/licensing.htm)。完整原包许可保留在 ZIP 和 EXE 内，当前公开分发依据见[官方分发条件](https://vb-audio.com/Services/licensing.htm)及 [第三方声明](../THIRD_PARTY_NOTICES.md)。企业部署不属于本次默认用途。

## 本轮验证与待验收项

自动测试覆盖旧格式迁移及备份失败、保存失败保留授权、同名离线设备、在线单设备解绑、待处理请求取消、并发重连、旧 secret 不能自动授权、已有凭据重连、按会话键盘队列清理、鼠标共享持有、另一台设备录音隔离，以及模拟安装结果和完整原包解出后的签名验证。APK 验证包含内嵌／HTTP 内容哈希、版本及 Range 下载。Kotlin 测试覆盖凭据继承、地址变化、不同电脑身份隔离及旧凭据撤销匹配。

Windows 官方构建完成 Go／Kotlin 单元测试和 `go vet`。默认自动构建跳过会实际修改本机音量、开机自启注册表的两项测试，其余测试执行。本机已有 VB-CABLE，检测结果为可用；没有重复运行安装向导。

仍需在干净 Windows 11 x64 环境人工验证离线安装、UAC 取消、重启后端点可用，以及新版手机传音进入 CABLE Output。本机无可用干净虚拟机，新版 APK 真机覆盖升级、凭据保留与系统扫码／网页唤起由用户实机验收；构建继续使用相同包名和签名，versionCode 递增。当前使用隔离的 API 34 模拟器验收布局和交互，结果及历史真机记录见 [verification.md](verification.md)，历史结果不作为本轮实机结论。
