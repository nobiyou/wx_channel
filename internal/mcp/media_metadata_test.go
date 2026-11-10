package mcpserver

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"wx_channel/internal/config"

	"wx_channel/internal/database"
)

func TestLocalMediaAndMetricSourcesThroughMCP(t *testing.T) {
	t.Cleanup(setupMCPDatabase(t))
	dir := t.TempDir()
	// Fractional duration and a portrait rotation must survive the actual SDK
	// text/structured response encoding rather than just a direct helper call.
	fixture := mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 1250), mediaProbeTrack("vide", 0, 1280, 720, 90))
	if err := os.WriteFile(filepath.Join(dir, "actual.mp4"), fixture, 0600); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 6, 1, 2, 3, 0, time.UTC)
	if err := database.NewDownloadRecordRepository().Create(&database.DownloadRecord{
		ID: "wire", VideoID: "video", FilePath: "actual.mp4", DownloadTime: at, Status: database.DownloadStatusCompleted,
		MetricsSnapshot: database.CaptureVideoMetrics(map[string]any{"likeCount": float64(0)}, "wechat_api", at),
	}); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.handler)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "local-metadata-contract", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearerRoundTripper{base: http.DefaultTransport, token: s.token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	for _, name := range []string{"get_local_media_metadata", "get_local_video"} {
		arguments := map[string]any{"record_id": "wire", "source": "download"}
		if name == "get_local_video" {
			arguments = map[string]any{"id": "wire", "source": "download"}
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("%s: %v, %v", name, result, err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(encoded, &out); err != nil {
			t.Fatal(err)
		}
		data := out["data"].(map[string]any)
		if name == "get_local_media_metadata" {
			if data["duration_seconds"] != 1.25 || data["resolution"] != "720x1280" || out["partial"] != false {
				t.Fatalf("wire metadata = %s", encoded)
			}
		} else if data["like_count"] != float64(0) || data["comment_count"] != nil || data["metrics_source"] != "wechat_api" || out["partial"] != true {
			t.Fatalf("wire provenance = %s", encoded)
		}
	}
}

func TestLocalMediaReadsImageDimensionsWithoutRecord(t *testing.T) {
	s := localContractServer(t)
	path := filepath.Join(s.deps.DownloadsDir, "actual.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, result, err := s.getLocalMediaMetadata(context.Background(), nil, localMediaMetadataInput{RelativePath: "actual.png"})
	if err != nil {
		t.Fatal(err)
	}
	data := result["data"].(map[string]any)
	if data["resolution"] != "3x2" || data["duration_seconds"] != nil || data["width"] == nil || data["height"] == nil {
		t.Fatalf("actual image metadata unavailable: %v", data)
	}
	if sources := data["metadata_sources"].(map[string]string); sources["resolution"] != "image_header" {
		t.Fatalf("metadata source = %v", sources)
	}
	localContractDownload(t, "image", path)
	_, err = database.GetDB().Exec("UPDATE download_records SET resolution = '999x999' WHERE id = 'image'")
	if err != nil {
		t.Fatal(err)
	}
	_, result, err = s.getLocalMediaMetadata(context.Background(), nil, localMediaMetadataInput{RecordID: "image", Source: "download"})
	if err != nil {
		t.Fatal(err)
	}
	if result["data"].(map[string]any)["resolution"] != "3x2" {
		t.Fatalf("stored resolution overrode file: %v", result)
	}
}

func TestLocalMediaUnknownsAndStoredFallback(t *testing.T) {
	s := localContractServer(t)
	path := filepath.Join(s.deps.DownloadsDir, "broken.mp4")
	if err := os.WriteFile(path, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	_, result, err := s.getLocalMediaMetadata(context.Background(), nil, localMediaMetadataInput{RelativePath: "broken.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	data := result["data"].(map[string]any)
	for _, key := range []string{"duration_seconds", "width", "height", "resolution"} {
		if value, exists := data[key]; !exists || value != nil {
			t.Errorf("%s = %v, present %v; want null", key, value, exists)
		}
	}
	if result["partial"] != true {
		t.Fatal("unreadable metadata not marked partial")
	}
	localContractDownload(t, "broken", path)
	if _, err := database.GetDB().Exec("UPDATE download_records SET duration = 8, resolution = '720p' WHERE id = 'broken'"); err != nil {
		t.Fatal(err)
	}
	_, result, err = s.getLocalMediaMetadata(context.Background(), nil, localMediaMetadataInput{RecordID: "broken", Source: "download"})
	if err != nil {
		t.Fatal(err)
	}
	data = result["data"].(map[string]any)
	if data["duration_seconds"] != int64(8) || data["resolution"] != "720p" || data["metadata_sources"].(map[string]string)["duration_seconds"] != "local_database" {
		t.Fatalf("fallback is not explicit: %v", data)
	}
}
