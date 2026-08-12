# LTAOO_PROBE_CLEANUP_READY=1
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9-]{1,80}$')][string]$RunId,
    [string]$RepoRoot = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-ProbeRepoRoot {
    param([string]$Value)
    if ([string]::IsNullOrWhiteSpace($Value)) {
        return (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
    }
    return (Resolve-Path -LiteralPath $Value).Path
}

function Resolve-RunRoot {
    param([string]$ResolvedRepoRoot, [string]$Value)
    $base = [IO.Path]::GetFullPath((Join-Path $ResolvedRepoRoot ".tmp_runtime\ltaoo-probe"))
    $candidate = [IO.Path]::GetFullPath((Join-Path $base $Value))
    if (-not $candidate.StartsWith($base + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "invalid_run_root"
    }
    return [pscustomobject]@{ base = $base; run = $candidate }
}

function Assert-NoReparsePoint {
    param([string]$BasePath, [string]$TargetPath)
    $base = [IO.Path]::GetFullPath($BasePath)
    $target = [IO.Path]::GetFullPath($TargetPath)
    if ($target -ne $base -and -not $target.StartsWith($base + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "target_outside_run_root"
    }
    $cursor = $base
    while ($true) {
        if ([IO.Directory]::Exists($cursor) -or [IO.File]::Exists($cursor)) {
            $item = Get-Item -Force -LiteralPath $cursor
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "reparse_point_rejected" }
        }
        if ($cursor -eq $target) { break }
        $relative = $target.Substring($cursor.Length).TrimStart([IO.Path]::DirectorySeparatorChar)
        $nextSegment = $relative.Split([IO.Path]::DirectorySeparatorChar)[0]
        $cursor = Join-Path $cursor $nextSegment
    }
}

function Assert-RunOwnedPath {
    param([string]$RunRoot, [string]$Value)
    if ([string]::IsNullOrWhiteSpace($Value)) { throw "manifest_path_invalid" }
    $candidate = [IO.Path]::GetFullPath($Value)
    if ($candidate -eq $RunRoot -or -not $candidate.StartsWith($RunRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "target_outside_run_root"
    }
    Assert-NoReparsePoint $RunRoot $candidate
    return $candidate
}

function Get-Sha256Hex {
    param([byte[]]$Bytes)
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return (($sha.ComputeHash($Bytes) | ForEach-Object { $_.ToString("x2") }) -join "") }
    finally { $sha.Dispose() }
}

function Get-CanonicalHash {
    param([Parameter()][AllowNull()][AllowEmptyCollection()][object]$Value)
    $json = $Value | ConvertTo-Json -Compress -Depth 8
    return Get-Sha256Hex ([Text.Encoding]::UTF8.GetBytes($json))
}

function Write-JsonAtomic {
    param([object]$Value, [string]$Path)
    $temporary = $Path + "." + [Guid]::NewGuid().ToString("N") + ".tmp"
    $backup = $temporary + ".bak"
    [IO.File]::WriteAllText($temporary, ($Value | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
    try {
        if ([IO.File]::Exists($Path)) { [IO.File]::Replace($temporary, $Path, $backup) }
        else { [IO.File]::Move($temporary, $Path) }
    } finally {
        if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
        if ([IO.File]::Exists($backup)) { [IO.File]::Delete($backup) }
    }
}

function Get-ProbeBaseline {
    param([int]$SelectedApiPort, [int]$SelectedProxyPort)
    $script:currentStage = "drift_user_proxy"
    $internetSettings = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Windows\CurrentVersion\Internet Settings')
    try {
        $userProxy = [ordered]@{
            proxy_enable = $internetSettings.GetValue('ProxyEnable', $null)
            proxy_server = $internetSettings.GetValue('ProxyServer', $null)
            proxy_override = $internetSettings.GetValue('ProxyOverride', $null)
            auto_config_url = $internetSettings.GetValue('AutoConfigURL', $null)
        }
    } finally {
        if ($null -ne $internetSettings) { $internetSettings.Dispose() }
    }
    $script:currentStage = "drift_winhttp"
    $winHttp = (& netsh.exe winhttp show proxy 2>&1 | Out-String)
    $script:currentStage = "drift_routes"
    $routes = @(Get-NetRoute | Sort-Object AddressFamily, DestinationPrefix, NextHop, RouteMetric, InterfaceIndex | Select-Object AddressFamily, DestinationPrefix, NextHop, RouteMetric, InterfaceIndex)
    $script:currentStage = "drift_roots"
    $currentRoots = @(Get-ChildItem Cert:\CurrentUser\Root | Select-Object -ExpandProperty Thumbprint | Sort-Object)
    $machineRoots = @(Get-ChildItem Cert:\LocalMachine\Root | Select-Object -ExpandProperty Thumbprint | Sort-Object)
    $script:currentStage = "drift_listeners"
    $listeners = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -in @($SelectedApiPort, $SelectedProxyPort) } | Sort-Object LocalPort, OwningProcess | Select-Object LocalAddress, LocalPort, OwningProcess)
    $script:currentStage = "drift_processes"
    $processes = [Collections.Generic.List[object]]::new()
    foreach ($process in @(Get-Process | Where-Object { $_.ProcessName -match 'clash|wechat|wx_video_download' } | Sort-Object ProcessName, Id)) {
        $started = ""
        try { $started = $process.StartTime.ToUniversalTime().ToString("o") } catch { $started = "unavailable" }
        [void]$processes.Add([ordered]@{ name = $process.ProcessName; id = $process.Id; started = $started })
    }
    $script:currentStage = "drift_hash_user_proxy"
    $userProxyHash = Get-CanonicalHash $userProxy
    $script:currentStage = "drift_hash_winhttp"
    $winHttpHash = Get-CanonicalHash $winHttp
    $script:currentStage = "drift_hash_routes"
    $routeHash = Get-CanonicalHash -Value (, $routes)
    $script:currentStage = "drift_hash_current_roots"
    $currentRootsHash = Get-CanonicalHash -Value (, $currentRoots)
    $script:currentStage = "drift_hash_machine_roots"
    $machineRootsHash = Get-CanonicalHash -Value (, $machineRoots)
    $script:currentStage = "drift_hash_listeners"
    $listenersHash = Get-CanonicalHash -Value (, $listeners)
    $script:currentStage = "drift_hash_processes"
    $processHash = Get-CanonicalHash -Value (, @($processes))
    return [ordered]@{
        schema_version = 1
        captured_at = [DateTimeOffset]::UtcNow.ToString("o")
        user_proxy_sha256 = $userProxyHash
        winhttp_proxy_sha256 = $winHttpHash
        route_table_sha256 = $routeHash
        current_user_roots_sha256 = $currentRootsHash
        local_machine_roots_sha256 = $machineRootsHash
        probe_listeners_sha256 = $listenersHash
        related_processes_sha256 = $processHash
    }
}

$runRoot = $null
$receiptPath = $null
$warnings = [Collections.Generic.List[string]]::new()
$currentStage = "startup"

try {
    $repoRootValue = Resolve-ProbeRepoRoot $RepoRoot
    $resolved = Resolve-RunRoot $repoRootValue $RunId
    if (-not [IO.Directory]::Exists($resolved.base) -or -not [IO.Directory]::Exists($resolved.run)) { throw "run_not_found" }
    Assert-NoReparsePoint $resolved.base $resolved.run
    $runRoot = $resolved.run
    $receiptPath = Join-Path $runRoot "cleanup-receipt.json"
    $manifestPath = Join-Path $runRoot "manifest.json"
    if (-not [IO.File]::Exists($manifestPath)) { throw "manifest_not_found" }
    try { $manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json } catch { throw "manifest_invalid" }
    if ([int]$manifest.schema_version -ne 1 -or [string]$manifest.run_id -cne $RunId) { throw "manifest_invalid" }
    if (-not [string]::Equals([IO.Path]::GetFullPath([string]$manifest.runtime_root), $runRoot, [StringComparison]::OrdinalIgnoreCase)) {
        throw "manifest_invalid"
    }

    $currentStage = "ownership_validation"
    $certificateFile = Assert-RunOwnedPath $runRoot ([string]$manifest.ca.certificate_file)
    $privateKeyFile = Assert-RunOwnedPath $runRoot ([string]$manifest.ca.private_key_file)
    $configFile = Assert-RunOwnedPath $runRoot ([string]$manifest.ltaoo.config_file)
    $secretsRoot = Assert-RunOwnedPath $runRoot (Join-Path $runRoot "secrets")
    if ([string]$manifest.ca.store -cne "CurrentUser\Root") { throw "certificate_store_invalid" }
    $thumbprint = ([string]$manifest.ca.thumbprint).Replace(" ", "").ToUpperInvariant()
    if ($thumbprint -ne "" -and $thumbprint -notmatch '^[0-9A-F]{40,128}$') { throw "thumbprint_invalid" }
    $pidValue = [int]$manifest.ltaoo.pid
    if ($pidValue -gt 0) {
        if ([string]::IsNullOrWhiteSpace([string]$manifest.ltaoo.process_start_time) -or [string]$manifest.ltaoo.executable_sha256 -notmatch '^[0-9a-fA-F]{64}$') {
            throw "process_identity_invalid"
        }
    }

    $currentStage = "process"
    $processStopped = $true
    if ($pidValue -gt 0) {
        $process = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
        if ($null -ne $process) {
            $identityMatches = $false
            try {
                $startMatches = $process.StartTime.ToUniversalTime().ToString("o") -eq [string]$manifest.ltaoo.process_start_time
                $imageHash = (Get-FileHash -LiteralPath $process.Path -Algorithm SHA256).Hash.ToLowerInvariant()
                $hashMatches = $imageHash -eq ([string]$manifest.ltaoo.executable_sha256).ToLowerInvariant()
                $identityMatches = $startMatches -and $hashMatches
            } catch { $identityMatches = $false }
            if ($identityMatches) {
                Stop-Process -Id $process.Id -Force
                Wait-Process -Id $process.Id -Timeout 10 -ErrorAction SilentlyContinue
                $processStopped = $null -eq (Get-Process -Id $process.Id -ErrorAction SilentlyContinue)
            } else {
                $warnings.Add("ltaoo_process_identity_mismatch")
                $processStopped = $false
            }
        }
    }

    $currentStage = "certificate"
    $caAbsent = $true
    if ($thumbprint -ne "") {
        $certStorePath = "Cert:\CurrentUser\Root\" + $thumbprint
        if (Test-Path -LiteralPath $certStorePath) {
            & certutil.exe -user -delstore Root $thumbprint *> $null
            if ($LASTEXITCODE -ne 0) { throw "ca_removal_failed" }
        }
        $caAbsent = -not (Test-Path -LiteralPath $certStorePath)
        if (-not $caAbsent) { throw "ca_removal_failed" }
    }

    $currentStage = "files"
    foreach ($target in @($privateKeyFile, $certificateFile, $configFile)) {
        if ([IO.File]::Exists($target)) { Remove-Item -LiteralPath $target -Force }
    }
    if ([IO.Directory]::Exists($secretsRoot)) {
        if (@(Get-ChildItem -Force -LiteralPath $secretsRoot).Count -eq 0) { Remove-Item -LiteralPath $secretsRoot -Force }
        else { $warnings.Add("secrets_directory_not_empty") }
    }
    $privateKeyAbsent = -not [IO.File]::Exists($privateKeyFile)
    $certificateFileAbsent = -not [IO.File]::Exists($certificateFile)
    $configAbsent = -not [IO.File]::Exists($configFile)

    $currentStage = "drift"
    $baselinePath = Join-Path $runRoot "baseline.json"
    if ([IO.File]::Exists($baselinePath)) {
        $currentStage = "drift_baseline"
        try { $baseline = Get-Content -Raw -LiteralPath $baselinePath | ConvertFrom-Json } catch { throw "baseline_invalid" }
        $currentStage = "drift_ports"
        $apiPort = ([Uri][string]$manifest.ltaoo.api_base).Port
        $proxyPort = [int](([string]$manifest.ltaoo.proxy_endpoint).Split(':')[-1])
        $currentStage = "drift_capture"
        $after = Get-ProbeBaseline $apiPort $proxyPort
        $currentStage = "drift_compare"
        foreach ($field in @(
            "user_proxy_sha256", "winhttp_proxy_sha256", "route_table_sha256", "current_user_roots_sha256",
            "local_machine_roots_sha256", "probe_listeners_sha256", "related_processes_sha256"
        )) {
            $currentStage = "drift_compare_" + $field.Replace("_sha256", "")
            if ([string]$baseline.$field -ne [string]$after.$field) { $warnings.Add($field.Replace("_sha256", "")) }
        }
    } else {
        $warnings.Add("baseline_missing")
    }

    $currentStage = "receipt_build"
    $cleanupSuccess = $processStopped -and $caAbsent -and $privateKeyAbsent -and $certificateFileAbsent -and $configAbsent
    $receipt = [ordered]@{
        schema_version = 1
        run_id = $RunId
        process_stopped = $processStopped
        ca_absent = $caAbsent
        private_key_absent = $privateKeyAbsent
        certificate_file_absent = $certificateFileAbsent
        config_absent = $configAbsent
        external_drift_warnings = @($warnings | Select-Object -Unique)
        cleanup_success = $cleanupSuccess
        completed_at = [DateTimeOffset]::UtcNow.ToString("o")
    }
    $currentStage = "receipt_write"
    Write-JsonAtomic $receipt $receiptPath
    $currentStage = "receipt_output"
    [pscustomobject]@{ run_id = $RunId; cleanup_success = $cleanupSuccess; receipt_file = $receiptPath } | ConvertTo-Json -Compress
    if (-not $cleanupSuccess) { exit 1 }
} catch {
    $known = @(
        "invalid_run_root", "run_not_found", "reparse_point_rejected", "manifest_not_found", "manifest_invalid",
        "manifest_path_invalid", "target_outside_run_root", "certificate_store_invalid", "thumbprint_invalid",
        "process_identity_invalid", "ca_removal_failed", "baseline_invalid"
    )
    $reasonCode = if ($_.Exception.Message -in $known) { $_.Exception.Message } else { "unexpected_" + $currentStage }
    [pscustomobject]@{ run_id = $RunId; cleanup_success = $false; reason_code = $reasonCode } | ConvertTo-Json -Compress
    exit 1
}
