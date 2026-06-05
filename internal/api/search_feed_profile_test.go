package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wx_channel/internal/services"
	"wx_channel/internal/websocket"
)

type stubSharedFeedProfileService struct {
	enabled bool
	fetch   func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error)
}

func (s stubSharedFeedProfileService) Enabled() bool {
	return s.enabled
}

func (s stubSharedFeedProfileService) FetchVideoProfile(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
	if s.fetch == nil {
		return nil, errors.New("fetch not implemented")
	}
	return s.fetch(ctx, shareURL)
}

func TestIsSharedFeedURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "weixin sph short link",
			raw:  "https://weixin.qq.com/sph/A1b2C3d4",
			want: true,
		},
		{
			name: "channels preview share link",
			raw:  "https://channels.weixin.qq.com/finder-preview/pages/sph?id=A1b2C3d4",
			want: true,
		},
		{
			name: "escaped share link",
			raw:  "https%3A%2F%2Fchannels.weixin.qq.com%2Ffinder-preview%2Fpages%2Fsph%3Fid%3DA1b2C3d4",
			want: true,
		},
		{
			name: "normal feed url",
			raw:  "https://channels.weixin.qq.com/web/pages/feed?feed_id=finder_123&oid=Zm9v&nid=YmFy",
			want: false,
		},
		{
			name: "empty",
			raw:  "",
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isSharedFeedURL(tc.raw); got != tc.want {
				t.Fatalf("isSharedFeedURL(%q) = %t, want %t", tc.raw, got, tc.want)
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

			req, ok := body.(websocket.FeedProfileBody)
			if !ok {
				t.Fatalf("unexpected body type: %T", body)
			}
			calledBody = req
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

func TestRegisterRoutesSupportsChannelsSharedFeedProfile(t *testing.T) {
	t.Parallel()

	var calledKey string

	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			calledKey = key
			return []byte(`{"errCode":0,"data":{"object":{"id":"feed-1"}}}`), nil
		},
	}

	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/channels/shared_feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if calledKey != "key:channels:shared_feed_profile" {
		t.Fatalf("called key = %s, want key:channels:shared_feed_profile", calledKey)
	}
}

func TestGetSharedFeedProfileUsesBackendParseWhenConfigured(t *testing.T) {
	t.Parallel()

	service := &SearchService{
		sphService: stubSharedFeedProfileService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				if shareURL != "https://weixin.qq.com/sph/A1b2C3d4" {
					t.Fatalf("shareURL = %q", shareURL)
				}
				return &services.SphFeedResponse{
					ErrCode: 0,
					ErrMsg:  "ok",
					Data: services.SphFeedData{
						SceneInfo: services.SphSceneInfo{DynamicExportID: "export-id-123"},
						AuthorInfo: services.SphAuthorInfo{
							Nickname: "作者A",
						},
						FeedInfo: services.SphFeedInfo{
							VideoURL:       "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz&foo=1",
							OriginVideoURL: "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz",
							Description:    "分享视频标题",
							MediaType:      4,
							CoverURL:       "https://cdn.example.com/cover.jpg",
							CreateTime:     1717200000,
						},
					},
				}, nil
			},
		},
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			t.Fatalf("callAPI should not be used when backend parse succeeds")
			return nil, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels/shared_feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()

	service.GetSharedFeedProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Code int `json:"code"`
		Data struct {
			ErrCode int `json:"errCode"`
			Data    struct {
				Object struct {
					ID         string `json:"id"`
					ObjectDesc struct {
						Description string `json:"description"`
						Media       []struct {
							URL string `json:"url"`
						} `json:"media"`
					} `json:"objectDesc"`
				} `json:"object"`
				SceneInfo struct {
					DynamicExportID string `json:"dynamicExportId"`
				} `json:"sceneInfo"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Code != 0 {
		t.Fatalf("code = %d, want 0", body.Code)
	}
	if body.Data.ErrCode != 0 {
		t.Fatalf("inner errCode = %d, want 0", body.Data.ErrCode)
	}
	if body.Data.Data.SceneInfo.DynamicExportID != "export-id-123" {
		t.Fatalf("dynamicExportId = %q", body.Data.Data.SceneInfo.DynamicExportID)
	}
	if body.Data.Data.Object.ID != "export-id-123" {
		t.Fatalf("object id = %q", body.Data.Data.Object.ID)
	}
	if body.Data.Data.Object.ObjectDesc.Description != "分享视频标题" {
		t.Fatalf("description = %q", body.Data.Data.Object.ObjectDesc.Description)
	}
	if len(body.Data.Data.Object.ObjectDesc.Media) != 1 {
		t.Fatalf("media count = %d, want 1", len(body.Data.Data.Object.ObjectDesc.Media))
	}
	if body.Data.Data.Object.ObjectDesc.Media[0].URL != "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz" {
		t.Fatalf("media url = %q", body.Data.Data.Object.ObjectDesc.Media[0].URL)
	}
}

func TestGetSharedFeedProfileFallsBackToPageAPIWhenBackendParseFails(t *testing.T) {
	t.Parallel()

	var calledKey string

	service := &SearchService{
		sphService: stubSharedFeedProfileService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				return nil, errors.New("worker unavailable")
			},
		},
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			calledKey = key
			return []byte(`{"errCode":0,"data":{"object":{"id":"fallback-feed"}}}`), nil
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

func TestParseSphReturnsBackendFeedResponse(t *testing.T) {
	t.Parallel()

	service := &SearchService{
		sphService: stubSharedFeedProfileService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				return &services.SphFeedResponse{
					ErrCode: 0,
					ErrMsg:  "ok",
					Data: services.SphFeedData{
						SceneInfo: services.SphSceneInfo{DynamicExportID: "export-id-123"},
						FeedInfo: services.SphFeedInfo{
							VideoURL:       "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz&foo=1",
							OriginVideoURL: "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz",
						},
					},
				}, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels/parse_sph?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()

	service.ParseSph(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Code int `json:"code"`
		Data struct {
			Data struct {
				SceneInfo struct {
					DynamicExportID string `json:"dynamicExportId"`
				} `json:"sceneInfo"`
				FeedInfo struct {
					OriginVideoURL string `json:"originVideoUrl"`
				} `json:"feedInfo"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Code != 0 {
		t.Fatalf("code = %d, want 0", body.Code)
	}
	if body.Data.Data.SceneInfo.DynamicExportID != "export-id-123" {
		t.Fatalf("dynamicExportId = %q", body.Data.Data.SceneInfo.DynamicExportID)
	}
	if body.Data.Data.FeedInfo.OriginVideoURL != "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz" {
		t.Fatalf("originVideoUrl = %q", body.Data.Data.FeedInfo.OriginVideoURL)
	}
}

func TestResolveSharedFeedLinksUsesBackendMode(t *testing.T) {
	t.Parallel()

	service := &SearchService{
		sphService: stubSharedFeedProfileService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				if shareURL != "https://weixin.qq.com/sph/A1b2C3d4" {
					t.Fatalf("shareURL = %q", shareURL)
				}
				return &services.SphFeedResponse{
					ErrCode: 0,
					Data: services.SphFeedData{
						SceneInfo: services.SphSceneInfo{DynamicExportID: "export-id-123"},
						AuthorInfo: services.SphAuthorInfo{
							Nickname: "后端作者",
						},
						FeedInfo: services.SphFeedInfo{
							VideoURL:       "https://cdn.example.com/video.mp4?foo=1&encfilekey=abc&token=xyz&bar=2",
							OriginVideoURL: "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz",
							Description:    "后端分享视频",
							CoverURL:       "https://cdn.example.com/cover.jpg",
							CreateTime:     1717200000,
						},
					},
				}, nil
			},
		},
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			t.Fatalf("callAPI should not be used in backend mode")
			return nil, nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/channels/share/resolve", strings.NewReader(`{"mode":"backend","urls":["https://weixin.qq.com/sph/A1b2C3d4"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	service.ResolveSharedFeedLinks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Code int `json:"code"`
		Data struct {
			BackendEnabled bool `json:"backendEnabled"`
			Resolved       []struct {
				InputURL   string `json:"inputUrl"`
				Channel    string `json:"channel"`
				ID         string `json:"id"`
				Title      string `json:"title"`
				AuthorName string `json:"authorName"`
				URL        string `json:"url"`
				Key        string `json:"key"`
				CoverURL   string `json:"coverUrl"`
				SourceURL  string `json:"sourceUrl"`
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
	if !body.Data.BackendEnabled {
		t.Fatalf("backendEnabled = false, want true")
	}
	if len(body.Data.Resolved) != 1 {
		t.Fatalf("resolved count = %d, want 1", len(body.Data.Resolved))
	}
	item := body.Data.Resolved[0]
	if item.Channel != "backend" {
		t.Fatalf("channel = %q, want backend", item.Channel)
	}
	if item.ID != "export-id-123" {
		t.Fatalf("id = %q, want export-id-123", item.ID)
	}
	if item.Title != "后端分享视频" {
		t.Fatalf("title = %q", item.Title)
	}
	if item.AuthorName != "后端作者" {
		t.Fatalf("authorName = %q", item.AuthorName)
	}
	if item.URL != "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz" {
		t.Fatalf("url = %q", item.URL)
	}
	if item.Key != "" {
		t.Fatalf("key = %q, want empty", item.Key)
	}
	if item.SourceURL != "" {
		t.Fatalf("sourceUrl = %q, want empty", item.SourceURL)
	}
	if len(body.Data.Failed) != 0 {
		t.Fatalf("failed count = %d, want 0", len(body.Data.Failed))
	}
}

func TestResolveSharedFeedLinksAutoFallsBackToPage(t *testing.T) {
	t.Parallel()

	var calledKey string

	service := &SearchService{
		sphService: stubSharedFeedProfileService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				return nil, errors.New("worker unavailable")
			},
		},
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			calledKey = key
			return []byte(`{"errCode":0,"data":{"object":{"id":"page-export-id","nickname":"页面作者","contact":{"nickname":"页面作者"},"objectDesc":{"description":"页面分享视频","mediaType":4,"media":[{"url":"https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&token=tok456","decodeKey":"987654321","thumbUrl":"https://cdn.example.com/page-cover.jpg","videoPlayLen":12,"fileSize":1048576,"videoResolution":"1080p"}]}},"sceneInfo":{"dynamicExportId":"page-export-id"}}}`), nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/channels/share/resolve", strings.NewReader(`{"mode":"auto","urls":["https://weixin.qq.com/sph/A1b2C3d4"]}`))
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
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Resolved) != 1 {
		t.Fatalf("resolved count = %d, want 1", len(body.Data.Resolved))
	}
	item := body.Data.Resolved[0]
	if item.Channel != "page" {
		t.Fatalf("channel = %q, want page", item.Channel)
	}
	if item.ID != "page-export-id" {
		t.Fatalf("id = %q", item.ID)
	}
	if item.Title != "页面分享视频" {
		t.Fatalf("title = %q", item.Title)
	}
	if item.AuthorName != "页面作者" {
		t.Fatalf("authorName = %q", item.AuthorName)
	}
	if item.URL != "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&token=tok456" {
		t.Fatalf("url = %q", item.URL)
	}
	if item.Key != "987654321" {
		t.Fatalf("key = %q", item.Key)
	}
	if item.CoverURL != "https://cdn.example.com/page-cover.jpg" {
		t.Fatalf("coverUrl = %q", item.CoverURL)
	}
	if item.Resolution != "1080p" {
		t.Fatalf("resolution = %q", item.Resolution)
	}
	if item.DurationMs != 12000 {
		t.Fatalf("durationMs = %d, want 12000", item.DurationMs)
	}
	if item.Size != 1048576 {
		t.Fatalf("size = %d, want 1048576", item.Size)
	}
}

func TestResolveSharedFeedLinksPageModeSkipsBackend(t *testing.T) {
	t.Parallel()

	var calledKey string

	service := &SearchService{
		sphService: stubSharedFeedProfileService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				t.Fatalf("backend fetch should not be used in page mode")
				return nil, nil
			},
		},
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			calledKey = key
			return []byte(`{"errCode":0,"data":{"object":{"id":"page-only-id","nickname":"页面作者","objectDesc":{"description":"页面模式视频","mediaType":4,"media":[{"url":"https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc123&token=tok456","thumbUrl":"https://cdn.example.com/page-cover.jpg"}]}}}}`), nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/channels/share/resolve", strings.NewReader(`{"mode":"page","urls":["https://weixin.qq.com/sph/A1b2C3d4"]}`))
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
				Channel string `json:"channel"`
				Title   string `json:"title"`
			} `json:"resolved"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Resolved) != 1 {
		t.Fatalf("resolved count = %d, want 1", len(body.Data.Resolved))
	}
	if body.Data.Resolved[0].Channel != "page" {
		t.Fatalf("channel = %q, want page", body.Data.Resolved[0].Channel)
	}
	if body.Data.Resolved[0].Title != "页面模式视频" {
		t.Fatalf("title = %q", body.Data.Resolved[0].Title)
	}
}

func TestRegisterRoutesSupportsChannelsParseSph(t *testing.T) {
	t.Parallel()

	service := &SearchService{
		sphService: stubSharedFeedProfileService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				return &services.SphFeedResponse{
					ErrCode: 0,
					Data: services.SphFeedData{
						SceneInfo: services.SphSceneInfo{DynamicExportID: "export-id-route"},
					},
				}, nil
			},
		},
	}

	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/channels/parse_sph?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRegisterRoutesSupportsChannelsShareResolve(t *testing.T) {
	t.Parallel()

	service := &SearchService{
		sphService: stubSharedFeedProfileService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				return &services.SphFeedResponse{
					ErrCode: 0,
					Data: services.SphFeedData{
						SceneInfo: services.SphSceneInfo{DynamicExportID: "export-id-route"},
						AuthorInfo: services.SphAuthorInfo{
							Nickname: "作者A",
						},
						FeedInfo: services.SphFeedInfo{
							OriginVideoURL: "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz",
							Description:    "路由测试视频",
						},
					},
				}, nil
			},
		},
	}

	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/channels/share/resolve", strings.NewReader(`{"mode":"backend","urls":["https://weixin.qq.com/sph/A1b2C3d4"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestBuildSharedFeedProfileCompatResponseFallsBackDynamicExportID(t *testing.T) {
	t.Parallel()

	result := services.BuildSharedFeedProfileCompatResponse(&services.SphFeedResponse{
		ErrCode: 0,
		Data: services.SphFeedData{
			FeedInfo: services.SphFeedInfo{
				VideoURL: "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz",
			},
		},
	})

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data should be a map")
	}
	sceneInfo, ok := data["sceneInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("sceneInfo should be a map")
	}
	if got := sceneInfo["dynamicExportId"]; got != "shared_feed" {
		t.Fatalf("dynamicExportId = %#v, want shared_feed", got)
	}
}
