param([string]$JavaHome,[string]$SdkRoot,[switch]$UseLocalProxy,[switch]$Instrumentation)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'versioning.ps1')
. (Join-Path $PSScriptRoot 'android-signing.ps1')
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
    Invoke-TapDeckAndroidSigning {
        $taskGradleTasks = @('assembleRelease', 'testReleaseUnitTest')
        if ($Instrumentation) { $taskGradleTasks += 'assembleReleaseAndroidTest' }
        if (Test-Path '.\gradlew.bat') { & .\gradlew.bat @taskGradleTasks --console=plain } else { & (Join-Path $taskProjectRoot '.tools\gradle-9.3.1\bin\gradle.bat') wrapper --gradle-version 9.3.1 @taskGradleTasks --console=plain }
        if ($LASTEXITCODE -ne 0) { throw 'Android release build failed' }
    }
    if ((Get-FileHash -LiteralPath (Join-Path $taskProjectRoot 'version.properties') -Algorithm SHA256).Hash -ne $taskAndroidVersionHash) { throw 'Version configuration changed during Android build' }
    $taskAndroidApk = Get-TapDeckAndroidBuild $taskProjectRoot $taskAndroidVersions
    Assert-TapDeckAndroidReleaseApk $taskAndroidApk $taskAndroidVersions $taskProjectRoot $SdkRoot
    New-Item -ItemType Directory -Path (Join-Path $taskProjectRoot 'dist') -Force | Out-Null
    $taskVersionedApk = Join-Path $taskProjectRoot ('dist\' + $taskAndroidApk.Name)
    foreach ($taskApkCopy in @($taskVersionedApk, (Join-Path $taskProjectRoot 'dist\TapDeck.apk'))) {
        Copy-Item -LiteralPath $taskAndroidApk.Path -Destination $taskApkCopy -Force
        if ((Get-FileHash -LiteralPath $taskApkCopy -Algorithm SHA256).Hash.ToLowerInvariant() -cne $taskAndroidApk.SHA256) { throw 'Android distribution APK hash mismatch' }
    }
    Write-Host ('Android 安装包：' + $taskVersionedApk)
} finally { Pop-Location }
