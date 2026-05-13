# Checkpoint

Date: 2026-05-13
State: in_progress

## TodoCheckpointDraft

Current todo:

1. Normalize browser single-download request shaping
2. Align feed/home menus
3. Align batch-download request shaping
4. Keep wrapper orchestration semantics-neutral
5. Run full verification

Completed todos:

- approved spec
- approved implementation plan
- fixed `.worktrees/` ignore rule
- created isolated worktree `feat/original-video-semantics`
- added server-side failing tests for original/spec normalization
- implemented server-side normalization helpers
- passed targeted and broad `internal/handlers` tests

Active slice:

- prepare browser-side canonical request normalization in `download.js`

Next step:

- commit Task 1 server-side changes
- start Task 2 RED/GREEN in `download.js`

## Evidence

- implementation plan committed at `68f1c72`
- current repo has unrelated user changes in `web/about.html` and `wx_channel.upx`, so implementation must stay isolated
- isolated worktree created at `E:\netdisk\wx-down\wx_channel_new\wx_channel\.worktrees\feat-original-video-semantics`
- targeted RED failed due to missing `downloadVideoMode` helpers
- `go test ./internal/handlers -run "TestIsOriginalVideoURL|TestNormalizeOriginalVideoURL|TestDownloadVideoModeFromRequest|TestDownloadConnectionCountFromMode"` passed
- `go test ./internal/handlers` passed

## DriftCheckDraft

- scope: still matches approved original-video unification plan; only Task 1 is implemented so far
- compatibility: unchanged
- retirement: legacy `original` marker still compatibility-only target
- decision: continue

## ResumeStateHint

- do not implement on `main`
- Task 1 complete in worktree
- next owner file is `internal/assets/inject/download.js`
