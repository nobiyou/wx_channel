package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"wx_channel/hub_server/database"
)

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
