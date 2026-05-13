# Original Video Semantics Unification Design

Date: 2026-05-13
Status: Drafted after user approval to continue design

## Goal

Unify `wx_channel` so that "original video" means the same thing everywhere:

- single-video download menus
- batch download flows
- server-side download handling
- compatibility handling for historical request shapes

The target behavior must match the working mental model already verified in `wx_channels_download`:

- original video = no explicit transcoding spec selected
- specific quality = explicit `fileFormat`/spec selected

## Architecture

The change is a semantic unification, not a new feature family. The canonical owners are:

- browser request construction: `internal/assets/inject/download.js`
- menu entry points: `internal/assets/inject/feed.js`, `internal/assets/inject/home.js`
- batch request construction: `internal/assets/inject/batch_download.js`
- injected wrapper compatibility: `internal/handlers/script.go`
- server-side normalization and execution: `internal/handlers/upload.go`

The design intentionally avoids changing unrelated console UI, cover download, comment collection, or web docs in this phase.

## Tech Stack

- Go handlers for server-side request normalization and downloading
- Embedded injected JavaScript for browser-side menu and request generation
- Existing direct-download and decrypt flow already present in `download.js`

## Baseline / Authority Refs

- Root product summary: [README.md](E:\netdisk\wx-down\wx_channel_new\wx_channel\README.md)
- User documentation for download options: [docs/INTRODUCTION.md](E:\netdisk\wx-down\wx_channel_new\wx_channel\docs\INTRODUCTION.md)
- Batch download user documentation: [docs/BATCH_DOWNLOAD_GUIDE.md](E:\netdisk\wx-down\wx_channel_new\wx_channel\docs\BATCH_DOWNLOAD_GUIDE.md)
- Current single-download owner: [internal/assets/inject/download.js](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\assets\inject\download.js)
- Current batch owner: [internal/assets/inject/batch_download.js](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\assets\inject\batch_download.js)
- Current server owner: [internal/handlers/upload.go](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\handlers\upload.go)
- Current wrapper owner: [internal/handlers/script.go](E:\netdisk\wx-down\wx_channel_new\wx_channel\internal\handlers\script.go)

## Compatibility Boundary

Must preserve:

- existing single-video download UI presence and menu structure
- batch-download ability to submit selected videos
- existing cover-download behavior
- existing direct-download/decrypt path for encrypted videos
- acceptance of legacy requests that still encode original-video intent as `X-snsvideoflag=original`

Must change:

- canonical meaning of "original video" becomes "no explicit spec selected"
- legacy `original` query-parameter contract becomes compatibility-only, not the primary semantic path

## Problem Statement

`wx_channel` currently exposes "原始视频" in the UI, but the semantics are split:

1. Menu entries in feed/home pass `spec = null`, suggesting original-video intent.
2. `download.js` treats `spec = null` as "use current/default direct URL", which is not guaranteed to be the same semantic contract as original-source download.
3. `upload.go` still treats `X-snsvideoflag=original` as the signal for original-video mode, then strips it back to a default URL.
4. `batch_download.js` carries its own request-building logic, so single and batch behaviors can diverge.

This mismatch makes the user-facing label stable while the actual meaning varies by entry path.

## Scope

In scope:

- unify single-download request semantics
- unify batch-download request semantics
- centralize browser-side request normalization
- centralize server-side original-video recognition and compatibility fallback
- ensure download record and execution mode can distinguish original vs spec-driven downloads
- add tests for server normalization and branch selection

Out of scope:

- redesigning UI layout or visual language
- rewriting the entire download API schema
- changing cover-download behavior
- changing comment capture, browse history, or console export behavior
- editing user-facing docs outside Aegis artifacts in this design phase

## Options Considered

### Option 1: Keep Legacy `X-snsvideoflag=original` As Canonical

Browser and batch code would explicitly send `X-snsvideoflag=original` for original-video intent, and the server would continue recognizing that as the main contract.

Pros:

- smallest implementation delta
- minimal server changes

Cons:

- preserves a private semantic contract that diverges from `wx_channels_download`
- keeps ambiguity between "no spec" and "original"
- leaves duplicate request-building logic hard to reason about

### Option 2: Canonicalize On `spec == null`, Keep `original` As Compatibility

Browser and batch code treat `null`/missing spec as original-video intent. Specific spec still adds `X-snsvideoflag=<fileFormat>`. The server accepts both forms, but internally normalizes to the canonical "original-mode" meaning.

Pros:

- aligns with `wx_channels_download`
- keeps UX unchanged
- bounded code churn
- allows gradual retirement of historical request shapes

Cons:

- touches both browser and server owners
- requires careful compatibility tests

### Option 3: Introduce A New Explicit `downloadMode`

Browser and batch code would send `downloadMode: original|spec` and the server would stop inferring intent from URL shape.

Pros:

- cleanest semantics
- best long-term API clarity

Cons:

- largest contract change
- requires broader request-shape migration
- unnecessary for the current requested fix

## Recommended Approach

Adopt **Option 2**.

This is the smallest design that actually resolves the ambiguity instead of re-labeling it. It aligns with the verified behavior model from `wx_channels_download` while limiting change scope to the actual semantic owners.

## Detailed Design

### 1. Browser-Side Canonical Request Normalization

Create one canonical helper in `download.js` that derives the effective download mode and URL from:

- current profile
- optional selected spec

Rules:

- if selected spec exists and has `fileFormat`, build a spec URL by appending `X-snsvideoflag=<fileFormat>`
- otherwise, treat the request as original-video mode and build the original-source URL path
- preserve decrypt key, title, author, and metadata population exactly as before

The helper should return a normalized structure that both single-download and batch-download code can use.

### 2. Single-Video UI Semantics

`feed.js` and `home.js` keep the current menu shape:

- highlighted "原始视频" item
- specific spec items below it

But the click path must now delegate to the canonical helper so that:

- `data-index = -1` really means original-video mode
- selected spec means explicit transcoding target

The menu label stays unchanged because the user-facing wording is already correct.

### 3. Batch Download Semantics

`batch_download.js` must stop hand-carrying its own interpretation of default versus original versus spec.

The batch flow should:

- normalize each selected video through the same browser-side helper or equivalent shared routine
- preserve ability to skip unsupported items
- preserve current queue/progress UX
- submit original-mode and spec-mode tasks with the same semantic distinction as single-download

If the batch flow currently lacks per-video spec selection, its default selected behavior must still map to the same canonical original-video rule rather than an accidental "current/default stream" rule.

### 4. Server-Side Normalization

`upload.go` becomes the canonical server owner of download intent recognition.

Server normalization should classify requests into:

- original mode
- specific spec mode

Accepted inputs:

- canonical browser requests that imply original mode by absence of specific spec
- explicit spec requests
- legacy requests carrying `X-snsvideoflag=original`

Server behavior:

- legacy `original` marker is accepted and normalized
- original mode continues to use single-connection execution where that behavior is currently required
- specific spec mode preserves current behavior

The internal logic must stop treating legacy URL shape as the only way to know original intent.

### 5. Wrapper Layer

`script.go` currently wraps `__wx_channels_handle_click_download__` to pause/resume video and keep cross-cutting behavior.

This wrapper must remain signature-compatible with the updated canonical request path. It should not re-introduce semantic branching or duplicate original/spec interpretation. Its job remains orchestration, not download-mode ownership.

### 6. Recording and Naming

Where download records or internal metadata distinguish tasks, original-video mode and specific-spec mode should remain distinguishable. The design does not require a user-visible label change, but it does require enough metadata consistency that de-duplication and debugging can tell these flows apart.

The design prefers using normalized server-side metadata over inferring from unstable raw URLs.

## Data Flow

### Single Download

1. User opens feed/home menu
2. User clicks "原始视频" or a spec item
3. `download.js` normalizes mode + URL
4. Browser either uses direct frontend path or posts normalized request to `/__wx_channels_api/download_video`
5. Server normalizes compatibility inputs again
6. Server executes original-mode or spec-mode download

### Batch Download

1. User selects multiple videos
2. Batch code formats each video
3. Each selected item is normalized through the same mode rules
4. Requests are submitted with consistent original/spec semantics
5. Server applies the same normalization and execution logic

## Error Handling

- Missing profile or URL remains a user-visible browser error
- Unsupported live items remain non-downloadable
- Legacy malformed `original` URLs are tolerated and normalized where possible
- If normalization cannot construct a valid original URL, fail fast with explicit logs instead of silently falling back to an arbitrary stream URL

## Testing Strategy

### Automated

Primary required automated coverage is in Go because that is the stable test harness already present in the repo.

Add tests for:

- original-mode detection
- specific-spec detection
- legacy `X-snsvideoflag=original` normalization
- branch selection that enforces single-connection behavior for original mode
- regression coverage ensuring specific-spec requests do not collapse into original mode

### Manual

Manual verification remains required because the injected browser runtime appears to lack a mature automated JS test harness.

Required manual checks:

1. feed page menu: original item
2. feed page menu: specific spec item
3. home page menu: original item
4. batch selection submission
5. encrypted original-video direct path
6. cover download regression check

## Ripple Signal Triage

Expanded scope is justified because:

- owner expansion: single-download and batch-download both own user-visible semantics
- downstream expansion: server de-duplication/execution depends on the same semantic distinction
- verification expansion: both UI paths and server normalization need evidence

No further subsystem expansion is needed in this design.

## Repair Track

- repaired object: original-video semantic contract
- root cause: split ownership and historical reliance on `X-snsvideoflag=original`
- canonical owner after repair: `download.js` for browser normalization, `upload.go` for server normalization
- impact: single-download and batch-download produce aligned requests and execution behavior
- verification: Go normalization tests plus targeted manual browser checks

## Retirement Track

- retired object: `X-snsvideoflag=original` as primary semantic source of truth
- action: demote to compatibility-only handling
- retained boundary: legacy requests still accepted during transition
- future deletion trigger: once all active browser emitters and persisted workflows no longer rely on the legacy marker

## Risks and Unknowns

- Batch flow may contain additional hidden assumptions about default stream choice not visible from current top-level reads
- Current wrapper logic in `script.go` may rely on argument shape subtleties when pausing/resuming downloads
- Existing download-record de-duplication might already key off raw URL shape and may need targeted adjustment during implementation

These are implementation risks, not design blockers.

## Non-Goals

- No UI redesign
- No broad API redesign
- No doc rewrite outside Aegis artifacts in this phase
- No migration of unrelated download features

## Working Artifacts

### TaskIntentDraft

- Outcome: make original-video behavior consistent across single and batch download paths
- Scope: injected browser runtime plus server normalization
- Risks: compatibility with legacy requests; wrapper orchestration regressions

### BaselineReadSetHint

- `README.md`
- `docs/INTRODUCTION.md`
- `docs/BATCH_DOWNLOAD_GUIDE.md`
- `internal/assets/inject/download.js`
- `internal/assets/inject/feed.js`
- `internal/assets/inject/home.js`
- `internal/assets/inject/batch_download.js`
- `internal/handlers/upload.go`
- `internal/handlers/script.go`

### ImpactStatementDraft

- affected layers: injected UI, browser request shaping, server download normalization
- invariants: UI entries remain present; cover download unchanged; legacy original-marker requests still accepted
- non-goals: no visual redesign, no console rewrite, no broad API schema change
