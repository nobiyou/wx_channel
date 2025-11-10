package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizePath(t *testing.T) {
	baseDir := "/tmp/test"
	
	tests := []struct {
		name      string
		baseDir   string
		path      string
		wantError bool
	}{
		{
			name:      "正常路径",
			baseDir:   baseDir,
			path:      "videos/test.mp4",
			wantError: false,
		},
		{
			name:      "路径遍历攻击",
			baseDir:   baseDir,
			path:      "../../etc/passwd",
			wantError: true,
		},
		{
			name:      "相对路径",
			baseDir:   baseDir,
			path:      "./videos/test.mp4",
			wantError: false,
		},
		{
			name:      "空路径",
			baseDir:   baseDir,
			path:      "",
			wantError: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SanitizePath(tt.baseDir, tt.path)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("SanitizePath(%q, %q) 应该返回错误", tt.baseDir, tt.path)
				}
			} else {
				if err != nil {
					t.Errorf("SanitizePath(%q, %q) 返回错误: %v", tt.baseDir, tt.path, err)
				}
				if result == "" && tt.path != "" {
					t.Errorf("SanitizePath(%q, %q) 结果不应该为空", tt.baseDir, tt.path)
				}
			}
		})
	}
}

func TestEnsureDir(t *testing.T) {
	// 创建临时目录用于测试
	tmpDir := filepath.Join(os.TempDir(), "test_ensure_dir")
	defer os.RemoveAll(tmpDir)
	
	t.Run("创建新目录", func(t *testing.T) {
		testDir := filepath.Join(tmpDir, "new_dir")
		err := EnsureDir(testDir)
		if err != nil {
			t.Errorf("EnsureDir(%q) 返回错误: %v", testDir, err)
		}
		
		// 检查目录是否存在
		if _, err := os.Stat(testDir); os.IsNotExist(err) {
			t.Errorf("目录 %q 应该存在", testDir)
		}
	})
	
	t.Run("目录已存在", func(t *testing.T) {
		testDir := filepath.Join(tmpDir, "existing_dir")
		os.MkdirAll(testDir, 0755)
		
		err := EnsureDir(testDir)
		if err != nil {
			t.Errorf("EnsureDir(%q) 在目录已存在时返回错误: %v", testDir, err)
		}
	})
}

func TestGetBaseDir(t *testing.T) {
	baseDir, err := GetBaseDir()
	if err != nil {
		t.Errorf("GetBaseDir() 返回错误: %v", err)
	}
	
	if baseDir == "" {
		t.Error("GetBaseDir() 结果不应该为空")
	}
	
	// 检查路径是否存在
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		t.Errorf("基础目录 %q 应该存在", baseDir)
	}
}

