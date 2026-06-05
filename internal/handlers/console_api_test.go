package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
)

func TestIsPathWithinBase(t *testing.T) {
	base := filepath.Clean(filepath.Join("C:", "downloads"))

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{
			name:   "base directory itself",
			target: base,
			want:   true,
		},
		{
			name:   "file inside base directory",
			target: filepath.Join(base, "author", "video.mp4"),
			want:   true,
		},
		{
			name:   "path traversal outside base",
			target: filepath.Clean(filepath.Join(base, "..", "Windows", "system.ini")),
			want:   false,
		},
		{
			name:   "sibling directory",
			target: filepath.Clean(filepath.Join(filepath.Dir(base), "other", "video.mp4")),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPathWithinBase(base, tt.target)
			if got != tt.want {
				t.Fatalf("isPathWithinBase(%q, %q) = %v, want %v", base, tt.target, got, tt.want)
			}
		})
	}
}

func TestHandleQueueFail_InvalidJSONReturnsBadRequest(t *testing.T) {
	handler := &ConsoleAPIHandler{}
	req := httptest.NewRequest(http.MethodPut, "/api/queue/test-id/fail", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	handler.HandleQueueFail(rr, req, "test-id")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp APIResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Fatalf("success = true, want false")
	}
	if resp.Error != "invalid request body" {
		t.Fatalf("error = %q, want %q", resp.Error, "invalid request body")
	}
}

func TestValidateVideoPlayTargetURL(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		wantError bool
	}{
		{
			name:      "valid https url",
			rawURL:    "https://example.com/video.mp4",
			wantError: false,
		},
		{
			name:      "invalid scheme",
			rawURL:    "file:///tmp/video.mp4",
			wantError: true,
		},
		{
			name:      "localhost blocked",
			rawURL:    "http://localhost/video.mp4",
			wantError: true,
		},
		{
			name:      "private ipv4 blocked",
			rawURL:    "http://192.168.1.20/video.mp4",
			wantError: true,
		},
		{
			name:      "loopback ipv6 blocked",
			rawURL:    "http://[::1]/video.mp4",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateVideoPlayTargetURL(tt.rawURL)
			gotError := err != nil
			if gotError != tt.wantError {
				t.Fatalf("validateVideoPlayTargetURL(%q) error = %v, wantError=%v", tt.rawURL, err, tt.wantError)
			}
		})
	}
}

func TestHandleVideoPlay_BlockedLocalAddress(t *testing.T) {
	handler := &ConsoleAPIHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/video/play?url=http://127.0.0.1/video.mp4", nil)
	rr := httptest.NewRecorder()

	handler.HandleVideoPlay(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp APIResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Fatalf("success = true, want false")
	}
	if !strings.Contains(resp.Error, "local/private") {
		t.Fatalf("error = %q, want contains %q", resp.Error, "local/private")
	}
}

func TestValidatePathInBase(t *testing.T) {
	baseDir := t.TempDir()
	insideFile := filepath.Join(baseDir, "video.mp4")
	if err := os.WriteFile(insideFile, []byte("x"), 0644); err != nil {
		t.Fatalf("write inside file failed: %v", err)
	}

	tests := []struct {
		name      string
		target    string
		allowDir  bool
		wantError bool
	}{
		{
			name:      "file inside base",
			target:    insideFile,
			allowDir:  false,
			wantError: false,
		},
		{
			name:      "directory allowed",
			target:    baseDir,
			allowDir:  true,
			wantError: false,
		},
		{
			name:      "directory disallowed",
			target:    baseDir,
			allowDir:  false,
			wantError: true,
		},
		{
			name:      "path outside base",
			target:    filepath.Join(filepath.Dir(baseDir), "outside.mp4"),
			allowDir:  false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePathInBase(baseDir, tt.target, tt.allowDir)
			gotError := err != nil
			if gotError != tt.wantError {
				t.Fatalf("validatePathInBase(%q, %q, %v) error=%v, wantError=%v", baseDir, tt.target, tt.allowDir, err, tt.wantError)
			}
		})
	}
}

func TestValidatePathInBase_SymlinkEscapeBlocked(t *testing.T) {
	baseDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.mp4")
	if err := os.WriteFile(outsideFile, []byte("x"), 0644); err != nil {
		t.Fatalf("write outside file failed: %v", err)
	}

	linkPath := filepath.Join(baseDir, "escape.mp4")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not available on this environment: %v", err)
	}

	_, err := validatePathInBase(baseDir, linkPath, false)
	if err == nil {
		t.Fatalf("expected symlink escape to be blocked")
	}
}

func TestIsAllowedVideoExtension(t *testing.T) {
	valid := []string{"a.mp4", "a.webm", "a.ogv", "a.avi", "a.mkv", "a.mov"}
	for _, path := range valid {
		if !isAllowedVideoExtension(path) {
			t.Fatalf("expected allowed extension for %q", path)
		}
	}

	invalid := []string{"a.exe", "a.txt", "a", "a.mp4.exe"}
	for _, path := range invalid {
		if isAllowedVideoExtension(path) {
			t.Fatalf("expected disallowed extension for %q", path)
		}
	}
}

func TestParseJSON_BodyTooLarge(t *testing.T) {
	handler := &ConsoleAPIHandler{}
	var payload struct {
		Data string `json:"data"`
	}

	tooLarge := `{"data":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(tooLarge))

	err := handler.parseJSON(req, &payload)
	if err == nil {
		t.Fatalf("expected error for oversized body, got nil")
	}
	if !strings.Contains(err.Error(), "request body too large") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVideoProxyHTTPClient_CheckRedirect(t *testing.T) {
	client := newVideoProxyHTTPClient()
	if client.CheckRedirect == nil {
		t.Fatalf("CheckRedirect should not be nil")
	}

	blockedURL, _ := url.Parse("http://127.0.0.1/video.mp4")
	blockedReq := &http.Request{URL: blockedURL}
	if err := client.CheckRedirect(blockedReq, nil); err == nil {
		t.Fatalf("expected redirect validation error for localhost target")
	}

	allowedURL, _ := url.Parse("https://example.com/video.mp4")
	allowedReq := &http.Request{URL: allowedURL}
	if err := client.CheckRedirect(allowedReq, nil); err != nil {
		t.Fatalf("unexpected redirect validation error: %v", err)
	}
}

func TestHandleSettingsGet_UsesConfigRadarEnabled(t *testing.T) {
	cleanup := databaseTestSetupForHandlers(t)
	defer cleanup()

	repo := database.NewSettingsRepository()
	if err := repo.Set(database.SettingKeyRadarEnabled, "true"); err != nil {
		t.Fatalf("seed radar_enabled failed: %v", err)
	}

	cfg := &config.Config{RadarEnabled: false}
	handler := NewConsoleAPIHandler(cfg, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr := httptest.NewRecorder()
	handler.HandleSettingsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp struct {
		Success bool              `json:"success"`
		Data    database.Settings `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response")
	}
	if resp.Data.RadarEnabled {
		t.Fatalf("radarEnabled = true, want false from config")
	}
}

func TestHandleSettingsGet_UsesConfigDownloadFilenameTemplate(t *testing.T) {
	cleanup := databaseTestSetupForHandlers(t)
	defer cleanup()

	cfg := &config.Config{
		RadarEnabled:             false,
		DownloadFilenameTemplate: "{date}_{author}_{title}_{duration}",
	}
	handler := NewConsoleAPIHandler(cfg, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr := httptest.NewRecorder()
	handler.HandleSettingsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp struct {
		Success bool              `json:"success"`
		Data    database.Settings `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response")
	}
	if resp.Data.DownloadFilenameTemplate != "{date}_{author}_{title}_{duration}" {
		t.Fatalf("downloadFilenameTemplate = %q", resp.Data.DownloadFilenameTemplate)
	}
}

func TestHandleSettingsGet_UsesSharedFeedBackendStatusFromConfig(t *testing.T) {
	cleanup := databaseTestSetupForHandlers(t)
	defer cleanup()

	cfg := &config.Config{
		Cloudflare: config.CloudflareConfig{
			SphHostname: "https://worker.example.com",
		},
	}
	handler := NewConsoleAPIHandler(cfg, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr := httptest.NewRecorder()
	handler.HandleSettingsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp struct {
		Success bool              `json:"success"`
		Data    database.Settings `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response")
	}
	if !resp.Data.SharedFeedBackendEnabled {
		t.Fatalf("sharedFeedBackendEnabled = false, want true")
	}
	if resp.Data.SharedFeedBackendType != "worker" {
		t.Fatalf("sharedFeedBackendType = %q, want worker", resp.Data.SharedFeedBackendType)
	}
}

func TestHandleSettingsUpdate_IgnoresRadarEnabledPayload(t *testing.T) {
	cleanup := databaseTestSetupForHandlers(t)
	defer cleanup()

	cfg := &config.Config{RadarEnabled: false}
	handler := NewConsoleAPIHandler(cfg, nil, nil)

	body := `{"downloadDir":"downloads","downloadFilenameWithVideoId":false,"chunkSize":10485760,"concurrentLimit":3,"autoCleanupEnabled":false,"autoCleanupDays":30,"maxRetries":3,"radarEnabled":true,"theme":"light"}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.HandleSettingsUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	value, err := database.NewSettingsRepository().Get(database.SettingKeyRadarEnabled)
	if err != nil {
		t.Fatalf("get radar_enabled failed: %v", err)
	}
	if value != "" {
		t.Fatalf("radar_enabled persisted as %q, want empty", value)
	}
}

func databaseTestSetupForHandlers(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	_ = database.Close()
	if err := database.Initialize(&database.Config{DBPath: dbPath}); err != nil {
		t.Fatalf("initialize test database failed: %v", err)
	}

	return func() {
		if err := database.Close(); err != nil {
			t.Fatalf("close test database failed: %v", err)
		}
	}
}
