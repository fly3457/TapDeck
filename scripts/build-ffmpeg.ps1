# 构建接收端视频模式内嵌的精简 LGPL ffmpeg.exe。
# 源码包版本与 SHA-256 固定在本脚本中；FFmpeg 源码包另经官方发布密钥 GPG 签名校验（如本机有 gpg）。
# 编译在一次性 Docker ubuntu:24.04 容器内完成，不修改本机或 WSL 环境。
# 产物写入 windows/internal/video/assets：ffmpeg.exe、manifest.json、COPYING.LGPLv2.1。
param(
    [string]$SourceDirectory,
    [string]$Proxy = $env:HTTPS_PROXY,
    # 容器内 apt 使用的 HTTP 代理（例如 http://host.docker.internal:7897）；留空则直连 Ubuntu 软件源。
    [string]$ContainerProxy
)
$ErrorActionPreference = 'Stop'
$taskProjectRoot = Split-Path -Parent $PSScriptRoot
if (-not $SourceDirectory) { $SourceDirectory = Join-Path $taskProjectRoot '.tools\ffmpeg-src' }
$taskAssets = Join-Path $taskProjectRoot 'windows\internal\video\assets'

$taskFFmpegVersion = '8.1.3'
$taskNvcodecVersion = '12.0.16.1'   # 兼容 2022 年底以来的 NVIDIA 驱动（522.25+）
$taskAmfVersion = '1.5.3'
$taskSources = @(
    @{ Name = "ffmpeg-$taskFFmpegVersion.tar.xz"; Url = "https://ffmpeg.org/releases/ffmpeg-$taskFFmpegVersion.tar.xz"; Sha256 = '7138d28c96d9d3e3af4ee3d8cad72741f8ffb40da90c1112235dea3ecd3178a3' },
    @{ Name = "nv-codec-headers-n$taskNvcodecVersion.tar.gz"; Url = "https://github.com/FFmpeg/nv-codec-headers/archive/refs/tags/n$taskNvcodecVersion.tar.gz"; Sha256 = '37e31c7ed0c9bf2da74646a3ec426c38a6d29e60b1fb7bff3e03a99b9412e050' },
    @{ Name = "AMF-headers-v$taskAmfVersion.tar.gz"; Url = "https://github.com/GPUOpen-LibrariesAndSDKs/AMF/releases/download/v$taskAmfVersion/AMF-headers-v$taskAmfVersion.tar.gz"; Sha256 = '0f521ad98041f8001147cded4a660127f18b1c27a801a5cd691bda1fa48381be' }
)
# FFmpeg release signing key <ffmpeg-devel@ffmpeg.org>
$taskFFmpegKey = 'FCF986EA15E6E293A5644F10B4322F04D67658D8'

function Get-TaskSha256([string]$Path) { (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant() }

New-Item -ItemType Directory -Force -Path $SourceDirectory | Out-Null
foreach ($taskSource in $taskSources) {
    $taskPath = Join-Path $SourceDirectory $taskSource.Name
    if (-not (Test-Path -LiteralPath $taskPath) -or (Get-TaskSha256 $taskPath) -ne $taskSource.Sha256) {
        Write-Host "下载 $($taskSource.Url)"
        $taskArgs = @('-sSL', '--max-time', '600', '-o', $taskPath, $taskSource.Url)
        if ($Proxy) { $taskArgs = @('--proxy', $Proxy) + $taskArgs }
        & curl.exe @taskArgs
        if ($LASTEXITCODE -ne 0) { throw "下载失败：$($taskSource.Url)" }
    }
    if ((Get-TaskSha256 $taskPath) -ne $taskSource.Sha256) { throw "源码包哈希不符：$($taskSource.Name)" }
}

$taskGpg = Get-Command gpg.exe -ErrorAction SilentlyContinue
if (-not $taskGpg -and (Test-Path 'C:\Program Files\Git\usr\bin\gpg.exe')) { $taskGpg = Get-Item 'C:\Program Files\Git\usr\bin\gpg.exe' }
if ($taskGpg) {
    $taskTarball = Join-Path $SourceDirectory "ffmpeg-$taskFFmpegVersion.tar.xz"
    $taskSignature = "$taskTarball.asc"
    $taskKeyFile = Join-Path $SourceDirectory 'ffmpeg-devel.asc'
    foreach ($taskPair in @(@($taskSignature, "https://ffmpeg.org/releases/ffmpeg-$taskFFmpegVersion.tar.xz.asc"), @($taskKeyFile, 'https://ffmpeg.org/ffmpeg-devel.asc'))) {
        if (-not (Test-Path -LiteralPath $taskPair[0])) {
            $taskArgs = @('-sSL', '--max-time', '120', '-o', $taskPair[0], $taskPair[1])
            if ($Proxy) { $taskArgs = @('--proxy', $Proxy) + $taskArgs }
            & curl.exe @taskArgs
        }
    }
    $taskHome = Join-Path ([IO.Path]::GetTempPath()) ('tapdeck-gpg-' + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $taskHome | Out-Null
    try {
        $env:GNUPGHOME = $taskHome
        & $taskGpg.FullName -q --import $taskKeyFile 2>$null
        $taskStatus = & $taskGpg.FullName --status-fd 1 --verify $taskSignature $taskTarball 2>$null
        if (-not ($taskStatus -match "VALIDSIG $taskFFmpegKey")) { throw 'FFmpeg 源码包签名校验失败' }
        Write-Host 'FFmpeg 源码包签名有效（FCF9 86EA 15E6 E293 A564 4F10 B432 2F04 D676 58D8）'
    } finally {
        Remove-Item Env:GNUPGHOME -ErrorAction SilentlyContinue
        Remove-Item -Recurse -Force -LiteralPath $taskHome -ErrorAction SilentlyContinue
    }
} else {
    Write-Warning '未找到 gpg，仅使用固定的 SHA-256 校验源码包'
}

& docker info --format '{{.ServerVersion}}' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Docker 引擎未运行，请先启动 Docker Desktop' }

$taskOut = Join-Path ([IO.Path]::GetTempPath()) ('tapdeck-ffmpeg-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $taskOut | Out-Null
try {
    $taskDockerArgs = @('run', '--rm', '-v', "${SourceDirectory}:/src:ro", '-v', "$(Join-Path $PSScriptRoot 'ffmpeg'):/scripts:ro", '-v', "${taskOut}:/out")
    if ($ContainerProxy) { $taskDockerArgs += @('-e', "http_proxy=$ContainerProxy", '-e', "https_proxy=$ContainerProxy") }
    $taskDockerArgs += @('ubuntu:24.04', 'bash', '/scripts/build.sh', $taskFFmpegVersion, $taskNvcodecVersion, $taskAmfVersion)
    & docker @taskDockerArgs
    if ($LASTEXITCODE -ne 0) { throw 'ffmpeg 构建失败' }

    $taskExe = Join-Path $taskOut 'ffmpeg.exe'
    $taskImports = Get-Content -LiteralPath (Join-Path $taskOut 'imports.txt')
    $taskAllowed = 'ADVAPI32|bcrypt|d3d11|dxgi|GDI32|KERNEL32|mfplat|mfuuid|msvcrt|ole32|OLEAUT32|PSAPI|SHELL32|SHLWAPI|USER32|WS2_32|api-ms-win-.*|MFReadWrite|evr|strmiids|d3d12|CFGMGR32|VERSION|dxva2'
    foreach ($taskLine in $taskImports) {
        $taskDll = ($taskLine -replace '.*DLL Name:\s*', '').Trim() -replace '\.dll$', ''
        if ($taskDll -notmatch "^($taskAllowed)$") { throw "ffmpeg.exe 依赖了未预期的 DLL：$taskDll" }
    }
    New-Item -ItemType Directory -Force -Path $taskAssets | Out-Null
    Copy-Item -LiteralPath $taskExe -Destination (Join-Path $taskAssets 'ffmpeg.exe') -Force
    Copy-Item -LiteralPath (Join-Path $taskOut 'COPYING.LGPLv2.1') -Destination (Join-Path $taskAssets 'COPYING.LGPLv2.1') -Force
    $taskManifest = [ordered]@{
        version   = "FFmpeg $taskFFmpegVersion（LGPL-2.1，nv-codec-headers $taskNvcodecVersion，AMF headers $taskAmfVersion）"
        source    = ($taskSources | ForEach-Object { "$($_.Url) sha256=$($_.Sha256)" }) -join '；'
        sha256    = Get-TaskSha256 (Join-Path $taskAssets 'ffmpeg.exe')
        bytes     = (Get-Item -LiteralPath (Join-Path $taskAssets 'ffmpeg.exe')).Length
        configure = (Get-Content -Raw -LiteralPath (Join-Path $taskOut 'configure.txt')).Trim()
    }
    $taskJson = $taskManifest | ConvertTo-Json
    [IO.File]::WriteAllText((Join-Path $taskAssets 'manifest.json'), $taskJson + "`n", (New-Object Text.UTF8Encoding($false)))
    Write-Host "ffmpeg.exe：$($taskManifest.bytes) 字节，SHA-256 $($taskManifest.sha256)"
} finally {
    Remove-Item -Recurse -Force -LiteralPath $taskOut -ErrorAction SilentlyContinue
}
