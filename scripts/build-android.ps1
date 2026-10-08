param([string]$JavaHome,[string]$SdkRoot,[switch]$UseLocalProxy)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
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
    New-Item -ItemType Directory -Path (Join-Path $taskProjectRoot 'dist') -Force | Out-Null
    Copy-Item '.\app\build\outputs\apk\debug\app-debug.apk' (Join-Path $taskProjectRoot 'dist\TapDeck-debug.apk') -Force
    $taskApkMetadata = Get-Content -Raw -LiteralPath '.\app\build\outputs\apk\debug\output-metadata.json' | ConvertFrom-Json
    $taskApkElement = @($taskApkMetadata.elements | Where-Object { $_.outputFile -eq 'app-debug.apk' })
    if ($taskApkElement.Count -ne 1 -or $taskApkElement[0].versionName -notmatch '^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$') { throw 'Android APK version metadata missing or invalid' }
    $taskVersionedApk = Join-Path $taskProjectRoot ('dist\TapDeck-' + $taskApkElement[0].versionName + '.apk')
    Copy-Item -LiteralPath '.\app\build\outputs\apk\debug\app-debug.apk' -Destination $taskVersionedApk -Force
    Write-Host ('Android 安装包：' + $taskVersionedApk)
} finally { Pop-Location }
