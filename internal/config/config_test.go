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

// MockDatabaseLoader 模拟数据库配置加载器
type MockDatabaseLoader struct {
	data map[string]string
}

func NewMockDatabaseLoader() *MockDatabaseLoader {
	return &MockDatabaseLoader{
		data: make(map[string]string),
	}
}

func (m *MockDatabaseLoader) Get(key string) (string, error) {
	value, exists := m.data[key]
	if !exists {
		return "", nil
	}
	return value, nil
}

func (m *MockDatabaseLoader) GetInt(key string, defaultValue int) (int, error) {
	value, err := m.Get(key)
	if err != nil || value == "" {
		return defaultValue, err
	}
	// 简单的字符串转整数
	result := 0
	for _, c := range value {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		} else {
			return defaultValue, nil
		}
	}
	return result, nil
}

func (m *MockDatabaseLoader) GetInt64(key string, defaultValue int64) (int64, error) {
	intVal, err := m.GetInt(key, int(defaultValue))
	return int64(intVal), err
}

func (m *MockDatabaseLoader) GetBool(key string, defaultValue bool) (bool, error) {
	value, err := m.Get(key)
	if err != nil || value == "" {
		return defaultValue, err
	}
	return value == "true" || value == "1", nil
}

func (m *MockDatabaseLoader) Set(key, value string) {
	m.data[key] = value
}

func TestConfigPriority(t *testing.T) {
	// 重置全局配置
	globalConfig = nil
	dbLoader = nil
	
	// 1. 测试默认配置
	cfg := Load()
	_ = cfg.DownloadsDir  // 记录默认值但不使用
	_ = cfg.ChunkSize     // 记录默认值但不使用
	_ = cfg.MaxRetries    // 记录默认值但不使用
	
	// 2. 测试数据库配置优先级最高
	globalConfig = nil // 重置
	mockDB := NewMockDatabaseLoader()
	mockDB.Set("download_dir", "db_downloads")
	mockDB.Set("chunk_size", "5242880") // 5MB
	mockDB.Set("max_retries", "5")
	
	SetDatabaseLoader(mockDB)
	cfg = Load()
	
	if cfg.DownloadsDir != "db_downloads" {
		t.Errorf("数据库配置应该优先，期望下载目录为 'db_downloads'，实际为 '%s'", cfg.DownloadsDir)
	}
	
	if cfg.ChunkSize != 5242880 {
		t.Errorf("数据库配置应该优先，期望分片大小为 5242880，实际为 %d", cfg.ChunkSize)
	}
	
	if cfg.MaxRetries != 5 {
		t.Errorf("数据库配置应该优先，期望最大重试次数为 5，实际为 %d", cfg.MaxRetries)
	}
	
	// 3. 测试重新加载配置
	mockDB.Set("max_retries", "10")
	cfg = Reload()
	
	if cfg.MaxRetries != 10 {
		t.Errorf("重新加载后，期望最大重试次数为 10，实际为 %d", cfg.MaxRetries)
	}
	
	// 清理
	globalConfig = nil
	dbLoader = nil
}

