# TapDeck 0.3（Windows / Android）

Kotlin Android 触控板与 Go Windows 接收端原型。手机通过 Wi-Fi 控制电脑鼠标、发送 1–8 个可配置快捷键，通过可拖动语音圆球以长按或免按方式把手机麦克风传到 Windows 虚拟麦克风。

项目源码采用 [MIT 许可证](LICENSE)，第三方组件说明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。源码仓库为 [fly3457/TapDeck](https://github.com/fly3457/TapDeck)，完整交付包（Windows 程序与 Android APK）在 [GitHub Releases](https://github.com/fly3457/TapDeck/releases) 下载。当前为原型版本，已通过与待人工验收的项目分别记录在验证文档中。

## 使用

1. 在 Windows 11 x64 启动 `dist/TapDeck.exe`。窗口关闭后保留托盘程序；从托盘选择“退出”才停止接收。可在设置页「设置与状态」勾选“随 Windows 登录自动启动接收端”，登录后自动在后台监听（等价命令 `TapDeck.exe --autostart-on` / `--autostart-off`）。自启用 `--headless` 启动：**同样常驻托盘**，只是不弹出设置窗口，左键单击托盘图标即可打开设置。
2. 支持 Android 8/API 26 及以上。两端连接同一可互通的 Wi-Fi，用手机系统或浏览器扫码工具扫描 PC“连接”页的“扫码安装 Android 端”二维码，直接打开安装与配对网页。点击“下载 Android APK”，按系统提示安装。APK 已内嵌进单个 EXE，网页显示版本和完整 SHA-256，详见 [`docs/apk-download.md`](docs/apk-download.md)。
3. Android“连接”页输入 PC 窗口中的网址，例如 `http://192.168.1.11:41080/pair`。手机只显示校验码（不需要点确认），在 PC 设置窗口核对一致后点“校验码一致，允许”即可。成功后保存凭据，下次启动自动重连。**一台 PC 最多同时接 5 个控制端**（多台同时在线时状态行显示“已连接 N 个控制端：…”），详见 [`docs/multi-controller.md`](docs/multi-controller.md)。
4. 已安装用户在网页点击“打开 TapDeck 连接”，也可在 App 中手动输入网址。App 不再提供摄像头扫码入口。所有首次配对都必须由 PC 核对校验码并允许，旧链接里的 secret 不会自动授权；已有凭据仍可正常重连。
5. 配对成功后无需再操作：重开 Android App、重启接收端或电脑重启（且已开启自启）都会自动恢复连接；Android 端在启动、网络变化和回到前台时都会重试，间隔从 1 秒递增到 30 秒。详见 [`docs/auto-connect.md`](docs/auto-connect.md)。
6. PC“快捷键”页勾选 1–8 项，编辑名称和组合键，点击“保存并同步配置”。默认启用复制、粘贴、撤销、回车；新增四项默认关闭。启用项必须有名称和有效按键。支持左右 Ctrl / Alt / Shift / Win、字母、数字、F1–F24、Enter、Tab、Space、Esc、Backspace、方向键、翻页键、小键盘、标点键，以及音量加 / 减 / 静音。组合键既可直接输入，也可以用“录入”对话框或“单键选择”下拉；两者都会保留左右修饰键，特殊键使用统一名称（例如 `Backspace`、`Esc`、`VolumeUp`）。手机上的快捷键按钮按下即按下、松手即抬起：轻点是一次完整按键，**按住则组合键持续按下**（方向键、退格等会连续生效），断开连接时接收端会释放该会话持有的按键。详细支持范围与限制见 [`docs/special-keys.md`](docs/special-keys.md)。
7. 语音输入方式在语音区顶部的模式按钮里选择（默认长按语音输入）：按钮上是 Lucide 图标 + 模式名（长按＝`mic-audio-lines`、单击＝`mic-signal`），按钮右侧跟着当前提示（`按住说话，松手结束` / `轻点开始，再点结束`）。圆形控件按住达到 300 ms 开始长按录音，松手结束；切到单击语音输入后为方形控件，轻点开始、再轻点结束，录音期间按任意快捷键会先结束录音。录音控件可以拖动调整位置。首次需要授予麦克风权限，授权后重新操作；拒绝权限不会影响键鼠。

Windows EXE 已内置原版签名的 **FakerInput 0.1.1 x64** 安装包和 MIT 许可证。PC“快捷键”页默认“自动”发送：有驱动时使用虚拟键盘；缺少驱动时显示“安装 / 修复虚拟键盘”入口，安装请求 Windows 管理员授权及必要的发布者确认，禁止自动重启。日常运行使用普通权限，正常退出不会卸载驱动。已有驱动不会在启动时重复安装。

豆包会忽略 SendInput 的软件注入事件，本机已验证虚拟键盘能触发长按和免按。**左右修饰键必须与豆包设置一致**：豆包显示“右 Ctrl + M / 右 Ctrl + L”时，分别填写 `RightCtrl+M`、`RightCtrl+L`、`RightCtrl+L`；`Ctrl` 指左 Ctrl。热键字段请用“单键选择”下拉或“录入”对话框填写，两者都会保留左右；手输 `Ctrl+M` 会解析成左 Ctrl，豆包不认。语音只在目标输入框获得焦点时触发，TapDeck 自己的窗口不算合格目标（详见 `docs/voice-hotkey.md`）。豆包关闭全局语音快捷键时，还需要先在目标输入框中选择豆包输入法。手机传音形成文字的最终联合验收仍待人工完成，详细结果见 `docs/virtual-keyboard.md`。

首次打开 PC 设置窗口时，缺少 VB-CABLE 会提示安装；可稍后从“语音”页安装并重新检测。开机自启只检测状态，打开设置后才提示安装。请允许 TapDeck 访问局域网，地址以程序窗口显示为准。

## 麦克风与语音输入法

音频路径：`AudioRecord → 加密 UDP → 抖动缓冲 → WASAPI → CABLE Input → CABLE Output`。

- PC“语音”页的输出设备选择“自动选择 CABLE Input”或对应的 CABLE Input。
- 录音软件、会议软件或语音输入法的麦克风选择 **CABLE Output**。TapDeck 负责传音和发送热键，文字识别由所选的 PC 输入法完成。
- 两种手势同时可用。PC 分别配置“长按热键”“免按开始热键”“免按结束热键”：长按模式保持热键，免按模式在开、关时分别发送一次完整组合键。
- 三个热键默认留空，仅传音。每个按键字段的“单键选择”提供左 / 右 Alt、Ctrl、Shift；“录入”对话框会分别记录左右修饰键。豆包长按热键可选择“右 Alt”（保存为 `RightAlt`）或手输 `RightCtrl+M`；是否适合免按模式需依据输入法自身的开始 / 结束规则配置。普通 Alt / Ctrl / Shift 对应左侧键。
- 语音热键只在目标输入框获得焦点时有效，TapDeck 自己的窗口不是合格目标；此时实体键盘同样触发不了。本机实测与证据见 [`docs/voice-hotkey.md`](docs/voice-hotkey.md)。
- 正常结束最多排空 60 ms 音频，再等待尾音延迟后释放长按热键或发送免按结束热键。长按触摸取消、后台、锁屏、断线及录音错误立即停止采集并清空音频；免按拖动取消仅结束拖动。重新连接保持空闲，录音中修改热键从下一次录音生效。
- 没有虚拟声卡时，键鼠功能仍然可用，语音区显示明确错误。

Windows EXE 内嵌 **VB-CABLE Pack45 完整官方原包**，SHA-256 固定校验，解出全部文件后验证安装程序和 Windows 10 x64 驱动目录签名，以管理员身份打开官方安装向导。安装完成后须按 [官方说明](https://vb-audio.com/Cable/VBCABLE_ReferenceManual.pdf)重启 Windows，再确认两个音频端点可用；TapDeck 不自动重启或更改默认麦克风。VB-CABLE 来自 [VB-Audio](https://vb-audio.com/Cable/)，是 donationware；语音页保留原包许可和[捐赠 / 购买入口](https://vb-audio.com/Services/licensing.htm)。安装、分发条件与验收范围见 [`docs/installation.md`](docs/installation.md)。

## 主界面与手势

主界面覆盖安全视口，没有滚动容器。以视口宽度 `W` 计算：顶部连接栏高 **10% W**，底部内容高 **60% W**、上下各留 **1% W**；非全键盘模式的快捷键与语音各占 **30% W**，触控板填满剩余高度。两种模式共用底部容器，切换后边界一致；短窗口统一缩小纵向尺寸。完整尺寸与样式见 [`docs/android-ui.md`](docs/android-ui.md)。

- 单指移动鼠标；首次轻点等待 300 ms 后左击。第二次按下立即按住左键，快速松手后才形成双击；继续按住或移动则拖拽，松手释放。
- 触控板左上角的滑块图标打开灵敏度设置：默认 **1.0×**，范围 **0.5×–3.0×**，步长 **0.1**，调节立即生效，各 Android 设备独立保存；1.0× 对应此前 PC 灵敏度 2 的速度，可一键恢复默认。仅影响鼠标移动，滚动和缩放不变。
- 双指轻点右击；双指移动滚动，张开／合拢通过 Ctrl＋滚轮缩放。自然滚动开启时，手指上滑让内容上移；滚动方向仍在 PC 设置。
- 三指上滑打开 Windows 任务视图，下滑关闭；普通窗口下滑显示桌面，再上滑恢复原窗口。新手势需两端更新至 0.3.3，详见 [`docs/touchpad-gestures.md`](docs/touchpad-gestures.md)。
- 快捷键按原槽位顺序显示：1–4 个单行等宽；5–8 个四列两行，第二行不足四项保留空位。名称过长省略，按钮和文字随区域适配。
- 快捷键、全键盘及语音触发控件按下时轻震，长按功能生效时再反馈一次，不在松手或连续按住时反复震动。手机顶部连接图标 →“连接与设备设置”顶部提供“按键震动反馈”开关、“测试震动”与手机系统振动设置入口；开关默认开启、独立保存在当前设备，PC“设置与状态”标出入口位置。0.3.5 改用振动服务的设备调校效果并保留旧系统回退，遵循系统触感设置与硬件能力。
- 语音模式按钮在语音区顶部，采用与按键一致的圆角白底样式，包含 Lucide 图标与模式名（长按＝`mic-audio-lines`，单击＝`mic-signal`），右侧显示操作提示。默认长按为圆形，按住达到 300 ms 开始、松手结束；单击模式为方形，轻点开始、再点结束，录音中按快捷键只结束录音。控件可拖动、位置与模式记在本地，另一个手指可以同时操作触控板。
- 顶部 Lucide `keyboard` 图标（未激活 `#AAA`、激活 `#000`）切换全键盘与快捷键＋语音输入，两种模式的底部高度一致。0.3.6 将全键盘普通键宽改为安全视口宽度的 **8.9%**，键间距、行间距和四周留白均为 **1%**，保留四行双键位、第二行居中和特殊键网格跨度。短按只发主键位一次、长按只发副键位一次；Shift 短按单次大写、长按锁定，退格 / Ctrl / 回车 / `Shift+Enter` 长按保持按住；空格短按一次空格、长按语音输入。副键提示与主键整体居中，功能键统一使用 Lucide 矢量图标，详见 [`docs/full-keyboard.md`](docs/full-keyboard.md)。
- Android 应用图标沿用 PC 的蓝底 `#175CD3` 与白色方块；提供普通、圆形自适应及单色主题资源，适配系统桌面蒙版。
- 界面保留系统状态栏（时间、电量、Wi-Fi）；触控板黑底白字，底部键盘、快捷键与语音区统一为浅灰 `#DCDDDF`，普通键白色、特殊键 `#B6B9C2`，采用浅色细边、圆角与底部阴影。连接入口为 Lucide `plug` 图标：未连接时橙色、每 3 秒跳动一次，连接后绿色静止。空格上的麦克风使用 `mic-audio-lines`。**语音输入方式与全键盘开关都记在 Android 本地**（DataStore），重开 App 沿用。[Lucide 原始许可](licenses/Lucide-LICENSE.txt)同时保留在 APK 内。

## 构建

固定版本：Go 1.26.4、JDK 17、Android SDK Platform 37.0、Build Tools 36.0.0、Gradle 9.3.1、AGP 9.1.1。Android 采用 AGP 内置 Kotlin 2.2.10、Compose BOM 2025.04.01、OkHttp 5.5.0、Coroutines 1.10.2、Serialization 1.9.0、DataStore 1.1.7；已移除 ZXing 和摄像头权限。

Windows 使用 coder/websocket 1.8.15、go-ole 1.3.0、Walk、go-qrcode、x/sys/windows；准确版本与校验值在 `windows/go.mod` 和 `windows/go.sum`。WASAPI、SendInput 与 FakerInput HID 薄层客户端为本项目实现，Windows 构建无需 CGO 或额外键盘客户端 DLL。键盘工作进程使用同一 EXE 和继承的匿名管道；主进程异常退出时释放持有键并退出。快捷键点按保持 50 ms，键盘等待独立于鼠标处理。

驱动原包、固定哈希和许可证在 `windows/internal/driver/assets`。构建脚本与 EXE 解出安装包时均验证 SHA-256；安装前进行离线 Authenticode 验证。自动模式中 F13–F24 等描述符无法表示的按键使用 SendInput，强制 HID 模式会提示不支持。录音或按键执行期间禁止切换发送方式。

在项目根目录的 PowerShell 执行：

```powershell
.\scripts\build-windows.ps1 -JavaHome 'C:\path\to\jdk17' -SdkRoot "$env:LOCALAPPDATA\Android\Sdk"
# 接收端正在运行或需要独立发布目录时：
.\scripts\build-windows.ps1 -OutputDirectory 'dist\0.3.6'
.\scripts\install-android.ps1
# 同时连接多台设备时，使用 adb devices 查到的编号：
.\scripts\install-android.ps1 -Serial 'DEVICE_SERIAL'
```

发布统一通过 `build-windows.ps1`：每次先调用 Android 的 `assembleDebug` 和 `testDebugUnitTest`，成功后只从该次 Gradle 输出复制 APK、核对 SHA-256，生成版本清单，再执行 Go 测试、`go vet` 和 Windows 构建。Android 失败、APK 缺失或哈希不一致立即终止，不使用旧 `dist` APK。`build-android.ps1` 保留给 Android 独立开发。Android 包名保持 `com.yuncii.tapdeck`，版本 `0.3.6` / code `9`；使用现有开发签名，不更换签名密钥。

请预先安装 JDK 17，传入 `-JavaHome` 或设置 `JAVA_HOME`；本地 `.tools/jdk17` 中已有的 JDK 也可被脚本自动找到，该目录不随源码发布。首次安装 SDK 可使用 SDK Manager 安装 `platforms;android-37.0`、`build-tools;36.0.0` 和 `platform-tools`。本机的 `android/local.properties` 不提交到 Git。

国际依赖访问失败时，先检查 Clash Verge 与 Anycast，再使用 `build-android.ps1 -UseLocalProxy` 通过本机 SOCKS5 1080。Go 可在当前进程设置 `HTTP_PROXY/HTTPS_PROXY/ALL_PROXY=socks5h://127.0.0.1:1080`；若校验服务访问失败，可用 `GOPROXY=https://goproxy.cn,direct`，保留校验。

本机 JDK 的 AF_UNIX 回环连接存在兼容问题，Android 构建脚本使用不存在的 Unix socket 目录触发 JDK 的 TCP 回退；不会修改系统网络或持久环境变量。

## 防火墙与运行方式

默认端口：HTTP `41080`、WSS `41443`、UDP `41444`。配对页面仅提供连接引导与公开指纹；输入事件和配对凭据走 WSS，鼠标位移及音频走 AES-GCM UDP。

Windows 首次运行可按系统防火墙提示允许访问本地网络。也可在管理员 PowerShell 执行：

```powershell
.\scripts\setup-firewall.ps1
# 程序移到其他位置后：
.\scripts\setup-firewall.ps1 -ExecutablePath 'C:\Apps\TapDeck\TapDeck.exe'
```

此脚本只针对 TapDeck 可执行文件开放本地子网的 TCP/UDP 接入；移动 EXE 后请重新执行此脚本。

```powershell
.\dist\TapDeck-debug.exe --headless
.\dist\TapDeck-debug.exe --list-audio
.\dist\TapDeck-debug.exe --audio-probe 5
.\dist\TapDeck-debug.exe --keyboard-status
.\dist\TapDeck-debug.exe --apk-info
.\dist\TapDeck-debug.exe --cable-status
.\dist\TapDeck-debug.exe --extract-cable "$env:TEMP\TapDeckCable"
.\dist\TapDeck-debug.exe --extract-keyboard-driver "$env:TEMP\TapDeckDriver"
# 需要安装时才执行，会请求 Windows 管理员授权：
.\dist\TapDeck-debug.exe --install-keyboard-driver
```

`audio-probe` 只采集 CABLE Output，输出样本数量与电平统计，不保存录音。配置、日志、设备凭据与运行统计位于 `%LOCALAPPDATA%\TapDeck`。Android 凭据使用 Android Keystore 加密后保存在 DataStore，PC 私钥由 DPAPI 保护。

PC“连接”页列出已配对设备的名称、在线状态、设备标识和最近连接时间。“解除所选设备配对”确认目标后仅废止该设备凭据，并释放它持有的按键、鼠标按钮和语音热键；“解除全部配对”用于清空所有授权。保存失败时保留原授权。Android 收到 `pairing_revoked` 会清除这台电脑的凭据并停止自动重试，需要用户重新连接并在 PC 允许。Android 也可在“连接”页忘记电脑。

从控制协议 v1 升级时需要同时更新 PC 和 Android 至支持 v2 的版本。PC 自动迁移旧四槽配置，并在 `%LOCALAPPDATA%/TapDeck/config.v1.bak` 保留原文件；旧长按或起停热键迁移到对应字段。已有配对凭据保留，Android 圆球位置在“忘记电脑”后仍保留。

两端 0.3 继续使用控制协议 v2，保留已有配置、热键和配对凭据。旧 `paired.json`（设备 ID → 令牌哈希）首次加载时备份为 `paired.v1.bak`，迁移为带名称和时间的格式；旧设备名暂显示“旧设备（标识末 8 位）”，重连后补齐。旧 Android 0.2 仍可连接，但自动处理解绑通知需要 Android 0.3。

## 验证与范围

测试代码覆盖 Go/Kotlin 的共同 AES-GCM 编码向量、篡改拒绝、重复包与乱序、鼠标跨通道拖拽边界、旧录音隔离、缓冲上限、时钟漂移、配对确认、重连、解绑和心跳超时。

真机测试及性能结果见 `docs/verification.md`。设备测试可运行：

```powershell
cd android
.\gradlew.bat testDebugUnitTest assembleDebugAndroidTest
adb -s DEVICE_SERIAL install -r app\build\outputs\apk\androidTest\debug\app-debug-androidTest.apk
adb -s DEVICE_SERIAL shell am instrument -w -e class com.yuncii.tapdeck.DeviceTest com.yuncii.tapdeck.test/androidx.test.runner.AndroidJUnitRunner
# 单独运行 30 分钟测试：
adb -s DEVICE_SERIAL shell am instrument -w -e class 'com.yuncii.tapdeck.DeviceTest#wifiAudioAndMouseSoak' -e soakSeconds 1800 com.yuncii.tapdeck.test/androidx.test.runner.AndroidJUnitRunner
```

当前覆盖单 PC、单 Android、前台长按 / 免按传音和普通桌面应用。鼠标及 SendInput 受 Windows 权限隔离影响；系统 UAC 界面交由用户操作。APK 使用开发签名，Windows EXE 为未签名的本机构建原型，内置 MSI 保留上游有效签名。USB 仅用于安装与调试，业务数据通过 Wi-Fi 传输。

`dist/TapDeck-prototype.zip` 包含运行文件、使用及验证文档；构建说明对应本项目的完整源码目录。

协议格式、消息边界和安全约束见 `protocol/README.md`。
