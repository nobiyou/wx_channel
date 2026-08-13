# ltaoo Reply Relation Semantics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the incorrect “both relationship fields must equal the selected root” rule with separate root and direct-parent semantics while preserving redaction, request ceilings, cursor continuity, and compatibility totals.

**Architecture:** Keep raw relationship IDs only inside `probe-ltaoo-replies.ps1`. Parse each reply page into an in-memory observation, perform root-conflict and parent-self safety checks before another request, then classify direct parents against the union of embedded and bounded reply-page IDs. Emit granular anonymous counters plus the existing compatibility counters.

**Tech Stack:** Windows PowerShell 5.1, Go `testing`/`httptest`, JSON summaries, Git.

---

## File map

- Modify `scripts/ltaoo_probe_script_test.go`: extend summary types and add focused fixtures for nested, cross-page, unresolved, root-conflict, and self-parent behavior.
- Modify `scripts/probe-ltaoo-replies.ps1`: replace the old two-field classifier with immediate safety checks and deferred parent/root aggregation.
- Modify `docs/LTAOO_REPLY_TWO_PAGE_PROBE.md`: document the operator-visible granular counters and new failure codes.
- Modify `docs/superpowers/specs/2026-08-13-ltaoo-reply-two-page-probe-design.md`: mark the original two-field equality rule as superseded by the approved relation-semantics design.
- Modify `docs/superpowers/plans/2026-08-13-ltaoo-reply-two-page-probe.md`: mark the original relation task as superseded; do not rewrite historical implementation steps.

### Task 1: Specify granular and cross-page relationship evidence

**Files:**
- Modify: `scripts/ltaoo_probe_script_test.go:205-235`
- Modify: `scripts/ltaoo_probe_script_test.go:240-345`

- [ ] **Step 1: Extend the test summary types**

Add these fields to both the reply-page and totals structs in `replyProbeSummary`:

```go
RootRelationMatchCount      int `json:"root_relation_match_count"`
RootRelationGapCount        int `json:"root_relation_gap_count"`
RootRelationMismatchCount   int `json:"root_relation_mismatch_count"`
ParentToRootCount           int `json:"parent_to_root_count"`
ParentToKnownReplyCount     int `json:"parent_to_known_reply_count"`
ParentUnresolvedCount       int `json:"parent_unresolved_count"`
ParentGapCount              int `json:"parent_gap_count"`
ParentSelfReferenceCount    int `json:"parent_self_reference_count"`
```

- [ ] **Step 2: Add a cross-page parent fixture to the happy-path test**

Change the two reply responses so page one contains a forward parent reference and an unresolved parent, while page two contains the referenced parent and a backward reference:

```go
case "":
    fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[`+
        `{"commentId":%q,"replyCommentId":%q,"rootCommentId":%q,"content":%q},`+
        `{"commentId":"page-one-child","replyCommentId":"page-two-parent","rootCommentId":%q},`+
        `{"commentId":"page-one-unresolved","replyCommentId":"outside-bounded-window","rootCommentId":"0"},`+
        `{"commentId":"page-one-child","replyCommentId":"0"},`+
        `{"commentId":"","replyCommentId":%q,"rootCommentId":%q}`+
        `],"lastBuffer":%q}}}`,
        embeddedOne, rootID, rootID, bait, rootID, rootID, rootID, marker)
case marker:
    fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[`+
        `{"commentId":"page-two-parent","replyCommentId":%q,"rootCommentId":%q},`+
        `{"commentId":"page-two-child","replyCommentId":"page-one-child","rootCommentId":%q}`+
        `],"lastBuffer":"DO_NOT_REQUEST_REPLY_PAGE_THREE"}}}`,
        rootID, rootID, rootID)
```

Assert the granular totals and compatibility totals:

```go
if got := summary.Totals; got.RootRelationMatchCount != 5 ||
    got.RootRelationGapCount != 2 || got.RootRelationMismatchCount != 0 ||
    got.ParentToRootCount != 3 || got.ParentToKnownReplyCount != 2 ||
    got.ParentUnresolvedCount != 1 || got.ParentGapCount != 1 ||
    got.ParentSelfReferenceCount != 0 || got.RelationMatchCount != 10 ||
    got.RelationGapCount != 4 || got.RelationMismatchCount != 0 {
    t.Fatalf("wrong granular relation evidence: %+v", got)
}
```

Add `"page-two-parent"`, `"outside-bounded-window"`, and all other new raw fixture values to the existing leak scan.

- [ ] **Step 3: Replace the old mismatch test with explicit root-conflict and parent-self contracts**

Rename the old test to `TestReplyProbeStopsOnExplicitRootRelationMismatch`. Its reply must use a valid parent and a conflicting explicit root:

```go
fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[`+
    `{"commentId":"relation-reply-secret","replyCommentId":%q,"rootCommentId":"wrong-root-secret"}`+
    `],"lastBuffer":"must-not-be-used"}}}`, rootID)
```

Assert `reason_code=reply_root_relation_mismatch`, two comment requests, one reply request, no page-two request, `RootRelationMismatchCount=1`, and no secret leakage.

Add `TestReplyProbeStopsOnParentSelfReference` with:

```go
{"commentId":"self-reply-secret","replyCommentId":"self-reply-secret","rootCommentId":"self-root-secret"}
```

Assert `reason_code=reply_parent_self_reference`, `ParentSelfReferenceCount=1`, `RelationMismatchCount=1`, and no page-two request despite a non-empty marker.

- [ ] **Step 4: Add an unresolved-parent continuation contract**

Create `TestReplyProbeAllowsUnresolvedParentAndFetchesSecondPage`. Page one contains an explicit parent ID absent from both bounded pages and a non-empty marker; page two succeeds. Assert `verified_reply_two_pages`, three comment requests, `ParentUnresolvedCount=1`, and `RelationMismatchCount=0`.

- [ ] **Step 5: Run all new relation tests and verify RED**

Run:

```powershell
go test ./scripts -run 'TestReplyProbe(SelectsFirstEligibleRootAndFetchesExactlyTwoReplyPages|StopsOnExplicitRootRelationMismatch|StopsOnParentSelfReference|AllowsUnresolvedParentAndFetchesSecondPage)' -v -count=1
```

Expected: FAIL because the current probe rejects valid non-root parents, lacks the two new failure codes, or emits zero granular counters.

- [ ] **Step 6: Commit the failing contracts**

```powershell
git add scripts/ltaoo_probe_script_test.go
git commit -m "test: specify ltaoo parent relation semantics"
```

### Task 2: Implement immediate safety checks and deferred classification

**Files:**
- Modify: `scripts/probe-ltaoo-replies.ps1:235-345`
- Modify: `scripts/probe-ltaoo-replies.ps1:365-485`

- [ ] **Step 1: Replace `Get-RelationEvidence` with field and safety helpers**

```powershell
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
        $rootId = Get-RelationValue $reply "rootCommentId"
        $parentId = Get-RelationValue $reply "replyCommentId"
        if (-not [string]::IsNullOrEmpty($rootId) -and $rootId -ne "0" -and $rootId -cne $RootId) {
            $rootMismatch++
        }
        if (-not [string]::IsNullOrEmpty($replyId) -and $parentId -ceq $replyId) {
            $parentSelf++
        }
    }
    return [pscustomobject]@{ root_mismatch = $rootMismatch; parent_self = $parentSelf }
}
```

- [ ] **Step 2: Add the deferred granular classifier**

```powershell
function Get-ReplyRelationEvidence {
    param(
        [object[]]$Replies,
        [string]$RootId,
        [Collections.Generic.HashSet[string]]$KnownReplyIds
    )
    $e = [ordered]@{
        root_match = 0; root_gap = 0; root_mismatch = 0
        parent_root = 0; parent_known = 0; parent_unresolved = 0
        parent_gap = 0; parent_self = 0
    }
    foreach ($reply in $Replies) {
        $replyId = Get-RelationValue $reply "commentId"
        $rootId = Get-RelationValue $reply "rootCommentId"
        $parentId = Get-RelationValue $reply "replyCommentId"

        if ([string]::IsNullOrEmpty($rootId) -or $rootId -eq "0") { $e.root_gap++ }
        elseif ($rootId -ceq $RootId) { $e.root_match++ }
        else { $e.root_mismatch++ }

        if ([string]::IsNullOrEmpty($parentId) -or $parentId -eq "0") { $e.parent_gap++ }
        elseif (-not [string]::IsNullOrEmpty($replyId) -and $parentId -ceq $replyId) { $e.parent_self++ }
        elseif ($parentId -ceq $RootId) { $e.parent_root++ }
        elseif ($KnownReplyIds.Contains($parentId)) { $e.parent_known++ }
        else { $e.parent_unresolved++ }
    }
    return [pscustomobject]$e
}
```

- [ ] **Step 3: Preserve raw replies only in an in-memory page observation**

Change `New-ReplyPageSummary` to return `replies = $replies` and remove the old relation loop. Initialize the new and compatibility fields in `summary` to zero:

```powershell
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
```

The returned wrapper must be:

```powershell
return [pscustomobject]@{
    summary = $summary
    replies = $replies
    ids = $pageIds
    marker = $marker
}
```

Initialize `$replyObservations = [Collections.Generic.List[object]]::new()` before the main `try`, and add each observation immediately after its response is parsed. This keeps the current page available to the catch block even when an immediate safety check fails.

- [ ] **Step 4: Add a finalizer that overwrites page counters and recomputes totals**

```powershell
function Set-ReplyRelationEvidence {
    param([object]$Observation, [string]$RootId, [Collections.Generic.HashSet[string]]$KnownReplyIds)
    $e = Get-ReplyRelationEvidence $Observation.replies $RootId $KnownReplyIds
    $s = $Observation.summary
    $s.root_relation_match_count = $e.root_match
    $s.root_relation_gap_count = $e.root_gap
    $s.root_relation_mismatch_count = $e.root_mismatch
    $s.parent_to_root_count = $e.parent_root
    $s.parent_to_known_reply_count = $e.parent_known
    $s.parent_unresolved_count = $e.parent_unresolved
    $s.parent_gap_count = $e.parent_gap
    $s.parent_self_reference_count = $e.parent_self
    $s.relation_match_count = $e.root_match + $e.parent_root + $e.parent_known
    $s.relation_gap_count = $e.root_gap + $e.parent_gap + $e.parent_unresolved
    $s.relation_mismatch_count = $e.root_mismatch + $e.parent_self
}
```

Extend `Add-ReplyPageTotals` and the `$totals` initializer with every granular field. Before recomputing totals, reset all totals to zero and add every page exactly once; do not double-add duplicate counters.

- [ ] **Step 5: Apply safety checks before page two and classify against the final bounded union**

After parsing each reply page, add its IDs to a `KnownReplyIds` set that starts with embedded IDs. Run:

```powershell
$safety = Get-ImmediateRelationSafety $observation.replies $root.comment_id
if ($safety.root_mismatch -gt 0) { throw "reply_root_relation_mismatch" }
if ($safety.parent_self -gt 0) { throw "reply_parent_self_reference" }
```

For page one, this check occurs before dispatching page two. Once all bounded pages have been read, call `Set-ReplyRelationEvidence` for every observation against the complete ID union, then rebuild `$replyPages` and `$totals`. In the failure path, finalize all observations available at the time of failure before writing the failure summary.

Replace `reply_relation_mismatch` in the allowed reason list with:

```powershell
"reply_root_relation_mismatch", "reply_parent_self_reference"
```

Leave `comment_request_limit_exceeded` and every existing HTTP, schema, business, cursor, and loopback failure code unchanged.

- [ ] **Step 6: Run all relation tests and verify GREEN**

```powershell
go test ./scripts -run 'TestReplyProbe(SelectsFirstEligibleRootAndFetchesExactlyTwoReplyPages|StopsOnExplicitRootRelationMismatch|StopsOnParentSelfReference|AllowsUnresolvedParentAndFetchesSecondPage)' -v -count=1
```

Expected: PASS with the accepted parent cases reaching page two, both hard failures stopping before page two, and exact granular totals.

- [ ] **Step 7: Commit the implementation**

```powershell
git add scripts/probe-ltaoo-replies.ps1
git commit -m "feat: classify ltaoo reply parent relations"
```

### Task 3: Update operator documentation and superseded text

**Files:**
- Modify: `docs/LTAOO_REPLY_TWO_PAGE_PROBE.md:95-108`
- Modify: `docs/superpowers/specs/2026-08-13-ltaoo-reply-two-page-probe-design.md`
- Modify: `docs/superpowers/plans/2026-08-13-ltaoo-reply-two-page-probe.md`

- [ ] **Step 1: Replace the old equality rule in the runbook**

Document the eight granular counters, the compatibility formulas, and these stop rules:

```text
rootCommentId 显式非根值 -> reply_root_relation_mismatch
replyCommentId 等于自身 commentId -> reply_parent_self_reference
replyCommentId 指向未获取对象 -> parent_unresolved_count，仅记录，不额外请求
```

- [ ] **Step 2: Mark old design and plan text as superseded**

Add a short note next to the old two-field relation rule in both historical documents:

```markdown
> 关系语义已由 `docs/superpowers/specs/2026-08-13-ltaoo-reply-relation-semantics-design.md` 修订；实现与验收以修订规格为准。
```

Do not rewrite the historical commit sequence.

- [ ] **Step 3: Run the runbook contract test**

```powershell
go test ./scripts -run TestLtaooReplyProbeRunbookHasSafetySequence -v -count=1
```

Expected: PASS after updating any required-token list in the test to include both new failure codes and `parent_unresolved_count`.

- [ ] **Step 4: Commit documentation**

```powershell
git add docs/LTAOO_REPLY_TWO_PAGE_PROBE.md docs/superpowers/specs/2026-08-13-ltaoo-reply-two-page-probe-design.md docs/superpowers/plans/2026-08-13-ltaoo-reply-two-page-probe.md scripts/ltaoo_probe_script_test.go
git commit -m "docs: explain ltaoo reply relation evidence"
```

### Task 4: Complete automated and static verification

**Files:**
- Verify: `scripts/probe-ltaoo-replies.ps1`
- Verify: `scripts/ltaoo_probe_script_test.go`
- Verify: documentation changed in Task 4

- [ ] **Step 1: Parse the script with Windows PowerShell 5.1**

```powershell
$errors = $null
[void][System.Management.Automation.Language.Parser]::ParseFile(
  (Resolve-Path '.\scripts\probe-ltaoo-replies.ps1'),
  [ref]$null,
  [ref]$errors
)
if ($errors.Count -gt 0) { $errors | Format-List; exit 1 }
```

Expected: exit 0 and no parser errors.

- [ ] **Step 2: Run all script tests**

```powershell
go test ./scripts -v -count=1
```

Expected: PASS, including the new relationship tests and all existing request-limit, redaction, cursor, cleanup, and preparation tests.

- [ ] **Step 3: Check for leaks and obsolete failure logic**

```powershell
rg -n 'reply_relation_mismatch|wrong-root-secret|outside-bounded-window|self-reply-secret' scripts docs
```

Expected: fixture secrets occur only in Go tests; the obsolete reason code occurs only in an explicit superseded-history note, if retained at all.

- [ ] **Step 4: Check repository integrity**

```powershell
git diff --check
git status --short --branch
```

Expected: no whitespace errors and no uncommitted implementation changes after the final commit.

- [ ] **Step 5: Commit any verification-only corrections**

If Steps 1-4 require a correction, make the smallest change, rerun the exact failing command and the full suite, then commit:

```powershell
git add scripts/probe-ltaoo-replies.ps1 scripts/ltaoo_probe_script_test.go docs
git commit -m "fix: complete ltaoo relation verification"
```

If no correction is required, do not create an empty commit.

## Live-run gate after this plan

Do not perform a live run as part of implementation. After Task 5 passes, present the automated evidence and request separate approval for one new `run_id`. The live probe must still use the audited ltaoo commit, a temporary CurrentUser CA, exact Clash backup/restore, a `/sph/` clipboard link, no more than three comment requests, and independent residue verification.
