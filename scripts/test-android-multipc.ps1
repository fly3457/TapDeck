param([string]$Serial = 'emulator-5560', [string]$OutputDirectory, [switch]$SkipBuild)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'versioning.ps1')
$taskVersions = Get-TapDeckVersions $taskProjectRoot
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $taskProjectRoot ('dist/' + $taskVersions.ReceiverVersion + '/multipc') }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$taskAdb = Join-Path $env:LOCALAPPDATA 'Android/Sdk/platform-tools/adb.exe'
function Invoke-MultiPcDevice([string[]]$Arguments) {
    $taskResult = & $taskAdb -s $Serial @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw ($taskResult -join "`n") }
    return $taskResult
}
if ((Invoke-MultiPcDevice @('shell', 'getprop', 'ro.kernel.qemu') | Out-String).Trim() -ne '1') { throw 'An isolated emulator is required' }
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
if (-not $SkipBuild) {
    & (Join-Path $PSScriptRoot 'build-android.ps1')
    Push-Location (Join-Path $taskProjectRoot 'android')
    try {
        & .\gradlew.bat assembleDebugAndroidTest --console=plain
        if ($LASTEXITCODE -ne 0) { throw 'Android instrumentation build failed' }
    } finally { Pop-Location }
}
Invoke-MultiPcDevice @('install', '-r', (Join-Path $taskProjectRoot 'android/app/build/outputs/apk/debug/app-debug.apk'))
Invoke-MultiPcDevice @('install', '-r', (Join-Path $taskProjectRoot 'android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk'))
$taskInfo = Join-Path $OutputDirectory ('fixture-' + [guid]::NewGuid().ToString('N') + '.json')
$taskOldFixtureEnv = $env:TAPDECK_ANDROID_FIXTURE_INFO
$taskFixtureProcess = $null
$taskFixture = $null
try {
    $env:TAPDECK_ANDROID_FIXTURE_INFO = $taskInfo
    $taskFixtureProcess = Start-Process -FilePath (Get-Command go).Source -ArgumentList @('test', './internal/server', '-run', '^TestAndroidMultiPCFixture$', '-count=1', '-timeout', '10m', '-v') -WorkingDirectory (Join-Path $taskProjectRoot 'windows') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $OutputDirectory 'fixture.log') -RedirectStandardError (Join-Path $OutputDirectory 'fixture.err')
    $taskDeadline = [DateTime]::UtcNow.AddSeconds(30)
    while (-not (Test-Path -LiteralPath $taskInfo)) {
        if ($taskFixtureProcess.HasExited -or [DateTime]::UtcNow -gt $taskDeadline) { throw 'PC fixtures failed to start; see fixture.log and fixture.err' }
        Start-Sleep -Milliseconds 100
    }
    $taskFixture = Get-Content -LiteralPath $taskInfo -Raw | ConvertFrom-Json
    Invoke-MultiPcDevice @('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP') | Out-Null
    Invoke-MultiPcDevice @('shell', 'wm', 'dismiss-keyguard') | Out-Null
    $taskResult = Invoke-MultiPcDevice @('shell', 'am', 'instrument', '-w', '-e', 'pcA', $taskFixture.urls[0], '-e', 'pcB', $taskFixture.urls[1], '-e', 'pcC', $taskFixture.urls[2], '-e', 'pcAdmin', $taskFixture.admin, '-e', 'class', 'com.yuncii.tapdeck.MultiPcTest', 'com.yuncii.tapdeck.test/androidx.test.runner.AndroidJUnitRunner') | Out-String
    [IO.File]::WriteAllText((Join-Path $OutputDirectory 'android.log'), $taskResult, [Text.UTF8Encoding]::new($false))
    if ($taskResult -notmatch '(?m)^OK \(\d+ tests?\)\s*$') { throw $taskResult }
    Write-Host 'Multi-PC migration, UI and three-receiver integration tests passed.'
} finally {
    $env:TAPDECK_ANDROID_FIXTURE_INFO = $taskOldFixtureEnv
    try { Invoke-MultiPcDevice @('pull', '/sdcard/Android/data/com.yuncii.tapdeck/files/ui-validation/.', $OutputDirectory) | Out-Null } catch { Write-Warning $_ }
    if ($taskFixture) {
        try { Invoke-RestMethod -Method Post -Uri ($taskFixture.admin.Replace('10.0.2.2', '127.0.0.1') + '/control?action=finish') | Out-Null } catch { Write-Warning $_ }
    }
    if ($taskFixtureProcess) {
        if (-not $taskFixtureProcess.WaitForExit(5000)) { $taskFixtureProcess.Kill($true) }
        $taskFixtureProcess.Dispose()
    }
}
