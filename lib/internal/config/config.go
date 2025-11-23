package config

import (
	"os"
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

	// 安全配置
	SecretToken    string   // 本地授权令牌（可选，通过 WX_CHANNEL_TOKEN 注入）
	AllowedOrigins []string // 允许的 Origin 白名单（可选，通过 WX_CHANNEL_ALLOWED_ORIGINS 注入，逗号分隔）

	// 并发与限流
	UploadChunkConcurrency int // 分片上传并发上限
	UploadMergeConcurrency int // 合并并发上限
	DownloadConcurrency    int // 批量下载并发上限
	DownloadRetryCount     int // 批量下载重试次数

	// 日志配置
	LogFile      string // 日志文件路径（可选：WX_CHANNEL_LOG_FILE）
	MaxLogSizeMB int    // 单个日志文件最大 MB，达到后滚动（可选：WX_CHANNEL_LOG_MAX_MB）

	// 保存功能开关
	SavePageSnapshot bool // 是否保存页面快照（可选：WX_CHANNEL_SAVE_PAGE_SNAPSHOT，默认：true）
	SaveSearchData   bool // 是否保存搜索数据（可选：WX_CHANNEL_SAVE_SEARCH_DATA，默认：true）
	
	// UI 功能开关
	ShowLogButton bool // 是否显示左下角日志按钮（可选：WX_CHANNEL_SHOW_LOG_BUTTON，默认：false）
}

var globalConfig *Config

// Load 加载配置
func Load() *Config {
	if globalConfig == nil {
		globalConfig = &Config{
			Port:                   2025,                   // 监听端口（运行期可被命令行 -p/--port 覆盖）
			DefaultPort:            2025,                   // 参数解析失败时使用的默认端口
			Version:                "5.0.0.0",              // 版本号（用于前端缓存破坏等）
			DownloadsDir:           "downloads",            // 下载根目录
			RecordsFile:            "download_records.csv", // 下载记录 CSV 文件名
			CertFile:               "SunnyRoot.cer",        // 证书文件名（用于手动安装）
			MaxRetries:             3,                      // 前端分片上传失败重试次数
			ChunkSize:              2 << 20,                // 分片大小（字节），默认 2MB
			MaxUploadSize:          64 << 20,               // 服务端可接受的上传最大值（字节）
			BufferSize:             64 * 1024,              // 流式拷贝缓冲区大小（字节）
			CertInstallDelay:       3 * time.Second,        // 安装证书后的等待时间
			SaveDelay:              500 * time.Millisecond, // 某些保存动作的缓冲延迟
			SecretToken:            "",                     // 本地接口鉴权令牌（从 env WX_CHANNEL_TOKEN 注入）
			AllowedOrigins:         nil,                    // CORS 允许的 Origin 白名单（env WX_CHANNEL_ALLOWED_ORIGINS）
			UploadChunkConcurrency: 4,                      // 分片上传并发上限
			UploadMergeConcurrency: 1,                      // 分片合并并发上限
			DownloadConcurrency:    2,                      // 后端批量下载并发上限
			DownloadRetryCount:     3,                      // 后端批量下载重试次数
			LogFile:                "logs/wx_channel.log",  // 日志文件路径（默认开启）
			MaxLogSizeMB:           5,                      // 单个日志文件最大大小（MB），达到后滚动
			SavePageSnapshot:       true,                  // 默认开启页面快照保存
			SaveSearchData:         false,                  // 默认开启搜索数据保存
			ShowLogButton:          false,                  // 默认隐藏日志按钮
		}
		// 从环境变量加载可选令牌
		if token := os.Getenv("WX_CHANNEL_TOKEN"); token != "" {
			globalConfig.SecretToken = token
		}
		// 从环境变量加载可选 Origin 白名单（逗号分隔）
		if origins := os.Getenv("WX_CHANNEL_ALLOWED_ORIGINS"); origins != "" {
			// 简单切分并清理空格
			parts := []string{}
			start := 0
			for i := 0; i <= len(origins); i++ {
				if i == len(origins) || origins[i] == ',' {
					seg := origins[start:i]
					// 去空格
					trimmed := ""
					for j := 0; j < len(seg); j++ {
						if !(seg[j] == ' ' || seg[j] == '\t' || seg[j] == '\n' || seg[j] == '\r') {
							trimmed += string(seg[j])
						}
					}
					if trimmed != "" {
						parts = append(parts, trimmed)
					}
					start = i + 1
				}
			}
			globalConfig.AllowedOrigins = parts
		}

		// 日志环境变量
		if lf := os.Getenv("WX_CHANNEL_LOG_FILE"); lf != "" {
			globalConfig.LogFile = lf
		}
		if lmax := os.Getenv("WX_CHANNEL_LOG_MAX_MB"); lmax != "" {
			// 轻量解析
			n := 0
			for i := 0; i < len(lmax); i++ {
				if lmax[i] < '0' || lmax[i] > '9' {
					n = -1
					break
				}
			}
			if n != -1 {
				// 简化：若是纯数字，转为 int
				val := 0
				for i := 0; i < len(lmax); i++ {
					val = val*10 + int(lmax[i]-'0')
				}
				if val > 0 {
					globalConfig.MaxLogSizeMB = val
				}
			}
		}

		// 保存功能开关环境变量
		if saveSnapshot := os.Getenv("WX_CHANNEL_SAVE_PAGE_SNAPSHOT"); saveSnapshot != "" {
			globalConfig.SavePageSnapshot = saveSnapshot == "true" || saveSnapshot == "1" || saveSnapshot == "yes"
		}
		if saveSearch := os.Getenv("WX_CHANNEL_SAVE_SEARCH_DATA"); saveSearch != "" {
			globalConfig.SaveSearchData = saveSearch == "true" || saveSearch == "1" || saveSearch == "yes"
		}
		
		// UI 功能开关环境变量
		if showLogBtn := os.Getenv("WX_CHANNEL_SHOW_LOG_BUTTON"); showLogBtn != "" {
			globalConfig.ShowLogButton = showLogBtn == "true" || showLogBtn == "1" || showLogBtn == "yes"
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
