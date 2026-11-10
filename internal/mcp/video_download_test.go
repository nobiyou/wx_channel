package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
)

type fakeVideoDownloadEngine struct {
	url         string
	target      string
	connections int
	headers     map[string]string
	err         error
}

func (f *fakeVideoDownloadEngine) DownloadSync(_ context.Context, sourceURL, target string, connections int, headers map[string]string, _ func(float64, int64, int64)) (string, error) {
	f.url = sourceURL
	f.target = target
	f.connections = connections
	f.headers = headers
	if f.err != nil {
		return target, f.err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return target, err
	}
	if err := os.WriteFile(target, []byte("video"), 0600); err != nil {
		return target, err
	}
	return target, nil
}

func TestLocalVideoDownloaderUsesStoredBrowseSourceAndRecordsCompletion(t *testing.T) {
	cleanup := setupMCPDatabase(t)
	defer cleanup()
	downloadsDir := t.TempDir()
	engine := &fakeVideoDownloadEngine{}
	service := NewLocalVideoDownloader(VideoDownloaderDependencies{
		Config:       &config.Config{DownloadConnections: 8},
		DownloadsDir: downloadsDir,
		Engine:       engine,
	})

	browse := &database.BrowseRecord{
		ID:         "browse-download-1",
		Title:      "原始视频标题",
		Author:     "测试作者",
		VideoURL:   "https://finder.video.qq.com/251/20302/stodownload?token=secret&X-snsvideoflag=original",
		PageURL:    "https://channels.weixin.qq.com/web/pages/feed?pass_ticket=secret",
		BrowseTime: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
		Size:       1234,
	}
	if err := database.NewBrowseHistoryRepository().Create(browse); err != nil {
		t.Fatalf("create browse record: %v", err)
	}

	result, err := service.Download(context.Background(), VideoDownloadRequest{RecordID: browse.ID})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Status != database.DownloadStatusCompleted || result.Skipped || result.RelativePath == "" || result.SizeBytes != int64(len("video")) {
		t.Fatalf("download result = %#v", result)
	}
	if strings.Contains(engine.url, "X-snsvideoflag") {
		t.Fatalf("original URL marker was not normalized: %q", engine.url)
	}
	if engine.connections != 1 {
		t.Fatalf("connections = %d, want original single connection", engine.connections)
	}
	if engine.headers["Origin"] != "https://channels.weixin.qq.com" || engine.headers["Referer"] != browse.PageURL {
		t.Fatalf("download headers = %#v", engine.headers)
	}
	if strings.Contains(result.RelativePath, "secret") || filepath.IsAbs(result.RelativePath) {
		t.Fatalf("unsafe relative path = %q", result.RelativePath)
	}

	record, err := database.NewDownloadRecordRepository().GetByVideoID(browse.ID)
	if err != nil {
		t.Fatalf("read download record: %v", err)
	}
	if record == nil || record.Status != database.DownloadStatusCompleted || record.FilePath == "" {
		t.Fatalf("stored download record = %#v", record)
	}
	if _, err := os.Stat(record.FilePath); err != nil {
		t.Fatalf("stored file is unavailable: %v", err)
	}
}

func TestLocalVideoDownloaderSkipsExistingCompletedFile(t *testing.T) {
	cleanup := setupMCPDatabase(t)
	defer cleanup()
	downloadsDir := t.TempDir()
	path := filepath.Join(downloadsDir, "作者", "已有视频.mp4")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	browse := &database.BrowseRecord{
		ID: "browse-existing", Title: "已有视频", Author: "作者",
		VideoURL:   "https://finder.video.qq.com/251/20302/stodownload?token=secret",
		BrowseTime: time.Now(),
	}
	if err := database.NewBrowseHistoryRepository().Create(browse); err != nil {
		t.Fatal(err)
	}
	if err := database.NewDownloadRecordRepository().Create(&database.DownloadRecord{
		ID: browse.ID, VideoID: browse.ID, Title: browse.Title, Author: browse.Author,
		FilePath: path, Status: database.DownloadStatusCompleted, DownloadTime: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	engine := &fakeVideoDownloadEngine{}
	service := NewLocalVideoDownloader(VideoDownloaderDependencies{DownloadsDir: downloadsDir, Engine: engine})

	result, err := service.Download(context.Background(), VideoDownloadRequest{RecordID: browse.ID})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if !result.Skipped || result.Status != database.DownloadStatusCompleted || result.SizeBytes != int64(len("existing")) {
		t.Fatalf("skip result = %#v", result)
	}
	if engine.url != "" {
		t.Fatalf("engine was called for existing file: %#v", engine)
	}
}

func TestLocalVideoDownloaderRejectsMissingStoredVideoSource(t *testing.T) {
	cleanup := setupMCPDatabase(t)
	defer cleanup()
	browse := &database.BrowseRecord{ID: "browse-no-url", Title: "无地址", Author: "作者", BrowseTime: time.Now()}
	if err := database.NewBrowseHistoryRepository().Create(browse); err != nil {
		t.Fatal(err)
	}
	service := NewLocalVideoDownloader(VideoDownloaderDependencies{DownloadsDir: t.TempDir(), Engine: &fakeVideoDownloadEngine{}})
	_, err := service.Download(context.Background(), VideoDownloadRequest{RecordID: browse.ID})
	if err == nil || !strings.HasPrefix(err.Error(), "download_source_unavailable:") {
		t.Fatalf("missing source error = %v", err)
	}
}

func TestDownloadToolIsIndependentAndReportsInProgress(t *testing.T) {
	cleanup := setupMCPDatabase(t)
	defer cleanup()
	browse := &database.BrowseRecord{
		ID: "browse-action", Title: "动作视频", Author: "作者",
		VideoURL:   "https://finder.video.qq.com/251/20302/stodownload?token=secret",
		BrowseTime: time.Now(),
	}
	if err := database.NewBrowseHistoryRepository().Create(browse); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	server, err := NewServer(Dependencies{
		Config:       &config.Config{Port: 2025},
		DownloadsDir: t.TempDir(),
		VideoDownload: func(ctx context.Context, request VideoDownloadRequest) (VideoDownloadResult, error) {
			if request.RecordID != browse.ID {
				t.Errorf("download record id = %q, want %q", request.RecordID, browse.ID)
			}
			close(started)
			select {
			case <-release:
				return VideoDownloadResult{RecordID: request.RecordID, Status: database.DownloadStatusCompleted}, nil
			case <-ctx.Done():
				return VideoDownloadResult{}, ctx.Err()
			}
		},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer server.Shutdown(context.Background())

	_, first, err := server.downloadVideoForAnalysis(context.Background(), nil, downloadVideoForAnalysisInput{RecordID: browse.ID})
	if err != nil {
		t.Fatalf("first action error = %v", err)
	}
	if first["source"] != "local_download" || first["freshness"] != "live" {
		t.Fatalf("first envelope = %#v", first)
	}
	firstData := first["data"].(map[string]any)
	if firstData["status"] != database.DownloadStatusInProgress || firstData["started"] != true {
		t.Fatalf("first data = %#v", firstData)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("background download did not start")
	}

	_, second, err := server.downloadVideoForAnalysis(context.Background(), nil, downloadVideoForAnalysisInput{RecordID: browse.ID})
	if err != nil {
		t.Fatalf("second action error = %v", err)
	}
	secondData := second["data"].(map[string]any)
	if secondData["status"] != database.DownloadStatusInProgress || secondData["started"] != false {
		t.Fatalf("second data = %#v", secondData)
	}
	close(release)
}
