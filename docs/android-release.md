# Android 发布构建与签名

从 Android 0.3.19 起，对外 APK 及 Windows 内嵌包只使用显式签名的 `release` 构建。调试、JNI 调试和 shell profiling 均关闭；保留现有包名与应用数据格式。开发用 `assembleDebug` 不产生交付文件。

## 签名与升级

本轮沿用已公开版本的证书，保证 0.3.18 及更早的同签名安装可以覆盖升级，保留配对、device ID 和手机设置。证书 SHA-256 固定在 [release-certificate.sha256](../android/release-certificate.sha256)：

```text
7a73774806a038cf5ac53dff3aa4332390158c2ecbf55bebd5c01811ae543e30
```

这是历史开发证书，未换成新的正式证书；不可调试的 release 构建与证书更换是两件事。当前使用 GitHub 和接收端网页分发，不将该证书描述为符合应用商店的正式签名。后续更换证书需要另行设计兼容迁移，不能直接要求现有用户卸载并丢失配对。[Android 签名说明](https://developer.android.com/studio/publish/app-signing)

本机私钥已复制到工作区外的 `%USERPROFILE%\.tapdeck\android-release\tapdeck.jks`，使用随机的独立库密码和私钥密码，目录只允许当前用户与 SYSTEM 访问。原开发密钥没有被删除或自动替换；今后的发布流程不读取 SDK 的 `debug.keystore`。

## 配置维护者的构建环境

提供以下四个进程环境变量，通过受保护的凭据存储注入，密码不要写入仓库或命令行参数：

| 环境变量 | 内容 |
|---|---|
| `TAPDECK_ANDROID_STORE_FILE` | 签名库的绝对路径 |
| `TAPDECK_ANDROID_STORE_PASSWORD` | 签名库密码 |
| `TAPDECK_ANDROID_KEY_ALIAS` | 私钥别名 |
| `TAPDECK_ANDROID_KEY_PASSWORD` | 私钥密码 |

Windows 官方脚本也支持当前账户的 DPAPI 配置，默认路径 `%USERPROFILE%\.tapdeck\android-signing.xml`，或用 `TAPDECK_ANDROID_SIGNING_CONFIG` 指定。文件以 `Export-Clixml` 保存，包含 `StoreFile`、`KeyAlias` 和两个 `SecureString` 字段 `StorePassword`、`KeyPassword`。脚本在构建期间解密并注入变量，结束或失败后恢复原环境。四个变量部分缺失时直接报错，不混用不同配置。

DPAPI 配置绑定 Windows 账户与机器，不能当作可移植的密码备份。维护者需在独立的安全位置备份签名库和密码；更换构建机时重新配置。仓库只保存公开证书指纹。

## 构建和校验

```powershell
.\scripts\build-windows.ps1 -OutputDirectory dist/0.3.19
# 只构建 Android，同时生成匹配的 release 仪器测试 APK：
.\scripts\build-android.ps1 -Instrumentation
```

官方入口执行 `assembleRelease` 和 `testReleaseUnitTest`，只接受 Gradle `release/app-release.apk`。签名配置缺失、证书不符、未签名、包名或版本错误、`application-debuggable` 出现，都会中止交付，不回退到 debug 或旧 APK。Gradle 的 `preReleaseBuild` 也依赖签名检查，直接执行 `assembleRelease` 不会绕过配置要求。

`aapt` 检查实际 Manifest，`apksigner verify --verbose --print-certs` 验证签名和固定证书；然后比对复制、交付与内嵌 APK 的 SHA-256。两个接收端 `--apk-info` 及 `release-manifest.json` 同时记录 `build_type: release`、`debuggable: false` 和 `certificate_sha256`，接收端启动前校验对应元数据与包哈希。[apksigner 文档](https://developer.android.com/tools/apksigner)

带版本号的 `TapDeck-<Android版本>.apk` 用于交付；`dist/TapDeck.apk` 仅是本地安装脚本的兼容副本。不再生成或使用 `dist/TapDeck-debug.apk`，历史文件不能作为当前交付。仪器测试只在隔离模拟器中安装，测试 APK 不发布。
