package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"wx_channel/internal/response"
	"wx_channel/internal/services"
)

const (
	shareResolveModeAuto    = "auto"
	shareResolveModePage    = "page"
	shareResolveModeBackend = "backend"
)

type resolveSharedFeedLinksRequest struct {
	URLs []string `json:"urls"`
	Mode string   `json:"mode"`
}

type resolvedSharedFeedItem struct {
	InputURL   string            `json:"inputUrl"`
	Channel    string            `json:"channel"`
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	AuthorName string            `json:"authorName"`
	URL        string            `json:"url"`
	Key        string            `json:"key,omitempty"`
	CoverURL   string            `json:"coverUrl,omitempty"`
	SourceURL  string            `json:"sourceUrl,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Resolution string            `json:"resolution,omitempty"`
	DurationMs int64             `json:"durationMs,omitempty"`
	Size       int64             `json:"size,omitempty"`
}

type failedSharedFeedItem struct {
	InputURL string `json:"inputUrl"`
	Channel  string `json:"channel,omitempty"`
	Error    string `json:"error"`
}

// ResolveSharedFeedLinks resolves share links into batch-download-ready video items.
func (s *SearchService) ResolveSharedFeedLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req resolveSharedFeedLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "Invalid request body")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = shareResolveModeAuto
	}
	if mode != shareResolveModeAuto && mode != shareResolveModePage && mode != shareResolveModeBackend {
		response.Error(w, 400, "invalid mode")
		return
	}
	if len(req.URLs) == 0 {
		response.Error(w, 400, "urls is required")
		return
	}

	backendEnabled := s.sphService != nil && s.sphService.Enabled()
	resolved := make([]resolvedSharedFeedItem, 0, len(req.URLs))
	failed := make([]failedSharedFeedItem, 0)

	for _, rawURL := range req.URLs {
		inputURL := normalizeFeedProfileURL(rawURL)
		if inputURL == "" {
			failed = append(failed, failedSharedFeedItem{
				InputURL: strings.TrimSpace(rawURL),
				Error:    "url is required",
			})
			continue
		}
		if !isSharedFeedURL(inputURL) {
			failed = append(failed, failedSharedFeedItem{
				InputURL: inputURL,
				Error:    "invalid shared feed url",
			})
			continue
		}

		item, err := s.resolveSharedFeedLink(r.Context(), inputURL, mode)
		if err != nil {
			failed = append(failed, failedSharedFeedItem{
				InputURL: inputURL,
				Channel:  item.Channel,
				Error:    err.Error(),
			})
			continue
		}
		resolved = append(resolved, item)
	}

	response.Success(w, map[string]interface{}{
		"mode":           mode,
		"backendEnabled": backendEnabled,
		"resolved":       resolved,
		"failed":         failed,
	})
}

func (s *SearchService) resolveSharedFeedLink(ctx context.Context, inputURL, mode string) (resolvedSharedFeedItem, error) {
	switch mode {
	case shareResolveModeBackend:
		return s.resolveSharedFeedViaBackend(ctx, inputURL)
	case shareResolveModePage:
		return s.resolveSharedFeedViaPage(inputURL)
	case shareResolveModeAuto:
		var backendErr error
		if s.sphService != nil && s.sphService.Enabled() {
			item, err := s.resolveSharedFeedViaBackend(ctx, inputURL)
			if err == nil {
				return item, nil
			}
			backendErr = err
		}

		item, err := s.resolveSharedFeedViaPage(inputURL)
		if err == nil {
			return item, nil
		}
		if backendErr != nil {
			return resolvedSharedFeedItem{Channel: shareResolveModePage}, fmt.Errorf("backend parse failed: %v; page parse failed: %v", backendErr, err)
		}
		return resolvedSharedFeedItem{Channel: shareResolveModePage}, err
	default:
		return resolvedSharedFeedItem{}, fmt.Errorf("invalid mode")
	}
}

func (s *SearchService) resolveSharedFeedViaBackend(ctx context.Context, inputURL string) (resolvedSharedFeedItem, error) {
	if s.sphService == nil || !s.sphService.Enabled() {
		return resolvedSharedFeedItem{Channel: shareResolveModeBackend}, fmt.Errorf("cloudflare.sphHostname or cloudflare.sphCookie not configured")
	}

	resp, err := s.sphService.FetchVideoProfile(ctx, inputURL)
	if err != nil {
		return resolvedSharedFeedItem{Channel: shareResolveModeBackend}, err
	}

	return buildResolvedSharedFeedItemFromBackend(inputURL, resp), nil
}

func (s *SearchService) resolveSharedFeedViaPage(inputURL string) (resolvedSharedFeedItem, error) {
	data, err := s.fetchSharedFeedResolveProfile(GetFeedProfileRequest{URL: inputURL})
	if err != nil {
		return resolvedSharedFeedItem{Channel: shareResolveModePage}, err
	}

	return buildResolvedSharedFeedItemFromPage(inputURL, data)
}

func buildResolvedSharedFeedItemFromBackend(inputURL string, resp *services.SphFeedResponse) resolvedSharedFeedItem {
	item := resolvedSharedFeedItem{
		InputURL:   inputURL,
		Channel:    shareResolveModeBackend,
		ID:         strings.TrimSpace(resp.Data.SceneInfo.DynamicExportID),
		Title:      strings.TrimSpace(resp.Data.FeedInfo.Description),
		AuthorName: strings.TrimSpace(resp.Data.AuthorInfo.Nickname),
		URL:        strings.TrimSpace(resp.Data.FeedInfo.OriginVideoURL),
		CoverURL:   strings.TrimSpace(resp.Data.FeedInfo.CoverURL),
		Headers: map[string]string{
			"Origin":  "https://channels.weixin.qq.com",
			"Referer": "https://channels.weixin.qq.com/finder-preview/pages/feed",
		},
	}
	if item.ID == "" {
		item.ID = "shared_feed"
	}
	if item.URL == "" {
		item.URL = strings.TrimSpace(resp.Data.FeedInfo.VideoURL)
	}
	if item.URL == "" {
		item.URL = strings.TrimSpace(resp.Data.FeedInfo.H264VideoInfo.VideoURL)
	}
	if item.URL == "" {
		item.URL = strings.TrimSpace(resp.Data.FeedInfo.H265VideoInfo.VideoURL)
	}
	if item.Title == "" {
		item.Title = item.ID
	}
	if item.AuthorName == "" {
		item.AuthorName = "未知作者"
	}
	return item
}

func buildResolvedSharedFeedItemFromPage(inputURL string, raw []byte) (resolvedSharedFeedItem, error) {
	var payload struct {
		ErrCode int                    `json:"errCode"`
		ErrMsg  string                 `json:"errMsg"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return resolvedSharedFeedItem{Channel: shareResolveModePage}, fmt.Errorf("decode page response: %w", err)
	}
	if payload.ErrCode != 0 {
		message := strings.TrimSpace(payload.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("errCode=%d", payload.ErrCode)
		}
		return resolvedSharedFeedItem{Channel: shareResolveModePage}, fmt.Errorf("%s", message)
	}

	object, _ := payload.Data["object"].(map[string]interface{})
	sceneInfo, _ := payload.Data["sceneInfo"].(map[string]interface{})
	objectDesc, _ := object["objectDesc"].(map[string]interface{})
	contact, _ := object["contact"].(map[string]interface{})

	item := resolvedSharedFeedItem{
		InputURL:   inputURL,
		Channel:    shareResolveModePage,
		ID:         strings.TrimSpace(stringValue(object["id"])),
		Title:      strings.TrimSpace(stringValue(objectDesc["description"])),
		AuthorName: firstNonEmptyString(stringValue(object["nickname"]), stringValue(contact["nickname"])),
		Headers: map[string]string{
			"Origin":  "https://channels.weixin.qq.com",
			"Referer": "https://channels.weixin.qq.com/finder-preview/pages/feed",
		},
	}
	if item.ID == "" {
		item.ID = strings.TrimSpace(stringValue(sceneInfo["dynamicExportId"]))
	}
	if item.ID == "" {
		item.ID = "shared_feed"
	}
	if item.Title == "" {
		item.Title = item.ID
	}
	if item.AuthorName == "" {
		item.AuthorName = "未知作者"
	}

	media := firstMediaMap(objectDesc["media"])
	if media == nil {
		return resolvedSharedFeedItem{Channel: shareResolveModePage}, fmt.Errorf("page response missing media")
	}

	item.URL = buildPageMediaURL(media)
	item.Key = strings.TrimSpace(firstNonEmptyString(stringValue(media["decodeKey"]), stringValue(media["decryptKey"])))
	item.CoverURL = strings.TrimSpace(firstNonEmptyString(
		stringValue(media["thumbUrl"]),
		stringValue(media["coverUrl"]),
		stringValue(media["fullThumbUrl"]),
	))
	item.Resolution = strings.TrimSpace(stringValue(media["videoResolution"]))
	if item.Resolution == "" {
		width := intValue(media["width"])
		height := intValue(media["height"])
		if width > 0 && height > 0 {
			item.Resolution = fmt.Sprintf("%dx%d", width, height)
		}
	}
	item.Size = firstPositiveInt64(int64Value(media["fileSize"]), int64Value(media["cdnFileSize"]))
	item.DurationMs = firstPositiveInt64(
		int64Value(media["durationMs"]),
		int64Value(media["videoDuration"]),
		int64Value(media["videoPlayLen"])*1000,
	)

	if item.URL == "" {
		return resolvedSharedFeedItem{Channel: shareResolveModePage}, fmt.Errorf("page response missing media url")
	}

	return item, nil
}

func firstMediaMap(v interface{}) map[string]interface{} {
	items, ok := v.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	media, _ := items[0].(map[string]interface{})
	return media
}

func buildPageMediaURL(media map[string]interface{}) string {
	baseURL := strings.TrimSpace(stringValue(media["url"]))
	if baseURL == "" {
		return ""
	}
	urlToken := strings.TrimSpace(stringValue(media["urlToken"]))
	if urlToken == "" {
		return baseURL
	}
	return baseURL + urlToken
}

func int64Value(v interface{}) int64 {
	switch value := v.(type) {
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		n, _ := value.Int64()
		return n
	default:
		return 0
	}
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
