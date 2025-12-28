package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wx_channel/internal/utils"
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
	UploadChunkConcurrency int           // 分片上传并发上限
	UploadMergeConcurrency int           // 合并并发上限
	DownloadConcurrency    int           // 批量下载并发上限
	DownloadRetryCount     int           // 批量下载重试次数
	DownloadResumeEnabled  bool          // 批量下载断点续传开关
	DownloadTimeout        time.Duration // 批量下载单个文件超时时间

	// 日志配置
	LogFile      string // 日志文件路径（可选：WX_CHANNEL_LOG_FILE）
	MaxLogSizeMB int    // 单个日志文件最大 MB，达到后滚动（可选：WX_CHANNEL_LOG_MAX_MB）

	// 保存功能开关
	SavePageSnapshot bool // 是否保存页面快照（可选：WX_CHANNEL_SAVE_PAGE_SNAPSHOT，默认：true）
	SaveSearchData   bool // 是否保存搜索数据（可选：WX_CHANNEL_SAVE_SEARCH_DATA，默认：true）
	SavePageJS       bool // 是否保存页面JS文件（可选：WX_CHANNEL_SAVE_PAGE_JS，默认：false）

	// UI 功能开关
	ShowLogButton bool // 是否显示左下角日志按钮（可选：WX_CHANNEL_SHOW_LOG_BUTTON，默认：false）
}

var globalConfig *Config

// DatabaseConfigLoader 数据库配置加载器接口
type DatabaseConfigLoader interface {
	Get(key string) (string, error)
	GetInt(key string, defaultValue int) (int, error)
	GetInt64(key string, defaultValue int64) (int64, error)
	GetBool(key string, defaultValue bool) (bool, error)
}

var dbLoader DatabaseConfigLoader

// SetDatabaseLoader 设置数据库配置加载器
func SetDatabaseLoader(loader DatabaseConfigLoader) {
	dbLoader = loader
}

// Load 加载配置
// 优先级：数据库配置 > 环境变量 > 软件默认配置
func Load() *Config {
	if globalConfig == nil {
		globalConfig = loadConfig()
	}
	return globalConfig
}

// Reload 重新加载配置（用于数据库初始化后重新加载）
func Reload() *Config {
	globalConfig = loadConfig()
	return globalConfig
}

// loadConfig 执行实际的配置加载逻辑
func loadConfig() *Config {
	// 1. 首先设置软件默认配置
	config := getDefaultConfig()

	// 2. 然后从环境变量覆盖配置
	loadFromEnvironment(config)

	// 3. 最后从数据库覆盖配置（优先级最高）
	loadFromDatabase(config)

	return config
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() *Config {
	return &Config{
		Port:                   2025,                   // 监听端口（运行期可被命令行 -p/--port 覆盖）
		DefaultPort:            2025,                   // 参数解析失败时使用的默认端口
		Version:                "5.2.10",               // 版本号（用于前端缓存破坏等）
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
		DownloadConcurrency:    5,                      // 后端批量下载并发上限
		DownloadRetryCount:     3,                      // 后端批量下载重试次数
		DownloadResumeEnabled:  true,                   // 默认开启断点续传
		DownloadTimeout:        30 * time.Minute,       // 单个文件下载超时
		LogFile:                "logs/wx_channel.log",  // 日志文件路径（默认开启）
		MaxLogSizeMB:           5,                      // 单个日志文件最大大小（MB），达到后滚动
		SavePageSnapshot:       false,                  // 默认关闭页面快照保存
		SaveSearchData:         false,                  // 默认关闭搜索数据保存
		SavePageJS:             false,                  // 默认关闭JS文件保存（用于页面分析）
		ShowLogButton:          false,                  // 默认隐藏日志按钮
	}
}

// loadFromEnvironment 从环境变量加载配置
func loadFromEnvironment(config *Config) {
	// 从环境变量加载可选令牌
	if token := os.Getenv("WX_CHANNEL_TOKEN"); token != "" {
		config.SecretToken = token
	}

	// 从环境变量加载可选 Origin 白名单（逗号分隔）
	if origins := os.Getenv("WX_CHANNEL_ALLOWED_ORIGINS"); origins != "" {
		config.AllowedOrigins = parseCommaSeparatedString(origins)
	}

	// 日志环境变量
	if lf := os.Getenv("WX_CHANNEL_LOG_FILE"); lf != "" {
		config.LogFile = lf
	}
	if lmax := os.Getenv("WX_CHANNEL_LOG_MAX_MB"); lmax != "" {
		if val := parsePositiveInt(lmax); val > 0 {
			config.MaxLogSizeMB = val
		}
	}

	// 保存功能开关环境变量
	if saveSnapshot := os.Getenv("WX_CHANNEL_SAVE_PAGE_SNAPSHOT"); saveSnapshot != "" {
		config.SavePageSnapshot = parseBool(saveSnapshot)
	}
	if saveSearch := os.Getenv("WX_CHANNEL_SAVE_SEARCH_DATA"); saveSearch != "" {
		config.SaveSearchData = parseBool(saveSearch)
	}
	if saveJS := os.Getenv("WX_CHANNEL_SAVE_PAGE_JS"); saveJS != "" {
		config.SavePageJS = parseBool(saveJS)
	}

	// UI 功能开关环境变量
	if showLogBtn := os.Getenv("WX_CHANNEL_SHOW_LOG_BUTTON"); showLogBtn != "" {
		config.ShowLogButton = parseBool(showLogBtn)
	}

	// 并发配置环境变量
	if uploadChunk := os.Getenv("WX_CHANNEL_UPLOAD_CHUNK_CONCURRENCY"); uploadChunk != "" {
		if val, err := strconv.Atoi(uploadChunk); err == nil && val > 0 {
			config.UploadChunkConcurrency = val
		}
	}
	if uploadMerge := os.Getenv("WX_CHANNEL_UPLOAD_MERGE_CONCURRENCY"); uploadMerge != "" {
		if val, err := strconv.Atoi(uploadMerge); err == nil && val > 0 {
			config.UploadMergeConcurrency = val
		}
	}
	if downloadConcurrency := os.Getenv("WX_CHANNEL_DOWNLOAD_CONCURRENCY"); downloadConcurrency != "" {
		if val, err := strconv.Atoi(downloadConcurrency); err == nil && val > 0 {
			config.DownloadConcurrency = val
		}
	}

	// 下载目录环境变量
	if downloadDir := os.Getenv("WX_CHANNEL_DOWNLOAD_DIR"); downloadDir != "" {
		config.DownloadsDir = downloadDir
	}

	// 分片大小环境变量
	if chunkSize := os.Getenv("WX_CHANNEL_CHUNK_SIZE"); chunkSize != "" {
		if val, err := strconv.ParseInt(chunkSize, 10, 64); err == nil && val > 0 {
			config.ChunkSize = val
		}
	}

	// 最大重试次数环境变量
	if maxRetries := os.Getenv("WX_CHANNEL_MAX_RETRIES"); maxRetries != "" {
		if val, err := strconv.Atoi(maxRetries); err == nil && val >= 0 {
			config.MaxRetries = val
		}
	}
}

// loadFromDatabase 从数据库加载配置（优先级最高）
func loadFromDatabase(config *Config) {
	if dbLoader == nil {
		return // 数据库加载器未设置，跳过
	}

	// 下载目录
	if downloadDir, err := dbLoader.Get("download_dir"); err == nil && downloadDir != "" {
		config.DownloadsDir = downloadDir
	}

	// 分片大小
	if chunkSize, err := dbLoader.GetInt64("chunk_size", config.ChunkSize); err == nil {
		config.ChunkSize = chunkSize
	}

	// 最大重试次数
	if maxRetries, err := dbLoader.GetInt("max_retries", config.MaxRetries); err == nil {
		config.MaxRetries = maxRetries
	}

	// 并发限制
	if concurrentLimit, err := dbLoader.GetInt("concurrent_limit", config.DownloadConcurrency); err == nil {
		config.DownloadConcurrency = concurrentLimit
	}

	// 自动清理开关
	if autoCleanup, err := dbLoader.GetBool("auto_cleanup_enabled", false); err == nil && autoCleanup {
		// 可以根据需要添加自动清理相关配置
	}

	// 主题设置
	if theme, err := dbLoader.Get("theme"); err == nil && theme != "" {
		// 可以根据需要添加主题相关配置
	}

	// 日志文件路径
	if logFile, err := dbLoader.Get("log_file"); err == nil && logFile != "" {
		config.LogFile = logFile
	}

	// 日志文件最大大小
	if maxLogSize, err := dbLoader.GetInt("max_log_size_mb", config.MaxLogSizeMB); err == nil {
		config.MaxLogSizeMB = maxLogSize
	}

	// 保存功能开关
	if saveSnapshot, err := dbLoader.GetBool("save_page_snapshot", config.SavePageSnapshot); err == nil {
		config.SavePageSnapshot = saveSnapshot
	}
	if saveSearch, err := dbLoader.GetBool("save_search_data", config.SaveSearchData); err == nil {
		config.SaveSearchData = saveSearch
	}
	if saveJS, err := dbLoader.GetBool("save_page_js", config.SavePageJS); err == nil {
		config.SavePageJS = saveJS
	}

	// UI 功能开关
	if showLogBtn, err := dbLoader.GetBool("show_log_button", config.ShowLogButton); err == nil {
		config.ShowLogButton = showLogBtn
	}

	// 上传并发配置
	if uploadChunk, err := dbLoader.GetInt("upload_chunk_concurrency", config.UploadChunkConcurrency); err == nil {
		config.UploadChunkConcurrency = uploadChunk
	}
	if uploadMerge, err := dbLoader.GetInt("upload_merge_concurrency", config.UploadMergeConcurrency); err == nil {
		config.UploadMergeConcurrency = uploadMerge
	}

	// 下载重试次数
	if downloadRetry, err := dbLoader.GetInt("download_retry_count", config.DownloadRetryCount); err == nil {
		config.DownloadRetryCount = downloadRetry
	}

	// 下载断点续传开关
	if downloadResume, err := dbLoader.GetBool("download_resume_enabled", config.DownloadResumeEnabled); err == nil {
		config.DownloadResumeEnabled = downloadResume
	}
}

// 辅助函数
func parseCommaSeparatedString(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parsePositiveInt(s string) int {
	if val, err := strconv.Atoi(s); err == nil && val > 0 {
		return val
	}
	return 0
}

func parseBool(s string) bool {
	return s == "true" || s == "1" || s == "yes"
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

// GetDownloadsDir 获取下载目录
func (c *Config) GetDownloadsDir() string {
	return c.DownloadsDir
}

// GetResolvedDownloadsDir 获取解析后的下载目录路径
func (c *Config) GetResolvedDownloadsDir() (string, error) {
	return utils.ResolveDownloadDir(c.DownloadsDir)
}

// GetRecordsPath 获取记录文件完整路径
func (c *Config) GetRecordsPath() string {
	downloadsDir, err := c.GetResolvedDownloadsDir()
	if err != nil {
		// 如果解析失败，使用原始路径
		return filepath.Join(c.DownloadsDir, c.RecordsFile)
	}
	return filepath.Join(downloadsDir, c.RecordsFile)
}
