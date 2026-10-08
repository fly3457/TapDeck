# 版本与发布规则

本文件和根目录 [AGENTS.md](../AGENTS.md) 是长期规则。从 0.3.13 起，每次界面、功能、配置、构建、文档或项目规则迭代都递增版本、验证交付并提交 Git；同一迭代的测试、重试、重复编译及发布后链接补记不重复升版。

## 唯一版本来源

只修改根目录 [version.properties](../version.properties)，应用和构建脚本自动读取，禁止另写版本常量。

| 字段 | 当前值 | 用途 |
|---|---|---|
| `receiver.version` | `0.3.18` | Windows 接收端、诊断版和 HID 工具 |
| `controller.android.version` | `0.3.18` | Android 内部版本及 APK 文件名 |
| `controller.android.versionCode` | `21` | Android 覆盖安装序号 |

两端独立演进。例如只改接收端可发布 `0.3.19` 并内嵌 Android `0.3.18 / code 21`；每次 Android 更新必须同时递增接收端补丁版本并重建 EXE。接收端打包始终重新构建和核验清单指定的 Android。

应用版本为 `major.minor.patch`，默认加 patch；加 minor／major 时后续段归零，各段范围 0–65535。Android code 每次新交付递增，范围 1–2100000000，跨 major／minor 不重置，见 [Android 版本说明](https://developer.android.com/studio/publish/versioning)。

当前协议 v2、配置 schema 3；只在对应格式变更时递增。依赖版本及 Windows 公共组件标识不随应用升版。

## 一次迭代流程

1. 开始时读本文件，检查 Git 状态、版本及本轮范围。
2. 运行一次升版脚本。新值先完整校验，再原子替换；失败保留原值。

   ```powershell
   .\scripts\bump-version.ps1                         # 默认 All / Patch
   .\scripts\bump-version.ps1 -Target Receiver         # 仅接收端
   .\scripts\bump-version.ps1 -Target Android          # Android + code；接收端 patch 同步递增
   .\scripts\bump-version.ps1 -Target All -Part Minor  # 明确的双端里程碑
   ```

   各端从自身版本递增，不强行同号。`Android` 对 Android 使用所选 Part，接收端仅加 patch。
3. 完成实现与文档，按本轮风险运行必要回归。
4. 从仓库根目录执行官方构建：

   ```powershell
   . .\scripts\versioning.ps1
   $releaseVersions = Get-TapDeckVersions (Get-Location).Path
   .\scripts\build-windows.ps1 -OutputDirectory (Join-Path 'dist' $releaseVersions.ReceiverVersion)
   ```

5. 核验两个接收端的 `--apk-info`、三个 EXE 的文件版本、APK 内部版本及 SHA-256、`release-manifest.json` 和 `SHA256SUMS.txt`。更新[验证记录](verification.md)，注明版本组合、包路径、截图和实测范围。
6. 运行 `git diff --check`，将实现、版本、文档和验证记录一起提交 Git。构建产物、资源文件、内嵌 APK 和生成清单不提交。

脚本不自动升版；省略输出目录时兼容输出到 `dist`。独立 Android 构建生成带版本 APK 和本地兼容副本。

## 构建与内嵌校验

“最新控制端”指当前源码及版本清单指定的包。禁止复用旧 `dist` APK、降级回退或缺包继续交付。

官方顺序：

版本规则测试 → Android 构建与 Kotlin 测试 → Gradle 元数据和 APK 内部版本核验 → 从本次 Gradle 输出内嵌 → 内嵌／交付副本哈希比对 → 接收端资源生成 → Go 测试和 vet → EXE 构建 → 三个 EXE 文件版本 → 两个接收端 `--apk-info` → 发布清单。

缺包、空包、旧元数据、失败、版本或哈希不一致、构建期间版本清单变化均使交付失败。Gradle 增量构建允许使用，但必须检查当前源码并核验最终包。

| 对外产物 | 内部版本与要求 |
|---|---|
| `TapDeck-<接收端版本>.exe` | 正式窗口版，内嵌 APK |
| `TapDeck-debug-<接收端版本>.exe` | 控制台诊断版，内嵌相同 APK |
| `TapDeck-hidprobe-<接收端版本>.exe` | HID 诊断工具 |
| `TapDeck-<Android版本>.apk` | Android Manifest 与文件名版本一致 |

无版本号文件只作本地兼容副本。包名、签名及用户配置不因升版更改。ZIP 等额外交付也在文件名中携带版本，并附第三方许可；安装其他平台时同样核验内部版本。

## GitHub 发布

用户要求发布时，先提交并推送源码，再以交付提交创建版本标签。发布带版本号的 EXE、APK、校验清单及许可证；发布说明记录实际验证和待验范围。

发布后核对标签、资产及哈希，才把 README 和验证记录中的下载链接更新为真实公开 Release URL；链接补记单独提交并推送，沿用本轮版本。没有公开发布的构建只写本地路径。保留历史版本、截图路径和真实公开链接，禁止全仓库替换旧版本号。

## 新增平台

当前没有 iOS 产物，不填写虚构版本。新增端必须同时接入：

- 唯一版本清单中的独立版本和安装序号、读写校验、升版脚本、测试及发布清单。macOS 等接收端若独立演进，也先扩展字段并迁移所有读取方。
- 包内版本与带版本文件名。iOS 使用 `CFBundleShortVersionString` 和单调递增的 `CFBundleVersion`，IPA 为 `TapDeck-iOS-<版本>-<build>.ipa`，见 [Apple 版本字段](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleshortversionstring)及[构建序号](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleversion)。
- macOS／Xcode 等对应构建机生成本轮包与清单，包含源码标识、版本、序号、文件名和 SHA-256。接收端取清单指定的包，实际内嵌并读回校验；必需端缺包或旧包阻止交付。
- 独立管理应用版本、安装序号和协议能力，验收实际版本组合，不能以应用版本同号代替兼容性判断。
