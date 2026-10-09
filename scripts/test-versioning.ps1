$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'versioning.ps1')
. (Join-Path $PSScriptRoot 'android-signing.ps1')
$taskTestParent = [IO.Path]::GetFullPath((Join-Path (Split-Path -Parent $PSScriptRoot) '.tools'))
$taskTestRoot = Join-Path $taskTestParent ('versioning-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskTestRoot -Force | Out-Null
$taskTestVersionPath = Join-Path $taskTestRoot 'version.properties'
$script:taskVersionChecks = 0
function Assert-VersionCheck([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
    $script:taskVersionChecks++
}
function Assert-VersionFailure([scriptblock]$Action, [string]$Message) {
    $taskRejected = $false
    try { & $Action | Out-Null } catch { $taskRejected = $true }
    Assert-VersionCheck $taskRejected $Message
}
function Set-VersionFixture([string]$Receiver = '1.2.9', [string]$Android = '2.0.3', [int]$Code = 23) {
    [IO.File]::WriteAllText($taskTestVersionPath, "# fixture`nreceiver.version=$Receiver`ncontroller.android.version=$Android`ncontroller.android.versionCode=$Code`n", [Text.UTF8Encoding]::new($false))
}
try {
    Set-VersionFixture
    $taskSplit = Get-TapDeckVersions $taskTestRoot
    Assert-VersionCheck ($taskSplit.ReceiverVersion -eq '1.2.9' -and $taskSplit.AndroidVersion -eq '2.0.3') 'Independent versions rejected'
    foreach ($taskInvalid in @('1.2', '1.02.3', '1.2.3-beta', '65536.0.0', '../1.2.3')) {
        Assert-VersionFailure { Assert-TapDeckReleaseVersion $taskInvalid } "Invalid version accepted: $taskInvalid"
    }
    $taskValidText = [IO.File]::ReadAllText($taskTestVersionPath)
    Assert-VersionFailure { ConvertFrom-TapDeckVersionText ($taskValidText + "receiver.version=9.0.0`n") } 'Duplicate receiver version accepted'
    Assert-VersionFailure { ConvertFrom-TapDeckVersionText 'receiver.version=1.0.0' } 'Missing controller version accepted'
    Assert-VersionFailure { ConvertFrom-TapDeckVersionText ($taskValidText.Replace('versionCode', 'versioncode')) } 'Wrong-case Gradle property accepted'
    foreach ($taskCode in @('0', '-1', '2100000001', '99999999999999999')) {
        Assert-VersionFailure { ConvertFrom-TapDeckVersionText ($taskValidText.Replace('versionCode=23', ('versionCode=' + $taskCode))) } "Invalid code accepted: $taskCode"
    }
    & (Join-Path $PSScriptRoot 'bump-version.ps1') -ProjectRoot $taskTestRoot -Target Receiver
    $taskNext = Get-TapDeckVersions $taskTestRoot
    Assert-VersionCheck ($taskNext.ReceiverVersion -eq '1.2.10' -and $taskNext.AndroidVersion -eq '2.0.3' -and $taskNext.AndroidVersionCode -eq 23) 'Receiver update changed Android'
    & (Join-Path $PSScriptRoot 'bump-version.ps1') -ProjectRoot $taskTestRoot -Target Android -Part Minor
    $taskNext = Get-TapDeckVersions $taskTestRoot
    Assert-VersionCheck ($taskNext.ReceiverVersion -eq '1.2.11' -and $taskNext.AndroidVersion -eq '2.1.0' -and $taskNext.AndroidVersionCode -eq 24) 'Android update did not advance bundled receiver'
    & (Join-Path $PSScriptRoot 'bump-version.ps1') -ProjectRoot $taskTestRoot -Target All -Part Major
    $taskNext = Get-TapDeckVersions $taskTestRoot
    Assert-VersionCheck ($taskNext.ReceiverVersion -eq '2.0.0' -and $taskNext.AndroidVersion -eq '3.0.0' -and $taskNext.AndroidVersionCode -eq 25) 'All-target major update incorrect'
    Set-VersionFixture -Code 2100000000
    $taskBefore = [IO.File]::ReadAllText($taskTestVersionPath)
    Assert-VersionFailure { & (Join-Path $PSScriptRoot 'bump-version.ps1') -ProjectRoot $taskTestRoot -Target Android } 'Overflow update succeeded'
    Assert-VersionCheck ([IO.File]::ReadAllText($taskTestVersionPath) -ceq $taskBefore) 'Failed update damaged original versions'

    Set-VersionFixture
    $taskVersions = Get-TapDeckVersions $taskTestRoot
    $taskApkOutput = Join-Path $taskTestRoot 'android\app\build\outputs\apk\release'
    New-Item -ItemType Directory -Path $taskApkOutput -Force | Out-Null
    $taskMetadataPath = Join-Path $taskApkOutput 'output-metadata.json'
    $taskApkPath = Join-Path $taskApkOutput 'app-release.apk'
    [IO.File]::WriteAllText($taskApkPath, 'new controller package fixture')
    $taskMetadata = @{ applicationId = 'com.yuncii.tapdeck'; variantName = 'release'; elements = @(@{ outputFile = 'app-release.apk'; versionName = '2.0.2'; versionCode = 22 }) }
    [IO.File]::WriteAllText($taskMetadataPath, ($taskMetadata | ConvertTo-Json -Depth 4))
    Assert-VersionFailure { Get-TapDeckAndroidBuild $taskTestRoot $taskVersions } 'Stale Gradle APK metadata accepted'
    $taskMetadata.elements[0].versionName = '2.0.3'
    $taskMetadata.elements[0].versionCode = 23
    [IO.File]::WriteAllText($taskMetadataPath, ($taskMetadata | ConvertTo-Json -Depth 4))
    $taskApk = Get-TapDeckAndroidBuild $taskTestRoot $taskVersions
    Assert-VersionCheck ($taskApk.Name -eq 'TapDeck-2.0.3.apk') 'APK named using receiver version'
    foreach ($taskWrongOutput in @('app-debug.apk', 'app-release-unsigned.apk', '../debug/app-debug.apk')) {
        $taskMetadata.elements[0].outputFile = $taskWrongOutput
        [IO.File]::WriteAllText($taskMetadataPath, ($taskMetadata | ConvertTo-Json -Depth 4))
        Assert-VersionFailure { Get-TapDeckAndroidBuild $taskTestRoot $taskVersions } "Non-release output accepted: $taskWrongOutput"
    }
    $taskMetadata.elements[0].outputFile = 'app-release.apk'
    $taskMetadata.variantName = 'debug'
    [IO.File]::WriteAllText($taskMetadataPath, ($taskMetadata | ConvertTo-Json -Depth 4))
    Assert-VersionFailure { Get-TapDeckAndroidBuild $taskTestRoot $taskVersions } 'Debug variant metadata accepted'
    $taskMetadata.variantName = 'release'
    [IO.File]::WriteAllText($taskMetadataPath, ($taskMetadata | ConvertTo-Json -Depth 4))
    $taskBadging = @("package: name='com.yuncii.tapdeck' versionCode='23' versionName='2.0.3'", "application: label='TapDeck'")
    Assert-TapDeckAndroidManifest $taskBadging $taskVersions
    $script:taskVersionChecks++
    Assert-VersionFailure { Assert-TapDeckAndroidManifest ($taskBadging + 'application-debuggable') $taskVersions } 'Debuggable APK accepted'
    Assert-VersionFailure { Assert-TapDeckAndroidManifest @($taskBadging[0].Replace("versionCode='23'", "versionCode='22'")) $taskVersions } 'Stale APK manifest accepted'
    Assert-VersionFailure { Assert-TapDeckAndroidManifest @($taskBadging[0].Replace('com.yuncii.tapdeck', 'com.yuncii.tapdeck.debug')) $taskVersions } 'Debug package accepted'
    $taskCertificate = 'a' * 64
    $taskSignature = @('Verifies', 'Number of signers: 1', ('Signer #1 certificate SHA-256 digest: ' + $taskCertificate))
    Assert-TapDeckAndroidSignature $taskSignature $taskCertificate
    $script:taskVersionChecks++
    Assert-VersionFailure { Assert-TapDeckAndroidSignature @('DOES NOT VERIFY') $taskCertificate } 'Unsigned APK accepted'
    Assert-VersionFailure { Assert-TapDeckAndroidSignature $taskSignature ('b' * 64) } 'Unexpected certificate accepted'
    Assert-VersionFailure { Assert-TapDeckAndroidSignature ($taskSignature + ('Signer #2 certificate SHA-256 digest: ' + $taskCertificate)) $taskCertificate } 'Multiple APK signers accepted'
    $taskGoodInfo = @{ pc_version = '1.2.9'; version_name = '2.0.3'; version_code = 23; filename = $taskApk.Name; sha256 = $taskApk.SHA256; bytes = $taskApk.Bytes;
        build_type = 'release'; debuggable = $false; certificate_sha256 = $taskCertificate }
    Assert-VersionFailure { Assert-TapDeckApkInfo $taskGoodInfo $taskVersions $taskApk } 'Unverified APK embedded'
    $taskApk.Debuggable = $false
    $taskApk.CertificateSHA256 = $taskCertificate
    $taskApk.SignatureVerified = $true
    Assert-TapDeckApkInfo $taskGoodInfo $taskVersions $taskApk
    $script:taskVersionChecks++
    foreach ($taskField in @('pc_version', 'version_name', 'version_code', 'filename', 'sha256', 'bytes', 'build_type', 'debuggable', 'certificate_sha256')) {
        $taskBadInfo = $taskGoodInfo.Clone()
        $taskBadInfo[$taskField] = 'unexpected'
        Assert-VersionFailure { Assert-TapDeckApkInfo $taskBadInfo $taskVersions $taskApk } "Embedded mismatch accepted: $taskField"
    }
    $taskDebugInfo = $taskGoodInfo.Clone()
    $taskDebugInfo.debuggable = $true
    Assert-VersionFailure { Assert-TapDeckApkInfo $taskDebugInfo $taskVersions $taskApk } 'Debuggable embedded APK accepted'
    $taskMissingInfo = $taskGoodInfo.Clone()
    $taskMissingInfo.Remove('debuggable')
    Assert-VersionFailure { Assert-TapDeckApkInfo $taskMissingInfo $taskVersions $taskApk } 'Missing debuggable attestation accepted'
    $taskSigningNames = @('STORE_FILE', 'STORE_PASSWORD', 'KEY_ALIAS', 'KEY_PASSWORD', 'SIGNING_CONFIG')
    $taskSigningPrevious = @{}
    foreach ($taskName in $taskSigningNames) {
        $taskSigningPrevious[$taskName] = [Environment]::GetEnvironmentVariable('TAPDECK_ANDROID_' + $taskName, 'Process')
        [Environment]::SetEnvironmentVariable('TAPDECK_ANDROID_' + $taskName, $null, 'Process')
    }
    try {
        $env:TAPDECK_ANDROID_SIGNING_CONFIG = Join-Path $taskTestRoot 'missing.signing.xml'
        $taskSigningProbe = @{ Called = $false }
        Assert-VersionFailure { Invoke-TapDeckAndroidSigning { $taskSigningProbe.Called = $true } } 'Missing signing config accepted'
        Assert-VersionCheck (-not $taskSigningProbe.Called) 'Build ran without signing configuration'
        $env:TAPDECK_ANDROID_KEY_ALIAS = 'fixture'
        Assert-VersionFailure { Invoke-TapDeckAndroidSigning {} } 'Partial signing config accepted'
        $env:TAPDECK_ANDROID_KEY_ALIAS = $null
        $taskProtectedConfig = [pscustomobject]@{
            StoreFile = $taskApkPath; KeyAlias = 'fixture'
            StorePassword = ConvertTo-SecureString 'test-store-only' -AsPlainText -Force
            KeyPassword = ConvertTo-SecureString 'test-key-only' -AsPlainText -Force
        }
        $env:TAPDECK_ANDROID_SIGNING_CONFIG = Join-Path $taskTestRoot 'fixture.signing.xml'
        $taskProtectedConfig | Export-Clixml -LiteralPath $env:TAPDECK_ANDROID_SIGNING_CONFIG
        Assert-VersionFailure { Invoke-TapDeckAndroidSigning {
            $taskSigningProbe.Called = $true
            Assert-VersionCheck ($env:TAPDECK_ANDROID_STORE_FILE -ceq $taskApkPath -and $env:TAPDECK_ANDROID_KEY_ALIAS -ceq 'fixture') 'Protected config not loaded'
            throw 'Simulated Gradle failure'
        } } 'Build exception swallowed'
        Assert-VersionCheck $taskSigningProbe.Called 'Protected signing action never ran'
        foreach ($taskName in @('STORE_FILE', 'STORE_PASSWORD', 'KEY_ALIAS', 'KEY_PASSWORD')) {
            Assert-VersionCheck ([string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable('TAPDECK_ANDROID_' + $taskName, 'Process'))) 'Signing secret left in environment after failure'
        }
    } finally {
        foreach ($taskName in $taskSigningNames) {
            [Environment]::SetEnvironmentVariable('TAPDECK_ANDROID_' + $taskName, $taskSigningPrevious[$taskName], 'Process')
        }
    }
    [IO.File]::WriteAllText($taskApkPath, '')
    Assert-VersionFailure { Get-TapDeckAndroidBuild $taskTestRoot $taskVersions } 'Empty APK accepted'
    Remove-Item -LiteralPath $taskApkPath -Force
    Assert-VersionFailure { Get-TapDeckAndroidBuild $taskTestRoot $taskVersions } 'Missing APK accepted'
    Write-Host ("Versioning checks passed: $script:taskVersionChecks")
} finally {
    $taskResolved = (Resolve-Path -LiteralPath $taskTestRoot).Path
    if (-not $taskResolved.StartsWith(($taskTestParent + [IO.Path]::DirectorySeparatorChar), [StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path -Leaf $taskResolved) -notlike 'versioning-test-*') { throw 'Refusing cleanup outside version test workspace' }
    Remove-Item -LiteralPath $taskResolved -Recurse -Force
}
