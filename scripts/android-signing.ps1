# Explicit signing only; never fall back to an SDK debug keystore or create a key.
function Invoke-TapDeckAndroidSigning([scriptblock]$Action) {
    $taskNames = @('STORE_FILE', 'STORE_PASSWORD', 'KEY_ALIAS', 'KEY_PASSWORD')
    $taskPrevious = @{}
    foreach ($taskName in $taskNames) {
        $taskPrevious[$taskName] = [Environment]::GetEnvironmentVariable('TAPDECK_ANDROID_' + $taskName, 'Process')
    }
    try {
        $taskProvided = @($taskNames | Where-Object { -not [string]::IsNullOrEmpty($taskPrevious[$_]) })
        if ($taskProvided.Count -gt 0 -and $taskProvided.Count -ne $taskNames.Count) {
            throw 'Incomplete TAPDECK_ANDROID_* signing environment; all four values are required'
        }
        if ($taskProvided.Count -eq 0) {
            $taskConfigPath = $env:TAPDECK_ANDROID_SIGNING_CONFIG
            if (-not $taskConfigPath) { $taskConfigPath = Join-Path $env:USERPROFILE '.tapdeck\android-signing.xml' }
            if (-not (Test-Path -LiteralPath $taskConfigPath -PathType Leaf)) {
                throw 'Release signing is not configured. See docs/android-release.md; debug signing is never used as a fallback.'
            }
            $taskConfig = Import-Clixml -LiteralPath $taskConfigPath
            if (-not $taskConfig.StoreFile -or -not $taskConfig.KeyAlias -or
                $taskConfig.StorePassword -isnot [Security.SecureString] -or $taskConfig.KeyPassword -isnot [Security.SecureString]) {
                throw 'Invalid protected Android signing configuration'
            }
            $taskValues = @{
                STORE_FILE = $taskConfig.StoreFile
                KEY_ALIAS = $taskConfig.KeyAlias
                STORE_PASSWORD = [Net.NetworkCredential]::new('', $taskConfig.StorePassword).Password
                KEY_PASSWORD = [Net.NetworkCredential]::new('', $taskConfig.KeyPassword).Password
            }
            foreach ($taskName in $taskNames) {
                if ([string]::IsNullOrEmpty($taskValues[$taskName])) { throw 'Empty Android signing configuration value' }
                [Environment]::SetEnvironmentVariable('TAPDECK_ANDROID_' + $taskName, $taskValues[$taskName], 'Process')
            }
        }
        $taskStore = [Environment]::GetEnvironmentVariable('TAPDECK_ANDROID_STORE_FILE', 'Process')
        if (-not [IO.Path]::IsPathFullyQualified($taskStore) -or -not (Test-Path -LiteralPath $taskStore -PathType Leaf)) {
            throw 'TAPDECK_ANDROID_STORE_FILE must point to an existing absolute keystore path'
        }
        & $Action
    } finally {
        foreach ($taskName in $taskNames) {
            [Environment]::SetEnvironmentVariable('TAPDECK_ANDROID_' + $taskName, $taskPrevious[$taskName], 'Process')
        }
        $taskValues = $null
        $taskConfig = $null
    }
}
