package services

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
)

func TestCalculateDownloadFilePath_UsesTemplateWhenConfigured(t *testing.T) {
	config.Reload()
	cfg := config.Get()
	cfg.DownloadsDir = "downloads"
	cfg.DownloadFilenameTemplate = "{date}_{author}_{title}_{video_id}"

	item := &database.QueueItem{
		VideoID:   "vid123",
		Title:     "欢迎来到我的直播间",
		Author:    "测试作者",
		Duration:  90 * 1000,
		AddedTime: time.Date(2026, 5, 25, 12, 0, 0, 0, time.Local),
	}

	path := calculateDownloadFilePath(item)

	if !strings.Contains(path, filepath.Join("测试作者", "2026-05-25_测试作者_欢迎来到我的直播间_vid123.mp4")) {
		t.Fatalf("path = %s", path)
	}
}

func TestQueueService_Resume_AllowsFailedItems(t *testing.T) {
	if err := database.Initialize(&database.Config{DBPath: filepath.Join(t.TempDir(), "test.db")}); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	svc := NewQueueService()
	items, err := svc.AddToQueue([]VideoInfo{{VideoID: "retry-vid", Title: "t", VideoURL: "http://x"}})
	if err != nil || len(items) != 1 {
		t.Fatalf("add: %v, items=%d", err, len(items))
	}
	id := items[0].ID
	if err := svc.UpdateStatus(id, database.QueueStatusFailed); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	if err := svc.Resume(id); err != nil {
		t.Fatalf("Resume failed item: %v", err)
	}

	got, _ := svc.GetByID(id)
	if got.Status != database.QueueStatusPending {
		t.Fatalf("status = %s, want pending", got.Status)
	}
}

func TestCalculateDownloadFilePath_FlatLayoutWhenAuthorSubdirDisabled(t *testing.T) {
	config.Reload()
	cfg := config.Get()
	cfg.DownloadsDir = "downloads"
	cfg.DownloadFilenameTemplate = "{author}_{title}"
	cfg.DownloadAuthorSubdir = false

	item := &database.QueueItem{VideoID: "vid1", Title: "标题", Author: "作者"}

	path := calculateDownloadFilePath(item)

	if !strings.HasSuffix(path, filepath.Join("downloads", "作者_标题.mp4")) {
		t.Fatalf("path = %s", path)
	}
}

func TestQueueService_AddToQueue_SkipsVideosAlreadyActive(t *testing.T) {
	if err := database.Initialize(&database.Config{DBPath: filepath.Join(t.TempDir(), "test.db")}); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	svc := NewQueueService()
	videos := []VideoInfo{{VideoID: "dup-1", Title: "a", VideoURL: "http://x"}, {VideoID: "dup-2", Title: "b", VideoURL: "http://y"}}

	first, err := svc.AddToQueue(videos)
	if err != nil || len(first) != 2 {
		t.Fatalf("first add: %v, added=%d", err, len(first))
	}
	again, err := svc.AddToQueue(videos)
	if err != nil || len(again) != 0 {
		t.Fatalf("second add should skip both: %v, added=%d", err, len(again))
	}
	if err := svc.UpdateStatus(first[0].ID, database.QueueStatusCompleted); err != nil {
		t.Fatalf("complete: %v", err)
	}
	third, err := svc.AddToQueue(videos)
	if err != nil || len(third) != 1 || third[0].VideoID != "dup-1" {
		t.Fatalf("completed video should be re-addable: %v, added=%v", err, third)
	}
}
