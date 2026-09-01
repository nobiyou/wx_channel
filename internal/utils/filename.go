package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	// MaxDownloadPathLengthUTF16 keeps downloaded files below the conservative
	// Windows path budget while leaving room for filesystem-specific overhead.
	MaxDownloadPathLengthUTF16 = 240
	// MaxDownloadFilenameBodyLengthUTF16 is the maximum title portion used when
	// the target directory is short enough to allow it.
	MaxDownloadFilenameBodyLengthUTF16 = 180
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

var (
	htmlTagRegex    = regexp.MustCompile(`<[^>]*>`)
	htmlEntityRegex = regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
)

// cleanFilename removes content that is invalid in a Windows filename without
// applying a length limit. Length is a property of the final target path, not
// of an isolated title.
func cleanFilename(filename string) string {
	// 先移除HTML标签（如 <em class="highlight">纪录片</em>）
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

	// Windows 不允许文件名以空格或点结尾。
	filename = strings.TrimRight(strings.TrimSpace(filename), " .")

	// 如果文件名为空，使用默认名称
	if filename == "" {
		filename = "video_" + time.Now().Format("20060102_150405")
	}

	return filename
}

// CleanFilename 清理文件名，移除非法字符。
//
// 该函数保留旧的 50 个字符兼容行为。下载视频应使用
// CleanFilenameForDownload，并在最终目标目录上调用 FitFilenameToDirectory。
func CleanFilename(filename string) string {
	return truncateRunes(cleanFilename(filename), 50)
}

// CleanFilenameForDownload 清理下载文件名但不提前截断标题。
func CleanFilenameForDownload(filename string) string {
	return cleanFilename(filename)
}

// UTF16Length 返回 Windows 文件系统使用的 UTF-16 code unit 数量。
func UTF16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}

// TruncateUTF16 按 UTF-16 code unit 截断字符串，且不会切断代理项。
func TruncateUTF16(value string, maxUnits int) string {
	if maxUnits <= 0 {
		return ""
	}
	if UTF16Length(value) <= maxUnits {
		return value
	}

	var builder strings.Builder
	used := 0
	for _, r := range value {
		units := 1
		if r > 0xFFFF {
			units = 2
		}
		if used+units > maxUnits {
			break
		}
		builder.WriteRune(r)
		used += units
	}
	return builder.String()
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

// CleanFolderName 清理文件夹名称
func CleanFolderName(folderName string) string {
	// 先检查是否为空，避免 CleanFilename 生成时间戳名称
	if strings.TrimSpace(folderName) == "" {
		return "未知作者"
	}

	cleaned := CleanFilenameForDownload(folderName)

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

	// 作者目录保留较小的固定预算，文件名预算会再根据完整目录动态计算。
	cleaned = TruncateUTF16(cleaned, 50)
	cleaned = strings.TrimRight(cleaned, " .")

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
	filename = FitFilenameToDirectory(dir, filename, "")
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	ext := filepath.Ext(filename)

	for i := 1; i < maxAttempts; i++ {
		candidate := filepath.Join(dir, filename)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			// 文件不存在，可以使用
			return candidate
		}

		// 文件存在，尝试添加序号
		marker := fmt.Sprintf("(%d)", i)
		filename = FitFilenameToDirectory(dir, buildUniqueFilename(base, ext, "", marker), marker)
	}

	// 如果所有尝试都失败，添加时间戳
	timestamp := time.Now().Format("20060102_150405")
	marker := "_" + timestamp
	return filepath.Join(dir, FitFilenameToDirectory(dir, buildUniqueFilename(base, ext, "", marker), marker))
}

// GenerateVideoFilename 根据视频标题和ID生成文件名
// 默认仅使用标题；如果启用 includeVideoID，则追加视频ID。
func GenerateVideoFilename(title, videoID string, includeVideoID bool) string {
	// 清理标题
	var filename string
	if title != "" {
		filename = CleanFilenameForDownload(title)
	} else if includeVideoID && videoID != "" {
		filename = "video_" + CleanFilenameForDownload(videoID)
	} else if videoID != "" {
		filename = "video"
	} else {
		filename = "video"
	}

	// 如果启用，才在文件名中包含ID
	if includeVideoID && videoID != "" {
		videoID = CleanFilenameForDownload(videoID)
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

	return CleanFilenameForDownload(rendered)
}

// BuildVideoFilename 根据模板或默认规则生成文件名主体。
func BuildVideoFilename(meta VideoFilenameMeta, includeVideoID bool, template string) string {
	if rendered := RenderFilenameTemplate(meta, template); rendered != "" {
		return rendered
	}
	return GenerateVideoFilename(meta.Title, meta.VideoID, includeVideoID)
}

// VideoFilenameRequiredSuffix 返回默认视频命名中应始终保留的 ID 及其后缀。
// 模板由用户完全控制，因此这里只保护默认命名追加的 videoID 尾段。
func VideoFilenameRequiredSuffix(filename, videoID string) string {
	if strings.TrimSpace(videoID) == "" {
		return ""
	}

	safeID := CleanFilenameForDownload(videoID)
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	marker := "_" + safeID
	if index := strings.LastIndex(base, marker); index >= 0 {
		return base[index:]
	}
	return ""
}

func downloadPathPrefixLength(dir string) int {
	marker := filepath.Join(dir, "x")
	return UTF16Length(marker) - UTF16Length("x")
}

// FitFilenameToDirectory 返回适合 dir 的文件名。
// requiredSuffix 会被放在标题之后并始终保留，例如 _videoID_1080p。
func FitFilenameToDirectory(dir, filename, requiredSuffix string) string {
	cleaned := CleanFilenameForDownload(filename)
	ext := filepath.Ext(cleaned)
	base := strings.TrimSuffix(cleaned, ext)
	if base == "" {
		base = "video"
	}

	requiredSuffix = strings.TrimSpace(requiredSuffix)
	if requiredSuffix != "" && !strings.HasSuffix(base, requiredSuffix) {
		requiredSuffix = ""
	}

	title := base
	if requiredSuffix != "" {
		title = strings.TrimSuffix(base, requiredSuffix)
	}
	title = strings.TrimRight(title, " .")

	available := MaxDownloadPathLengthUTF16 - downloadPathPrefixLength(dir) - UTF16Length(ext)
	titleBudget := MaxDownloadFilenameBodyLengthUTF16
	if available-UTF16Length(requiredSuffix) < titleBudget {
		titleBudget = available - UTF16Length(requiredSuffix)
	}
	if titleBudget < 0 {
		titleBudget = 0
	}

	title = strings.TrimRight(TruncateUTF16(title, titleBudget), " .")
	if title == "" && requiredSuffix == "" && titleBudget >= UTF16Length("video") {
		title = "video"
	}

	return title + requiredSuffix + ext
}

// BuildDownloadFilePath builds a path whose filename observes the download
// path budget. It intentionally returns a path rather than an error because
// an overlong configured directory cannot be repaired by renaming the file.
func BuildDownloadFilePath(dir, filename, requiredSuffix string) string {
	return filepath.Join(dir, FitFilenameToDirectory(dir, filename, requiredSuffix))
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
	return GenerateUniquePathWithSuffix(dir, filename, "")
}

// GenerateUniquePathWithSuffix 生成不冲突的完整文件路径，并在重名时保留
// requiredSuffix（例如视频 ID 和画质后缀）。
func GenerateUniquePathWithSuffix(dir, filename, requiredSuffix string) string {
	filename = FitFilenameToDirectory(dir, filename, requiredSuffix)
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
		marker := fmt.Sprintf("(%d)", i)
		nextName := buildUniqueFilename(base, ext, requiredSuffix, marker)
		nextName = FitFilenameToDirectory(dir, nextName, marker+requiredSuffix)
		next := filepath.Join(dir, nextName)
		if _, err := os.Stat(next); os.IsNotExist(err) {
			return next
		}
	}

	marker := "_" + time.Now().Format("20060102_150405")
	timestampName := buildUniqueFilename(base, ext, requiredSuffix, marker)
	return filepath.Join(dir, FitFilenameToDirectory(dir, timestampName, marker+requiredSuffix))
}

func buildUniqueFilename(base, ext, requiredSuffix, marker string) string {
	if requiredSuffix != "" && strings.HasSuffix(base, requiredSuffix) {
		prefix := strings.TrimSuffix(base, requiredSuffix)
		return prefix + marker + requiredSuffix + ext
	}
	return base + marker + ext
}
