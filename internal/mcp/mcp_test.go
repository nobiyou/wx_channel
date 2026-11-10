package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
	"wx_channel/internal/websocket"
)

func TestStreamableHTTPRequiresBearerAndListsTools(t *testing.T) {
	downloadsDir := t.TempDir()
	server, err := NewServer(Dependencies{
		Config:       &config.Config{Port: 2025},
		DownloadsDir: downloadsDir,
		Version:      "test",
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	httpServer := httptest.NewServer(server.handler)
	defer httpServer.Close()

	unauthorized, err := http.Get(httpServer.URL)
	if err != nil {
		t.Fatalf("unauthorized GET failed: %v", err)
	}
	defer unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.StatusCode, http.StatusUnauthorized)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "wx-channel-test-client", Version: "test"}, nil)
	clientHTTP := &http.Client{Transport: bearerRoundTripper{base: http.DefaultTransport, token: server.token}}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   httpServer.URL,
		HTTPClient: clientHTTP,
	}, nil)
	if err != nil {
		t.Fatalf("client Connect() error = %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	got := map[string]bool{}
	for _, tool := range result.Tools {
		got[tool.Name] = true
		if tool.Annotations == nil {
			t.Errorf("tool %q has no annotations", tool.Name)
		} else if tool.Name == "download_video_for_analysis" || tool.Name == "start_video_analysis" || tool.Name == "save_video_analysis_review" {
			if tool.Annotations.ReadOnlyHint {
				t.Errorf("tool %q is incorrectly annotated read-only", tool.Name)
			}
			if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
				t.Errorf("tool %q is not explicitly annotated additive", tool.Name)
			}
		} else if !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q is not annotated read-only", tool.Name)
		}
	}
	if len(result.Tools) != 17 {
		t.Fatalf("unexpected tool count: %d", len(result.Tools))
	}
	for _, name := range []string{
		"get_runtime_status", "list_local_videos", "get_local_video", "get_local_media_metadata",
		"get_local_video_frames",
		"download_video_for_analysis",
		"start_video_analysis", "get_video_analysis_status", "get_video_analysis_shots", "get_video_analysis_report",
		"save_video_analysis_review",
		"search_channel_accounts", "search_channel_videos", "list_channel_videos", "get_channel_video_profile", "list_channel_comments", "resolve_channel_share_link",
	} {
		if !got[name] {
			t.Errorf("tool %q missing from ListTools", name)
		}
	}

	status, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_runtime_status"})
	if err != nil {
		t.Fatalf("CallTool(get_runtime_status) error = %v", err)
	}
	if status.IsError {
		t.Fatal("get_runtime_status returned an MCP tool error")
	}
	if status.StructuredContent == nil {
		t.Fatal("get_runtime_status has no structured content")
	}
	encoded, err := json.Marshal(status.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured status: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode structured status: %v", err)
	}
	if envelope["source"] != "runtime_diagnostics" || envelope["freshness"] != "live" {
		t.Fatalf("runtime envelope = %#v", envelope)
	}
}

func TestLocalVideoPaginationAndSafeMediaMetadata(t *testing.T) {
	cleanup := setupMCPDatabase(t)
	defer cleanup()
	downloadsDir := t.TempDir()
	mediaPath := filepath.Join(downloadsDir, "video.mp4")
	if err := os.WriteFile(mediaPath, []byte("video"), 0600); err != nil {
		t.Fatalf("write media fixture: %v", err)
	}

	browse := &database.BrowseRecord{
		ID: "browse-1", Title: "浏览视频", Author: "作者甲", BrowseTime: time.Now().Add(-time.Hour), Duration: 12, Size: 123,
		Resolution: "720p", LikeCount: 3,
	}
	if err := database.NewBrowseHistoryRepository().Create(browse); err != nil {
		t.Fatalf("create browse record: %v", err)
	}
	download := &database.DownloadRecord{
		ID: "download-1", VideoID: "video-1", Title: "下载视频", Author: "作者乙", FilePath: mediaPath,
		FileSize: 5, Duration: 8, Resolution: "1080p", Status: database.DownloadStatusCompleted, DownloadTime: time.Now(),
	}
	if err := database.NewDownloadRecordRepository().Create(download); err != nil {
		t.Fatalf("create download record: %v", err)
	}

	server, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: downloadsDir, Version: "test"})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	_, first, err := server.listLocalVideos(context.Background(), nil, listLocalVideosInput{PageSize: 1})
	if err != nil {
		t.Fatalf("first local page error = %v", err)
	}
	firstData := first["data"].(map[string]any)
	if firstData["total"] != int64(2) {
		t.Fatalf("first total = %#v, want 2", firstData["total"])
	}
	if firstData["has_more"] != true || firstData["next_cursor"] == "" {
		t.Fatalf("first page cursor = %#v", firstData)
	}
	items := firstData["items"].([]map[string]any)
	if len(items) != 1 || items[0]["source"] != "download" {
		t.Fatalf("first page items = %#v", items)
	}

	_, second, err := server.listLocalVideos(context.Background(), nil, listLocalVideosInput{PageSize: 1, Cursor: firstData["next_cursor"].(string)})
	if err != nil {
		t.Fatalf("second local page error = %v", err)
	}
	secondData := second["data"].(map[string]any)
	if secondData["has_more"] != false || len(secondData["items"].([]map[string]any)) != 1 {
		t.Fatalf("second page = %#v", secondData)
	}

	_, metadata, err := server.getLocalMediaMetadata(context.Background(), nil, localMediaMetadataInput{RecordID: "download-1", Source: "download"})
	if err != nil {
		t.Fatalf("local media metadata error = %v", err)
	}
	metadataData := metadata["data"].(map[string]any)
	if metadataData["relative_path"] != "video.mp4" || metadataData["exists"] != true {
		t.Fatalf("metadata = %#v", metadataData)
	}

	_, _, err = server.getLocalMediaMetadata(context.Background(), nil, localMediaMetadataInput{RelativePath: "../outside.mp4"})
	if err == nil || err.Error() != "path_forbidden: the media path is outside the configured downloads directory" {
		t.Fatalf("path escape error = %v", err)
	}
}

func TestSanitizeLivePayloadRemovesSecretsAndSignedURLs(t *testing.T) {
	payload, redacted, err := sanitizeJSONPayload([]byte(`{"data":{"title":"保留","decryptKey":"secret","token":"secret","videoUrl":"https://finder.video.qq.com/251/20302/stodownload?token=secret","coverUrl":"https://cdn.example.com/cover.jpg"}}`))
	if err != nil {
		t.Fatalf("sanitizeJSONPayload() error = %v", err)
	}
	if redacted < 3 {
		t.Fatalf("redacted = %d, want at least 3", redacted)
	}
	encoded, _ := json.Marshal(payload)
	if string(encoded) == "" || string(encoded) == "{}" {
		t.Fatal("sanitized payload unexpectedly empty")
	}
	var sanitized map[string]map[string]string
	if err := json.Unmarshal(encoded, &sanitized); err != nil {
		t.Fatalf("decode sanitized payload: %v", err)
	}
	if sanitized["data"]["title"] != "保留" ||
		sanitized["data"]["videoUrl"] != "[redacted_signed_url]" ||
		sanitized["data"]["coverUrl"] != "https://cdn.example.com/cover.jpg" {
		t.Fatalf("sanitized payload = %s", encoded)
	}
}

func TestLiveToolReportsNoReadyClient(t *testing.T) {
	server, err := NewServer(Dependencies{
		Config:       &config.Config{Port: 2025},
		DownloadsDir: t.TempDir(),
		Hub:          newTestHub(),
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	_, _, err = server.searchChannelVideos(context.Background(), nil, channelSearchInput{Keyword: "测试"})
	if err == nil || !strings.Contains(err.Error(), "no_ready_client") {
		t.Fatalf("no ready client error = %v", err)
	}
}

func TestRuntimeStatusSanitizesClientPageURL(t *testing.T) {
	hub := &fakeLiveHub{status: websocket.ClientStatus{
		Fresh: true,
		Href:  "https://channels.weixin.qq.com/web/pages/home?pass_ticket=secret&exportkey=secret#top",
	}}
	server, err := NewServer(Dependencies{
		Config:       &config.Config{Port: 2025},
		DownloadsDir: t.TempDir(),
		Hub:          hub,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	_, result, err := server.getRuntimeStatus(context.Background(), nil, emptyInput{})
	if err != nil {
		t.Fatalf("getRuntimeStatus() error = %v", err)
	}
	warnings := result["warnings"].([]string)
	foundWarning := false
	for _, warning := range warnings {
		if warning == "client_page_urls_sanitized" {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("runtime warnings = %#v", warnings)
	}
	data := result["data"].(map[string]any)
	clients := data["clients"].([]websocket.ClientStatus)
	if len(clients) != 1 || clients[0].Href != "https://channels.weixin.qq.com/web/pages/home" {
		t.Fatalf("sanitized clients = %#v", clients)
	}
	if hub.status.Href == clients[0].Href {
		t.Fatal("runtime status mutated the hub status")
	}
}

func TestLiveToolUsesReadyHubAndSanitizesResponse(t *testing.T) {
	hub := &fakeLiveHub{
		status: websocket.ClientStatus{
			Fresh:          true,
			APIReady:       true,
			SupportsSearch: true,
		},
		response: jsoniter.RawMessage(`{"data":{"objectList":[{"id":"video-1","objectDesc":{"description":"实时视频","media":[{"url":"https://finder.video.qq.com/251/20302/stodownload?token=secret","thumbUrl":"https://cdn.example.com/cover.jpg"}]},"token":"secret"}],"lastBuff":""}}`),
	}
	server, err := NewServer(Dependencies{
		Config:       &config.Config{Port: 2025},
		DownloadsDir: t.TempDir(),
		Hub:          hub,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	_, result, err := server.searchChannelVideos(context.Background(), nil, channelSearchInput{
		Keyword:    "科技",
		NextMarker: "marker-2",
	})
	if err != nil {
		t.Fatalf("searchChannelVideos() error = %v", err)
	}
	if result["source"] != "live_wechat_page" || result["freshness"] != "live" || result["partial"] != false {
		t.Fatalf("live envelope = %#v", result)
	}
	warnings := result["warnings"].([]string)
	if len(warnings) != 0 {
		t.Fatalf("live warnings = %#v", warnings)
	}
	data := result["data"].(map[string]any)
	liveData := data["items"].([]map[string]any)[0]
	if liveData["title"] != "实时视频" || liveData["token"] != nil || liveData["videoUrl"] != nil || liveData["cover_url"] != "https://cdn.example.com/cover.jpg" || data["result"] != nil {
		t.Fatalf("projected live data = %#v", data)
	}

	if len(hub.calls) != 1 {
		t.Fatalf("hub calls = %d, want 1", len(hub.calls))
	}
	call := hub.calls[0]
	if call.key != "key:channels:contact_list" {
		t.Fatalf("hub key = %q", call.key)
	}
	body, ok := call.body.(websocket.SearchContactBody)
	if !ok {
		t.Fatalf("hub body type = %T", call.body)
	}
	if body.Keyword != "科技" || body.Type != 3 || body.NextMarker != "marker-2" || body.RequestId == "" {
		t.Fatalf("hub body = %#v", body)
	}
}

type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (t bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

func setupMCPDatabase(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	if err := database.Initialize(&database.Config{DBPath: filepath.Join(dir, "records.db")}); err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	return func() { _ = database.Close() }
}

func newTestHub() *websocket.Hub {
	return websocket.NewHub()
}

type fakeLiveHub struct {
	status   websocket.ClientStatus
	response jsoniter.RawMessage
	calls    []fakeLiveCall
}

type fakeLiveCall struct {
	key     string
	body    any
	timeout time.Duration
}

func (h *fakeLiveHub) ClientStatuses() []websocket.ClientStatus {
	return []websocket.ClientStatus{h.status}
}

func (h *fakeLiveHub) GetClientForKey(key string) (*websocket.Client, error) {
	return &websocket.Client{}, nil
}

func (h *fakeLiveHub) CallAPIContext(_ context.Context, key string, body interface{}, timeout time.Duration) (jsoniter.RawMessage, error) {
	h.calls = append(h.calls, fakeLiveCall{key: key, body: body, timeout: timeout})
	if h.response == nil {
		return nil, fmt.Errorf("fake live response is empty")
	}
	return h.response, nil
}
