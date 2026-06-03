package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSphServiceEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  SphServiceConfig
		want bool
	}{
		{
			name: "nil config",
			cfg:  SphServiceConfig{},
			want: false,
		},
		{
			name: "disabled when empty",
			cfg:  SphServiceConfig{},
			want: false,
		},
		{
			name: "enabled by worker host",
			cfg:  SphServiceConfig{SphHostname: "worker.example.com"},
			want: true,
		},
		{
			name: "enabled by yuanbao cookie",
			cfg:  SphServiceConfig{SphCookie: "session=abc"},
			want: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := &SphService{
				cfgProvider: func() SphServiceConfig {
					return tc.cfg
				},
			}

			if got := service.Enabled(); got != tc.want {
				t.Fatalf("Enabled() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestCleanSharedVideoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "keep encfilekey and token only",
			raw:  "https://cdn.example.com/video.mp4?foo=1&encfilekey=abc123&token=tok456&bar=2",
			want: "https://cdn.example.com/video.mp4?encfilekey=abc123&token=tok456",
		},
		{
			name: "missing token",
			raw:  "https://cdn.example.com/video.mp4?encfilekey=abc123",
			want: "",
		},
		{
			name: "invalid url",
			raw:  "://bad-url",
			want: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := CleanSharedVideoURL(tc.raw); got != tc.want {
				t.Fatalf("CleanSharedVideoURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestFetchViaWorkerCleansOriginVideoURLFromVideoURL(t *testing.T) {
	t.Parallel()

	var seenURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/fetch_video_profile" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		seenURL = body.URL

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SphFeedResponse{
			ErrCode: 0,
			ErrMsg:  "ok",
			Data: SphFeedData{
				SceneInfo: SphSceneInfo{DynamicExportID: "export-id-1"},
				FeedInfo: SphFeedInfo{
					VideoURL:       "https://cdn.example.com/video.mp4?foo=1&encfilekey=abc&token=xyz&bar=2",
					OriginVideoURL: "",
				},
			},
		})
	}))
	defer srv.Close()

	service := &SphService{client: srv.Client()}
	resp, err := service.fetchViaWorker(context.Background(), srv.URL, "https://weixin.qq.com/sph/A1b2C3d4")
	if err != nil {
		t.Fatalf("fetchViaWorker error: %v", err)
	}

	if seenURL != "https://weixin.qq.com/sph/A1b2C3d4" {
		t.Fatalf("worker share url = %q", seenURL)
	}
	if resp.Data.FeedInfo.OriginVideoURL != "https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz" {
		t.Fatalf("originVideoUrl = %q", resp.Data.FeedInfo.OriginVideoURL)
	}
}

func TestFetchDirectUsesPlayableTokenAndExportFallbacks(t *testing.T) {
	t.Parallel()

	var parseCookie string
	var requestedExportID string
	var requestedToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/parse":
			parseCookie = r.Header.Get("cookie")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(SphParseResponse{
				Code: 0,
				Msg:  "ok",
				Data: SphParseData{
					WxExportID:  "fallback-export-id",
					PlayableURL: "https://channels.weixin.qq.com/finder-preview/pages/feed?token=general-token-123",
				},
			})
		case "/feed":
			var body struct {
				BaseReq struct {
					GeneralToken string `json:"generalToken"`
				} `json:"baseReq"`
				ExportID string `json:"exportId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode feed body: %v", err)
			}
			requestedExportID = body.ExportID
			requestedToken = body.BaseReq.GeneralToken

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(SphFeedResponse{
				ErrCode: 0,
				ErrMsg:  "ok",
				Data: SphFeedData{
					SceneInfo: SphSceneInfo{},
					FeedInfo: SphFeedInfo{
						VideoURL: "https://cdn.example.com/video.mp4?foo=1&encfilekey=abc123&token=tok456&bar=2",
					},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	service := &SphService{
		client:      srv.Client(),
		parseURL:    srv.URL + "/parse",
		feedInfoURL: srv.URL + "/feed",
	}

	resp, err := service.fetchDirect(context.Background(), "https://weixin.qq.com/sph/A1b2C3d4", "session=abc")
	if err != nil {
		t.Fatalf("fetchDirect error: %v", err)
	}

	if parseCookie != "session=abc" {
		t.Fatalf("parse cookie = %q", parseCookie)
	}
	if requestedExportID != "fallback-export-id" {
		t.Fatalf("exportId = %q", requestedExportID)
	}
	if requestedToken != "general-token-123" {
		t.Fatalf("generalToken = %q", requestedToken)
	}
	if resp.Data.SceneInfo.DynamicExportID != "fallback-export-id" {
		t.Fatalf("dynamicExportId = %q", resp.Data.SceneInfo.DynamicExportID)
	}
	if resp.Data.FeedInfo.OriginVideoURL != "https://cdn.example.com/video.mp4?encfilekey=abc123&token=tok456" {
		t.Fatalf("originVideoUrl = %q", resp.Data.FeedInfo.OriginVideoURL)
	}
}

func TestNormalizeSphHostnameAndPlayableParams(t *testing.T) {
	t.Parallel()

	if got := normalizeSphHostname("worker.example.com/"); got != "https://worker.example.com" {
		t.Fatalf("normalizeSphHostname() = %q", got)
	}

	token, exportID := extractPlayableTokenAndExportID("https://channels.weixin.qq.com/finder-preview/pages/feed?token=abc&eid=export-123")
	if token != "abc" || exportID != "export-123" {
		t.Fatalf("extractPlayableTokenAndExportID() = (%q, %q)", token, exportID)
	}

	token, exportID = extractPlayableTokenAndExportID("bad:// url")
	if token != "" || exportID != "" {
		t.Fatalf("invalid playable url should return empty values, got (%q, %q)", token, exportID)
	}
}

func TestFetchVideoProfileRejectsEmptyURL(t *testing.T) {
	t.Parallel()

	service := &SphService{
		cfgProvider: func() SphServiceConfig {
			return SphServiceConfig{SphCookie: "session=abc"}
		},
	}

	_, err := service.FetchVideoProfile(context.Background(), "   ")
	if err == nil || !strings.Contains(err.Error(), "url parameter is required") {
		t.Fatalf("FetchVideoProfile error = %v", err)
	}
}

func TestNewSphServiceWithConfigProviderUsesInjectedConfig(t *testing.T) {
	t.Parallel()

	service := NewSphServiceWithConfigProvider(func() SphServiceConfig {
		return SphServiceConfig{SphHostname: "worker.example.com"}
	})

	if !service.Enabled() {
		t.Fatalf("expected injected config to enable service")
	}
}
