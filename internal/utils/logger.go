package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	logLevelNames = map[LogLevel]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
	}
	logLevelColors = map[LogLevel]string{
		DEBUG: "\033[36m", // 青色
		INFO:  "\033[32m", // 绿色
		WARN:  "\033[33m", // 黄色
		ERROR: "\033[31m", // 红色
	}
	colorReset = "\033[0m"
)

// Logger 日志记录器
type Logger struct {
	mu          sync.Mutex
	file        *os.File
	logger      *log.Logger
	consoleLog  *log.Logger
	minLevel    LogLevel
	enableColor bool
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// InitLoggerWithRotation 初始化带日志轮转的日志系统
func InitLoggerWithRotation(level LogLevel, logFile string, maxSizeMB int) error {
	// 创建日志目录
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// 检查文件大小，如果超过限制则轮转
	if info, err := os.Stat(logFile); err == nil {
		sizeMB := info.Size() / (1024 * 1024)
		if int(sizeMB) >= maxSizeMB {
			// 轮转日志文件
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

	// 只写入文件，不写入控制台（控制台由 Info/Warn/Error 函数负责）
	defaultLogger = &Logger{
		file:        file,
		logger:      log.New(file, "", 0),
		consoleLog:  log.New(os.Stdout, "", 0),
		minLevel:    level,
		enableColor: true,
	}

	return nil
}

// GetLogger 获取默认日志记录器
func GetLogger() *Logger {
	if defaultLogger == nil {
		// 如果没有初始化，使用默认配置（固定文件名，支持轮转）
		_ = InitLoggerWithRotation(INFO, "logs/wx_channel.log", 5)
	}
	return defaultLogger
}

// SetLevel 设置最小日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// log 内部日志方法
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.minLevel {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelName := logLevelNames[level]
	message := fmt.Sprintf(format, args...)

	// 文件日志（无颜色，纯文本）
	fileLogLine := fmt.Sprintf("[%s] %s %s", timestamp, levelName, message)
	
	// 只写入文件
	l.logger.Println(fileLogLine)
}

// Debug 调试日志
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info 信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn 警告日志
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error 错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
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

// 全局便捷函数
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
	status := "成功"
	if !success {
		status = "失败"
	}
	sizeMB := float64(size) / (1024 * 1024)
	GetLogger().Info("[下载] %s | ID=%s | 标题=%s | 作者=%s | 大小=%.2fMB | URL=%s",
		status, videoID, title, author, sizeMB, url)
}

// LogComment 记录评论采集操作
func LogComment(videoID, title string, commentCount int, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().Info("[评论采集] %s | ID=%s | 标题=%s | 评论数=%d",
		status, videoID, title, commentCount)
}

// LogBatchDownload 记录批量下载操作
func LogBatchDownload(total, success, failed int) {
	GetLogger().Info("[批量下载] 完成 | 总数=%d | 成功=%d | 失败=%d",
		total, success, failed)
}

// LogAPI 记录API调用
func LogAPI(method, path string, statusCode int, duration time.Duration) {
	GetLogger().Info("[API] %s %s | 状态=%d | 耗时=%v",
		method, path, statusCode, duration)
}

// LogUploadInit 记录上传初始化
func LogUploadInit(uploadID string, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().Info("[上传初始化] %s | UploadID=%s", status, uploadID)
}

// LogUploadChunk 记录分片上传
func LogUploadChunk(uploadID string, index, total int, sizeMB float64, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().Info("[分片上传] %s | UploadID=%s | 分片=%d/%d | 大小=%.2fMB",
		status, uploadID, index+1, total, sizeMB)
}

// LogUploadMerge 记录分片合并
func LogUploadMerge(uploadID, filename, author string, totalChunks int, sizeMB float64, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().Info("[分片合并] %s | UploadID=%s | 文件=%s | 作者=%s | 分片数=%d | 大小=%.2fMB",
		status, uploadID, filename, author, totalChunks, sizeMB)
}

// LogDirectUpload 记录直接上传
func LogDirectUpload(filename, author string, sizeMB float64, encrypted bool, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	encStatus := ""
	if encrypted {
		encStatus = " [已解密]"
	}
	GetLogger().Info("[直接上传] %s | 文件=%s | 作者=%s | 大小=%.2fMB%s",
		status, filename, author, sizeMB, encStatus)
}

// LogCSVOperation 记录CSV操作
func LogCSVOperation(operation, videoID, title string, success bool, reason string) {
	status := "成功"
	if !success {
		status = "失败"
	}
	if reason != "" {
		GetLogger().Info("[CSV操作] %s | 操作=%s | ID=%s | 标题=%s | 原因=%s",
			status, operation, videoID, title, reason)
	} else {
		GetLogger().Info("[CSV操作] %s | 操作=%s | ID=%s | 标题=%s",
			status, operation, videoID, title)
	}
}

// LogCSVRebuild 记录CSV重建
func LogCSVRebuild(filePath string, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().Warn("[CSV重建] %s | 文件=%s", status, filePath)
}

// LogSystemStart 记录系统启动
func LogSystemStart(port int, proxyMode string) {
	GetLogger().Info("[系统启动] 服务已启动 | 端口=%d | 代理模式=%s", port, proxyMode)
}

// LogSystemShutdown 记录系统关闭
func LogSystemShutdown(reason string) {
	GetLogger().Info("[系统关闭] 服务正在关闭 | 原因=%s", reason)
}

// LogConfigLoad 记录配置加载
func LogConfigLoad(configPath string, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().Info("[配置加载] %s | 路径=%s", status, configPath)
}

// LogAuthFailed 记录认证失败
func LogAuthFailed(endpoint, clientIP string) {
	GetLogger().Warn("[认证失败] 端点=%s | 客户端IP=%s", endpoint, clientIP)
}

// LogCORSBlocked 记录CORS拦截
func LogCORSBlocked(origin, endpoint string) {
	GetLogger().Warn("[CORS拦截] 来源=%s | 端点=%s", origin, endpoint)
}

// LogDiskSpace 记录磁盘空间检查
func LogDiskSpace(path string, availableGB, totalGB float64) {
	usagePercent := (totalGB - availableGB) / totalGB * 100
	if usagePercent > 90 {
		GetLogger().Warn("[磁盘空间] 路径=%s | 可用=%.2fGB | 总计=%.2fGB | 使用率=%.1f%%",
			path, availableGB, totalGB, usagePercent)
	} else {
		GetLogger().Debug("[磁盘空间] 路径=%s | 可用=%.2fGB | 总计=%.2fGB | 使用率=%.1f%%",
			path, availableGB, totalGB, usagePercent)
	}
}

// LogConcurrency 记录并发状态
func LogConcurrency(operation string, active, max int) {
	GetLogger().Debug("[并发控制] 操作=%s | 活跃=%d | 最大=%d", operation, active, max)
}

// LogRetry 记录重试操作
func LogRetry(operation string, attempt, maxAttempts int, err error) {
	GetLogger().Warn("[重试] 操作=%s | 尝试=%d/%d | 错误=%v",
		operation, attempt, maxAttempts, err)
}

// LogCleanup 记录清理操作
func LogCleanup(operation string, itemsRemoved int, success bool) {
	status := "成功"
	if !success {
		status = "失败"
	}
	GetLogger().Info("[清理] %s | 操作=%s | 清理项数=%d", status, operation, itemsRemoved)
}
