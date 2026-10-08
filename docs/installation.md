# TapDeck 0.3 安装、设备管理与虚拟声卡

## 安装与配对

PC“连接”页二维码内容是 `http://<PC 地址>:<HTTP 端口>/pair`。手机用系统扫码工具打开网页，下载内置 Android APK；安装后返回网页点击“打开 TapDeck 连接”。已安装用户也可在 App“连接与设置”的“输入PC连接窗口URL”标题右侧点扫码图标，扫描 PC 二维码自动填写网址，再点“连接”；仍支持手动输入。弹窗版本下方提供 GitHub 项目主页入口。

0.3.15 的 PC 连接页依次显示二维码与三步说明、网址及复制／网页按钮、已配对设备，不再显示刷新二维码和默认审批按钮。首次请求到达时自动弹出设备名、标识和两行校验码；其他页面或托盘状态也会提示，多请求依次处理。关闭或 Esc 等同拒绝；手机取消、超时或接收停止后自动关闭失效请求。Android 0.3.15 收到拒绝／超时原因后停止本次自动重试，等待用户再次点击连接；已有凭据的自动重连不变。

“连接”“关于”不显示保存按钮：配对和解绑独立持久化，输入测试仅保留在当前窗口。“快捷键”“语音”“设置与状态”继续显示“保存并同步配置”，统一保存各页配置；切换页面不丢失未保存的编辑。二维码在启动和网址变化时自动更新。

App 内扫码使用 [ZXing Android Embedded](https://github.com/journeyapps/zxing-android-embedded)，仅在点击扫码时请求相机权限；相机硬件可选，无相机或拒绝权限时仍可手动输入。扫码只回填 PC 配对网址，取消或扫到非配对二维码时保留原值，不自动发起连接。所有首次配对都显示校验码，由 PC 核对并允许。旧链接的 `secret` 字段不会产生授权。控制协议仍为 v2，现有凭据和旧客户端的 `pair_confirm` 保持兼容。

Android 0.3.16 可分别配对多台 PC。点击应用顶部电脑名称切换，管理页可设置备注名及单独忘记。每次只连接一台；目标离线时只重试该电脑，录音结束后才能切换。覆盖安装自动迁移旧配对和语音选择，手机标识、灵敏度和震动设置保持。详细步骤见 [多 PC 配对与切换](multi-pc.md)。

## 发布构建

发布统一使用根目录的 `scripts/build-windows.ps1`，默认产物位于 `dist`。可传 `-OutputDirectory` 输出到独立目录，方便在已有接收端运行时验证新版本。

1. 从 `version.properties` 读取接收端和 Android 独立版本，运行版本规则测试，再调用 Android 构建和 Kotlin 单元测试。
2. 从 `android/app/build/outputs/apk/debug/app-debug.apk` 复制该次 Gradle 产物，不读取旧 `dist` APK。
3. 验证 Gradle `output-metadata.json` 与 APK 内部版本均匹配 Android 版本清单，对比复制前后的 SHA-256，生成内嵌版本清单。
4. 从接收端版本生成 Windows 清单与 VERSIONINFO 资源，运行 Go 测试、`go vet`，构建带版本号的 EXE，并校验文件版本及产品版本。
5. 从两个接收端 EXE 读取 `--apk-info`，核对实际内嵌 APK 版本、code、名称、大小与哈希，生成 `release-manifest.json` 和 `SHA256SUMS.txt`。APK 缺失、Android 失败、哈希或版本信息不一致均终止交付。

当前接收端为 `0.3.17`，Android `versionName=0.3.17`、`versionCode=20`，均来自根目录 `version.properties`，两端版本允许不同。保留 `com.yuncii.tapdeck` 及本机原有开发签名。APK 文件名为 `TapDeck-0.3.17.apk`；独立 Android 构建、Windows 交付目录和内嵌下载均使用 Android 版本命名，另保留 `TapDeck-debug.apk` 兼容副本。手机“连接与设置”标题下显示安装版本，网页和 PC 显示内嵌版本及完整 SHA-256。`TapDeck-debug.exe --version` 输出接收端版本，`--apk-info` 可分别核对两端版本、下载文件名和内嵌哈希。每次迭代的修改位置和固定步骤见 [版本管理](versioning.md)。

换构建机时必须保留相同签名密钥，才能覆盖已有安装。主界面说明见 [android-ui.md](android-ui.md)，本次单 EXE 输出至 `dist/0.3.17`。更新时退出旧托盘程序后启动新版 EXE，再覆盖安装 Android APK。连接不支持双指缩放和三指窗口操作的旧电脑端时，App 会提示新手势需要升级。

Windows 主程序为 `TapDeck-0.3.17.exe`，控制台诊断版为 `TapDeck-debug-0.3.17.exe`，HID 工具为 `TapDeck-hidprobe-0.3.17.exe`。三者的文件版本与产品版本均显示 `0.3.17`，Windows 数字版本为 `0.3.17.0`。构建仍保留 `TapDeck.exe`、`TapDeck-debug.exe` 和 `TapDeck-hidprobe.exe`，分别与带版本号的文件完全相同。资源生成器保留原有 Common Controls、DPI 和管理员权限清单。

## 虚拟键盘启动检测

0.3.14 在打开 PC 设置窗口时自动检测 FakerInput。检测结合键盘工作进程的实际 HID 可用性与 PnP 硬件 ID `root\FakerInput`：未安装时弹出安装提示，已安装但不可用时提示重新检测／修复，正常可用时不弹窗。已禁用或尚未就绪的设备不会误判成未安装。检测失败时显示错误，不自动发起安装。

安装提示每次运行只出现一次，取消后仍可在“快捷键”页点“安装 / 修复虚拟键盘”或“重新检测”。开机后台自启延后到用户打开窗口，按键或录音忙碌时延后提示；键盘与声卡安装提示依次处理，结果弹窗关闭后才继续。只有用户选择安装／修复时才调用内置原版签名 MSI，并由 Windows 请求管理员授权。详情见 [虚拟键盘说明](virtual-keyboard.md)。

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

首次正常打开设置且缺失时提示安装；开机自启只更新状态。语音页保留“安装虚拟声卡”“重新检测”、VB-Audio 官网及原包许可；移除单独的“捐赠 / 购买”按钮，0.3.14 进一步移除固定的“欢迎捐赠”“安装后需重启 Windows”文字，保留来源与 donationware 说明。安装会解出 ZIP 全部文件，验证 `VBCABLE_Setup_x64.exe` 和 `vbaudio_cable64_win10.cat` 的离线签名，管理员授权后打开官方交互向导。TapDeck 不静默接受原包许可、不修改驱动、不导入签名证书。

根据 [VB-Audio 官方安装说明](https://vb-audio.com/Cable/VBCABLE_ReferenceManual.pdf)，安装完成后必须重启 Windows。TapDeck 显示取消、失败、已安装或需要重启的结果，由用户自行安排重启。安装记录保留系统启动时间，重开 App 仍显示需要重启；通过 [Windows LastBootUpTime](https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-operatingsystem#lastbootuptime)确认系统启动时间已改变后再重新检测，两个端点可用才完成安装验收。

- TapDeck 输出选择 **CABLE Input**。
- 系统音频输入或目标输入法的麦克风选择 **CABLE Output**。
- TapDeck 不自动更改 Windows 默认麦克风。

PC“快捷键”页分为“键盘环境”和“快捷键设置”；键盘发送方式、驱动检测与安装位于上方。PC“语音”页分为“语音输入环境”和“语音快捷键设置”。环境板块以红色单行显示上述路由，VB-CABLE 就绪状态与 donationware／VB-Audio 说明同排。最后一行依次为输入电平、音频设备选择、“刷新音频设备”和“系统音频输入设置”；电平前不再重复设备名称。语音配置的启用、名称和类型同排，勾选展开热键，取消勾选收起热键并保留字段；长按类型的字段显示“长按热键”。手机单击语音方块空闲时显示“开始”，准备、传音和停止阶段显示“结束”。

“系统音频输入设置”直接打开 Windows“声音 → 录制”设备列表，可选中 CABLE Output 并按需设为默认输入设备；若控制面板启动失败，则尝试 [Windows 默认麦克风设置](https://learn.microsoft.com/en-us/windows/apps/develop/launch/launch-settings#sound)。入口只打开设置，不自动切换麦克风。“刷新音频设备”用于更新 TapDeck 的输出设备列表。

VB-CABLE 来自 [VB-Audio / Vincent Burel](https://vb-audio.com/Cable/)，是 donationware。完整原包许可保留在 ZIP 和 EXE 内，当前公开分发依据见[官方分发条件](https://vb-audio.com/Services/licensing.htm)及 [第三方声明](../THIRD_PARTY_NOTICES.md)。企业部署不属于本次默认用途。

## 本轮验证与待验收项

自动测试覆盖旧格式迁移及备份失败、保存失败保留授权、同名离线设备、在线单设备解绑、待处理请求取消、并发重连、旧 secret 不能自动授权、已有凭据重连、按会话键盘队列清理、鼠标共享持有、另一台设备录音隔离，以及模拟安装结果和完整原包解出后的签名验证。APK 验证包含内嵌／HTTP 内容哈希、版本及 Range 下载。Kotlin 测试覆盖凭据继承、地址变化、不同电脑身份隔离及旧凭据撤销匹配。

0.3.17 官方 Windows 构建完成 59 项 Kotlin 单元测试、29 项版本检查、Go 测试和 `go vet`，两个接收端内嵌的 APK 与独立交付包哈希相同。默认自动构建跳过会实际修改本机音量、开机自启注册表的两项测试。本轮在 96 DPI 下验证 PC 默认 740×800、最小 680×700 窗口的两页分组、单行提示、字段边界和启用展开；API 34 隔离模拟器在普通屏、小屏大字体下完成 6 项语音回归。0.3.16 的五种尺寸／字体矩阵、扫码及三个独立接收端多电脑链路结果保留在历史记录；本轮未重复驱动安装向导。

仍需在干净 Windows 11 x64 环境人工验证离线安装、UAC 取消、重启后端点可用，以及新版手机传音进入 CABLE Output。本机无可用干净虚拟机和已连接实体手机，新版 APK 真机覆盖升级、凭据保留、系统扫码／网页唤起，以及一部手机与两台真实 PC 的反复切换和录音限制仍待验收。构建继续使用相同包名和签名，versionCode 递增；模拟器结果不代表实体手机端到端验收。详情见 [verification.md](verification.md)。
