package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 手动集成测试：需要 CDP 桥 62000 与橱窗小程序在场。
// WX_LIVE_CDP_TEST=1 go test -tags nosqlite ./internal/handlers/ -run TestCDPProductHeadImageLive -v
func TestCDPProductImagesLive(t *testing.T) {
	if os.Getenv("WX_LIVE_CDP_TEST") != "1" {
		t.Skip("set WX_LIVE_CDP_TEST=1 才跑")
	}
	urls, specs, title, err := cdpProductImages(context.Background(), "10000005998657", "wx2cb5453a4f995e38")
	if err != nil {
		t.Fatalf("CDP 商品详情失败: %v", err)
	}
	if len(urls) == 0 {
		t.Fatalf("没有主图，title=%q", title)
	}
	t.Logf("title=%s 主图 %d 张、规格图 %d 张（第一张 %.70s）", title, len(urls), len(specs), urls[0])
}

// 手动集成测试 2：完整后端链路（透传拿商品卡 → CDP 商品详情 → 下载图片落盘）。
// 需要 wx_channel(2025) 有 ready 页面 + 橱窗小程序开着。
// WX_LIVE_CDP_TEST=1 go test -tags nosqlite ./internal/handlers/ -run TestDownloadProductImagesLive -v
func TestDownloadProductImagesLive(t *testing.T) {
	if os.Getenv("WX_LIVE_CDP_TEST") != "1" {
		t.Skip("set WX_LIVE_CDP_TEST=1 才跑")
	}
	h := &BatchHandler{}
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "fake.mp4")
	if err := os.WriteFile(videoPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := &BatchTask{ID: "15032177947323275361", Title: "live-test", DownloadProductImages: true}
	h.downloadProductImages(context.Background(), task, videoPath)
	if task.ProductImagePath == "" {
		t.Fatal("没有落盘商品图（看日志里的 🖼️ 原因）")
	}
	first := strings.Split(task.ProductImagePath, ",")[0]
	st, err := os.Stat(first)
	if err != nil || st.Size() == 0 {
		t.Fatalf("落盘文件异常: %v", err)
	}
	t.Logf("落盘: 首张 %s (%d bytes)", first, st.Size())
	if !strings.Contains(task.ProductImagePath, ",") {
		t.Fatalf("只落了 1 张，应该全部主图：%s", task.ProductImagePath)
	}
	imgDir := filepath.Dir(first)
	files, _ := filepath.Glob(filepath.Join(imgDir, "head_*"))
	t.Logf("目录下共 %d 张主图: %v", len(files), files)
}
