# Initial Baseline Snapshot

Date: 2026-05-13

## 1. Project Structure

- `main` entrypoint builds a Windows-oriented local downloader for WeChat Channels
- `internal/handlers/` owns proxy interception, injected script serving, upload/download APIs, and console APIs
- `internal/assets/inject/` owns browser-side injected runtime for feed/home/profile pages
- `internal/services/` owns download records, queues, exports, cleanup, and Gopeed integration
- `internal/database/` owns local persistence
- `web/` owns the web console frontend
- `docs/` owns user-facing product and operational docs

## 2. Tech Stack

- Go application runtime
- Embedded frontend and injected JavaScript assets
- SunnyNet-based interception/proxy flow
- Local file storage and SQLite-style persistence through `internal/database`

## 3. Ownership Mapping

- Browser download behavior: `internal/assets/inject/download.js`
- Feed download UI: `internal/assets/inject/feed.js`
- Home download UI: `internal/assets/inject/home.js`
- Batch download UI/behavior: `internal/assets/inject/batch_download.js`
- Script wrapping and injection order: `internal/handlers/script.go`
- Server-side direct download and persistence: `internal/handlers/upload.go`

## 4. Contract Inventory

- `POST /__wx_channels_api/download_video`
- `POST /__wx_channels_api/save_cover`
- `POST /__wx_channels_api/save_video`
- Injected global functions such as `insert_download_btn`, `__wx_channels_handle_click_download__`, and logging helpers

## 5. Dependency Direction Convention

- Injected page scripts depend on shared injected helpers
- Handler layer bridges browser requests into services and storage
- Services should not depend on page-specific injected behavior

## 6. Test System

- Go tests exist in multiple `internal/` packages
- Limited evidence of automated JavaScript tests in the injected runtime
- Non-trivial browser behavior changes need Go-level coverage plus manual verification

## 7. Build and Deploy

- Local Go build produces `wx_channel.exe`
- Embedded assets are served by the application itself
- GitHub release flow exists; recent commits show active packaging and asset sync work

## 8. Known Anti-Patterns

- Browser-side behavior is partially wrapped again in `script.go`, which can obscure the canonical owner of download semantics
- Download semantics are split across single-download UI, batch UI, and server fallback logic
- Historical compatibility logic around `X-snsvideoflag=original` risks preserving ambiguous semantics

## 9. Last Review Findings

- No prior Aegis review artifacts existed before this snapshot
- Original video behavior is documented for users but not consistently represented across injected UI, batch processing, and server handling

## 10. Compatibility Boundaries

- Existing single-video and batch download user flows must keep working
- Existing cover download and comment collection behaviors must not regress
- Existing stored or in-flight requests using legacy `X-snsvideoflag=original` must remain accepted during transition
