# Checkpoint

Date: 2026-05-13
State: in_progress

## TodoCheckpointDraft

Current todo:

1. Run full verification

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
- fixed stale JWT init test to match current auto-generated-secret behavior
- fixed `RoundRobinSelector` nondeterminism by sorting clients before round-robin selection and starting from the first slot
- passed targeted `hub_server/middleware` and `internal/websocket` regression tests

Active slice:

- run final full-repo regression after targeted fixes

Next step:

- run `go test ./...`
- if green, commit verification fixes and close out

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
- `go build ./...` passed
- `go test ./...` initially failed in `hub_server/middleware` because `TestInitJWTSecretFromEnv` still expected a missing-env error while `InitJWTSecretFromEnv` now generates a random secret by design
- `go test ./...` initially failed in `internal/websocket` because `RoundRobinSelector` depended on Go map iteration order and started from the second slot due to pre-modulo increment
- `go test ./hub_server/middleware -run TestInitJWTSecretFromEnv -count=1` passed after aligning the stale test with current design
- `go test ./internal/websocket -run "TestRoundRobinSelector|TestRoundRobinSelectorStableAcrossFreshMaps|TestConcurrentAccess" -count=1` passed after stabilizing round-robin ordering

## DriftCheckDraft

- scope: still matches approved original-video unification plan; verification follow-up only repaired pre-existing full-suite failures needed to close the branch cleanly
- compatibility: feed/home still pass `spec/null`; script wrapper still only pauses/resumes and forwards one argument
- retirement: browser-side ad hoc default-stream semantics are now folded into a canonical original-mode helper; legacy `original` marker remains compatibility-only on the server; unstable map-order round-robin behavior is retired
- decision: continue

## ResumeStateHint

- do not implement on `main`
- implementation complete in worktree
- remaining work is final full-suite verification and commit of verification repairs
