# TapDeck 验证记录

测试日期：2026-10-06 至 2026-10-09。使用页描述当前行为；历史结果按当时版本理解。截图中的示例连接和录音状态不作为实机识别证明。

当前：接收端 **0.3.18**、Android **0.3.18 / code 21** · [安装](installation.md) · [版本规则](versioning.md)

## 0.3.18 键盘间隙触控、语音提示与文档整理

2026-10-09 以 0.3.17 提交为基线。Android 保留原 1% 视觉间隙和键面尺寸，触控范围扩展到相邻间隙中线；左右及上下相邻键平分，中线归右／下，四周和第二行居中留白保留。间隙中的触摸沿用短按、副键长按、多指修饰键及断线释放。

PC“语音快捷键设置”的名称长度提示单独一行，以下热键说明另占一行：

> 热键：在手机端激活语音时触发，一般设置为PC端的语音输入法快捷键，留空则只传输音频。

README 简化为功能列表、首次使用、截图和致谢，补充 FakerInput、VB-CABLE 用途与可选安装、官方来源、豆包输入法官网。各使用和开发文档统一当前行为、移除过时的当前版本表述；保留历史实测、截图和已公开链接。第三方许可证原文未修改。

| 验证项 | 结果 |
|---|---|
| 几何与 JVM | 60 项、0 失败／错误。新增中线与触控范围测试；覆盖多种宽度、受限高度、0–1024 px 极小／非整齐尺寸，键面包含、无重叠、外边距保持 |
| 真实触摸注入 | 每种布局验证 29 个横向间隙的两侧、Q/W 精确中线、所有行间隙邻近键中心；各点击只产生一对按下／释放和一次反馈。副键长按、多指 Ctrl+C、退格长按后断线及不可用状态通过；输入回调使用测试后端，不向桌面注入 |
| Android 五种矩阵 | API 34 隔离模拟器：1080×2400 / 420 dpi / 100% 字体 20 项；720×1280 / 300 dpi / 130% 6 项；640×960 / 320 dpi / 130% 11 项；1404×1872 / 300 dpi / 100% 6 项；1080 宽、应用高度 600 px 的受限视口 11 项，共 54 项通过。含零／一／多台、同名／长名称管理、录音三阶段和扫码／外部 Intent 限制 |
| 测试重试 | 首轮平板动画测试误把飞行中的图标当作静止基线，改为先读取基线并同时观察基线与跳动；平板 6 项和受限视口 11 项重跑通过。多 PC 首轮备注框取节点过早，改为等待可见输入框；另一次冷启动重跑在输入持有阶段发生心跳超时，同一已启动模拟器完整重跑 3 项通过，失败日志保留为 `.tools/ui-validation/multipc-0.3.18-cold.log`。未改变产品动画、心跳阈值或窗口尺寸 |
| 多 PC 真实协议 | 专项 3 项通过：真实 Keystore 迁移和失败保留、管理界面、三个独立证书／配置／凭据接收端的 HTTP、固定指纹 WSS 与加密 UDP。覆盖 A→B→A、快速 A→B→C、离线目标重试、恢复和免确认重连、单独撤销及重新授权、输入与语音配置归属；音频与输入为测试后端 |
| PC 原生页面 | 生产页面构造函数在 740×800、最小 680×700 窗口各验证快捷键及语音，共 4 个用例通过，实际 96 dpi。新说明完整单行，名称提示独立；分组、音频操作行、启用／名称／类型同排、禁用收起热键及隐藏值保留均通过 |
| 官方构建 | `scripts/build-windows.ps1 -OutputDirectory dist/0.3.18` 通过 29 项版本规则检查、Android 构建、60 项 JVM 测试、Go 测试、vet 及三个 EXE。沿用脚本对会实际修改本机音量和开机自启注册表两项测试的排除 |
| 版本与内嵌 | 本轮仅执行一次 `-Target Android`，两端升至 0.3.18 / Android code 21。三个 EXE 文件／产品版本为 0.3.18；正式与诊断 EXE 实际运行 `--apk-info`，均内嵌 Android 0.3.18 / code 21，文件名、大小、SHA-256 匹配。清单全部 8 个版本／兼容产物的大小和哈希复核通过 |
| APK 与覆盖安装 | aapt 确认包名 `com.yuncii.tapdeck`、versionName 0.3.18、code 21、min API 26；apksigner 验证通过。继续使用开发签名，证书 SHA-256 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。Gradle、交付及内嵌 APK 哈希一致，模拟器覆盖安装成功 |

### 产物与日志

官方交付目录 `dist/0.3.18`，包含带版本号 EXE／APK、两个 `.apk-info.json`、`release-manifest.json` 和 `SHA256SUMS.txt`。最终构建后仅调整测试基线取样、文档和截图，生产代码未再变化。

| 产物 | 字节数 | SHA-256 |
|---|---:|---|
| `TapDeck-0.3.18.apk` | 11,507,764 | `bc39d948ad5ad7ee36b0e0c66d5ef8d8b09958a848be9f423a7cf5787e51b475` |
| `TapDeck-0.3.18.exe` | 26,668,032 | `253716eb6534fdda4b712a733ddb6b5d206e7c8ee4be35b556a7edfc7dbcd8cd` |
| `TapDeck-debug-0.3.18.exe` | 32,177,152 | `55ce0ee9f49cd3a6eb5583ef1824df586294d31980127631cb3135540dd9003e` |
| `TapDeck-hidprobe-0.3.18.exe` | 8,048,128 | `420fe1d005247a20557a9e18b4ca0d7b085a52081414d69fc6078af90ee5dcbc` |

日志：`.tools/ui-validation/windows-0.3.18-build.log`、`pc-0.3.18-ui.log`、`android-0.3.18-ui.log`、`android-0.3.18-ui-retry.log`、`android-0.3.18-multipc-warm.log`；各布局结果见 `dist/0.3.18/screenshots/*.log`，三接收端专项见 `dist/0.3.18/multipc/{android,fixture}.log`。

### 当前截图与待验收

[PC 默认语音](screenshots/0.3.18/pc-voice-740x800-96dpi-top.png) · [最小语音](screenshots/0.3.18/pc-voice-680x700-96dpi-top.png) · [取消启用](screenshots/0.3.18/pc-voice-680x700-96dpi-disabled.png) · [默认快捷键](screenshots/0.3.18/pc-shortcuts-740x800-96dpi.png) · [最小快捷键](screenshots/0.3.18/pc-shortcuts-680x700-96dpi.png)

[手机键盘](screenshots/0.3.18/phone-100-keyboard.png) · [手机大字体](screenshots/0.3.18/phone-130-keyboard.png) · [小屏大字体](screenshots/0.3.18/small-130-keyboard.png) · [平板](screenshots/0.3.18/tablet-100-keyboard.png) · [受限高度](screenshots/0.3.18/restricted-100-keyboard.png)

[手机开始](screenshots/0.3.18/phone-100-voice-toggle-idle.png) · [手机结束](screenshots/0.3.18/phone-100-voice-toggle-transmitting.png) · [小屏开始](screenshots/0.3.18/small-130-voice-toggle-idle.png) · [多 PC 列表](screenshots/0.3.18/phone-100-multipc-many.png) · [录音切换限制](screenshots/0.3.18/small-130-multipc-recording-disabled.png)

未连接实体手机，本轮仍待：实体设备间隙操作和覆盖升级、一部手机／两台真实 PC 反复切换、实际 AudioRecord → CABLE Output → 输入法识别、干净系统驱动安装及 PC 高 DPI 布局。历史实机结果保留在下方，不能代替这些当前验收。

测试后模拟器尺寸、密度和字体恢复至 1080×2400 / 420 dpi / 100%，三个临时接收端及隔离 AVD 已退出。

### 公开发布

2026-10-09 已公开 [v0.3.18 Release（测试版）](https://github.com/fly3457/TapDeck/releases/tag/v0.3.18)。标签指向实现、版本、文档和截图提交 [3f5aaee](https://github.com/fly3457/TapDeck/commit/3f5aaeedbf3a1f63f66d6e8dc4180b8b3944ae26)，GitHub 公开时间为 `2026-10-08T18:53:13Z`。

上传 11 项资产：四个带版本号的 EXE／APK、完整 ZIP、两个内嵌信息 JSON、版本清单、校验文件、LICENSE 和第三方声明。全部 GitHub 资产的大小与服务端 SHA-256 均逐项匹配本地；远端 main、标签及标签指向的提交核对通过。对外清单位于 `dist/0.3.18/release`，记录源提交并去除本地兼容副本；官方原始构建清单保持不变。

另从公开 Release 下载 APK、版本清单和校验文件，内容与本地一致；Windows 直达下载链接最终返回 HTTP 200。下载 APK 的 SHA-256 与上表、两个 EXE 内嵌包一致。

[完整 ZIP](https://github.com/fly3457/TapDeck/releases/download/v0.3.18/TapDeck-0.3.18-windows-android.zip) 为 55,702,021 字节，SHA-256 `66b26316e4d624851d460fb6d362f7d7378ad2cd3d7ee6b7e8ace7df28a27511`，包含四个版本包和原始许可证；打开 ZIP 逐项核验四个包的大小／哈希及许可文件。独立下载见 [Windows](https://github.com/fly3457/TapDeck/releases/download/v0.3.18/TapDeck-0.3.18.exe)、[Android](https://github.com/fly3457/TapDeck/releases/download/v0.3.18/TapDeck-0.3.18.apk)。

## 历史验证

<details>
<summary>展开 0.3.17 及更早版本的原始记录</summary>

## 0.3.17 PC 输入设置分组与 Android 单击语音文案

2026-10-09 以已提交的 0.3.16 为基线完成。PC“快捷键”页分为“键盘环境”“快捷键设置”，键盘发送方式、虚拟键盘检测与安装集中在上方。PC“语音”页分为“语音输入环境”“语音快捷键设置”：音频路由单行显示，VB-CABLE 就绪与 donationware／VB-Audio 来源同排，最后一行为输入电平、音频设备选择、刷新及系统音频输入设置；设备全名不再重复显示在电平前，诊断状态保留在电平提示中。

每组语音配置将启用、名称和类型放在一行，移除折叠箭头；勾选展开热键，取消勾选只收起热键，名称和类型仍可编辑。隐藏字段和切换类型前的热键保留，“触发热键”改为“长按热键”。Android 单击方块空闲显示“开始”，准备、传音、停止阶段显示“结束”，禁用状态继续显示“禁用”。语音手势和控制协议保持原规则。

| 验证项 | 结果 |
|---|---|
| PC 原生界面 | `TestKeyboardSectionsUI`、`TestVoiceProfileEditorUI` 使用正式页面构造函数，在默认 740×800 和最小 680×700 窗口各完成一次，共 4 个窗口用例通过；实际 DPI 为 96。分组无重叠，八个快捷键横排及语音启用／名称／类型同排，音频操作行未越界；就绪状态和路由提示完整单行显示，未把最小窗口扩大 |
| 表单行为 | 取消启用只隐藏热键，名称和类型仍可编辑；重新启用恢复对应类型字段。类型往返保留长按／开始／结束热键，所有组关闭后仍保留名称和隐藏字段；默认第三组未启用，滚动后可访问 |
| Android 回归 | API 34 隔离模拟器：1080×2400 / 420 DPI / 字体 100%、640×960 / 320 DPI / 字体 130%，各通过配置轮换／禁用／录音冻结、单击停止消费鼠标点击、协议请求快照与空格语音 3 项，共 6 项通过。配置回归复核当前阶段及电平，截图前等待界面绘制完成 |
| 截图检查 | 检查两种 PC 尺寸的快捷键与语音页面，以及语音全部关闭后的表单。普通屏、小屏大字体下，方块“开始／结束”完整显示；准备为橙色、传音为绿色并显示 42% 电平、停止为灰色，阶段截图正确区分 |
| 构建与基础回归 | 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.17` 完成 29 项版本规则检查、59 项 Kotlin JVM 测试、Go 测试、`go vet` 和三个 Windows EXE 构建。沿用构建脚本对实际修改本机音量、开机自启注册表两个用例的排除；测试 APK 使用项目 JDK 17 构建成功 |
| 版本与内嵌 | 一次 `-Target All` 升版后接收端 0.3.17、Android 0.3.17 / code 20。三个 EXE 的文件／产品版本、两个接收端 `--apk-info`、APK 内部版本均一致；最终核对清单全部 8 个产物的大小／SHA-256，以及 Gradle、交付和内嵌 APK 哈希 |
| 签名与升级 | `apksigner verify --print-certs` 通过，继续使用原开发签名，证书 SHA-256 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`；隔离模拟器覆盖安装成功 |

本地交付目录 `dist/0.3.17`，包含带版本号产物、两个 `.apk-info.json`、`release-manifest.json` 和 `SHA256SUMS.txt`。构建成功后未修改生产代码；仅补充截图同步等待及文档。

| 产物 | 字节数 | SHA-256 |
|---|---:|---|
| `TapDeck-0.3.17.apk` | 11,507,764 | `e4ef17a34e15193b8dcca9ccbe9a58f622c686197276893e01bb04f8ef10689b` |
| `TapDeck-0.3.17.exe` | 26,667,520 | `ef69e5b4ed7492b70a41169ef3e80c6d9461bff2439e76efd2d2d52c20f2a9ed` |
| `TapDeck-debug-0.3.17.exe` | 32,177,152 | `d0f75a05e115732f25d85df41904fcb2943835cde4adfe1836d2dc412c7ebc22` |
| `TapDeck-hidprobe-0.3.17.exe` | 8,048,128 | `f282b1576a82925c162ef2dc8f33c92214198263ccfc03779116c93cfab119de` |

构建日志 `.tools/ui-validation/windows-0.3.17-build.log`，PC 原生界面日志 `.tools/ui-validation/pc-0.3.17-ui.log`，Android 日志 `.tools/ui-validation/android-0.3.17-ui.log` 和 `dist/0.3.17/screenshots/{phone-100,small-130}.log`。本轮只重新运行普通屏和小屏大字体的语音回归，0.3.16 的完整矩阵及多 PC 链路结果保留在下面的历史记录。

代表性截图：[默认快捷键](screenshots/0.3.17/pc-shortcuts-740x800-96dpi.png)、[最小快捷键](screenshots/0.3.17/pc-shortcuts-680x700-96dpi.png)、[默认语音](screenshots/0.3.17/pc-voice-740x800-96dpi-top.png)、[最小语音](screenshots/0.3.17/pc-voice-680x700-96dpi-top.png)、[取消启用后的表单](screenshots/0.3.17/pc-voice-680x700-96dpi-disabled.png)、[默认第三组](screenshots/0.3.17/pc-voice-740x800-96dpi-defaults-bottom.png)、[手机开始](screenshots/0.3.17/phone-100-voice-toggle-idle.png)、[手机结束](screenshots/0.3.17/phone-100-voice-toggle-transmitting.png)、[小屏开始](screenshots/0.3.17/small-130-voice-toggle-idle.png)、[小屏准备](screenshots/0.3.17/small-130-voice-toggle-preparing.png)、[小屏传音](screenshots/0.3.17/small-130-voice-toggle-transmitting.png)、[小屏停止](screenshots/0.3.17/small-130-voice-toggle-stopping.png)。PC 截图为独立原生页面及注入的就绪状态，Android 连接与录音状态为测试数据，不作为实际设备检测、传音或输入法识别证明。

本轮没有已连接的实体手机，未重复真实 AudioRecord → CABLE Output → 输入法识别、两台真实 PC 切换及干净系统驱动安装。实体手机覆盖升级与上述链路继续列为待验收；PC 高 DPI 布局本轮未验收。模拟器尺寸、密度和字体已恢复至 1080×2400 / 420 DPI / 100%，隔离 AVD 已关闭。本轮只生成本地交付包，公开 0.3.6 Release 链接保持不变。

## 0.3.16 Android 多 PC 配对与顶部切换

2026-10-08 以已提交的 0.3.15 配对流程为基线完成。Android 保存多台 PC，顶部名称展开可滚动列表，当前目标置顶；支持备注名、清空备注恢复系统名称和单独忘记。新配对失败、拒绝、过期或取消保留原有电脑；未完成的新配对不在重启后自动恢复。目标离线只重试该电脑，授权撤销清空对应凭据并保留“需重新配对”条目。录音准备、传音、停止处理中禁止改变连接，扫码与外部链接执行同样检查，不延后自动切换。

多电脑目录继续使用 Android Keystore AES-GCM 加密，以规范化的 TLS 公钥指纹标识 PC；旧单条凭据、语音组和旧模式在一个 DataStore 事务内迁移并移除旧键，失败保留原数据。手机 device ID、灵敏度、震动、键盘模式和语音控件位置保持。语音组选择按 PC 记忆，快捷键／能力随当前会话更新。切换清除旧手势、键盘和快捷键持有、旧配置与 UDP 队列；UI 回调、网络事件、录音线程、重连及异步存储按身份和会话代次隔离。沿用协议 v2 和 PC 授权机制。

| 验证项 | 结果 |
|---|---|
| 目录与升级迁移 | 新增 5 项 Kotlin 目录测试覆盖迁移、指纹规范化、同指纹更新地址／凭据、同名不同身份、备注与语音独立、忘记、旧凭据撤销匹配及目录校验。API 34 实际 Keystore 测试覆盖加密旧数据迁移、device ID／灵敏度保留、无明文令牌、事务抛错和损坏密文保留原始数据；未模拟设备存储耗尽 |
| 多电脑专项 | `scripts/test-android-multipc.ps1` 的 3 项 Android 测试全部通过。脚本创建三个独立证书、配置及配对文件的临时 Go 接收端，通过真实 HTTP、固定指纹 WSS 和加密 UDP 通信；输入与音频采用测试后端，不操作桌面或实际输出音频 |
| 切换与撤销 | 链路覆盖 A→B→A、快速连续 A→B→A 和 A→B→C 最后选择生效、B 停服时保留目标／仅重试 B、服务恢复、原凭据免确认重连、A 撤销不影响 B、撤销状态重启恢复、新令牌重新授权且备注保留、忘记当前及非当前项、拒绝／到期／取消后原列表保留及新 ViewModel 恢复目标 |
| 输入与语音隔离 | 真实链路验证持有 Shift、Ctrl、鼠标按钮及快捷键时切换，旧接收端释放且无活动会话；旧 UI 回调不会在新 PC 产生输入，新的键盘／UDP 鼠标只到当前接收端；两台 PC 的配置和语音选择独立。补充采音回调测试，旧会话或旧录音 ID 的帧不能改变新录音电平／队列。既有拒绝后迟到回调、语音停止和按键取消回归通过 |
| UI 尺寸矩阵 | API 34 隔离模拟器：1080×2400／420 dpi／100% 字体 19 项，720×1280／300 dpi／130% 5 项，640×960／320 dpi／130% 10 项，1404×1872／300 dpi／100% 5 项，1080 宽且应用高度限制 600 px 的视口 10 项，共 49 项通过。每种配置包含零／一／多台、同名与长名称、选择器和管理页滚动、备注、单独忘记、录音三个阶段的扫码及外部 Intent 限制；拒绝的外部链接立即消费，页面重建无链接可重放 |
| 扫码入口 | 普通屏和小屏 130% 字体各 1 项，共 2 项通过：扫码仅回填，取消保留地址，无效网址提示，GitHub Intent 和管理入口可见。使用系统 Activity 测试拦截返回，不代表真实相机扫码已验收 |
| 构建与检查 | 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.16` 通过：29 项版本规则检查、59 项 Kotlin JVM 测试（0 失败／错误）、Go 测试、`go vet`、APK 与三个 EXE 构建。保留脚本对实际改变系统音量和开机自启注册表两项测试的排除 |
| 版本与内嵌 | 仅执行一次 `bump-version.ps1 -Target Android`，Android 0.3.16／code 19，接收端同步递增为 0.3.16。核验 APK 内部版本、三个 EXE 文件／产品版本、正式版和诊断版实际 `--apk-info`；Gradle、独立交付和内嵌 APK 一致，发布清单八个文件的大小与 SHA-256 逐一复核通过 |
| APK 签名 | `apksigner verify --print-certs` 通过，证书 SHA-256 保持 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`，包名不变 |

交付目录 `dist/0.3.16` 包含以下带版本产物，同时生成 `release-manifest.json`、`SHA256SUMS.txt` 和两份 `*.exe.apk-info.json`：

| 产物 | 字节数 | SHA-256 |
|---|---:|---|
| `TapDeck-0.3.16.apk` | 11,768,479 | `86c3d70d0e07c73786d3fe6e73034630fb247f4b7eea30dcdba9e7e6fc570c85` |
| `TapDeck-0.3.16.exe` | 26,918,912 | `212e214e6ee4c97a487207ee56705540cd2c1f2709e79bd8fae6e814a998a64e` |
| `TapDeck-debug-0.3.16.exe` | 32,404,480 | `0e825db004037ab6ae496867c793be342a067b2a113f6ff99337cf1bfc6ebbcc` |
| `TapDeck-hidprobe-0.3.16.exe` | 8,048,128 | `241800dd1fa02c1c4ebcd4e88f9b4a08958bc6e42018edcb6cef94c5fa910d75` |

构建日志 `.tools/ui-validation/windows-0.3.16-build.log`，专项测试 `dist/0.3.16/multipc/android.log`、`fixture.log`，矩阵日志 `dist/0.3.16/screenshots/{phone-100,phone-130,small-130,tablet-100,restricted-100}.log`，扫码日志 `phone-100-scan.log`、`small-130-scan.log`。矩阵脚本可重复运行；专项脚本仅接受模拟器，保存／恢复测试前的完整偏好设置并清理临时接收端。

代表性截图：[电脑列表](screenshots/0.3.16/phone-100-multipc-two.png)、[管理电脑](screenshots/0.3.16/phone-100-multipc-manage.png)、[小屏大字体列表](screenshots/0.3.16/small-130-multipc-two.png)、[小屏管理页](screenshots/0.3.16/small-130-multipc-manage.png)、[录音期间禁用切换](screenshots/0.3.16/small-130-multipc-recording-disabled.png)、[多台电脑滚动列表](screenshots/0.3.16/phone-100-multipc-many.png)。截图中的名称、地址和连接状态为测试数据，不作为实机连接证明。管理页和选择器均可滚动查看超出屏幕的项目。

本轮没有已连接的实体手机；一部实体手机与两台真实 PC 的反复切换、覆盖升级后免确认重连、全部录音阶段及系统扫码／外部网页入口仍待实机验收。接收端停服覆盖连接丢失路径，尚未模拟真实 Wi-Fi 切网。真实 AudioRecord 到 CABLE Output／输入法识别、物理 PC 上的键鼠释放与干净系统驱动安装仍沿用实机待验收项。模拟器显示参数已恢复，隔离 AVD 和临时接收端已关闭。未替换用户运行中的接收端或重新安装驱动；本次为本地交付，公开 0.3.6 Release 链接保持不变。

## 0.3.15 连接与语音界面精简、自动配对确认

2026-10-08 完成 PC 连接／关于页的保存按钮和占位移除，连接页改为二维码与三步说明、网址操作、已配对设备。二维码随启动和网址变化自动更新。配对、解绑独立持久化；快捷键、语音、设置与状态保留全局保存并同步，切换页不丢失草稿。

首次手机请求触发原生确认框，展示设备名、完整标识和两行校验码；只有明确允许才授权，关闭／Esc 等同拒绝。按请求 ID 排队、去重，过期、断开或停止接收关闭对应弹窗，弹窗期间继续更新状态并延后驱动提示。等待配对和连接后的消息共用 WebSocket 读取通道。协议仍为 v2，配置 schema 仍为 3；新增的 `pairing_rejected`、`pairing_expired` 使用原有 `error` 消息，Android 结束本次重试并显示原因，保留已有凭据。

语音页将设备选择及两个操作按钮排成一行，分隔线下显示可折叠的三组横向表单。每次打开设置按启用状态初始展开，折叠与启用独立，切换类型和折叠保留隐藏热键。Android“连接与设置”增加 GitHub 链接，分割线上下各 16dp，扫码与“输入PC连接窗口URL”同排；仅已连接时显示“忘记当前电脑”。

| 验证项 | 结果 |
|---|---|
| 配对服务 | Go 用例通过：允许、拒绝原因、到期原因、手机断开及时清除、停止接收、请求 ID 隔离、首个决策生效及过期／取消后不能授权。原有凭据重连、解绑、并发连接及保存失败回归通过 |
| PC 原生界面 | 740×800 和最小 680×700 逻辑窗口，在本机 120 DPI（125%）显示器上通过。检查五页保存按钮显隐与草稿保留、无请求不弹窗、从隐藏窗口及其他页显示、请求排队／去重、允许、窗口关闭、实际 Esc 消息、超时／断开关闭。Walk 原生对话框从独立窗口消息进入模态循环，验证循环运行期间仍处理状态更新 |
| 语音编辑 | 默认展开已启用组、独立折叠、实时组名、切换类型后隐藏热键保留、长名称、纵向滚动和最小窗口横向行边界检查通过。现有热键录入单元测试覆盖特殊键解析、左右修饰键和组合键；本轮未向用户实际输入窗口注入按键 |
| Android 设备回归 | Android 14 / API 34 隔离模拟器，1080×2400／420 dpi／100% 字体与 640×960／320 dpi／130% 字体各 5 项，共 10 项通过：连接入口与设置、设备设置保持、扫码回填／取消／无效网址、拒绝／到期后停止重试与陈旧回调隔离、语音配置轮换／禁用／录音冻结。GitHub 的浏览器 Intent 及网址、扫码位置、滚动后忘记按钮均有断言 |
| 构建与静态检查 | 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.15` 通过：29 项版本规则检查、54 项 Kotlin JVM 测试（0 失败／错误）、Go 测试、`go vet`、APK 与三个 EXE 构建。沿用脚本对实际修改音量与开机自启注册表两个测试的排除 |
| 版本与内嵌 | 仅执行一次 `bump-version.ps1 -Target All`，两端 0.3.15／Android code 18。APK 内部版本、三个 EXE 的文件／产品版本、接收端运行时版本通过；正式版与诊断版均实际读取 `--apk-info`。Gradle、独立交付、内嵌资源及 EXE 报告的 APK SHA-256 相同，发布清单八个产物的哈希逐一复核通过 |
| APK 签名 | `apksigner verify --print-certs` 通过，证书 SHA-256 仍为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`，保持包名和签名 |

交付目录 `dist/0.3.15` 包含 `TapDeck-0.3.15.exe`、`TapDeck-debug-0.3.15.exe`、`TapDeck-hidprobe-0.3.15.exe` 和 `TapDeck-0.3.15.apk`。APK 为 11,442,228 字节，SHA-256 为 `d4bedef4135e9e1b85cc537cad02cea2c0654b31649b17c8a63cdf04ab2740c9`。发布校验文件为 `release-manifest.json`、`SHA256SUMS.txt` 及两份 `*.exe.apk-info.json`。构建日志位于 `.tools/ui-validation/windows-0.3.15-build.log`；PC 原生验证日志为 `.tools/ui-validation/pc-0.3.15-ui-125.log`，Android 日志为 `dist/0.3.15/screenshots/phone-100.log`、`small-130.log`。

代表性截图：[连接页](screenshots/0.3.15/pc-connection-740x800-120dpi.png)、[最小连接页](screenshots/0.3.15/pc-connection-680x700-120dpi.png)、[配对弹窗](screenshots/0.3.15/pc-pairing-680x700-120dpi.png)、[语音页](screenshots/0.3.15/pc-voice-740x800-120dpi-top.png)、[最小语音页](screenshots/0.3.15/pc-voice-680x700-120dpi-top.png)、[滚动底部](screenshots/0.3.15/pc-voice-680x700-120dpi-bottom.png)、[折叠状态](screenshots/0.3.15/pc-voice-680x700-120dpi-collapsed.png)、[Android 普通屏](screenshots/0.3.15/phone-100-pairing-scan-filled.png)、[小屏 130% 字体](screenshots/0.3.15/small-130-pairing-scan-filled.png)、[小屏已连接操作](screenshots/0.3.15/small-130-pairing-connected-actions.png)。PC 截图由复用生产组件的独立原生测试窗口生成，设备与网址为示例；Android 连接和语音状态为测试注入，不作为实体手机端到端结果。

模拟器显示参数已恢复，隔离 AVD 已关闭。本轮未替换用户运行中的接收端、安装到实体手机或重新安装驱动；真实手机与电脑配对、系统浏览器／相机唤起及传音识别仍需实机验收。仅交付本地版本，公开 0.3.6 Release 链接保持原值。

## 0.3.14 虚拟键盘启动检测、默认快捷键与音频输入入口

2026-10-08 增加 FakerInput 启动检测：缺失提示安装，已安装但不可用提示修复，已就绪时不打扰；后台自启、输入忙碌和安装中延后检查，每次运行只提示一次。语音页新增“系统音频输入设置”，优先打开 Windows“声音 → 录制”设备列表；顶部红色提示改为“系统音频输入或目标输入法的麦克风选择 CABLE Output。”，移除固定的欢迎捐赠与重启文案，保留来源、donationware 和原包许可。

PC 与 Android 新默认值统一为：音量- `VolumeDown`、上 `Up`、音量+ `VolumeUp`、退格 `Backspace`、左 `Left`、下 `Down`、右 `Right`、回车 `Return`，八项全部启用。应用升级继续保留已保存配置；按本轮用户明确选择，另外替换当前电脑的八项设置。

| 验证项 | 结果 |
|---|---|
| 默认配置与当前电脑 | 独立 Go 校验确认 `Default()`、空目录首次加载、Android 默认声明、八个按键解析及本机保存值逐项一致。当前接收端界面的名称、顺序、热键及启用状态均匹配，执行“保存并同步配置”返回成功；手机当时离线，尚未观察重连后的实机显示 |
| 本机备份与持久化 | `%LOCALAPPDATA%\TapDeck\config.json` 原始内容先备份为 `config.before-shortcuts-0.3.14-20261008-174325.json.bak`。本机文件只原子替换八项快捷键并将 revision 27 → 28，读取回验通过；其余字段与修改前一致，保留原 schema，未改配对文件或停止当前接收端 |
| 虚拟键盘 | 新增用例覆盖缺失／未就绪／可用、后台或忙碌延后、检测错误后重试、取消后不重复提示及精确匹配 `root\FakerInput`。本机 `--keyboard-status` 显示 `installation=ready`、实际 HID、FakerInput 0.1.1 / API 1；独立调用 PnP 检测路径（传入 HID 不可用，未禁用真实设备）仍正确识别已安装 |
| 音频设置入口 | 单元测试验证打开 `control.exe mmsys.cpl,,1`，启动失败回退至 `ms-settings:sound-defaultinputproperties`，均失败时报告错误；正常路径只打开一个窗口，不改默认设备。本轮未在运行中的旧版接收端点击新版入口 |
| PC 布局 | 独立屏幕外 Walk 编辑器在 740×800 和最小 680×700 逻辑窗口、120 DPI（125%）下通过。检查两张顶部截图，红色提示完整换行，四个驱动／许可按钮与新的音频设置入口可见，三组配置保持纵向滚动；原有字段切换检查通过 |
| 自动检查与构建 | 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.14` 通过：29 项版本规则检查、54 项 Kotlin JVM 测试（0 失败／错误）、Go 测试、`go vet`、APK 与三个 EXE 构建。继续排除会实际修改本机音量和开机自启注册表的两个测试 |
| 版本、内嵌及签名 | 两端版本 0.3.14 / Android code 17；三个 EXE 的文件版本、产品版本及数字版本核验通过，正式版与诊断版实际 `--apk-info` 对应同一最新 APK。交付／Gradle／内嵌 APK 哈希一致，`apksigner verify --print-certs` 通过，证书与 0.3.13 相同 |

交付目录 `dist/0.3.14` 包含 `TapDeck-0.3.14.exe`、`TapDeck-debug-0.3.14.exe`、`TapDeck-hidprobe-0.3.14.exe` 和 `TapDeck-0.3.14.apk`；本次 APK 为 11,442,228 字节，SHA-256 为 `a8d08ab9250895e954706201fc01692186207925695dbe04f52aaac564376dde`。版本与校验清单为 `release-manifest.json`、`SHA256SUMS.txt` 及两份 `*.exe.apk-info.json`。构建、布局、默认值／PnP 检查日志分别位于 `.tools/ui-validation/windows-0.3.14-build.log`、`pc-voice-0.3.14-ui.log`、`shortcuts-driver-0.3.14.log`。

代表性截图：[语音页](screenshots/0.3.14/pc-voice-top.png)、[最小窗口](screenshots/0.3.14/pc-voice-minimum.png)。本轮未实际重新安装／修复本机驱动，也未在干净系统验证缺失驱动的完整安装流程；Android 设备 UI 与真实传音链路未重跑。仅生成本地交付包，公开 0.3.6 下载链接保持原值。

## 0.3.13 版本长期规则与最新控制端内嵌校验

2026-10-08 将每次迭代升版、所有平台产物携带版本、接收端每次打包嵌入最新控制端，以及完成后提交 Git 写入 `AGENTS.md` 和 [版本管理](versioning.md)。新增唯一版本来源 `version.properties`，接收端与 Android 可独立演进；本轮通过递增脚本从 0.3.12 / code 15 升至接收端 0.3.13、Android 0.3.13 / code 16。

- `scripts/test-versioning.ps1` 的 29 项检查通过：不同版本组合、接收端独立升版、Android 更新同时递增接收端、major/minor 递增、无效／重复／大小写错误字段、版本及 code 越界、失败保留原文件、过期 Gradle 元数据、缺包／空包，以及最终 EXE 的版本、code、文件名、大小和哈希不一致均拒绝。
- 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.13` 通过：54 项 Kotlin JVM 测试、Go 测试、`go vet`、Android APK 及三个 Windows EXE 构建；继续排除实际改变系统音量和开机自启注册表的两个测试。
- APK 内部 Manifest 经 SDK `aapt` 检查，与 Gradle 元数据和统一版本清单一致；`apksigner verify --print-certs` 通过，签名证书与 0.3.12 相同，包名保持 `com.yuncii.tapdeck`。
- 三个 EXE 的文件版本与产品版本均为 `0.3.13`，数字版本 `0.3.13.0`；诊断版 `--version` 为 `TapDeck 0.3.13`。正式版与诊断版均实际执行 `--apk-info`，核对内嵌 Android 0.3.13 / code 16、文件名、字节数及 SHA-256。
- Gradle、独立 Android、接收端内嵌及交付目录 APK 哈希一致。三个 EXE 与其兼容副本分别哈希一致；构建自动生成的 `release-manifest.json` 和 `SHA256SUMS.txt` 记录八个交付／兼容文件，实际文件哈希复核通过。
- Android 截图目录和 Windows 手势日志读取对应端版本，不再维护手写版本路径。此轮未启动设备 UI、实体手机或实际 PC 接收服务；iOS 等未来平台仅记录接入规划，没有生成或声称验证其安装包。

交付目录为 `dist/0.3.13`，包含 `TapDeck-0.3.13.exe`、`TapDeck-debug-0.3.13.exe`、`TapDeck-hidprobe-0.3.13.exe` 和 `TapDeck-0.3.13.apk`。APK 为 11,442,228 字节，SHA-256 为 `5ab4c847c96c88855c0ed573a9e35656089f408312b14b1abe0704e71aa9d647`。构建日志为 `.tools/ui-validation/windows-0.3.13-versioning-build.log`，两份 EXE 内嵌核对结果为交付目录下的 `*.exe.apk-info.json`。本轮仅生成本地交付包，既有公开 0.3.6 下载链接保持原值。

## 0.3.12 Windows 文件名与文件属性版本

2026-10-08 为 Windows 构建增加版本文件名，沿用本轮应用版本 0.3.12 / Android code 15。正式版、控制台诊断版及 HID 工具分别生成 `TapDeck-0.3.12.exe`、`TapDeck-debug-0.3.12.exe`、`TapDeck-hidprobe-0.3.12.exe`，并保留三个不带版本号的兼容副本。

- 新构建资源工具从 Gradle 元数据取得版本，生成 VERSIONINFO；Windows 文件属性中的 `FileVersion`、`ProductVersion` 均为 `0.3.12`，数字版本为 `0.3.12.0`。正式构建脚本自动核对这些属性。
- 读取三个实际 EXE 的 PE 资源，逐字节确认应用清单与原 `app.manifest` 一致，同时存在版本资源；原有 Common Controls、DPI 与 `asInvoker` 设置保留。
- 三组带版本／兼容文件的 SHA-256 分别一致。带版本号的诊断版 `--version` 与 `--apk-info` 均正常，内嵌 APK 的版本和哈希保持本节下方记录的值。
- 官方构建入口通过 Android 构建及 JVM 检查（54 项已有结果复用）、Go 测试、`go vet` 和三种 EXE 构建；沿用对系统音量与开机自启注册表两个动作测试的排除。未更改本机配对、系统配置或开机自启注册表。

产物位于 `dist/0.3.12`；文件属性结果保存于 `windows-version-info.json`，全部 EXE／APK 的校验值更新至 `SHA256SUMS.txt`。本次构建日志为 `.tools/ui-validation/windows-0.3.12-versioned-build.log`，仅生成本地交付包。

## 0.3.12 第三组语音默认值与捐赠按钮精简

2026-10-08 第三组默认名称由“GPT听写”改为“自定义语音输入”，长按触发、单击开始、单击结束三个热键字段全部留空；默认类型仍为长按、默认关闭。PC 与 Android 默认声明一致，已保存的 schema 3 配置保持原名称与按键，旧配置升级时新增的第三组采用新默认值。

核对[官网分发条件](https://vb-audio.com/Services/licensing.htm)及内嵌原包许可后，移除 PC 语音页独立“捐赠 / 购买”按钮；保留 VB-Audio 官网、原包许可、来源和 donationware 说明，用户可经官网找到捐赠及购买许可入口。判断依据和分发说明同步记录于 [第三方声明](../THIRD_PARTY_NOTICES.md)。

- 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.12` 通过：54 项 Kotlin JVM 测试、Go 测试、`go vet`、APK 及三个 Windows EXE 构建。沿用对系统音量和开机自启注册表两个动作测试的排除。
- 更新默认值、名称边界和迁移用例，检查第三组名称与全部空热键；服务端同类型不同组路由测试显式设置测试热键，保留原有身份／revision／旧客户端覆盖。
- 独立屏幕外 Walk 编辑器通过 740×800 与最小 680×700 逻辑窗口、120 DPI（125%）的布局与字段保留检查。6 张截图显示顶部按钮减少为四个，第三组新名称完整可见、触发热键为空；切换单击后的隐藏字段保留逻辑继续通过。
- PC `--version`、`--apk-info` 与 Android 均为 0.3.12 / code 15，内嵌、Gradle 和交付 APK 哈希一致，沿用原签名证书。本轮未重新运行 Android 模拟器或实体手机测试，未启动实际 PC 接收服务。

交付目录为 `dist/0.3.12`，APK `TapDeck-0.3.12.apk` 为 11,442,228 字节，SHA-256 为 `a1d0393e93b6b8e104831a1c74e8dc4f28dbb09ecab948985821e0a9ee46c84b`。产物校验值保存在目录内 `SHA256SUMS.txt`；构建及 PC 布局日志为 `.tools/ui-validation/windows-0.3.12-build.log` 和 `.tools/ui-validation/pc-voice-0.3.12-ui.log`。

代表性截图：[语音页按钮](screenshots/0.3.12/pc-voice-top.png)、[第三组默认值](screenshots/0.3.12/pc-voice-defaults-bottom.png)。本轮仅生成本地交付包。

## 0.3.11 本机快捷键设为默认与三次弹跳提醒

2026-10-08 读取本机 `%LOCALAPPDATA%\TapDeck\config.json`（schema 2、revision 27），确认与原默认值不同：第四项为“说话” `Ctrl+L`，第五至八项分别绑定 `Left`、`Up`、`Down`、`Right`，八项全部启用。按照用户要求，将这八项的名称、顺序、按键和启用状态设为 PC 新安装与 Android 的默认配置，完整列表见 [README](../README.md)。已有配置不重置，旧四槽位升级时新增四槽仍为空且关闭。

未连接图标每轮由一次往返改为三次连续弹跳：相对高度 100% / 50% / 25%，每次往返 440 / 310 / 220 ms，最高 `0.012W`。上升使用减速曲线、下落使用加速曲线，轮次仍每 3 秒启动一次；连接成功、打开弹窗或退到后台时取消并复位。

| 验证项 | 结果 |
|---|---|
| 当前设置与新默认值 | 独立 Go 校验确认 PC 默认值、空目录首次加载值及 Android 默认声明均与本机八项逐项一致。对本机配置的临时副本执行 schema 2 → 3 迁移，八项保持不变；原配置文件字节未改变 |
| Kotlin JVM | 54 项通过，0 失败／错误；提醒时序用例验证每轮三次逐渐降低的弹跳、每 3 秒重启，以及等待／弹跳中的取消和重新启用后的等待 |
| Go 与构建 | 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.11` 通过 Android 构建、Go 测试、`go vet` 和三个 Windows EXE 构建；沿用对系统音量及开机自启注册表两个动作测试的排除。旧四项配置迁移、保存与启用项校验继续通过 |
| Android 回归 | Android 14 / API 34 隔离模拟器，1080×2400 / 100% 字体、640×960 / 130% 字体及 1080 px 宽、应用高 600 px 的受限视口各 2 项，共 6 项通过。验证未连接图标有位移、连接后复位且静止、可打开连接设置，以及语音配置切换／禁用／录音冻结 |
| 界面与产物 | 普通屏、小屏截图确认默认八项完整显示为两行。Android 0.3.11 / code 14，PC 版本与内嵌 APK 一致；APK 验签通过并沿用原证书，模拟器覆盖安装成功 |

交付目录为 `dist/0.3.11`，APK `TapDeck-0.3.11.apk` 为 11,664,303 字节，SHA-256 为 `63178de09f4c18c80d7261872412fe8028173f7305e6b7736982b4b6a66255d8`。签名证书 SHA-256 为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。产物校验值保存在交付目录 `SHA256SUMS.txt`；构建日志为 `.tools/ui-validation/windows-0.3.11-build.log`，设备日志及截图在交付目录 `screenshots`。

代表性截图：[普通屏默认八项](screenshots/0.3.11/phone-100-default-shortcuts.png)、[小屏默认八项](screenshots/0.3.11/small-130-default-shortcuts.png)。截图的连接和语音状态为测试模拟；本轮没有实体手机连接，未验收实际电脑按键效果。模拟器显示参数恢复后关闭隔离 AVD，本轮仅生成本地交付包。

## 0.3.10 Android 语音提示与电平条布局

2026-10-08 将语音操作说明移至底部，与“可拖动到区域任意位置”合并成一行，配置切换按钮单独居中。激活后的准备、录音和停止阶段，底部改为“麦克风电平百分比 · 单击／长按操作说明”。横向电平进度移至语音区顶边，位于快捷键与语音区之间，按用户后续反馈改为 2 倍显示长度（最多填满整行），百分比仍为原电平；保留录音控件周围的电平反馈及原有交互。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM | 54 项通过，0 失败／错误 |
| Android 设备回归 | Android 14 / API 34 隔离模拟器：1080×2400 / 100% 字体、640×960 / 130% 字体、1080 px 宽且应用高 600 px 的受限视口，各 3 项，共 9 项通过。复用配置轮换／禁用／录音冻结、快捷键与键盘布局、触控板与语音多指独立操作用例；电平条长度调整后，三种布局的语音配置用例另各复验 1 次，均通过 |
| 截图检查 | 三种布局均保存长按／单击空闲、准备、传输、停止及禁用／最长名称画面。检查普通屏、小屏大字体和受限高度的代表画面：底部文字单行完整，录音显示 42% 时横向条约占 84% 行宽，电平条位于快捷键下方，底部无横向进度条 |
| 构建 | 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.10` 通过 Android 构建、Go 测试、`go vet` 及三个 Windows EXE 构建；沿用脚本对系统音量和开机自启注册表两个动作测试的排除 |
| 版本与签名 | Android 为 0.3.10 / code 13；PC `--version`、`--apk-info` 与 APK 元数据一致。Gradle、交付及 EXE 内嵌 APK 哈希一致，沿用原签名，模拟器覆盖安装成功 |

交付目录为 `dist/0.3.10`，主安装包 `TapDeck-0.3.10.apk`（11,664,064 字节），APK SHA-256 为 `7881eb2ca86c64d138a8988582785b9218cfb9931e165060b1d9b36ee103ca96`。签名证书 SHA-256 为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。产物校验值在 `dist/0.3.10/SHA256SUMS.txt`；构建日志为 `.tools/ui-validation/windows-0.3.10-build.log`，模拟器日志及 33 张截图位于交付目录 `screenshots`。

代表性截图：[普通屏单击录音](screenshots/0.3.10/phone-100-voice-toggle-transmitting.png)、[小屏长按空闲](screenshots/0.3.10/small-130-voice-hold-idle.png)、[小屏单击录音](screenshots/0.3.10/small-130-voice-toggle-transmitting.png)、[受限高度长按录音](screenshots/0.3.10/restricted-100-voice-hold-transmitting.png)。

本轮连接、录音状态与电平使用测试模拟，未重新验证实体手机传音与 PC 输入法识别。模拟器显示参数恢复后关闭隔离 AVD。未停止或替换用户运行中的 PC 接收端；公开下载链接仍指向已发布的 0.3.6，本轮生成本地交付包。

## 0.3.9 三组语音配置

2026-10-08 按确认方案实现固定三组语音配置。新安装默认启用单击 `RightCtrl+L` 起停、长按 `RightAlt`，第三组“GPT听写”使用长按 `Ctrl+Shift+M` 且默认关闭。每组支持名称、启用、类型和对应热键；PC 保留上方音频设置，下面纵向滚动，保存后同步。Android 显示组名并按启用顺序轮换，只有一组或正在录音时禁止切换，全部关闭时禁用录音。全键盘长按空格使用当前组且始终松手结束。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM | 54 项通过。新增覆盖默认值、ASCII／中文／混排／补充平面 Unicode 边界、按 ID 记忆、改名／换类型、关闭后回退和旧 PC 配置 |
| 配置迁移与保存 | Go 覆盖 schema 0/1/2 → 3，旧热键（包括空键）、其他设置与原始字节备份保留；已有不同备份保留并追加后缀；无效名称、缺失组、备份失败及写入失败不覆盖原文件 |
| 语音通信 | WSS 服务端测试验证同类型两组按不同 ID 使用不同热键，过期 revision／关闭组／未知 ID 返回带录音 ID 的错误与最新配置；旧手机按类型使用第一组，没有匹配则拒绝。Android 测试验证新字段、旧 PC 模式请求、手势与热键类型分离、停止／错误复位和过期回复隔离 |
| 录音快照 | 开始后 PC 改名、修改热键并关闭全部组，仍按原开始／结束热键完成；重复停止／晚到完成不重复结束。Android 在准备、录音、停止时保持原名称与形状，切换无操作或震动，全部关闭后仍可停止当前录音 |
| Android 设备回归 | Android 14 / API 34 隔离模拟器，1080×2400 / 100% 字体 16 项、小屏 640×960 / 130% 字体 9 项，共 25 项通过。覆盖 0/1/2/3 组、循环顺序、重开后选中组记忆、最长名称、禁用按钮、触控板首次点击仅停止单击录音、空格语音、键盘／快捷键／多指手势与现有设备设置 |
| PC 界面 | 独立屏幕外 Walk 编辑器在 740×800、最小 680×700 逻辑窗口及本机 120 DPI（125%）下完成截图检查；滚动可访问第三组，类型切换后隐藏热键保留，8 字名称完整。修正 Walk 固定横向滚动区域居中收窄的问题，并为纵向滚动条预留空间 |
| 构建 | 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.9` 通过 Android 构建、Go 测试、`go vet` 及三个 EXE 构建；沿用脚本排除系统音量与开机自启注册表两个动作测试 |
| 版本与签名 | PC `--version`、`--apk-info`、Gradle 及 APK badging 均为 0.3.9 / code 12；Gradle、交付及 EXE 内嵌 APK 哈希一致。包名 `com.yuncii.tapdeck`、最低 API 26、现有签名保持不变，模拟器覆盖安装成功 |

交付目录为 `dist/0.3.9`，主安装包为 `TapDeck-0.3.9.apk`（11,664,324 字节），APK SHA-256 为 `cba628521fca2cddc0ca3536662b0a665800bb56e6275550bd4d6d8d2077b016`。签名证书 SHA-256 仍为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。校验值在 `dist/0.3.9/SHA256SUMS.txt`；构建日志为 `.tools/ui-validation/windows-0.3.9-build.log`，最终设备日志和截图在交付目录 `screenshots`。

代表性截图：[PC 最小窗口](screenshots/0.3.9/pc-voice-680x700-120dpi-top.png)、[第三组与滚动底部](screenshots/0.3.9/pc-voice-680x700-120dpi-bottom.png)、[小屏单组最长名称](screenshots/0.3.9/small-130-voice-single-long-name.png)、[全部关闭](screenshots/0.3.9/small-130-voice-disabled.png)。

本轮新旧端兼容为协议／组件级回归，输入与音频路由使用测试替身，另验证模拟器实际 AudioRecord 的释放；未在实体手机与真实输入法之间重做传音、识别和三组热键联动。未停止或替换用户运行中的 0.3.7 接收端。模拟器显示参数恢复后关闭隔离 AVD。公开下载链接仍指向已发布的 0.3.6，本轮仅生成本地交付包。

## 0.3.8 连接设置精简与扫码入口移动

2026-10-08 用户反馈：实体手机在关闭系统震动开关后，TapDeck 仍可正常震动。基于这次实机确认，移除 Android“连接与设备设置”中的“系统震动设置”入口，保留 App 震动开关和“测试震动”按钮。扫码图标从网址输入框内移至下方操作行，已连接时与“忘记当前电脑”同排并靠右，未连接时也保持右对齐。

- 版本为 `0.3.8` / code `11`。官方 Windows 构建入口通过 51 项 Kotlin JVM 测试、Go 测试、`go vet` 及两端构建；沿用对本机音量和开机自启注册表两个动作测试的排除。
- Android 14 / API 34 隔离模拟器分别验证 1080×2400 / 100% 字体和 640×960 / 130% 字体。两组的扫码回填和设备设置用例均通过：有效二维码回填、取消／错误二维码保留网址、错误后重新扫码、不自动连接、震动开关和测试按钮均正常。
- 检查普通屏幕及小屏的已连接／未连接截图，系统震动入口已移除，扫码图标与忘记按钮同排且靠右。小屏内容可滚动；现有扫码用例改为滚动后操作可点击父节点，解决滚动动画期间图标坐标变化导致的测试定位失败。扫码结果和连接状态使用测试模拟，未连接实际 PC。
- 交付目录为 `dist/0.3.8`，主安装包为 `TapDeck-0.3.8.apk`，Windows EXE 内嵌相同 APK。两端版本和内嵌／Gradle／交付 APK 的哈希一致，签名证书保持 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。APK SHA-256 为 `8966657a5d6bb6290fffaa77976983786d73fc2443fde3d5a1b7c38e6582fb43`；校验文件、截图及回归日志保存在交付目录，构建日志为 `.tools/ui-validation/windows-0.3.8-build.log`。

模拟器显示参数已恢复，隔离 AVD 已关闭；本轮未向实体手机安装，也未替换正在运行的 PC 接收端。上述震动实机结论来自用户对上一版本的反馈。

## 0.3.7 直接震动、快捷键间距、版本信息与语音停止同步

2026-10-08 完成：Android 直接调用振动服务，快捷键横向／纵向间距和四周外边距统一为安全视口宽度的 1%（受限窗口缩小纵向尺寸），连接设置显示版本；APK 命名为 `TapDeck-0.3.7.apk`，PC 新增“关于”页和手机输入测试框。Android 为 `0.3.7` / code `10`，Windows 从同一份 Gradle 元数据取得 `0.3.7`，控制协议保持 v2。

按用户确认的行为，单击语音录音中的触控板点击只停止录音，本轮按下／抬起不向电脑发送鼠标消息；停止完成后的下一次点击才操作鼠标。匹配当前录音的 PC 停止或错误消息会释放手机采音并复位状态；陈旧回复不影响新的录音。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM | 51 项通过，0 失败／错误；反馈策略改为仅由 App 开关和马达可用性决定，保留失败、异常及单次请求覆盖 |
| Android 矩阵 | API 34 隔离模拟器四组共 33 次测试通过：1080×2400 / 100% 字体 14 项、640×960 / 130% 8 项、1404×1872 / 100% 3 项、宽 1080 px 且应用高 600 px 的受限视口 8 项 |
| 快捷键与设置 | 1／4／5／8 项快捷键实际边界的左右／底部外间距、横向／纵向间隙符合 1% 规则，误差不超过 1.5 px；上留白复用原生容器。全键盘和语音布局、灵敏度持久化、连接提醒、震动开关及新版本显示检查通过 |
| 直接震动 | `CLICK` / `HEAVY_CLICK` 在模拟器振动服务留下 `Usage=PHYSICAL_EMULATION`、`status: finished` 记录；App 开关关闭时无新请求，系统触感开关关闭或触感强度设为 0 时仍完成请求。测试后恢复原设置，不把模拟服务记录当作实体马达实测 |
| 单击语音与鼠标 | 模拟 WSS 验证准备／传输时首次左击只发送一次 `mic_stop`，停止期间的右击也不发送鼠标；匹配回复后恢复下一次鼠标点击。消费完整按下／抬起和重复 down；长按录音仍允许操作鼠标 |
| PC 停止与采音 | 实际启动模拟器 AudioRecord，旧 ID 的回复不停止线程，当前 ID 的回复释放采音线程并清空模式与电平；后续录音不受旧停止回复影响，错误回复也复位为 idle。没有把录音发送到实际 PC |
| Windows 关于 | 使用与生产相同声明的独立 Walk 窗口验证版本、必要信息和输入框布局，中文／英文／换行输入及清空通过；保存 `pc-about.png` 和日志。该预览不启动接收服务，不代表实际手机键盘或语音识别链路已验收 |
| 构建与静态检查 | 官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.7` 成功，Go 测试、`go vet`、Android 主／测试 APK 及 Windows GUI／诊断／HID 探针构建通过；沿用对系统音量和开机自启注册表两个动作测试的排除 |
| 版本、APK 与签名 | `--version` 为 TapDeck 0.3.7；`--apk-info` 的 PC 版本、Android 版本、code 和带版本的文件名符合预期。Gradle 输出、独立版本 APK、交付及内嵌 APK 的 SHA-256 一致；新旧签名证书相同，模拟器 `install -r` 覆盖安装成功 |

交付目录为 `dist/0.3.7`，保留 `TapDeck-debug.apk` 兼容副本。APK SHA-256 为 `69cb55639b07e487659db0164210765ad0cefce494c7120fd9af705beaed1c66`，签名证书 SHA-256 为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。各产物校验值保存于 `dist/0.3.7/SHA256SUMS.txt`，本轮模拟器日志和截图位于 `dist/0.3.7/screenshots`。

实体手机未安装本轮 APK，现有 PC 接收端未停止或替换；模拟器显示与触感设置恢复，测试麦克风授权撤销，隔离 AVD 关闭。普通系统触感开关不再由 App 主动拦截，但总震动开关、硬件反馈强度及厂商策略仍可能限制输出；API 26–32 的实际兼容行为、真机震动手感、覆盖升级、原配对重连及手机语音识别到 PC 输入框的完整链路仍需实机验收。没有发布新的 GitHub Release。

## 2026-10-08 Android 扫码回填与连接设置

“连接与设备设置”的网址输入框增加扫码入口，扫描 PC 的 HTTP `/pair` 二维码后回填网址，仍由用户点击“连接”发起配对。说明改为“输入PC设置窗口显示的配对网址：”。删除震动开关下方说明，“测试震动”改为实心按钮，与带右箭头的“系统震动设置”同排。

- Android APK 构建成功，51 项 Kotlin JVM 测试通过，0 失败／错误；新增二维码网址解析验证，覆盖有效地址、空白、错误协议、非配对路径、凭据及无效端口。
- Android 14 / API 34 隔离模拟器：1080×2400 / 100% 字体、640×960 / 130% 字体均通过扫码结果回填与震动设置交互，共 4 次用例通过。扫码用例通过 ActivityMonitor 注入成功、取消和错误二维码结果，检查原网址保留、错误后重扫成功及不自动连接；小屏错误提示位于可滚动区域，滚动后验证可见。
- 实际打开扫码页，验证相机权限拒绝后的中文提示、重新授权、竖屏相机预览及返回弹窗后保留网址。普通屏幕和小屏截图确认震动按钮、系统设置文字与右箭头同排。
- APK 位于 `dist/TapDeck-debug.apk`，SHA-256 为 `1998be562a6560d2cbbdf44da1d8614910be38402d6f9b8604f8fa6ab480ac9f`；预览图保存在 `dist/android-pairing-settings/`。模拟器显示参数已恢复；未向实体设备安装，本轮尚未验证真机镜头对 PC 二维码的实际识别与震动手感。

## 0.3.6 Android 图标与全键盘优化

2026-10-08 为 Android 增加 PC 同款蓝底 `#175CD3`、白色方块图标：自适应图层为 108dp，白色方块居中且边长 48dp；另有普通兼容图标、圆形资源与 API 33+ 单色主题层。Manifest 同时指定 `android:icon` 和 `android:roundIcon`。

全键盘普通键宽为安全视口宽度的 8.9%，横向间隙、正常视口行间距及四周留白均为 1%。Shift / Backspace / Shift+Enter 为 13.85%，Ctrl / Enter 为 18.8%，空格为 33.65%；第二行居中。键盘集中计算累计边界后取整，上下复用底部容器留白，底部总高度仍为 0.62W。受限窗口只缩小纵向尺寸，快捷键和语音区间距、字号、键值及交互保持原样。Android 为 `0.3.6` / code `9`，控制协议继续使用 v2，交付目录为 `dist/0.3.6`。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM | 49 项通过，0 失败／错误；新增 3 项覆盖普通／特殊键宽、第二行居中、像素舍入、受限窗口纵向缩放和极小尺寸的无重叠／无越界 |
| Android 矩阵 | Android 14 / API 34 隔离模拟器五组配置合计 34 次交互测试通过：1080×2400 / 100% 字体 12 项、720×1280 / 130% 3 项、640×960 / 130% 8 项、1404×1872 / 100% 3 项、宽 1080 px 且应用高 600 px 的受限视口 8 项 |
| 实际键面边界 | 五组均检查 33 个键面实际像素宽度、横向间隙、行间距、四周留白及第二行居中；误差不超过 1–2 px，无重叠或越界。第一、三、四行对齐，模式切换保持触控板与底部边界；1／4／5／8 项快捷键布局通过 |
| 图标 | 检查普通与圆形自适应资源的蓝白配色、48dp 前景范围和单色层；保存系统／圆形／圆角方形／单色蒙版预览。模拟器 Pixel Launcher 桌面及应用列表均显示新版图标。系统按当前配置优先选择 roundIcon，测试同时检查两种资源；二进制 Manifest 的两个属性也已核对 |
| 截图检查 | 矩阵生成 36 张图片，另保存 2 张真实模拟器桌面／应用列表截图，共 38 张。已检查五组全键盘、图标蒙版及小屏／受限窗口八项快捷键画面：主副标签、Ctrl 文字、Shift / Backspace / Shift+Enter / Enter 图标均在键面内。受限窗口下方白色区域为测试夹具留白 |
| 按键与多指回归 | Ctrl 短按、多指 Ctrl+C、Shift 单次大写、Enter、Shift+Enter、主副键短长按、空格录音起停及退出清理通过；触控板与语音控件独立多指分发、滚动／缩放／三指识别和反馈次数通过。模拟器振动服务 CLICK / HEAVY_CLICK 的 TOUCH 完成记录及开关禁用检查通过 |
| Go 与 Windows | 官方构建入口的 Go 测试、go vet 与三种 Windows 构建通过；沿用对 TestVolumeControlChangesEndpoint、TestEnableDisableRoundTrip 两个硬件动作测试的排除 |
| APK 与签名 | Gradle 输出、Windows embed 与交付 APK 的 SHA-256 一致；GUI／控制台 EXE 的 --apk-info 均为 0.3.6 / code 9、11103144 字节及相同 SHA-256。包名 com.yuncii.tapdeck、最低 API 26 和现有签名证书保持原值；模拟器以 install -r 覆盖安装 |

正式构建使用 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.6`，日志为 `.tools/ui-validation/windows-0.3.6-build.log`；最终五组通过日志和截图位于 `dist/0.3.6/screenshots`。仅调整图标测试对系统选用圆形资源的假设，产品 APK 未因此改变。键盘尺寸与图标说明见 [Android 主界面](android-ui.md) 和 [全键盘](full-keyboard.md)。已将代表性截图保留到源码文档：可查看 [手机全键盘](screenshots/0.3.6/phone-100-keyboard.png)、[图标蒙版](screenshots/0.3.6/phone-100-launcher-icon-masks.png) 和 [应用列表](screenshots/0.3.6/phone-100-launcher-apps.png)。

APK SHA-256 为 `41c40caeb9db6b9cfd20a48deb0a6b2f72ec6f8dc04d8f598b2f969f87a32b3f`，签名证书 SHA-256 为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。四个产物的校验值见 `dist/0.3.6/SHA256SUMS.txt`。

模拟器显示、密度、字体和触感设置均已恢复，隔离的 TapDeckUiValidation AVD 已关闭；用户运行中的 `dist/0.3.5/TapDeck.exe` 未停止或替换。本轮未安装到实体手机。真机覆盖升级、原配对重连、本机设置保留、厂商桌面蒙版、实际震动和按键手感仍由用户验收；API 26–32 的图标资源已编译，本轮只在 API 34 执行运行时验证。模拟器中的电脑连接及输入／音频回调为测试模拟，不能代表真实 PC 输入或传音验收。

## 0.3.5 灵敏度弹窗精简与震动修复

2026-10-08 删除灵敏度弹窗里的慢速／快速标签和立即生效／本机保存说明。倍率范围、实时更新、本机保存及恢复默认保持原样。此前仅调用 View 的键盘触感接口，没有检查结果；本轮在 Android API 29+ 直接调用振动服务的预设 CLICK / HEAVY_CLICK，补齐普通 VIBRATE 权限，API 26–28 使用 VIRTUAL_KEY / LONG_PRESS 并检查返回值。所有按键区域共用反馈控制器，遵循本机开关、系统触感及强度设置。

手机顶部连接图标打开“连接与设备设置”，顶部为默认开启的“按键震动反馈”及“测试震动”，可进入手机系统声音与振动设置；系统触感关闭或无马达时显示对应提示，返回后重新检测。PC“设置与状态”标明手机开关的位置。Android 为 `0.3.5` / code `8`，控制协议继续使用 v2，交付目录为 `dist/0.3.5`。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM | 46 项通过，0 失败／错误；新增 5 项覆盖本机开关立即变化、系统关闭、无马达、服务失败、旧系统返回值及不重复请求 |
| Android 矩阵 | Android 14 / API 34 隔离模拟器的五组配置合计 33 次交互测试通过：手机 1080×2400 / 100% 字体 11 项、720×1280 / 130% 3 项、小屏 640×960 / 130% 8 项、平板 1404×1872 / 100% 3 项、宽 1080 px 且应用高 600 px 的受限视口 8 项；保存 35 张截图 |
| 灵敏度与开关 | 验证指定文字不再显示，滑块上下限、恢复默认、即时鼠标倍率换算、重开后保存及忘记配对不重置继续通过。震动开关关闭后测试按钮提示开启，打开后可发送测试请求；小屏的弹窗内容可滚动 |
| 振动服务 | 额外用例确认 VIBRATE 权限已授予，CLICK 和 HEAVY_CLICK 在模拟器振动服务留下 `Usage=TOUCH`、`status: finished` 记录。应用关闭、系统触感关闭或强度设为关闭时不增加记录；系统设置在 finally 中恢复。该记录表示模拟服务完成，不能代表实体手机马达效果 |
| 按键与手势 | 快捷键、全键盘及语音控件的反馈次数、短长按、松手清理、模式切换和多指操作通过；同时回归拖拽、双指滚动／缩放和三指窗口消息的 Android 识别 |
| Go 与 Windows | 官方构建入口的 Go 测试、go vet 与三种 Windows 构建通过；沿用脚本对系统音量及 HID 安装状态两个硬件动作测试的排除，未改变 PC 输入协议 |
| APK 与签名 | Gradle 输出、embed、交付 APK 与两种 EXE 的 `--apk-info` 均为 0.3.5 / code 8，SHA-256 一致。包名 `com.yuncii.tapdeck`、最低 API 26，签名证书保持原值；模拟器以 install -r 覆盖安装 |

正式构建为 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.5`，日志保存在 `.tools/ui-validation/windows-0.3.5-build.log`。最终五组通过日志和截图位于 `dist/0.3.5/screenshots`。矩阵中调整了测试同步：快捷键等待 Compose 应用禁用状态后再点击，平板截图采样最多观察两轮提醒，避免慢截图错过 360 ms 动画；产品的禁用逻辑、连接动画及 APK 未因此改变。脚本支持 `-Variants` 重跑指定配置，未重复运行已通过的其他配置。

APK SHA-256 为 `5e682f993a661b2b471094fd86395cb6e0e698d0c1b6a5999089f66543eff0b6`，签名证书 SHA-256 为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。四个产物的校验值见 `dist/0.3.5/SHA256SUMS.txt`。

模拟器显示、字体和触感设置均已恢复，隔离的 TapDeckUiValidation AVD 已关闭，用户的现有 PC 接收端未停止或替换。实体手机在只读诊断时 ADB 断开，因此未获得其系统触感设置，不能认定其未震动的具体原因；本轮未安装到实体手机。实际震动手感、手机系统设置入口、真机覆盖升级和原配对重连仍由用户验收；API 26–28 的实际触感回退也尚未在对应设备运行。

## 0.3.4 设备灵敏度与按键震动

2026-10-07 将触控板灵敏度迁移到各 Android 设备。左上角滑块图标打开设置，旁边实时显示倍率；默认 1.0×，最小 0.5×、最大 3.0×，步长 0.1，并可恢复默认。1.0× 对应原 PC 灵敏度 2，PC 固定兼容字段并移除编辑框。快捷键、全键盘、语音控件按下轻震，长按功能生效后反馈一次，连接设置可关闭。Android 为 `0.3.4` / code `7`，控制协议继续使用 v2；交付目录为 `dist/0.3.4`。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM | 41 项通过，0 失败／错误；新增 4 项覆盖倍率范围、默认基准、无效值、读取与修改竞态、保持其他本机设置、慢写入顺序、关闭时保存、不同设备独立性及读写失败后的继续调节 |
| Android 布局矩阵 | 五组配置共 32 次交互测试通过：1080×2400 / 100% 字体 10 项、720×1280 / 130% 3 项、640×960 / 130% 8 项、1404×1872 / 100% 3 项、1080 px 宽且应用高 600 px 的受限视口 8 项；共 35 张截图 |
| 灵敏度交互及保存 | 未连接时图标可点击；0.5×、3.0× 及恢复 1.0× 可用，数值同步；重开 App 和忘记配对后仍保留灵敏度、反馈开关。真实 TapClient 鼠标转换中，同样 10 dp 位移分别产生 10、20、60 px 累计值；PC 配置变化不会覆盖本机倍率 |
| 图标与多指 | 图标点击取消待发轻点、不产生鼠标移动或点击，短视口中的按钮完整位于触控板内；标题、提示与图标不重叠。回归双击、拖拽、滚动、缩放、三指滑动、键盘组合键，以及录音同时操作触控板 |
| 反馈触发次数 | 快捷键短按／持续按住均只反馈一次；全键盘短按一次，长按功能生效再反馈一次；圆形录音长按为按下＋生效两次，方形起停及模式切换各一次；抬手、其他区域手指、禁用快捷键不额外反馈。此项观察回调，不代表已测量手机马达 |
| Go 与 Windows | 官方入口的 Go 测试及 `go vet ./...` 通过，三种 Windows 构建成功。新增配置迁移／保存和服务端快照／更新测试，确认 sensitivity 固定 2、快捷键与滚动方向保留；沿用构建脚本对系统音量及 HID 安装状态两个硬件动作测试的排除 |
| APK 一致性 | Gradle 输出、embed、交付 APK 和两种 EXE 的 `--apk-info` 均为 0.3.4 / code 7，SHA-256 一致。包名 `com.yuncii.tapdeck`、最低 API 26；签名证书与 0.3.3 一致，保留已有配对存储及签名 |

正式构建通过 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.4`；构建日志为 `.tools/ui-validation/windows-0.3.4-build.log`。矩阵中修复了新图标首次测量无点击范围，以及极短区域按钮越界；像素验收按图标与文字区分，并排除底部分隔线。最终各配置日志与截图位于 `dist/0.3.4/screenshots`，受限视口在同一最终 APK 上单独重跑 8 项并通过。

交付 APK SHA-256 为 `bc77be6c732b1095ca67cf0b8c542997f4b2e8afa339970cba4108330278eb32`；签名证书 SHA-256 为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。完整四个产物的校验值见 `dist/0.3.4/SHA256SUMS.txt`。Lucide 新增图标声明已补齐，原始许可继续随 APK 分发。

模拟器显示参数已恢复，隔离的 `TapDeckUiValidation` AVD 已关闭。本轮未安装到实体手机，也未停止或替换用户现有接收端。实际震动强弱、系统触感开关效果、真机覆盖升级及原凭据重连、多台手机同时使用各自倍率的实际手感由用户验收；模拟器的按键、连接和录音回调使用模拟。更新时退出旧托盘并启动新版 EXE，Android 直接覆盖安装 APK，无需卸载旧应用。

## 0.3.3 触控板手势

2026-10-07 实现延迟单击、第二次按下只持有左键、松手才双击、自然滚动符号修正、双指缩放和三指窗口操作。Android 为 `0.3.3` / code `6`，控制协议继续使用 v2。正式交付位于 `dist/0.3.3`，由官方 Windows 构建入口生成，含本次 APK 的单 EXE。行为说明见 [触控板手势](touchpad-gestures.md)。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM | 37 项通过，0 失败／错误，包括 16 项手势状态机测试；覆盖 300 ms 边界、第二次按下／松手、抖动、静止长按、拖拽、取消、电脑双击时间、指针 ID、两指模式锁定、缩放步进及三指一次触发 |
| Android 模拟器 | 五组配置共 23 次交互测试通过；正常、小屏 130% 字体和受限窗口重复验证手势与真实 MotionEvent 适配；语音持续录音时，另一指移动、另两指缩放、另三指窗口滑动均独立工作，语音手指不计入触控板指针 |
| 布局与截图 | 25 张截图，检查普通、全键盘及受限窗口；黑底触控板及两行新提示无重叠，底部两种模式边界及旧键位保持一致。受限窗口下方留白来自测试夹具；极小字号采用抗锯齿像素检测，不要求每个文字像素为纯色 |
| Go 自动测试 | 全部通过；新增 13 项确定性测试覆盖可靠消息屏障、旧能力兼容、非法／已关闭消息、单批 Ctrl＋滚轮、部分失败清理、其他设备 Ctrl／语音键持有、三态转换、外部窗口关闭／选择校正、修饰键冲突与待执行动作撤销 |
| Windows 原生鼠标 | 独立窗口实际收到第二次按下 `WM_LBUTTONDOWN`，松手后才收到 `UP → DBLCLK → UP`；静止长按没有 DBLCLK。缩放收到带 `MK_CONTROL` 的正／负 120 滚轮；临时 Ctrl 释放，已有 RightCtrl 保留 |
| Windows 原生窗口 | Win＋Tab、Esc、Win＋D 的三态转换和重复手势均通过；手动打开任务视图后可下滑关闭，外部 Esc 后状态重新校正；显示桌面后恢复原测试窗口，原本最小化的测试窗口保持最小化。任务视图动画的临时窗口身份变化已处理 |
| 实际浏览器／图片 | 隔离 Edge 配置中，网页及独立图片查看器的 `devicePixelRatio` 均为 `1.00 → 1.10 → 1.00`；直接使用生产 Ctrl＋滚轮发送路径，结束后关闭测试配置并恢复鼠标位置 |
| 构建与静态检查 | Kotlin、Android 主／测试 APK、Windows GUI／诊断／HID 探针构建成功，`go vet ./...` 通过。官方构建跳过修改本机音量、开机自启注册表的两项测试；桌面动作的原生测试默认跳过，另行显式运行 |
| APK 一致性与签名 | Gradle 输出、Windows embed、交付 APK 和 EXE `--apk-info` 均为 code 6 / 0.3.3，SHA-256 一致。包名 `com.yuncii.tapdeck`、最低 API 26，签名证书与 0.3.2 相同，保留原有配对存储格式 |

| 模拟器配置 | 尺寸 / density / 系统字体 | 测试项数 |
|---|---|---:|
| phone-100 | 1080×2400 / 420 / 100% | 7 |
| phone-130 | 720×1280 / 300 / 130% | 2 |
| small-130 | 640×960 / 320 / 130% | 6 |
| tablet-100 | 1404×1872 / 300 / 100% | 2 |
| restricted-100 | 1080×2400 / 420 / 100%，应用视口高度限制 600 px | 6 |

模拟器显示参数已恢复，隔离的 `TapDeckUiValidation` AVD 已关闭。日志与截图见 `dist/0.3.3/screenshots`；Android 测试的电脑连接与录音回调使用模拟，不代表手机到电脑的完整链路已验证。原生环境为 Windows 11 专业版 build 26200、Edge 154.0.4258.62，验收由 `scripts/test-windows-gestures.ps1` 重复执行，日志为 `.tools/ui-validation/native-gestures-0.3.3.log`；正式构建日志为 `.tools/ui-validation/windows-0.3.3-build.log`。用户原有 `dist/0.3.1` 接收端未被停止或替换。

交付 APK SHA-256 为 `302e7ab69d7d50da68174343603d0644c38c61ca6a09d2cf8177285db030dd9f`；新旧签名证书 SHA-256 均为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。四个交付文件的完整校验值见 `dist/0.3.3/SHA256SUMS.txt`。

本轮未安装到实体手机。真机覆盖升级、原凭据重连、手机经网络控制 Windows 的双击／拖拽／缩放／三指操作及不同图片软件的兼容性由用户验收。新手势需要退出旧托盘程序并启动 0.3.3 EXE，再覆盖安装 APK；旧接收端连接仍兼容，但 App 会提示新手势需要升级。

## 0.3.2 键盘反馈与系统音量修复

2026-10-07 完成副键提示放大与居中、统一功能键矢量图标、第四行 Alt 改为 Ctrl，以及 Windows 默认播放设备的音量控制修复。Android 为 `0.3.2` / code `5`，两端控制协议仍为 v2。发布由官方 `scripts/build-windows.ps1 -OutputDirectory dist/0.3.2` 生成；实体手机测试由用户执行。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM 测试 | 21 项通过，0 失败／错误／跳过；包含 Ctrl 与字母组合的持有、释放回归 |
| Android 仪器测试 | 5 组配置执行 18 项，全部通过；1／4／5／8 个快捷键、两种模式边界、33 个键面、连接入口与动画、录音起停、拖动和多指分发均通过 |
| 键盘交互 | Ctrl 短按、真实多指 Ctrl+C、Shift 单次大写、Enter、Shift+Enter、主副键长短按、空格语音和退出释放通过；Ctrl 多指测试在正常、小屏 130% 字体与受限窗口重复执行 |
| 截图检查 | 生成 25 张截图；已检查五组全键盘及普通／受限窗口的八项快捷键截图，副键提示不贴上沿、主副标签无重叠，功能键图标线宽一致，Shift+Enter 组合图标完整显示 |
| Go 测试 | 默认播放设备选择与切换、设备消失后恢复、COM 引用释放、三种输入后端的音量路由、HID 修饰键持有、失败重试及清理通过；实际默认端点的只读测试也执行通过 |
| 实际 Windows 音量 | 独立 Core Audio 读取确认默认音量 `0.3310 → 0.3600 → 0.3400`，只有系统默认播放端点变化；验证后精确恢复到 `0.3310`，全部原静音状态恢复 |
| 构建与静态检查 | Android 主 APK／测试 APK、Windows GUI／诊断／HID 探针 EXE 构建成功，`go vet` 通过；官方自动构建仍跳过改变本机音量及开机自启注册表的两项测试，音量另行按上项验证 |
| 内嵌 APK | 当前 Gradle 输出、Windows embed 目录、交付 APK 与 EXE `--apk-info` 的版本、大小及 SHA-256 一致 |
| 签名与许可 | 包名 `com.yuncii.tapdeck`、最低 API 26，0.3.1 与 0.3.2 签名证书一致；Lucide 完整上游许可随 APK 分发且与源码副本哈希一致 |

模拟器矩阵日志和截图位于 `dist/0.3.2/screenshots`：

| 配置 | 尺寸 / density / 系统字体 | 执行项数 |
|---|---|---:|
| phone-100 | 1080×2400 / 420 / 100% | 6 |
| phone-130 | 720×1280 / 300 / 130% | 2 |
| small-130 | 640×960 / 320 / 130% | 4 |
| tablet-100 | 1404×1872 / 300 / 100% | 2 |
| restricted-100 | 1080×2400 / 420 / 100%，应用视口高度限制为 600 px | 4 |

显示参数已恢复，隔离的 API 34 模拟器已关闭。受限窗口截图下方留白来自测试夹具，并非主面板的高度错误。截图中的电脑连接、快捷键配置与输入／录音回调由测试模拟；手机到 Windows 接收端的完整音量链路、真机覆盖升级及凭据保留仍需用户验收。Windows 音量测试日志为 `.tools/ui-validation/volume-0.3.2-verification.log`，官方构建日志为 `.tools/ui-validation/windows-0.3.2-build.log`。

交付目录 `dist/0.3.2`，APK SHA-256 为 `85fe3899031d9783816a98c9cbe452869eaceb8b73acc8066067451c1ce2c1a7`；其余产物校验值见该目录的 `SHA256SUMS.txt`。新旧 APK 签名证书 SHA-256 均为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。音量修复需要更新 Windows 接收端：退出旧托盘程序，再运行新版 `TapDeck.exe`。

## 历史：0.3.1 Android 主界面与键盘样式

2026-10-07 完成宽度驱动的顶部栏、共享底部容器、浅灰键盘／快捷键／语音样式和连接图标提醒。Android 为 `0.3.1` / code `4`，控制协议仍为 v2。尺寸及样式定义见 [android-ui.md](android-ui.md)。本轮实体手机由用户测试；使用隔离的 API 34 模拟器验证布局和交互，没有安装到用户手机或替换正在运行的 PC 接收端。

| 验证项 | 结果 |
|---|---|
| Kotlin JVM 测试 | 21 项通过，0 失败；覆盖正常／受限／极小视口、语音拖动边界、3 秒提醒周期及取消，以及键位与配对凭据回归 |
| 安全视口布局与截图 | 5 组配置全部通过，生成 25 张截图；两种模式底部边界一致，1／4／5／8 个快捷键与 33 个键面未重叠，受限窗口的触控板标题和手势提示均完整显示 |
| 连接图标 | 橙色跳动、绿色静止和复位、后台停止、点击打开连接设置通过 |
| 按键与多指 | 短按主键、长按副键、空格语音、按住键释放及组件销毁清理通过；原生父容器同时分发语音和触控板手指，录音起停、模式切换和拖动通过 |
| 构建 | Android APK、Android 测试包、Windows GUI／诊断 EXE 构建成功；Go 测试及 `go vet` 通过，官方入口仍跳过会修改本机音量和开机自启注册表的两项测试 |
| APK 一致性 | 当前 Gradle 输出、Windows embed 目录、交付 APK 和 EXE `--apk-info` 的 SHA-256 一致；包名 `com.yuncii.tapdeck`、最低 API 26、新旧签名相同 |
| 许可声明 | Lucide 完整上游许可同时保留在源码与 APK assets 中，内容哈希一致 |

模拟器矩阵共执行 18 次测试，日志与截图见 `dist/0.3.1/screenshots`：

| 配置 | 尺寸 / density / 系统字体 | 执行项数 |
|---|---|---:|
| phone-100 | 1080×2400 / 420 / 100% | 6 |
| phone-130 | 720×1280 / 300 / 130% | 2 |
| small-130 | 640×960 / 320 / 130% | 4 |
| tablet-100 | 1404×1872 / 300 / 100% | 2 |
| restricted-100 | 1080×2400 / 420 / 100%，应用视口高度限制为 600 px | 4 |

矩阵通过 `scripts/test-android-ui.ps1` 重复执行；显示参数在结束时恢复，临时模拟器随后关闭。限制高度的截图中，下方留白是测试夹具模拟小窗口的结果，不属于控制面板。测试模拟电脑连接、快捷键配置和输入／录音回调，未连接真实电脑输入或音频设备。

当前交付目录为 `dist/0.3.1`，Android APK SHA-256 为 `6eec671e5c395b51e469645955b33c9503373d9dd80fe46b4317edb541ca8095`；其余产物校验值见该目录的 `SHA256SUMS.txt`。新旧 APK 签名证书 SHA-256 均为 `7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30`。

**尚待用户真机验收**：实际手机覆盖升级和原配对凭据保留、不同设备的触感与字体表现、真实 PC 按键／手势效果及语音输入链路。API 26–33 与 35 以上版本没有执行本轮设备测试；最低版本由构建配置保持。历史设备结果不作为 0.3.1 真机结论。

## 历史：0.3.0 安装引导、设备管理与虚拟声卡集成

本轮两端版本为 0.3，控制协议仍为 v2。完成 Go／Kotlin 单元测试、`go vet` 和官方两端构建；旧 APK 替换、Android 构建失败中止 PC 构建、内嵌及 HTTP 下载哈希、APK 包名／版本／签名、原包完整解出及离线签名均验证通过。本机 VB-CABLE 检测为可用，没有重复安装。详细自动测试范围和待验收条件见 [installation.md](installation.md)。

新产物位于 `dist/0.3.0`，当前旧 `dist/TapDeck.exe` 正在运行。未关闭用户接收端或修改现有配对配置。没有可用干净 Windows 11 虚拟机，ADB 没有可用的已授权设备；因此新版本真实覆盖升级及凭据保留、系统扫码／网页唤起、干净环境离线安装／管理员取消／重启后端点可用和手机传音仍待实机验收。以下记录属于此前版本，不作为本轮通过项。

## 历史：0.3 Windows 虚拟键盘更新（Android 0.2）

最终 Windows EXE 内置原版签名 FakerInput 0.1.1 x64 MSI、MIT 许可证与版本清单，提供自动 / HID / SendInput 选择、安装 / 修复入口和实际后端状态。Android APK 仍为 0.2，控制协议仍为 v2。详细说明见 [虚拟键盘验证](virtual-keyboard.md)。

- 官方 MSI 安装成功，设备状态 OK，API v1，Secure Boot 与内存完整性保持开启，无需重启。
- 实际 Raw Input 设备事件与键盘钩子标记检查通过：关联设备事件没有 `LLKHF_INJECTED`。用户现场确认虚拟键盘的右 Ctrl+M 长按及右 Ctrl+L 免按均可触发豆包。
- 2026-10-07 复查：定位并修复“组合键录入不区分左右修饰键”的缺陷（按右 Ctrl+M 会记录成 `Ctrl+M`），同机复测虚拟键盘 `RightCtrl+M` 连续 8 次触发豆包语音、`LeftCtrl+M` 不触发、SendInput 不触发；豆包语音只接受真正获得焦点的输入框。证据与操作说明见 [豆包语音热键实测](voice-hotkey.md)。
- 2026-10-07 特殊按键：定位并修复“后退键 / ESC / 音量键保存不了”（录入对话框用了 walk 的按键名，与解析器名称表不一致，音量键则完全缺失），改为统一按键表；音量加减与静音实测无法通过 SendInput 注入（六种编码系统均无反应），改由 Core Audio `IAudioEndpointVolume` 的 `VolumeStepUp` / `VolumeStepDown` / `SetMute` 实现。详见 [特殊按键](special-keys.md)。
- 2026-10-07 全键盘：状态栏新增「全键盘」开关，激活后下方快捷键区与语音区换成全键盘（新消息 `key_down` / `key_up` 与 `internal/keyboard` 的 `KeyAsync`）。按上传的截图改为 4 行双键位布局（短按主键位、长按副键位），并修掉「关闭键盘时底部空出一大截」：原来的占位视图也带权重，键盘关闭时仍分走约三分之一高度，改为两种模式下只保留一组可见子视图、由 LinearLayout 按可见权重归一化。用 `cmd/keywatch`（轮询 `GetAsyncKeyState`，与焦点无关）实测：字母 `a s j` 正确、Shift 长按锁定后字母带 Shift 且 Shift 不释放、退格轻点 51 ms / 按住 1.2 s 保持、回车轻点 56 ms、`Shift+Enter` 轻点 9 ms / 按住 1.1 s 期间 `Shift` 与 `Enter` 一直保持按下（即长按连续 ⇧⏎）。旧语义下的三条记录已被后续改动取代，保留在此说明当时的问题：按住 `k` 会先出 `K` 再改发 `'`、空格按住 409 ms 会先出空格再切 `Shift+Enter`、句点长按 2 s 会触发语音长按热键。标点键补齐 USB HID usage（原先整条组合键会退到豆包不认的 SendInput）；键盘上没有的 `…` 新增 `Ellipsis`（`KEYEVENTF_UNICODE`，自动走 SendInput）。`go test ./...` 新增 `TestUnicodeOnlyKeyFallsBackToSendInput`、`TestPunctuationKeysUseHID`、`TestUnicodeOnlyKey`；`gradlew testDebugUnitTest` 新增 `KeyHoldTest` 8 项。按键宽度改为按屏幕宽度固定（常规键 8.5%、间隙 1.5%、`[Shift]`/`[Backspace]`/`[Shift+Enter]` 13.5%、`[alt]`/`[Enter]` 18.5%、空格 33.5%，两侧各留 0.75%，2 行 9 键不铺满）；截图逐像素量得常规键 118 px＝8.4%、间隙 21 px＝1.5%、`[Shift]`/`[Backspace]` 188 px、`[alt]`/`[Enter]` 258 px、空格 469 px、1/3/4 行右边缘均在 x≈1390（屏宽 1404）。`b3c24ae` 起再调整：4 行第 4 格改为主副共用 `Shift+Enter`（长按连续 ⇧⏎，实测按住 1.1 s 期间 `Shift`/`Enter` 一直保持按下）；2 行整行居中、一格内两个键位都居中且字号加大、描边加深、特殊键用 `#CCC` 灰底；`. ` 的副键位改为 `,`（键盘里不再有语音键）；双键位键改为「短按只发主键位一次、长按只发副键位一次」，长按不再先冒出一个主键位、副键位也不再连发（`KeyHold` 拆分 `dualShort` / `dualLong`，长按判定留在手势层）；空格键的长按最终定为「长按语音输入」：短按一次空格，长按同时开始传音并保持 PC 端长按热键，松手结束，键面在传音期间变蓝（真机实测：按住 1.6 s 时不出现空格，PC 端 `RightAlt` 被按住 1.5 s）；`. ` 的长按为 `,`。`gradlew testDebugUnitTest` 的 `KeyHoldTest` 同步改为校验新语义。平板重新连接后已按上表完成真机复测（字母长按只出副键位一次、空格长按传音、Shift 锁定、退格连续、2 行居中与灰底样式）。
- 2026-10-07 Android 接入 Lucide 图标并搬家模式切换：状态区去掉语音输入方式开关，改为只放一个 Lucide `keyboard` 图标按钮（未激活 `#AAA`、激活 `#000`，点击切换全键盘）；语音输入方式的切换移入语音区顶部的胶囊按钮（长按＝`mic-audio-lines`、单击＝`mic-signal`），按钮右侧跟随当前提示「按住说话，松手结束」/「轻点开始，再点结束」。三个图标按 Lucide 原图转成矢量 drawable（`ic_lucide_keyboard` / `ic_lucide_mic_audio_lines` / `ic_lucide_mic_signal`，24 dp、线宽 2、圆角端点）。两种模式都写入 DataStore（`voice_mode` / `keyboard_mode`）并在启动时恢复。真机实测：点按钮在长按↔单击之间切换（图标、文字、提示与控件形状同步变成方形「轻点」），点键盘图标打开全键盘且图标由 `#AAA` 变 `#000`；重开 App 后键盘保持激活、语音保持单击模式（状态区图标采样 grey=490 / black=0 对应未激活）。模式按钮行顶部留白减半（`topInset()/2`）：实测按钮上沿距语音区上沿 28 px ≈ 14.9 dp，此前约 63 px；点击移动后的按钮仍能正常切换模式与控件形状。
- 2026-10-07 配对只由 PC 确认 + 一台 PC 接 5 个控制端：Android 端不再需要点“与电脑一致”，收到 `pair_challenge` 后直接进入“等待电脑允许连接”（仍自动回一条 `pair_confirm` 以便兼容老接收端，新接收端收到即忽略）；接收端 `Server.current` 改为 `sessions map[uint64]*session`，上限 `server.MaxSessions = 5`，名额检查放在配对之前（满了直接回 `too_many_clients`，同一台设备重连先替换旧会话），UDP 按会话 id 路由、配置更新广播给所有会话、断开只中断自己的采音。真机 + `cmd/ctrlprobe` 实测：平板 + 4 个模拟控制端同时在线时状态行显示「已连接 5 个控制端：Probe-1、Probe-2、Probe-3、Probe-4、Sony XQ-DQ72」，第 6 个立刻收到「被拒绝：接收端最多同时连接 5 个控制端，请先断开其中一个」；只发 `hello` 的控制端在 PC 点「校验码一致，允许」后直接打印「已建立会话：e3d00800147c89ad」，状态行变为「已连接 2 个控制端：PairLive、Sony XQ-DQ72」。`go test ./internal/server/` 新增 `TestMultipleControllersUpToLimit`、`TestSameDeviceReplacesItsSession`、`TestPacketsRouteToOwningSession`、`TestConfigUpdateReachesEverySession`、`TestPairWithoutClientConfirmation`。详见 [配对确认与多控制端](multi-controller.md)。
- 2026-10-07 配对网页内置 APK 下载：编译好的 APK 通过 `internal/apkdist`（`//go:embed assets`）打进接收端，`scripts/build-windows.ps1` 构建前把 `dist\TapDeck-debug.apk` 复制成 `assets\TapDeck.apk`（未找到时只警告，不含内置包也能编译；`assets/*.apk` 不入库）。新增 `GET /apk`（`application/vnd.android.package-archive`、`filename="TapDeck.apk"`、支持 Range 断点续传，未内置时 404 + Release 提示）与 `GET /apk/qr.png`（`go-qrcode` 512 px，内容为 `http://<请求 Host>/apk`），配对页 `/pair` 按占位符注入「下载 Android 端」区块。实测：EXE 由 12.8 MB 增至 24.4 MB（内嵌 10.93 MB APK）；`GET /apk` 返回 11460424 字节且与 `dist\TapDeck-debug.apk` 的 SHA-256 一致；配对页含二维码与下载按钮；平板 Chrome 打开配对页显示二维码区块，平板 curl 从 `/apk` 下载后 `sha256sum` 与 PC 端一致。详见 [内置 APK 与扫码下载](apk-download.md)。
- 2026-10-07 接收端 `--headless` 补托盘：此前 `--headless` 只跑服务端、不创建窗口与托盘（开机自启或手动用该参数重启后，进程在跑但托盘没有图标，双击 EXE 只弹「已运行」提示框，无从打开设置）。现在两种模式都会创建窗口与托盘，`--headless` 只是把设置窗口隐藏（`mw.Hide()`），点击托盘再显示。实测 `--headless` 启动后主窗口 `IsWindowVisible=false`、日志出现「已常驻托盘，设置窗口未显示」且没有「托盘图标创建失败」。关闭窗口仍然只是隐藏，右键托盘「退出」结束进程。
- 2026-10-07 快捷键按住：快捷键按钮改为按下即按下、松手即抬起（新增 `shortcut_hold_start` / `shortcut_hold_stop` 消息与 `session.holds`）。实测按住「上」时 `GetAsyncKeyState` 连续 3.5 秒报告 `Up` 按下，松手立即释放；轻点为一次完整按下 / 抬起。`go test ./...` 含新增的 `TestShortcutHoldPressesAndReleases`。详见 [特殊按键](special-keys.md)。
- 2026-10-07 Android 界面：语音输入方式开关移到顶部状态区（「连接」旁，文字与「连接」同号），默认长按语音输入＝圆形控件、单击语音输入＝方形控件（形状只由开关决定，切换立即生效）；四个区域固定为 10% / 40% / 25% / 25%，录音控件在最后 25% 内自由拖动；保留系统状态栏（时间/电量可见）；单击录音期间按任意快捷键会先结束录音；触控板黑底白字，语音区偏深米黄背景（`#FDE6AF`），底部提示行与「麦克风电平」共用一个位置、电平条在其下方；状态区两个开关之间以及与「连接」之间各留 16 dp 间隙（此前语音开关与「全键盘」开关紧挨在一起，实测两开关间距约 24 dp、不再粘连）。真机（Tab8C / Android 11）实测区域像素、开关切换、方形/圆形控件、轻点起停、长按录音、拖动与快捷键停录均通过，详见 [Android 语音模式](android-voice-mode.md)。
- 2026-10-07 自动连接：Android 端启动、网络变化、回到前台时都会按 1 s→30 s 退避重试保存的配对（不再要求已持有 token）；PC 端新增随 Windows 登录自启动（注册表 `HKCU\...\Run` + `--headless`，设置页可勾选，命令行 `--autostart-on/off`）。实测重开 App 约 6 s、重启接收端后约 8 s、模拟开机自启后 2 s 内自动恢复连接，详见 [自动连接](auto-connect.md)。
- 新接收端配合 Tab8C 完成 100 次混合语音操作（83.181 秒），覆盖拖动、取消、后台与断线重连；采音正常停止。
- 实际 Android 长按期间强制终止主进程，持有的 Ctrl+M 约 62 ms 内释放，工作进程退出，Android 断线停止采音。
- 六个左右修饰键按下 / 释放通过；长按 Ctrl+M 期间发送 Ctrl+C、Ctrl+F24 未释放持有的 Ctrl / M，最终释放正确。
- 已验证从 EXE 离线解出原版 MSI，哈希一致、离线 Authenticode 校验通过。重复启动显示已有实例提示。配对文件哈希未变化。
- `go test ./...` 与 `go vet ./...` 通过，含异步准备取消、迟到就绪、跨后端引用计数、快照、重复停止、设备写入失败及管道断开清理。

两次各 60 秒的同时传音 / 鼠标复测均保持连接、结束后采音停止，最大音频缓冲 6 帧。控制 RTT 中位数 / p95 / 最大值分别为 **11 / 29 / 68 ms** 和 **12 / 27 / 54 ms**。PC 鼠标处理 p95 第一次为 **2.124 ms**，复测后最后 4,096 个样本的滚动窗口为 **4.223 ms**。PC 处理满足 5 ms 目标，控制 RTT 本次未达到 20 ms 目标，不把历史 0.1 的较低结果当作本次验收。

根据用户要求停止追加测试，剩余交互与输入法验收由用户人工进行。尚未完成手机传音识别文字、Android 三种入口的豆包效果及各 10 轮应用内起停、实际复制 / 粘贴、安装取消 / 要求重启场景、完整音频延迟及新版本 30 分钟长测。100 次测试证明采音和键盘生命周期正常，不代表完成了 100 次豆包识别。

本机 Tab8C 固件在部分设备测试结束后会冻结主 APK（Package enabled=3）；已使用 `adb shell pm enable com.yuncii.tapdeck` 恢复应用。人工验收时若应用无法启动，请在设备中取消冻结。最终保留用户现有配置及配对，不用测试默认配置覆盖。

## 环境

| 项目 | 本次环境 |
|---|---|
| Windows | Windows 11 Pro x64，Build 26200 |
| 接收端 | Go 1.26.4，CGO_ENABLED=0，Walk 原生窗口及托盘 |
| Android | ONYX Tab8C，Android 11 / API 30，无摄像头，1404×1872，density 300 |
| Android 构建 | JDK 17.0.20.1，Gradle 9.3.1，AGP 9.1.1，SDK Platform 37.0，Build Tools 36.0.0 |
| 网络 | 实际 Wi-Fi：PC 192.168.1.11，Android 192.168.1.7；USB 仅用于安装、ADB 与测试控制 |
| 虚拟声卡 | 已安装 VB-CABLE Pack 45；WASAPI 枚举到 CABLE Input 与 CABLE Output |

本机管理员安装已成功完成，日志为 `.tools/admin-setup.log`。曾发生的日志占用由同时使用 Start-Transcript 和 Add-Content 写同一个文件导致，脚本已修复。没有执行系统重启。

## 0.2 语音圆球与快捷键扩展

本次交付版本为 0.2.0，两端使用控制协议 v2，UDP 编码保持不变。Windows 和 Tab8C 均已更新。按用户要求，剩余交互体验和输入法效果交由人工验收，未继续追加性能长测。

| 验证项 | 结果 |
|---|---|
| Go 构建、单元与集成测试 | `go test ./...`、`go vet ./...` 通过；包含配置迁移、左右修饰键扫描码与引用计数、录音配置快照、重复停止、迟到结束回调、禁用与过期快捷键、v1 连接升级提示 |
| Android 构建与 JVM 测试 | 0.2 APK 构建成功，5 个协议 / 配置 / 布局比例测试通过 |
| 真机回归套件 | 最终安装包对应的设备测试通过：报告 13 项，其中 10 项执行、3 项按条件跳过（外部按键探针、接收端重启、长测）；日志 `.tools/android-v2-final-device.log` |
| 100 次混合语音操作 | 真机通过；交替长按与免按，包含拖动、取消、后台和主动断开再连接；每轮恢复空闲，AudioRecord 不再运行 |
| 准备中停止 | 10 次快速停止测试通过，迟到的就绪消息未启动采音 |
| 圆球手势与位置 | 命中范围、单击、双击、300 ms 长按、拖动、边界限制、触摸取消、独立手指跟踪及重启后位置恢复通过 |
| 圆球与触控板同时操作 | 原生多指分发测试通过，另一根手指操作触控板不会结束圆球录音 |
| 六个左右修饰键 | 使用 PC 下拉配置，经 Android Wi-Fi 长按触发，Windows `GetAsyncKeyState` 实际观察到左 / 右 Alt、Ctrl、Shift 按下及释放；六项均通过，结果 `.tools/v2-key-results.json` |
| 1 / 4 / 5 / 8 个快捷键 | PC 勾选并保存后，Android 数量、单行 / 双行布局、原槽位映射及区域边界通过；5 项用槽位 1、2、3、4、8，覆盖中间关闭槽位 |
| 多尺寸与字体 | 8 个快捷键下，720×1280 / density 300 / 字体 130%，以及 640×960 / density 320 / 字体 130% 的布局边界和无重叠断言通过，截图确认各区完整、文字和按钮未溢出；原生尺寸设备回归通过 |
| 旧配置与凭据 | 旧四槽位和语音模式迁移、原文件备份、保存重读由 Go 测试验证；现有 Tab8C 凭据保留并自动重连 |

本次结束时已恢复 Tab8C 物理分辨率 1404×1872、density 300、字体 100%。PC 接收端保持运行，Android 已恢复启动。快捷键恢复默认前四项启用，三个语音热键留空，便于人工选择输入法热键。

### 人工验收入口

1. PC“语音”页将长按热键选为“右 Alt”，输入法麦克风选 **CABLE Output**；在需要输入文字的应用里按住圆球、说话、松开，检查豆包起停和识别。
2. 根据输入法规则配置免按开始 / 结束热键；双击圆球开始，再单击圆球结束，检查两次热键动作和识别。
3. 拖动圆球到语音区边缘、重启 App，检查位置恢复；录音时操作触控板，再尝试后台、熄屏和断网，检查是否停止。
4. PC 勾选 1–8 项、关闭中间槽位，保存同步；在普通桌面应用中检查按钮数量、名称和实际组合键效果。

**豆包实际识别和免按起停语义尚未验收。** 左右修饰键的系统按下 / 释放已验证，但目标输入窗口未能获得前台焦点，因此没有把物理按键验证等同于输入法效果。完整采集到虚拟麦克风的音频延迟仍未测得。

以下为 0.1 原型的历史验证；其长测数值不代表 0.2 的新增实测结果。

## 0.1 历史功能与回归

| 验证项 | 结果 |
|---|---|
| Go 构建、单元与集成测试 | 通过 `go test ./...`，`go vet ./...`；EXE 无 CGO |
| Android 构建与 JVM 测试 | APK 构建成功，4 个协议 / 配置测试通过 |
| 编码与非法输入 | Go/Kotlin AES-GCM 固定向量相同；篡改、重复、乱序、丢包累计恢复及跨通道鼠标边界测试通过 |
| 模糊测试 | UDP 解码约 66 万次输入，未发生崩溃 |
| URL 配对 | Tab8C 手输网址，两端核对校验码、PC 允许、保存凭据、自动重连已验证 |
| 配对撤销与超时 | 本机 TLS/WSS 集成测试覆盖拒绝、解绑后旧令牌失效、停止接收时取消待配对、心跳超时释放输入 |
| 实际鼠标 | 真机滑动使 PC 光标移动；触控板手势测试覆盖左击、右击、双击拖拽取消、双指滚动 |
| 录音与触控板同时操作 | 实际多指事件注入通过；按住语音键时另一个手指移动鼠标，不产生错误滚动 |
| 四个快捷键配置 | PC 修改名称、保存后，Android 实际画面同步显示新名称；测试后恢复默认配置 |
| 录音停止 | 真机 AudioRecord 连续启动 / 停止 10 次，停止后不再产生采样回调 |
| 100 次语音操作 | 真机通过，包含松手、ACTION_CANCEL、每 10 次切到后台再恢复；每轮回到空闲 |
| 接收端中断 | 传音时停止 PC 接收：Android 停止采集；启动接收后自动重连，能够再次开始及结束录音 |
| 应用主动断开 | 录音时主动断开，再从已保存凭据恢复连接；旧录音编号清除，能够重新录音及停止 |
| 音频设备缺失 | 使用不存在的输出端点做 TLS/WSS 集成测试，收到明确 mic_error；4 个快捷键仍进入注入器（模拟注入器） |
| 熄屏 | 设备无安全锁；实际发送 KEYCODE_SLEEP 后录音回到空闲，唤醒后保持空闲 |
| 麦克风权限拒绝 | 实际点击系统“拒绝”：PC 音频包为 0；滑动触控板仍收到鼠标包并保持连接。测试后恢复录音权限 |
| Windows 状态界面 | 固定 UI 的 OS 线程后，连接状态及鼠标 / 音频统计持续刷新；刷新回调最多排队一次 |
| Windows 退出 | 通过程序的退出动作结束接收与进程；关闭窗口隐藏到托盘 |

设备测试源码：`android/app/src/androidTest/java/com/yuncii/tapdeck/DeviceTest.kt`。30 分钟测试和 PC 中断测试需要传入对应参数；日常测试运行时跳过它们。

## 0.1 历史屏幕适配

固定区域为连接 10%、触控板 50%、快捷键 20%、语音 20%，没有滚动容器。

| 配置 | 检查 |
|---|---|
| 原生 1404×1872，字体 100% | 宽度与高度比例断言通过，实际界面无重叠 |
| 模拟 720×1280，density 300，字体 130% | 比例断言通过，截图检查四区完整、按钮可触达 |
| 模拟 640×960，density 320，字体 130% | 比例断言通过，文字与四个按钮未越出区域 |

Tab8C 固件拒绝写入 160% 和 200% 字体缩放，因此不声称这些缩放已在本设备验收。测试结束已恢复物理分辨率、原始密度和字体 100%。

## 0.1 历史音频信号与性能

实际链路为手机 AudioRecord → Wi-Fi UDP → PC 抖动缓冲 → WASAPI → CABLE Input → CABLE Output。在 CABLE Output 录音端捕获 6 秒，得到 287,040 个样本，peak 1,958、RMS 203.89（16-bit 标度），证明实际传音链路有信号。该探针只输出统计，不保存录音。

30 分钟测试同时持续采音及约每 16 ms 发送一次相对鼠标位移，结果：

| 指标 | 实测 |
|---|---:|
| 持续时间 | 1,800 秒 |
| RTT 采样数 | 6,949 |
| 控制 RTT 中位数 / p95 / 最大值 | 9 / 15 / 117 ms |
| PC 收到有效鼠标包至注入处理结束 p95 | 约 1.516 ms |
| 抖动缓冲历史最大 | 6 帧，即 60 ms |
| 连接 | 全程保持 |
| 结束后录音 | 正常停止，缓冲回到 0 |

RTT 使用客户端单调时钟测量 WSS 心跳往返，每约 250 ms 读取最新值。PC 处理延迟包含解密、解析与 SendInput 调用，以最后 4,096 个样本的窗口统计；不包含 Windows 应用最终呈现的延迟。

30 分钟测试完成后修正了 Android 多指分发与 Windows 状态刷新。相同传输 / 音频处理实现再做 60 秒复测：RTT 中位数 14 ms、p95 19 ms、最大 30 ms；PC 处理 p95 约 1.511 ms，缓冲历史最大 5 帧，连接保持，结束正常停止。

详细日志和 JSON 在本机 `.tools/android-soak-final.log`、`.tools/soak-1800.json`、`.tools/android-final-soak60.log`、`.tools/soak-final60.json`。补静音统计包含录音起停及正常尾音排空，不应直接当作网络丢包率。

## 尚未验收的部分

- **完整音频延迟**：尚未测出 AudioRecord 采集时刻至 CABLE Output 录音端的 p95，不能声称已达到 150 ms 或 80 ms。需要带采集时间标记、时钟校准和虚拟声卡录音端检测的独立测量。
- **快捷键应用内效果**：0.2 已验证 1–8 项配置同步及槽位映射；实际复制、粘贴、撤销、回车等效果仍需在用户选定的普通桌面应用中验收。
- **具体语音输入法**：0.2 已实际验证六个左右修饰键按下 / 释放；豆包识别以及长按、免按起停效果交由用户人工验收。应将其麦克风选择为 CABLE Output，并配置对应热键。
- **摄像头扫码、其他 Android / Windows 设备**：无摄像头的 Tab8C 已验证网址入口；摄像头扫码与其他硬件尚未实机验收。
- **Android 37 局域网权限与安全锁屏**：已有对应权限处理及 onStop 停音逻辑；本次真机为 API 30，且没有 PIN / 密码锁。

## 交付

本次交付 0.2.0：`dist/TapDeck.exe` 为 Windows GUI 接收端，`dist/TapDeck-debug.exe` 提供控制台诊断，`dist/TapDeck-debug.apk` 为开发签名 APK。`dist/TapDeck-prototype.zip` 包含上述程序、使用说明、协议和本验证记录；`dist/SHA256SUMS.txt` 提供校验值。依赖、构建和防火墙脚本保留在源码中，VB-CABLE 驱动单独安装，不随包分发。

最终交付时 PC 接收端保持运行、Android 保持配对且麦克风空闲。持久凭据留在受保护的本机应用目录。

</details>
