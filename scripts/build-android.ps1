param([string]$JavaHome,[string]$SdkRoot,[switch]$UseLocalProxy)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'versioning.ps1')
$taskAndroidVersions = Get-TapDeckVersions $taskProjectRoot
$taskAndroidVersionHash = (Get-FileHash -LiteralPath (Join-Path $taskProjectRoot 'version.properties') -Algorithm SHA256).Hash
if (-not $JavaHome) { $taskBundledJdk = Get-ChildItem (Join-Path $taskProjectRoot '.tools\jdk17') -Directory -ErrorAction SilentlyContinue | Select-Object -First 1; if ($taskBundledJdk) { $JavaHome=$taskBundledJdk.FullName } else { $JavaHome=$env:JAVA_HOME } }
if (-not $JavaHome) { throw 'Set -JavaHome to JDK 17 or later' }
$env:JAVA_HOME=$JavaHome
if (-not $SdkRoot) { $SdkRoot=Join-Path $env:LOCALAPPDATA 'Android\Sdk' }
('sdk.dir=' + $SdkRoot.Replace('\','/')) | Set-Content (Join-Path $taskProjectRoot 'android\local.properties') -Encoding ascii
# This host rejects AF_UNIX connections; a non-existing socket directory forces the JDK TCP fallback.
$env:JAVA_TOOL_OPTIONS='-Djdk.net.unixdomain.tmpdir="' + (Join-Path $taskProjectRoot '.tools\disabled-unix-sockets').Replace('\','/') + '"'
if ($UseLocalProxy) { $env:JAVA_TOOL_OPTIONS += ' -DsocksProxyHost=127.0.0.1 -DsocksProxyPort=1080 -DsocksNonProxyHosts=localhost|127.*|[::1]' }
Push-Location (Join-Path $taskProjectRoot 'android')
try {
    if (Test-Path '.\gradlew.bat') { & .\gradlew.bat assembleDebug testDebugUnitTest --console=plain } else { & (Join-Path $taskProjectRoot '.tools\gradle-9.3.1\bin\gradle.bat') wrapper --gradle-version 9.3.1 assembleDebug testDebugUnitTest --console=plain }
    if ($LASTEXITCODE -ne 0) { throw 'Android build failed' }
    if ((Get-FileHash -LiteralPath (Join-Path $taskProjectRoot 'version.properties') -Algorithm SHA256).Hash -ne $taskAndroidVersionHash) { throw 'Version configuration changed during Android build' }
    $taskAndroidApk = Get-TapDeckAndroidBuild $taskProjectRoot $taskAndroidVersions
    # Inspect the packaged manifest too, not just Gradle's adjacent JSON metadata.
    $taskAapt = Get-ChildItem -LiteralPath (Join-Path $SdkRoot 'build-tools') -Directory |
        Where-Object { $_.Name -match '^\d+\.\d+\.\d+$' -and (Test-Path -LiteralPath (Join-Path $_.FullName 'aapt.exe')) } |
        Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1
    if (-not $taskAapt) { throw 'Android SDK aapt.exe is required to verify APK versions' }
    $taskBadging = & (Join-Path $taskAapt.FullName 'aapt.exe') dump badging $taskAndroidApk.Path
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect Android APK manifest' }
    $taskPackage = @($taskBadging | Where-Object { $_ -match '^package:' })
    if ($taskPackage.Count -ne 1 -or $taskPackage[0] -notmatch "name='com\.yuncii\.tapdeck' versionCode='([0-9]+)' versionName='([^']+)'") { throw 'Invalid Android APK package metadata' }
    if ([int]$Matches[1] -ne $taskAndroidVersions.AndroidVersionCode -or $Matches[2] -cne $taskAndroidVersions.AndroidVersion) { throw 'APK manifest differs from version.properties' }
    New-Item -ItemType Directory -Path (Join-Path $taskProjectRoot 'dist') -Force | Out-Null
    $taskVersionedApk = Join-Path $taskProjectRoot ('dist\' + $taskAndroidApk.Name)
    foreach ($taskApkCopy in @($taskVersionedApk, (Join-Path $taskProjectRoot 'dist\TapDeck-debug.apk'))) {
        Copy-Item -LiteralPath $taskAndroidApk.Path -Destination $taskApkCopy -Force
        if ((Get-FileHash -LiteralPath $taskApkCopy -Algorithm SHA256).Hash.ToLowerInvariant() -cne $taskAndroidApk.SHA256) { throw 'Android distribution APK hash mismatch' }
    }
    Write-Host ('Android 安装包：' + $taskVersionedApk)
} finally { Pop-Location }
