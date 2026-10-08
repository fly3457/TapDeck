# 版本管理与每次迭代交付

本文件和根目录 `AGENTS.md` 是项目长期规则。每次完成迭代都要修改版本、构建验证并提交 Git；同一迭代内的重复编译、测试和失败重试沿用已确定的版本。此规则从 0.3.13 开始执行，包含只调整打包或项目规则的迭代。

## 唯一版本来源与两端独立版本

根目录 `version.properties` 是唯一需要修改的版本数据文件：

| 字段 | 当前值 | 用途 |
|---|---|---|
| `receiver.version` | `0.3.16` | Windows 接收端及随包诊断工具版本 |
| `controller.android.version` | `0.3.16` | Android 用户可见版本、APK 文件名 |
| `controller.android.versionCode` | `19` | Android 覆盖安装时的单调递增序号 |

接收端与控制端已经可以使用不同版本，例如下一轮只改接收端时，可由接收端 `0.3.17` 嵌入 Android `0.3.16 / code 19`。控制端自身未变化时，不必仅为了与接收端同号而重发 Android；但每次接收端打包仍必须重新执行当前 Android 构建与校验。每次控制端更新必须同时递增接收端补丁版本并重新交付 EXE，因为内嵌内容已经发生变化。

应用版本采用三段数字 `major.minor.patch`。普通迭代默认递增 patch，明确的里程碑可选择 minor 或 major；递增 minor/major 会将后面的段归零。当前工具将每段限制为 0–65535，以适配 Windows 数字版本资源。控制协议当前为 v2，配置 schema 当前为 3，它们只随对应协议／数据格式变更递增，不跟随应用版本递增；第三方驱动版本、SDK 版本和 Windows 清单内的公共组件标识也不属于本应用版本。

Android `versionCode` 在 Android 每次交付新版本时递增，跨 major/minor 不重置。项目校验范围为 1–2100000000，Android 官方要求后续发布使用更大序号。[Android 版本说明](https://developer.android.com/studio/publish/versioning)

## 每轮需要修改或核对的位置

| 位置 | 操作方式 | 是否每轮手改 |
|---|---|---|
| `version.properties` | 运行递增脚本，选择本轮涉及的端 | **必须递增** |
| `android/app/build.gradle.kts` | 自动读取 Android 版本与 code，生成包内 Manifest 和 `BuildConfig` | 否，禁止另写版本常量 |
| Android“连接与设置”的版本显示 | 自动使用 `BuildConfig.VERSION_NAME / VERSION_CODE` | 否 |
| `scripts/build-android.ps1` | 核验 Gradle 元数据和 APK 内部 Manifest，生成 `TapDeck-<Android版本>.apk` | 否 |
| `scripts/build-windows.ps1` | 读取接收端版本；`-X main.appVersion=...` 注入程序；从当前 Android 构建内嵌 APK | 否 |
| `windows/cmd/winresources` 生成的 `rsrc.syso` | 写入 EXE 的 FileVersion、ProductVersion 与数字版本 `X.Y.Z.0` | 否，资源文件不提交 |
| PC“关于”、`--version`、内置 APK 说明和配对下载网页 | 从各自程序版本／内嵌清单读取；接收端版本与 Android 版本分别展示 | 否 |
| EXE 与 APK 文件名、内嵌 `apk.json` | 脚本自动使用对应端的版本；APK 清单还包含 code 和 SHA-256 | 否，产物不提交 |
| `release-manifest.json`、`SHA256SUMS.txt` | 打包成功后自动生成本次版本组合、包大小、产物列表与完整哈希 | 否，产物不提交 |
| `scripts/test-android-ui.ps1`、`scripts/test-windows-gestures.ps1` | 自动按 Android／接收端版本命名截图目录和手势日志 | 否 |
| `README.md`、`docs/installation.md`、`docs/android-ui.md` | 更新“当前版本”、code、当前交付路径与版本来源说明 | **每轮检查并更新** |
| `docs/apk-download.md`、`docs/versioning.md` | 更新当前示例、版本表及发生变化的构建规则 | **每轮检查并更新** |
| `docs/verification.md` | 顶部追加新版本、版本组合、验证结果、实际包路径和必要限制 | **每轮追加** |
| 其他功能说明、Release 说明、Git 提交 | 仅更新与本轮相关的当前描述；发布时采用实际生成的版本文件名 | **按本轮范围更新** |

历史验证、历史截图、变更引入版本、测试 fixture 和第三方版本应保留原值，不能全仓库替换所有旧版本号。公开下载链接只有实际上传发布成功后才更新。本地构建成功不等于已经发布。

## 固定迭代步骤

1. 查看 `version.properties`、Git 状态及本轮范围。一次迭代只执行一次递增，默认两个现有端一起升补丁版本。
2. 执行下列命令之一；脚本先校验所有新值，再原子替换单个版本文件。失败时保留原值，修复原因后重试，不把失败计作一次升版。

   ```powershell
   .\scripts\bump-version.ps1                         # 默认 All / Patch
   .\scripts\bump-version.ps1 -Target Receiver         # Android 未变化
   .\scripts\bump-version.ps1 -Target Android          # Android + code；接收端 patch 也递增
   .\scripts\bump-version.ps1 -Target All -Part Minor  # 明确的双端里程碑
   ```

3. 完成代码和当前版本文档同步。`Receiver` 只改接收端版本；`Android` 对 Android 使用所选 Part，对接收端只加 patch；`All` 对两端各自使用所选 Part。脚本不会强行将已经分开的版本重新对齐。
4. 从仓库根目录执行官方构建。输出目录可自动取接收端版本，避免手写旧路径：

   ```powershell
   . .\scripts\versioning.ps1
   $releaseVersions = Get-TapDeckVersions (Get-Location).Path
   .\scripts\build-windows.ps1 -OutputDirectory (Join-Path 'dist' $releaseVersions.ReceiverVersion)
   ```

   构建脚本本身不递增版本。省略 `-OutputDirectory` 时仍输出到 `dist`，兼容已有脚本；独立 Android 构建始终向 `dist` 生成带版本 APK 和兼容副本。

5. 检查 `release-manifest.json` 中 `receiver_version`、`controllers`、所有文件哈希，以及两个接收端 EXE 的 `.apk-info.json`。按本轮功能运行必要回归，将结果写入 `docs/verification.md`。
6. 运行 `git diff --check`，提交版本、实现与文档。对用户交付带版本号的 EXE／APK 链接和提交号，确认工作区没有本轮遗留改动。

## 接收端必须内嵌最新控制端

“最新”指当前待交付源码及 `version.properties` 指定的控制端版本，不是文件修改时间最新的旧包，也不是不匹配当前源码的远端预发布。当前接收端的必备控制端为 Android。

官方构建顺序不可跳过：版本规则测试 → Android 构建与 Kotlin 测试 → Gradle 元数据和 APK 内部版本检查 → 从本次 Gradle 输出复制 APK → 比对内嵌／交付副本 SHA-256 → 生成接收端版本资源 → Go 测试、`go vet` 和 EXE 构建 → 检查三个 EXE 的文件版本 → 从正式版与诊断版 EXE 读取 `--apk-info` → 核对接收端版本、Android 版本、code、文件名、大小及 SHA-256 → 输出发布清单。

缺包、空包、旧版元数据、构建失败、版本／哈希不一致或构建期间版本清单变化都必须使打包失败，不得回退到 `dist` 缓存，不得只发一个尚未完成验证的 EXE。Gradle 自身的增量构建可以使用：它仍先检查当前源码输入，后续还会核验最终包。

`TapDeck-<接收端版本>.exe`、`TapDeck-debug-<接收端版本>.exe`、`TapDeck-hidprobe-<接收端版本>.exe` 是 Windows 带版本产物。前两个内嵌 APK，HID 工具仅做诊断。`TapDeck-<Android版本>.apk` 是 Android 交付包。无版本号的 EXE／APK 仅为相同内容的兼容副本，不应用于掩盖版本变化。包名、签名密钥和已有用户配置不随升版更改。

## 新增 iOS 或其他端的接入规划

目前没有 iOS 构建，不填写虚构的 iOS 版本或产物。新增端时必须一起完成以下接入：

- 在唯一版本文件加入该端独立字段，如 `controller.ios.version` 和 `controller.ios.build`，扩展读取校验、递增脚本、测试及发布清单；每次交付递增对应版本和安装构建序号。未来 macOS 等接收端可在有独立发布节奏时扩展 `receiver.<平台>.version`，迁移所有读取方后再启用。
- iOS 用户可见版本映射到 `CFBundleShortVersionString`，安装构建序号映射到 `CFBundleVersion`；前者采用三段数字，后者使用单调递增序号，应用“关于”页显示自身版本。[Apple 用户可见版本](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleshortversionstring)、[Apple 构建版本](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleversion)
- 可交付的 IPA 使用 `TapDeck-iOS-<版本>-<build>.ipa`，归档、符号包及其他平台安装包也携带平台和版本。不能仅给下载文件改名，必须校验内部版本。
- 需要 macOS/Xcode 的控制端由对应构建机生成本轮产物与包含源码标识、版本、构建序号、文件名、SHA-256 的清单；接收端打包须取得本轮清单指定的最新包并实际内嵌、读回验证。任何必需平台缺包／旧包都阻止交付，不能静默忽略。
- 应用版本、安装构建序号、协议版本分别管理。不同应用版本之间通过协议版本和能力声明判断可用功能，新增端需要验证支持的版本组合。
