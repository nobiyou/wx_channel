Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-LtaooSha256Hex {
    param([Parameter(Mandatory = $true)][byte[]]$Bytes)
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return (($sha.ComputeHash($Bytes) | ForEach-Object { $_.ToString('x2') }) -join '') }
    finally { $sha.Dispose() }
}

function Get-LtaooFileHash {
    param([Parameter(Mandatory = $true)][string]$LiteralPath)
    return (Get-FileHash -LiteralPath $LiteralPath -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Get-LtaooStringHash {
    param([Parameter(Mandatory = $true)][string]$Value)
    $bytes = [Text.UTF8Encoding]::new($false).GetBytes($Value)
    try { return Get-LtaooSha256Hex -Bytes $bytes }
    finally { [Array]::Clear($bytes, 0, $bytes.Length) }
}

function Get-LtaooRuntimePathsHash {
    param([Parameter(Mandatory = $true)][string[]]$LiteralPaths)
    $canonical = foreach ($path in $LiteralPaths) {
        (Resolve-Path -LiteralPath $path).Path.ToLowerInvariant()
    }
    return Get-LtaooSha256Hex ([Text.Encoding]::UTF8.GetBytes(($canonical -join "`n")))
}

function Assert-LtaooNoReparseInPath {
    param([Parameter(Mandatory = $true)][string]$LiteralPath)
    $target = [IO.Path]::GetFullPath($LiteralPath)
    $root = [IO.Path]::GetPathRoot($target)
    $relative = $target.Substring($root.Length)
    $cursor = $root
    foreach ($segment in $relative.Split([IO.Path]::DirectorySeparatorChar, [StringSplitOptions]::RemoveEmptyEntries)) {
        $cursor = Join-Path $cursor $segment
        if ([IO.Directory]::Exists($cursor) -or [IO.File]::Exists($cursor)) {
            $item = Get-Item -Force -LiteralPath $cursor
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'reparse_point_rejected' }
        }
    }
}

function Assert-LtaooNoReparsePoint {
    param(
        [Parameter(Mandatory = $true)][string]$BasePath,
        [Parameter(Mandatory = $true)][string]$TargetPath
    )
    $base = [IO.Path]::GetFullPath($BasePath)
    $target = [IO.Path]::GetFullPath($TargetPath)
    if ($target -ne $base -and -not $target.StartsWith($base + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'path_outside_runtime_root'
    }
    $cursor = $base
    while ($true) {
        if ([IO.Directory]::Exists($cursor) -or [IO.File]::Exists($cursor)) {
            $item = Get-Item -Force -LiteralPath $cursor
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'reparse_point_rejected' }
        }
        if ($cursor -eq $target) { break }
        $relative = $target.Substring($cursor.Length).TrimStart([IO.Path]::DirectorySeparatorChar)
        $nextSegment = $relative.Split([IO.Path]::DirectorySeparatorChar)[0]
        $cursor = Join-Path $cursor $nextSegment
    }
}

function Assert-LtaooAllowedProperties {
    param(
        [Parameter(Mandatory = $true)][object]$Value,
        [Parameter(Mandatory = $true)][string[]]$AllowedProperties
    )
    $allowed = @{}
    foreach ($name in $AllowedProperties) { $allowed[$name] = $true }
    foreach ($property in $Value.PSObject.Properties) {
        if (-not $allowed.ContainsKey($property.Name)) { throw 'json_property_not_allowed' }
    }
    foreach ($name in $AllowedProperties) {
        if ($null -eq $Value.PSObject.Properties[$name]) { throw 'json_property_missing' }
    }
}

function Read-LtaooStrictJson {
    param(
        [Parameter(Mandatory = $true)][string]$LiteralPath,
        [Parameter(Mandatory = $true)][string[]]$AllowedProperties
    )
    if (-not [IO.File]::Exists($LiteralPath)) { throw 'json_file_missing' }
    $item = Get-Item -Force -LiteralPath $LiteralPath
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.Length -gt 1MB) { throw 'json_file_unsafe' }
    try {
        $bytes = [IO.File]::ReadAllBytes($LiteralPath)
        $text = [Text.UTF8Encoding]::new($false, $true).GetString($bytes)
        $value = $text | ConvertFrom-Json
    }
    catch { throw 'json_file_invalid' }
    Assert-LtaooAllowedProperties -Value $value -AllowedProperties $AllowedProperties
    return $value
}

function Assert-LtaooOwnerOnlyAcl {
    param([Parameter(Mandatory = $true)][string]$LiteralPath)
    $currentSid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $trustedSids = @($currentSid, 'S-1-3-4', 'S-1-5-18', 'S-1-5-32-544')
    $acl = Get-Acl -LiteralPath $LiteralPath
    try { $ownerSid = [Security.Principal.SecurityIdentifier]::new($acl.Owner).Value }
    catch { $ownerSid = ([Security.Principal.NTAccount]::new($acl.Owner)).Translate([Security.Principal.SecurityIdentifier]).Value }
    if (-not $acl.AreAccessRulesProtected -or $ownerSid -ne $currentSid) { throw 'grant_acl_not_owner_only' }
    $currentUserAllowed = $false
    foreach ($rule in $acl.Access) {
        $ruleSid = $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
        if ($ruleSid -notin $trustedSids -or $rule.IsInherited -or $rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow) {
            throw 'grant_acl_not_owner_only'
        }
        if ($ruleSid -eq $currentSid) { $currentUserAllowed = $true }
    }
    if (-not $currentUserAllowed) { throw 'grant_acl_not_owner_only' }
}

function Use-LtaooRunGrant {
    param(
        [Parameter(Mandatory = $true)][string]$GrantPath,
        [Parameter(Mandatory = $true)][string]$RunId,
        [Parameter(Mandatory = $true)][string]$RequestPath,
        [Parameter(Mandatory = $true)][string[]]$RuntimePaths,
        [Parameter(Mandatory = $true)][string]$LtaooExePath,
        [Parameter(Mandatory = $true)][string]$BatchExePath,
        [string]$ExpectedAuthorizationMode = 'wechat-channels-local-runtime-v1',
        [string]$RouterKind = '',
        [string]$RouterExePath = '',
        [string]$RouterConfigPath = '',
        [string]$RouterCapabilityFingerprint = ''
    )
    $legacyMode = 'wechat-channels-local-runtime-v1'
    $routerMode = 'wechat-channels-local-runtime-v2'
    $commonProperties = @(
        'schema_version', 'authorization_mode', 'run_id', 'windows_sid', 'request_sha256',
        'runtime_paths_sha256', 'ltaoo_executable_sha256', 'batch_executable_sha256',
        'expires_at', 'actions'
    )
    if ($ExpectedAuthorizationMode -ceq $legacyMode) {
        if (
            -not [string]::IsNullOrWhiteSpace($RouterKind) -or
            -not [string]::IsNullOrWhiteSpace($RouterExePath) -or
            -not [string]::IsNullOrWhiteSpace($RouterConfigPath) -or
            -not [string]::IsNullOrWhiteSpace($RouterCapabilityFingerprint)
        ) {
            throw 'grant_argument_group_invalid'
        }
        $allowedProperties = $commonProperties
        $expectedSchemaVersion = 1
    } elseif ($ExpectedAuthorizationMode -ceq $routerMode) {
        if (
            [string]::IsNullOrWhiteSpace($RouterKind) -or
            [string]::IsNullOrWhiteSpace($RouterExePath) -or
            [string]::IsNullOrWhiteSpace($RouterConfigPath) -or
            [string]::IsNullOrWhiteSpace($RouterCapabilityFingerprint)
        ) {
            throw 'grant_argument_group_invalid'
        }
        if ($RouterKind -cne 'mihomo') { throw 'router_kind_unsupported' }
        if ($RouterCapabilityFingerprint -cnotmatch '^[a-f0-9]{64}$') { throw 'grant_router_capability_fingerprint_invalid' }
        $allowedProperties = @(
            $commonProperties
            'router_kind'
            'router_executable_sha256'
            'router_config_path_sha256'
            'router_capability_fingerprint'
        )
        $expectedSchemaVersion = 2
    } else {
        throw 'grant_authorization_mode_unsupported'
    }
    Assert-LtaooOwnerOnlyAcl -LiteralPath $GrantPath
    $grant = Read-LtaooStrictJson -LiteralPath $GrantPath -AllowedProperties $allowedProperties
    if ([int]$grant.schema_version -ne $expectedSchemaVersion -or [string]$grant.authorization_mode -cne $ExpectedAuthorizationMode -or [string]$grant.run_id -cne $RunId) {
        throw 'grant_identity_invalid'
    }
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    if ([string]$grant.windows_sid -cne $sid) { throw 'grant_sid_mismatch' }
    $expiresText = [string]$grant.expires_at
    if ($expiresText -notmatch '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,7})?(?:Z|[+-]\d{2}:\d{2})$') { throw 'grant_expiry_invalid' }
    $expires = [DateTimeOffset]::Parse($expiresText, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::RoundtripKind)
    if ($expires -le [DateTimeOffset]::UtcNow -or $expires -gt [DateTimeOffset]::UtcNow.AddMinutes(6)) { throw 'grant_expired' }
    if ([string]$grant.request_sha256 -cne (Get-LtaooFileHash -LiteralPath $RequestPath)) { throw 'grant_request_mismatch' }
    if ([string]$grant.runtime_paths_sha256 -cne (Get-LtaooRuntimePathsHash -LiteralPaths $RuntimePaths)) { throw 'grant_runtime_paths_mismatch' }
    if ([string]$grant.ltaoo_executable_sha256 -cne (Get-LtaooFileHash -LiteralPath $LtaooExePath)) { throw 'grant_ltaoo_hash_mismatch' }
    if ([string]$grant.batch_executable_sha256 -cne (Get-LtaooFileHash -LiteralPath $BatchExePath)) { throw 'grant_batch_hash_mismatch' }
    if ($ExpectedAuthorizationMode -ceq $routerMode) {
        if ([string]$grant.router_kind -cne $RouterKind) { throw 'grant_router_kind_mismatch' }
        if ([string]$grant.router_executable_sha256 -cne (Get-LtaooFileHash -LiteralPath $RouterExePath)) { throw 'grant_router_hash_mismatch' }
        $canonicalConfigPath = [IO.Path]::GetFullPath($RouterConfigPath).ToLowerInvariant()
        if ([string]$grant.router_config_path_sha256 -cne (Get-LtaooStringHash -Value $canonicalConfigPath)) { throw 'grant_router_config_path_mismatch' }
        if ([string]$grant.router_capability_fingerprint -cne $RouterCapabilityFingerprint) { throw 'grant_router_capability_mismatch' }
    }
    $actions = @($grant.actions | ForEach-Object { [string]$_ } | Sort-Object)
    $expected = if ($ExpectedAuthorizationMode -ceq $routerMode) {
        @('install_current_user_ca', 'modify_proxy_router', 'start_ltaoo')
    } else {
        @('install_current_user_ca', 'modify_clash', 'start_ltaoo')
    }
    if (($actions -join '|') -cne ($expected -join '|')) { throw 'grant_actions_invalid' }
    Remove-Item -LiteralPath $GrantPath -Force
    if ([IO.File]::Exists($GrantPath)) { throw 'grant_consume_failed' }
    return $grant
}

function Get-LtaooNewline {
    param([Parameter(Mandatory = $true)][string]$Text)
    if ($Text.Contains("`r`n")) { return "`r`n" }
    return "`n"
}

function Get-LtaooSequenceIndent {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][string]$Section
    )
    $escaped = [Text.RegularExpressions.Regex]::Escape($Section)
    $pattern = '(?ms)^' + $escaped + ':[ \t]*(?:\[[ \t]*\])?[ \t]*\r?\n(?:(?:[ \t]*#.*)?\r?\n)*(?<indent>[ \t]*)-(?:[ \t]|$)'
    $match = [Text.RegularExpressions.Regex]::Match($Text, $pattern)
    if ($match.Success) { return $match.Groups['indent'].Value }
    return '  '
}

function Add-LtaooClashBlock {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$')][string]$RunId,
        [Parameter(Mandatory = $true)][ValidateRange(1, 65535)][int]$ProxyPort
    )
    if ($Text -match '# TREND_RADAR_WX_(BEGIN|END) ') { throw 'clash_runtime_marker_exists' }
    $newline = Get-LtaooNewline -Text $Text
    $proxyName = 'trendradar-wx-' + $RunId
    $proxyIndent = Get-LtaooSequenceIndent -Text $Text -Section 'proxies'
    $proxyPropertyIndent = $proxyIndent + '  '
    $rulesIndent = Get-LtaooSequenceIndent -Text $Text -Section 'rules'
    $proxyBlock = @(
        ($proxyIndent + "# TREND_RADAR_WX_BEGIN $RunId PROXY"),
        ($proxyIndent + "- name: $proxyName"),
        ($proxyPropertyIndent + 'type: http'),
        ($proxyPropertyIndent + 'server: 127.0.0.1'),
        ($proxyPropertyIndent + "port: $ProxyPort"),
        ($proxyIndent + "# TREND_RADAR_WX_END $RunId PROXY")
    ) -join $newline
    $ruleBlock = @(
        ($rulesIndent + "# TREND_RADAR_WX_BEGIN $RunId RULES"),
        ($rulesIndent + '- PROCESS-NAME,wx_video_download.exe,DIRECT'),
        ($rulesIndent + "- PROCESS-NAME,WeChatAppEx.exe,$proxyName"),
        ($rulesIndent + "- PROCESS-NAME,Weixin.exe,$proxyName"),
        ($rulesIndent + "- PROCESS-NAME,WeChat.exe,$proxyName"),
        ($rulesIndent + "# TREND_RADAR_WX_END $RunId RULES")
    ) -join $newline
    $proxyPattern = '(?m)^proxies:[ \t]*(?:\[[ \t]*\])?[ \t]*(\r?\n)'
    $rulesPattern = '(?m)^rules:[ \t]*(?:\[[ \t]*\])?[ \t]*(\r?\n)'
    if (-not [Text.RegularExpressions.Regex]::IsMatch($Text, $proxyPattern) -or -not [Text.RegularExpressions.Regex]::IsMatch($Text, $rulesPattern)) {
        throw 'clash_required_sections_missing'
    }
    $updated = [Text.RegularExpressions.Regex]::Replace($Text, $proxyPattern, { param($match) 'proxies:' + $match.Groups[1].Value + $proxyBlock + $newline }, 1)
    $updated = [Text.RegularExpressions.Regex]::Replace($updated, $rulesPattern, { param($match) 'rules:' + $match.Groups[1].Value + $ruleBlock + $newline }, 1)
    return $updated
}

function Remove-LtaooClashBlock {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$')][string]$RunId
    )
    $escaped = [Text.RegularExpressions.Regex]::Escape($RunId)
    foreach ($kind in @('PROXY', 'RULES')) {
        $pattern = "(?ms)^[ \t]*# TREND_RADAR_WX_BEGIN $escaped $kind\r?\n.*?^[ \t]*# TREND_RADAR_WX_END $escaped $kind\r?\n?"
        $matches = [Text.RegularExpressions.Regex]::Matches($Text, $pattern)
        if ($matches.Count -ne 1) { throw 'clash_runtime_marker_invalid' }
        $Text = [Text.RegularExpressions.Regex]::Replace($Text, $pattern, '', 1)
    }
    if ($Text -match '# TREND_RADAR_WX_(BEGIN|END) ') { throw 'clash_foreign_runtime_marker_present' }
    return $Text
}

function ConvertFrom-LtaooUtf8Bytes {
    param([Parameter(Mandatory = $true)][byte[]]$Bytes)
    $hasBom = $Bytes.Length -ge 3 -and $Bytes[0] -eq 0xEF -and $Bytes[1] -eq 0xBB -and $Bytes[2] -eq 0xBF
    $offset = if ($hasBom) { 3 } else { 0 }
    $length = $Bytes.Length - $offset
    $encoding = [Text.UTF8Encoding]::new($false, $true)
    try { $decoded = $encoding.GetString($Bytes, $offset, $length) }
    catch { throw 'clash_config_not_utf8' }
    return [pscustomobject]@{ Text = $decoded; HasBom = [bool]$hasBom }
}

function ConvertTo-LtaooUtf8Bytes {
    param([Parameter(Mandatory = $true)][string]$Text, [Parameter(Mandatory = $true)][bool]$WithBom)
    $body = [Text.UTF8Encoding]::new($false, $true).GetBytes($Text)
    if (-not $WithBom) { return $body }
    $preamble = [Text.UTF8Encoding]::new($true).GetPreamble()
    $combined = [byte[]]::new($preamble.Length + $body.Length)
    [Array]::Copy($preamble, 0, $combined, 0, $preamble.Length)
    [Array]::Copy($body, 0, $combined, $preamble.Length, $body.Length)
    return $combined
}

function Get-ClashRecoveryAction {
    param(
        [Parameter(Mandatory = $true)][string]$BaselineHash,
        [Parameter(Mandatory = $true)][string]$TemporaryHash,
        [Parameter(Mandatory = $true)][string]$CurrentHash,
        [Parameter(Mandatory = $true)][bool]$MarkerPresent
    )
    if ($CurrentHash -ceq $TemporaryHash) { return 'restore_backup' }
    if ($MarkerPresent) { return 'remove_marker' }
    if ($CurrentHash -ceq $BaselineHash) { return 'already_restored' }
    return 'attention_required'
}

function Test-LtaooProcessIdentity {
    param(
        [Parameter(Mandatory = $true)][int]$ProcessId,
        [Parameter(Mandatory = $true)][string]$ExpectedPath,
        [Parameter(Mandatory = $true)][string]$ExpectedStartTime,
        [Parameter(Mandatory = $true)][string]$ExpectedSha256
    )
    $process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
    if ($null -eq $process) { return $false }
    try {
        $startMatches = $process.StartTime.ToUniversalTime().ToString('o') -ceq $ExpectedStartTime
        $pathMatches = [string]::Equals([IO.Path]::GetFullPath($process.Path), [IO.Path]::GetFullPath($ExpectedPath), [StringComparison]::OrdinalIgnoreCase)
        $hashMatches = (Get-LtaooFileHash -LiteralPath $process.Path) -ceq $ExpectedSha256
        return $startMatches -and $pathMatches -and $hashMatches
    } catch { return $false }
}

function Test-LtaooProcessIdentityOrAbsent {
    param(
        [Parameter(Mandatory = $true)][int]$ProcessId,
        [Parameter(Mandatory = $true)][string]$ExpectedPath,
        [Parameter(Mandatory = $true)][string]$ExpectedStartTime,
        [Parameter(Mandatory = $true)][string]$ExpectedSha256
    )
    if (Test-LtaooProcessIdentity -ProcessId $ProcessId -ExpectedPath $ExpectedPath -ExpectedStartTime $ExpectedStartTime -ExpectedSha256 $ExpectedSha256) {
        return $true
    }
    return $null -eq (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue)
}

function Wait-LtaooProcessStopped {
    param(
        [Parameter(Mandatory = $true)][int]$ProcessId,
        [ValidateRange(1, 60000)][int]$TimeoutMilliseconds = 10000,
        [ValidateRange(1, 1000)][int]$PollMilliseconds = 100
    )
    $deadline = [DateTime]::UtcNow.AddMilliseconds($TimeoutMilliseconds)
    do {
        if ($null -eq (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue)) { return $true }
        $remaining = ($deadline - [DateTime]::UtcNow).TotalMilliseconds
        if ($remaining -le 0) { break }
        Start-Sleep -Milliseconds ([Math]::Min($PollMilliseconds, [int][Math]::Ceiling($remaining)))
    } while ($true)
    return $null -eq (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue)
}

function Get-LtaooClashController {
    param([Parameter(Mandatory = $true)][string]$Text)
    $controllerMatch = [Text.RegularExpressions.Regex]::Match($Text, '(?m)^external-controller:[ \t]*["'']?([^"''#\r\n]+)["'']?[ \t]*(?:#.*)?$')
    $secret = ''
    $secretMatch = [Text.RegularExpressions.Regex]::Match($Text, '(?m)^secret:[ \t]*(?:"([^"]*)"|''([^'']*)''|([^#\r\n]*))[ \t]*(?:#.*)?$')
    if ($secretMatch.Success) {
        foreach ($groupIndex in @(1, 2, 3)) {
            if ($secretMatch.Groups[$groupIndex].Success) { $secret = $secretMatch.Groups[$groupIndex].Value.Trim(); break }
        }
    }
    if ($controllerMatch.Success) {
        $authority = $controllerMatch.Groups[1].Value.Trim()
        $uri = $null
        if (-not [Uri]::TryCreate(('http://' + $authority), [UriKind]::Absolute, [ref]$uri)) { throw 'clash_controller_invalid' }
        $ip = $null
        if (-not [Net.IPAddress]::TryParse($uri.Host, [ref]$ip) -or -not [Net.IPAddress]::IsLoopback($ip) -or $uri.Port -lt 1) {
            throw 'clash_controller_not_loopback'
        }
        return [pscustomobject]@{
            Kind = 'http'
            Uri = $uri.GetLeftPart([UriPartial]::Authority)
            PipeName = ''
            Secret = $secret
        }
    }

    $pipeMatch = [Text.RegularExpressions.Regex]::Match($Text, '(?m)^external-controller-pipe:[ \t]*(?:"([^"]+)"|''([^'']+)''|([^#\r\n]+))[ \t]*(?:#.*)?$')
    if (-not $pipeMatch.Success) { throw 'clash_controller_missing' }
    $pipePath = ''
    foreach ($groupIndex in @(1, 2, 3)) {
        if ($pipeMatch.Groups[$groupIndex].Success) { $pipePath = $pipeMatch.Groups[$groupIndex].Value.Trim(); break }
    }
    $pipePrefix = '\\.\pipe\'
    if (-not $pipePath.StartsWith($pipePrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'clash_controller_pipe_invalid' }
    $pipeName = $pipePath.Substring($pipePrefix.Length)
    if ($pipeName -notmatch '^[A-Za-z0-9._-]{1,128}$') { throw 'clash_controller_pipe_invalid' }
    return [pscustomobject]@{
        Kind = 'pipe'
        Uri = ''
        PipeName = $pipeName
        Secret = $secret
    }
}

function Test-LtaooClashConfig {
    param(
        [Parameter(Mandatory = $true)][string]$ClashExePath,
        [Parameter(Mandatory = $true)][string]$ConfigPath,
        [string]$DataDirectory = ''
    )
    $resolvedDataDirectory = if ([string]::IsNullOrWhiteSpace($DataDirectory)) {
        [IO.Path]::GetDirectoryName($ConfigPath)
    } else {
        [IO.Path]::GetFullPath($DataDirectory)
    }
    if (-not [IO.Directory]::Exists($resolvedDataDirectory)) { throw 'clash_data_directory_missing' }
    if ($ConfigPath.Contains('"') -or $resolvedDataDirectory.Contains('"')) { throw 'unsupported_path_character' }
    $arguments = '-t -d "' + $resolvedDataDirectory + '" -f "' + $ConfigPath + '"'
    $process = Start-Process -FilePath $ClashExePath -ArgumentList $arguments -PassThru -Wait -WindowStyle Hidden
    if ($process.ExitCode -ne 0) { throw 'clash_config_invalid' }
}

function Invoke-LtaooClashReload {
    param(
        [Parameter(Mandatory = $true)][object]$Controller,
        [Parameter(Mandatory = $true)][string]$ConfigPath
    )
    $body = @{ path = $ConfigPath } | ConvertTo-Json -Compress
    if ([string]$Controller.Kind -ceq 'pipe') {
        Invoke-LtaooClashPipeReload -PipeName ([string]$Controller.PipeName) -Secret ([string]$Controller.Secret) -Body $body
        return
    }
    if ([string]$Controller.Kind -cne 'http') { throw 'clash_controller_invalid' }
    $headers = @{}
    if (-not [string]::IsNullOrEmpty([string]$Controller.Secret)) { $headers.Authorization = 'Bearer ' + [string]$Controller.Secret }
    $response = Invoke-WebRequest -UseBasicParsing -Method Put -Uri ([string]$Controller.Uri + '/configs?force=true') -Headers $headers -ContentType 'application/json' -Body $body
    if ([int]$response.StatusCode -lt 200 -or [int]$response.StatusCode -ge 300) { throw 'clash_reload_failed' }
}

function Invoke-LtaooClashPipeReload {
    param(
        [Parameter(Mandatory = $true)][string]$PipeName,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Secret,
        [Parameter(Mandatory = $true)][string]$Body
    )
    if ($PipeName -notmatch '^[A-Za-z0-9._-]{1,128}$') { throw 'clash_controller_pipe_invalid' }
    $bodyBytes = [Text.UTF8Encoding]::new($false).GetBytes($Body)
    $authorization = ''
    if (-not [string]::IsNullOrEmpty($Secret)) { $authorization = 'Authorization: Bearer ' + $Secret + "`r`n" }
    $head = 'PUT /configs?force=true HTTP/1.1' + "`r`n" +
        'Host: localhost' + "`r`n" + $authorization +
        'Content-Type: application/json' + "`r`n" +
        'Content-Length: ' + $bodyBytes.Length + "`r`n" +
        'Connection: close' + "`r`n`r`n"
    $headBytes = [Text.ASCIIEncoding]::new().GetBytes($head)
    $pipe = [IO.Pipes.NamedPipeClientStream]::new('.', $PipeName, [IO.Pipes.PipeDirection]::InOut, [IO.Pipes.PipeOptions]::Asynchronous)
    try {
        $pipe.Connect(5000)
        $pipe.Write($headBytes, 0, $headBytes.Length)
        $pipe.Write($bodyBytes, 0, $bodyBytes.Length)
        $pipe.Flush()
        $buffer = [byte[]]::new(4096)
        $readTask = $pipe.ReadAsync($buffer, 0, $buffer.Length)
        if (-not $readTask.Wait(5000)) { throw 'clash_reload_timeout' }
        $read = $readTask.Result
        if ($read -le 0) { throw 'clash_reload_failed' }
        $responseHead = [Text.ASCIIEncoding]::new().GetString($buffer, 0, $read)
        $statusMatch = [Text.RegularExpressions.Regex]::Match($responseHead, '^HTTP/1\.[01] ([0-9]{3})')
        if (-not $statusMatch.Success) { throw 'clash_reload_failed' }
        $statusCode = [int]$statusMatch.Groups[1].Value
        if ($statusCode -lt 200 -or $statusCode -ge 300) { throw 'clash_reload_failed' }
    } finally {
        $pipe.Dispose()
    }
}

function Write-LtaooBytesAtomic {
    param(
        [Parameter(Mandatory = $true)][byte[]]$Bytes,
        [Parameter(Mandatory = $true)][string]$LiteralPath
    )
    $temporary = $LiteralPath + '.' + [Guid]::NewGuid().ToString('N') + '.tmp'
    $backup = $temporary + '.bak'
    $destinationAcl = $null
    if ([IO.File]::Exists($LiteralPath)) { $destinationAcl = Get-Acl -LiteralPath $LiteralPath }
    [IO.File]::WriteAllBytes($temporary, $Bytes)
    try {
        if ($null -ne $destinationAcl) { Set-Acl -LiteralPath $temporary -AclObject $destinationAcl }
        if ([IO.File]::Exists($LiteralPath)) { [IO.File]::Replace($temporary, $LiteralPath, $backup) }
        else { [IO.File]::Move($temporary, $LiteralPath) }
    } finally {
        if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
        if ([IO.File]::Exists($backup)) { [IO.File]::Delete($backup) }
    }
}

function Write-LtaooJsonAtomic {
    param(
        [Parameter(Mandatory = $true)][object]$Value,
        [Parameter(Mandatory = $true)][string]$LiteralPath
    )
    $json = $Value | ConvertTo-Json -Depth 12
    Write-LtaooBytesAtomic -Bytes ([Text.UTF8Encoding]::new($false).GetBytes($json)) -LiteralPath $LiteralPath
}

function ConvertTo-LtaooPem {
    param([Parameter(Mandatory = $true)][string]$Label, [Parameter(Mandatory = $true)][byte[]]$Bytes)
    $base64 = [Convert]::ToBase64String($Bytes)
    $builder = [Text.StringBuilder]::new()
    [void]$builder.AppendLine("-----BEGIN $Label-----")
    for ($offset = 0; $offset -lt $base64.Length; $offset += 64) {
        [void]$builder.AppendLine($base64.Substring($offset, [Math]::Min(64, $base64.Length - $offset)))
    }
    [void]$builder.AppendLine("-----END $Label-----")
    return $builder.ToString()
}

function Set-LtaooOwnerOnlyAcl {
    param([Parameter(Mandatory = $true)][string]$LiteralPath, [Parameter(Mandatory = $true)][bool]$Directory)
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
    $rule = [Security.AccessControl.FileSystemAccessRule]::new($sid, [Security.AccessControl.FileSystemRights]::FullControl, $inheritance, [Security.AccessControl.PropagationFlags]::None, [Security.AccessControl.AccessControlType]::Allow)
    [void]$security.AddAccessRule($rule)
    Set-Acl -LiteralPath $LiteralPath -AclObject $security
}

function New-LtaooRunCertificate {
    param(
        [Parameter(Mandatory = $true)][string]$RunId,
        [Parameter(Mandatory = $true)][string]$SecretsRoot
    )
    [void][IO.Directory]::CreateDirectory($SecretsRoot)
    Set-LtaooOwnerOnlyAcl -LiteralPath $SecretsRoot -Directory $true
    $certPath = Join-Path $SecretsRoot 'ca-cert.pem'
    $keyPath = Join-Path $SecretsRoot 'ca-key.pem'
    $subject = 'CN=TrendRadarWX-' + $RunId
    $rsa = [Security.Cryptography.RSA]::Create(2048)
    try {
        $request = [Security.Cryptography.X509Certificates.CertificateRequest]::new($subject, $rsa, [Security.Cryptography.HashAlgorithmName]::SHA256, [Security.Cryptography.RSASignaturePadding]::Pkcs1)
        $request.CertificateExtensions.Add([Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]::new($true, $false, 0, $true))
        $keyUsage = [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::KeyCertSign -bor [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::CrlSign
        $request.CertificateExtensions.Add([Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new($keyUsage, $true))
        $certificate = $request.CreateSelfSigned([DateTimeOffset]::UtcNow.AddMinutes(-5), [DateTimeOffset]::UtcNow.AddDays(1))
        try {
            [IO.File]::WriteAllText($certPath, (ConvertTo-LtaooPem -Label 'CERTIFICATE' -Bytes $certificate.Export([Security.Cryptography.X509Certificates.X509ContentType]::Cert)), [Text.UTF8Encoding]::new($false))
            if ($rsa -is [Security.Cryptography.RSACng]) {
                $privateKeyBytes = $rsa.Key.Export([Security.Cryptography.CngKeyBlobFormat]::Pkcs8PrivateBlob)
            } elseif ($null -ne $rsa.PSObject.Methods['ExportPkcs8PrivateKey']) {
                $privateKeyBytes = $rsa.ExportPkcs8PrivateKey()
            } else {
                throw 'private_key_export_unsupported'
            }
            [IO.File]::WriteAllText($keyPath, (ConvertTo-LtaooPem -Label 'PRIVATE KEY' -Bytes $privateKeyBytes), [Text.UTF8Encoding]::new($false))
            Set-LtaooOwnerOnlyAcl -LiteralPath $keyPath -Directory $false
            return [pscustomobject]@{ CertificatePath = $certPath; PrivateKeyPath = $keyPath; Thumbprint = $certificate.Thumbprint.ToUpperInvariant(); Subject = $subject }
        } finally { $certificate.Dispose() }
    } finally { $rsa.Dispose() }
}

function Install-LtaooCurrentUserCA {
    param([Parameter(Mandatory = $true)][string]$CertificatePath, [Parameter(Mandatory = $true)][string]$Thumbprint)
    & certutil.exe -user -addstore Root $CertificatePath *> $null
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath ('Cert:\CurrentUser\Root\' + $Thumbprint))) { throw 'ca_install_failed' }
}

function Remove-LtaooCurrentUserCA {
    param([Parameter(Mandatory = $true)][string]$Thumbprint)
    if (Test-Path -LiteralPath ('Cert:\CurrentUser\Root\' + $Thumbprint)) {
        & certutil.exe -user -delstore Root $Thumbprint *> $null
    }
    return -not (Test-Path -LiteralPath ('Cert:\CurrentUser\Root\' + $Thumbprint))
}

Export-ModuleMember -Function @(
    'Get-LtaooFileHash', 'Get-LtaooStringHash', 'Get-LtaooRuntimePathsHash', 'Assert-LtaooNoReparseInPath', 'Assert-LtaooNoReparsePoint', 'Assert-LtaooAllowedProperties',
    'Read-LtaooStrictJson', 'Assert-LtaooOwnerOnlyAcl', 'Use-LtaooRunGrant', 'Add-LtaooClashBlock', 'Remove-LtaooClashBlock',
    'Get-ClashRecoveryAction', 'ConvertFrom-LtaooUtf8Bytes', 'ConvertTo-LtaooUtf8Bytes', 'Test-LtaooProcessIdentity', 'Test-LtaooProcessIdentityOrAbsent', 'Wait-LtaooProcessStopped', 'Get-LtaooClashController', 'Test-LtaooClashConfig', 'Invoke-LtaooClashReload',
    'Write-LtaooBytesAtomic', 'Write-LtaooJsonAtomic', 'New-LtaooRunCertificate', 'Install-LtaooCurrentUserCA',
    'Remove-LtaooCurrentUserCA', 'Set-LtaooOwnerOnlyAcl'
)
