# Task Intent

Date: 2026-05-13
Slug: original-video-semantics-unification

## Requested Outcome

Implement the approved plan so `wx_channel` uses one consistent meaning for original video across:

- feed/home single download
- batch download
- server-side execution

## Scope

- execute the approved implementation plan in `docs/aegis/plans/2026-05-13-original-video-semantics-unification.md`
- start with server-side tests and normalization
- continue into browser-side request normalization and batch parity

## Non-Goals

- do not modify `web/about.html`
- do not restore or alter `wx_channel.upx`
- do not redesign UI
- do not rewrite unrelated docs

## Baseline Read Set

- `docs/aegis/specs/2026-05-13-original-video-semantics-unification-design.md`
- `docs/aegis/plans/2026-05-13-original-video-semantics-unification.md`
- `internal/handlers/upload.go`
- `internal/assets/inject/download.js`
- `internal/assets/inject/feed.js`
- `internal/assets/inject/home.js`
- `internal/assets/inject/batch_download.js`
- `internal/handlers/script.go`

## Impact Statement

- affected layers: injected browser runtime, server download normalization, batch-download request shaping
- compatibility boundary: preserve current menus, cover download, encrypted direct-download flow, and legacy `X-snsvideoflag=original` acceptance
- main risks: browser direct-download regressions, batch semantic drift, record/de-dup coupling to URL shape
