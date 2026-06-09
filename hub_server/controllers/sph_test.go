package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wx_channel/hub_server/database"
	"wx_channel/hub_server/middleware"
	"wx_channel/hub_server/ws"
	"wx_channel/internal/services"

	"github.com/coder/websocket"
)

type stubHubSharedFeedService struct {
	enabled bool
	fetch   func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error)
}

func (s stubHubSharedFeedService) Enabled() bool {
	return s.enabled
}

func (s stubHubSharedFeedService) FetchVideoProfile(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
	if s.fetch == nil {
		return nil, nil
	}
	return s.fetch(ctx, shareURL)
}

func closeSphTestDB(t *testing.T) {
	t.Helper()

	if database.DB == nil {
		return
	}

	sqlDB, err := database.DB.DB()
	if err != nil {
		t.Fatalf("DB.DB(): %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("Close DB: %v", err)
	}

	database.DB = nil
}

func initSphTestDB(t *testing.T) {
	t.Helper()

	closeSphTestDB(t)

	dbPath := filepath.Join(t.TempDir(), "hub_server_test.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB(%s): %v", dbPath, err)
	}

	t.Cleanup(func() {
		closeSphTestDB(t)
	})
}

func TestParseSphRequiresURL(t *testing.T) {
	initSphTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/channels/parse_sph", nil)
	rec := httptest.NewRecorder()

	ParseSph(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "url is required") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestParseSphRequiresHubConfig(t *testing.T) {
	initSphTestDB(t)

	t.Setenv("HUB_SPH_COOKIE", "")
	t.Setenv("HUB_SPH_HOSTNAME", "")

	req := httptest.NewRequest(http.MethodGet, "/api/channels/parse_sph?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()

	ParseSph(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "sph settings not configured") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestParseSphRejectsInvalidJSONBody(t *testing.T) {
	initSphTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/channels/parse_sph", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	ParseSph(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestParseSphPostReadsURLFromBody(t *testing.T) {
	initSphTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fetch_video_profile" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errCode":0,"errMsg":"ok","data":{"sceneInfo":{"dynamicExportId":"export-id-123"},"feedInfo":{"videoUrl":"https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz"}}}`))
	}))
	defer srv.Close()

	t.Setenv("HUB_SPH_HOSTNAME", srv.URL)
	t.Setenv("HUB_SPH_COOKIE", "")

	body := strings.NewReader(`{"url":"https://weixin.qq.com/sph/A1b2C3d4"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/channels/parse_sph", body)
	rec := httptest.NewRecorder()

	ParseSph(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"dynamicExportId":"export-id-123"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestGetSharedFeedProfileReturnsCompatShape(t *testing.T) {
	initSphTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fetch_video_profile" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errCode":0,"errMsg":"ok","data":{"sceneInfo":{"dynamicExportId":"export-id-123"},"authorInfo":{"nickname":"作者A","headImgUrl":"https://cdn.example.com/avatar.jpg"},"feedInfo":{"videoUrl":"https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz","originVideoUrl":"https://cdn.example.com/video.mp4?encfilekey=abc&token=xyz","description":"分享视频标题","coverUrl":"https://cdn.example.com/cover.jpg","createtime":1718000000}}}`))
	}))
	defer srv.Close()

	t.Setenv("HUB_SPH_HOSTNAME", srv.URL)
	t.Setenv("HUB_SPH_COOKIE", "")

	req := httptest.NewRequest(http.MethodGet, "/api/channels/shared_feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()

	GetSharedFeedProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"description":"分享视频标题"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"headImgUrl":"https://cdn.example.com/avatar.jpg"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"url":"https://cdn.example.com/video.mp4?encfilekey=abc\u0026token=xyz"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestGetSharedFeedProfileFallsBackToPageWhenBackendParseFails(t *testing.T) {
	initSphTestDB(t)

	originalFactory := newHubSharedFeedService
	originalPageFetcher := fetchSharedFeedProfileViaPage
	t.Cleanup(func() {
		newHubSharedFeedService = originalFactory
		fetchSharedFeedProfileViaPage = originalPageFetcher
	})

	newHubSharedFeedService = func() hubSharedFeedService {
		return stubHubSharedFeedService{
			enabled: true,
			fetch: func(ctx context.Context, shareURL string) (*services.SphFeedResponse, error) {
				if shareURL != "https://weixin.qq.com/sph/A1b2C3d4" {
					t.Fatalf("shareURL = %q", shareURL)
				}
				return nil, errors.New("backend parse failed")
			},
		}
	}

	fetchSharedFeedProfileViaPage = func(ctx context.Context, hub *ws.Hub, userID uint, shareURL string) (interface{}, error) {
		if userID != 7 {
			t.Fatalf("userID = %d, want 7", userID)
		}
		if shareURL != "https://weixin.qq.com/sph/A1b2C3d4" {
			t.Fatalf("shareURL = %q", shareURL)
		}
		return map[string]interface{}{
			"errCode": 0,
			"errMsg":  "ok",
			"data": map[string]interface{}{
				"object": map[string]interface{}{
					"id": "page-object-1",
					"objectDesc": map[string]interface{}{
						"description": "页面回退标题",
					},
				},
			},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels/shared_feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserID, uint(7)))
	rec := httptest.NewRecorder()

	GetSharedFeedProfileHandler(nil)(rec, req)

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
					} `json:"objectDesc"`
				} `json:"object"`
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
	if body.Data.Data.Object.ID != "page-object-1" {
		t.Fatalf("object id = %q", body.Data.Data.Object.ID)
	}
	if body.Data.Data.Object.ObjectDesc.Description != "页面回退标题" {
		t.Fatalf("description = %q", body.Data.Data.Object.ObjectDesc.Description)
	}
}

func TestGetSharedFeedProfileReturnsServiceUnavailableWhenNoPageFallbackAvailable(t *testing.T) {
	initSphTestDB(t)

	originalFactory := newHubSharedFeedService
	originalPageFetcher := fetchSharedFeedProfileViaPage
	t.Cleanup(func() {
		newHubSharedFeedService = originalFactory
		fetchSharedFeedProfileViaPage = originalPageFetcher
	})

	newHubSharedFeedService = func() hubSharedFeedService {
		return stubHubSharedFeedService{enabled: false}
	}

	fetchSharedFeedProfileViaPage = func(ctx context.Context, hub *ws.Hub, userID uint, shareURL string) (interface{}, error) {
		return nil, errors.New("no ready client")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels/shared_feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()

	GetSharedFeedProfileHandler(nil)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["message"] != "No ready WeChat page is available for shared feed profile." {
		t.Fatalf("message = %#v", body["message"])
	}
}

func TestGetSharedFeedProfileFallsBackViaLiveHubClient(t *testing.T) {
	initSphTestDB(t)

	originalFactory := newHubSharedFeedService
	t.Cleanup(func() {
		newHubSharedFeedService = originalFactory
	})
	newHubSharedFeedService = func() hubSharedFeedService {
		return stubHubSharedFeedService{enabled: false}
	}

	hub := ws.NewHub()
	go hub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeWs)
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/client?client_id=test-live-client"
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	heartbeatPayload, err := json.Marshal(ws.HeartbeatPayload{
		Hostname:            "test-host",
		Version:             "test-version",
		Status:              "online",
		APIReady:            true,
		WSClients:           1,
		ReadyClients:        1,
		ProfileReadyClients: 1,
		SupportsProfile:     true,
		Methods: map[string]bool{
			"finderGetCommentDetail": true,
		},
	})
	if err != nil {
		t.Fatalf("marshal heartbeat: %v", err)
	}
	heartbeatMessage, err := json.Marshal(ws.CloudMessage{
		ID:        "heartbeat-test-live-client",
		Type:      ws.MsgTypeHeartbeat,
		ClientID:  "test-live-client",
		Payload:   heartbeatPayload,
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("marshal heartbeat message: %v", err)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, heartbeatMessage); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, message, err := conn.Read(context.Background())
			if err != nil {
				return
			}

			var cloudMsg ws.CloudMessage
			if err := json.Unmarshal(message, &cloudMsg); err != nil {
				continue
			}
			if cloudMsg.Type != ws.MsgTypeCommand {
				continue
			}

			var cmd ws.CommandPayload
			if err := json.Unmarshal(cloudMsg.Payload, &cmd); err != nil {
				continue
			}
			if cmd.Action != "api_call" {
				continue
			}

			var req struct {
				Key  string                 `json:"key"`
				Body map[string]interface{} `json:"body"`
			}
			if err := json.Unmarshal(cmd.Data, &req); err != nil {
				continue
			}
			if req.Key != "key:channels:shared_feed_profile" {
				continue
			}

			respPayload, _ := json.Marshal(ws.ResponsePayload{
				RequestID: cloudMsg.ID,
				Success:   true,
				Data: json.RawMessage(`{
					"errCode":0,
					"errMsg":"ok",
					"data":{
						"object":{"id":"live-object-1","objectDesc":{"description":"在线页面回退成功","media":[{"url":"https://cdn.example.com/live.mp4"}]}},
						"authorInfo":{"nickname":"在线作者"},
						"feedInfo":{"originVideoUrl":"https://cdn.example.com/live.mp4","description":"在线页面回退成功"}
					}
				}`),
			})
			msgBytes, _ := json.Marshal(ws.CloudMessage{
				ID:        "resp-" + cloudMsg.ID,
				Type:      ws.MsgTypeResponse,
				ClientID:  "test-live-client",
				Payload:   respPayload,
				Timestamp: time.Now().Unix(),
			})
			_ = conn.Write(context.Background(), websocket.MessageText, msgBytes)
			return
		}
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hub.GetClient("test-live-client") != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if hub.GetClient("test-live-client") == nil {
		t.Fatalf("hub client was not registered")
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var node struct {
			Status          string
			SupportsProfile bool
		}
		err := database.DB.Table("nodes").
			Select("status", "supports_profile").
			Where("id = ?", "test-live-client").
			Take(&node).Error
		if err == nil && node.Status == "online" && node.SupportsProfile {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	var registeredNode struct {
		Status          string
		SupportsProfile bool
	}
	if err := database.DB.Table("nodes").
		Select("status", "supports_profile").
		Where("id = ?", "test-live-client").
		Take(&registeredNode).Error; err != nil {
		t.Fatalf("load registered node: %v", err)
	}
	if registeredNode.Status != "online" || !registeredNode.SupportsProfile {
		t.Fatalf("node not profile-ready: status=%q supportsProfile=%v", registeredNode.Status, registeredNode.SupportsProfile)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/channels/shared_feed/profile?url=https://weixin.qq.com/sph/A1b2C3d4", nil)
	rec := httptest.NewRecorder()

	GetSharedFeedProfileHandler(hub)(rec, req)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("live hub client did not receive command in time")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
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
					} `json:"objectDesc"`
				} `json:"object"`
				AuthorInfo struct {
					Nickname string `json:"nickname"`
				} `json:"authorInfo"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || body.Data.ErrCode != 0 {
		t.Fatalf("unexpected codes: code=%d errCode=%d", body.Code, body.Data.ErrCode)
	}
	if body.Data.Data.Object.ID != "live-object-1" {
		t.Fatalf("object id = %q", body.Data.Data.Object.ID)
	}
	if body.Data.Data.Object.ObjectDesc.Description != "在线页面回退成功" {
		t.Fatalf("description = %q", body.Data.Data.Object.ObjectDesc.Description)
	}
	if body.Data.Data.AuthorInfo.Nickname != "在线作者" {
		t.Fatalf("nickname = %q", body.Data.Data.AuthorInfo.Nickname)
	}
}

func TestGetSphSettingsReturnsMaskedState(t *testing.T) {
	initSphTestDB(t)

	if err := database.SetSetting("sph.enabled", "true"); err != nil {
		t.Fatalf("SetSetting enabled: %v", err)
	}
	if err := database.SetSetting("sph.cookie", "abcd1234efgh5678"); err != nil {
		t.Fatalf("SetSetting cookie: %v", err)
	}
	if err := database.SetSetting("sph.hostname", "https://worker.example.com"); err != nil {
		t.Fatalf("SetSetting hostname: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings/sph", nil)
	rec := httptest.NewRecorder()

	GetSphSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"hasCookie":true`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"hostname":"https://worker.example.com"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "abcd1234efgh5678") {
		t.Fatalf("cookie should be masked, body = %q", rec.Body.String())
	}
}

func TestUpdateSphSettingsPersistsValues(t *testing.T) {
	initSphTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings/sph", strings.NewReader(`{"enabled":true,"cookie":"cookie-value","hostname":"https://worker.example.com"}`))
	rec := httptest.NewRecorder()

	UpdateSphSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	cookie, err := database.GetSetting("sph.cookie")
	if err != nil {
		t.Fatalf("GetSetting cookie: %v", err)
	}
	if cookie != "cookie-value" {
		t.Fatalf("cookie = %q", cookie)
	}

	hostname, err := database.GetSetting("sph.hostname")
	if err != nil {
		t.Fatalf("GetSetting hostname: %v", err)
	}
	if hostname != "https://worker.example.com" {
		t.Fatalf("hostname = %q", hostname)
	}
}

func TestLoadHubSphConfigPrefersDatabase(t *testing.T) {
	initSphTestDB(t)

	t.Setenv("HUB_SPH_COOKIE", "env-cookie")
	t.Setenv("HUB_SPH_HOSTNAME", "https://env.example.com")

	if err := database.SetSetting("sph.enabled", "true"); err != nil {
		t.Fatalf("SetSetting enabled: %v", err)
	}
	if err := database.SetSetting("sph.cookie", "db-cookie"); err != nil {
		t.Fatalf("SetSetting cookie: %v", err)
	}
	if err := database.SetSetting("sph.hostname", "https://db.example.com"); err != nil {
		t.Fatalf("SetSetting hostname: %v", err)
	}

	cfg := loadHubSphConfig()
	if cfg.SphCookie != "db-cookie" {
		t.Fatalf("cookie = %q", cfg.SphCookie)
	}
	if cfg.SphHostname != "https://db.example.com" {
		t.Fatalf("hostname = %q", cfg.SphHostname)
	}
}

func TestLoadHubSphConfigFallsBackToEnvWhenNoSettings(t *testing.T) {
	initSphTestDB(t)

	t.Setenv("HUB_SPH_COOKIE", "env-cookie")
	t.Setenv("HUB_SPH_HOSTNAME", "https://env.example.com")

	_ = database.SetSetting("sph.enabled", "")
	_ = database.SetSetting("sph.cookie", "")
	_ = database.SetSetting("sph.hostname", "")

	cfg := loadHubSphConfig()
	if cfg.SphCookie != "env-cookie" {
		t.Fatalf("cookie = %q", cfg.SphCookie)
	}
	if cfg.SphHostname != "https://env.example.com" {
		t.Fatalf("hostname = %q", cfg.SphHostname)
	}
}

func TestUpdateSphSettingsDisableClearsValues(t *testing.T) {
	initSphTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings/sph", strings.NewReader(`{"enabled":false,"cookie":"cookie-value","hostname":"https://worker.example.com"}`))
	rec := httptest.NewRecorder()

	UpdateSphSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	enabled, _ := database.GetSetting("sph.enabled")
	cookie, _ := database.GetSetting("sph.cookie")
	hostname, _ := database.GetSetting("sph.hostname")
	if enabled != "false" || cookie != "" || hostname != "" {
		t.Fatalf("enabled=%q cookie=%q hostname=%q", enabled, cookie, hostname)
	}
}

func TestGetSphSettingsResponseShape(t *testing.T) {
	initSphTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings/sph", nil)
	rec := httptest.NewRecorder()

	GetSphSettings(rec, req)

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != float64(0) {
		t.Fatalf("code = %#v", body["code"])
	}
}
