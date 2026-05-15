package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	// 保持对外的 Logger 结构，但内部换成 zerolog
	defaultLogger *Logger
	once          sync.Once
)

// Logger 封装 zerolog
type Logger struct {
	mu         sync.Mutex
	zLogger    zerolog.Logger
	fileLogger zerolog.Logger
	file       *os.File
	minLevel   LogLevel
}

// InitLoggerWithRotation 初始化带日志轮转的日志系统
func InitLoggerWithRotation(level LogLevel, logFile string, maxSizeMB int) error {
	// 创建日志目录
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// 检查文件大小，如果超过限制则轮转 (保持原有的简单启动时轮转逻辑)
	if info, err := os.Stat(logFile); err == nil {
		sizeMB := info.Size() / (1024 * 1024)
		if int(sizeMB) >= maxSizeMB {
			timestamp := time.Now().Format("20060102_150405")
			backupFile := fmt.Sprintf("%s.%s", logFile, timestamp)
			_ = os.Rename(logFile, backupFile)
		}
	}

	// 打开日志文件
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// 配置 zerolog
	// 文件输出 JSON (或普通文本，这里为了现代化建议用 JSON，但为了人类可读性，zerolog file 一般也推荐 ConsoleWriter 也可以，或者纯 JSON)
	// 如果用户想要 "现代化"，JSON 是更好的机器可读格式。但为了兼容原有 "cat log" 的体验，可能 ConsoleWriter (无颜色) 更好？
	// 让我们同时输出到控制台(带颜色)和文件(JSON 或 纯文本)。
	// 以前的逻辑是：只写入文件，不写入控制台（控制台由 Info/Warn/Error 函数负责? 不，原代码 consoleLog 也是 log.New(os.Stdout...)）
	// 原代码：defaultLogger.logger 写入 file, defaultLogger.consoleLog 写入 stdout.

	// Zerolog MultiLevelWriter
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "2006-01-02 15:04:05"}
	// fileWriter := file // 文件里存 JSON，方便分析？或者也存文本？
	// 为了方便查看，文件里我们也暂时用 ConsoleWriter (no color) 或者是纯 JSON。
	// 通常生产环境文件存 JSON。但这是一个客户端工具，由用户直接看日志，可能文本更好。
	// 让我们让文件也存文本格式，或者 JSON。
	// 考虑到 gap analysis 说 "Standardize error handling" 和 "Modernization", JSON is safer.
	// 但是用户之前的日志是文本的。
	// 让我们用 zerolog 的 ConsoleWriter 但去除颜色写入文件，这样格式好看。

	// fileOutput := zerolog.ConsoleWriter{Out: file, TimeFormat: "2006-01-02 15:04:05", NoColor: true}
	// 为了更好的机器可读性和标准实践，文件输出应使用普通 JSON
	// 但为了匹配之前的行为（纯文本），让我们使用不带颜色的 ConsoleWriter
	fileOutput := zerolog.ConsoleWriter{Out: file, TimeFormat: "2006-01-02 15:04:05", NoColor: true}

	multi := zerolog.MultiLevelWriter(consoleWriter, fileOutput)

	zLog := zerolog.New(multi).With().Timestamp().Logger()
	fLog := zerolog.New(fileOutput).With().Timestamp().Logger()

	// 设置级别
	var zLevel zerolog.Level
	switch level {
	case DEBUG:
		zLevel = zerolog.DebugLevel
	case INFO:
		zLevel = zerolog.InfoLevel
	case WARN:
		zLevel = zerolog.WarnLevel
	case ERROR:
		zLevel = zerolog.ErrorLevel
	default:
		zLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(zLevel)

	defaultLogger = &Logger{
		file:       file,
		zLogger:    zLog,
		fileLogger: fLog,
		minLevel:   level,
	}

	// 同时替换全局 log，以防甚至第三方库用 log.Print
	log.Logger = zLog

	return nil
}

// GetLogger 获取默认日志记录器
func GetLogger() *Logger {
	if defaultLogger == nil {
		// 默认初始化
		_ = InitLoggerWithRotation(INFO, "logs/wx_channel.log", 5)
	}
	return defaultLogger
}

// SetLevel 设置最小日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level

	var zLevel zerolog.Level
	switch level {
	case DEBUG:
		zLevel = zerolog.DebugLevel
	case INFO:
		zLevel = zerolog.InfoLevel
	case WARN:
		zLevel = zerolog.WarnLevel
	case ERROR:
		zLevel = zerolog.ErrorLevel
	default:
		zLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(zLevel)
}

// Debug 调试日志
func (l *Logger) Debug(format string, args ...interface{}) {
	l.zLogger.Debug().Msgf(format, args...)
}

// Info 信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	l.zLogger.Info().Msgf(format, args...)
}

// Warn 警告日志
func (l *Logger) Warn(format string, args ...interface{}) {
	l.zLogger.Warn().Msgf(format, args...)
}

// Error 错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	l.zLogger.Error().Msgf(format, args...)
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Info 信息日志
func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

// Warn 警告日志
func Warn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

// Error 错误日志
func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

// LogDebug 全局便捷函数
func LogDebug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

func LogInfo(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

func LogWarn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

func LogError(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

// LogDownload 记录下载操作
func LogDownload(videoID, title, author, url string, size int64, success bool) {
	event := GetLogger().zLogger.Info()
	if !success {
		event = GetLogger().zLogger.Warn()
	}

	status := "成功"
	if !success {
		status = "失败"
	}
	sizeMB := float64(size) / (1024 * 1024)

	// 使用 structured logging 字段
	event.Str("type", "下载").
		Str("status", status).
		Str("id", videoID).
		Str("title", title).
		Str("author", author).
		Float64("size_mb", sizeMB).
		Str("url", url).
		Msgf("[下载] %s | %s", title, status)
}

// LogComment 记录评论导出操作
func LogComment(videoID, title string, commentCount int, success bool) {
	event := GetLogger().zLogger.Info()
	if !success {
		event = GetLogger().zLogger.Warn()
	}
	status := "成功"
	if !success {
		status = "失败"
	}
	event.Str("type", "评论导出").
		Str("status", status).
		Str("id", videoID).
		Str("title", title).
		Int("count", commentCount).
		Msgf("[评论导出] %s | %d条", title, commentCount)
}

// LogBatchDownload 记录批量下载操作
func LogBatchDownload(total, success, failed int) {
	GetLogger().zLogger.Info().
		Str("type", "批量下载").
		Int("total", total).
		Int("success", success).
		Int("failed", failed).
		Msg("批量下载完成")
}

// LogDownloadError 记录下载错误详情
func LogDownloadError(videoID, title, author, url string, err error, retryCount int) {
	GetLogger().zLogger.Error().
		Str("type", "下载错误").
		Str("id", videoID).
		Str("title", title).
		Str("author", author).
		Int("retry_count", retryCount).
		Err(err).
		Str("url", url).
		Msg("下载发生错误")
}

// LogDownloadRetry 记录下载重试
func LogDownloadRetry(videoID, title string, attempt, maxRetries int, err error) {
	GetLogger().zLogger.Warn().
		Str("type", "下载重试").
		Str("id", videoID).
		Str("title", title).
		Int("attempt", attempt).
		Int("max_retries", maxRetries).
		Err(err).
		Msgf("正在重试 %d/%d", attempt, maxRetries)
}

// LogAPI 记录API调用
func LogAPI(method, path string, statusCode int, duration time.Duration) {
	GetLogger().zLogger.Info().
		Str("type", "API").
		Str("method", method).
		Str("path", path).
		Int("status", statusCode).
		Dur("duration", duration).
		Msgf("%s %s %d", method, path, statusCode)
}

// LogUploadInit 记录上传初始化
func LogUploadInit(uploadID string, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().zLogger.Info().
		Str("type", "上传初始化").
		Str("upload_id", uploadID).
		Str("status", status).
		Msgf("上传初始化 %s", status)
}

// LogUploadChunk 记录分片上传
func LogUploadChunk(uploadID string, index, total int, sizeMB float64, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().zLogger.Info().
		Str("type", "分片上传").
		Str("upload_id", uploadID).
		Int("index", index+1).
		Int("total", total).
		Float64("size_mb", sizeMB).
		Str("status", status).
		Msgf("分片 %d/%d %s", index+1, total, status)
}

// LogUploadMerge 记录分片合并
func LogUploadMerge(uploadID, filename, author string, totalChunks int, sizeMB float64, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().zLogger.Info().
		Str("type", "分片合并").
		Str("upload_id", uploadID).
		Str("filename", filename).
		Str("author", author).
		Int("chunks", totalChunks).
		Float64("size_mb", sizeMB).
		Str("status", status).
		Msgf("合并文件 %s %s", filename, status)
}

// LogDirectUpload 记录直接上传
func LogDirectUpload(filename, author string, sizeMB float64, encrypted bool, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().zLogger.Info().
		Str("type", "直接上传").
		Str("filename", filename).
		Str("author", author).
		Float64("size_mb", sizeMB).
		Bool("encrypted", encrypted).
		Str("status", status).
		Msgf("直接上传 %s %s", filename, status)
}

// LogCSVOperation 记录CSV操作
func LogCSVOperation(operation, videoID, title string, success bool, reason string) {
	status := "成功"
	if !success {
		status = "失败"
	}
	event := GetLogger().zLogger.Info().
		Str("type", "CSV操作").
		Str("operation", operation).
		Str("id", videoID).
		Str("title", title).
		Str("status", status)

	if reason != "" {
		event.Str("reason", reason)
	}
	event.Msgf("CSV %s %s", operation, status)
}

// LogCSVRebuild 记录CSV重建
func LogCSVRebuild(filePath string, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().zLogger.Warn().
		Str("type", "CSV重建").
		Str("file", filePath).
		Str("status", status).
		Msgf("重建CSV %s", status)
}

// LogSystemStart 记录系统启动
func LogSystemStart(port int, proxyMode string) {
	GetLogger().zLogger.Info().
		Str("type", "系统启动").
		Int("port", port).
		Str("proxy_mode", proxyMode).
		Msg("服务已启动")
}

// LogSystemShutdown 记录系统关闭
func LogSystemShutdown(reason string) {
	GetLogger().zLogger.Info().
		Str("type", "系统关闭").
		Str("reason", reason).
		Msg("服务正在关闭")
}

// LogConfigLoad 记录配置加载
func LogConfigLoad(configPath string, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().zLogger.Info().
		Str("type", "配置加载").
		Str("path", configPath).
		Str("status", status).
		Msgf("加载配置 %s", status)
}

// LogAuthFailed 记录认证失败
func LogAuthFailed(endpoint, clientIP string) {
	GetLogger().zLogger.Warn().
		Str("type", "认证失败").
		Str("endpoint", endpoint).
		Str("ip", clientIP).
		Msg("访问被拒绝")
}

// LogCORSBlocked 记录CORS拦截
func LogCORSBlocked(origin, endpoint string) {
	GetLogger().zLogger.Warn().
		Str("type", "CORS拦截").
		Str("origin", origin).
		Str("endpoint", endpoint).
		Msg("跨域请求被拦截")
}

// LogDiskSpace 记录磁盘空间检查
func LogDiskSpace(path string, availableGB, totalGB float64) {
	usagePercent := (totalGB - availableGB) / totalGB * 100
	event := GetLogger().zLogger.Debug()
	if usagePercent > 90 {
		event = GetLogger().zLogger.Warn()
	}
	event.Str("type", "磁盘检查").
		Str("path", path).
		Float64("available_gb", availableGB).
		Float64("total_gb", totalGB).
		Float64("usage_percent", usagePercent).
		Msgf("磁盘使用率 %.1f%%", usagePercent)
}

// LogConcurrency 记录并发状态
func LogConcurrency(operation string, active, max int) {
	GetLogger().zLogger.Debug().
		Str("type", "并发控制").
		Str("operation", operation).
		Int("active", active).
		Int("max", max).
		Msg("并发状态更新")
}

// LogRetry 记录重试操作
func LogRetry(operation string, attempt, maxAttempts int, err error) {
	GetLogger().zLogger.Warn().
		Str("type", "重试").
		Str("operation", operation).
		Int("attempt", attempt).
		Int("max_attempts", maxAttempts).
		Err(err).
		Msg("操作重试")
}

// LogCleanup 记录清理操作
func LogCleanup(operation string, itemsRemoved int, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().zLogger.Info().
		Str("type", "清理").
		Str("operation", operation).
		Int("removed", itemsRemoved).
		Str("status", status).
		Msgf("清理完成 %s", status)
}

// FileInfo 仅记录到文件的信息日志
func (l *Logger) FileInfo(format string, args ...interface{}) {
	l.fileLogger.Info().Msgf(format, args...)
}

// LogFileInfo 仅记录到文件的信息日志(全局)
func LogFileInfo(format string, args ...interface{}) {
	GetLogger().FileInfo(format, args...)
}
