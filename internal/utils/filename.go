package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// VideoFilenameMeta 表示生成视频文件名所需的元数据。
type VideoFilenameMeta struct {
	Title      string
	VideoID    string
	Author     string
	Duration   time.Duration
	CreateTime time.Time
	SizeBytes  int64
	SizeText   string
}

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

	// Windows 文件系统会自动去除文件夹名称末尾的点（.）
	// 为了确保创建文件夹和查找路径时使用相同的名称，我们需要手动去除末尾的点
	// 这样可以避免路径不匹配的问题（如 "机器.." 会被 Windows 创建为 "机器"）
	cleaned = strings.TrimRight(cleaned, ".")

	// 如果去除末尾点后为空，使用默认名称
	if strings.TrimSpace(cleaned) == "" {
		cleaned = "未知作者"
	}

	// 文件夹名称也需要限制长度，但可以稍微宽松一些
	// 限制为 50 个字符，避免路径过长
	maxLength := 50
	runes := []rune(cleaned)
	if len(runes) > maxLength {
		cleaned = string(runes[:maxLength]) + "..."
		// 再次去除末尾的点（如果截断后添加的省略号导致末尾有点）
		cleaned = strings.TrimRight(cleaned, ".")
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
// 默认仅使用标题；如果启用 includeVideoID，则追加视频ID。
func GenerateVideoFilename(title, videoID string, includeVideoID bool) string {
	// 清理标题
	var filename string
	if title != "" {
		filename = CleanFilename(title)
	} else if includeVideoID && videoID != "" {
		filename = "video_" + videoID
	} else if videoID != "" {
		filename = "video"
	} else {
		filename = "video"
	}

	// 如果启用，才在文件名中包含ID
	if includeVideoID && videoID != "" {
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

var repeatedSeparatorRegex = regexp.MustCompile(`_+`)

// RenderFilenameTemplate 渲染下载文件名模板。
func RenderFilenameTemplate(meta VideoFilenameMeta, template string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}

	replacements := map[string]string{
		"{date}":     formatTemplateDate(meta.CreateTime),
		"{datetime}": formatTemplateDatetime(meta.CreateTime),
		"{author}":   strings.TrimSpace(meta.Author),
		"{title}":    strings.TrimSpace(meta.Title),
		"{duration}": formatTemplateDuration(meta.Duration),
		"{video_id}": strings.TrimSpace(meta.VideoID),
		"{size}":     formatTemplateSize(meta.SizeBytes, meta.SizeText),
	}

	rendered := template
	for token, value := range replacements {
		rendered = strings.ReplaceAll(rendered, token, value)
	}

	rendered = strings.TrimSpace(rendered)
	rendered = repeatedSeparatorRegex.ReplaceAllString(rendered, "_")
	rendered = strings.Trim(rendered, " _-.")
	if rendered == "" {
		return ""
	}

	return CleanFilename(rendered)
}

// BuildVideoFilename 根据模板或默认规则生成文件名主体。
func BuildVideoFilename(meta VideoFilenameMeta, includeVideoID bool, template string) string {
	if rendered := RenderFilenameTemplate(meta, template); rendered != "" {
		return rendered
	}
	return GenerateVideoFilename(meta.Title, meta.VideoID, includeVideoID)
}

func formatTemplateDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func formatTemplateDatetime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02_15-04-05")
}

func formatTemplateDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}

	totalSeconds := int64(d.Round(time.Second) / time.Second)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func formatTemplateSize(sizeBytes int64, fallback string) string {
	if sizeBytes > 0 {
		return formatHumanFileSize(sizeBytes)
	}
	return strings.TrimSpace(fallback)
}

func formatHumanFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GenerateUniquePath 生成不冲突的完整文件路径。
func GenerateUniquePath(dir, filename string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".mp4"
	}

	candidate := filepath.Join(dir, filename)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}

	for i := 1; i < 1000; i++ {
		next := filepath.Join(dir, fmt.Sprintf("%s(%d)%s", base, i, ext))
		if _, err := os.Stat(next); os.IsNotExist(err) {
			return next
		}
	}

	return filepath.Join(dir, fmt.Sprintf("%s_%s%s", base, time.Now().Format("20060102_150405"), ext))
}
