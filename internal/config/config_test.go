package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	cfg := Load()
	
	if cfg == nil {
		t.Fatal("Load() 返回 nil")
	}
	
	// 测试默认值
	if cfg.Port == 0 {
		t.Error("默认端口不应该为0")
	}
	
	if cfg.DefaultPort == 0 {
		t.Error("默认端口不应该为0")
	}
	
	if cfg.Version == "" {
		t.Error("版本号不应该为空")
	}
	
	if cfg.DownloadsDir == "" {
		t.Error("下载目录不应该为空")
	}
	
	if cfg.MaxRetries <= 0 {
		t.Error("最大重试次数应该大于0")
	}
	
	if cfg.ChunkSize <= 0 {
		t.Error("分片大小应该大于0")
	}
}

func TestGet(t *testing.T) {
	cfg1 := Get()
	cfg2 := Get()
	
	if cfg1 != cfg2 {
		t.Error("Get() 应该返回相同的配置实例")
	}
}

func TestSetPort(t *testing.T) {
	cfg := Load()
	originalPort := cfg.Port
	
	// 测试设置端口
	newPort := 8080
	cfg.SetPort(newPort)
	
	if cfg.Port != newPort {
		t.Errorf("SetPort(%d) 后，端口应该是 %d，但得到 %d", newPort, newPort, cfg.Port)
	}
	
	// 恢复原始端口
	cfg.SetPort(originalPort)
}

func TestGetRecordsPath(t *testing.T) {
	cfg := Load()
	path := cfg.GetRecordsPath()
	
	if path == "" {
		t.Error("GetRecordsPath() 结果不应该为空")
	}
	
	// 路径应该包含下载目录和记录文件名
	if !contains(path, cfg.DownloadsDir) {
		t.Errorf("记录路径 %q 应该包含下载目录 %q", path, cfg.DownloadsDir)
	}
	
	if !contains(path, cfg.RecordsFile) {
		t.Errorf("记录路径 %q 应该包含记录文件名 %q", path, cfg.RecordsFile)
	}
}

// contains 检查字符串是否包含子串（简单实现）
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

