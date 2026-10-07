param([switch]$Console)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
New-Item -ItemType Directory -Force -Path (Join-Path $taskProjectRoot 'dist') | Out-Null
$env:CGO_ENABLED = '0'
$taskBundledDriver = Join-Path $taskProjectRoot 'windows\internal\driver\assets\FakerInput_Setup_0.1.1_x64.msi'
if ((Get-FileHash -LiteralPath $taskBundledDriver -Algorithm SHA256).Hash.ToLowerInvariant() -ne '4c0aefb7340051a91d606776243298b5cd1143ef5508bbae6800c474f9ed0840') { throw 'Bundled upstream driver hash mismatch' }
Push-Location (Join-Path $taskProjectRoot 'windows')
try {
    # Use the verified, fixed-version module cache when available. Running an
    # explicit @version otherwise queries deprecation metadata on every build.
    $taskGoCache = (go env GOMODCACHE).Trim()
    $taskRsrcSource = Join-Path $taskGoCache 'github.com\akavel\rsrc@v0.10.2'
    if (Test-Path (Join-Path $taskRsrcSource 'go.mod')) {
        $taskManifest = Join-Path $taskProjectRoot 'windows\cmd\tapdeck\app.manifest'
        $taskResource = Join-Path $taskProjectRoot 'windows\cmd\tapdeck\rsrc.syso'
        Push-Location $taskRsrcSource
        try { go run . -manifest $taskManifest -o $taskResource } finally { Pop-Location }
    } else { go run github.com/akavel/rsrc@v0.10.2 -manifest cmd/tapdeck/app.manifest -o cmd/tapdeck/rsrc.syso }
    if ($LASTEXITCODE -ne 0) { throw 'Manifest compilation failed' }
    Copy-Item -LiteralPath 'cmd\tapdeck\rsrc.syso' -Destination 'cmd\hidprobe\rsrc.syso' -Force
    # 把编译好的 Android 安装包放进 embed 目录，接收端就能在配对网页上给出下载二维码。
    $taskApkDir = Join-Path $taskProjectRoot 'windows\internal\apkdist\assets'
    New-Item -ItemType Directory -Force -Path $taskApkDir | Out-Null
    Get-ChildItem -LiteralPath $taskApkDir -Filter *.apk -ErrorAction SilentlyContinue | Remove-Item -Force
    $taskApkSource = Join-Path $taskProjectRoot 'dist\TapDeck-debug.apk'
    if (-not (Test-Path $taskApkSource)) { $taskApkSource = Join-Path $taskProjectRoot 'android\app\build\outputs\apk\debug\app-debug.apk' }
    if (Test-Path $taskApkSource) {
        Copy-Item -LiteralPath $taskApkSource -Destination (Join-Path $taskApkDir 'TapDeck.apk') -Force
        Write-Host ('内嵌 Android 安装包：{0}（{1:N2} MB）' -f (Split-Path -Leaf $taskApkSource), ((Get-Item $taskApkSource).Length / 1MB))
    } else {
        Write-Warning '未找到 TapDeck-debug.apk：本次接收端不含内置 APK，配对网页只显示 Release 链接（先运行 scripts/build-android.ps1）'
    }
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    $taskLinkFlags = '-s -w'
    if (-not $Console) { $taskLinkFlags += ' -H=windowsgui' }
    go build -trimpath -ldflags $taskLinkFlags -o (Join-Path $taskProjectRoot 'dist\TapDeck.exe') ./cmd/tapdeck
    if ($LASTEXITCODE -ne 0) { throw 'Windows build failed' }
    go build -trimpath -o (Join-Path $taskProjectRoot 'dist\TapDeck-debug.exe') ./cmd/tapdeck
    if ($LASTEXITCODE -ne 0) { throw 'Windows console build failed' }
    go build -trimpath -o (Join-Path $taskProjectRoot 'dist\TapDeck-hidprobe.exe') ./cmd/hidprobe
    if ($LASTEXITCODE -ne 0) { throw 'HID diagnostic build failed' }
} finally { Pop-Location }
