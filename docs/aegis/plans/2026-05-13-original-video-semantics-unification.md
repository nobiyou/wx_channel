# Original Video Semantics Unification Implementation Plan

Date: 2026-05-13
Status: Ready for execution

## Goal

Implement the approved design to make `wx_channel` treat "original video" consistently across single-download UI, batch-download UI, and server-side download handling, while preserving compatibility with legacy requests that still use `X-snsvideoflag=original`.

## Architecture

Canonical owners for this implementation:

- browser-side request normalization: `internal/assets/inject/download.js`
- single-download menus: `internal/assets/inject/feed.js`, `internal/assets/inject/home.js`
- batch-download request generation: `internal/assets/inject/batch_download.js`
- wrapper/orchestration compatibility: `internal/handlers/script.go`
- server-side request normalization and execution branching: `internal/handlers/upload.go`

## Tech Stack

- Go for server-side request handling and tests
- Embedded JavaScript for injected browser runtime
- Existing Gopeed-backed downloader and in-page direct-download/decrypt flow

## Baseline / Authority Refs

- [docs/aegis/specs/2026-05-13-original-video-semantics-unification-design.md](E:\netdisk\wx-down\wx_channel_new\wx_channel\docs\aegis\specs\2026-05-13-original-video-semantics-unification-design.md)
- [docs/aegis/baseline/2026-05-13-initial-baseline.md](E:\netdisk\wx-down\wx_channel_new\wx_channel\docs\aegis\baseline\2026-05-13-initial-baseline.md)
- [docs/INTRODUCTION.md](E:\netdisk\wx-down\wx_channel_new\wx_channel\docs\INTRODUCTION.md)
- [docs/BATCH_DOWNLOAD_GUIDE.md](E:\netdisk\wx-down\wx_channel_new\wx_channel\docs\BATCH_DOWNLOAD_GUIDE.md)
- [internal/assets/inject/download.js](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\assets\inject\download.js)
- [internal/assets/inject/feed.js](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\assets\inject\feed.js)
- [internal/assets/inject/home.js](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\assets\inject\home.js)
- [internal/assets/inject/batch_download.js](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\assets\inject\batch_download.js)
- [internal/handlers/upload.go](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\handlers\upload.go)
- [internal/handlers/script.go](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\handlers\script.go)

## Compatibility Boundary

Must preserve:

- current feed/home download menus and entry points
- current cover download behavior
- current encrypted-video direct-download behavior
- current batch-download queue/progress UX
- acceptance of legacy requests carrying `X-snsvideoflag=original`

Must change:

- original-video semantics become canonicalized on "no explicit spec selected"
- browser and batch code stop using ad hoc default-stream interpretations
- server no longer depends on legacy `original` parameter as the primary source of truth

## Verification

Primary verification commands:

```powershell
go test ./internal/handlers ./internal/models ./internal/config
go test ./...
go build ./...
```

Manual verification checklist after code changes:

1. Feed page: click `原始视频` and inspect emitted request behavior.
2. Feed page: click a specific spec option and confirm `X-snsvideoflag=<fileFormat>`.
3. Home page: click `原始视频` and confirm same behavior as feed page.
4. Batch selection: confirm selected videos use the same original/spec rule as single download.
5. Encrypted original-video path: confirm direct-download/decrypt still succeeds.
6. Cover download: confirm no regression.

## Scope Check

### Facts

- `feed.js` and `home.js` already present `原始视频` to users.
- `download.js` currently treats `spec == null` as a default direct-download path.
- `upload.go` still infers original-video mode from legacy `X-snsvideoflag=original`.
- `batch_download.js` has its own request-shaping logic.

### Assumptions

- No existing automated JavaScript test harness exists for injected runtime behavior.
- Go tests are the only stable automated verification layer available for this change.

### Unknowns

- Whether download-record de-duplication keys on normalized metadata or raw URLs.
- Whether batch-download request generation currently injects additional fields not obvious from top-level reads.

### Ripple Signal Triage

- Owner expansion: yes, single-download and batch-download both touched.
- Downstream/consumer expansion: yes, server execution behavior depends on the same semantics.
- Contract expansion: yes, browser-to-server intent encoding changes in practice.
- Verification expansion: yes, automated Go tests plus manual browser verification required.

## File Map

Files to modify:

- `internal/handlers/upload.go`
- `internal/handlers/script.go`
- `internal/assets/inject/download.js`
- `internal/assets/inject/feed.js`
- `internal/assets/inject/home.js`
- `internal/assets/inject/batch_download.js`

Files to create:

- `internal/handlers/upload_test.go`

Files intentionally not modified in this plan:

- `web/about.html`
- `wx_channel.upx`
- existing user docs in `docs/`

## Risks

- Browser-side helper extraction could accidentally change direct-download control flow for encrypted videos.
- Batch flow may rely on a hidden default-stream assumption and silently change user expectations if not verified carefully.
- Server-side file naming or de-duplication may be coupled to current resolution/file-format fields.

## Rollback Surface

- Browser helper extraction can be reverted independently from server normalization.
- Legacy `X-snsvideoflag=original` handling stays in place during this implementation, which keeps rollback low-risk.

## Retirement

- Legacy `X-snsvideoflag=original` remains accepted but is demoted to compatibility-only handling.
- Any duplicate client-side original/spec interpretation in batch code is retired in favor of shared normalization.

## Task 1 - Lock down server-side original/spec normalization with tests

Files:

- create `internal/handlers/upload_test.go`
- modify `internal/handlers/upload.go`

Why:

- The server is the stable enforcement point for compatibility and execution branching.

Impact / Compatibility:

- Preserves legacy requests while making normalized intent testable and explicit.

Verification:

```powershell
go test ./internal/handlers -run "TestIsOriginalVideoURL|TestNormalizeOriginalVideoURL|Test.*Original.*Mode"
```

- [ ] Write test: add table-driven tests in `internal/handlers/upload_test.go` for:
  - legacy URL with `X-snsvideoflag=original`
  - URL with specific `X-snsvideoflag=WT111`
  - URL with no `X-snsvideoflag`
  - normalization of legacy original URL back to canonical direct URL
  - branch decision that original mode selects single connection while specific spec does not
- [ ] Verify RED: run
  ```powershell
  go test ./internal/handlers -run "TestIsOriginalVideoURL|TestNormalizeOriginalVideoURL|Test.*Original.*Mode"
  ```
  and confirm failures are due to missing or incorrect original/spec normalization behavior
- [ ] Minimal code: refactor `internal/handlers/upload.go` to introduce explicit helpers for:
  - detecting legacy original-marker requests
  - detecting specific spec requests
  - deriving normalized download mode from request fields and URL
  - applying single-connection behavior based on normalized original mode instead of only legacy query shape
- [ ] Verify GREEN: rerun
  ```powershell
  go test ./internal/handlers -run "TestIsOriginalVideoURL|TestNormalizeOriginalVideoURL|Test.*Original.*Mode"
  ```
  then run
  ```powershell
  go test ./internal/handlers
  ```
- [ ] Commit:
  ```powershell
  git add internal/handlers/upload.go internal/handlers/upload_test.go
  git commit -m "test: cover original video normalization"
  ```

Repair Track:

- repaired object: server-side original/spec classification
- action: move from raw-URL-only inference to normalized mode helpers
- impact: stable compatibility behavior with test coverage
- verification: handler tests above

Retirement Track:

- retired object: raw `X-snsvideoflag=original` as sole source of truth
- action: keep compatibility but demote ownership
- retained boundary: legacy callers still accepted

## Task 2 - Centralize browser-side request normalization in download.js

Files:

- modify `internal/assets/inject/download.js`

Why:

- Single-download and batch-download semantics cannot stay aligned if URL building remains duplicated or implicit.

Impact / Compatibility:

- Preserves existing UI entry points while changing the internal source of truth for request shaping.

Verification:

```powershell
go build ./...
```

Manual:

- open feed/home menu and inspect logged download URL/mode for original vs specific spec

- [ ] Write test: no automated JS test harness is available for injected runtime code in this repo; use deterministic helper extraction plus the manual assertions listed in this task as the explicit verification boundary
- [ ] Verify RED: confirm current code path in `download.js` still uses separate `if (spec)`/default direct URL behavior and lacks a single canonical helper
- [ ] Minimal code: in `internal/assets/inject/download.js`, introduce a canonical helper that:
  - receives `profile` and optional `spec`
  - returns normalized mode metadata plus effective URL
  - treats missing spec as original-video mode
  - treats present `spec.fileFormat` as specific-spec mode
  - preserves current decrypt/direct-download fields
- [ ] Verify GREEN: run
  ```powershell
  go build ./...
  ```
  then manually verify:
  - original item logs/use canonical original mode
  - specific spec item appends `X-snsvideoflag=<fileFormat>`
- [ ] Commit:
  ```powershell
  git add internal/assets/inject/download.js
  git commit -m "refactor: unify browser download normalization"
  ```

Repair Track:

- repaired object: browser request-shaping source of truth
- action: extract canonical helper in `download.js`
- impact: shared semantics for single and batch flows
- verification: build plus manual URL/mode checks

Retirement Track:

- retired object: ad hoc default direct-URL branch as semantic stand-in for original-video mode
- action: replace with explicit normalized helper

## Task 3 - Rewire feed and home single-download menus to the canonical helper

Files:

- modify `internal/assets/inject/feed.js`
- modify `internal/assets/inject/home.js`

Why:

- User-visible menu labels already promise original-video behavior; these entry points must reliably target the canonical helper.

Impact / Compatibility:

- Menu structure and wording stay the same.

Verification:

Manual:

- feed page original item
- feed page specific spec item
- home page original item

- [ ] Write test: no automated JS test harness exists; define manual acceptance on feed/home pages as the RED/ GREEN evidence path
- [ ] Verify RED: inspect current click handlers and confirm `data-index=-1` still only implies `null` without guaranteed canonical semantics
- [ ] Minimal code: update `feed.js` and `home.js` so menu clicks continue to call `__wx_channels_handle_click_download__`, but only after `download.js` owns the canonical interpretation; avoid any local re-interpretation of original/spec
- [ ] Verify GREEN: manually validate on feed/home:
  - `原始视频` follows canonical original behavior
  - specific spec item follows canonical specific-spec behavior
  - menu still opens/closes correctly
- [ ] Commit:
  ```powershell
  git add internal/assets/inject/feed.js internal/assets/inject/home.js
  git commit -m "fix: align feed and home original video actions"
  ```

Repair Track:

- repaired object: single-download entry-point consistency
- action: remove local semantic drift from feed/home menus
- impact: UI labels now match execution behavior
- verification: manual feed/home checks

## Task 4 - Make batch download reuse the same original/spec normalization rules

Files:

- modify `internal/assets/inject/batch_download.js`
- modify `internal/assets/inject/download.js`

Why:

- Scope `3` requires batch-download semantics to match single-download semantics.

Impact / Compatibility:

- Preserves selection, queue, and progress UX while changing how effective URLs and mode metadata are computed.

Verification:

```powershell
go build ./...
```

Manual:

- select videos in batch UI and verify request behavior for original-mode submissions

- [ ] Write test: because there is no browser-side JS harness, define one deterministic normalization call path that batch code must use, and prepare manual evidence steps for batch request inspection
- [ ] Verify RED: inspect current batch code and confirm it still duplicates formatting/request-shaping behavior instead of calling the canonical helper
- [ ] Minimal code: update `internal/assets/inject/batch_download.js` so each selected video:
  - is normalized through the same original/spec helper or a shared equivalent in `download.js`
  - preserves skip behavior for unsupported live items
  - submits request payloads whose original/spec intent matches single-download behavior
- [ ] Verify GREEN: run
  ```powershell
  go build ./...
  ```
  then manually verify:
  - batch-selected normal videos submit canonical original-mode URLs/metadata when no spec is chosen
  - specific-spec batch paths, if present, still preserve explicit spec behavior
  - progress UI and cancellation still work
- [ ] Commit:
  ```powershell
  git add internal/assets/inject/batch_download.js internal/assets/inject/download.js
  git commit -m "fix: unify batch original video semantics"
  ```

Repair Track:

- repaired object: batch-download semantic drift
- action: remove duplicated request shaping
- impact: batch and single downloads behave the same
- verification: build plus manual batch checks

Retirement Track:

- retired object: batch-local interpretation of default stream versus original stream
- action: collapse onto shared normalization

## Task 5 - Keep script wrapper orchestration compatible and non-authoritative

Files:

- modify `internal/handlers/script.go`

Why:

- The wrapper currently intercepts `__wx_channels_handle_click_download__`; it must not accidentally reintroduce semantic branching or break the updated call contract.

Impact / Compatibility:

- Pause/resume behavior preserved; download intent ownership stays below the wrapper.

Verification:

```powershell
go build ./...
```

Manual:

- click original and specific-spec download once each and confirm pause/resume still works

- [ ] Write test: wrapper behavior is browser-oriented and has no current automated test harness; use build + manual verification as the bounded verification method
- [ ] Verify RED: inspect current wrapper and confirm it forwards one positional argument and may implicitly rely on old behavior
- [ ] Minimal code: adjust `internal/handlers/script.go` so the wrapper:
  - preserves argument forwarding
  - does not infer original/spec mode itself
  - preserves pause/resume and cover-download overrides
- [ ] Verify GREEN: run
  ```powershell
  go build ./...
  ```
  then manually verify pause/resume still occurs during download initiation
- [ ] Commit:
  ```powershell
  git add internal/handlers/script.go
  git commit -m "refactor: keep download wrapper semantics-neutral"
  ```

Repair Track:

- repaired object: wrapper compatibility with normalized download contract
- action: keep wrapper orchestration-only
- impact: avoids reintroducing semantic ambiguity
- verification: build plus manual pause/resume checks

## Task 6 - Full regression verification before completion

Files:

- no new code; verification only

Why:

- The change crosses browser-injected runtime and server execution boundaries.

Impact / Compatibility:

- Final evidence bundle for the approved scope.

Verification:

```powershell
go test ./internal/handlers ./internal/models ./internal/config
go test ./...
go build ./...
git status --short --branch
```

- [ ] Write test: add no new test code in this task; this task's evidence comes from rerunning the impacted automated suites and collecting manual verification results
- [ ] Verify RED: if any targeted tests or build commands fail, stop and fix before claiming completion
- [ ] Minimal code: only bugfix follow-up allowed if regression appears
- [ ] Verify GREEN: run all commands above and capture exit status and notable output
- [ ] Commit:
  ```powershell
  git status --short --branch
  ```
  No commit in this step unless regression fixes were required

Residual Risk:

- Browser-side semantics still rely on manual verification because no automated JS test harness was introduced in this scope.
- Download-record de-duplication semantics may still need follow-up if downstream behavior proves URL-sensitive.
