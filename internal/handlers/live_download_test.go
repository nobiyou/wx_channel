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
