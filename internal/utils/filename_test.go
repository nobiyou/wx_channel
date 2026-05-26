package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateVideoFilename_WithVideoIDByDefault(t *testing.T) {
	filename := GenerateVideoFilename("测试标题", "123456789", true)
	if filename != "测试标题_123456789.mp4" {
		t.Fatalf("filename = %s, want 测试标题_123456789.mp4", filename)
	}
}

func TestGenerateVideoFilename_WithoutVideoIDWhenDisabled(t *testing.T) {
	filename := GenerateVideoFilename("测试标题", "123456789", false)
	if filename != "测试标题" {
		t.Fatalf("filename = %s, want 测试标题", filename)
	}
}

func TestBuildVideoFilename_UsesTemplateWhenConfigured(t *testing.T) {
	meta := VideoFilenameMeta{
		Title:      "Welcome to my livestream",
		VideoID:    "vid_123",
		Author:     "ExampleCreator",
		Duration:   time.Hour + 54*time.Minute + 28*time.Second,
		CreateTime: time.Date(2026, 5, 20, 12, 30, 45, 0, time.Local),
	}

	filename := BuildVideoFilename(meta, false, "{date}_{author}_{title}_{duration}")
	want := "2026-05-20_ExampleCreator_Welcome to my livestream"
	if filename != want {
		t.Fatalf("filename = %s, want %s", filename, want)
	}
}

func TestBuildVideoFilename_FallsBackToDefaultWhenTemplateEmpty(t *testing.T) {
	meta := VideoFilenameMeta{
		Title:   "测试标题",
		VideoID: "123456789",
	}

	filename := BuildVideoFilename(meta, true, "")
	if filename != "测试标题_123456789.mp4" {
		t.Fatalf("filename = %s, want 测试标题_123456789.mp4", filename)
	}
}

func TestRenderFilenameTemplate_SkipsMissingFieldsAndTrimsSeparators(t *testing.T) {
	meta := VideoFilenameMeta{
		Title: "欢迎来到我的直播间",
	}

	filename := RenderFilenameTemplate(meta, "{date}_{author}_{title}_{duration}")
	if filename != "欢迎来到我的直播间" {
		t.Fatalf("filename = %s, want 欢迎来到我的直播间", filename)
	}
}

func TestRenderFilenameTemplate_IncludesVideoIDWhenRequested(t *testing.T) {
	meta := VideoFilenameMeta{
		Title:   "测试标题",
		VideoID: "video_001",
	}

	filename := RenderFilenameTemplate(meta, "{title}_{video_id}")
	if filename != "测试标题_video_001" {
		t.Fatalf("filename = %s, want 测试标题_video_001", filename)
	}
}

func TestRenderFilenameTemplate_FormatsSizeFromBytes(t *testing.T) {
	meta := VideoFilenameMeta{
		Title:     "测试标题",
		SizeBytes: 3 * 1024 * 1024,
	}

	filename := RenderFilenameTemplate(meta, "{title}_{size}")
	if filename != "测试标题_3.00 MB" {
		t.Fatalf("filename = %s, want 测试标题_3.00 MB", filename)
	}
}

func TestRenderFilenameTemplate_UsesSizeFallbackText(t *testing.T) {
	meta := VideoFilenameMeta{
		Title:    "测试标题",
		SizeText: "28.77MB",
	}

	filename := RenderFilenameTemplate(meta, "{title}_{size}")
	if filename != "测试标题_28.77MB" {
		t.Fatalf("filename = %s, want 测试标题_28.77MB", filename)
	}
}

func TestGenerateUniquePath_AppendsSequenceWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "测试标题.mp4")
	if err := os.WriteFile(base, []byte("x"), 0644); err != nil {
		t.Fatalf("prepare file failed: %v", err)
	}

	path := GenerateUniquePath(dir, "测试标题.mp4")
	if path == base {
		t.Fatalf("expected unique path, got %s", path)
	}
	if !strings.Contains(path, "测试标题(1).mp4") {
		t.Fatalf("path = %s, want suffix (1)", path)
	}
}

func TestCleanFilename_LongTitle(t *testing.T) {
	// 测试超长的中文标题（构造一个超过100字符的标题）
	longTitle := strings.Repeat("这是一个非常长的视频标题", 15) // 15 * 13 = 195 字符
	cleaned := CleanFilename(longTitle)

	// 检查长度是否被限制
	runes := []rune(cleaned)
	if len(runes) > 50 {
		t.Errorf("文件名长度超过限制: 期望 <= 50, 实际 = %d", len(runes))
	}

	// 检查是否截断（不应该包含省略号，因为实现中移除了）
	if strings.HasSuffix(cleaned, "...") {
		t.Errorf("超长文件名不应该以 '...' 结尾")
	}

	t.Logf("原始标题长度: %d 字符", len([]rune(longTitle)))
	t.Logf("清理后标题: %s", cleaned)
	t.Logf("清理后长度: %d 字符", len(runes))
}

func TestCleanFilename_NormalTitle(t *testing.T) {
	// 测试正常长度的标题
	normalTitle := "这是一个正常的视频标题"
	cleaned := CleanFilename(normalTitle)

	if cleaned != normalTitle {
		t.Errorf("正常标题不应该被修改: 期望 = %s, 实际 = %s", normalTitle, cleaned)
	}
}

func TestCleanFilename_IllegalChars(t *testing.T) {
	// 测试包含非法字符的标题
	illegalTitle := "视频<标题>:测试/文件\\名称|问号?星号*"
	cleaned := CleanFilename(illegalTitle)

	// 检查非法字符是否被替换
	illegalChars := `<>:"/\|?*`
	for _, char := range illegalChars {
		if strings.ContainsRune(cleaned, char) {
			t.Errorf("清理后的文件名仍包含非法字符: %c", char)
		}
	}

	t.Logf("原始标题: %s", illegalTitle)
	t.Logf("清理后标题: %s", cleaned)
}

func TestCleanFolderName_LongName(t *testing.T) {
	// 测试超长的作者名称（构造一个超过50字符的名称）
	longName := strings.Repeat("这是一个非常长的作者名称", 8) // 8 * 13 = 104 字符
	cleaned := CleanFolderName(longName)

	// 检查长度是否被限制
	runes := []rune(cleaned)
	if len(runes) > 50 { // 限制为 50 个字符（去除末尾点后可能少于 50）
		t.Errorf("文件夹名长度超过限制: 期望 <= 50, 实际 = %d", len(runes))
	}

	// 检查是否以点结尾（不应该，因为会被去除）
	if strings.HasSuffix(cleaned, ".") {
		t.Errorf("文件夹名不应该以点结尾（Windows 会自动去除）")
	}

	t.Logf("原始名称长度: %d 字符", len([]rune(longName)))
	t.Logf("清理后名称: %s", cleaned)
	t.Logf("清理后长度: %d 字符", len(runes))
}

func TestCleanFolderName_EmptyName(t *testing.T) {
	// 测试空名称
	cleaned := CleanFolderName("")

	if cleaned != "未知作者" {
		t.Errorf("空名称应该返回默认值: 期望 = 未知作者, 实际 = %s", cleaned)
	}
}

func TestCleanFolderName_TrailingDots(t *testing.T) {
	// 测试末尾点的处理（Windows 文件系统会自动去除末尾的点）
	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		{"机器..", "机器", "去除末尾两个点"},
		{"机器...", "机器", "去除末尾三个点"},
		{"机器.", "机器", "去除末尾一个点"},
		{"机器.测试", "机器.测试", "中间的点保留"},
		{".机器", ".机器", "开头的点保留"},
		{"机器", "机器", "没有点，不变"},
		{"测试..", "测试", "去除末尾两个点"},
		{"测试.", "测试", "去除末尾一个点"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cleaned := CleanFolderName(tc.input)
			if cleaned != tc.expected {
				t.Errorf("输入: %q, 期望: %q, 实际: %q", tc.input, tc.expected, cleaned)
			}
		})
	}
}
