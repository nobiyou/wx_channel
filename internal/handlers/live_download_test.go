package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFFmpegHeaderBlockKeepsLiveRequestContext(t *testing.T) {
	block := buildFFmpegHeaderBlock(map[string]string{
		"Origin":  "https://channels.weixin.qq.com",
		"Cookie":  "session=abc",
		"Ignored": "value",
	}, "https://channels.weixin.qq.com/web/pages/live")

	for _, want := range []string{
		"Origin: https://channels.weixin.qq.com\r\n",
		"Referer: https://channels.weixin.qq.com/web/pages/live\r\n",
		"Cookie: session=abc\r\n",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("FFmpeg header block missing %q: %q", want, block)
		}
	}
	if strings.Contains(block, "Ignored") {
		t.Fatalf("unexpected header leaked into FFmpeg command: %q", block)
	}
}

func TestResolveLiveFFmpegUsesConfiguredExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ffmpeg-test.exe")
	if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	t.Setenv("WX_CHANNEL_FFMPEG_PATH", path)

	got, err := resolveLiveFFmpeg()
	if err != nil {
		t.Fatalf("resolveLiveFFmpeg() error = %v", err)
	}
	if got != path {
		t.Fatalf("resolveLiveFFmpeg() = %q, want %q", got, path)
	}
}

func TestAutoSaveLiveRecordingRequiresNonEmptyTempFile(t *testing.T) {
	h := &UploadHandler{}
	root := t.TempDir()
	tmpPath := filepath.Join(root, "live.tmp")
	videoPath := filepath.Join(root, "saved", "live.flv")

	saved, ok, err := h.autoSaveLiveRecording(LiveDownloadRequest{Title: "断流测试"}, "live-id", videoPath, tmpPath, filepath.Join(root, "saved"))
	if err != nil || ok || saved != "" {
		t.Fatalf("missing temp file should not save: path=%q saved=%t err=%v", saved, ok, err)
	}

	if err := os.WriteFile(tmpPath, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	saved, ok, err = h.autoSaveLiveRecording(LiveDownloadRequest{Title: "空文件测试"}, "empty-id", videoPath, tmpPath, filepath.Join(root, "saved"))
	if err != nil || ok || saved != "" {
		t.Fatalf("empty temp file should not save: path=%q saved=%t err=%v", saved, ok, err)
	}

	if err := os.WriteFile(tmpPath, []byte("recorded bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "saved"), 0700); err != nil {
		t.Fatal(err)
	}
	saved, ok, err = h.autoSaveLiveRecording(LiveDownloadRequest{Title: "有效片段测试", Author: "测试作者"}, "saved-id", videoPath, tmpPath, filepath.Join(root, "saved"))
	if err != nil || !ok || saved == "" {
		t.Fatalf("non-empty temp file should save: path=%q saved=%t err=%v", saved, ok, err)
	}
	if _, err := os.Stat(saved); err != nil {
		t.Fatalf("saved recording missing: %v", err)
	}
}
