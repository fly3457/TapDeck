# 第三方组件与许可证

TapDeck 自有源码采用根目录 [MIT License](LICENSE)。第三方组件保留各自的许可证、版权及声明，不因项目的 MIT 许可证而改变。依赖的准确版本由 `windows/go.mod`、`windows/go.sum` 及 Android Gradle 配置定义。

## Windows

| 组件 | 版本 | 许可证 | 保留的原文 |
|---|---|---|---|
| [Go](https://go.dev/) 标准库与运行时 | 1.26.4 构建 | BSD-3-Clause | [LICENSE](licenses/Go-LICENSE.txt)、[PATENTS](licenses/Go-PATENTS.txt) |
| [coder/websocket](https://github.com/coder/websocket) | 1.8.15 | ISC | [LICENSE](licenses/coder-websocket-LICENSE.txt) |
| [go-ole](https://github.com/go-ole/go-ole) | 1.3.0 | MIT | [LICENSE](licenses/go-ole-LICENSE.txt) |
| [Walk](https://github.com/lxn/walk) | c389da54e794 | BSD-3-Clause | [LICENSE](licenses/walk-LICENSE.txt) |
| [win](https://github.com/lxn/win) | a377121e959e | BSD-3-Clause | [LICENSE](licenses/win-LICENSE.txt) |
| [go-qrcode](https://github.com/skip2/go-qrcode) | da1b6568686e | MIT | [LICENSE](licenses/go-qrcode-LICENSE.txt) |
| [golang.org/x/sys](https://cs.opensource.google/go/x/sys) | 0.48.0 | BSD-3-Clause | [LICENSE](licenses/x-sys-LICENSE.txt)、[PATENTS](licenses/x-sys-PATENTS.txt) |
| [govaluate](https://github.com/Knetic/govaluate) | 3.0.0 | MIT | [LICENSE](licenses/govaluate-LICENSE.txt) |
| [rsrc](https://github.com/akavel/rsrc)，构建资源工具 | 0.10.2 | MIT | [LICENSE](licenses/rsrc-LICENSE.txt) |
| [FakerInput](https://github.com/Ryochan7/FakerInput/releases/tag/v0.1.1)，内嵌驱动安装包 | 0.1.1 x64 | MIT | [LICENSE](windows/internal/driver/assets/LICENSE.FakerInput.txt)、[分发副本](licenses/FakerInput-LICENSE.txt) |
| [VB-CABLE](https://vb-audio.com/Cable/)，内嵌完整官方原包 | Driver Pack45 | VB-Audio donationware / 专有许可 | [原包 readme](windows/internal/vbcable/assets/LICENSE.VB-CABLE.txt)、[官方分发条件](https://vb-audio.com/Services/licensing.htm) |

内嵌 MSI 是未修改的上游签名安装包；来源、发布者及 SHA-256 记录在 `windows/internal/driver/assets/manifest.json`。HID 客户端依据公开的报告格式使用 Go 实现，接口参考 [FakerInputDll](https://github.com/Ryochan7/FakerInputDll)，不分发其源码或客户端 DLL。上游已归档，当前版本固定使用。

## Android 与构建工具

以下组件使用 Apache License 2.0，原文见 [Apache-2.0.txt](licenses/Apache-2.0.txt)。上游发行文件中的许可证及 NOTICE 保持原样；Android 依赖通过 Gradle 获取，没有把其源码改为 MIT。

已解析的运行依赖清单见 [android-runtime-components.json](licenses/android-runtime-components.json)，从对应发行 JAR / AAR 中提取的声明保留在 [licenses/android/](licenses/android/)。清单包含平台与依赖约束元数据，不表示每一项都独立打入 APK。部分组件的 POM 通过上游父 POM 定义许可证，因此清单中可能没有单独的许可证字段。

| 组件 | 使用版本 | 上游 |
|---|---|---|
| AndroidX / Compose / Activity / Lifecycle / DataStore | Compose BOM 2025.04.01；其他版本见 `android/app/build.gradle.kts` | [AndroidX](https://android.googlesource.com/platform/frameworks/support/) |
| Kotlin 标准库、Compose / Serialization 编译插件 | 编译插件 2.2.10；标准库由 Gradle 解析 | [Kotlin](https://github.com/JetBrains/kotlin) |
| kotlinx.coroutines | 1.10.2 | [Coroutines](https://github.com/Kotlin/kotlinx.coroutines) |
| kotlinx.serialization | 1.9.0 | [Serialization](https://github.com/Kotlin/kotlinx.serialization) |
| OkHttp 与其 Okio 依赖 | OkHttp 5.5.0 | [OkHttp](https://github.com/square/okhttp)、[Okio](https://github.com/square/okio) |
| ZXing Android Embedded 与 ZXing Core | 4.3.0 / 3.4.1 | [JourneyApps](https://github.com/journeyapps/zxing-android-embedded)、[ZXing](https://github.com/zxing/zxing) |
| Gradle Wrapper | 9.3.1 | [Gradle](https://github.com/gradle/gradle) |
| Android Gradle Plugin，外部构建工具 | 9.1.1 | [Android Tools](https://android.googlesource.com/platform/tools/base/) |

DataStore 的 `androidx.datastore:datastore-preferences-external-protobuf:1.1.7` 使用 **BSD-3-Clause**，其上游 Protobuf 原文另见 [Protobuf-LICENSE.txt](licenses/Protobuf-LICENSE.txt)。该组件不适用上表的 Apache 2.0 分组说明。

Gradle Wrapper 的原始版权和许可头保留在 `android/gradlew` 与 `android/gradlew.bat` 中。测试工具 JUnit 4（EPL-1.0）、Hamcrest（BSD）及 AndroidX Test 不属于主 APK 的运行依赖，由依赖管理器获取。

## Lucide 图标

Android 使用 Lucide 的 `keyboard`、`mic-audio-lines`、`mic-signal`、`plug`、`arrow-big-up`、`corner-down-left`、`delete`、`sliders-horizontal`、`scan-line` 和 `chevron-right` 矢量图标；Shift+Enter 组合 `arrow-big-up` 与 `corner-down-left`。图标适用 ISC 许可及其原始版权声明。完整上游原文（含 Feather 来源附录）保留在 [Lucide-LICENSE.txt](licenses/Lucide-LICENSE.txt)，同一份声明也随 APK 的 `assets/Lucide-LICENSE.txt` 分发。

## VB-CABLE 原包与分发

VB-CABLE 的作者和来源是 **VB-Audio / Vincent Burel**，官网 [vb-audio.com](https://vb-audio.com/Cable/)。它是 **donationware**，欢迎通过[官方捐赠 / 购买页面](https://vb-audio.com/Services/licensing.htm)支持作者。TapDeck 的 MIT 许可证不适用于 VB-CABLE。

单 EXE 内嵌未修改的 `VBCABLE_Driver_Pack45.zip`，来源、大小与 SHA-256 见 [manifest.json](windows/internal/vbcable/assets/manifest.json)。解包保留全部文件及原始 `readme.txt`，由用户打开官方安装向导。语音页提供来源、donationware 说明、原文许可和官网入口，可经官网找到捐赠及许可购买途径。[官方分发条款](https://vb-audio.com/Services/licensing.htm)要求让用户识别来源、了解 donationware 模式并能捐赠或付费，未指定必须使用独立按钮；据此，0.3.12 移除单独的“捐赠 / 购买”按钮，其他说明与入口保留。本轮用途为个人及公开发布，企业部署应按该页面的相应许可方案处理。

原包 readme 中关于作者授权的限制原文保留；公开分发以官网当前分发条件为依据。安装需要管理员授权并按[官方安装说明](https://vb-audio.com/Cable/VBCABLE_ReferenceManual.pdf)重启 Windows。TapDeck 不修改驱动、不添加自签名证书、不更改默认麦克风。

豆包输入法等语音识别应用由用户单独安装，不随 TapDeck 分发。

分发 TapDeck 时，请同时保留项目 LICENSE、本文及 `licenses/` 下对应声明。交付 ZIP 已包含这些文件；内嵌 FakerInput MSI 的许可证也保留在 Windows EXE 内。
