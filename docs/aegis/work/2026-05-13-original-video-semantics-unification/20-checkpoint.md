# Checkpoint

Date: 2026-05-13
State: in_progress

## TodoCheckpointDraft

Current todo:

1. Keep wrapper orchestration semantics-neutral
2. Run full verification

Completed todos:

- approved spec
- approved implementation plan
- fixed `.worktrees/` ignore rule
- created isolated worktree `feat/original-video-semantics`
- added server-side failing tests for original/spec normalization
- implemented server-side normalization helpers
- passed targeted and broad `internal/handlers` tests
- centralized browser-side original/spec normalization in `download.js`
- aligned batch download to reuse canonical original-mode normalization
- re-checked `feed.js`, `home.js`, and `script.go`; they remain semantics-neutral entry/wrapper layers with no extra original/spec branching
- passed JS syntax checks for `download.js` and `batch_download.js`
- passed deterministic JS helper semantics check for original vs specific spec
- passed `go test ./internal/handlers ./internal/models ./internal/config`

Active slice:

- collect remaining regression evidence and decide commit boundaries

Next step:

- rerun broader verification with bounded commands
- commit front-end normalization changes if no new blocker appears

## Evidence

- implementation plan committed at `68f1c72`
- current repo has unrelated user changes in `web/about.html` and `wx_channel.upx`, so implementation must stay isolated
- isolated worktree created at `E:\netdisk\wx-down\wx_channel_new\wx_channel\.worktrees\feat-original-video-semantics`
- targeted RED failed due to missing `downloadVideoMode` helpers
- `go test ./internal/handlers -run "TestIsOriginalVideoURL|TestNormalizeOriginalVideoURL|TestDownloadVideoModeFromRequest|TestDownloadConnectionCountFromMode"` passed
- `go test ./internal/handlers` passed
- `download.js` previously used separate `if (spec)` and default direct-download URL shaping
- `batch_download.js` previously submitted `video.url` directly without reusing single-download original/spec normalization
- `node` syntax/evaluation checks passed for `internal/assets/inject/download.js` and `internal/assets/inject/batch_download.js`
- deterministic helper check passed for canonical original mode vs `X-snsvideoflag=<fileFormat>` specific mode
- `go test ./internal/handlers ./internal/models ./internal/config` passed
- `go test ./...` and `go build ./...` have only timeout evidence so far; no pass/fail conclusion yet

## DriftCheckDraft

- scope: still matches approved original-video unification plan; Tasks 1-4 are now implemented without expanding beyond approved scope
- compatibility: feed/home still pass `spec/null`; script wrapper still only pauses/resumes and forwards one argument
- retirement: browser-side ad hoc default-stream semantics are now folded into a canonical original-mode helper; legacy `original` marker remains compatibility-only on the server
- decision: continue

## ResumeStateHint

- do not implement on `main`
- Tasks 1-4 implemented in worktree
- remaining work is broader verification and commit slicing
