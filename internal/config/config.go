package config

import (
	"time"
)

// Config 应用程序配置
type Config struct {
	// 网络配置
	Port        int
	DefaultPort int

	// 应用信息
	Version string

	// 文件路径配置
	DownloadsDir string
	RecordsFile  string
	CertFile     string

	// 上传配置
	MaxRetries    int   // 最大重试次数
	ChunkSize     int64 // 分片大小（字节）
	MaxUploadSize int64 // 最大上传大小（字节）
	BufferSize    int64 // 缓冲区大小（字节）

	// 时间配置
	CertInstallDelay time.Duration // 证书安装延迟
	SaveDelay        time.Duration // 保存延迟
}

var globalConfig *Config

// Load 加载配置
func Load() *Config {
	if globalConfig == nil {
		globalConfig = &Config{
			Port:             2025,
			DefaultPort:      2025,
			Version:          "20251018",
			DownloadsDir:     "downloads",
			RecordsFile:      "download_records.csv",
			CertFile:         "SunnyRoot.cer",
			MaxRetries:       3,
			ChunkSize:        2 << 20,   // 2MB
			MaxUploadSize:    64 << 20,  // 64MB
			BufferSize:       64 * 1024, // 64KB
			CertInstallDelay: 3 * time.Second,
			SaveDelay:        500 * time.Millisecond,
		}
	}
	return globalConfig
}

// Get 获取全局配置
func Get() *Config {
	if globalConfig == nil {
		return Load()
	}
	return globalConfig
}

// SetPort 设置端口
func (c *Config) SetPort(port int) {
	c.Port = port
}

// GetRecordsPath 获取记录文件完整路径
func (c *Config) GetRecordsPath() string {
	return c.DownloadsDir + "/" + c.RecordsFile
}
