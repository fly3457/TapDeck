发布统一执行 scripts/build-windows.ps1。
该入口每次先调用 Android Gradle 构建和 Kotlin 测试，然后从该次 Gradle 输出复制
TapDeck.apk 到本目录，核对 SHA-256 并写入 apk.json（版本、versionCode、完整哈希），
再进行 Go 测试、go vet 和 Windows 构建。

Android 构建失败、APK 缺失或哈希不一致时立即终止 PC 构建。旧 dist APK 不参与内嵌。
接收端启动前再次验证 APK 和元数据完整性，缺包的直接 go build 不能作为发布产物。
*.apk 和 apk.json 是构建产物，已在 .gitignore 忽略。