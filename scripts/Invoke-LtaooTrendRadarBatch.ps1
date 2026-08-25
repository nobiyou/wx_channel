[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$RequestPath,
    [Parameter(Mandatory = $true)][string]$GrantPath,
    [Parameter(Mandatory = $true)][string]$RunRoot,
    [Parameter(Mandatory = $true)][string]$RuntimeJournalPath,
    [Parameter(Mandatory = $true)][string]$LtaooExePath,
    [Parameter(Mandatory = $true)][string]$BatchExePath,
    [string]$RouterKind = '',
    [string]$RouterExePath = '',
    [string]$RouterConfigPath = '',
    [string]$RouterCapabilityFingerprint = '',
    [string]$ClashExePath = '',
    [string]$ClashConfigPath = '',
    [switch]$AutoRefreshWechatPage,
    [ValidateRange(1, 65535)][int]$ApiPort = 2022,
    [ValidateRange(1, 65535)][int]$ProxyPort = 2023
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'LtaooRuntime.psm1') -Force
Import-Module (Join-Path $PSScriptRoot 'LtaooRouter.psm1') -Force

function Resolve-RuntimeFile {
    param([Parameter(Mandatory = $true)][string]$LiteralPath)
    $resolved = (Resolve-Path -LiteralPath $LiteralPath).Path
    if (-not [IO.File]::Exists($resolved)) { throw 'runtime_file_missing' }
    $item = Get-Item -Force -LiteralPath $resolved
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'reparse_point_rejected' }
    Assert-LtaooNoReparseInPath -LiteralPath $resolved
    return $resolved
}

function Get-ListenerAbsent {
    param([Parameter(Mandatory = $true)][int[]]$Ports)
    return @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -in $Ports }).Count -eq 0
}

function Stop-JournalLtaooProcess {
    param([Parameter(Mandatory = $true)][object]$Journal)
    $pidValue = [int]$Journal.ltaoo_pid
    if ($pidValue -le 0) { return $true }
    $process = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
    if ($null -eq $process) { return $true }
    try {
        if (-not (Test-LtaooProcessIdentityOrAbsent -ProcessId $pidValue -ExpectedPath ([string]$Journal.ltaoo_path) -ExpectedStartTime ([string]$Journal.ltaoo_start_time) -ExpectedSha256 ([string]$Journal.ltaoo_sha256))) { return $false }
        $process = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
        if ($null -eq $process) { return $true }
        if (-not (Test-LtaooProcessIdentity -ProcessId $pidValue -ExpectedPath ([string]$Journal.ltaoo_path) -ExpectedStartTime ([string]$Journal.ltaoo_start_time) -ExpectedSha256 ([string]$Journal.ltaoo_sha256))) {
            return $null -eq (Get-Process -Id $pidValue -ErrorAction SilentlyContinue)
        }
        Stop-Process -Id $process.Id -Force
        return Wait-LtaooProcessStopped -ProcessId $process.Id -TimeoutMilliseconds 10000 -PollMilliseconds 100
    } catch {
        return $null -eq (Get-Process -Id $pidValue -ErrorAction SilentlyContinue)
    }
}

function Restore-JournalRouterConfig {
    param(
        [Parameter(Mandatory = $true)][object]$Journal,
        [Parameter(Mandatory = $true)][object]$Backend
    )
    $configPath = [string]$Journal.router_config_path
    $backupPath = [string]$Journal.router_backup_path
    if (-not [IO.File]::Exists($configPath)) { return $false }
    if (
        [string]$Journal.router_kind -cne [string]$Backend.Kind -or
        (-not [string]::IsNullOrWhiteSpace([string]$Journal.router_executable_path) -and -not [string]::Equals([IO.Path]::GetFullPath([string]$Journal.router_executable_path), [IO.Path]::GetFullPath([string]$Backend.ExecutablePath), [StringComparison]::OrdinalIgnoreCase)) -or
        (-not [string]::IsNullOrWhiteSpace([string]$Journal.router_executable_sha256) -and [string]$Journal.router_executable_sha256 -cne (Get-LtaooFileHash -LiteralPath ([string]$Backend.ExecutablePath))) -or
        -not [string]::Equals([IO.Path]::GetFullPath($configPath), [IO.Path]::GetFullPath([string]$Backend.ConfigPath), [StringComparison]::OrdinalIgnoreCase)
    ) { return $false }
    try {
        $currentBytes = [IO.File]::ReadAllBytes($configPath)
        $currentHash = Get-LtaooFileHash -LiteralPath $configPath
        $currentDecoded = ConvertFrom-LtaooUtf8Bytes -Bytes $currentBytes
        $currentText = $currentDecoded.Text
        $markerPresent = $currentText.Contains('# TREND_RADAR_WX_BEGIN ' + [string]$Journal.run_id + ' ')
        $action = Get-LtaooRouterRecoveryAction -Backend $Backend -BaselineHash ([string]$Journal.router_baseline_sha256) -TemporaryHash ([string]$Journal.router_temporary_sha256) -CurrentHash $currentHash -MarkerPresent $markerPresent
        if ($action -eq 'already_restored') {
            Test-LtaooRouterConfig -Backend $Backend -ConfigPath $configPath -DataDirectory (Split-Path -Parent $configPath)
            return $true
        }
        if (-not [IO.File]::Exists($backupPath)) { return $false }
        $backupDecoded = ConvertFrom-LtaooUtf8Bytes -Bytes ([IO.File]::ReadAllBytes($backupPath))
        $controller = Get-LtaooRouterController -Backend $Backend -Text $backupDecoded.Text
        switch ($action) {
            'restore_backup' {
                Write-LtaooBytesAtomic -Bytes ([IO.File]::ReadAllBytes($backupPath)) -LiteralPath $configPath
            }
            'remove_marker' {
                $recovered = Remove-LtaooRouterBlock -Backend $Backend -Text $currentText -RunId ([string]$Journal.run_id)
                $candidate = Join-Path ([string]$Journal.run_root) 'router-recovery-candidate.yaml'
                [IO.File]::WriteAllBytes($candidate, (ConvertTo-LtaooUtf8Bytes -Text $recovered -WithBom $currentDecoded.HasBom))
                Test-LtaooRouterConfig -Backend $Backend -ConfigPath $candidate -DataDirectory (Split-Path -Parent $configPath)
                Write-LtaooBytesAtomic -Bytes ([IO.File]::ReadAllBytes($candidate)) -LiteralPath $configPath
                Remove-Item -LiteralPath $candidate -Force
            }
            default { return $false }
        }
        Test-LtaooRouterConfig -Backend $Backend -ConfigPath $configPath -DataDirectory (Split-Path -Parent $configPath)
        Invoke-LtaooRouterReload -Backend $Backend -Controller $controller -ConfigPath $configPath
        $restoredText = (ConvertFrom-LtaooUtf8Bytes -Bytes ([IO.File]::ReadAllBytes($configPath))).Text
        return -not $restoredText.Contains('# TREND_RADAR_WX_')
    } catch { return $false }
}

function Remove-JournalSecrets {
    param(
        [Parameter(Mandatory = $true)][object]$Journal,
        [Parameter(Mandatory = $true)][string]$RuntimeBase,
        [Parameter(Mandatory = $true)][bool]$RemoveRouterBackup
    )
    try {
        $secretsRoot = [IO.Path]::GetFullPath([string]$Journal.secrets_root)
        Assert-LtaooNoReparsePoint -BasePath $RuntimeBase -TargetPath $secretsRoot
        if ([IO.Directory]::Exists($secretsRoot)) {
            foreach ($item in @(Get-ChildItem -Force -LiteralPath $secretsRoot)) {
                if ($item.PSIsContainer) { return $false }
                Remove-Item -LiteralPath $item.FullName -Force
            }
            Remove-Item -LiteralPath $secretsRoot -Force
        }
        if ($RemoveRouterBackup) {
            foreach ($name in @('router-baseline.bin', 'router-candidate.yaml', 'router-recovery-candidate.yaml', 'clash-baseline.bin', 'clash-candidate.yaml', 'clash-recovery-candidate.yaml', 'ltaoo-runtime.yaml')) {
                $target = Join-Path ([string]$Journal.run_root) $name
                Assert-LtaooNoReparsePoint -BasePath $RuntimeBase -TargetPath $target
                if ([IO.File]::Exists($target)) { Remove-Item -LiteralPath $target -Force }
            }
        }
        $backupAbsent = -not [IO.File]::Exists([string]$Journal.router_backup_path)
        return (-not [IO.Directory]::Exists($secretsRoot)) -and $backupAbsent
    } catch { return $false }
}

function Invoke-JournalCleanup {
    param(
        [Parameter(Mandatory = $true)][object]$Journal,
        [Parameter(Mandatory = $true)][string]$RuntimeBase,
        [Parameter(Mandatory = $true)][object]$Backend
    )
    $processStopped = Stop-JournalLtaooProcess -Journal $Journal
    $caAbsent = $true
    if (-not [string]::IsNullOrWhiteSpace([string]$Journal.ca_thumbprint)) {
        try { $caAbsent = Remove-LtaooCurrentUserCA -Thumbprint ([string]$Journal.ca_thumbprint) } catch { $caAbsent = $false }
    }
    $routerRestored = Restore-JournalRouterConfig -Journal $Journal -Backend $Backend
    $secretsDeleted = Remove-JournalSecrets -Journal $Journal -RuntimeBase $RuntimeBase -RemoveRouterBackup $routerRestored
    $portsReleased = Get-ListenerAbsent -Ports @([int]$Journal.api_port, [int]$Journal.proxy_port)
    $safe = $processStopped -and $caAbsent -and $routerRestored -and $secretsDeleted -and $portsReleased
    $reasons = @()
    if (-not $safe) { $reasons = @('cleanup_attention_required') }
    $receipt = [ordered]@{
        schema_version = [int]$Journal.schema_version
        run_id = [string]$Journal.run_id
        safe = [bool]$safe
        ca_absent = [bool]$caAbsent
        process_stopped = [bool]$processStopped
        ports_released = [bool]$portsReleased
        secrets_deleted = [bool]$secretsDeleted
        completed_at = [DateTimeOffset]::UtcNow.ToString('o')
        reason_codes = $reasons
    }
    if ([int]$Journal.schema_version -eq 1) { $receipt.clash_restored = [bool]$routerRestored }
    else { $receipt.router_restored = [bool]$routerRestored }
    return $receipt
}

function Read-RuntimeJournal {
    param([Parameter(Mandatory = $true)][string]$LiteralPath)
    if (-not [IO.File]::Exists($LiteralPath)) { throw 'json_file_missing' }
    $item = Get-Item -Force -LiteralPath $LiteralPath
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.Length -gt 1MB) { throw 'json_file_unsafe' }
    try { $journal = Get-Content -Raw -LiteralPath $LiteralPath | ConvertFrom-Json }
    catch { throw 'json_file_invalid' }
    $commonProperties = @(
        'schema_version', 'run_id', 'run_root', 'phase', 'created_at', 'ca_thumbprint',
        'ltaoo_pid', 'ltaoo_start_time', 'ltaoo_path', 'ltaoo_sha256', 'secrets_root',
        'api_port', 'proxy_port', 'request_path', 'batch_path', 'batch_sha256'
    )
    if ([int]$journal.schema_version -eq 1) {
        $legacyProperties = @($commonProperties) + @('clash_config_path', 'clash_baseline_sha256', 'clash_temporary_sha256', 'clash_backup_path')
        Assert-LtaooAllowedProperties -Value $journal -AllowedProperties $legacyProperties
        $journal = [pscustomobject][ordered]@{
            schema_version = 1; run_id = $journal.run_id; run_root = $journal.run_root; phase = $journal.phase; created_at = $journal.created_at
            ca_thumbprint = $journal.ca_thumbprint; ltaoo_pid = $journal.ltaoo_pid; ltaoo_start_time = $journal.ltaoo_start_time; ltaoo_path = $journal.ltaoo_path; ltaoo_sha256 = $journal.ltaoo_sha256
            router_kind = 'mihomo'; router_executable_path = ''; router_executable_sha256 = ''; router_config_path = $journal.clash_config_path; router_baseline_sha256 = $journal.clash_baseline_sha256
            router_temporary_sha256 = $journal.clash_temporary_sha256; router_backup_path = $journal.clash_backup_path; router_capability_fingerprint = ''
            secrets_root = $journal.secrets_root; api_port = $journal.api_port; proxy_port = $journal.proxy_port; request_path = $journal.request_path; batch_path = $journal.batch_path; batch_sha256 = $journal.batch_sha256
        }
    } elseif ([int]$journal.schema_version -eq 2) {
        $routerProperties = @($commonProperties) + @(
            'router_kind', 'router_executable_path', 'router_executable_sha256', 'router_config_path', 'router_baseline_sha256',
            'router_temporary_sha256', 'router_backup_path', 'router_capability_fingerprint'
        )
        Assert-LtaooAllowedProperties -Value $journal -AllowedProperties $routerProperties
    } else { throw 'runtime_journal_invalid' }
    if ([string]$journal.run_id -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$' -or [string]$journal.router_kind -cne 'mihomo') { throw 'runtime_journal_invalid' }
    return $journal
}

function Invoke-JournalFinalize {
    param(
        [Parameter(Mandatory = $true)][object]$Journal,
        [Parameter(Mandatory = $true)][string]$ReceiptPath
    )
    $priorRunRoot = [IO.Path]::GetFullPath([string]$Journal.run_root)
    $priorRequest = Resolve-RuntimeFile -LiteralPath ([string]$Journal.request_path)
    $priorBatch = Resolve-RuntimeFile -LiteralPath ([string]$Journal.batch_path)
    Assert-LtaooNoReparsePoint -BasePath $priorRunRoot -TargetPath $priorRequest
    if ((Get-LtaooFileHash -LiteralPath $priorBatch) -cne [string]$Journal.batch_sha256) { throw 'batch_executable_changed' }
    foreach ($argumentPath in @($priorRequest, $priorRunRoot, $ReceiptPath)) {
        if ($argumentPath.Contains('"')) { throw 'unsupported_path_character' }
    }
    $draftPath = Join-Path $priorRunRoot ('batch.partial-' + [string]$Journal.run_id)
    $resultPath = Join-Path $priorRunRoot 'collection-result.json'
    if (-not [IO.Directory]::Exists($draftPath) -or -not [IO.File]::Exists($resultPath)) { return $null }
    $arguments = 'finalize --request "' + $priorRequest + '" --run-root "' + $priorRunRoot + '" --cleanup-receipt "' + $ReceiptPath + '"'
    $process = Start-Process -FilePath $priorBatch -ArgumentList $arguments -WorkingDirectory $priorRunRoot -PassThru -Wait -WindowStyle Hidden
    if ($process.ExitCode -notin @(0, 2, 3, 4)) { throw 'batch_finalize_failed' }
    return [int]$process.ExitCode
}

function Test-JournalPublicationClosed {
    param([Parameter(Mandatory = $true)][object]$Journal)
    $runRoot = [IO.Path]::GetFullPath([string]$Journal.run_root)
    $draftPath = Join-Path $runRoot ('batch.partial-' + [string]$Journal.run_id)
    $finalPath = Join-Path $runRoot 'batch'
    return [IO.Directory]::Exists($finalPath) -or -not [IO.Directory]::Exists($draftPath)
}

$genericValues = @($RouterKind, $RouterExePath, $RouterConfigPath, $RouterCapabilityFingerprint)
$legacyValues = @($ClashExePath, $ClashConfigPath)
$genericProvided = @($genericValues | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) }).Count
$legacyProvided = @($legacyValues | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) }).Count
if ($ApiPort -eq $ProxyPort) { exit 64 }
if ($genericProvided -eq 4 -and $legacyProvided -eq 0) {
    if ($RouterKind -cne 'mihomo' -or $RouterCapabilityFingerprint -cnotmatch '^[a-f0-9]{64}$') { exit 64 }
    $authorizationMode = 'wechat-channels-local-runtime-v2'
    $effectiveRouterKind = $RouterKind
    $effectiveRouterExePath = $RouterExePath
    $effectiveRouterConfigPath = $RouterConfigPath
    $effectiveRouterCapabilityFingerprint = $RouterCapabilityFingerprint
} elseif ($genericProvided -eq 0 -and $legacyProvided -eq 2) {
    $authorizationMode = 'wechat-channels-local-runtime-v1'
    $effectiveRouterKind = 'mihomo'
    $effectiveRouterExePath = $ClashExePath
    $effectiveRouterConfigPath = $ClashConfigPath
    $effectiveRouterCapabilityFingerprint = ('0' * 64)
} else { exit 64 }

$mutex = $null
$mutexAcquired = $false
$finalExitCode = 4
$request = $null
$runId = ''
$journal = $null
$journalPath = $null
$runtimeBase = $null
$resolvedRunRoot = $null
$resolvedRequest = $null
$resolvedGrant = $null
$resolvedLtaoo = $null
$resolvedRouter = $null
$resolvedRouterConfig = $null
$routerBackend = $null
$resolvedBatch = $null
$receiptPath = $null
$draftExists = $false
$currentStage = 'startup'

try {
    $currentStage = 'windows_identity'
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $mutexNameHash = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($sid)).Replace('/', '_').Replace('+', '-').TrimEnd('=')
    $mutex = [Threading.Mutex]::new($false, 'Local\TrendRadar-WeChatChannels-' + $mutexNameHash)
    $mutexAcquired = $mutex.WaitOne(0)
    if (-not $mutexAcquired) { exit 3 }

    $currentStage = 'run_root_resolution'
    $resolvedRunRoot = (Resolve-Path -LiteralPath $RunRoot).Path
    $runtimeBase = (Resolve-Path -LiteralPath (Split-Path -Parent $resolvedRunRoot)).Path
    Assert-LtaooNoReparsePoint -BasePath $runtimeBase -TargetPath $resolvedRunRoot
    $currentStage = 'run_root_acl'
    Assert-LtaooOwnerOnlyAcl -LiteralPath $resolvedRunRoot
    $currentStage = 'journal_path'
    $journalPath = [IO.Path]::GetFullPath($RuntimeJournalPath)
    if ([IO.Path]::GetFileName($journalPath) -cne 'runtime-journal.json') { throw 'runtime_journal_path_invalid' }
    Assert-LtaooNoReparsePoint -BasePath $runtimeBase -TargetPath $journalPath

    $currentStage = 'router_executable'
    $resolvedRouter = Resolve-RuntimeFile -LiteralPath $effectiveRouterExePath
    $resolvedRouterConfig = Resolve-RuntimeFile -LiteralPath $effectiveRouterConfigPath
    $routerBackend = New-LtaooRouterBackend -RouterKind $effectiveRouterKind -ExecutablePath $resolvedRouter -ConfigPath $resolvedRouterConfig
    if ([IO.File]::Exists($journalPath)) {
        $currentStage = 'journal_recovery'
        $prior = Read-RuntimeJournal -LiteralPath $journalPath
        $priorRunRoot = [IO.Path]::GetFullPath([string]$prior.run_root)
        Assert-LtaooNoReparsePoint -BasePath $runtimeBase -TargetPath $priorRunRoot
        $priorReceipt = Invoke-JournalCleanup -Journal $prior -RuntimeBase $runtimeBase -Backend $routerBackend
        $priorReceiptPath = Join-Path $priorRunRoot 'cleanup-receipt.input.json'
        Write-LtaooJsonAtomic -Value $priorReceipt -LiteralPath $priorReceiptPath
        [void](Invoke-JournalFinalize -Journal $prior -ReceiptPath $priorReceiptPath)
        if (-not [bool]$priorReceipt.safe -or -not (Test-JournalPublicationClosed -Journal $prior)) { exit 3 }
        Remove-Item -LiteralPath $journalPath -Force
    }

    $currentStage = 'resolve_runtime_files'
    $resolvedRequest = Resolve-RuntimeFile -LiteralPath $RequestPath
    $resolvedGrant = Resolve-RuntimeFile -LiteralPath $GrantPath
    Assert-LtaooNoReparsePoint -BasePath $resolvedRunRoot -TargetPath $resolvedRequest
    Assert-LtaooNoReparsePoint -BasePath $resolvedRunRoot -TargetPath $resolvedGrant
    $resolvedLtaoo = Resolve-RuntimeFile -LiteralPath $LtaooExePath
    $resolvedBatch = Resolve-RuntimeFile -LiteralPath $BatchExePath
    if ([IO.Path]::GetFileName($resolvedLtaoo) -ine 'wx_video_download.exe' -or [IO.Path]::GetFileName($resolvedBatch) -ine 'wx_channel_ltaoo_batch.exe') {
        throw 'runtime_executable_invalid'
    }

    $requestAllowedProperties = @('schema_version', 'run_id', 'keyword', 'content_urls', 'limits', 'output_root')
    $request = Read-LtaooStrictJson -LiteralPath $resolvedRequest -AllowedProperties $requestAllowedProperties
    $runId = [string]$request.run_id
    if ($runId -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$') { throw 'request_run_id_invalid' }

    $runtimePaths = @($resolvedLtaoo, $resolvedRouter, $resolvedRouterConfig, $resolvedBatch, $runtimeBase)
    if ($authorizationMode -ceq 'wechat-channels-local-runtime-v2') {
        [void](Use-LtaooRunGrant -GrantPath $resolvedGrant -RunId $runId -RequestPath $resolvedRequest -RuntimePaths $runtimePaths -LtaooExePath $resolvedLtaoo -BatchExePath $resolvedBatch -ExpectedAuthorizationMode $authorizationMode -RouterKind $effectiveRouterKind -RouterExePath $resolvedRouter -RouterConfigPath $resolvedRouterConfig -RouterCapabilityFingerprint $effectiveRouterCapabilityFingerprint)
    } else {
        [void](Use-LtaooRunGrant -GrantPath $resolvedGrant -RunId $runId -RequestPath $resolvedRequest -RuntimePaths $runtimePaths -LtaooExePath $resolvedLtaoo -BatchExePath $resolvedBatch)
    }
    $currentStage = 'grant_consumed'
    if (-not (Get-ListenerAbsent -Ports @($ApiPort, $ProxyPort))) { throw 'runtime_ports_in_use' }

    $backupPath = Join-Path $resolvedRunRoot 'router-baseline.bin'
    $baselineBytes = [IO.File]::ReadAllBytes($resolvedRouterConfig)
    [IO.File]::WriteAllBytes($backupPath, $baselineBytes)
    Set-LtaooOwnerOnlyAcl -LiteralPath $backupPath -Directory $false
    $baselineHash = Get-LtaooFileHash -LiteralPath $backupPath
    $baselineDecoded = ConvertFrom-LtaooUtf8Bytes -Bytes $baselineBytes
    $baselineText = $baselineDecoded.Text
    $controller = Get-LtaooRouterController -Backend $routerBackend -Text $baselineText
    $temporaryText = Add-LtaooRouterBlock -Backend $routerBackend -Text $baselineText -RunId $runId -ProxyPort $ProxyPort
    $candidatePath = Join-Path $resolvedRunRoot 'router-candidate.yaml'
    [IO.File]::WriteAllBytes($candidatePath, (ConvertTo-LtaooUtf8Bytes -Text $temporaryText -WithBom $baselineDecoded.HasBom))
    Set-LtaooOwnerOnlyAcl -LiteralPath $candidatePath -Directory $false
    $currentStage = 'candidate_validation'
    Test-LtaooRouterConfig -Backend $routerBackend -ConfigPath $candidatePath -DataDirectory (Split-Path -Parent $resolvedRouterConfig)
    $currentStage = 'candidate_validated'
    $temporaryHash = Get-LtaooFileHash -LiteralPath $candidatePath

    $secretsRoot = Join-Path $resolvedRunRoot 'secrets'
    $journal = [ordered]@{
        schema_version = 2; run_id = $runId; run_root = $resolvedRunRoot; phase = 'authorized'; created_at = [DateTimeOffset]::UtcNow.ToString('o')
        ca_thumbprint = ''; ltaoo_pid = 0; ltaoo_start_time = ''; ltaoo_path = $resolvedLtaoo; ltaoo_sha256 = Get-LtaooFileHash -LiteralPath $resolvedLtaoo
        router_kind = $effectiveRouterKind; router_executable_path = $resolvedRouter; router_executable_sha256 = Get-LtaooFileHash -LiteralPath $resolvedRouter; router_config_path = $resolvedRouterConfig
        router_baseline_sha256 = $baselineHash; router_temporary_sha256 = $temporaryHash; router_backup_path = $backupPath; router_capability_fingerprint = $effectiveRouterCapabilityFingerprint
        secrets_root = $secretsRoot; api_port = $ApiPort; proxy_port = $ProxyPort
        request_path = $resolvedRequest; batch_path = $resolvedBatch; batch_sha256 = Get-LtaooFileHash -LiteralPath $resolvedBatch
    }
    Write-LtaooJsonAtomic -Value $journal -LiteralPath $journalPath
    $currentStage = 'journal_written'

    $certificate = New-LtaooRunCertificate -RunId $runId -SecretsRoot $secretsRoot
    $currentStage = 'certificate_created'
    $journal.ca_thumbprint = $certificate.Thumbprint
    $journal.phase = 'certificate_created'
    Write-LtaooJsonAtomic -Value $journal -LiteralPath $journalPath
    Install-LtaooCurrentUserCA -CertificatePath $certificate.CertificatePath -Thumbprint $certificate.Thumbprint
    $currentStage = 'certificate_installed'
    $journal.phase = 'certificate_installed'
    Write-LtaooJsonAtomic -Value $journal -LiteralPath $journalPath

    Write-LtaooBytesAtomic -Bytes ([IO.File]::ReadAllBytes($candidatePath)) -LiteralPath $resolvedRouterConfig
    Invoke-LtaooRouterReload -Backend $routerBackend -Controller $controller -ConfigPath $resolvedRouterConfig
    $currentStage = 'router_reloaded'
    $journal.phase = 'router_reloaded'
    Write-LtaooJsonAtomic -Value $journal -LiteralPath $journalPath

    $certYaml = $certificate.CertificatePath.Replace('\', '/')
    $keyYaml = $certificate.PrivateKeyPath.Replace('\', '/')
    $ltaooConfigPath = Join-Path $resolvedRunRoot 'ltaoo-runtime.yaml'
    $ltaooYaml = @"
api:
  protocol: http
  hostname: 127.0.0.1
  port: $ApiPort
proxy:
  enabled: true
  system: false
  hostname: 127.0.0.1
  port: $ProxyPort
  tun: false
  skipInstallRootCert: true
cert:
  file: "$certYaml"
  key: "$keyYaml"
  name: "TrendRadarWX-$runId"
"@
    [IO.File]::WriteAllText($ltaooConfigPath, $ltaooYaml, [Text.UTF8Encoding]::new($false))
    Set-LtaooOwnerOnlyAcl -LiteralPath $ltaooConfigPath -Directory $false
    foreach ($runtimeArgumentPath in @($ltaooConfigPath, $resolvedRequest, $resolvedRunRoot, $receiptPath)) {
        if ($null -ne $runtimeArgumentPath -and $runtimeArgumentPath.Contains('"')) { throw 'unsupported_path_character' }
    }
    $ltaooArguments = '-c "' + $ltaooConfigPath + '"'
    $ltaooProcess = Start-Process -FilePath $resolvedLtaoo -ArgumentList $ltaooArguments -WorkingDirectory $resolvedRunRoot -PassThru -WindowStyle Hidden
    $journal.ltaoo_pid = $ltaooProcess.Id
    $journal.ltaoo_start_time = $ltaooProcess.StartTime.ToUniversalTime().ToString('o')
    $journal.phase = 'ltaoo_started'
    $currentStage = 'ltaoo_started'
    Write-LtaooJsonAtomic -Value $journal -LiteralPath $journalPath

    $apiBase = 'http://127.0.0.1:' + $ApiPort
    $ready = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        if ($ltaooProcess.HasExited) { break }
        try {
            $status = Invoke-WebRequest -UseBasicParsing -Uri ($apiBase + '/api/status') -TimeoutSec 2
            if ([int]$status.StatusCode -eq 200) { $ready = $true; break }
        } catch { }
        Start-Sleep -Milliseconds 500
    }
    if (-not $ready) { throw 'ltaoo_readiness_failed' }
    $currentStage = 'ltaoo_ready'

    if ($AutoRefreshWechatPage) {
        $currentStage = 'page_refresh'
        $refreshScript = Join-Path $PSScriptRoot 'Invoke-WeChatPageRefresh.ps1'
        if (-not [IO.File]::Exists($refreshScript)) { throw 'page_refresh_helper_missing' }
        $refreshCode = @(& $refreshScript)
        if ($refreshCode.Count -ne 1 -or [string]$refreshCode[0] -cne 'wechat_page_refresh_sent') {
            throw 'wechat_page_refresh_failed'
        }
        Start-Sleep -Milliseconds 500
    }

    $journal.phase = 'collecting'
    $currentStage = 'collecting'
    Write-LtaooJsonAtomic -Value $journal -LiteralPath $journalPath
    $collectArguments = 'collect --request "' + $resolvedRequest + '" --run-root "' + $resolvedRunRoot + '" --api-base "' + $apiBase + '"'
    $batchProcess = Start-Process -FilePath $resolvedBatch -ArgumentList $collectArguments -WorkingDirectory $resolvedRunRoot -PassThru -Wait -WindowStyle Hidden
    $draftExists = [IO.Directory]::Exists((Join-Path $resolvedRunRoot ('batch.partial-' + $runId)))
    if ($batchProcess.ExitCode -notin @(0, 2, 4)) { throw 'batch_collect_failed' }
    $finalExitCode = [int]$batchProcess.ExitCode
    $journal.phase = 'collection_closed'
    $currentStage = 'collection_closed'
    Write-LtaooJsonAtomic -Value $journal -LiteralPath $journalPath
} catch {
    $safeFailureCode = [string]$_.Exception.Message
    if ($safeFailureCode -notmatch '^[a-z][a-z0-9_]{2,80}$') { $safeFailureCode = 'runtime_failed_' + $currentStage }
    [Console]::Error.WriteLine($safeFailureCode)
    $finalExitCode = 4
} finally {
    if ($null -ne $journal -and $null -ne $runtimeBase -and $null -ne $routerBackend) {
        try {
            $journal.phase = 'cleaning'
            Write-LtaooJsonAtomic -Value $journal -LiteralPath $journalPath
            $receipt = Invoke-JournalCleanup -Journal ([pscustomobject]$journal) -RuntimeBase $runtimeBase -Backend $routerBackend
            $receiptPath = Join-Path $resolvedRunRoot 'cleanup-receipt.input.json'
            Write-LtaooJsonAtomic -Value $receipt -LiteralPath $receiptPath
            if (-not [bool]$receipt.safe) { $finalExitCode = 3 }
            if ($draftExists -and [IO.File]::Exists((Join-Path $resolvedRunRoot 'collection-result.json'))) {
                $finalExitCode = [int](Invoke-JournalFinalize -Journal ([pscustomobject]$journal) -ReceiptPath $receiptPath)
            }
            $publicationClosed = Test-JournalPublicationClosed -Journal ([pscustomobject]$journal)
            if ([bool]$receipt.safe -and $publicationClosed -and [IO.File]::Exists($journalPath)) {
                Remove-Item -LiteralPath $journalPath -Force
            } elseif (-not $publicationClosed) {
                $finalExitCode = 3
            }
        } catch { $finalExitCode = 3 }
    } elseif ($null -ne $resolvedRunRoot) {
        foreach ($name in @('router-baseline.bin', 'router-candidate.yaml', 'router-recovery-candidate.yaml', 'clash-baseline.bin', 'clash-candidate.yaml', 'clash-recovery-candidate.yaml', 'ltaoo-runtime.yaml')) {
            $target = Join-Path $resolvedRunRoot $name
            if ([IO.File]::Exists($target)) { Remove-Item -LiteralPath $target -Force }
        }
    }
    if ($mutexAcquired -and $null -ne $mutex) { try { $mutex.ReleaseMutex() } catch { } }
    if ($null -ne $mutex) { $mutex.Dispose() }
}

exit $finalExitCode
