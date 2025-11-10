package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
	
	"github.com/fatih/color"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger 日志记录器
type Logger struct {
	level LogLevel
	file  *os.File
    maxSizeBytes int64
}

var defaultLogger *Logger

// InitLogger 初始化日志系统
func InitLogger(level LogLevel, logFile string) error {
	// 如果指定了日志文件，创建文件句柄
	var file *os.File
	var err error
	
	if logFile != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(logFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %v", err)
		}
		
		file, err = os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %v", err)
		}
	}
	
	defaultLogger = &Logger{
		level: level,
        file:  file,
        maxSizeBytes: 0,
	}
	
	return nil
}

// InitLoggerWithRotation 初始化带文件大小滚动的日志系统
func InitLoggerWithRotation(level LogLevel, logFile string, maxSizeMB int) error {
    var file *os.File
    var err error
    if logFile != "" {
        logDir := filepath.Dir(logFile)
        if err := os.MkdirAll(logDir, 0755); err != nil {
            return fmt.Errorf("创建日志目录失败: %v", err)
        }
        file, err = os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
        if err != nil {
            return fmt.Errorf("打开日志文件失败: %v", err)
        }
    }
    defaultLogger = &Logger{
        level: level,
        file:  file,
        maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
    }
    return nil
}

// GetLogger 获取默认日志记录器
func GetLogger() *Logger {
	if defaultLogger == nil {
		defaultLogger = &Logger{
			level: INFO,
			file:  nil,
		}
	}
	return defaultLogger
}

// log 内部日志方法
func (l *Logger) log(level string, colorFn func(string, ...interface{}) string, msg string, args ...interface{}) {
	formattedMsg := fmt.Sprintf(msg, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMsg := fmt.Sprintf("[%s] %s %s", timestamp, level, formattedMsg)
	
	// 输出到控制台（带颜色）
	fmt.Println(colorFn(logMsg))
	
	// 输出到文件（如果有）
    if l.file != nil {
        // 简易滚动：达到大小则关闭并按时间戳重命名
        if l.maxSizeBytes > 0 {
            if fi, err := l.file.Stat(); err == nil {
                if fi.Size()+int64(len(logMsg))+1 > l.maxSizeBytes {
                    _ = l.file.Close()
                    oldPath := fi.Name()
                    // Stat Name 仅文件名，需获取真实路径
                    // 这里简单处理：通过 Fd 无法获取路径，要求调用方使用固定路径打开，因此使用环境保存
                    // 折中方案：让调用方仅使用 InitLoggerWithRotation 并再次打开相同路径
                    // 通过 /proc/self/fd 不跨平台，这里改为：记录环境变量 WX_CHANNEL_LOG_FILE
                    lf := os.Getenv("WX_CHANNEL_LOG_FILE")
                    if lf == "" {
                        lf = oldPath
                    }
                    rotated := lf + "." + time.Now().Format("20060102_150405")
                    _ = os.Rename(lf, rotated)
                    // 重新打开原始路径
                    newFile, err := os.OpenFile(lf, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
                    if err == nil {
                        l.file = newFile
                    }
                }
            }
        }
        fmt.Fprintln(l.file, logMsg)
	}
}

// Debug 调试日志
func (l *Logger) Debug(msg string, args ...interface{}) {
	if l.level <= DEBUG {
		l.log("DEBUG", color.CyanString, msg, args...)
	}
}

// Info 信息日志
func (l *Logger) Info(msg string, args ...interface{}) {
	if l.level <= INFO {
		l.log("INFO", color.GreenString, msg, args...)
	}
}

// Warn 警告日志
func (l *Logger) Warn(msg string, args ...interface{}) {
	if l.level <= WARN {
		l.log("WARN", color.YellowString, msg, args...)
	}
}

// Error 错误日志
func (l *Logger) Error(msg string, args ...interface{}) {
	if l.level <= ERROR {
		l.log("ERROR", color.RedString, msg, args...)
	}
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// 便捷函数，使用默认日志记录器
func Debug(msg string, args ...interface{}) {
	GetLogger().Debug(msg, args...)
}

func Info(msg string, args ...interface{}) {
	GetLogger().Info(msg, args...)
}

func Warn(msg string, args ...interface{}) {
	GetLogger().Warn(msg, args...)
}

func Error(msg string, args ...interface{}) {
	GetLogger().Error(msg, args...)
}

