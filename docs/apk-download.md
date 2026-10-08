# 内置 Android 安装包与扫码下载（0.3，2026-10-07）

新手机要装 TapDeck，以前必须先有 APK 文件（Release 下载或 adb）。现在**编译好的 APK 直接打进接收端**，配对网页上给出下载二维码：手机扫码即可下载并安装。

## 用法

1. PC 上正常启动构建产物 `TapDeck-<版本>.exe`（兼容副本 `TapDeck.exe` 或 `--headless` 也一样）。
2. 手机浏览器打开配对网址 `http://<PC 地址>:41080/pair`（PC 设置窗口里的网址，或二维码）。
3. 网页「下载 Android 端」区块显示 APK 下载按钮、下载二维码、版本、文件名与大小、下载地址和完整 SHA-256。
4. 手机扫码（或点按钮）下载带 Android 版本号的安装包，例如 `TapDeck-0.3.14.apk`，按系统提示允许「安装未知应用」后安装。装好后再用同一个页面完成配对。

## 实现

### 1. 内嵌

- 新增 `windows/internal/apkdist`：用 `//go:embed assets` 把 `assets/*.apk` 打进 EXE，导出 `Available()`、`Name()`、`Bytes()`、`SHA256()`、`SizeText()`。
- `assets/*.apk` 和 `assets/apk.json` 是构建产物，已在 `.gitignore` 中忽略。
- 官方入口 `scripts/build-windows.ps1` 每次先调用 Android 构建和 Kotlin 测试；核对 Gradle 元数据及 APK 内部 Manifest 与根目录 `version.properties` 中的 Android 版本、code 一致后，仅从该次 Gradle 输出目录复制 `app-debug.apk`，核对 SHA-256 并写入版本清单，再测试和构建 Windows。文件名为 `TapDeck-<Android版本>.apk`，Windows 使用独立的接收端版本，两端无需同号；独立 Android 构建也生成带版本号的文件。不会读取旧 `dist` APK；Android 失败、缺包、版本或哈希不一致立即失败。
- 构建完成后读取正式版和诊断版 EXE 的 `--apk-info`，比对实际嵌入的版本、code、文件名、大小、SHA-256 以及接收端版本；通过后生成 `release-manifest.json`，记录接收端与各控制端的版本组合，以及 `SHA256SUMS.txt`。每次迭代都要升版，长期规则见 [版本管理](versioning.md)。
- `cmd/tapdeck` 启动前验证内嵌 APK 与元数据，再调用 `s.SetAPK(...)` 注入内容、文件名、版本与完整 SHA-256。设置页「设置与状态」和「关于」显示内嵌版本、code、大小和完整哈希；「关于」另显示 PC 版本。源码直接 `go build` 的 PC 版本显示为 `dev`，缺失有效 APK 时不能启动接收端，不能用于发布。

### 2. 两个新路由（HTTP 端口，默认 41080）

| 路由 | 说明 |
|---|---|
| `GET /apk` | 下载内置 APK：`Content-Type: application/vnd.android.package-archive`、`Content-Disposition: attachment; filename="TapDeck-0.3.14.apk"`（文件名随 Android 版本变化）、带 `Content-Length`，用 `http.ServeContent` 因此支持 Range 断点续传。未内置时返回 404 + 提示去 GitHub Release 下载。 |
| `GET /apk/qr.png` | 下载地址的二维码 PNG（`go-qrcode`，512 px，纠错等级 M）。二维码内容取请求的 `Host`，所以手机从哪个地址打开配对页，二维码就指向哪个地址（IP 或主机名都行）。 |

PC“连接”页的二维码始终打开 `/pair` 网页，不含 secret。网页下载区块显示版本和完整 SHA-256，安装后点击“打开 TapDeck 连接”；每次首次配对都由 PC 核对并允许。网页中的 `/apk/qr.png` 是可选的下载直链二维码。页面 CSP 保持 `default-src 'self'`。

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

## 已知限制与注意

- 内嵌的是**当前构建的 APK**（默认 debug 签名）。换 APK 后要重新运行 `build-windows.ps1`，EXE 才会带上新的包。
- Chrome 等浏览器下载 `.apk` 会先弹「此类文件可能有害，是否仍要下载」，需要用户确认；这是浏览器行为，页面里已提示允许「未知来源」安装。
- 本轮接收端必须通过内嵌 APK 和版本清单校验才启动，不发布缺包的 EXE。
- 本轮单 EXE 同时内嵌 APK 与 VB-CABLE 完整原包，发布需使用统一构建入口；新增哈希、版本、旧 APK 和构建失败的验证见 [installation.md](installation.md)。
