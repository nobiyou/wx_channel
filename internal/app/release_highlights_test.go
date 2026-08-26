package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"

	"wx_channel/internal/config"
	"wx_channel/internal/version"
)

func TestReleaseHighlightsMatchCurrentVersion(t *testing.T) {
	if releaseHighlightsVersion != version.Current {
		t.Fatalf("release highlights version %s does not match current version %s", releaseHighlightsVersion, version.Current)
	}
	if len(releaseHighlights) != 5 {
		t.Fatalf("release highlights count = %d, want 5", len(releaseHighlights))
	}

	joined := strings.Join(releaseHighlights[:], "\n")
	for _, expected := range []string{
		"批量下载修复",
		"下载模式对齐",
		"签名参数保持",
		"低码率防误保存",
		"边界回归覆盖",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("release highlights missing %q", expected)
		}
	}

	for _, retired := range []string{"评论异步导出", "进度保护完善"} {
		if strings.Contains(joined, retired) {
			t.Fatalf("release highlights still contain retired v5.7.0 item %q", retired)
		}
	}
}

func TestPrintTitleRendersCurrentReleaseHighlights(t *testing.T) {
	var output bytes.Buffer
	previousOutput := color.Output
	previousNoColor := color.NoColor
	color.Output = &output
	color.NoColor = true
	t.Cleanup(func() {
		color.Output = previousOutput
		color.NoColor = previousNoColor
	})

	app := &App{Cfg: &config.Config{Version: version.Current}}
	app.printTitle()
	rendered := output.String()

	if !strings.Contains(rendered, "v"+version.Current+" 更新要点") {
		t.Fatalf("startup title does not show current release version: %q", rendered)
	}
	for _, highlight := range releaseHighlights {
		if !strings.Contains(rendered, highlight) {
			t.Fatalf("startup title missing release highlight %q", highlight)
		}
	}
	for _, retired := range []string{"评论异步导出", "进度保护完善"} {
		if strings.Contains(rendered, retired) {
			t.Fatalf("startup title still contains retired v5.7.0 item %q", retired)
		}
	}
}
