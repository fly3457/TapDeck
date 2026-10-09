param([string]$Serial)
$ErrorActionPreference='Stop'
$taskProjectRoot=Split-Path -Parent $PSScriptRoot
$taskAdbCommand = Get-Command adb -ErrorAction SilentlyContinue
$taskAdbPath = if ($taskAdbCommand) { $taskAdbCommand.Source } else { Join-Path $env:LOCALAPPDATA 'Android\Sdk\platform-tools\adb.exe' }
if (-not (Test-Path -LiteralPath $taskAdbPath)) { throw 'Android platform-tools (adb) not found' }
$taskAdbArguments = @()
if ($Serial) { $taskAdbArguments = @('-s', $Serial) }
& $taskAdbPath @taskAdbArguments install -r (Join-Path $taskProjectRoot 'dist\TapDeck.apk')
if ($LASTEXITCODE -ne 0) { throw 'APK install failed' }
# Some BOOX firmware disables newly sideloaded applications; enable only this package.
& $taskAdbPath @taskAdbArguments shell pm enable com.yuncii.tapdeck
& $taskAdbPath @taskAdbArguments shell am start -n com.yuncii.tapdeck/.MainActivity
