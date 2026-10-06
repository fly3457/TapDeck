# TapDeck 0.3（Windows）/ 0.2（Android）

Kotlin Android 触控板与 Go Windows 接收端原型。手机通过 Wi-Fi 控制电脑鼠标、发送 1–8 个可配置快捷键，通过可拖动语音圆球以长按或免按方式把手机麦克风传到 Windows 虚拟麦克风。

项目源码采用 [MIT 许可证](LICENSE)，第三方组件说明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。源码仓库为 [fly3457/TapDeck](https://github.com/fly3457/TapDeck)，完整交付包（Windows 程序与 Android APK）在 [GitHub Releases](https://github.com/fly3457/TapDeck/releases) 下载。当前为原型版本，已通过与待人工验收的项目分别记录在验证文档中。

## 使用

1. 在 Windows 11 x64 启动 `dist/TapDeck.exe`。窗口关闭后保留托盘程序；从托盘选择“退出”才停止接收。可在设置页「设置与状态」勾选“随 Windows 登录自动启动接收端”，登录后自动在后台监听（等价命令 `TapDeck.exe --autostart-on` / `--autostart-off`）。
2. 在 Android 安装 `dist/TapDeck-debug.apk`，支持 Android 8/API 26 及以上。两端连接同一可互通的 Wi-Fi。
3. Android“连接”页输入 PC 窗口中的网址，例如 `http://192.168.1.11:41080/pair`。核对两端显示的校验码，在 Android 确认，在 PC 允许连接。成功后保存凭据，下次启动自动重连。
4. 也可以在 Android 浏览器访问这个网址，点击“打开 TapDeck 配对”。有摄像头的设备还可扫描二维码；二维码的自动授权凭证有效期 120 秒且只使用一次，过期后可刷新或改为两端核对校验码。
5. 配对成功后无需再操作：重开 Android App、重启接收端或电脑重启（且已开启自启）都会自动恢复连接；Android 端在启动、网络变化和回到前台时都会重试，间隔从 1 秒递增到 30 秒。详见 [`docs/auto-connect.md`](docs/auto-connect.md)。
6. PC“快捷键”页勾选 1–8 项，编辑名称和组合键，点击“保存并同步配置”。默认启用复制、粘贴、撤销、回车；新增四项默认关闭。启用项必须有名称和有效按键。支持左右 Ctrl / Alt / Shift / Win、字母、数字、F1–F24、Enter、Tab、Space、Esc、Backspace、方向键、翻页键、小键盘、标点键，以及音量加 / 减 / 静音。组合键既可直接输入，也可以用“录入”对话框或“单键选择”下拉；两者都会保留左右修饰键，特殊键使用统一名称（例如 `Backspace`、`Esc`、`VolumeUp`）。手机上的快捷键按钮按下即按下、松手即抬起：轻点是一次完整按键，**按住则组合键持续按下**（方向键、退格等会连续生效），断开连接时接收端会释放全部按键。详细支持范围与限制见 [`docs/special-keys.md`](docs/special-keys.md)。
7. 语音输入方式在顶部状态区「连接」旁边的开关里选择（默认长按语音输入）：圆形控件按住达到 300 ms 开始长按录音，松手结束；切到单击语音输入后为方形控件，轻点开始、再轻点结束，录音期间按任意快捷键会先结束录音。录音控件可以拖动调整位置。首次需要授予麦克风权限，授权后重新操作；拒绝权限不会影响键鼠。

Windows EXE 已内置原版签名的 **FakerInput 0.1.1 x64** 安装包和 MIT 许可证。PC“快捷键”页默认“自动”发送：有驱动时使用虚拟键盘；缺少驱动时显示“安装 / 修复虚拟键盘”入口，安装请求 Windows 管理员授权及必要的发布者确认，禁止自动重启。日常运行使用普通权限，正常退出不会卸载驱动。已有驱动不会在启动时重复安装。

豆包会忽略 SendInput 的软件注入事件，本机已验证虚拟键盘能触发长按和免按。**左右修饰键必须与豆包设置一致**：豆包显示“右 Ctrl + M / 右 Ctrl + L”时，分别填写 `RightCtrl+M`、`RightCtrl+L`、`RightCtrl+L`；`Ctrl` 指左 Ctrl。热键字段请用“单键选择”下拉或“录入”对话框填写，两者都会保留左右；手输 `Ctrl+M` 会解析成左 Ctrl，豆包不认。语音只在目标输入框获得焦点时触发，TapDeck 自己的窗口不算合格目标（详见 `docs/voice-hotkey.md`）。豆包关闭全局语音快捷键时，还需要先在目标输入框中选择豆包输入法。手机传音形成文字的最终联合验收仍待人工完成，详细结果见 `docs/virtual-keyboard.md`。

首次使用时，请在自己的 PC 安装 VB-CABLE 并允许 TapDeck 访问局域网；地址以程序窗口显示为准。无摄像头设备可使用网址配对。

## 麦克风与语音输入法

音频路径：`AudioRecord → 加密 UDP → 抖动缓冲 → WASAPI → CABLE Input → CABLE Output`。

- PC“语音”页的输出设备选择“自动选择 CABLE Input”或对应的 CABLE Input。
- 录音软件、会议软件或语音输入法的麦克风选择 **CABLE Output**。TapDeck 负责传音和发送热键，文字识别由所选的 PC 输入法完成。
- 两种手势同时可用。PC 分别配置“长按热键”“免按开始热键”“免按结束热键”：长按模式保持热键，免按模式在开、关时分别发送一次完整组合键。
- 三个热键默认留空，仅传音。每个按键字段的“单键选择”提供左 / 右 Alt、Ctrl、Shift；“录入”对话框会分别记录左右修饰键。豆包长按热键可选择“右 Alt”（保存为 `RightAlt`）或手输 `RightCtrl+M`；是否适合免按模式需依据输入法自身的开始 / 结束规则配置。普通 Alt / Ctrl / Shift 对应左侧键。
- 语音热键只在目标输入框获得焦点时有效，TapDeck 自己的窗口不是合格目标；此时实体键盘同样触发不了。本机实测与证据见 [`docs/voice-hotkey.md`](docs/voice-hotkey.md)。
- 正常结束最多排空 60 ms 音频，再等待尾音延迟后释放长按热键或发送免按结束热键。长按触摸取消、后台、锁屏、断线及录音错误立即停止采集并清空音频；免按拖动取消仅结束拖动。重新连接保持空闲，录音中修改热键从下一次录音生效。
- 没有虚拟声卡时，键鼠功能仍然可用，语音区显示明确错误。

VB-CABLE 由用户从 [官方页面](https://vb-audio.com/Cable/)单独安装，安装驱动需要 Windows 管理员权限。本项目的发布文件不捆绑该驱动。

## 主界面与手势

主界面覆盖应用视口，没有滚动容器。状态区 / 触控板 / 快捷键区 / 录音控件区按 **10% / 40% / 25% / 25%** 分配高度。比例集中在 `MainActivity.kt` 的 `Regions`；每个区的间距和安全留白包含在该区内。

- 单指移动鼠标；轻点左击；连续两次轻点双击。
- 第二次轻点后继续按住并移动，可拖拽，松手释放。
- 双指轻点右击；双指移动滚动。灵敏度与自然滚动方向在 PC 设置。
- 快捷键按原槽位顺序显示：1–4 个单行等宽；5–8 个四列两行，第二行不足四项保留空位。名称过长省略，按钮和文字随区域适配。
- 语音输入方式的开关在顶部状态区「连接」按钮旁（文字与「连接」同号），默认「长按语音输入」：触发控件是**圆形**，按住达到阈值开始、松手结束。切到「单击语音输入」：控件变成**方形**，轻点开始、再轻点结束；**单击（免按）录音期间按任意快捷键会先结束录音**。录音控件可以在语音区内任意拖动，位置保存在 Android 本地并适配屏幕变化。另一个手指可以同时操作触控板。
- 顶部状态栏右侧是语音输入方式开关与「全键盘」开关（互斥），再往右是「连接」。打开全键盘后，下方快捷键区与语音区整体换成一块全键盘（Ctrl / Alt / Win / Esc / Tab / 方向键 / 字母 / 符号 / 导航键），再次关闭即恢复。按键按下即按下、松手即抬起，按住会自动重复；Shift 为一次性修饰键。详见 [`docs/full-keyboard.md`](docs/full-keyboard.md)。
- 界面保留系统状态栏（时间、电量、Wi-Fi）；触控板为黑底白字，语音输入区为米黄背景。

## 构建

固定版本：Go 1.26.4、JDK 17、Android SDK Platform 37.0、Build Tools 36.0.0、Gradle 9.3.1、AGP 9.1.1。Android 采用 AGP 内置 Kotlin 2.2.10、Compose BOM 2025.04.01、OkHttp 5.5.0、Coroutines 1.10.2、Serialization 1.9.0、DataStore 1.1.7、ZXing Embedded 4.3.0。

Windows 使用 coder/websocket 1.8.15、go-ole 1.3.0、Walk、go-qrcode、x/sys/windows；准确版本与校验值在 `windows/go.mod` 和 `windows/go.sum`。WASAPI、SendInput 与 FakerInput HID 薄层客户端为本项目实现，Windows 构建无需 CGO 或额外键盘客户端 DLL。键盘工作进程使用同一 EXE 和继承的匿名管道；主进程异常退出时释放持有键并退出。快捷键点按保持 50 ms，键盘等待独立于鼠标处理。

驱动原包、固定哈希和许可证在 `windows/internal/driver/assets`。构建脚本与 EXE 解出安装包时均验证 SHA-256；安装前进行离线 Authenticode 验证。自动模式中 F13–F24 等描述符无法表示的按键使用 SendInput，强制 HID 模式会提示不支持。录音或按键执行期间禁止切换发送方式。

在项目根目录的 PowerShell 执行：

```powershell
.\scripts\build-windows.ps1
.\scripts\build-android.ps1 -JavaHome 'C:\path\to\jdk17' -SdkRoot "$env:LOCALAPPDATA\Android\Sdk"
.\scripts\install-android.ps1
# 同时连接多台设备时，使用 adb devices 查到的编号：
.\scripts\install-android.ps1 -Serial 'DEVICE_SERIAL'
```

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
.\dist\TapDeck-debug.exe --extract-keyboard-driver "$env:TEMP\TapDeckDriver"
# 需要安装时才执行，会请求 Windows 管理员授权：
.\dist\TapDeck-debug.exe --install-keyboard-driver
```

`audio-probe` 只采集 CABLE Output，输出样本数量与电平统计，不保存录音。配置、日志、设备凭据与运行统计位于 `%LOCALAPPDATA%\TapDeck`。Android 凭据使用 Android Keystore 加密后保存在 DataStore，PC 私钥由 DPAPI 保护。

PC 设置中的“解除配对”立即断开连接并废止长期凭据。Android 可在“连接”页忘记电脑。接收端保存端口、快捷键启用状态、设备、音量、灵敏度、三个语音热键及 `keyboard_backend`（`auto` / `hid` / `sendinput`）设置。

升级时需要同时更新 PC 和 Android 至 0.2（控制协议 v2）。PC 自动迁移旧四槽配置，并在 `%LOCALAPPDATA%/TapDeck/config.v1.bak` 保留原文件；旧长按或起停热键迁移到对应字段。已有配对凭据保留，Android 圆球位置在“忘记电脑”后仍保留。

此次 Windows 0.3 更新继续使用协议 v2，兼容现有 Android 0.2 APK；已有配置缺少 `keyboard_backend` 时默认自动，配置、热键及配对凭据保留。

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
