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
