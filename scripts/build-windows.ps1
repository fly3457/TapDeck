param([switch]$Console,[string]$JavaHome,[string]$SdkRoot,[switch]$UseLocalProxy,[string]$OutputDirectory)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'versioning.ps1')
$taskVersions = Get-TapDeckVersions $taskProjectRoot
$taskVersionHash = (Get-FileHash -LiteralPath (Join-Path $taskProjectRoot 'version.properties') -Algorithm SHA256).Hash
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $taskProjectRoot 'dist' }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$taskAndroidArguments = @{}
if ($JavaHome) { $taskAndroidArguments.JavaHome = $JavaHome }
if ($SdkRoot) { $taskAndroidArguments.SdkRoot = $SdkRoot }
if ($UseLocalProxy) { $taskAndroidArguments.UseLocalProxy = $true }
# A PC release always starts with this source tree's Android build.
& (Join-Path $PSScriptRoot 'test-versioning.ps1')
& (Join-Path $PSScriptRoot 'build-android.ps1') @taskAndroidArguments
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$env:CGO_ENABLED = '0'
$taskBundledDriver = Join-Path $taskProjectRoot 'windows\internal\driver\assets\FakerInput_Setup_0.1.1_x64.msi'
if ((Get-FileHash -LiteralPath $taskBundledDriver -Algorithm SHA256).Hash.ToLowerInvariant() -ne '4c0aefb7340051a91d606776243298b5cd1143ef5508bbae6800c474f9ed0840') { throw 'Bundled upstream driver hash mismatch' }
$taskBundledCable = Join-Path $taskProjectRoot 'windows\internal\vbcable\assets\VBCABLE_Driver_Pack45.zip'
if ((Get-FileHash -LiteralPath $taskBundledCable -Algorithm SHA256).Hash.ToLowerInvariant() -ne 'b950e39f01af1d04ea623c8f6d8eb9b6ea5c477c637295fabf20631c85116bfb') { throw 'Bundled VB-CABLE pack hash mismatch' }
Push-Location (Join-Path $taskProjectRoot 'windows')
try {
    # 把编译好的 Android 安装包放进 embed 目录，接收端就能在配对网页上给出下载二维码。
    $taskApkDir = Join-Path $taskProjectRoot 'windows\internal\apkdist\assets'
    $taskApk = Get-TapDeckAndroidBuild $taskProjectRoot $taskVersions
    New-Item -ItemType Directory -Force -Path $taskApkDir | Out-Null
    Get-ChildItem -LiteralPath $taskApkDir -Filter *.apk -ErrorAction SilentlyContinue | Remove-Item -Force
    $taskVersion = $taskVersions.ReceiverVersion
    go run ./cmd/winresources -version $taskVersion
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource compilation failed' }
    $taskEmbeddedMetadata = @{ version_name = $taskApk.VersionName; version_code = $taskApk.VersionCode; sha256 = $taskApk.SHA256 } | ConvertTo-Json
    [IO.File]::WriteAllText((Join-Path $taskApkDir 'apk.json'),$taskEmbeddedMetadata,[Text.UTF8Encoding]::new($false))
    foreach ($taskApkCopy in @((Join-Path $taskApkDir $taskApk.Name), (Join-Path $OutputDirectory 'TapDeck-debug.apk'), (Join-Path $OutputDirectory $taskApk.Name))) {
        Copy-Item -LiteralPath $taskApk.Path -Destination $taskApkCopy -Force
        if ((Get-FileHash -LiteralPath $taskApkCopy -Algorithm SHA256).Hash.ToLowerInvariant() -cne $taskApk.SHA256) { throw 'Embedded or distribution APK hash mismatch' }
    }
    Write-Host ('接收端 {0} 内嵌 Android {1}（code {2}）：SHA-256 {3}' -f $taskVersion,$taskApk.VersionName,$taskApk.VersionCode,$taskApk.SHA256)
    go test ./... -skip '^(TestVolumeControlChangesEndpoint|TestEnableDisableRoundTrip)$'
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    $taskVersionFlags = '-X main.appVersion=' + $taskVersion
    $taskLinkFlags = '-s -w ' + $taskVersionFlags
    if (-not $Console) { $taskLinkFlags += ' -H=windowsgui' }
    $taskReceiverName = 'TapDeck-' + $taskVersion + '.exe'
    $taskDebugName = 'TapDeck-debug-' + $taskVersion + '.exe'
    $taskProbeName = 'TapDeck-hidprobe-' + $taskVersion + '.exe'
    go build -trimpath -ldflags $taskLinkFlags -o (Join-Path $OutputDirectory $taskReceiverName) ./cmd/tapdeck
    if ($LASTEXITCODE -ne 0) { throw 'Windows build failed' }
    go build -trimpath -ldflags $taskVersionFlags -o (Join-Path $OutputDirectory $taskDebugName) ./cmd/tapdeck
    if ($LASTEXITCODE -ne 0) { throw 'Windows console build failed' }
    go build -trimpath -o (Join-Path $OutputDirectory $taskProbeName) ./cmd/hidprobe
    if ($LASTEXITCODE -ne 0) { throw 'HID diagnostic build failed' }
    # Verify the Explorer Details fields, then keep stable names for existing scripts.
    $taskArtifacts = @(
        @{ Name = $taskReceiverName; Alias = 'TapDeck.exe' },
        @{ Name = $taskDebugName; Alias = 'TapDeck-debug.exe' },
        @{ Name = $taskProbeName; Alias = 'TapDeck-hidprobe.exe' }
    )
    $taskFiles = @($taskApk.Name, 'TapDeck-debug.apk')
    foreach ($taskArtifact in $taskArtifacts) {
        $taskArtifactPath = Join-Path $OutputDirectory $taskArtifact.Name
        $taskFileVersion = (Get-Item -LiteralPath $taskArtifactPath).VersionInfo
        if ($taskFileVersion.FileVersion -ne $taskVersion -or $taskFileVersion.ProductVersion -ne $taskVersion) { throw ('Windows version metadata mismatch: ' + $taskArtifact.Name) }
        Copy-Item -LiteralPath $taskArtifactPath -Destination (Join-Path $OutputDirectory $taskArtifact.Alias) -Force
        if ((Get-FileHash -LiteralPath $taskArtifactPath -Algorithm SHA256).Hash -ne
            (Get-FileHash -LiteralPath (Join-Path $OutputDirectory $taskArtifact.Alias) -Algorithm SHA256).Hash) { throw ('Windows compatibility copy mismatch: ' + $taskArtifact.Alias) }
        $taskFiles += @($taskArtifact.Name, $taskArtifact.Alias)
        Write-Host ('Windows 程序：' + $taskArtifactPath)
    }
    # Read back the APK actually linked into BOTH receiver executables, without starting a server.
    foreach ($taskReceiver in @($taskReceiverName, $taskDebugName)) {
        $taskInfoPath = Join-Path $OutputDirectory ($taskReceiver + '.apk-info.json')
        $taskErrorPath = Join-Path $OutputDirectory ($taskReceiver + '.apk-info.log')
        $taskProcess = Start-Process -FilePath (Join-Path $OutputDirectory $taskReceiver) -ArgumentList '--apk-info' -WindowStyle Hidden -PassThru -RedirectStandardOutput $taskInfoPath -RedirectStandardError $taskErrorPath
        try {
            if (-not $taskProcess.WaitForExit(30000)) { $taskProcess.Kill(); throw 'Timed out verifying embedded APK' }
            if ($taskProcess.ExitCode -ne 0) { throw ('Embedded APK verification failed: ' + (Get-Content -LiteralPath $taskErrorPath -Raw)) }
        } finally { $taskProcess.Dispose() }
        Assert-TapDeckApkInfo (Get-Content -LiteralPath $taskInfoPath -Raw | ConvertFrom-Json) $taskVersions $taskApk
    }
    $taskReportedVersion = (& (Join-Path $OutputDirectory $taskDebugName) --version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $taskReportedVersion -cne ('TapDeck ' + $taskVersion)) { throw 'Receiver runtime version mismatch' }
    if ((Get-FileHash -LiteralPath (Join-Path $taskProjectRoot 'version.properties') -Algorithm SHA256).Hash -ne $taskVersionHash) { throw 'Version configuration changed during receiver build' }
    $taskFileRecords = @($taskFiles | ForEach-Object {
        $taskFilePath = Join-Path $OutputDirectory $_
        @{ filename = $_; bytes = (Get-Item -LiteralPath $taskFilePath).Length; sha256 = (Get-FileHash -LiteralPath $taskFilePath -Algorithm SHA256).Hash.ToLowerInvariant() }
    })
    $taskRelease = [ordered]@{
        schema = 1
        built_at = [DateTime]::UtcNow.ToString('o')
        receiver_version = $taskVersion
        controllers = @(@{ platform = 'android'; version = $taskApk.VersionName; version_code = $taskApk.VersionCode; filename = $taskApk.Name; sha256 = $taskApk.SHA256; bytes = $taskApk.Bytes })
        artifacts = $taskFileRecords
    }
    [IO.File]::WriteAllText((Join-Path $OutputDirectory 'release-manifest.json'), ($taskRelease | ConvertTo-Json -Depth 5), [Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllLines((Join-Path $OutputDirectory 'SHA256SUMS.txt'), [string[]]@($taskFileRecords | ForEach-Object { $_.sha256 + '  ' + $_.filename }), [Text.UTF8Encoding]::new($false))
    Write-Host ('版本、内嵌 APK 及产物清单校验通过：' + (Join-Path $OutputDirectory 'release-manifest.json'))
} finally { Pop-Location }
