package utils

import (
	"path/filepath"
	"runtime"
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveDownloadDir(tt.downloadDir)
			
			if tt.expectError && err == nil {
				t.Errorf("ResolveDownloadDir() 期望错误但没有返回错误")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("ResolveDownloadDir() 意外错误 = %v", err)
			}
			
			if !tt.expectError && !filepath.IsAbs(result) {
				t.Errorf("ResolveDownloadDir() = %v, 期望绝对路径", result)
			}
		})
	}
}