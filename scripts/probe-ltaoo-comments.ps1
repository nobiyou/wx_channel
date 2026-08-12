[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9-]{1,80}$')][string]$RunId,
    [Parameter(Mandatory = $true)][string]$ShareUrl,
    [string]$RepoRoot = "",
    [string]$ApiBase = ""
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
    return $candidate
}

function Assert-LoopbackApiBase {
    param([string]$Value)
    try { $uri = [Uri]$Value } catch { throw "api_base_not_loopback" }
    if (-not $uri.IsAbsoluteUri -or $uri.Scheme -ne "http" -or $uri.Host -ne "127.0.0.1" -or $uri.IsDefaultPort) {
        throw "api_base_not_loopback"
    }
    return $uri.GetLeftPart([UriPartial]::Authority).TrimEnd('/')
}

function Get-Sha256Hex {
    param([byte[]]$Bytes)
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        return (($sha.ComputeHash($Bytes) | ForEach-Object { $_.ToString("x2") }) -join "")
    } finally {
        $sha.Dispose()
    }
}

function Get-TextHash {
    param([string]$Value)
    return Get-Sha256Hex ([Text.Encoding]::UTF8.GetBytes($Value))
}

function Get-SaltedTextHash {
    param([byte[]]$Salt, [string]$Value)
    $valueBytes = [Text.Encoding]::UTF8.GetBytes($Value)
    $combined = New-Object byte[] ($Salt.Length + $valueBytes.Length)
    [Array]::Copy($Salt, 0, $combined, 0, $Salt.Length)
    [Array]::Copy($valueBytes, 0, $combined, $Salt.Length, $valueBytes.Length)
    return Get-Sha256Hex $combined
}

function Write-JsonAtomic {
    param([object]$Value, [string]$Path)
    $temporary = $Path + "." + [Guid]::NewGuid().ToString("N") + ".tmp"
    $json = $Value | ConvertTo-Json -Depth 10
    [IO.File]::WriteAllText($temporary, $json, [Text.UTF8Encoding]::new($false))
    try {
        if ([IO.File]::Exists($Path)) {
            [IO.File]::Replace($temporary, $Path, $null)
        } else {
            [IO.File]::Move($temporary, $Path)
        }
    } finally {
        if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
    }
}

function Invoke-LocalJsonGet {
    param([Net.Http.HttpClient]$Client, [string]$Uri, [string]$Stage)
    try {
        $response = $Client.GetAsync($Uri).GetAwaiter().GetResult()
        try {
            $statusCode = [int]$response.StatusCode
            if (-not $response.IsSuccessStatusCode) { throw ($Stage + "_http_error") }
            $raw = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            try { $body = $raw | ConvertFrom-Json } catch { throw ($Stage + "_schema_error") }
            return [pscustomobject]@{ status_code = $statusCode; body = $body }
        } finally {
            $response.Dispose()
        }
    } catch {
        if ($_.Exception.Message -in @(($Stage + "_http_error"), ($Stage + "_schema_error"))) { throw }
        throw ($Stage + "_request_failed")
    }
}

function Assert-BusinessSuccess {
    param([object]$Response, [string]$Stage)
    try {
        if ([int]$Response.body.code -ne 0 -or [int]$Response.body.data.errCode -ne 0) {
            throw ($Stage + "_business_error")
        }
    } catch {
        if ($_.Exception.Message -eq ($Stage + "_business_error")) { throw }
        throw ($Stage + "_schema_error")
    }
}

function New-PageSummary {
    param([object]$Response, [byte[]]$Salt, [Collections.Generic.HashSet[string]]$PriorIds)
    $comments = @()
    if ($null -ne $Response.body.data.data.commentInfo) { $comments = @($Response.body.data.data.commentInfo) }
    $pageIds = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $hashes = [Collections.Generic.List[string]]::new()
    $pageDuplicates = 0
    $crossDuplicates = 0
    foreach ($comment in $comments) {
        $commentId = [string]$comment.commentId
        if ([string]::IsNullOrEmpty($commentId)) { continue }
        if (-not $pageIds.Add($commentId)) { $pageDuplicates++ }
        if ($PriorIds.Contains($commentId)) { $crossDuplicates++ }
        [void]$PriorIds.Add($commentId)
        $hashes.Add((Get-SaltedTextHash $Salt $commentId))
    }
    $marker = [string]$Response.body.data.data.lastBuffer
    return [pscustomobject]@{
        summary = [ordered]@{
            http_status = $Response.status_code
            business_code = [int]$Response.body.data.errCode
            comment_count = $comments.Count
            comment_id_hashes = @($hashes)
            page_duplicate_count = $pageDuplicates
            cross_page_duplicate_count = $crossDuplicates
            last_buffer_present = -not [string]::IsNullOrEmpty($marker)
            last_buffer_length = $marker.Length
            last_buffer_hash = if ([string]::IsNullOrEmpty($marker)) { "" } else { Get-SaltedTextHash $Salt $marker }
        }
        marker = $marker
    }
}

$repoRootValue = $null
$runRoot = $null
$summaryPath = $null
$client = $null
$commentRequestCount = 0
$reasonCode = "unexpected_failure"
$currentStage = "startup"

try {
    $currentStage = "manifest"
    $repoRootValue = Resolve-ProbeRepoRoot $RepoRoot
    $runRoot = Resolve-RunRoot $repoRootValue $RunId
    $manifestPath = Join-Path $runRoot "manifest.json"
    $summaryPath = Join-Path $runRoot "probe-summary.json"
    if (-not [IO.File]::Exists($manifestPath)) { throw "manifest_not_found" }
    try { $manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json } catch { throw "manifest_invalid" }
    if ([int]$manifest.schema_version -ne 1 -or [string]$manifest.run_id -cne $RunId) { throw "manifest_invalid" }
    if ([IO.Path]::GetFullPath([string]$manifest.runtime_root) -ne $runRoot) { throw "manifest_invalid" }

    $selectedApiBase = if ([string]::IsNullOrWhiteSpace($ApiBase)) { [string]$manifest.ltaoo.api_base } else { $ApiBase }
    $apiBaseValue = Assert-LoopbackApiBase $selectedApiBase

    $currentStage = "http_client"
    Add-Type -AssemblyName System.Net.Http
    $client = [Net.Http.HttpClient]::new()
    $client.Timeout = [TimeSpan]::FromSeconds(30)

    $currentStage = "status"
    $status = Invoke-LocalJsonGet $client ($apiBaseValue + "/api/status") "status"
    if ([int]$status.body.code -ne 0 -or -not [bool]$status.body.data.api.listening -or -not [bool]$status.body.data.proxy.listening) {
        throw "ltaoo_not_ready"
    }

    $currentStage = "profile"
    $profileUrl = $apiBaseValue + "/api/channels/feed/profile?url=" + [Uri]::EscapeDataString($ShareUrl)
    $currentStage = "profile_request"
    $profile = Invoke-LocalJsonGet $client $profileUrl "profile"
    $currentStage = "profile_business"
    Assert-BusinessSuccess $profile "profile"
    $currentStage = "profile_fields"
    $oid = [string]$profile.body.data.data.object.id
    $nid = [string]$profile.body.data.data.object.objectNonceId
    if ([string]::IsNullOrEmpty($oid) -or [string]::IsNullOrEmpty($nid)) { throw "profile_schema_error" }

    $currentStage = "hash_setup"
    $salt = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($salt) } finally { $rng.Dispose() }
    $seenIds = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)

    $currentStage = "page_one"
    $pageOneUrl = $apiBaseValue + "/api/channels/feed/comment/list?oid=" + [Uri]::EscapeDataString($oid) + "&nid=" + [Uri]::EscapeDataString($nid)
    $pageOne = Invoke-LocalJsonGet $client $pageOneUrl "page_one"
    $commentRequestCount = 1
    Assert-BusinessSuccess $pageOne "page_one"
    $pageOneResult = New-PageSummary $pageOne $salt $seenIds
    $pages = [Collections.Generic.List[object]]::new()
    $pages.Add($pageOneResult.summary)

    $marker = [string]$pageOneResult.marker
    $requestMarkerHash = ""
    $cursorContinuity = $false
    $finalStatus = "inconclusive_no_second_page"
    $reasonCode = "first_page_has_no_marker"

    if (-not [string]::IsNullOrEmpty($marker)) {
        $currentStage = "page_two"
        $requestMarkerHash = Get-SaltedTextHash $salt $marker
        $pageTwoUrl = $pageOneUrl + "&next_marker=" + [Uri]::EscapeDataString($marker)
        $pageTwo = Invoke-LocalJsonGet $client $pageTwoUrl "page_two"
        $commentRequestCount = 2
        Assert-BusinessSuccess $pageTwo "page_two"
        $pageTwoResult = New-PageSummary $pageTwo $salt $seenIds
        $pages.Add($pageTwoResult.summary)
        $cursorContinuity = $requestMarkerHash -eq [string]$pageOneResult.summary.last_buffer_hash
        if (-not $cursorContinuity) { throw "cursor_continuity_failed" }
        $finalStatus = "verified_two_pages"
        $reasonCode = "two_pages_verified"
    }

    $currentStage = "summary"
    $summary = [ordered]@{
        schema_version = 1
        run_id = $RunId
        completed_at = [DateTimeOffset]::UtcNow.ToString("o")
        share_url_sha256 = Get-TextHash $ShareUrl
        oid_sha256 = Get-TextHash $oid
        nid_sha256 = Get-TextHash $nid
        status_http = $status.status_code
        profile_http = $profile.status_code
        profile_business_code = [int]$profile.body.data.errCode
        pages = @($pages)
        comment_request_count = $commentRequestCount
        second_request_cursor_hash = $requestMarkerHash
        cursor_continuity = $cursorContinuity
        status = $finalStatus
        reason_code = $reasonCode
    }
    Write-JsonAtomic $summary $summaryPath
    [pscustomobject]@{ run_id = $RunId; status = $finalStatus; summary_file = $summaryPath } | ConvertTo-Json -Compress
} catch {
    $allowed = @(
        "invalid_run_root", "manifest_not_found", "manifest_invalid", "api_base_not_loopback", "ltaoo_not_ready",
        "status_http_error", "status_schema_error", "status_request_failed", "profile_http_error", "profile_schema_error",
        "profile_request_failed", "profile_business_error", "page_one_http_error", "page_one_schema_error", "page_one_request_failed",
        "page_one_business_error", "page_two_http_error", "page_two_schema_error", "page_two_request_failed", "page_two_business_error",
        "cursor_continuity_failed"
    )
    if ($_.Exception.Message -in $allowed) { $reasonCode = $_.Exception.Message } else { $reasonCode = "unexpected_" + $currentStage }
    if ($null -ne $summaryPath -and $null -ne $runRoot -and [IO.Directory]::Exists($runRoot)) {
        $failure = [ordered]@{
            schema_version = 1
            run_id = $RunId
            completed_at = [DateTimeOffset]::UtcNow.ToString("o")
            share_url_sha256 = Get-TextHash $ShareUrl
            comment_request_count = $commentRequestCount
            cursor_continuity = $false
            status = "failed"
            reason_code = $reasonCode
        }
        Write-JsonAtomic $failure $summaryPath
    }
    [pscustomobject]@{ run_id = $RunId; status = "failed"; reason_code = $reasonCode } | ConvertTo-Json -Compress
    exit 1
} finally {
    if ($null -ne $client) { $client.Dispose() }
}
