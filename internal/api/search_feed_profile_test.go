package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wx_channel/internal/websocket"
)

func TestIsSharedFeedURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "weixin sph short link", raw: "https://weixin.qq.com/sph/A1b2C3d4", want: true},
		{name: "channels preview share link", raw: "https://channels.weixin.qq.com/finder-preview/pages/sph?id=A1b2C3d4", want: true},
		{name: "escaped share link", raw: "https%3A%2F%2Fchannels.weixin.qq.com%2Ffinder-preview%2Fpages%2Fsph%3Fid%3DA1b2C3d4", want: true},
		{name: "normal feed url", raw: "https://channels.weixin.qq.com/web/pages/feed?feed_id=finder_123&oid=Zm9v&nid=YmFy", want: false},
		{name: "empty", raw: "", want: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := isSharedFeedURL(test.raw); got != test.want {
				t.Fatalf("isSharedFeedURL(%q) = %t, want %t", test.raw, got, test.want)
			}
		})
	}
}

func TestGetFeedProfileUsesSharedRPCForShareLinks(t *testing.T) {
	t.Parallel()

	var calledKey string
	var calledBody websocket.FeedProfileBody
	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			calledKey = key
			if timeout != 60*time.Second {
				t.Fatalf("timeout = %s, want 60s", timeout)
			}
			var ok bool
			calledBody, ok = body.(websocket.FeedProfileBody)
			if !ok {
				t.Fatalf("unexpected body type: %T", body)
			}
			return []byte(`{"errCode":0,"data":{"object":{"id":"feed-1"}}}`), nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels/feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()
	service.GetFeedProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if calledKey != "key:channels:shared_feed_profile" {
		t.Fatalf("called key = %s, want key:channels:shared_feed_profile", calledKey)
	}
	if calledBody.URL != "https://weixin.qq.com/sph/A1b2C3d4" {
		t.Fatalf("called body url = %q", calledBody.URL)
	}
}

func TestGetFeedProfileKeepsNormalRPCForFeedURLs(t *testing.T) {
	t.Parallel()

	var calledKey string
	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			calledKey = key
			return []byte(`{"errCode":0,"data":{"object":{"id":"feed-1"}}}`), nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels/feed/profile?url=https://channels.weixin.qq.com/web/pages/feed?oid=Zm9v&nid=YmFy", nil)
	rec := httptest.NewRecorder()
	service.GetFeedProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if calledKey != "key:channels:feed_profile" {
		t.Fatalf("called key = %s, want key:channels:feed_profile", calledKey)
	}
}

func TestGetSharedFeedProfileUsesPageRPCOnly(t *testing.T) {
	t.Parallel()

	var calledKey string
	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			calledKey = key
			return []byte(`{"errCode":0,"data":{"object":{"id":"page-feed"}}}`), nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels/shared_feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()
	service.GetSharedFeedProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if calledKey != "key:channels:shared_feed_profile" {
		t.Fatalf("called key = %s, want key:channels:shared_feed_profile", calledKey)
	}
}

func TestResolveSharedFeedLinksUsesPageData(t *testing.T) {
	t.Parallel()

	var calledKey string
	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			calledKey = key
			return []byte(`{"errCode":0,"data":{"object":{"id":"page-export-id","nickname":"页面作者","contact":{"nickname":"页面作者"},"objectDesc":{"description":"页面分享视频","mediaType":4,"media":[{"url":"https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&token=tok456","decodeKey":"987654321","thumbUrl":"https://cdn.example.com/page-cover.jpg","videoPlayLen":12,"fileSize":1048576,"videoResolution":"1080p"}]}} ,"sceneInfo":{"dynamicExportId":"page-export-id"}}}`), nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/channels/share/resolve", strings.NewReader(`{"urls":["https://weixin.qq.com/sph/A1b2C3d4"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	service.ResolveSharedFeedLinks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if calledKey != "key:channels:shared_feed_resolve" {
		t.Fatalf("called key = %s, want key:channels:shared_feed_resolve", calledKey)
	}

	var body struct {
		Code int `json:"code"`
		Data struct {
			Resolved []struct {
				Channel    string `json:"channel"`
				ID         string `json:"id"`
				Title      string `json:"title"`
				AuthorName string `json:"authorName"`
				URL        string `json:"url"`
				Key        string `json:"key"`
				CoverURL   string `json:"coverUrl"`
				Resolution string `json:"resolution"`
				DurationMs int64  `json:"durationMs"`
				Size       int64  `json:"size"`
			} `json:"resolved"`
			Failed []interface{} `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 {
		t.Fatalf("code = %d, want 0", body.Code)
	}
	if len(body.Data.Resolved) != 1 {
		t.Fatalf("resolved count = %d, want 1", len(body.Data.Resolved))
	}
	item := body.Data.Resolved[0]
	if item.Channel != "page" || item.ID != "page-export-id" || item.Title != "页面分享视频" || item.AuthorName != "页面作者" {
		t.Fatalf("unexpected resolved identity: %+v", item)
	}
	if item.URL != "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&token=tok456" {
		t.Fatalf("url = %q", item.URL)
	}
	if item.Key != "987654321" || item.CoverURL != "https://cdn.example.com/page-cover.jpg" {
		t.Fatalf("unexpected media metadata: %+v", item)
	}
	if item.Resolution != "1080p" || item.DurationMs != 12000 || item.Size != 1048576 {
		t.Fatalf("unexpected media dimensions: %+v", item)
	}
	if len(body.Data.Failed) != 0 {
		t.Fatalf("failed count = %d, want 0", len(body.Data.Failed))
	}
}

func TestResolveSharedFeedLinksRejectsNonShareURL(t *testing.T) {
	t.Parallel()

	service := &SearchService{}
	req := httptest.NewRequest(http.MethodPost, "/api/channels/share/resolve", strings.NewReader(`{"urls":["https://example.com/video"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	service.ResolveSharedFeedLinks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with per-item failure", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid shared feed url") {
		t.Fatalf("response = %s", rec.Body.String())
	}
}

func TestGetSharedFeedProfileTranslatesPageContextMismatch(t *testing.T) {
	t.Parallel()

	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			return nil, errors.New("API error (code=-70003): JSAPI_JSONPARSE_FAILED")
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/channels/shared_feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()
	service.GetSharedFeedProfile(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "页面上下文") {
		t.Fatalf("response = %s", rec.Body.String())
	}
}

func TestRegisterRoutesDoesNotExposePureBackendParser(t *testing.T) {
	t.Parallel()

	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			return []byte(`{"errCode":0,"data":{"object":{"id":"page-feed"}}}`), nil
		},
	}
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/channels/parse_sph?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("parse_sph status = %d, want 404", rec.Code)
	}
}
