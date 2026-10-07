#Requires -RunAsAdministrator
param([string]$ExecutablePath)
$ErrorActionPreference = 'Stop'
if (-not $ExecutablePath) { $ExecutablePath = Join-Path (Split-Path -Parent $PSScriptRoot) 'dist\TapDeck.exe' }
$taskReceiverPath = (Resolve-Path -LiteralPath $ExecutablePath).Path
foreach ($taskProtocol in @('TCP','UDP')) {
    $taskRuleName = 'TapDeck-LAN-' + $taskProtocol
    $taskExistingRule = Get-NetFirewallRule -Name $taskRuleName -ErrorAction SilentlyContinue
    if ($taskExistingRule) { Remove-NetFirewallRule -Name $taskRuleName }
    New-NetFirewallRule -Name $taskRuleName -DisplayName $taskRuleName -Direction Inbound -Action Allow -Program $taskReceiverPath -Protocol $taskProtocol -RemoteAddress LocalSubnet -Profile Any | Out-Null
}
Write-Output ('已允许 TapDeck 在本地子网接收连接：' + $taskReceiverPath)
