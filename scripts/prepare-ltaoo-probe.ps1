[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$LtaooExePath,
    [string]$RepoRoot = "",
    [int]$ApiPort = 2022,
    [int]$ProxyPort = 2023
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

function Get-Sha256Hex {
    param([byte[]]$Bytes)
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return (($sha.ComputeHash($Bytes) | ForEach-Object { $_.ToString("x2") }) -join "") }
    finally { $sha.Dispose() }
}

function Get-CanonicalHash {
    param([object]$Value)
    $json = $Value | ConvertTo-Json -Compress -Depth 8
    return Get-Sha256Hex ([Text.Encoding]::UTF8.GetBytes($json))
}

function Write-JsonAtomic {
    param([object]$Value, [string]$Path)
    $temporary = $Path + "." + [Guid]::NewGuid().ToString("N") + ".tmp"
    [IO.File]::WriteAllText($temporary, ($Value | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
    try {
        if ([IO.File]::Exists($Path)) { [IO.File]::Replace($temporary, $Path, $null) }
        else { [IO.File]::Move($temporary, $Path) }
    } finally {
        if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
    }
}

function Assert-NoReparsePoint {
    param([string]$BasePath, [string]$TargetPath)
    $base = [IO.Path]::GetFullPath($BasePath)
    $target = [IO.Path]::GetFullPath($TargetPath)
    if ($target -ne $base -and -not $target.StartsWith($base + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "path_outside_probe_root"
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

function Get-ProbeBaseline {
    param([int]$SelectedApiPort, [int]$SelectedProxyPort)
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
    $winHttp = (& netsh.exe winhttp show proxy 2>&1 | Out-String)
    $routes = @(Get-NetRoute | Sort-Object AddressFamily, DestinationPrefix, NextHop, RouteMetric, InterfaceIndex | Select-Object AddressFamily, DestinationPrefix, NextHop, RouteMetric, InterfaceIndex)
    $currentRoots = @(Get-ChildItem Cert:\CurrentUser\Root | Select-Object -ExpandProperty Thumbprint | Sort-Object)
    $machineRoots = @(Get-ChildItem Cert:\LocalMachine\Root | Select-Object -ExpandProperty Thumbprint | Sort-Object)
    $listeners = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -in @($SelectedApiPort, $SelectedProxyPort) } | Sort-Object LocalPort, OwningProcess | Select-Object LocalAddress, LocalPort, OwningProcess)
    $processes = [Collections.Generic.List[object]]::new()
    foreach ($process in @(Get-Process | Where-Object { $_.ProcessName -match 'clash|wechat|wx_video_download' } | Sort-Object ProcessName, Id)) {
        $started = ""
        try { $started = $process.StartTime.ToUniversalTime().ToString("o") } catch { $started = "unavailable" }
        $processes.Add([ordered]@{ name = $process.ProcessName; id = $process.Id; started = $started })
    }
    return [ordered]@{
        schema_version = 1
        captured_at = [DateTimeOffset]::UtcNow.ToString("o")
        user_proxy_sha256 = Get-CanonicalHash $userProxy
        winhttp_proxy_sha256 = Get-CanonicalHash $winHttp
        route_table_sha256 = Get-CanonicalHash $routes
        current_user_roots_sha256 = Get-CanonicalHash $currentRoots
        local_machine_roots_sha256 = Get-CanonicalHash $machineRoots
        probe_listeners_sha256 = Get-CanonicalHash $listeners
        related_processes_sha256 = Get-CanonicalHash @($processes)
        api_port_in_use = [bool](@($listeners | Where-Object { $_.LocalPort -eq $SelectedApiPort }).Count)
        proxy_port_in_use = [bool](@($listeners | Where-Object { $_.LocalPort -eq $SelectedProxyPort }).Count)
    }
}

function ConvertTo-Pem {
    param([string]$Label, [byte[]]$Bytes)
    $base64 = [Convert]::ToBase64String($Bytes)
    $builder = [Text.StringBuilder]::new()
    [void]$builder.AppendLine("-----BEGIN $Label-----")
    for ($offset = 0; $offset -lt $base64.Length; $offset += 64) {
        [void]$builder.AppendLine($base64.Substring($offset, [Math]::Min(64, $base64.Length - $offset)))
    }
    [void]$builder.AppendLine("-----END $Label-----")
    return $builder.ToString()
}

function Set-OwnerOnlyAcl {
    param([string]$Path, [bool]$Directory)
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
    if ($Directory) {
        $security = [Security.AccessControl.DirectorySecurity]::new()
        $inheritance = [Security.AccessControl.InheritanceFlags]::ContainerInherit -bor [Security.AccessControl.InheritanceFlags]::ObjectInherit
    } else {
        $security = [Security.AccessControl.FileSecurity]::new()
        $inheritance = [Security.AccessControl.InheritanceFlags]::None
    }
    $security.SetOwner($sid)
    $security.SetAccessRuleProtection($true, $false)
    $rule = [Security.AccessControl.FileSystemAccessRule]::new(
        $sid,
        [Security.AccessControl.FileSystemRights]::FullControl,
        $inheritance,
        [Security.AccessControl.PropagationFlags]::None,
        [Security.AccessControl.AccessControlType]::Allow
    )
    [void]$security.AddAccessRule($rule)
    Set-Acl -LiteralPath $Path -AclObject $security
    $applied = Get-Acl -LiteralPath $Path
    foreach ($accessRule in $applied.Access) {
        if (-not $accessRule.IsInherited -and $accessRule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -ne $sid.Value) {
            throw "private_acl_not_exclusive"
        }
    }
    if (-not $applied.AreAccessRulesProtected) { throw "private_acl_not_exclusive" }
}

function New-RunSuffix {
    $bytes = New-Object byte[] 6
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    return (($bytes | ForEach-Object { $_.ToString("x2") }) -join "")
}

$runId = ([DateTime]::UtcNow.ToString("yyyyMMdd-HHmmss") + "-" + (New-RunSuffix))
$repoRootValue = $null
$runRoot = $null
$manifestPath = $null
$currentStage = "startup"

try {
    if ($ApiPort -lt 1 -or $ApiPort -gt 65535 -or $ProxyPort -lt 1 -or $ProxyPort -gt 65535 -or $ApiPort -eq $ProxyPort) {
        throw "invalid_ports"
    }
    $repoRootValue = Resolve-ProbeRepoRoot $RepoRoot
    $resolvedExe = (Resolve-Path -LiteralPath $LtaooExePath).Path
    $leaf = [IO.Path]::GetFileName($resolvedExe)
    if ($leaf -match '^(?i)wx_channel.*\.exe$') { throw "nobiyou_executable_rejected" }
    if (-not [IO.File]::Exists($resolvedExe) -or [IO.Path]::GetExtension($resolvedExe) -ine ".exe") { throw "ltaoo_executable_required" }

    $probeBase = [IO.Path]::GetFullPath((Join-Path $repoRootValue ".tmp_runtime\ltaoo-probe"))
    if (-not [IO.Directory]::Exists($probeBase)) { [void][IO.Directory]::CreateDirectory($probeBase) }
    Assert-NoReparsePoint $probeBase $probeBase
    $runRoot = [IO.Path]::GetFullPath((Join-Path $probeBase $runId))
    Assert-NoReparsePoint $probeBase $runRoot
    [void][IO.Directory]::CreateDirectory($runRoot)
    $manifestPath = Join-Path $runRoot "manifest.json"

    $currentStage = "baseline"
    $baseline = Get-ProbeBaseline $ApiPort $ProxyPort
    Write-JsonAtomic $baseline (Join-Path $runRoot "baseline.json")
    if ($baseline.api_port_in_use -or $baseline.proxy_port_in_use) { throw "ports_already_in_use" }

    $cleanupScript = Join-Path $PSScriptRoot "cleanup-ltaoo-probe.ps1"
    if (-not [IO.File]::Exists($cleanupScript) -or -not ((Get-Content -Raw -LiteralPath $cleanupScript).Contains("# LTAOO_PROBE_CLEANUP_READY=1"))) {
        throw "cleanup_not_implemented"
    }

    $currentStage = "certificate"
    $secretsRoot = Join-Path $runRoot "secrets"
    [void][IO.Directory]::CreateDirectory($secretsRoot)
    Set-OwnerOnlyAcl $secretsRoot $true
    $certPath = Join-Path $secretsRoot "ca-cert.pem"
    $keyPath = Join-Path $secretsRoot "ca-key.pem"
    $subject = "CN=WXChannelsPOC-$runId"
    $rsa = [Security.Cryptography.RSA]::Create(2048)
    try {
        $request = [Security.Cryptography.X509Certificates.CertificateRequest]::new(
            $subject, $rsa, [Security.Cryptography.HashAlgorithmName]::SHA256,
            [Security.Cryptography.RSASignaturePadding]::Pkcs1
        )
        $request.CertificateExtensions.Add([Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]::new($true, $false, 0, $true))
        $keyUsage = [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::KeyCertSign -bor [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::CrlSign
        $request.CertificateExtensions.Add([Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new($keyUsage, $true))
        $request.CertificateExtensions.Add([Security.Cryptography.X509Certificates.X509SubjectKeyIdentifierExtension]::new($request.PublicKey, $false))
        $certificate = $request.CreateSelfSigned([DateTimeOffset]::UtcNow.AddMinutes(-5), [DateTimeOffset]::UtcNow.AddDays(2))
        try {
            $certDer = $certificate.Export([Security.Cryptography.X509Certificates.X509ContentType]::Cert)
            $keyDer = $rsa.Key.Export([Security.Cryptography.CngKeyBlobFormat]::Pkcs8PrivateBlob)
            [IO.File]::WriteAllText($certPath, (ConvertTo-Pem "CERTIFICATE" $certDer), [Text.UTF8Encoding]::new($false))
            [IO.File]::WriteAllText($keyPath, (ConvertTo-Pem "PRIVATE KEY" $keyDer), [Text.UTF8Encoding]::new($false))
            Set-OwnerOnlyAcl $keyPath $false
            $thumbprint = $certificate.Thumbprint.ToUpperInvariant()
        } finally {
            $certificate.Dispose()
        }
    } finally {
        $rsa.Dispose()
    }

    $currentStage = "configuration"
    foreach ($pathValue in @($certPath, $keyPath)) {
        if ($pathValue.Contains('"')) { throw "unsupported_path_character" }
    }
    $certYaml = $certPath.Replace('\', '/')
    $keyYaml = $keyPath.Replace('\', '/')
    $configPath = Join-Path $runRoot "ltaoo-probe.yaml"
    $yaml = @"
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
  keyFile: "$keyYaml"
  name: "WXChannelsPOC-$runId"
"@
    [IO.File]::WriteAllText($configPath, $yaml, [Text.UTF8Encoding]::new($false))

    $repoCommit = "unknown"
    try { $repoCommit = (& git -C $repoRootValue rev-parse HEAD 2>$null | Select-Object -First 1).Trim() } catch { $repoCommit = "unknown" }
    $exeHash = (Get-FileHash -LiteralPath $resolvedExe -Algorithm SHA256).Hash.ToLowerInvariant()
    $manifest = [ordered]@{
        schema_version = 1
        run_id = $runId
        created_at = [DateTimeOffset]::UtcNow.ToString("o")
        repo_commit = $repoCommit
        runtime_root = $runRoot
        ca = [ordered]@{
            store = "CurrentUser\Root"
            thumbprint = $thumbprint
            subject = $subject
            certificate_file = $certPath
            private_key_file = $keyPath
        }
        ltaoo = [ordered]@{
            executable_sha256 = $exeHash
            pid = 0
            process_start_time = ""
            config_file = $configPath
            api_base = ("http://127.0.0.1:{0}" -f $ApiPort)
            proxy_endpoint = ("127.0.0.1:{0}" -f $ProxyPort)
        }
    }
    Write-JsonAtomic $manifest $manifestPath

    $currentStage = "confirmation"
    Write-Host ("run_id: " + $runId)
    Write-Host ("CA thumbprint: " + $thumbprint)
    Write-Host ("API: http://127.0.0.1:{0}" -f $ApiPort)
    Write-Host ("Proxy: 127.0.0.1:{0}" -f $ProxyPort)
    Write-Host "Clash, system proxy, WinHTTP proxy, and routes will not be modified."
    $expected = "INSTALL $runId"
    $actual = Read-Host "Type '$expected' to install the CurrentUser test CA and start ltaoo"
    if ($actual -cne $expected) { throw "confirmation_rejected" }

    $currentStage = "certificate_install"
    & certutil.exe -user -addstore Root $certPath *> $null
    if ($LASTEXITCODE -ne 0) { throw "ca_install_failed" }
    $certStorePath = "Cert:\CurrentUser\Root\" + $thumbprint
    if (-not (Test-Path -LiteralPath $certStorePath)) { throw "ca_install_failed" }

    $currentStage = "process_start"
    $argumentLine = '-c "' + $configPath + '"'
    $process = Start-Process -FilePath $resolvedExe -ArgumentList $argumentLine -WorkingDirectory $runRoot -PassThru -WindowStyle Hidden
    Start-Sleep -Milliseconds 500
    if ($process.HasExited) { throw "ltaoo_exited_early" }
    $manifest.ltaoo.pid = $process.Id
    $manifest.ltaoo.process_start_time = $process.StartTime.ToUniversalTime().ToString("o")
    Write-JsonAtomic $manifest $manifestPath

    [pscustomobject]@{
        run_id = $runId
        manifest_file = $manifestPath
        api_base = $manifest.ltaoo.api_base
        proxy_endpoint = $manifest.ltaoo.proxy_endpoint
    } | ConvertTo-Json -Compress
} catch {
    $known = @(
        "invalid_ports", "nobiyou_executable_rejected", "ltaoo_executable_required", "path_outside_probe_root",
        "reparse_point_rejected", "ports_already_in_use", "cleanup_not_implemented", "unsupported_path_character",
        "private_acl_not_exclusive", "confirmation_rejected", "ca_install_failed", "ltaoo_exited_early"
    )
    $reasonCode = if ($_.Exception.Message -in $known) { $_.Exception.Message } else { "unexpected_" + $currentStage }
    if ($null -ne $manifestPath -and [IO.File]::Exists($manifestPath)) {
        try {
            $cleanupText = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot "cleanup-ltaoo-probe.ps1")
            if ($cleanupText.Contains("# LTAOO_PROBE_CLEANUP_READY=1")) {
                & (Join-Path $PSScriptRoot "cleanup-ltaoo-probe.ps1") -RunId $runId -RepoRoot $repoRootValue | Out-Null
            }
        } catch { }
    }
    [pscustomobject]@{ run_id = $runId; status = "failed"; reason_code = $reasonCode } | ConvertTo-Json -Compress
    exit 1
}
