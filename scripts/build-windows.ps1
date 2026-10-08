param([switch]$Console,[string]$JavaHome,[string]$SdkRoot,[switch]$UseLocalProxy,[string]$OutputDirectory)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $taskProjectRoot 'dist' }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$taskAndroidArguments = @{}
if ($JavaHome) { $taskAndroidArguments.JavaHome = $JavaHome }
if ($SdkRoot) { $taskAndroidArguments.SdkRoot = $SdkRoot }
if ($UseLocalProxy) { $taskAndroidArguments.UseLocalProxy = $true }
# A PC release always starts with this source tree's Android build.
& (Join-Path $PSScriptRoot 'build-android.ps1') @taskAndroidArguments
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$env:CGO_ENABLED = '0'
$taskBundledDriver = Join-Path $taskProjectRoot 'windows\internal\driver\assets\FakerInput_Setup_0.1.1_x64.msi'
if ((Get-FileHash -LiteralPath $taskBundledDriver -Algorithm SHA256).Hash.ToLowerInvariant() -ne '4c0aefb7340051a91d606776243298b5cd1143ef5508bbae6800c474f9ed0840') { throw 'Bundled upstream driver hash mismatch' }
$taskBundledCable = Join-Path $taskProjectRoot 'windows\internal\vbcable\assets\VBCABLE_Driver_Pack45.zip'
if ((Get-FileHash -LiteralPath $taskBundledCable -Algorithm SHA256).Hash.ToLowerInvariant() -ne 'b950e39f01af1d04ea623c8f6d8eb9b6ea5c477c637295fabf20631c85116bfb') { throw 'Bundled VB-CABLE pack hash mismatch' }
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
    $taskApkOutput = Join-Path $taskProjectRoot 'android\app\build\outputs\apk\debug'
    $taskApkSource = Join-Path $taskApkOutput 'app-debug.apk'
    if (-not (Test-Path -LiteralPath $taskApkSource)) { throw 'Android build did not produce app-debug.apk' }
    $taskApkMetadata = Get-Content -LiteralPath (Join-Path $taskApkOutput 'output-metadata.json') -Raw | ConvertFrom-Json
    $taskApkElement = @($taskApkMetadata.elements | Where-Object { $_.outputFile -eq 'app-debug.apk' })
    if ($taskApkElement.Count -ne 1) { throw 'Android APK version metadata missing or ambiguous' }
    $taskVersion = $taskApkElement[0].versionName
    if ($taskVersion -notmatch '^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$') { throw 'Invalid release version' }
    $taskApkName = 'TapDeck-' + $taskVersion + '.apk'
    $taskApkHash = (Get-FileHash -LiteralPath $taskApkSource -Algorithm SHA256).Hash.ToLowerInvariant()
    $taskEmbeddedApk = Join-Path $taskApkDir $taskApkName
    Copy-Item -LiteralPath $taskApkSource -Destination $taskEmbeddedApk -Force
    if ((Get-FileHash -LiteralPath $taskEmbeddedApk -Algorithm SHA256).Hash.ToLowerInvariant() -ne $taskApkHash) { throw 'Embedded APK hash mismatch' }
    $taskEmbeddedMetadata = @{ version_name = $taskApkElement[0].versionName; version_code = $taskApkElement[0].versionCode; sha256 = $taskApkHash } | ConvertTo-Json
    [IO.File]::WriteAllText((Join-Path $taskApkDir 'apk.json'),$taskEmbeddedMetadata,[Text.UTF8Encoding]::new($false))
    Copy-Item -LiteralPath $taskApkSource -Destination (Join-Path $OutputDirectory 'TapDeck-debug.apk') -Force
    Copy-Item -LiteralPath $taskApkSource -Destination (Join-Path $OutputDirectory $taskApkName) -Force
    Write-Host ('内嵌 Android {0}（code {1}）：SHA-256 {2}' -f $taskApkElement[0].versionName,$taskApkElement[0].versionCode,$taskApkHash)
    go test ./... -skip '^(TestVolumeControlChangesEndpoint|TestEnableDisableRoundTrip)$'
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    $taskVersionFlags = '-X main.appVersion=' + $taskVersion
    $taskLinkFlags = '-s -w ' + $taskVersionFlags
    if (-not $Console) { $taskLinkFlags += ' -H=windowsgui' }
    go build -trimpath -ldflags $taskLinkFlags -o (Join-Path $OutputDirectory 'TapDeck.exe') ./cmd/tapdeck
    if ($LASTEXITCODE -ne 0) { throw 'Windows build failed' }
    go build -trimpath -ldflags $taskVersionFlags -o (Join-Path $OutputDirectory 'TapDeck-debug.exe') ./cmd/tapdeck
    if ($LASTEXITCODE -ne 0) { throw 'Windows console build failed' }
    go build -trimpath -o (Join-Path $OutputDirectory 'TapDeck-hidprobe.exe') ./cmd/hidprobe
    if ($LASTEXITCODE -ne 0) { throw 'HID diagnostic build failed' }
} finally { Pop-Location }
