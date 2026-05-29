package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveDownloadDir(t *testing.T) {
	tests := []struct {
		name        string
		downloadDir string
		expectAbs   bool
	}{
		{
			name:        "相对路径",
			downloadDir: "downloads",
			expectAbs:   true,
		},
		{
			name:        "绝对路径 - Windows",
			downloadDir: "C:\\downloads",
			expectAbs:   true,
		},
		{
			name:        "绝对路径 - Unix",
			downloadDir: "/tmp/downloads",
			expectAbs:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 跳过不适用于当前操作系统的测试
			if runtime.GOOS == "windows" && tt.downloadDir == "/tmp/downloads" {
				t.Skip("跳过Unix路径测试（当前为Windows）")
			}
			if runtime.GOOS != "windows" && tt.downloadDir == "C:\\downloads" {
				t.Skip("跳过Windows路径测试（当前为非Windows）")
			}

			result, err := ResolveDownloadDir(tt.downloadDir)
			if err != nil {
				t.Errorf("ResolveDownloadDir() error = %v", err)
				return
			}

			if tt.expectAbs && !filepath.IsAbs(result) {
				t.Errorf("ResolveDownloadDir() = %v, 期望绝对路径", result)
			}

			// 对于相对路径，结果应该包含原始路径
			if !filepath.IsAbs(tt.downloadDir) {
				if !contains(result, tt.downloadDir) {
					t.Errorf("ResolveDownloadDir() = %v, 应该包含 %v", result, tt.downloadDir)
				}
			}

			// 对于绝对路径，结果应该等于输入
			if filepath.IsAbs(tt.downloadDir) {
				if result != tt.downloadDir {
					t.Errorf("ResolveDownloadDir() = %v, 期望 %v", result, tt.downloadDir)
				}
			}
		})
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestResolveDownloadDirEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		downloadDir string
		expectError bool
	}{
		{
			name:        "空字符串",
			downloadDir: "",
			expectError: false, // 应该返回基础目录
		},
		{
			name:        "点路径",
			downloadDir: ".",
			expectError: false,
		},
		{
			name:        "双点路径",
			downloadDir: "..",
			expectError: false,
		},
	}

	baseDir, err := GetBaseDir()
	if err != nil {
		t.Fatalf("无法获取基础目录: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveDownloadDir(tt.downloadDir)

			if tt.expectError && err == nil {
				t.Errorf("ResolveDownloadDir() 期望错误但没有返回错误")
			}

			if !tt.expectError && err != nil {
				t.Errorf("ResolveDownloadDir() 意外错误 = %v", err)
			}

			if !tt.expectError {
				if !filepath.IsAbs(result) {
					t.Errorf("ResolveDownloadDir() = %v, 期望绝对路径", result)
				}

				// 验证结果是否正确解析
				expected := filepath.Join(baseDir, tt.downloadDir)
				if result != expected {
					// 尝试 Clean 后比较，因为 filepath.Join 可能会清理路径
					if filepath.Clean(result) != filepath.Clean(expected) {
						t.Errorf("ResolveDownloadDir() = %v, 期望 %v", result, expected)
					}
				}
			}
		})
	}
}

func TestBuildTempDownloadPath_UsesTargetDirAndHint(t *testing.T) {
	target := filepath.Join("downloads", "author", "video.mp4")
	got := BuildTempDownloadPath(target, "video-123")

	if filepath.Dir(got) != filepath.Dir(target) {
		t.Fatalf("BuildTempDownloadPath() dir = %s, want %s", filepath.Dir(got), filepath.Dir(target))
	}
	if !strings.Contains(filepath.Base(got), "video.mp4.video-123.tmp") {
		t.Fatalf("BuildTempDownloadPath() = %s, want suffix with hint", got)
	}
}

func TestMoveFileToAvailablePath_RenamesWhenTargetExists(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "video.mp4")
	src := filepath.Join(dir, "video.mp4.task1.tmp")

	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatalf("write target failed: %v", err)
	}
	if err := os.WriteFile(src, []byte("new"), 0644); err != nil {
		t.Fatalf("write src failed: %v", err)
	}

	got, err := MoveFileToAvailablePath(src, target)
	if err != nil {
		t.Fatalf("MoveFileToAvailablePath() error = %v", err)
	}
	if got == target {
		t.Fatalf("MoveFileToAvailablePath() = %s, want renamed path", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src should be moved away, stat err = %v", err)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read moved file failed: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("moved file contents = %q, want %q", string(data), "new")
	}
}
