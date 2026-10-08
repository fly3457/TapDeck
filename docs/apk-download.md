# 从 PC 下载 Android APK

Windows EXE 内嵌本次构建的 Android APK，不必另找安装文件。

## 下载与配对

1. 启动 PC 接收端，手机浏览器打开“连接”页的网址或扫描二维码。
2. 网页显示 APK 版本、文件名、大小和 SHA-256，点击下载或扫描下载二维码。
3. 按 Android 提示允许当前浏览器安装应用，安装后打开 TapDeck 完成配对。
4. 手机填写网址后点击连接，首次仍由 PC 核对并允许；二维码不携带授权 secret。

当前 APK 为 [TapDeck-0.3.18.apk](https://github.com/fly3457/TapDeck/releases/download/v0.3.18/TapDeck-0.3.18.apk)，Android `0.3.18 / code 21`。换版本后使用相同签名覆盖安装，PC 配对页也随内嵌版本更新。

## 接口与构建

| 地址 | 内容 |
|---|---|
| `/pair` | 安装与配对网页 |
| `/api/pair-info` | 连接元数据，不暴露令牌或 UDP 密钥 |
| `/apk` | APK 下载，带版本文件名及 Range 续传 |
| `/apk/qr.png` | 下载二维码，使用访问网页的主机地址 |

`windows/internal/apkdist` 内嵌 APK 和元数据，启动前校验。官方 `scripts/build-windows.ps1` 先构建和测试 Android，核验 Gradle 与 APK 内部版本，再从本次输出内嵌并比对两个接收端的 `--apk-info`。失败不能复用旧包，见[版本规则](versioning.md)。

构建生成 `release-manifest.json` 和 `SHA256SUMS.txt`；独立 APK 与两个 EXE 内嵌包必须同哈希。源码直接构建缺少有效内嵌包时不能发布。

下方保留早期 0.2 APK 下载实测；当前产物和签名见[验证记录](verification.md)。

<details>
<summary>0.2 APK 下载历史实测</summary>

## 历史版本实测（0.2 APK，2026-10-07）

| 项目 | 结果 |
|---|---|
| 构建 | `scripts/build-windows.ps1` 输出「内嵌 Android 安装包：TapDeck-debug.apk（10.93 MB）」，EXE 由 12.8 MB 增至 24.4 MB |
| `GET /health` | `{"app":"TapDeck","version":2}` |
| `GET /apk` | 200、`application/vnd.android.package-archive`、`Content-Length: 11460424`、`filename="TapDeck.apk"`；PC 端下载后的 SHA-256 与 `dist\TapDeck-debug.apk` 完全一致 |
| `GET /pair` | 页面含 `/apk/qr.png`、`href="/apk" download`、`TapDeck.apk`、SHA-256 提示 |
| `GET /apk/qr.png` | 200、`image/png`、598 字节；`TestAPKQRCodePointsAtDownloadURL` 断言它与 `qrcode.Encode("http://<host>/apk")` 逐字节相同 |
| 平板（ONYX Tab8C / Android 11） | Chrome 打开 `http://192.168.1.11:41080/pair` 显示「下载 Android 端」区块与二维码；用平板 curl 下载 `/apk` 得 HTTP 200、11460424 字节，`sha256sum` 与 PC 端一致（测试文件已删除） |
| 单元测试 | `internal/apkdist`：有/无/空 APK 三种情况；`internal/server`：下载响应头与内容、未内置时的 404 提示、二维码内容、配对页含下载入口与不含占位符、未内置时只给 Release 链接 |

</details>
