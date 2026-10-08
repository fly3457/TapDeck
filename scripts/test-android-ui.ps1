param([string]$Serial = 'emulator-5560', [string]$OutputDirectory, [switch]$SkipBuild,
    [ValidateSet('phone-100', 'phone-130', 'small-130', 'tablet-100', 'restricted-100')][string[]]$Variants)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'versioning.ps1')
$taskVersions = Get-TapDeckVersions $taskProjectRoot
$taskAdb = Join-Path $env:LOCALAPPDATA 'Android\Sdk\platform-tools\adb.exe'
if (-not (Test-Path -LiteralPath $taskAdb)) { throw 'Android platform-tools not found' }
function Invoke-TapDeckDevice([string[]]$Arguments) {
    $taskOutput = & $taskAdb -s $Serial @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw ($taskOutput -join "`n") }
    return $taskOutput
}
if ((Invoke-TapDeckDevice -Arguments @('shell', 'getprop', 'ro.kernel.qemu') | Out-String).Trim() -ne '1') {
    throw 'This matrix changes display settings and must run on an Android emulator.'
}
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $taskProjectRoot ('dist\android-' + $taskVersions.AndroidVersion + '\screenshots') }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
if (-not $SkipBuild) {
    & (Join-Path $PSScriptRoot 'build-android.ps1')
    Push-Location (Join-Path $taskProjectRoot 'android')
    try {
        & .\gradlew.bat assembleDebugAndroidTest --console=plain
        if ($LASTEXITCODE -ne 0) { throw 'Android UI test build failed' }
    } finally { Pop-Location }
}
$taskApk = Join-Path $taskProjectRoot 'android\app\build\outputs\apk\debug\app-debug.apk'
$taskTestApk = Join-Path $taskProjectRoot 'android\app\build\outputs\apk\androidTest\debug\app-debug-androidTest.apk'
Invoke-TapDeckDevice -Arguments @('install', '-r', $taskApk)
Invoke-TapDeckDevice -Arguments @('install', '-r', $taskTestApk)
$taskSizeBefore = Invoke-TapDeckDevice -Arguments @('shell', 'wm', 'size') | Out-String
$taskDensityBefore = Invoke-TapDeckDevice -Arguments @('shell', 'wm', 'density') | Out-String
$taskFontBefore = (Invoke-TapDeckDevice -Arguments @('shell', 'settings', 'get', 'system', 'font_scale') | Out-String).Trim()
$taskRestoreSize = if ($taskSizeBefore -match 'Override size:\s*(\d+x\d+)') { $Matches[1] } else { 'reset' }
$taskRestoreDensity = if ($taskDensityBefore -match 'Override density:\s*(\d+)') { $Matches[1] } else { 'reset' }
$taskVariants = @(
    @{ Name = 'phone-100'; Size = '1080x2400'; Density = '420'; Font = '1.0'; Full = $true },
    @{ Name = 'phone-130'; Size = '720x1280'; Density = '300'; Font = '1.3' },
    @{ Name = 'small-130'; Size = '640x960'; Density = '320'; Font = '1.3'; Gestures = $true },
    @{ Name = 'tablet-100'; Size = '1404x1872'; Density = '300'; Font = '1.0' },
    @{ Name = 'restricted-100'; Size = '1080x2400'; Density = '420'; Font = '1.0'; Height = '600'; Gestures = $true }
)
if ($Variants) { $taskVariants = @($taskVariants | Where-Object { $Variants -contains $_.Name }) }
try {
    Invoke-TapDeckDevice -Arguments @('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP') | Out-Null
    Invoke-TapDeckDevice -Arguments @('shell', 'wm', 'dismiss-keyguard') | Out-Null
    foreach ($taskVariant in $taskVariants) {
        Invoke-TapDeckDevice -Arguments @('shell', 'am', 'force-stop', 'com.yuncii.tapdeck') | Out-Null
        Invoke-TapDeckDevice -Arguments @('shell', 'wm', 'size', $taskVariant.Size) | Out-Null
        Invoke-TapDeckDevice -Arguments @('shell', 'wm', 'density', $taskVariant.Density) | Out-Null
        Invoke-TapDeckDevice -Arguments @('shell', 'settings', 'put', 'system', 'font_scale', $taskVariant.Font) | Out-Null
        $taskCases = @('controllerModesAndShortcutVariants', 'connectionIconReminderAndSettings', 'deviceSensitivitySliderPersistsAndKeepsPcConfigIndependent', 'voiceProfileButtonsCycleDisableAndFreeze')
        if ($taskVariant.Full) { $taskCases += @('percentageLayoutAndCameraOptional', 'touchpadSettingsIconCancelsPendingClickAndDoesNotMoveMouse', 'keyFeedbackUsesVibratorServiceWithSystemTouchFeedbackOff', 'launcherIconUsesPcColorsAndSafeAdaptiveLayers', 'toggleVoiceStopsAndConsumesTouchpadClick', 'pcVoiceStopReleasesRecorderAndIgnoresStaleReplies', 'voiceProfilesWireSnapshotAndSpaceGesture', 'pairingRejectionStopsRetryAndIgnoresStaleFailure') }
        if ($taskVariant.Full -or $taskVariant.Gestures) { $taskCases += @('gesturesMoveClickScrollDragAndCancel', 'multiFingerZoomSwipeAndPointerIds', 'keyboardShortLongVoiceAndDisposalRelease', 'compactVoiceAndTouchpadPointersStayIndependent', 'shortcutFeedbackOncePerPressAndDisabledDoesNotTrigger') }
        $taskClassList = ($taskCases | ForEach-Object { 'com.yuncii.tapdeck.DeviceTest#' + $_ }) -join ','
        $taskClassList += ',com.yuncii.tapdeck.MultiPcTest#pickerManagementAndRecordingRestrictions'
        if ($taskVariant.Full) { $taskClassList += ',com.yuncii.tapdeck.DeviceTest#microphoneFramesRetainTheirOriginalSessionAndRecording' }
        $taskArgs = @('shell', 'am', 'instrument', '-w', '-e', 'uiLabel', $taskVariant.Name, '-e', 'class', $taskClassList)
        if ($taskVariant.Height) { $taskArgs += @('-e', 'uiHeight', $taskVariant.Height) }
        $taskArgs += 'com.yuncii.tapdeck.test/androidx.test.runner.AndroidJUnitRunner'
        $taskResult = Invoke-TapDeckDevice -Arguments $taskArgs | Out-String
        [IO.File]::WriteAllText((Join-Path $OutputDirectory ($taskVariant.Name + '.log')), $taskResult, [Text.UTF8Encoding]::new($false))
        if ($taskResult -notmatch '(?m)^OK \(\d+ tests?\)\s*$') { throw ($taskVariant.Name + ': ' + $taskResult) }
        Write-Host ($taskVariant.Name + ': passed ' + ($taskCases.Count + 1 + [int]$taskVariant.Full) + ' tests')
    }
    Invoke-TapDeckDevice -Arguments @('pull', '/sdcard/Android/data/com.yuncii.tapdeck/files/ui-validation/.', $OutputDirectory)
} finally {
    Invoke-TapDeckDevice -Arguments @('shell', 'am', 'force-stop', 'com.yuncii.tapdeck') | Out-Null
    Invoke-TapDeckDevice -Arguments @('shell', 'wm', 'size', $taskRestoreSize) | Out-Null
    Invoke-TapDeckDevice -Arguments @('shell', 'wm', 'density', $taskRestoreDensity) | Out-Null
    if ($taskFontBefore -eq 'null') {
        Invoke-TapDeckDevice -Arguments @('shell', 'settings', 'delete', 'system', 'font_scale') | Out-Null
    } else {
        Invoke-TapDeckDevice -Arguments @('shell', 'settings', 'put', 'system', 'font_scale', $taskFontBefore) | Out-Null
    }
}
