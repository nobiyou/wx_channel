package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"wx_channel/internal/websocket"
)

func shareResolveTestResponse(id string) []byte {
	return []byte(fmt.Sprintf(`{"errCode":0,"data":{"object":{"id":"%s","nickname":"author","objectDesc":{"description":"title","media":[{"url":"https://video.example/%s.mp4","decodeKey":"key"}]}}}}`, id, id))
}

func shareResolveTestRequest(t *testing.T, urls []string) *http.Request {
	t.Helper()
	body, err := json.Marshal(resolveSharedFeedLinksRequest{URLs: urls})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/channels/share/resolve", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestResolveSharedFeedLinksUsesBoundedConcurrencyAndPreservesOrder(t *testing.T) {
	var calls atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32
	service := &SearchService{
		callAPIContext: func(_ context.Context, _ string, body interface{}, _ time.Duration) ([]byte, error) {
			calls.Add(1)
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			active.Add(-1)
			request := body.(websocket.FeedProfileBody)
			id := strings.TrimPrefix(request.URL, "https://weixin.qq.com/sph/")
			return shareResolveTestResponse(id), nil
		},
	}

	urls := []string{
		"https://weixin.qq.com/sph/A",
		"https://weixin.qq.com/sph/B",
		"https://weixin.qq.com/sph/C",
		"https://weixin.qq.com/sph/D",
	}
	recorder := httptest.NewRecorder()
	service.ResolveSharedFeedLinks(recorder, shareResolveTestRequest(t, urls))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if calls.Load() != int32(len(urls)) {
		t.Fatalf("calls = %d, want %d", calls.Load(), len(urls))
	}
	if maxActive.Load() < 2 || maxActive.Load() > shareResolveWorkers {
		t.Fatalf("max active = %d, want between 2 and %d", maxActive.Load(), shareResolveWorkers)
	}

	var response struct {
		Data struct {
			Resolved []resolvedSharedFeedItem `json:"resolved"`
			Failed   []failedSharedFeedItem   `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Resolved) != len(urls) || len(response.Data.Failed) != 0 {
		t.Fatalf("unexpected result: %#v", response.Data)
	}
	for index, item := range response.Data.Resolved {
		want := urls[index]
		if item.InputURL != want {
			t.Fatalf("resolved[%d].inputUrl = %q, want %q", index, item.InputURL, want)
		}
	}
}

func TestResolveSharedFeedLinksDeduplicatesRequests(t *testing.T) {
	var calls atomic.Int32
	service := &SearchService{
		callAPIContext: func(_ context.Context, _ string, body interface{}, _ time.Duration) ([]byte, error) {
			calls.Add(1)
			request := body.(websocket.FeedProfileBody)
			return shareResolveTestResponse(strings.TrimPrefix(request.URL, "https://weixin.qq.com/sph/")), nil
		},
	}
	url := "https://weixin.qq.com/sph/DUP"
	recorder := httptest.NewRecorder()
	service.ResolveSharedFeedLinks(recorder, shareResolveTestRequest(t, []string{url, url}))

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	var response struct {
		Data struct {
			Resolved []resolvedSharedFeedItem `json:"resolved"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Resolved) != 2 {
		t.Fatalf("resolved count = %d, want 2", len(response.Data.Resolved))
	}
}

func TestResolveSharedFeedLinksReturnsStructuredUnavailableError(t *testing.T) {
	service := &SearchService{
		callAPIContext: func(context.Context, string, interface{}, time.Duration) ([]byte, error) {
			return nil, websocket.ErrNoReadyClient
		},
	}
	recorder := httptest.NewRecorder()
	service.ResolveSharedFeedLinks(recorder, shareResolveTestRequest(t, []string{
		"https://weixin.qq.com/sph/A",
	}))

	var response struct {
		Data struct {
			Failed []failedSharedFeedItem `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Failed) != 1 || response.Data.Failed[0].ErrorCode != "no_ready_client" {
		t.Fatalf("failed result = %#v", response.Data.Failed)
	}
}

func TestShareResolveErrorCodeMapsHubTimeout(t *testing.T) {
	if got := shareResolveErrorCode(fmt.Errorf("wrapped: %w", websocket.ErrRequestTimeout)); got != "timeout" {
		t.Fatalf("shareResolveErrorCode(ErrRequestTimeout) = %q, want timeout", got)
	}
}

func TestIsSharedFeedURLRejectsLookalikes(t *testing.T) {
	for _, rawURL := range []string{
		"https://evil.example/weixin.qq.com/sph/A",
		"http://weixin.qq.com/sph/A",
		"https://weixin.qq.com/sph/A/extra",
		"https://channels.weixin.qq.com/finder-preview/pages/sph?id=bad.value",
	} {
		if isSharedFeedURL(rawURL) {
			t.Fatalf("isSharedFeedURL(%q) = true, want false", rawURL)
		}
	}
}

func TestResolveSharedFeedLinksLimitsBatchSize(t *testing.T) {
	urls := make([]string, maxShareResolveURLs+1)
	for index := range urls {
		urls[index] = fmt.Sprintf("https://weixin.qq.com/sph/%d", index)
	}
	recorder := httptest.NewRecorder()
	service := &SearchService{}
	service.ResolveSharedFeedLinks(recorder, shareResolveTestRequest(t, urls))

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "too many urls") {
		t.Fatalf("status = %d, response = %s", recorder.Code, recorder.Body.String())
	}
}
