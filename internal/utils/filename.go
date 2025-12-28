package utils

import (
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"
)

// CleanFilename 清理文件名，移除非法字符
func CleanFilename(filename string) string {
	// 先移除HTML标签（如 <em class="highlight">纪录片</em>）
	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)
	filename = htmlTagRegex.ReplaceAllString(filename, "")
	
	// 处理常见的HTML实体
	htmlEntities := map[string]string{
		"&nbsp;": " ",
		"&amp;":  "&",
		"&lt;":   "<",
		"&gt;":   ">",
		"&quot;": "\"",
		"&apos;": "'",
		"&#39;":  "'",
		"&#34;":  "\"",
	}
	for entity, replacement := range htmlEntities {
		filename = strings.ReplaceAll(filename, entity, replacement)
	}
	
	// 移除剩余的HTML实体（如 &#123; 或 &unknown;）
	htmlEntityRegex := regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
	filename = htmlEntityRegex.ReplaceAllString(filename, "")
	
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

	// 限制文件名长度，避免路径过长导致保存失败
	// Windows 路径限制为 260 字符，考虑到目录路径和扩展名，文件名主体限制为 50 个字符
	// 使用 rune 而不是 byte 来正确处理中文等多字节字符
	maxLength := 50
	runes := []rune(filename)
	if len(runes) > maxLength {
		// 截断，不添加省略号（避免文件名中出现特殊字符）
		filename = string(runes[:maxLength])
	}

	return filename
}

// CleanFolderName 清理文件夹名称
func CleanFolderName(folderName string) string {
	// 先检查是否为空，避免 CleanFilename 生成时间戳名称
	if strings.TrimSpace(folderName) == "" {
		return "未知作者"
	}
	
	cleaned := CleanFilename(folderName)

	// 如果清理后为空（理论上不会发生，因为 CleanFilename 会生成默认名称），使用默认名称
	if cleaned == "" {
		cleaned = "未知作者"
	}

	// 文件夹名称也需要限制长度，但可以稍微宽松一些
	// 限制为 50 个字符，避免路径过长
	maxLength := 50
	runes := []rune(cleaned)
	if len(runes) > maxLength {
		cleaned = string(runes[:maxLength]) + "..."
	}

	return cleaned
}

// EnsureExtension 确保文件名有指定的扩展名
func EnsureExtension(filename, ext string) string {
    if !strings.HasPrefix(ext, ".") {
        ext = "." + ext
    }

    // 获取当前文件的扩展名
    currentExt := filepath.Ext(filename)
    
    // 如果当前扩展名与期望的扩展名相同，则保持不变
    if currentExt == ext {
        return filename
    }
    
    // 如果当前扩展名与期望的不同，追加新的扩展名
    // 如果没有扩展名，直接添加
    return filename + ext
}

// GenerateUniqueFilename 生成唯一的文件名，避免覆盖
func GenerateUniqueFilename(dir, filename string, maxAttempts int) string {
    base := strings.TrimSuffix(filename, filepath.Ext(filename))
    ext := filepath.Ext(filename)

    for i := 1; i < maxAttempts; i++ {
        candidate := filepath.Join(dir, filename)
        if _, err := os.Stat(candidate); os.IsNotExist(err) {
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

// GenerateVideoFilename 根据视频标题和ID生成文件名
// 优先使用视频ID确保唯一性，如果标题相同但ID不同，文件名会包含ID
// 格式：标题_ID.mp4 或 标题.mp4（如果没有ID）
func GenerateVideoFilename(title, videoID string) string {
	// 清理标题
	var filename string
	if title != "" {
		filename = CleanFilename(title)
	} else if videoID != "" {
		filename = "video_" + videoID
	} else {
		filename = "video_" + time.Now().Format("20060102_150405")
	}

	// 如果有视频ID，在文件名中包含ID以确保唯一性
	// 格式：标题_ID.mp4
	if videoID != "" {
		// 检查文件名中是否已包含ID（避免重复添加）
		idPattern := "_" + videoID
		if !strings.Contains(filename, idPattern) {
			// 移除扩展名（如果有）
			base := strings.TrimSuffix(filename, filepath.Ext(filename))
			ext := filepath.Ext(filename)
			if ext == "" {
				ext = ".mp4"
			}
			// 添加ID：标题_ID.mp4
			filename = base + "_" + videoID + ext
		}
	}

	return filename
}