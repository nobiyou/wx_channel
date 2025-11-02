package utils

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// CleanFilename 清理文件名，移除非法字符
func CleanFilename(filename string) string {
	// 移除Windows非法文件名字符
	filename = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		// 控制字符：换行、回车、制表符等
		if r < 32 || r == 127 {
			return '_'
		}
		return r
	}, filename)

	// 去除首尾空格
	filename = strings.TrimSpace(filename)

	// 如果文件名为空，使用默认名称
	if filename == "" {
		filename = "video_" + time.Now().Format("20060102_150405")
	}

	return filename
}

// CleanFolderName 清理文件夹名称
func CleanFolderName(folderName string) string {
	cleaned := CleanFilename(folderName)

	// 如果文件夹名为空，使用默认名称
	if cleaned == "" {
		cleaned = "未知作者"
	}

	return cleaned
}

// EnsureExtension 确保文件名有指定的扩展名
func EnsureExtension(filename, ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	if !strings.HasSuffix(strings.ToLower(filename), strings.ToLower(ext)) {
		return filename + ext
	}
	return filename
}

// GenerateUniqueFilename 生成唯一的文件名，避免覆盖
func GenerateUniqueFilename(dir, filename string, maxAttempts int) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	ext := filepath.Ext(filename)

	for i := 1; i < maxAttempts; i++ {
		candidate := filepath.Join(dir, filename)
		if _, err := filepath.EvalSymlinks(candidate); err != nil {
			// 文件不存在，可以使用
			return candidate
		}

		// 文件存在，尝试添加序号
		filename = fmt.Sprintf("%s(%d)%s", base, i, ext)
	}

	// 如果所有尝试都失败，添加时间戳
	timestamp := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("%s_%s%s", base, timestamp, ext))
}
