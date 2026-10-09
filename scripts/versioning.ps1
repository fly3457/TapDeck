# Shared release metadata and packaging checks. No build or mutation when dot-sourced.
function Assert-TapDeckReleaseVersion([string]$Version) {
    if ($Version -cnotmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
        throw "Invalid release version: $Version (expected major.minor.patch)"
    }
    foreach ($taskComponent in $Version.Split('.')) {
        [uint16]$taskNumber = 0
        if (-not [uint16]::TryParse($taskComponent, [ref]$taskNumber)) { throw "Version component exceeds 65535: $Version" }
    }
}

function ConvertFrom-TapDeckVersionText([string]$Text) {
    $taskValues = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::Ordinal)
    foreach ($taskLine in ($Text -split '\r?\n')) {
        $taskLine = $taskLine.Trim()
        if (-not $taskLine -or $taskLine.StartsWith('#')) { continue }
        if ($taskLine -cnotmatch '^([a-zA-Z][a-zA-Z0-9.]*)=([^\s]+)$') { throw "Invalid version.properties line: $taskLine" }
        $taskKey = $Matches[1]
        if ($taskValues.ContainsKey($taskKey)) { throw "Duplicate version key: $taskKey" }
        $taskValues[$taskKey] = $Matches[2]
    }
    foreach ($taskKey in @('receiver.version', 'controller.android.version', 'controller.android.versionCode')) {
        if (-not $taskValues.ContainsKey($taskKey)) { throw "Missing version key: $taskKey" }
    }
    Assert-TapDeckReleaseVersion $taskValues['receiver.version']
    Assert-TapDeckReleaseVersion $taskValues['controller.android.version']
    [int]$taskCode = 0
    if ($taskValues['controller.android.versionCode'] -notmatch '^[1-9][0-9]*$' -or
        -not [int]::TryParse($taskValues['controller.android.versionCode'], [ref]$taskCode) -or $taskCode -gt 2100000000) {
        throw 'Android versionCode must be in 1..2100000000'
    }
    return [pscustomobject]@{
        ReceiverVersion = $taskValues['receiver.version']
        AndroidVersion = $taskValues['controller.android.version']
        AndroidVersionCode = $taskCode
    }
}

function Get-TapDeckVersions([string]$ProjectRoot) {
    return ConvertFrom-TapDeckVersionText ([IO.File]::ReadAllText((Join-Path $ProjectRoot 'version.properties')))
}

function Get-TapDeckNextVersion([string]$Version, [ValidateSet('Patch', 'Minor', 'Major')][string]$Part = 'Patch') {
    Assert-TapDeckReleaseVersion $Version
    $taskParts = @($Version.Split('.') | ForEach-Object { [int]$_ })
    switch ($Part) {
        'Major' { $taskParts[0]++; $taskParts[1] = 0; $taskParts[2] = 0 }
        'Minor' { $taskParts[1]++; $taskParts[2] = 0 }
        'Patch' { $taskParts[2]++ }
    }
    $taskNext = $taskParts -join '.'
    Assert-TapDeckReleaseVersion $taskNext
    return $taskNext
}

function Get-TapDeckAndroidBuild([string]$ProjectRoot, $Versions) {
    $taskOutput = Join-Path $ProjectRoot 'android\app\build\outputs\apk\release'
    $taskMetadata = Get-Content -LiteralPath (Join-Path $taskOutput 'output-metadata.json') -Raw | ConvertFrom-Json
    $taskElements = @($taskMetadata.elements)
    if ($taskMetadata.applicationId -cne 'com.yuncii.tapdeck' -or $taskMetadata.variantName -cne 'release' -or $taskElements.Count -ne 1 -or
        $taskElements[0].outputFile -cne 'app-release.apk') { throw 'Signed release APK metadata missing or ambiguous' }
    $taskElement = $taskElements[0]
    if ($taskElement.versionName -cne $Versions.AndroidVersion -or $taskElement.versionCode -ne $Versions.AndroidVersionCode) {
        throw 'Android build version differs from version.properties; refusing stale APK'
    }
    $taskPath = Join-Path $taskOutput 'app-release.apk'
    $taskFile = Get-Item -LiteralPath $taskPath
    if ($taskFile.Length -le 0) { throw 'Android APK is empty' }
    return [pscustomobject]@{
        Path = $taskPath
        Name = 'TapDeck-' + $Versions.AndroidVersion + '.apk'
        VersionName = $Versions.AndroidVersion
        VersionCode = $Versions.AndroidVersionCode
        SHA256 = (Get-FileHash -LiteralPath $taskPath -Algorithm SHA256).Hash.ToLowerInvariant()
        Bytes = $taskFile.Length
        BuildType = 'release'
        Debuggable = $null
        CertificateSHA256 = $null
        SignatureVerified = $false
    }
}

function Assert-TapDeckAndroidManifest([string[]]$Badging, $Versions) {
    $taskPackage = @($Badging | Where-Object { $_ -match '^package:' })
    if ($taskPackage.Count -ne 1 -or $taskPackage[0] -notmatch "name='com\.yuncii\.tapdeck' versionCode='([0-9]+)' versionName='([^']+)'") {
        throw 'Invalid Android APK package metadata'
    }
    if ([int]$Matches[1] -ne $Versions.AndroidVersionCode -or $Matches[2] -cne $Versions.AndroidVersion) {
        throw 'APK manifest differs from version.properties'
    }
    if (@($Badging | Where-Object { $_ -match '^application-debuggable(?:\s|$)' }).Count -ne 0) {
        throw 'Debuggable Android APK cannot be distributed or embedded'
    }
}

function Assert-TapDeckAndroidSignature([string[]]$Signature, [string]$ExpectedCertificate) {
    if ($ExpectedCertificate -cnotmatch '^[0-9a-f]{64}$') { throw 'Invalid expected release certificate SHA-256' }
    $taskDigests = @($Signature | Where-Object { $_ -match '^Signer #\d+ certificate SHA-256 digest:' })
    if (@($Signature | Where-Object { $_ -ceq 'Verifies' }).Count -ne 1 -or $taskDigests.Count -ne 1 -or
        $taskDigests[0] -cne ('Signer #1 certificate SHA-256 digest: ' + $ExpectedCertificate)) {
        throw 'APK signature is invalid or differs from the pinned release certificate'
    }
}

function Assert-TapDeckAndroidReleaseApk($Apk, $Versions, [string]$ProjectRoot, [string]$SdkRoot) {
    $taskTools = Get-ChildItem -LiteralPath (Join-Path $SdkRoot 'build-tools') -Directory |
        Where-Object { $_.Name -match '^\d+\.\d+\.\d+$' -and (Test-Path -LiteralPath (Join-Path $_.FullName 'aapt.exe')) -and
            (Test-Path -LiteralPath (Join-Path $_.FullName 'apksigner.bat')) } |
        Sort-Object { [version]$_.Name } -Descending | Select-Object -First 1
    if (-not $taskTools) { throw 'Android SDK aapt and apksigner are required to verify release APKs' }
    $taskBadging = & (Join-Path $taskTools.FullName 'aapt.exe') dump badging $Apk.Path
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect Android APK manifest' }
    Assert-TapDeckAndroidManifest $taskBadging $Versions
    $taskSignature = & (Join-Path $taskTools.FullName 'apksigner.bat') verify --verbose --print-certs $Apk.Path
    if ($LASTEXITCODE -ne 0) { throw 'Android APK signature verification failed' }
    $taskCertificate = [IO.File]::ReadAllText((Join-Path $ProjectRoot 'android\release-certificate.sha256')).Trim()
    Assert-TapDeckAndroidSignature $taskSignature $taskCertificate
    if ((Get-FileHash -LiteralPath $Apk.Path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $Apk.SHA256) {
        throw 'Android APK changed during security verification'
    }
    $Apk.Debuggable = $false
    $Apk.CertificateSHA256 = $taskCertificate
    $Apk.SignatureVerified = $true
}

function Assert-TapDeckApkInfo($Info, $Versions, $Apk) {
    if (-not $Apk.SignatureVerified -or $Info.build_type -cne 'release' -or $Info.debuggable -isnot [bool] -or $Info.debuggable -or
        $Info.certificate_sha256 -cne $Apk.CertificateSHA256 -or $Info.pc_version -cne $Versions.ReceiverVersion -or
        $Info.version_name -cne $Versions.AndroidVersion -or $Info.version_code -ne $Versions.AndroidVersionCode -or
        $Info.filename -cne $Apk.Name -or $Info.sha256 -cne $Apk.SHA256 -or $Info.bytes -ne $Apk.Bytes) {
        throw 'EXE contains an unexpected controller APK or receiver version'
    }
}
