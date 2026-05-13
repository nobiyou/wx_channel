# Checkpoint

Date: 2026-05-13
State: in_progress

## TodoCheckpointDraft

Current todo:

1. Fix `.worktrees/` ignore rule and create isolated worktree
2. Add server-side failing tests for original/spec normalization
3. Implement server-side normalization helpers
4. Normalize browser single-download request shaping
5. Align feed/home menus
6. Align batch-download request shaping
7. Keep wrapper orchestration semantics-neutral
8. Run full verification

Completed todos:

- approved spec
- approved implementation plan

Active slice:

- prepare safe execution environment and long-task checkpoint

Next step:

- commit ignore/work checkpoint files
- create isolated worktree for implementation branch

## Evidence

- implementation plan committed at `68f1c72`
- current repo has unrelated user changes in `web/about.html` and `wx_channel.upx`, so implementation must stay isolated

## DriftCheckDraft

- scope: still matches approved original-video unification plan
- compatibility: unchanged
- retirement: legacy `original` marker still compatibility-only target
- decision: continue

## ResumeStateHint

- do not implement on `main`
- after checkpoint commit, create worktree under `.worktrees/`
- start RED with `internal/handlers/upload_test.go`
