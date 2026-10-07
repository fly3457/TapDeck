# 内置 Android 安装包与扫码下载（2026-10-07）

新手机要装 TapDeck，以前必须先有 APK 文件（Release 下载或 adb）。现在**编译好的 APK 直接打进接收端**，配对网页上给出下载二维码：手机扫码即可下载并安装。

## 用法

1. PC 上正常启动 `dist/TapDeck.exe`（`--headless` 也一样）。
2. 手机浏览器打开配对网址 `http://<PC 地址>:41080/pair`（PC 设置窗口里的网址，或二维码）。
3. 页面下方「下载 Android 端」区块显示二维码、下载按钮、文件名与大小、以及下载地址和 SHA-256 前 16 位。
4. 手机扫码（或点按钮）下载 `TapDeck.apk`，按系统提示允许「安装未知应用」后安装。装好后再用同一个页面完成配对。

## 实现

### 1. 内嵌

- 新增 `windows/internal/apkdist`：用 `//go:embed assets` 把 `assets/*.apk` 打进 EXE，导出 `Available()`、`Name()`、`Bytes()`、`SHA256()`、`SizeText()`。
- `assets` 目录里始终保留 `README.txt`，因此**没有 APK 也能编译**（只构建 Windows 端时正常通过，只是不含内置包）。`assets/*.apk` 已在 `.gitignore` 中忽略，不提交二进制。
- `scripts/build-windows.ps1` 在 `go build` 前把 `dist\TapDeck-debug.apk`（缺失时退回 Gradle 输出目录）复制成 `windows\internal\apkdist\assets\TapDeck.apk`，并打印内嵌结果；找不到 APK 时打印警告，构建照常进行。
- `cmd/tapdeck` 启动时调用 `s.SetAPK(...)` 注入内容、文件名与 SHA-256。设置页「设置与状态」页会显示本次构建是否内置安装包。

### 2. 两个新路由（HTTP 端口，默认 41080）

| 路由 | 说明 |
|---|---|
| `GET /apk` | 下载内置 APK：`Content-Type: application/vnd.android.package-archive`、`Content-Disposition: attachment; filename="TapDeck.apk"`、带 `Content-Length`，用 `http.ServeContent` 因此支持 Range 断点续传。未内置时返回 404 + 提示去 GitHub Release 下载。 |
| `GET /apk/qr.png` | 下载地址的二维码 PNG（`go-qrcode`，512 px，纠错等级 M）。二维码内容取请求的 `Host`，所以手机从哪个地址打开配对页，二维码就指向哪个地址（IP 或主机名都行）。 |

配对页 `/pair` 用 `<!--APK-->` 占位符注入下载区块：有内置包时是二维码 + 下载按钮 + 大小 + SHA-256 前 16 位；没有时是一条 Release 链接。页面 CSP 保持 `default-src 'self'`（二维码是同源图片，不需要放开 `data:`）。

## 实测

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
- 没有内置包时（只构建 Windows 端）页面只显示 GitHub Release 链接，不会出现空白或 404 页面。
- 接收端体积随 APK 增大（当前 +10.93 MB）；如需瘦身可以把 APK 放到 Release，只在页面给链接。
