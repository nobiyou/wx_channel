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
    $backup = $temporary + ".bak"
    $json = $Value | ConvertTo-Json -Depth 12
    [IO.File]::WriteAllText($temporary, $json, [Text.UTF8Encoding]::new($false))
    try {
        if ([IO.File]::Exists($Path)) {
            [IO.File]::Replace($temporary, $Path, $backup)
        } else {
            [IO.File]::Move($temporary, $Path)
        }
    } finally {
        if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
        if ([IO.File]::Exists($backup)) { [IO.File]::Delete($backup) }
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

function Test-ObjectProperty {
    param([object]$Object, [string]$Name)
    return $null -ne $Object -and $Object.PSObject.Properties.Name -contains $Name
}

function Get-LtaooStatusEvidence {
    param([object]$Response)
    try {
        if (-not (Test-ObjectProperty $Response "body")) { throw "status_schema_error" }
        $body = $Response.body
        if (-not (Test-ObjectProperty $body "code")) { throw "status_schema_error" }
        if ([int]$body.code -ne 0) { throw "ltaoo_not_ready" }
        if (-not (Test-ObjectProperty $body "data") -or $null -eq $body.data) { throw "status_schema_error" }
        $data = $body.data

        $hasApi = Test-ObjectProperty $data "api"
        $hasProxy = Test-ObjectProperty $data "proxy"
        if ($hasApi -or $hasProxy) {
            if (-not $hasApi -or -not $hasProxy -or $null -eq $data.api -or $null -eq $data.proxy) { throw "status_schema_error" }
            if (-not (Test-ObjectProperty $data.api "listening") -or -not (Test-ObjectProperty $data.proxy "listening")) { throw "status_schema_error" }
            if ($data.api.listening -isnot [bool] -or $data.proxy.listening -isnot [bool]) { throw "status_schema_error" }
            return [pscustomobject]@{
                status_schema = "modern"
                readiness_proof = "listeners_and_profile"
                profile_allowed = [bool]($data.api.listening -and $data.proxy.listening)
            }
        }

        $hasChannels = Test-ObjectProperty $data "channels"
        $hasVersion = Test-ObjectProperty $data "version"
        if ($hasChannels -or $hasVersion) {
            if (-not $hasChannels -or -not $hasVersion -or $null -eq $data.channels) { throw "status_schema_error" }
            if (-not (Test-ObjectProperty $data.channels "available")) { throw "status_schema_error" }
            if ($data.version -isnot [string] -or [string]::IsNullOrWhiteSpace([string]$data.version)) { throw "status_schema_error" }
            return [pscustomobject]@{
                status_schema = "legacy"
                readiness_proof = "profile"
                profile_allowed = $true
            }
        }

        throw "status_schema_error"
    } catch {
        if ($_.Exception.Message -in @("ltaoo_not_ready", "status_schema_error")) { throw }
        throw "status_schema_error"
    }
}

function ConvertTo-NonNegativeInt64 {
    param([object]$Value)
    $integralTypes = @([byte], [sbyte], [int16], [uint16], [int32], [uint32], [int64], [uint64])
    if ($null -eq $Value -or -not ($integralTypes -contains $Value.GetType())) { return $null }
    $text = [Convert]::ToString($Value, [Globalization.CultureInfo]::InvariantCulture)
    if ($text -notmatch '^(0|[1-9][0-9]*)$') { return $null }
    $parsed = 0L
    if (-not [Int64]::TryParse($text, [Globalization.NumberStyles]::None,
            [Globalization.CultureInfo]::InvariantCulture, [ref]$parsed)) { return $null }
    return $parsed
}

function Get-ObjectItems {
    param([object]$Value)
    if ($null -eq $Value) { return @() }
    return @($Value) | Where-Object {
        $null -ne $_ -and $_ -isnot [string] -and $null -ne $_.PSObject -and @($_.PSObject.Properties).Count -gt 0
    }
}

function Get-CommentItems {
    param([object]$Response, [string]$Stage)
    try {
        if (-not (Test-ObjectProperty $Response.body "data") -or
            -not (Test-ObjectProperty $Response.body.data "data") -or
            -not (Test-ObjectProperty $Response.body.data.data "commentInfo")) {
            throw ($Stage + "_schema_error")
        }
        return @(Get-ObjectItems $Response.body.data.data.commentInfo)
    } catch {
        if ($_.Exception.Message -eq ($Stage + "_schema_error")) { throw }
        throw ($Stage + "_schema_error")
    }
}

function Get-LastBuffer {
    param([object]$Response, [string]$Stage)
    try {
        if (-not (Test-ObjectProperty $Response.body.data.data "lastBuffer")) { throw ($Stage + "_schema_error") }
        return [string]$Response.body.data.data.lastBuffer
    } catch {
        if ($_.Exception.Message -eq ($Stage + "_schema_error")) { throw }
        throw ($Stage + "_schema_error")
    }
}

function Select-EligibleRoot {
    param([object[]]$Comments)
    foreach ($comment in $Comments) {
        if ($null -eq $comment -or -not (Test-ObjectProperty $comment "commentId")) { continue }
        $commentId = [string]$comment.commentId
        if ([string]::IsNullOrWhiteSpace($commentId)) { continue }
        $embeddedValue = if (Test-ObjectProperty $comment "levelTwoComment") { Get-ObjectItems $comment.levelTwoComment } else { $null }
        [object[]]$embedded = @()
        if ($null -ne $embeddedValue) { $embedded = @($embeddedValue) }
        $expand = if (Test-ObjectProperty $comment "expandCommentCount") { ConvertTo-NonNegativeInt64 $comment.expandCommentCount } else { $null }
        if ($null -ne $expand -and $expand -gt $embedded.Count) {
            return [pscustomobject]@{ comment = $comment; comment_id = $commentId; expand_count = $expand; embedded = $embedded }
        }
    }
    return $null
}

function Assert-CommentRequestBudget {
    param(
        [int]$CommentCount,
        [int]$TopCount,
        [int]$ReplyCount,
        [Parameter(Mandatory = $true)][ValidateSet("top", "reply")][string]$Kind
    )
    if ($CommentCount -ge 3 -or
        ($Kind -eq "top" -and $TopCount -ge 1) -or
        ($Kind -eq "reply" -and $ReplyCount -ge 2)) {
        throw "comment_request_limit_exceeded"
    }
}

function Get-RelationValue {
    param([object]$Reply, [string]$Name)
    $property = $Reply.PSObject.Properties[$Name]
    if ($null -eq $property) { return "" }
    return [string]$property.Value
}

function Get-ImmediateRelationSafety {
    param([object[]]$Replies, [string]$RootId)
    $rootMismatch = 0
    $parentSelf = 0
    foreach ($reply in $Replies) {
        $replyId = Get-RelationValue $reply "commentId"
        $sourceRootId = Get-RelationValue $reply "rootCommentId"
        $parentId = Get-RelationValue $reply "replyCommentId"
        if (-not [string]::IsNullOrEmpty($sourceRootId) -and $sourceRootId -ne "0" -and $sourceRootId -cne $RootId) {
            $rootMismatch++
        }
        if (-not [string]::IsNullOrEmpty($replyId) -and $parentId -ceq $replyId) {
            $parentSelf++
        }
    }
    return [pscustomobject]@{ root_mismatch = $rootMismatch; parent_self = $parentSelf }
}

function Get-ReplyRelationEvidence {
    param(
        [object[]]$Replies,
        [string]$RootId,
        [Collections.Generic.HashSet[string]]$KnownReplyIds
    )
    $evidence = [ordered]@{
        root_match = 0
        root_gap = 0
        root_mismatch = 0
        parent_root = 0
        parent_known = 0
        parent_unresolved = 0
        parent_gap = 0
        parent_self = 0
    }
    foreach ($reply in $Replies) {
        $replyId = Get-RelationValue $reply "commentId"
        $sourceRootId = Get-RelationValue $reply "rootCommentId"
        $parentId = Get-RelationValue $reply "replyCommentId"

        if ([string]::IsNullOrEmpty($sourceRootId) -or $sourceRootId -eq "0") {
            $evidence.root_gap++
        } elseif ($sourceRootId -ceq $RootId) {
            $evidence.root_match++
        } else {
            $evidence.root_mismatch++
        }

        if ([string]::IsNullOrEmpty($parentId) -or $parentId -eq "0") {
            $evidence.parent_gap++
        } elseif (-not [string]::IsNullOrEmpty($replyId) -and $parentId -ceq $replyId) {
            $evidence.parent_self++
        } elseif ($parentId -ceq $RootId) {
            $evidence.parent_root++
        } elseif ($KnownReplyIds.Contains($parentId)) {
            $evidence.parent_known++
        } else {
            $evidence.parent_unresolved++
        }
    }
    return [pscustomobject]$evidence
}

function New-EmbeddedSummary {
    param([object[]]$Replies, [byte[]]$Salt)
    $ids = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $hashes = [Collections.Generic.List[string]]::new()
    $missing = 0
    foreach ($reply in $Replies) {
        $replyId = if (Test-ObjectProperty $reply "commentId") { [string]$reply.commentId } else { "" }
        if ([string]::IsNullOrEmpty($replyId)) {
            $missing++
            continue
        }
        [void]$ids.Add($replyId)
        $hashes.Add((Get-SaltedTextHash $Salt $replyId))
    }
    return [pscustomobject]@{ ids = $ids; hashes = @($hashes); missing = $missing }
}

function New-ReplyPageSummary {
    param(
        [object]$Response,
        [string]$Stage,
        [int]$PageNumber,
        [string]$RootId,
        [byte[]]$Salt,
        [Collections.Generic.HashSet[string]]$EmbeddedIds,
        [Collections.Generic.HashSet[string]]$PriorReplyPageIds
    )
    $replies = @(Get-CommentItems $Response $Stage)
    $marker = Get-LastBuffer $Response $Stage
    $pageIds = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $hashes = [Collections.Generic.List[string]]::new()
    $pageDuplicates = 0
    $crossDuplicates = 0
    $embeddedDuplicates = 0
    $missingIds = 0

    foreach ($reply in $replies) {
        $replyId = if (Test-ObjectProperty $reply "commentId") { [string]$reply.commentId } else { "" }
        if ([string]::IsNullOrEmpty($replyId)) {
            $missingIds++
        } else {
            if (-not $pageIds.Add($replyId)) { $pageDuplicates++ }
            if ($PriorReplyPageIds.Contains($replyId)) { $crossDuplicates++ }
            if ($EmbeddedIds.Contains($replyId)) { $embeddedDuplicates++ }
            $hashes.Add((Get-SaltedTextHash $Salt $replyId))
        }
    }
    foreach ($replyId in $pageIds) { [void]$PriorReplyPageIds.Add($replyId) }

    $summary = [ordered]@{
            page_number = $PageNumber
            http_status = $Response.status_code
            business_code = [int]$Response.body.data.errCode
            reply_count = $replies.Count
            missing_id_count = $missingIds
            reply_id_hashes = @($hashes)
            page_duplicate_count = $pageDuplicates
            cross_reply_page_duplicate_count = $crossDuplicates
            embedded_duplicate_count = $embeddedDuplicates
            root_relation_match_count = 0
            root_relation_gap_count = 0
            root_relation_mismatch_count = 0
            parent_to_root_count = 0
            parent_to_known_reply_count = 0
            parent_unresolved_count = 0
            parent_gap_count = 0
            parent_self_reference_count = 0
            relation_match_count = 0
            relation_gap_count = 0
            relation_mismatch_count = 0
            last_buffer_present = -not [string]::IsNullOrEmpty($marker)
            last_buffer_length = $marker.Length
            last_buffer_hash = if ([string]::IsNullOrEmpty($marker)) { "" } else { Get-SaltedTextHash $Salt $marker }
    }
    return [pscustomobject]@{
        summary = $summary
        replies = $replies
        ids = $pageIds
        marker = $marker
    }
}

function Set-ReplyRelationEvidence {
    param(
        [object]$Observation,
        [string]$RootId,
        [Collections.Generic.HashSet[string]]$KnownReplyIds
    )
    $evidence = Get-ReplyRelationEvidence $Observation.replies $RootId $KnownReplyIds
    $summary = $Observation.summary
    $summary.root_relation_match_count = $evidence.root_match
    $summary.root_relation_gap_count = $evidence.root_gap
    $summary.root_relation_mismatch_count = $evidence.root_mismatch
    $summary.parent_to_root_count = $evidence.parent_root
    $summary.parent_to_known_reply_count = $evidence.parent_known
    $summary.parent_unresolved_count = $evidence.parent_unresolved
    $summary.parent_gap_count = $evidence.parent_gap
    $summary.parent_self_reference_count = $evidence.parent_self
    $summary.relation_match_count = $evidence.root_match + $evidence.parent_root + $evidence.parent_known
    $summary.relation_gap_count = $evidence.root_gap + $evidence.parent_gap + $evidence.parent_unresolved
    $summary.relation_mismatch_count = $evidence.root_mismatch + $evidence.parent_self
}

function New-ReplyTotals {
    return [pscustomobject][ordered]@{
        reply_count = 0
        missing_id_count = 0
        page_duplicate_count = 0
        cross_reply_page_duplicate_count = 0
        embedded_duplicate_count = 0
        root_relation_match_count = 0
        root_relation_gap_count = 0
        root_relation_mismatch_count = 0
        parent_to_root_count = 0
        parent_to_known_reply_count = 0
        parent_unresolved_count = 0
        parent_gap_count = 0
        parent_self_reference_count = 0
        relation_match_count = 0
        relation_gap_count = 0
        relation_mismatch_count = 0
    }
}

function Add-ReplyPageTotals {
    param([object]$Totals, [object]$Page)
    $Totals.reply_count += [int]$Page.reply_count
    $Totals.missing_id_count += [int]$Page.missing_id_count
    $Totals.page_duplicate_count += [int]$Page.page_duplicate_count
    $Totals.cross_reply_page_duplicate_count += [int]$Page.cross_reply_page_duplicate_count
    $Totals.embedded_duplicate_count += [int]$Page.embedded_duplicate_count
    $Totals.root_relation_match_count += [int]$Page.root_relation_match_count
    $Totals.root_relation_gap_count += [int]$Page.root_relation_gap_count
    $Totals.root_relation_mismatch_count += [int]$Page.root_relation_mismatch_count
    $Totals.parent_to_root_count += [int]$Page.parent_to_root_count
    $Totals.parent_to_known_reply_count += [int]$Page.parent_to_known_reply_count
    $Totals.parent_unresolved_count += [int]$Page.parent_unresolved_count
    $Totals.parent_gap_count += [int]$Page.parent_gap_count
    $Totals.parent_self_reference_count += [int]$Page.parent_self_reference_count
    $Totals.relation_match_count += [int]$Page.relation_match_count
    $Totals.relation_gap_count += [int]$Page.relation_gap_count
    $Totals.relation_mismatch_count += [int]$Page.relation_mismatch_count
}

function Get-ReplyEvidenceState {
    param(
        [Collections.Generic.List[object]]$Observations,
        [string]$RootId,
        [Collections.Generic.HashSet[string]]$KnownReplyIds
    )
    $pages = [Collections.Generic.List[object]]::new()
    $totals = New-ReplyTotals
    foreach ($observation in $Observations) {
        Set-ReplyRelationEvidence $observation $RootId $KnownReplyIds
        [void]$pages.Add($observation.summary)
        Add-ReplyPageTotals $totals $observation.summary
    }
    return [pscustomobject]@{ pages = $pages; totals = $totals }
}

$repoRootValue = $null
$runRoot = $null
$summaryPath = $null
$client = $null
$statusSchema = ""
$readinessProof = ""
$reasonCode = "unexpected_failure"
$currentStage = "startup"
$commentRequestCount = 0
$topLevelRequestCount = 0
$replyRequestCount = 0
$cursorContinuity = $false
$secondReplyRequestCursorHash = ""
$topPageSummary = $null
$selectedRootSummary = $null
$status = $null
$profile = $null
$topPage = $null
$replyPageOne = $null
$replyPageTwo = $null
$replyPages = [Collections.Generic.List[object]]::new()
$replyObservations = [Collections.Generic.List[object]]::new()
$knownReplyIds = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
$totals = New-ReplyTotals

try {
    $currentStage = "manifest"
    $repoRootValue = Resolve-ProbeRepoRoot $RepoRoot
    $runRoot = Resolve-RunRoot $repoRootValue $RunId
    $manifestPath = Join-Path $runRoot "manifest.json"
    $summaryPath = Join-Path $runRoot "reply-probe-summary.json"
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
    $statusEvidence = Get-LtaooStatusEvidence $status
    $statusSchema = [string]$statusEvidence.status_schema
    $readinessProof = [string]$statusEvidence.readiness_proof
    if (-not [bool]$statusEvidence.profile_allowed) { throw "ltaoo_not_ready" }

    $currentStage = "profile"
    $profileUrl = $apiBaseValue + "/api/channels/feed/profile?url=" + [Uri]::EscapeDataString($ShareUrl)
    $profile = Invoke-LocalJsonGet $client $profileUrl "profile"
    Assert-BusinessSuccess $profile "profile"
    try {
        $oid = [string]$profile.body.data.data.object.id
        $nid = [string]$profile.body.data.data.object.objectNonceId
    } catch { throw "profile_schema_error" }
    if ([string]::IsNullOrEmpty($oid) -or [string]::IsNullOrEmpty($nid)) { throw "profile_schema_error" }

    $salt = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($salt) } finally { $rng.Dispose() }

    $currentStage = "top_page"
    $topPageUrl = $apiBaseValue + "/api/channels/feed/comment/list?oid=" + [Uri]::EscapeDataString($oid) + "&nid=" + [Uri]::EscapeDataString($nid)
    Assert-CommentRequestBudget $commentRequestCount $topLevelRequestCount $replyRequestCount "top"
    $commentRequestCount++
    $topLevelRequestCount++
    $topPage = Invoke-LocalJsonGet $client $topPageUrl "top_page"
    Assert-BusinessSuccess $topPage "top_page"
    $topComments = @(Get-CommentItems $topPage "top_page")
    $root = Select-EligibleRoot $topComments
    $topPageSummary = [ordered]@{
        http_status = $topPage.status_code
        business_code = [int]$topPage.body.data.errCode
        comment_count = $topComments.Count
        eligible_root_found = $null -ne $root
    }

    $finalStatus = "inconclusive_no_eligible_root"
    $reasonCode = "top_page_has_no_eligible_root"

    if ($null -ne $root) {
        $embedded = New-EmbeddedSummary $root.embedded $salt
        $selectedRootSummary = [ordered]@{
            comment_id_hash = Get-SaltedTextHash $salt $root.comment_id
            expand_comment_count = [Int64]$root.expand_count
            embedded_reply_count = $root.embedded.Count
            embedded_missing_id_count = $embedded.missing
            embedded_reply_id_hashes = @($embedded.hashes)
        }
        $priorReplyPageIds = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
        foreach ($embeddedId in $embedded.ids) { [void]$knownReplyIds.Add($embeddedId) }
        $replyBaseUrl = $topPageUrl + "&comment_id=" + [Uri]::EscapeDataString($root.comment_id)

        $currentStage = "reply_page_one"
        Assert-CommentRequestBudget $commentRequestCount $topLevelRequestCount $replyRequestCount "reply"
        $commentRequestCount++
        $replyRequestCount++
        $replyPageOne = Invoke-LocalJsonGet $client $replyBaseUrl "reply_page_one"
        Assert-BusinessSuccess $replyPageOne "reply_page_one"
        $replyPageOneResult = New-ReplyPageSummary $replyPageOne "reply_page_one" 1 $root.comment_id $salt $embedded.ids $priorReplyPageIds
        [void]$replyObservations.Add($replyPageOneResult)
        foreach ($replyId in $replyPageOneResult.ids) { [void]$knownReplyIds.Add($replyId) }
        $evidenceState = Get-ReplyEvidenceState $replyObservations $root.comment_id $knownReplyIds
        $replyPages = $evidenceState.pages
        $totals = $evidenceState.totals
        $relationSafety = Get-ImmediateRelationSafety $replyPageOneResult.replies $root.comment_id
        if ($relationSafety.root_mismatch -gt 0) { throw "reply_root_relation_mismatch" }
        if ($relationSafety.parent_self -gt 0) { throw "reply_parent_self_reference" }

        $replyMarker = [string]$replyPageOneResult.marker
        $finalStatus = "inconclusive_no_second_reply_page"
        $reasonCode = "reply_page_one_has_no_marker"

        if (-not [string]::IsNullOrEmpty($replyMarker)) {
            $currentStage = "reply_page_two"
            $secondReplyRequestCursorHash = Get-SaltedTextHash $salt $replyMarker
            $replyPageTwoUrl = $replyBaseUrl + "&next_marker=" + [Uri]::EscapeDataString($replyMarker)
            Assert-CommentRequestBudget $commentRequestCount $topLevelRequestCount $replyRequestCount "reply"
            $commentRequestCount++
            $replyRequestCount++
            $replyPageTwo = Invoke-LocalJsonGet $client $replyPageTwoUrl "reply_page_two"
            Assert-BusinessSuccess $replyPageTwo "reply_page_two"
            $replyPageTwoResult = New-ReplyPageSummary $replyPageTwo "reply_page_two" 2 $root.comment_id $salt $embedded.ids $priorReplyPageIds
            [void]$replyObservations.Add($replyPageTwoResult)
            foreach ($replyId in $replyPageTwoResult.ids) { [void]$knownReplyIds.Add($replyId) }
            $evidenceState = Get-ReplyEvidenceState $replyObservations $root.comment_id $knownReplyIds
            $replyPages = $evidenceState.pages
            $totals = $evidenceState.totals
            $relationSafety = Get-ImmediateRelationSafety $replyPageTwoResult.replies $root.comment_id
            if ($relationSafety.root_mismatch -gt 0) { throw "reply_root_relation_mismatch" }
            if ($relationSafety.parent_self -gt 0) { throw "reply_parent_self_reference" }
            $cursorContinuity = $secondReplyRequestCursorHash -eq [string]$replyPageOneResult.summary.last_buffer_hash
            if (-not $cursorContinuity) { throw "reply_cursor_continuity_failed" }
            $finalStatus = "verified_reply_two_pages"
            $reasonCode = "reply_two_pages_verified"
        }
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
        status_schema = $statusSchema
        readiness_proof = $readinessProof
        profile_http = $profile.status_code
        profile_business_code = [int]$profile.body.data.errCode
        top_page = $topPageSummary
        selected_root = $selectedRootSummary
        reply_pages = @($replyPages)
        totals = $totals
        top_level_request_count = $topLevelRequestCount
        reply_request_count = $replyRequestCount
        comment_request_count = $commentRequestCount
        second_reply_request_cursor_hash = $secondReplyRequestCursorHash
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
        "profile_request_failed", "profile_business_error", "top_page_http_error", "top_page_schema_error", "top_page_request_failed",
        "top_page_business_error", "reply_page_one_http_error", "reply_page_one_schema_error", "reply_page_one_request_failed",
        "reply_page_one_business_error", "reply_page_two_http_error", "reply_page_two_schema_error", "reply_page_two_request_failed",
        "reply_page_two_business_error", "reply_root_relation_mismatch", "reply_parent_self_reference",
        "reply_cursor_continuity_failed", "comment_request_limit_exceeded"
    )
    if ($_.Exception.Message -in $allowed) { $reasonCode = $_.Exception.Message } else { $reasonCode = "unexpected_" + $currentStage }
    if ($null -ne $summaryPath -and $null -ne $runRoot -and [IO.Directory]::Exists($runRoot)) {
        $failure = [ordered]@{
            schema_version = 1
            run_id = $RunId
            completed_at = [DateTimeOffset]::UtcNow.ToString("o")
            share_url_sha256 = Get-TextHash $ShareUrl
            top_level_request_count = $topLevelRequestCount
            reply_request_count = $replyRequestCount
            comment_request_count = $commentRequestCount
            cursor_continuity = $false
            status = "failed"
            reason_code = $reasonCode
        }
        if (-not [string]::IsNullOrEmpty($statusSchema)) { $failure["status_schema"] = $statusSchema }
        if (-not [string]::IsNullOrEmpty($readinessProof)) { $failure["readiness_proof"] = $readinessProof }
        if ($null -ne $status) { $failure["status_http"] = $status.status_code }
        if ($null -ne $profile) {
            $failure["profile_http"] = $profile.status_code
            if (Test-ObjectProperty $profile.body "data" -and Test-ObjectProperty $profile.body.data "errCode") {
                $failure["profile_business_code"] = [int]$profile.body.data.errCode
            }
        }
        if ($null -ne $topPage) {
            $failure["top_page_http"] = $topPage.status_code
            if (Test-ObjectProperty $topPage.body "data" -and Test-ObjectProperty $topPage.body.data "errCode") {
                $failure["top_page_business_code"] = [int]$topPage.body.data.errCode
            }
        }
        if ($null -ne $replyPageOne) {
            $failure["reply_page_one_http"] = $replyPageOne.status_code
            if (Test-ObjectProperty $replyPageOne.body "data" -and Test-ObjectProperty $replyPageOne.body.data "errCode") {
                $failure["reply_page_one_business_code"] = [int]$replyPageOne.body.data.errCode
            }
        }
        if ($null -ne $replyPageTwo) {
            $failure["reply_page_two_http"] = $replyPageTwo.status_code
            if (Test-ObjectProperty $replyPageTwo.body "data" -and Test-ObjectProperty $replyPageTwo.body.data "errCode") {
                $failure["reply_page_two_business_code"] = [int]$replyPageTwo.body.data.errCode
            }
        }
        if ($null -ne $topPageSummary) { $failure["top_page"] = $topPageSummary }
        if ($null -ne $selectedRootSummary) { $failure["selected_root"] = $selectedRootSummary }
        if ($replyPages.Count -gt 0) { $failure["reply_pages"] = @($replyPages); $failure["totals"] = $totals }
        Write-JsonAtomic $failure $summaryPath
    }
    [pscustomobject]@{ run_id = $RunId; status = "failed"; reason_code = $reasonCode } | ConvertTo-Json -Compress
    exit 1
} finally {
    if ($null -ne $client) { $client.Dispose() }
}
