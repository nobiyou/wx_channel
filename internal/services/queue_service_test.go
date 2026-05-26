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

