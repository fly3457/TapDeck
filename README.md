# TapDeck

**面向 Vibe Coding 的手机语音与触控控制台。**

把 Android 手机放在电脑旁，用语音向 AI 编程工具描述需求，用快捷键提交、复制、粘贴和撤销，再用触控板与键盘查看结果、补充细节。电脑显示编辑器、AI 对话和运行预览，手机集中提供输入与操作，让长段提示词和反复迭代更顺手。

TapDeck 将手机麦克风的声音传到 Windows，配合电脑上的语音输入法把话语转换成当前输入框中的文字。语音识别由你选择的输入法完成，TapDeck 负责传音、触发热键和控制键鼠。

**[下载 Windows EXE](https://github.com/fly3457/TapDeck/releases/download/v0.3.6/TapDeck.exe) · [下载 Android APK](https://github.com/fly3457/TapDeck/releases/download/v0.3.6/TapDeck-debug.apk) · [查看完整发布包](https://github.com/fly3457/TapDeck/releases/tag/v0.3.6)**

以上为 0.3.6 公开下载链接。当前接收端与 Android 源码版本为 0.3.15 / Android code 18：PC 连接与关于页移除保存按钮，首次配对自动弹出校验码确认框；语音配置支持折叠和同行表单；Android“连接与设置”增加项目主页，扫码入口移至 URL 标题右侧。统一版本清单支持两端独立升版，EXE 内嵌本次构建的最新 APK。

## 为 Vibe Coding 准备的功能

| 功能 | 在 AI 编程中的用途 |
|---|---|
| 手机语音输入 | 用已有手机的麦克风描述需求、复述报错、补充上下文。支持按住说话、松手结束，也支持轻点开始、再点结束的连续输入 |
| 1–8 个可配置快捷键 | 把复制、粘贴、撤销、回车等常用操作放在手机上；按 AI 工具的使用习惯设置名称与组合键 |
| 触控板与全键盘 | 移动、点击、拖拽、滚动、缩放和切换窗口；需要时切到全键盘补英文、数字和符号，长按空格直接说话 |
| 每台手机独立调节 | 触控板灵敏度为 0.5×–3.0×，默认 1.0×；按键震动、语音模式和控件位置保存在当前 Android 设备上 |
| 配对后自动重连 | 首次由 PC 核对校验码并允许，之后重开 App 或接收端自动重连；支持电脑登录后在托盘后台启动 |
| 单 EXE 交付 | Windows 程序内嵌对应版本的 APK、虚拟键盘与虚拟声卡安装包；手机扫码即可打开安装与配对网页 |

## 一轮 Vibe Coding 怎么用

1. 在电脑上选中 AI 编程工具的输入框。
2. 用手机说出需求，例如：“把登录按钮加大，提交时显示加载状态，失败时保留输入内容。”
3. 语音输入法生成文字后，用手机快捷键按工具习惯确认、换行或撤销；切换全键盘补上数字、符号或英文。
4. 用触控板切到编辑器、文档或运行预览，查看结果，再继续说出下一轮修改。

默认启用八个快捷键，名称、顺序与按键如下；可在 PC“快捷键”页修改，再点“保存并同步配置”。升级保留已有设置，旧四槽位配置补充的四项仍关闭。

| 顺序 | 默认名称 | 按键 |
|---|---|---|
| 1 | 音量- | `VolumeDown` |
| 2 | 上 | `Up` |
| 3 | 音量+ | `VolumeUp` |
| 4 | 退格 | `Backspace` |
| 5 | 左 | `Left` |
| 6 | 下 | `Down` |
| 7 | 右 | `Right` |
| 8 | 回车 | `Return` |

## 界面预览

快捷键＋语音模式与全键盘模式共用底部区域，切换时触控板边界保持一致。

<p>
  <img src="docs/screenshots/0.3.6/phone-100-shortcuts-4.png" alt="TapDeck 快捷键和语音输入界面" width="280">
  <img src="docs/screenshots/0.3.6/phone-100-keyboard.png" alt="TapDeck 全键盘界面" width="280">
</p>

以上为 0.3.6 模拟器截图，快捷键名称及组合键使用演示配置。

## 下载与安装

下表公开下载包为 **0.3.6（测试版）**，Android `versionCode=9`；接收端源码及本地构建为 **0.3.15**，内嵌 Android **0.3.15** / code `18`。支持 **Windows 11 x64**、**Android 8 / API 26 及以上**；手机与电脑需要处于可互通的局域网。

| 文件 | 用途 |
|---|---|
| [TapDeck.exe](https://github.com/fly3457/TapDeck/releases/download/v0.3.6/TapDeck.exe) | Windows 接收端，已内嵌新版 APK 和驱动原包，可独立运行 |
| [TapDeck-debug.apk](https://github.com/fly3457/TapDeck/releases/download/v0.3.6/TapDeck-debug.apk) | Android App，保留现有包名和签名，可覆盖安装 |
| [TapDeck-0.3.6.zip](https://github.com/fly3457/TapDeck/releases/download/v0.3.6/TapDeck-0.3.6.zip) | 完整交付包，含 EXE、APK、诊断工具、说明、许可证与验证截图 |
| [SHA256SUMS.txt](https://github.com/fly3457/TapDeck/releases/download/v0.3.6/SHA256SUMS.txt) | 文件与完整 ZIP 的 SHA-256 校验值 |

首次使用：

1. **启动电脑端。** 下载并运行 `TapDeck.exe`，允许访问本地网络。正常打开窗口时检测 FakerInput 虚拟键盘，缺失时提示安装，已安装但不可用时提示修复；也可稍后在“快捷键”页操作。窗口关闭后程序留在托盘，从托盘选择“退出”才停止接收；升级时先退出旧版本。
2. **安装手机端。** 用手机系统扫码工具扫描 PC“连接”页的二维码，打开网页下载 APK；也可使用上面的直接下载链接。已安装用户在网页点“打开 TapDeck 连接”，或在 App 中输入 PC 显示的网址。
3. **完成配对。** PC 自动弹出确认框，核对手机上的校验码后点“允许连接”。关闭或拒绝会结束本次请求，需要在手机重新点击连接；配对成功后保存凭据，下次自动重连。
4. **配置语音链路。** PC“语音”页可安装 VB-CABLE 并重新检测。按红色提示，TapDeck 输出选择 **CABLE Input**，系统音频输入或目标输入法的麦克风选择 **CABLE Output**；“系统音频输入设置”直接打开 Windows“声音 → 录制”设备列表。
5. **设置语音配置。** 在 PC“语音”页选择需要的组、名称和类型，填写与输入法一致的热键后保存同步；首次使用手机麦克风时允许录音权限。
6. **安排常用快捷键。** PC“快捷键”页启用 1–8 项并保存同步，就可以开始操作。

## 语音联动设置

音频路径为：**手机麦克风 → 局域网传音 → CABLE Input / Output → PC 语音输入法 → 当前输入框**。

- 手机语音区顶部显示当前组名，按 PC 顺序轮换已启用配置。只有一组时切换按钮置灰；全部关闭时显示“语音未启用”。圆形控件按住说话、松手结束，方形控件轻点开始、再点结束。录音期间不能换组，控件位置与选中组保存在本机。单击录音时第一次快捷键或触控板点击只结束录音。
- PC 固定三组，每组可命名、启用和选择长按／单击类型。长按设置一个触发热键；单击分别设置开始／结束热键。空键仅传音；目标输入框需获得焦点。新安装默认启用“单击语音输入”（开始／结束均 `RightCtrl+L`）和“长按语音输入”（`RightAlt`）；第三组“自定义语音输入”默认关闭、类型为长按，所有热键均留空。升级保留已有热键。
- 全键盘长按空格使用当前组，始终按住开始、松手结束；单击组也会在松手时发送结束热键。全部关闭时空格不触发录音。详细规则见 [三组语音配置](docs/voice-profiles.md)。
- 左右修饰键分别识别。例如输入法设置为右 Ctrl＋M，应填写 `RightCtrl+M`；`Ctrl+M` 表示左 Ctrl＋M。可使用“录入”或“单键选择”填写。
- Windows EXE 内嵌 FakerInput 虚拟键盘安装包。正常启动窗口时自动检测并提示安装／修复，每次运行只提示一次；后台自启延后到打开设置时提示。“快捷键”页可随时重新检测或安装／修复，键盘发送方式默认使用“自动”。
- VB-CABLE 安装需管理员授权并重启 Windows。TapDeck 提供安装与检测入口，麦克风选择按上面的 CABLE 路由设置，程序不自动更改 Windows 默认麦克风。

豆包输入法的热键配置与历史实测见 [语音热键说明](docs/voice-hotkey.md)，驱动与安装细节见 [安装引导](docs/installation.md) 和 [虚拟键盘说明](docs/virtual-keyboard.md)。

## 常用操作

| 手机操作 | 电脑效果 |
|---|---|
| 单指移动、轻点 | 移动鼠标、左击；首次轻点等待约 300 ms 判定 |
| 双击后第二次按住 | 持有左键，可拖拽；快速松手才形成双击 |
| 双指轻点、滑动 | 右击、滚动；自然滚动方向可在 PC 设置 |
| 双指张开、合拢 | 通过 Ctrl＋滚轮放大、缩小，适用于支持该操作的应用 |
| 三指上下滑 | 打开／关闭任务视图，或显示桌面／恢复原窗口状态 |
| 快捷键轻点、按住 | 发送一次组合键，或持续保持按下；松手释放 |
| 全键盘短按、长按 | 短按发送主键、长按发送副键；Ctrl、Shift、回车及 Shift＋回车支持相应组合和保持操作 |

触控板左上角图标可调灵敏度。手机顶部连接图标打开“连接与设置”，其中提供“按键震动反馈”开关和“测试震动”按钮。

手机“连接与设置”中可查看安装版本和项目主页，“输入PC连接窗口URL”右侧的扫码图标用于回填网址，点击“连接”才发起请求；“忘记当前电脑”仅在已连接时显示于输入框下方。PC“关于”页显示版本和应用信息，下方的多行文本框可测试手机键盘与语音输入；先点击输入框或“开始输入测试”，语音识别仍使用电脑当前输入法。

## 连接与设备管理

一台 PC 最多同时连接 **5 个 Android 控制端**，适合手机、平板按需使用。PC“连接”页显示已配对设备及在线状态，可以按设备解除配对。各设备共享电脑的鼠标和键盘，当前语音为单路录音。

PC“设置与状态”可开启“随 Windows 登录自动启动接收端”，登录后在托盘后台运行。手机在启动、网络恢复及回到前台时尝试重连；被解除配对后，需要重新在 PC 允许连接。

TapDeck 的键鼠指令与音频使用加密局域网连接；所选语音输入法如何识别、是否联网，取决于该输入法自身。

## 开发与构建

Android 使用 Kotlin，Windows 接收端使用 Go，控制协议为 v2。当前工具版本为 Go 1.26.4、JDK 17、Android SDK Platform 37、Build Tools 36.0.0、Gradle 9.3.1 和 AGP 9.1.1。

在仓库根目录的 PowerShell 执行官方发布入口：

```powershell
. .\scripts\versioning.ps1
$releaseVersions = Get-TapDeckVersions (Get-Location).Path
.\scripts\build-windows.ps1 -OutputDirectory (Join-Path 'dist' $releaseVersions.ReceiverVersion)
# 自行指定 JDK 和 Android SDK 时：
.\scripts\build-windows.ps1 -JavaHome 'C:\path\to\jdk17' -SdkRoot "$env:LOCALAPPDATA\Android\Sdk"
```

每次新迭代先运行 `scripts/bump-version.ps1`，默认同时递增两端补丁版本和 Android code；可选 `-Target Receiver` 或 `-Target Android`，后者也递增接收端补丁版本以交付最新内嵌包。重复编译不重复升版。根目录 [version.properties](version.properties) 是唯一版本来源，详细的修改位置、强制交付步骤及 iOS 等未来端规划见 [版本管理](docs/versioning.md)。

该入口先运行版本规则测试、构建 Android 并运行 Kotlin 测试，核验 Gradle 元数据和 APK 内部版本，再从本次 Gradle 输出复制 `TapDeck-0.3.15.apk`、核对 SHA-256，然后运行 Go 测试、`go vet` 和 Windows 构建。接收端与 Android 版本可不同。构建后读取两个接收端 EXE 的 `--apk-info`，核对实际内嵌的版本、code、文件名、大小及哈希，生成 `release-manifest.json` 和 `SHA256SUMS.txt`。任何检查失败均终止，不沿用旧 APK。签名密钥不随源码分发；覆盖既有 Android 安装须使用相同签名。

Windows 生成 `TapDeck-0.3.15.exe`、`TapDeck-debug-0.3.15.exe` 和 `TapDeck-hidprobe-0.3.15.exe`；文件名、程序内版本与“属性 → 详细信息”中的文件版本／产品版本均自动取自接收端版本。无版本号的 EXE 及 `TapDeck-debug.apk` 作为兼容副本保留，对外交付使用带版本号文件。

Android 单独开发可用 [build-android.ps1](scripts/build-android.ps1)，模拟器矩阵可用 [test-android-ui.ps1](scripts/test-android-ui.ps1)。国际依赖连接失败时，先检查 Clash Verge 和 Anycast，Android 构建可加 `-UseLocalProxy` 使用本机 SOCKS5 1080。构建与内嵌规则见 [APK 分发说明](docs/apk-download.md)。

## 验证与当前范围

0.3.15 已通过 54 项 Kotlin 单元测试、29 项版本规则检查、Go 测试、`go vet` 和两端构建。PC 在 740×800、最小 680×700 逻辑窗口和 120 DPI 下通过连接、配对弹窗及语音布局检查；Android 普通屏和小屏 130% 字体共 10 项设备回归通过。配对拒绝、关闭／Esc、超时、断开、排队与已有凭据重连均有测试覆盖；交付 APK 与两个接收端 EXE 内嵌版本和 SHA-256 一致。截图、验证范围和真机待验收项见 [验证记录](docs/verification.md)。

当前 Android 使用竖屏布局，Windows 运行文件为未签名测试构建，APK 使用现有开发签名。用户已确认其手机在关闭系统震动开关后仍可使用 App 震动；其他机型表现、覆盖升级及原配对重连，以及手机语音转换成目标输入框文字的完整链路，按验证记录逐项验收。多设备共享键鼠，语音一次只由一台设备传送。

## 文档与许可

| 主题 | 文档 |
|---|---|
| 界面、灵敏度与震动 | [Android 主界面](docs/android-ui.md)、[全键盘](docs/full-keyboard.md) |
| 手势与窗口切换 | [触控板手势](docs/touchpad-gestures.md) |
| 配对、驱动与设备管理 | [安装引导](docs/installation.md)、[多个控制端](docs/multi-controller.md) |
| 快捷键与语音热键 | [支持的按键](docs/special-keys.md)、[语音热键](docs/voice-hotkey.md) |
| 自动连接 | [自动重连与后台自启](docs/auto-connect.md) |
| 版本及打包规则 | [版本管理与每轮必做事项](docs/versioning.md)、[APK 分发](docs/apk-download.md) |
| 验证及通信协议 | [验证记录](docs/verification.md)、[协议说明](protocol/README.md) |

TapDeck 源码采用 [MIT 许可证](LICENSE)。第三方组件按各自许可分发，详见 [第三方声明](THIRD_PARTY_NOTICES.md)。内嵌 VB-CABLE 来自 VB-Audio，适用其专有许可及 donationware 条件；程序保留来源、许可、donationware 说明和官网入口，可经官网捐赠或购买许可；完整交付包保留相关声明。
