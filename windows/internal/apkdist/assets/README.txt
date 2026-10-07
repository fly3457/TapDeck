构建接收端时，scripts/build-windows.ps1 会把 dist\TapDeck-debug.apk 复制到本目录并命名为
TapDeck.apk，然后用 go:embed 打进 TapDeck.exe：配对网页（http://<PC>:41080/pair）会显示
这个 APK 的下载二维码。

本目录里的 *.apk 不提交到 Git（见 .gitignore），只保留这个说明文件，因此
「只构建 Windows 端」时也能编译，只是接收端不含内置 APK，配对网页改为只显示
GitHub Release 下载链接。

要生成含内置 APK 的接收端，依次执行：

    .\scripts\build-android.ps1 -JavaHome <JDK17> -SdkRoot <Android SDK>
    .\scripts\build-windows.ps1
