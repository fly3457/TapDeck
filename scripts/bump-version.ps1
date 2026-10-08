param(
    [ValidateSet('All', 'Receiver', 'Android')][string]$Target = 'All',
    [ValidateSet('Patch', 'Minor', 'Major')][string]$Part = 'Patch',
    [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot)
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'versioning.ps1')
$taskVersionPath = Join-Path ([IO.Path]::GetFullPath($ProjectRoot)) 'version.properties'
$taskOriginal = [IO.File]::ReadAllText($taskVersionPath)
$taskVersions = ConvertFrom-TapDeckVersionText $taskOriginal
# A controller update also changes the receiver's embedded distribution.
$taskReceiverPart = if ($Target -eq 'Android') { 'Patch' } else { $Part }
$taskNextReceiver = Get-TapDeckNextVersion $taskVersions.ReceiverVersion $taskReceiverPart
$taskUpdated = [regex]::Replace($taskOriginal, '(?m)^receiver\.version=[^\r\n]+', ('receiver.version=' + $taskNextReceiver))
if ($Target -ne 'Receiver') {
    $taskNextAndroid = Get-TapDeckNextVersion $taskVersions.AndroidVersion $Part
    $taskUpdated = [regex]::Replace($taskUpdated, '(?m)^controller\.android\.version=[^\r\n]+', ('controller.android.version=' + $taskNextAndroid))
    $taskUpdated = [regex]::Replace($taskUpdated, '(?m)^controller\.android\.versionCode=[^\r\n]+', ('controller.android.versionCode=' + ($taskVersions.AndroidVersionCode + 1)))
}
$taskNext = ConvertFrom-TapDeckVersionText $taskUpdated
if ($taskNext.ReceiverVersion -ceq $taskVersions.ReceiverVersion -or
    ($Target -ne 'Receiver' -and ($taskNext.AndroidVersion -ceq $taskVersions.AndroidVersion -or $taskNext.AndroidVersionCode -le $taskVersions.AndroidVersionCode))) {
    throw 'Version update did not advance the requested targets'
}
# Validate first, then replace a single file atomically; failures retain the original.
$taskTemporary = $taskVersionPath + '.' + [guid]::NewGuid().ToString('N') + '.tmp'
try {
    [IO.File]::WriteAllText($taskTemporary, $taskUpdated, [Text.UTF8Encoding]::new($false))
    [IO.File]::Replace($taskTemporary, $taskVersionPath, [NullString]::Value)
} finally {
    if (Test-Path -LiteralPath $taskTemporary) { Remove-Item -LiteralPath $taskTemporary -Force }
}
Write-Host ('Receiver {0}; Android {1} (code {2}). Update current-version docs and verification before committing.' -f $taskNext.ReceiverVersion, $taskNext.AndroidVersion, $taskNext.AndroidVersionCode)
