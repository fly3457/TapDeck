发布统一执行 scripts/build-windows.ps1。
版本唯一来源为仓库根目录 version.properties，接收端与 Android 版本可不同。
该入口每次先调用 Android Gradle release 构建和 Kotlin 测试，校验 APK 内部版本及 Gradle
元数据与版本清单一致，使用 aapt 与 apksigner 验证不可调试和固定签名证书，
然后从该次 Gradle release 输出复制 TapDeck-<Android版本>.apk 到本目录，
核对 SHA-256 并写入 apk.json（版本、versionCode、完整哈希、构建类型、debuggable 和证书指纹），
再进行 Go 测试、go vet 和 Windows 构建。最终读取两个接收端 EXE 的 --apk-info，
再次核对内嵌版本、code、文件名、大小、哈希及安全元数据，生成 release-manifest.json。

Android 构建失败、APK 缺失或哈希不一致时立即终止 PC 构建。旧 dist APK 不参与内嵌。
接收端启动前再次验证 APK 和元数据完整性，缺包的直接 go build 不能作为发布产物。
*.apk 和 apk.json 是构建产物，已在 .gitignore 忽略。
