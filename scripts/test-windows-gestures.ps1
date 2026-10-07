param([int]$BrowserPort = 9326, [string]$OutputDirectory)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $taskProjectRoot '.tools\ui-validation' }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$taskEdgePath = Join-Path ${env:ProgramFiles(x86)} 'Microsoft\Edge\Application\msedge.exe'
if (-not (Test-Path -LiteralPath $taskEdgePath)) { throw 'Microsoft Edge not found for native zoom acceptance' }
if ($BrowserPort -lt 1024 -or $BrowserPort -gt 65535) { throw 'Invalid browser debugging port' }
if (Get-NetTCPConnection -LocalPort $BrowserPort -State Listen -ErrorAction SilentlyContinue) { throw 'Browser debugging port is already in use' }
$taskProfile = Join-Path $OutputDirectory ('edge-gesture-profile-' + [guid]::NewGuid().ToString('N'))
$taskPage = Join-Path $OutputDirectory 'gesture-zoom.html'
$taskImage = Join-Path $OutputDirectory 'gesture-zoom.png'
[IO.File]::WriteAllText($taskPage, '<!doctype html><meta charset="utf-8"><title>TapDeckGestureZoomValidation</title><style>body{font:32px sans-serif;background:#f3f4f6;padding:40px}svg{width:1000px;height:600px}</style><h1>TapDeck zoom validation</h1><p>Ctrl + wheel: enlarge and restore</p><svg viewBox="0 0 600 300"><rect width="600" height="300" fill="#cbd5e1"/><circle cx="300" cy="150" r="100" fill="#2563eb"/></svg>')
Add-Type -AssemblyName System.Drawing
$taskBitmap = [Drawing.Bitmap]::new(800, 500)
$taskGraphics = [Drawing.Graphics]::FromImage($taskBitmap)
try {
    $taskGraphics.Clear([Drawing.Color]::LightGray)
    $taskGraphics.FillEllipse([Drawing.Brushes]::RoyalBlue, 200, 50, 400, 400)
    $taskBitmap.Save($taskImage, [Drawing.Imaging.ImageFormat]::Png)
} finally { $taskGraphics.Dispose(); $taskBitmap.Dispose() }
$taskPreviousGesture = $env:TAPDECK_NATIVE_GESTURE_TEST
$taskPreviousEdge = $env:TAPDECK_NATIVE_EDGE_ZOOM
$taskPreviousImage = $env:TAPDECK_NATIVE_EDGE_IMAGE
Write-Host 'Native acceptance briefly opens test windows, Task View and the desktop, then restores them.'
try {
    Start-Process -FilePath $taskEdgePath -ArgumentList @("--remote-debugging-port=$BrowserPort", ('--user-data-dir="' + $taskProfile + '"'), '--no-first-run', '--no-default-browser-check', ([uri]$taskPage).AbsoluteUri) -WindowStyle Hidden | Out-Null
    $taskReadyDeadline = [DateTime]::UtcNow.AddSeconds(15)
    do {
        $taskTargets = try { Invoke-RestMethod -Uri "http://127.0.0.1:$BrowserPort/json/list" -TimeoutSec 2 } catch { @() }
        if (@($taskTargets | Where-Object { $_.title -eq 'TapDeckGestureZoomValidation' }).Count) { break }
        if ([DateTime]::UtcNow -gt $taskReadyDeadline) { throw 'Isolated Edge fixture did not start' }
        Start-Sleep -Milliseconds 200
    } while ($true)
    $env:TAPDECK_NATIVE_GESTURE_TEST = '1'
    $env:TAPDECK_NATIVE_EDGE_ZOOM = "$BrowserPort"
    $env:TAPDECK_NATIVE_EDGE_IMAGE = $taskImage
    Push-Location (Join-Path $taskProjectRoot 'windows')
    try {
        & go test ./internal/keyboard -run '^TestNative' -count=1 -v *> (Join-Path $OutputDirectory 'native-gestures-0.3.3.log')
        $taskResult = $LASTEXITCODE
    } finally { Pop-Location }
    Get-Content -LiteralPath (Join-Path $OutputDirectory 'native-gestures-0.3.3.log')
    if ($taskResult -ne 0) { throw 'Native gesture acceptance failed' }
} finally {
    $env:TAPDECK_NATIVE_GESTURE_TEST = $taskPreviousGesture
    $env:TAPDECK_NATIVE_EDGE_ZOOM = $taskPreviousEdge
    $env:TAPDECK_NATIVE_EDGE_IMAGE = $taskPreviousImage
    # Only the new profile belongs to this run; never terminate ordinary Edge.
    Get-CimInstance Win32_Process -Filter "Name = 'msedge.exe'" | Where-Object {
        $_.ExecutablePath -eq $taskEdgePath -and $_.CommandLine -and $_.CommandLine.Contains($taskProfile)
    } | ForEach-Object { Stop-Process -Id $_.ProcessId -ErrorAction SilentlyContinue }
}
