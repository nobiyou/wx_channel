package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/response"
	"wx_channel/internal/websocket"
)

const (
	shareResolveChannelPage  = "page"
	maxShareResolveURLs      = 50
	shareResolveWorkers      = 4
	shareResolveBatchTimeout = 90 * time.Second
	shareResolveItemTimeout  = 20 * time.Second
)

type resolveSharedFeedLinksRequest struct {
	URLs []string `json:"urls"`
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
	InputURL  string `json:"inputUrl"`
	Channel   string `json:"channel,omitempty"`
	ErrorCode string `json:"errorCode"`
	Error     string `json:"error"`
}

type shareResolveJob struct {
	index    int
	inputURL string
}

type shareResolveResult struct {
	inputURL    string
	duplicateOf int
	duplicate   bool
	processed   bool
	resolved    *resolvedSharedFeedItem
	failed      *failedSharedFeedItem
}

// ResolveSharedFeedLinks resolves share links into batch-download-ready video items.
func (s *SearchService) ResolveSharedFeedLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req resolveSharedFeedLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "Invalid request body")
		return
	}

	if len(req.URLs) == 0 {
		response.Error(w, 400, "urls is required")
		return
	}
	if len(req.URLs) > maxShareResolveURLs {
		response.Error(w, 400, fmt.Sprintf("too many urls (max %d)", maxShareResolveURLs))
		return
	}

	results := make([]shareResolveResult, len(req.URLs))
	jobs := make([]shareResolveJob, 0, len(req.URLs))
	leaders := make(map[string]int, len(req.URLs))
	for index, rawURL := range req.URLs {
		inputURL := normalizeFeedProfileURL(rawURL)
		results[index].inputURL = inputURL
		if inputURL == "" {
			results[index].inputURL = strings.TrimSpace(rawURL)
			results[index].failed = &failedSharedFeedItem{
				InputURL:  results[index].inputURL,
				ErrorCode: "missing_url",
				Error:     "url is required",
			}
			results[index].processed = true
			continue
		}
		if !isSharedFeedURL(inputURL) {
			results[index].failed = &failedSharedFeedItem{
				InputURL:  inputURL,
				ErrorCode: "invalid_url",
				Error:     "invalid shared feed url",
			}
			results[index].processed = true
			continue
		}
		if leader, exists := leaders[inputURL]; exists {
			results[index].duplicateOf = leader
			results[index].duplicate = true
			continue
		}
		leaders[inputURL] = index
		jobs = append(jobs, shareResolveJob{index: index, inputURL: inputURL})
	}

	ctx, cancel := context.WithTimeout(r.Context(), shareResolveBatchTimeout)
	defer cancel()
	resolveSharedFeedJobs(ctx, s, jobs, results)

	for index := range results {
		if !results[index].duplicate {
			continue
		}
		leader := results[results[index].duplicateOf]
		results[index].processed = true
		if leader.resolved != nil {
			item := *leader.resolved
			item.InputURL = results[index].inputURL
			results[index].resolved = &item
		} else if leader.failed != nil {
			failure := *leader.failed
			failure.InputURL = results[index].inputURL
			results[index].failed = &failure
		}
	}

	resolved := make([]resolvedSharedFeedItem, 0, len(req.URLs))
	failed := make([]failedSharedFeedItem, 0, len(req.URLs))
	for _, result := range results {
		if result.resolved != nil {
			resolved = append(resolved, *result.resolved)
		} else if result.failed != nil {
			failed = append(failed, *result.failed)
		}
	}

	response.Success(w, map[string]interface{}{
		"resolved": resolved,
		"failed":   failed,
	})
}

func (s *SearchService) resolveSharedFeedLink(inputURL string) (resolvedSharedFeedItem, error) {
	return s.resolveSharedFeedLinkContext(context.Background(), inputURL)
}

func (s *SearchService) resolveSharedFeedLinkContext(ctx context.Context, inputURL string) (resolvedSharedFeedItem, error) {
	data, err := s.fetchSharedFeedResolveProfileContext(ctx, GetFeedProfileRequest{URL: inputURL}, shareResolveItemTimeout)
	if err != nil {
		return resolvedSharedFeedItem{Channel: shareResolveChannelPage}, err
	}

	return buildResolvedSharedFeedItemFromPage(inputURL, data)
}

func resolveSharedFeedJobs(ctx context.Context, service *SearchService, jobs []shareResolveJob, results []shareResolveResult) {
	if len(jobs) == 0 {
		return
	}
	workerCount := shareResolveWorkers
	if len(jobs) < workerCount {
		workerCount = len(jobs)
	}

	jobCh := make(chan shareResolveJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				itemCtx, cancel := context.WithTimeout(ctx, shareResolveItemTimeout)
				item, err := service.resolveSharedFeedLinkContext(itemCtx, job.inputURL)
				cancel()
				results[job.index].processed = true
				if err != nil {
					results[job.index].failed = &failedSharedFeedItem{
						InputURL:  job.inputURL,
						Channel:   item.Channel,
						ErrorCode: shareResolveErrorCode(err),
						Error:     err.Error(),
					}
					continue
				}
				results[job.index].resolved = &item
			}
		}()
	}
	wg.Wait()
}

func shareResolveErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if isClientUnavailableError(err) {
		return "no_ready_client"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, websocket.ErrRequestTimeout) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "request_canceled"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "decode page response") || strings.Contains(message, "missing media") {
		return "invalid_response"
	}
	return "resolve_failed"
}

func buildResolvedSharedFeedItemFromPage(inputURL string, raw []byte) (resolvedSharedFeedItem, error) {
	var payload struct {
		ErrCode int                    `json:"errCode"`
		ErrMsg  string                 `json:"errMsg"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return resolvedSharedFeedItem{Channel: shareResolveChannelPage}, fmt.Errorf("decode page response: %w", err)
	}
	if payload.ErrCode != 0 {
		message := strings.TrimSpace(payload.ErrMsg)
		if message == "" {
			message = fmt.Sprintf("errCode=%d", payload.ErrCode)
		}
		return resolvedSharedFeedItem{Channel: shareResolveChannelPage}, fmt.Errorf("%s", message)
	}

	object, _ := payload.Data["object"].(map[string]interface{})
	sceneInfo, _ := payload.Data["sceneInfo"].(map[string]interface{})
	objectDesc, _ := object["objectDesc"].(map[string]interface{})
	contact, _ := object["contact"].(map[string]interface{})

	item := resolvedSharedFeedItem{
		InputURL:   inputURL,
		Channel:    shareResolveChannelPage,
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
		return resolvedSharedFeedItem{Channel: shareResolveChannelPage}, fmt.Errorf("page response missing media")
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
		return resolvedSharedFeedItem{Channel: shareResolveChannelPage}, fmt.Errorf("page response missing media url")
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
