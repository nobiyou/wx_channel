# ltaoo Reply Two-Page Probe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a bounded, redacted ltaoo feasibility probe that fetches one top-level comment page, chooses the first eligible root comment, and fetches at most two reply pages while proving cursor continuity, duplicate behavior, and explicit reply/root relationship semantics.

**Architecture:** Add an independent PowerShell entry point, `scripts/probe-ltaoo-replies.ps1`, so the already-verified top-level probe remains unchanged. Reuse the existing manifest, loopback-only API, modern/legacy status evidence, hashing, atomic-summary, preparation, and cleanup contracts. Drive the new script with loopback `httptest.Server` fixtures from the existing Windows-only Go test suite, then perform one separately approved live run against the pinned audited ltaoo commit.

**Tech Stack:** Windows PowerShell 5.1, Go 1.26 test harness, `net/http/httptest`, ltaoo local HTTP API, SHA-256, Git worktree.

---

## Fixed contracts

The implementation must use these exact contracts throughout the tasks below.

### Request budget

| Request | Maximum | Counter |
|---|---:|---|
| `/api/status` | 1 | not part of comment budget |
| `/api/channels/feed/profile` | 1 | not part of comment budget |
| top-level `/api/channels/feed/comment/list` | 1 | `top_level_request_count` |
| reply `/api/channels/feed/comment/list` | 2 | `reply_request_count` |
| all comment-list requests | 3 | `comment_request_count` |

Increment each comment counter immediately before calling `GetAsync`. Never retry, request top-level page two, select a second root, or request reply page three.

### Result document

Write only `.tmp_runtime/ltaoo-probe/<run_id>/reply-probe-summary.json`; do not overwrite `probe-summary.json`. The success/inconclusive document has this stable shape:

```json
{
  "schema_version": 1,
  "run_id": "safe-run-id",
  "completed_at": "2026-08-13T00:00:00.0000000+00:00",
  "share_url_sha256": "64 lowercase hex",
  "oid_sha256": "64 lowercase hex",
  "nid_sha256": "64 lowercase hex",
  "status_http": 200,
  "status_schema": "modern",
  "readiness_proof": "listeners_and_profile",
  "profile_http": 200,
  "profile_business_code": 0,
  "top_page": {
    "http_status": 200,
    "business_code": 0,
    "comment_count": 4,
    "eligible_root_found": true
  },
  "selected_root": {
    "comment_id_hash": "salted 64 lowercase hex",
    "expand_comment_count": 6,
    "embedded_reply_count": 2,
    "embedded_missing_id_count": 0,
    "embedded_reply_id_hashes": ["salted 64 lowercase hex"]
  },
  "reply_pages": [
    {
      "page_number": 1,
      "http_status": 200,
      "business_code": 0,
      "reply_count": 3,
      "missing_id_count": 0,
      "reply_id_hashes": ["salted 64 lowercase hex"],
      "page_duplicate_count": 1,
      "cross_reply_page_duplicate_count": 0,
      "embedded_duplicate_count": 1,
      "relation_match_count": 2,
      "relation_gap_count": 1,
      "relation_mismatch_count": 0,
      "last_buffer_present": true,
      "last_buffer_length": 24,
      "last_buffer_hash": "salted 64 lowercase hex"
    }
  ],
  "totals": {
    "reply_count": 5,
    "missing_id_count": 1,
    "page_duplicate_count": 1,
    "cross_reply_page_duplicate_count": 1,
    "embedded_duplicate_count": 1,
    "relation_match_count": 5,
    "relation_gap_count": 2,
    "relation_mismatch_count": 0
  },
  "top_level_request_count": 1,
  "reply_request_count": 2,
  "comment_request_count": 3,
  "second_reply_request_cursor_hash": "salted 64 lowercase hex",
  "cursor_continuity": true,
  "status": "verified_reply_two_pages",
  "reason_code": "reply_two_pages_verified"
}
```

For `inconclusive_no_eligible_root`, set `selected_root` to `null`, `reply_pages` to `[]`, both reply counters to zero, and use reason `top_page_has_no_eligible_root`. For `inconclusive_no_second_reply_page`, include root and reply page one, use reason `reply_page_one_has_no_marker`, and leave `second_reply_request_cursor_hash` empty with `cursor_continuity=false`.

A failure document may omit fields that were not yet available, but must always contain `schema_version`, `run_id`, `completed_at`, `share_url_sha256`, all three request counters, `cursor_continuity=false`, `status=failed`, and a stable `reason_code`. Add `status_schema` and `readiness_proof` once known. Never include raw URLs, `oid/nid`, comment/reply IDs, cursors, content, nickname/account/avatar fields, request headers, cookies, proxy credentials, or raw response bodies.

### Stable failure reasons

Use the existing manifest/status/profile reason codes plus:

```text
top_page_http_error
top_page_schema_error
top_page_request_failed
top_page_business_error
reply_page_one_http_error
reply_page_one_schema_error
reply_page_one_request_failed
reply_page_one_business_error
reply_page_two_http_error
reply_page_two_schema_error
reply_page_two_request_failed
reply_page_two_business_error
reply_relation_mismatch
reply_cursor_continuity_failed
comment_request_limit_exceeded
```

## Task 1: Register the new tracked script and make its absence fail

**Files:**

- Modify: `.gitignore:164-176`
- Modify: `scripts/ltaoo_probe_script_test.go:83-93`
- Create: `scripts/probe-ltaoo-replies.ps1`

- [ ] Add the new negation immediately after the existing top-level probe entry:

```gitignore
!scripts/probe-ltaoo-comments.ps1
!scripts/probe-ltaoo-replies.ps1
!scripts/cleanup-ltaoo-probe.ps1
```

- [ ] Add `probe-ltaoo-replies.ps1` to `TestLtaooProbeScriptsExist` and `TestLtaooProbeScriptsParseInWindowsPowerShell`.

- [ ] Run the existence test before creating the file and confirm RED:

```powershell
go test ./scripts -run TestLtaooProbeScriptsExist -v -count=1
```

Expected: failure naming `probe-ltaoo-replies.ps1`.

- [ ] Create a parseable placeholder that fails closed and leaks no input:

```powershell
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9-]{1,80}$')][string]$RunId,
    [Parameter(Mandatory = $true)][string]$ShareUrl,
    [string]$RepoRoot = "",
    [string]$ApiBase = ""
)
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
[pscustomobject]@{ run_id = $RunId; status = "failed"; reason_code = "not_implemented" } | ConvertTo-Json -Compress
exit 1
```

- [ ] Run the existence and parser tests and confirm GREEN:

```powershell
go test ./scripts -run 'TestLtaooProbeScriptsExist|TestLtaooProbeScriptsParseInWindowsPowerShell' -v -count=1
```

Expected: both pass.

- [ ] Commit the surface contract:

```powershell
git add .gitignore scripts/probe-ltaoo-replies.ps1 scripts/ltaoo_probe_script_test.go
git commit -m "test: register ltaoo reply probe"
```

## Task 2: Specify the happy path, root selection, redaction, and request ceiling

**Files:**

- Modify: `scripts/ltaoo_probe_script_test.go` after `TestProbeFetchesExactlyTwoPagesAndRedacts`

- [ ] Add a test named `TestReplyProbeSelectsFirstEligibleRootAndFetchesExactlyTwoReplyPages`. Its fixture must return top-level comments in this order:

```go
topBody := fmt.Sprintf(`{
  "code":0,"data":{"errCode":0,"data":{"commentInfo":[
    {"commentId":"","expandCommentCount":9,"levelTwoComment":[]},
    {"commentId":"fully-embedded-root","expandCommentCount":1,"levelTwoComment":[{"commentId":"embedded-full"}]},
    {"commentId":"invalid-count-root","expandCommentCount":"NaN","levelTwoComment":[]},
    {"commentId":%q,"expandCommentCount":6,"levelTwoComment":[
      {"commentId":%q,"content":%q},
      {"commentId":%q,"nickname":%q}
    ]},
    {"commentId":"later-eligible-root","expandCommentCount":99,"levelTwoComment":[]}
  ],"lastBuffer":"TOP_LEVEL_MARKER_MUST_NOT_BE_USED"}}}`, rootID, embeddedOne, bait, embeddedTwo, bait)
```

The server must reject any comment request whose `oid` or `nid` differs, any top-level request containing `next_marker`, any reply request for a root other than `rootID`, and any fourth comment request.

- [ ] Make reply page one contain:

```go
replyOne := fmt.Sprintf(`{
  "code":0,"data":{"errCode":0,"data":{"commentInfo":[
    {"commentId":%q,"replyCommentId":%q,"rootCommentId":%q,"content":%q},
    {"commentId":"page-one-unique","replyCommentId":"0","rootCommentId":%q},
    {"commentId":"page-one-unique","replyCommentId":%q},
    {"commentId":"","replyCommentId":%q,"rootCommentId":%q}
  ],"lastBuffer":%q}}}`, embeddedOne, rootID, rootID, bait, rootID, rootID, rootID, rootID, marker)
```

This yields one embedded duplicate, one within-page duplicate, one missing reply ID, explicit matches, and relation gaps caused only by absent/`"0"` fields.

- [ ] Make reply page two contain a cross-page duplicate and a unique reply, with a non-empty second-page marker that must never trigger a third page:

```go
replyTwo := fmt.Sprintf(`{
  "code":0,"data":{"errCode":0,"data":{"commentInfo":[
    {"commentId":"page-one-unique","replyCommentId":%q,"rootCommentId":%q},
    {"commentId":"page-two-unique","replyCommentId":%q,"rootCommentId":%q}
  ],"lastBuffer":"DO_NOT_REQUEST_REPLY_PAGE_THREE"}}}`, rootID, rootID, rootID, rootID)
```

- [ ] Use `marker := "reply+cursor/with=reserved"` and assert the server receives exactly the original marker from `r.URL.Query().Get("next_marker")`. This proves standard URL encoding without changing the decoded value.

- [ ] Assert all of these postconditions:

```go
if summary.Status != "verified_reply_two_pages" || summary.ReasonCode != "reply_two_pages_verified" {
    t.Fatalf("unexpected result: %+v", summary)
}
if summary.TopLevelRequestCount != 1 || summary.ReplyRequestCount != 2 || summary.CommentRequestCount != 3 {
    t.Fatalf("unexpected budgets: %+v", summary)
}
if !summary.CursorContinuity || summary.SecondReplyRequestCursorHash != summary.ReplyPages[0].LastBufferHash {
    t.Fatalf("cursor continuity not proven: %+v", summary)
}
if summary.SelectedRoot.ExpandCommentCount != 6 || summary.SelectedRoot.EmbeddedReplyCount != 2 {
    t.Fatalf("wrong root selected: %+v", summary.SelectedRoot)
}
if summary.ReplyPages[0].PageDuplicateCount != 1 || summary.ReplyPages[0].EmbeddedDuplicateCount != 1 ||
   summary.ReplyPages[1].CrossReplyPageDuplicateCount != 1 {
    t.Fatalf("wrong duplicate evidence: %+v", summary.ReplyPages)
}
if commentRequests != 3 || replyRequests != 2 {
    t.Fatalf("server observed comment=%d reply=%d", commentRequests, replyRequests)
}
```

- [ ] Inspect both `reply-probe-summary.json` and combined terminal output. Assert neither contains any member of:

```go
[]string{shareURL, oid, nid, rootID, embeddedOne, embeddedTwo, marker, bait,
    "fully-embedded-root", "invalid-count-root", "later-eligible-root",
    "page-one-unique", "page-two-unique", "TOP_LEVEL_MARKER_MUST_NOT_BE_USED",
    "DO_NOT_REQUEST_REPLY_PAGE_THREE"}
```

- [ ] Run the new test and confirm RED because the placeholder returns `not_implemented`:

```powershell
go test ./scripts -run TestReplyProbeSelectsFirstEligibleRootAndFetchesExactlyTwoReplyPages -v -count=1
```

Expected: failure from the reply probe process.

## Task 3: Implement the bounded happy path

**Files:**

- Modify: `scripts/probe-ltaoo-replies.ps1`

- [ ] Replace the placeholder with the parameter block and these helpers copied behavior-for-behavior from `probe-ltaoo-comments.ps1`: `Resolve-ProbeRepoRoot`, `Resolve-RunRoot`, `Assert-LoopbackApiBase`, `Get-Sha256Hex`, `Get-TextHash`, `Get-SaltedTextHash`, `Write-JsonAtomic`, `Invoke-LocalJsonGet`, `Assert-BusinessSuccess`, `Test-ObjectProperty`, and `Get-LtaooStatusEvidence`. Do not create a shared module in this slice.

- [ ] Add a strict nonnegative-integer converter. It must reject booleans, fractional values, negative values, exponent/floating spellings, leading signs, and overflow:

```powershell
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
```

- [ ] Count only object-like embedded replies and select the first eligible root in source order:

```powershell
function Get-ObjectItems {
    param([object]$Value)
    if ($null -eq $Value) { return @() }
    return @($Value) | Where-Object { $null -ne $_ -and $_ -isnot [string] -and $_.PSObject.Properties.Count -gt 0 }
}

function Select-EligibleRoot {
    param([object[]]$Comments)
    foreach ($comment in $Comments) {
        if ($null -eq $comment -or -not (Test-ObjectProperty $comment "commentId")) { continue }
        $commentId = [string]$comment.commentId
        if ([string]::IsNullOrWhiteSpace($commentId)) { continue }
        $embedded = if (Test-ObjectProperty $comment "levelTwoComment") { @(Get-ObjectItems $comment.levelTwoComment) } else { @() }
        $expand = if (Test-ObjectProperty $comment "expandCommentCount") { ConvertTo-NonNegativeInt64 $comment.expandCommentCount } else { $null }
        if ($null -ne $expand -and $expand -gt $embedded.Count) {
            return [pscustomobject]@{ comment = $comment; comment_id = $commentId; expand_count = $expand; embedded = $embedded }
        }
    }
    return $null
}
```

- [ ] Add a guard used immediately before every comment request:

```powershell
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
```

The call sites must follow this exact ordering:

```powershell
Assert-CommentRequestBudget $commentRequestCount $topLevelRequestCount $replyRequestCount "top"
$commentRequestCount++
$topLevelRequestCount++
$topPage = Invoke-LocalJsonGet $client $topPageUrl "top_page"
```

and, for each reply request:

```powershell
Assert-CommentRequestBudget $commentRequestCount $topLevelRequestCount $replyRequestCount "reply"
$commentRequestCount++
$replyRequestCount++
$replyPageOne = Invoke-LocalJsonGet $client $replyPageOneUrl "reply_page_one"
```

- [ ] Build URLs only with `Uri.EscapeDataString`:

```powershell
$topPageUrl = $apiBaseValue + "/api/channels/feed/comment/list?oid=" + [Uri]::EscapeDataString($oid) + "&nid=" + [Uri]::EscapeDataString($nid)
$replyBaseUrl = $topPageUrl + "&comment_id=" + [Uri]::EscapeDataString($root.comment_id)
$replyPageTwoUrl = $replyBaseUrl + "&next_marker=" + [Uri]::EscapeDataString($replyMarker)
```

- [ ] Generate a new 32-byte random salt after valid `oid/nid` are obtained. Keep it in memory only. Use unsalted `Get-TextHash` only for share URL, `oid`, and `nid`; use `Get-SaltedTextHash` for root/reply IDs and cursors.

- [ ] Complete the request flow with no loops: status, profile, top page, selection, reply page one, optional reply page two, then stop. A non-empty top-page `lastBuffer` must never be used.

- [ ] Emit the fixed result-document shape. For the happy path set:

```powershell
$finalStatus = "verified_reply_two_pages"
$reasonCode = "reply_two_pages_verified"
$cursorContinuity = $secondReplyRequestCursorHash -eq [string]$replyPages[0].last_buffer_hash
if (-not $cursorContinuity) { throw "reply_cursor_continuity_failed" }
```

- [ ] Run the happy-path test and confirm GREEN:

```powershell
go test ./scripts -run TestReplyProbeSelectsFirstEligibleRootAndFetchesExactlyTwoReplyPages -v -count=1
```

Expected: pass; server observes status + profile + exactly three comment-list requests.

- [ ] Commit the bounded happy path:

```powershell
git add scripts/probe-ltaoo-replies.ps1 scripts/ltaoo_probe_script_test.go
git commit -m "feat: probe two reply pages through ltaoo"
```

## Task 4: Implement duplicate and explicit relation evidence

**Files:**

- Modify: `scripts/probe-ltaoo-replies.ps1`
- Modify: `scripts/ltaoo_probe_script_test.go`

- [ ] Add a `Get-ReplyId` helper that returns an empty string for a missing/null ID and never invents a replacement.

- [ ] Add a relation classifier that evaluates both source fields independently. Missing, empty, or `"0"` contributes one gap; an explicit root match contributes one match; any other explicit value contributes one mismatch:

```powershell
function Get-RelationEvidence {
    param([object]$Reply, [string]$RootId)
    $match = 0; $gap = 0; $mismatch = 0
    foreach ($name in @("replyCommentId", "rootCommentId")) {
        $value = if (Test-ObjectProperty $Reply $name) { [string]$Reply.$name } else { "" }
        if ([string]::IsNullOrEmpty($value) -or $value -eq "0") { $gap++; continue }
        if ($value -ceq $RootId) { $match++ } else { $mismatch++ }
    }
    return [pscustomobject]@{ match = $match; gap = $gap; mismatch = $mismatch }
}
```

- [ ] Build the embedded-ID set once. For every reply page, build a fresh page set plus a persistent prior-reply-page set. For each non-empty ID:

```powershell
if (-not $pageIds.Add($replyId)) { $pageDuplicateCount++ }
if ($priorReplyPageIds.Contains($replyId)) { $crossReplyPageDuplicateCount++ }
if ($embeddedIds.Contains($replyId)) { $embeddedDuplicateCount++ }
[void]$priorReplyPageIds.Add($replyId)
$replyIdHashes.Add((Get-SaltedTextHash $Salt $replyId))
```

For an empty ID, increment only `missing_id_count`; do not add a hash or any set membership.

- [ ] If a page produces any explicit relation mismatch, finish summarizing the observed page, then throw `reply_relation_mismatch` before any subsequent request. The failure summary must report the current request counters; it must not contain raw mismatched values.

- [ ] Extend the happy-path test to assert exact totals from its fixture, including two relation checks per reply object. The expected relationship totals for the Task 2 fixture are:

```text
reply page 1: match=6, gap=2, mismatch=0
reply page 2: match=4, gap=0, mismatch=0
all pages:    match=10, gap=2, mismatch=0
```

- [ ] Add `TestReplyProbeStopsOnExplicitRelationMismatch`. Return a mismatch on reply page one and a non-empty marker; assert process failure, `reason_code=reply_relation_mismatch`, `comment_request_count=2`, `reply_request_count=1`, and no reply-page-two request.

- [ ] Run focused tests:

```powershell
go test ./scripts -run 'TestReplyProbeSelectsFirstEligibleRootAndFetchesExactlyTwoReplyPages|TestReplyProbeStopsOnExplicitRelationMismatch' -v -count=1
```

Expected: both pass.

- [ ] Commit evidence semantics:

```powershell
git add scripts/probe-ltaoo-replies.ps1 scripts/ltaoo_probe_script_test.go
git commit -m "test: verify reply duplicate and relation evidence"
```

## Task 5: Cover bounded inconclusive and early-failure paths

**Files:**

- Modify: `scripts/ltaoo_probe_script_test.go`
- Modify: `scripts/probe-ltaoo-replies.ps1`

- [ ] Add `TestReplyProbeStopsWhenNoEligibleRoot`. The top page must include a missing ID, a fully embedded root, an invalid count, and a negative count. Assert a successful process exit with:

```text
status=inconclusive_no_eligible_root
reason_code=top_page_has_no_eligible_root
top_level_request_count=1
reply_request_count=0
comment_request_count=1
reply_pages length=0
```

- [ ] Add `TestReplyProbeStopsWhenReplyPageOneHasNoMarker`. Assert:

```text
status=inconclusive_no_second_reply_page
reason_code=reply_page_one_has_no_marker
top_level_request_count=1
reply_request_count=1
comment_request_count=2
cursor_continuity=false
```

- [ ] Add `TestReplyProbeProfileFailureDoesNotRequestComments` using the legacy status fixture. Assert profile failure, zero comment counters, and only status/profile observed by the server.

- [ ] Add `TestReplyProbeTopPageFailureDoesNotRequestReplies`. Return a top-page business error and assert `top_page_business_error`, top count one, reply count zero, comment count one.

- [ ] Add `TestReplyProbeRejectsRemoteAPIBase` mirroring the existing top-level test, but invoke `probe-ltaoo-replies.ps1` and assert no share-link leak.

- [ ] Add `TestReplyProbeCountsTransportAttemptBeforeFailure`. Point a valid manifest at a loopback server that closes the connection on the top-page request. Assert `top_page_request_failed` and counters `1/0/1`; this is the regression test for increment-before-dispatch.

- [ ] Add a source-surface test `TestReplyProbeHasHardRequestCeiling` requiring all of these strings:

```go
[]string{
    "Assert-CommentRequestBudget", "$CommentCount -ge 3",
    "$TopCount -ge 1", "$ReplyCount -ge 2",
    "comment_request_limit_exceeded", "reply-probe-summary.json",
}
```

and rejecting `while (` and `do {` case-insensitively. A `foreach` used only for finite local arrays is allowed.

- [ ] Run the reply-probe suite:

```powershell
go test ./scripts -run 'TestReplyProbe' -v -count=1
```

Expected: all reply tests pass.

- [ ] Commit bounded failures:

```powershell
git add scripts/probe-ltaoo-replies.ps1 scripts/ltaoo_probe_script_test.go
git commit -m "test: enforce reply probe stop conditions"
```

## Task 6: Document the operator procedure and safety gates

**Files:**

- Create: `docs/LTAOO_REPLY_TWO_PAGE_PROBE.md`
- Modify: `scripts/ltaoo_probe_script_test.go:521-537`

- [ ] Add `TestLtaooReplyProbeRunbookHasSafetySequence`, requiring the runbook to contain:

```go
[]string{
    "prepare-ltaoo-probe.ps1", "probe-ltaoo-replies.ps1", "cleanup-ltaoo-probe.ps1",
    "INSTALL", "CurrentUser\\Root", "reply-probe-summary.json",
    "verified_reply_two_pages", "inconclusive_no_eligible_root",
    "inconclusive_no_second_reply_page", "reply_relation_mismatch",
    "comment_request_count", "reply_request_count", "cleanup_success",
    "Clash", "system: false", "tun: false", "status_schema", "readiness_proof",
}
```

- [ ] Run that test before adding the document and confirm RED:

```powershell
go test ./scripts -run TestLtaooReplyProbeRunbookHasSafetySequence -v -count=1
```

Expected: missing-file failure.

- [ ] Write the runbook with these sections and exact operational rules:

1. Purpose: one top page, one root, at most two reply pages; no full collector.
2. Preconditions: audited source build from `c0c2b8cc36af52ab2c3eb50cb7dc08b7d963efb0`, PC WeChat logged in, public sample with more replies than embedded previews.
3. Preparation: existing prepare script and exact `INSTALL <run_id>` confirmation.
4. Clash: operator-managed reversible backup; ltaoo direct; target traffic to `127.0.0.1:2023`; probe never edits Clash/system proxy/routes/TUN.
5. Execution: read the share URL into a PowerShell variable, call only `probe-ltaoo-replies.ps1`, then null the variable.
6. Result meanings and exact request counts for all three non-failure statuses.
7. Mandatory cleanup in a `finally` block: restore Clash first, clear clipboard, invoke existing cleanup, inspect `cleanup_success`, CA/key/config/process/port absence.
8. Explicit stop rule: do not run the full collector; a failure is diagnosed by stable reason code.

Use this execution example:

```powershell
$shareUrl = Read-Host 'Paste the selected public WeChat Channels share URL'
try {
  & powershell.exe -NoProfile -ExecutionPolicy Bypass `
    -File scripts/probe-ltaoo-replies.ps1 `
    -RunId $run.run_id `
    -ShareUrl $shareUrl
} finally {
  $shareUrl = $null
  Set-Clipboard -Value ''
  & powershell.exe -NoProfile -ExecutionPolicy Bypass `
    -File scripts/cleanup-ltaoo-probe.ps1 `
    -RunId $run.run_id
}
```

State directly above the example that Clash restoration is operator-owned and must occur before cleanup inside the real operational sequence; the snippet does not automate restoration.

- [ ] Run both runbook tests and confirm GREEN:

```powershell
go test ./scripts -run 'TestLtaooProbeRunbookHasSafetySequence|TestLtaooReplyProbeRunbookHasSafetySequence' -v -count=1
```

- [ ] Commit the runbook:

```powershell
git add docs/LTAOO_REPLY_TWO_PAGE_PROBE.md scripts/ltaoo_probe_script_test.go
git commit -m "docs: add ltaoo reply probe runbook"
```

## Task 7: Run the complete automated verification gate

**Files:**

- Verify only; modify files only if a test exposes a defect.

- [ ] Run all script tests from the isolated worktree:

```powershell
go test ./scripts -v -count=1
```

Expected: all existing prepare/top-level/cleanup tests and all new reply tests pass.

- [ ] Parse every probe script explicitly under Windows PowerShell 5.1:

```powershell
$names = @(
  'scripts/prepare-ltaoo-probe.ps1',
  'scripts/probe-ltaoo-comments.ps1',
  'scripts/probe-ltaoo-replies.ps1',
  'scripts/cleanup-ltaoo-probe.ps1'
)
foreach ($name in $names) {
  $tokens = $null
  $errors = $null
  [void][System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path $name), [ref]$tokens, [ref]$errors)
  if ($errors.Count) { throw "$name parse failed: $($errors.Message -join '; ')" }
}
```

Expected: no output and exit code zero.

- [ ] Check formatting, placeholders, and worktree scope:

```powershell
git diff --check HEAD
rg -n "not_implemented" scripts/probe-ltaoo-replies.ps1 docs/LTAOO_REPLY_TWO_PAGE_PROBE.md
git status --short
git diff --stat main...HEAD
```

Expected: `git diff --check` succeeds; `rg` finds nothing; only planned reply-probe files differ from `main`.

- [ ] Confirm automated tests left no real ltaoo or certificate/network state:

```powershell
Get-Process wx_video_download -ErrorAction SilentlyContinue
Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
  Where-Object { $_.LocalPort -in 2022,2023 }
Get-ChildItem Cert:\CurrentUser\Root |
  Where-Object { $_.Subject -like '*ltaoo*' -or $_.FriendlyName -like '*ltaoo*' }
```

Expected: no probe-owned process, listener, or CA.

- [ ] If verification required fixes, commit them separately:

```powershell
git add scripts/probe-ltaoo-replies.ps1 scripts/ltaoo_probe_script_test.go docs/LTAOO_REPLY_TWO_PAGE_PROBE.md .gitignore
git commit -m "fix: harden ltaoo reply probe verification"
```

## Task 8: Perform one separately approved live acceptance run

**Files:**

- Runtime artifacts only under ignored `.tmp_runtime/`; do not commit binaries, CA material, manifests, summaries, Clash backups, links, or logs.

- [ ] Materialize the audited source in this exact ignored directory without deleting or replacing any existing directory:

```powershell
$ltaooCommit = 'c0c2b8cc36af52ab2c3eb50cb7dc08b7d963efb0'
$ltaooSource = Join-Path (Resolve-Path .) ".tmp_runtime\ltaoo-source-$ltaooCommit"
$ltaooBuild = Join-Path (Resolve-Path .) ".tmp_runtime\ltaoo-build-$ltaooCommit"
if (-not (Test-Path -LiteralPath $ltaooSource)) {
  git clone https://github.com/ltaoo/wx_channels_download.git $ltaooSource
  if ($LASTEXITCODE -ne 0) { throw 'ltaoo clone failed' }
}
if (-not (Test-Path -LiteralPath $ltaooBuild)) {
  New-Item -ItemType Directory -Path $ltaooBuild | Out-Null
}
git -C $ltaooSource fetch origin $ltaooCommit
git -C $ltaooSource checkout --detach $ltaooCommit
git -C $ltaooSource rev-parse HEAD
git -C $ltaooSource status --short
```

Expected: `c0c2b8cc36af52ab2c3eb50cb7dc08b7d963efb0` and a clean checkout.

- [ ] Verify dependencies and rebuild the executable from source without VCS stamping:

```powershell
go -C $ltaooSource mod verify
$ltaooExe = Join-Path $ltaooBuild 'wx_video_download.exe'
go -C $ltaooSource build -buildvcs=false -mod=readonly -tags with_gvisor,embed_inject -o $ltaooExe .
Get-FileHash $ltaooExe -Algorithm SHA256
```

Record only commit ID and executable SHA-256 in the non-sensitive acceptance note. Do not commit the executable.

- [ ] Re-run `go test ./scripts -v -count=1` immediately before the live run. Stop if it is not green.

- [ ] Capture a read-only system baseline, run `prepare-ltaoo-probe.ps1`, and pause for the operator's exact `INSTALL <run_id>` confirmation. Do not synthesize or bypass this confirmation.

- [ ] Back up the exact current Clash configuration, add only the approved temporary direct/routing rules, hot-reload, and verify there is no proxy loop. Keep the backup outside Git and tied to this run ID. Do not change Clash TUN, system proxy, WinHTTP proxy, or routes beyond the already approved operational method.

- [ ] Ask the operator to open/refresh one public video page with enough replies and copy its share URL. Read it from the clipboard into process memory only; do not echo it or put it in command history.

- [ ] Execute the reply probe exactly once. Accept only these bounded observations:

```text
verified_reply_two_pages: comment_request_count=3, reply_request_count=2, cursor_continuity=true, relation_mismatch_count=0
inconclusive_no_eligible_root: comment_request_count=1, reply_request_count=0
inconclusive_no_second_reply_page: comment_request_count=2, reply_request_count=1
failed: no new comment request after the recorded failure
```

- [ ] In `finally`, restore the exact Clash backup, clear the clipboard, invoke `cleanup-ltaoo-probe.ps1` with the exact run ID, and inspect `cleanup-receipt.json`.

- [ ] Independently audit that the run-owned CA, private key, certificate file, config, process, ports `2022/2023`, temporary Clash rules, and Clash backup are absent. If any residue remains, the acceptance result is failed regardless of API outcome.

- [ ] Record only: ltaoo commit, EXE SHA-256, summary status/reason, request counts, cursor-continuity boolean, aggregate duplicate/relation counts, and cleanup receipt booleans. Never record raw identifiers or cursors.

## Task 9: Final review and branch handoff

**Files:**

- Review all branch changes.

- [ ] Compare the implementation against every section of `docs/superpowers/specs/2026-08-13-ltaoo-reply-two-page-probe-design.md` and this plan. In particular, verify first-eligible-root ordering, request counters before dispatch, missing-ID semantics, two-field relation semantics, and no third-page path.

- [ ] Run the final evidence commands immediately before claiming completion:

```powershell
go test ./scripts -v -count=1
git diff --check main...HEAD
git status --short --branch
git log --oneline --decorate main..HEAD
```

- [ ] If all checks pass, use `superpowers:finishing-a-development-branch` to present integration choices. Do not push or open a PR unless the user selects that action.

## Completion rule

This slice is complete only when the automated suite is green, the live run ends in one of the three bounded non-failure statuses (preferably `verified_reply_two_pages`), and exact cleanup is independently confirmed. A verified two-page reply result authorizes only the next isolated abnormal/repeated-cursor probe; it does not authorize the full collector. Dedupe behavior and long-list interruption remain separate later slices.
