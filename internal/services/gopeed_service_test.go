package services

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestDownloadSyncFallsBackWhenRangeProbeIsRejected(t *testing.T) {
	var rangeRequests atomic.Int32
	content := []byte("video-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			rangeRequests.Add(1)
			http.Error(w, "range probe rejected", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(content)
	}))
	defer server.Close()

	service := NewGopeedService(t.TempDir())
	t.Cleanup(func() {
		_ = service.Downloader.Close()
	})

	target := filepath.Join(t.TempDir(), "video.mp4")
	actualPath, err := service.DownloadSync(
		context.Background(),
		server.URL+"/video.mp4",
		target,
		1,
		map[string]string{"Referer": server.URL + "/"},
		nil,
	)
	if err != nil {
		t.Fatalf("DownloadSync() error = %v", err)
	}
	if rangeRequests.Load() == 0 {
		t.Fatal("expected Gopeed to make a range probe")
	}

	data, err := os.ReadFile(actualPath)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("downloaded content = %q, want %q", data, content)
	}
}
