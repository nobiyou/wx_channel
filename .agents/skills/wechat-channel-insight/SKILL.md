---
name: wechat-channel-insight
description: Use when the user asks to inspect wx_channel WeChat Channels video data, local browse/download history, search accounts by name, live account results, video details, comments, share links, or explains why live data is empty. This skill belongs to wx_channel and does not route to wx_channel_insight.
---

# wx_channel video data

Use the local `wx_channel` MCP server as the only data source for this workflow. The MCP server exposes stored records, live WeChat page queries, and one explicit download action. Do not invent fields, call undocumented public APIs, or treat `wx_channel_insight` as a dependency.

## Workflow

1. Call `get_runtime_status` first. Record the source, observation time, ready client count, and live capabilities.
2. For “最近浏览”“历史下载”“本地记录” or date/status filters, call `list_local_videos`. Continue with `cursor` when the user asks for more results.
3. For one stored record, call `get_local_video`. If a local file is present and the user asks about file details, call `get_local_media_metadata`.
4. For “最新”“搜索”“某个账号主页”“视频详情”“评论” or a supported share link, use the corresponding live tool only when `get_runtime_status` reports a matching fresh capability.
5. If a live capability is unavailable, explain that the supported WeChat Channels page must be open and its page API must be ready. Keep the MCP error code and give that recovery action.
6. Only request comments when the user asks for them, and page through `next_marker` without assuming the first page is complete.
7. For an account name without a known username, call `search_channel_accounts` first. Use the returned `username` with `list_channel_videos`; never substitute the display name or invent an account ID. If several accounts match and the target is unclear, present the candidates for selection.
8. Account search uses the same `search_ready_clients` capability as video search. Read normalized results from `data.items`. Continue by passing `data.next_marker` unchanged as `next_marker`; do not decode or encode it. Use `data.has_more` to decide whether another page exists.
9. For visual questions about a video, first require a completed `download` record and a local file. If the user has a browse record without a completed download, call `download_video_for_analysis` with its `record_id` and poll until it reports `completed`; never pass a raw URL or decryption key. If the user only has a live profile or share link, resolve it to a stored browse record through the normal wx_channel page flow before downloading. Do not treat metadata or a remote URL as visual evidence. Then use the separate `wechat-channel-video-analysis` workflow and `get_local_video_frames`. Keep data queries, download state, and sampled visual observations distinct.

## Tool routing

| User intent | Tool |
| --- | --- |
| runtime, page readiness, why data is empty | `get_runtime_status` |
| browse/download history | `list_local_videos` |
| one local record | `get_local_video` |
| local file metadata | `get_local_media_metadata` |
| download a stored browse record for analysis | `download_video_for_analysis` |
| live account name or keyword search | `search_channel_accounts` |
| live video keyword search | `search_channel_videos` |
| live account video list | `list_channel_videos` |
| live video details | `get_channel_video_profile` |
| live comments | `list_channel_comments` |
| supported share URL | `resolve_channel_share_link` |

## Response rules

- Start with the query condition, source, and `observed_at` time.
- Distinguish `live` from `stored`; never describe stored records as current live data.
- Preserve `partial`, `warnings`, `next_cursor`, `has_more`, and `next_marker` in the answer when present.
- Show missing upstream metrics as “未返回” or “不可用”. A missing play count must never become `0`.
- Historical records can contain default zeros without evidence that a metric was collected. If `stored_zero_metrics_unknown` is present, report those null metrics as “历史记录未能确认”; a live response with an explicit numeric zero is a known zero.
- New stored records expose `metrics_source`, `metrics_observed_at`, and per-field `metrics_availability`. An `observed` numeric zero is a confirmed zero even in stored data; `not_returned` stays unavailable. `legacy_unknown` and `stored_without_provenance` do not establish when or how a metric was captured. Keep metric observation time separate from browse/download time.
- `metrics_source_verification: client_reported` means the injected page reported the source and capture time; wx_channel has not independently verified the upstream response. Downloading a cached profile preserves its original capture time. Missing or invalid capture times remain unknown.
- Local media metadata reads MP4/MOV and PNG/JPEG/GIF headers. Read `metadata_sources` per field: `mp4_mvhd`, `mp4_tkhd`, and `image_header` describe file measurements; `local_database` means a stored fallback. Unsupported or damaged files retain nulls and warnings. These header measurements do not analyze video content.
- Video results expose `object_id`, `nonce_id`, `author_id`, and nullable interaction fields. Use these IDs for follow-up calls. Account results expose `username`; comments expose `comment_id` and `reply_count`. Inline replies may require another `list_channel_comments` call with `comment_id`.
- Share metadata may have `object_id: null` and a separate `share_reference`. Keep using the original share URL in that case; never pass the share reference as a normal video ID.
- On failure, preserve `error.code` and explain `error.recovery`. A confirmed empty `data.items` is different from `empty_live_response` or `unsupported_live_schema`. For `busy`, wait for active queries; for `response_too_large`, narrow the query or lower local `page_size`. Never silently skip a page.
- Distinguish “metadata obtained” from “frames inspected”. Local frame images support sampled visual descriptions through `wechat-channel-video-analysis`; they do not provide audio transcription, full-video coverage, complete OCR, script generation, or cloning.
- Never expose cookies, bearer tokens, decryption keys, Referer values, signed video URLs, or arbitrary absolute local paths.

## Scope boundary

This skill covers video data access and local media indexing. `download_video_for_analysis` is the single explicit download action and accepts only a stored browse record id. Sampled visual analysis belongs to `wechat-channel-video-analysis`. Exports, deletion, publishing, automatic login, and protocol bypasses remain outside this workflow.

