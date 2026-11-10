package mcpserver

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wx_channel/internal/websocket"
)

type emptyInput struct{}

type listLocalVideosInput struct {
	Source    string `json:"source,omitempty" jsonschema:"local data source: browse, download, or all"`
	Query     string `json:"query,omitempty" jsonschema:"title or author keyword"`
	AuthorID  string `json:"author_id,omitempty" jsonschema:"exact stored account id; only browse records preserve it"`
	Author    string `json:"author,omitempty" jsonschema:"author substring filter"`
	StartDate string `json:"start_date,omitempty" jsonschema:"inclusive date or RFC3339 timestamp"`
	EndDate   string `json:"end_date,omitempty" jsonschema:"inclusive date or RFC3339 timestamp"`
	Status    string `json:"status,omitempty" jsonschema:"download status filter"`
	PageSize  int    `json:"page_size,omitempty" jsonschema:"number of records, maximum 50"`
	Cursor    string `json:"cursor,omitempty" jsonschema:"opaque cursor from the previous page"`
}

type getLocalVideoInput struct {
	ID     string `json:"id" jsonschema:"local record id"`
	Source string `json:"source,omitempty" jsonschema:"browse or download; omit to try download then browse"`
}

type localMediaMetadataInput struct {
	RelativePath string `json:"relative_path,omitempty" jsonschema:"path relative to the configured downloads directory"`
	RecordID     string `json:"record_id,omitempty" jsonschema:"local record id used to resolve its stored media path"`
	Source       string `json:"source,omitempty" jsonschema:"browse or download when record_id is supplied"`
}

type channelSearchInput struct {
	Keyword    string `json:"keyword" jsonschema:"search keyword, maximum 100 characters"`
	NextMarker string `json:"next_marker,omitempty" jsonschema:"continuation marker returned by the previous page"`
}

type listChannelVideosInput struct {
	Username   string `json:"username" jsonschema:"video account username or finder id"`
	NextMarker string `json:"next_marker,omitempty" jsonschema:"continuation marker returned by the previous page"`
}

type channelVideoProfileInput struct {
	ObjectID string `json:"object_id,omitempty" jsonschema:"video object id"`
	NonceID  string `json:"nonce_id,omitempty" jsonschema:"video nonce id"`
	URL      string `json:"url,omitempty" jsonschema:"supported WeChat Channels page or share URL"`
}

type channelCommentsInput struct {
	ObjectID   string `json:"object_id" jsonschema:"video object id"`
	NonceID    string `json:"nonce_id" jsonschema:"video nonce id"`
	CommentID  string `json:"comment_id,omitempty" jsonschema:"comment id for replies, if supported"`
	NextMarker string `json:"next_marker,omitempty" jsonschema:"continuation marker returned by the previous page"`
}

type shareLinkInput struct {
	URL string `json:"url" jsonschema:"supported WeChat Channels share URL"`
}

type downloadVideoForAnalysisInput struct {
	RecordID string `json:"record_id" jsonschema:"browse record id from list_local_videos; the download URL is read from wx_channel storage"`
}

func registerTools(s *Server) {
	registerReadTool(s, &mcp.Tool{
		Name:        "get_runtime_status",
		Description: "Return wx_channel runtime, WebSocket page readiness, local MCP endpoint metadata, and available live query capabilities.",
	}, s.getRuntimeStatus)
	registerReadTool(s, &mcp.Tool{
		Name:        "list_local_videos",
		Description: "List stored browse and download records with bounded pagination and title, author, date, and status filters.",
	}, s.listLocalVideos)
	registerReadTool(s, &mcp.Tool{
		Name:        "get_local_video",
		Description: "Return one stored browse or download record and safe local media reference information.",
	}, s.getLocalVideo)
	registerReadTool(s, &mcp.Tool{
		Name:        "get_local_media_metadata",
		Description: "Return safe metadata for a local media file inside the configured downloads directory.",
	}, s.getLocalMediaMetadata)
	registerReadTool(s, &mcp.Tool{
		Name:        "get_local_video_frames",
		Description: "Return up to three bounded JPEG samples from a local MP4/MOV video for visual analysis. Uses local FFmpeg, requires no ready WeChat page, and does not analyze audio or the entire video.",
	}, s.getLocalVideoFrames)
	registerDownloadTool(s)
	registerVideoAnalysisTools(s)
	registerReadTool(s, &mcp.Tool{
		Name:        "search_channel_accounts",
		Description: "Search live WeChat Channels accounts by name or keyword; use the returned username with list_channel_videos. Use get_runtime_status first and next_marker to continue search results.",
	}, s.searchChannelAccounts)
	registerReadTool(s, &mcp.Tool{
		Name:        "search_channel_videos",
		Description: "Search live WeChat Channels videos through an already-ready desktop page; use get_runtime_status first.",
	}, s.searchChannelVideos)
	registerReadTool(s, &mcp.Tool{
		Name:        "list_channel_videos",
		Description: "List live videos for a WeChat Channels account using the page continuation marker.",
	}, s.listChannelVideos)
	registerReadTool(s, &mcp.Tool{
		Name:        "get_channel_video_profile",
		Description: "Read live details for one WeChat Channels video or supported share page.",
	}, s.getChannelVideoProfile)
	registerReadTool(s, &mcp.Tool{
		Name:        "list_channel_comments",
		Description: "Read one page of live comments for a WeChat Channels video.",
	}, s.listChannelComments)
	registerReadTool(s, &mcp.Tool{
		Name:        "resolve_channel_share_link",
		Description: "Resolve a supported WeChat Channels share link into safe live metadata.",
	}, s.resolveChannelShareLink)
}

func (s *Server) getRuntimeStatus(context.Context, *mcp.CallToolRequest, emptyInput) (*mcp.CallToolResult, map[string]any, error) {
	data := map[string]any{
		"mcp": map[string]any{
			"endpoint":     s.Endpoint(),
			"transport":    "streamable_http",
			"bind_address": "127.0.0.1",
			"auth":         s.TokenInfo(),
			"read_only":    false,
			"actions":      []string{"download_video_for_analysis", "start_video_analysis", "save_video_analysis_review"},
		},
	}
	warnings := []string{}
	_, decoderErr := findFFmpeg(s.deps.FFmpegPath)
	data["local_media_analysis"] = map[string]any{"ffmpeg_available": decoderErr == nil, "max_frames": maxVideoFrames, "max_video_bytes": maxSourceVideoBytes, "supported_formats": []string{"mp4", "mov", "m4v"}, "tool": "get_local_video_frames", "analysis_tool": "start_video_analysis", "analysis_max_video_bytes": maxAnalysisVideoBytes, "analysis_max_shots": maxAnalysisShots}
	modelStatus, model := videoAnalysisModelStatus("hybrid")
	apiProvider := ""
	if cfg, configured, err := loadVideoAnalysisModelConfig(); err == nil && configured {
		apiProvider = cfg.Provider
	}
	data["video_analysis"] = map[string]any{"tool": "start_video_analysis", "review_tool": "save_video_analysis_review", "default_provider": "codex", "requires_additional_api_key": false, "external_api": map[string]any{"provider": apiProvider, "status": modelStatus, "model": model, "supported_providers": []string{"openai_compatible", "gemini"}}, "external_upload_requires_opt_in": true, "download_required": true, "report_formats": []string{"json", "html"}, "local_enrichment": localEnrichmentCapability()}
	if s.deps.RuntimeDiagnostics != nil {
		snapshot := s.deps.RuntimeDiagnostics.Snapshot()
		if snapshot.Certificate.Error != "" {
			snapshot.Certificate.Error = "certificate_check_failed"
		}
		if snapshot.Injection.LastError != "" {
			snapshot.Injection.LastError = "page_injection_failed"
		}
		if snapshot.Lifecycle.LastError != "" {
			snapshot.Lifecycle.LastError = "page_lifecycle_failed"
		}
		data["runtime"] = snapshot
	} else {
		warnings = append(warnings, "runtime_diagnostics_unavailable")
	}
	statuses := []websocket.ClientStatus{}
	if s.deps.Hub != nil {
		statuses = s.deps.Hub.ClientStatuses()
	}
	statuses, hrefsRedacted := sanitizeClientStatuses(statuses)
	if len(statuses) > 50 {
		statuses = statuses[:50]
		warnings = append(warnings, "client_status_list_truncated")
	}
	if hrefsRedacted {
		warnings = append(warnings, "client_page_urls_sanitized")
	}
	data["clients"] = statuses
	data["client_count"] = len(statuses)
	capabilities := map[string]int{
		"search_ready_clients":  0,
		"feed_ready_clients":    0,
		"profile_ready_clients": 0,
		"comment_ready_clients": 0,
	}
	for _, status := range statuses {
		if status.Fresh && status.SupportsSearch {
			capabilities["search_ready_clients"]++
		}
		if status.Fresh && status.SupportsFeed {
			capabilities["feed_ready_clients"]++
		}
		if status.Fresh && status.SupportsProfile {
			capabilities["profile_ready_clients"]++
		}
		if status.Fresh && status.SupportsComment {
			capabilities["comment_ready_clients"]++
		}
	}
	data["live_capabilities"] = capabilities
	return nil, envelope("runtime_diagnostics", "live", data, warnings), nil
}

func sanitizeClientStatuses(statuses []websocket.ClientStatus) ([]websocket.ClientStatus, bool) {
	if len(statuses) == 0 {
		return statuses, false
	}
	sanitized := append([]websocket.ClientStatus(nil), statuses...)
	redacted := false
	for i := range sanitized {
		if sanitized[i].APIProbeError != "" {
			sanitized[i].APIProbeError = "page_api_probe_failed"
			redacted = true
		}
		path, changed := sanitizePageReference(sanitized[i].PagePath)
		sanitized[i].PagePath = path
		redacted = redacted || changed
		methods := map[string]bool{}
		for _, name := range []string{"finderSearch", "finderUserPage", "finderGetCommentDetail", "finderGetCommentList", "finderGetInteractionedFeedList"} {
			if value, ok := sanitized[i].Methods[name]; ok {
				methods[name] = value
			}
		}
		sanitized[i].Methods = methods
		if sanitized[i].Href == "" {
			continue
		}
		href, changed := sanitizePageReference(sanitized[i].Href)
		sanitized[i].Href = href
		redacted = redacted || changed
	}
	return sanitized, redacted
}

func sanitizePageReference(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", true
	}
	original := parsed.String()
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), parsed.String() != original
}

func (s *Server) searchChannelAccounts(ctx context.Context, _ *mcp.CallToolRequest, input channelSearchInput) (*mcp.CallToolResult, map[string]any, error) {
	return s.searchChannels(ctx, input, 1)
}

func (s *Server) searchChannelVideos(ctx context.Context, _ *mcp.CallToolRequest, input channelSearchInput) (*mcp.CallToolResult, map[string]any, error) {
	return s.searchChannels(ctx, input, 3)
}

func (s *Server) searchChannels(ctx context.Context, input channelSearchInput, searchType int) (*mcp.CallToolResult, map[string]any, error) {
	if err := validateLiveFields(input.NextMarker); err != nil {
		return nil, nil, err
	}
	keyword := strings.TrimSpace(input.Keyword)
	if keyword == "" {
		return nil, nil, errors.New("invalid_argument: keyword is required")
	}
	if len([]rune(keyword)) > 100 {
		return nil, nil, errors.New("invalid_argument: keyword is limited to 100 characters")
	}
	return s.callLive(ctx, "key:channels:contact_list", websocket.SearchContactBody{
		Keyword: keyword, Type: searchType, NextMarker: strings.TrimSpace(input.NextMarker), RequestId: uuid.NewString(),
	})
}

func (s *Server) listChannelVideos(ctx context.Context, _ *mcp.CallToolRequest, input listChannelVideosInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := validateLiveFields(input.NextMarker, input.Username); err != nil {
		return nil, nil, err
	}
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return nil, nil, errors.New("invalid_argument: username is required")
	}
	return s.callLive(ctx, "key:channels:feed_list", websocket.FeedListBody{
		Username: username, NextMarker: strings.TrimSpace(input.NextMarker),
	})
}

func (s *Server) getChannelVideoProfile(ctx context.Context, _ *mcp.CallToolRequest, input channelVideoProfileInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := validateLiveFields("", input.ObjectID, input.NonceID); err != nil {
		return nil, nil, err
	}
	if input.URL != "" && !isSupportedProfileURL(input.URL) {
		return nil, nil, failure("invalid_argument")
	}
	if strings.TrimSpace(input.URL) == "" && (strings.TrimSpace(input.ObjectID) == "" || strings.TrimSpace(input.NonceID) == "") {
		return nil, nil, errors.New("invalid_argument: object_id with nonce_id, or url is required")
	}
	key := "key:channels:feed_profile"
	if isSharedFeedURL(input.URL) {
		key = "key:channels:shared_feed_profile"
	}
	return s.callLive(ctx, key, websocket.FeedProfileBody{
		ObjectID: strings.TrimSpace(input.ObjectID), NonceID: strings.TrimSpace(input.NonceID), URL: strings.TrimSpace(input.URL),
	})
}

func (s *Server) listChannelComments(ctx context.Context, _ *mcp.CallToolRequest, input channelCommentsInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := validateLiveFields(input.NextMarker, input.ObjectID, input.NonceID, input.CommentID); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(input.ObjectID) == "" || (strings.TrimSpace(input.NonceID) == "" && strings.TrimSpace(input.CommentID) == "") {
		return nil, nil, errors.New("invalid_argument: object_id with nonce_id or comment_id is required")
	}
	return s.callLive(ctx, "key:channels:fetch_feed_comment_list", websocket.FeedCommentListBody{
		ObjectID: strings.TrimSpace(input.ObjectID), NonceID: strings.TrimSpace(input.NonceID), CommentID: strings.TrimSpace(input.CommentID), NextMarker: strings.TrimSpace(input.NextMarker),
	})
}

func (s *Server) resolveChannelShareLink(ctx context.Context, _ *mcp.CallToolRequest, input shareLinkInput) (*mcp.CallToolResult, map[string]any, error) {
	link := strings.TrimSpace(input.URL)
	if !isSharedFeedURL(link) {
		return nil, nil, errors.New("invalid_argument: unsupported WeChat Channels share URL")
	}
	return s.callLive(ctx, "key:channels:shared_feed_resolve", websocket.FeedProfileBody{URL: link})
}

func validateLiveFields(marker string, ids ...string) error {
	if len(marker) > maxMarkerBytes*3 {
		return failure("invalid_argument")
	}
	if _, err := url.PathUnescape(marker); err != nil {
		return failure("invalid_argument")
	}
	for _, id := range ids {
		if len(id) > 512 {
			return failure("invalid_argument")
		}
	}
	return nil
}

func isSupportedProfileURL(raw string) bool {
	if len(raw) > 8192 {
		return false
	}
	if isSharedFeedURL(raw) {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.User == nil && parsed.Port() == "" && strings.EqualFold(parsed.Hostname(), "channels.weixin.qq.com") && strings.TrimRight(parsed.Path, "/") == "/web/pages/feed"
}

func envelope(source, freshness string, data map[string]any, warnings []string) map[string]any {
	if warnings == nil {
		warnings = []string{}
	}
	return map[string]any{
		"source":      source,
		"observed_at": time.Now().UTC().Format(time.RFC3339),
		"freshness":   freshness,
		"partial":     len(warnings) > 0,
		"warnings":    warnings,
		"data":        data,
	}
}

func isSharedFeedURL(raw string) bool {
	if len(raw) > 8192 {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.Port() != "" {
		return false
	}
	if strings.EqualFold(parsed.Hostname(), "weixin.qq.com") {
		return strings.HasPrefix(strings.TrimRight(parsed.Path, "/"), "/sph/")
	}
	return strings.EqualFold(parsed.Hostname(), "channels.weixin.qq.com") &&
		strings.TrimRight(parsed.Path, "/") == "/finder-preview/pages/sph"
}
