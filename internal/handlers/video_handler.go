package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wx_channel/internal/database"
	"wx_channel/internal/response"
	"wx_channel/internal/utils"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// HandleProfile 处理视频信息请求
func (h *APIHandler) HandleProfile(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/profile" {
		return false
	}
	utils.LogFileInfo("[Profile API] 收到视频信息请求")

	// 授权与来源校验（可选）
	if h.getConfig() != nil && h.getConfig().SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.getConfig().SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, string(response.ErrorJSON(401, "unauthorized")), headers)
			return true
		}
	}
	if h.getConfig() != nil && len(h.getConfig().AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range h.getConfig().AllowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				headers := http.Header{}
				headers.Set("Content-Type", "application/json")
				headers.Set("X-Content-Type-Options", "nosniff")
				Conn.StopRequest(403, string(response.ErrorJSON(403, "forbidden_origin")), headers)
				return true
			}
		}
	}

	var data map[string]interface{}
	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取profile请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	if len(body) == 0 {
		utils.Warn("profile请求体为空，跳过处理")
		h.sendEmptyResponse(Conn)
		return true
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		utils.HandleError(err, "解析profile JSON数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 处理视频数据
	h.processVideoData(data)

	// 返回空响应
	h.sendEmptyResponse(Conn)
	return true
}

// processVideoData 处理视频数据并保存浏览记录
func (h *APIHandler) processVideoData(data map[string]interface{}) {
	// 记录视频信息到日志文件
	videoID := ""
	if id, ok := data["id"].(string); ok {
		videoID = id
	}
	title := ""
	if t, ok := data["title"].(string); ok {
		title = t
	}
	author := ""
	if n, ok := data["nickname"].(string); ok {
		author = n
	}
	authorID := ""
	if aid, ok := data["authorId"].(string); ok {
		authorID = aid
	}
	sizeMB := 0.0
	var size int64 = 0
	if s, ok := data["size"].(float64); ok {
		sizeMB = s / (1024 * 1024)
		size = int64(s)
	}
	url := ""
	if u, ok := data["url"].(string); ok {
		url = u
	}

	// 提取其他字段用于数据库保存
	var duration int64 = 0
	if d, ok := data["duration"].(float64); ok {
		duration = int64(d)
	}
	coverUrl := ""
	if c, ok := data["coverUrl"].(string); ok {
		coverUrl = c
	}
	var likeCount int64 = 0
	if l, ok := data["likeCount"].(float64); ok {
		likeCount = int64(l)
	}
	var commentCount int64 = 0
	if c, ok := data["commentCount"].(float64); ok {
		commentCount = int64(c)
	}
	var favCount int64 = 0
	if f, ok := data["favCount"].(float64); ok {
		favCount = int64(f)
	}
	var forwardCount int64 = 0
	if fw, ok := data["forwardCount"].(float64); ok {
		forwardCount = int64(fw)
	}
	// 提取解密密钥（用于加密视频下载）
	// decodeKey 可能是字符串或数字类型
	decryptKey := ""
	if k, ok := data["key"].(string); ok {
		decryptKey = k
	} else if k, ok := data["key"].(float64); ok {
		// 数字类型的key，转换为字符串
		decryptKey = fmt.Sprintf("%.0f", k)
	}

	// 提取分辨率信息：优先从media直接获取宽x高格式
	resolution := ""
	fileFormat := "" // 视频格式标识（如 xWT128, xWT111）

	// 前端发送的media是单个对象，不是数组
	if mediaItem, ok := data["media"].(map[string]interface{}); ok {
		// 从media直接获取width和height
		var width, height int64
		if w, ok := mediaItem["width"].(float64); ok {
			width = int64(w)
		}
		if h, ok := mediaItem["height"].(float64); ok {
			height = int64(h)
		}
		if width > 0 && height > 0 {
			resolution = fmt.Sprintf("%dx%d", width, height)
			utils.LogFileInfo("[分辨率] 从media获取: %s", resolution)
		}

		// 从spec中提取fileFormat和分辨率
		if spec, ok := mediaItem["spec"].([]interface{}); ok && len(spec) > 0 {
			// 遍历spec数组，找到最高质量的格式（通常是第一个或最后一个）
			for _, item := range spec {
				if specItem, ok := item.(map[string]interface{}); ok {
					// 提取fileFormat（如 xWT128, xWT111）
					if format, ok := specItem["fileFormat"].(string); ok && format != "" {
						fileFormat = format
						utils.LogFileInfo("[视频格式] 从spec获取: %s", fileFormat)
					}

					// 如果还没有分辨率，从spec中提取
					if resolution == "" {
						if w, ok := specItem["width"].(float64); ok {
							if h, ok := specItem["height"].(float64); ok {
								resolution = fmt.Sprintf("%dx%d", int64(w), int64(h))
								utils.LogFileInfo("[分辨率] 从spec获取: %s", resolution)
							}
						}
					}

					// 如果已经找到fileFormat，优先使用第一个（通常是最高质量）
					if fileFormat != "" {
						break
					}
				}
			}
		}
	}

	if resolution == "" {
		utils.LogFileInfo("[分辨率] 未能获取分辨率信息")
	}
	if fileFormat == "" {
		utils.LogFileInfo("[视频格式] 未能获取格式标识")
	}

	pageUrl := h.currentURL

	utils.LogFileInfo("[视频信息] ID=%s | 标题=%s | 作者=%s | 大小=%.2fMB | URL=%s | 加密=%t | 分辨率=%s",
		videoID, title, author, sizeMB, url, decryptKey != "", resolution)

	// 保存浏览记录到数据库
	metricsSource, _ := data["metricsSource"].(string)
	metricsObservedAt, _ := data["metricsObservedAt"].(string)
	metricsSnapshot := database.CaptureReportedVideoMetrics(data, metricsSource, metricsObservedAt, time.Now())
	h.saveBrowseRecord(videoID, title, author, authorID, duration, size, coverUrl, url, decryptKey, resolution, fileFormat, likeCount, commentCount, favCount, forwardCount, pageUrl, metricsSnapshot)
}

// saveBrowseRecord 保存浏览记录到数据库
func (h *APIHandler) saveBrowseRecord(videoID, title, author, authorID string, duration, size int64, coverUrl, videoUrl, decryptKey, resolution, fileFormat string, likeCount, commentCount, favCount, forwardCount int64, pageUrl, metricsSnapshot string) {
	// 检查数据库是否已初始化
	db := database.GetDB()
	if db == nil {
		utils.Warn("数据库未初始化，无法保存浏览记录")
		return
	}

	// 如果没有视频ID，生成一个
	if videoID == "" {
		videoID = fmt.Sprintf("browse_%d", time.Now().UnixNano())
	}

	// 创建浏览记录
	record := &database.BrowseRecord{
		ID:              videoID,
		Title:           title,
		Author:          author,
		AuthorID:        authorID,
		Duration:        duration,
		Size:            size,
		Resolution:      resolution,
		FileFormat:      fileFormat,
		CoverURL:        coverUrl,
		VideoURL:        videoUrl,
		DecryptKey:      decryptKey,
		BrowseTime:      time.Now(),
		LikeCount:       likeCount,
		CommentCount:    commentCount,
		FavCount:        favCount,
		ForwardCount:    forwardCount,
		PageURL:         pageUrl,
		MetricsSnapshot: metricsSnapshot,
	}

	// 保存到数据库
	repo := database.NewBrowseHistoryRepository()

	// 先检查是否已存在该记录
	existing, err := repo.GetByID(videoID)
	if err != nil {
		utils.Warn("检查浏览记录失败: %v", err)
		return
	}

	if existing != nil {
		// 更新现有记录
		record.CreatedAt = existing.CreatedAt
		// 如果现有记录没有解密密钥但新数据有，则更新
		if existing.DecryptKey == "" && decryptKey != "" {
			record.DecryptKey = decryptKey
		} else if existing.DecryptKey != "" {
			// 保留现有的解密密钥
			record.DecryptKey = existing.DecryptKey
		}
		err = repo.Update(record)
		if err != nil {
			utils.Warn("更新浏览记录失败: %v", err)
		} else {
			utils.LogFileInfo("✓ 浏览记录已更新: %s", title)
		}
	} else {
		// 创建新记录
		err = repo.Create(record)
		if err != nil {
			utils.Warn("保存浏览记录失败: %v", err)
		} else {
			utils.LogFileInfo("✓ 浏览记录已保存: %s", title)
		}
	}
}

// HandleTip 处理前端提示请求
func (h *APIHandler) HandleTip(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/tip" {
		return false
	}

	if h.getConfig() != nil && h.getConfig().SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.getConfig().SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, string(response.ErrorJSON(401, "unauthorized")), headers)
			return true
		}
	}
	if h.getConfig() != nil && len(h.getConfig().AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range h.getConfig().AllowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				headers := http.Header{}
				headers.Set("Content-Type", "application/json")
				headers.Set("X-Content-Type-Options", "nosniff")
				Conn.StopRequest(403, string(response.ErrorJSON(403, "forbidden_origin")), headers)
				return true
			}
		}
	}

	var data struct {
		Msg   string `json:"msg"`
		Level string `json:"level"`
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取tip请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	// 检查body是否为空
	if len(body) == 0 {
		utils.Warn("tip请求体为空，跳过处理")
		h.sendEmptyResponse(Conn)
		return true
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		utils.HandleError(err, "解析tip JSON数据")
		// 即使JSON解析失败，也返回空响应，避免重复处理
		h.sendEmptyResponse(Conn)
		return true
	}

	utils.LogTip(data.Level, data.Msg)

	h.sendEmptyResponse(Conn)
	return true
}

// extractResolutionFromSpec 从media.spec数组中提取分辨率
// spec 是一个包含不同视频格式信息的数组
// 优先查找 xWT111 格式（最高质量），然后提取其分辨率
func extractResolutionFromSpec(spec []interface{}) string {
	var bestResolution string
	var foundXWT111 bool

	for _, item := range spec {
		if specItem, ok := item.(map[string]interface{}); ok {
			fileId, _ := specItem["fileId"].(string)

			// 尝试从fileId解析分辨率
			// fileId 格式示例: "0+xWT111_1280x720_..."
			if fileId != "" {
				// 检查是否包含分辨率信息 (例如 1920x1080)
				resolution := parseResolutionFromFormatString(fileId)
				if resolution != "" {
					// 优先选择 xWT111 格式
					if strings.Contains(fileId, "xWT111") {
						bestResolution = resolution
						foundXWT111 = true
						break // 找到最佳格式，直接返回
					} else if !foundXWT111 {
						// 记录非最佳格式的分辨率，继续寻找
						bestResolution = resolution
					}
				}
			}
		}
	}

	return bestResolution
}

// parseResolutionFromFormatString 从格式字符串中解析分辨率
// 例如: "xWT111_1280x720" -> "720p"
func parseResolutionFromFormatString(formatStr string) string {
	// 匹配 数字x数字 格式
	re := regexp.MustCompile(`(\d{3,4})x(\d{3,4})`)
	matches := re.FindStringSubmatch(formatStr)
	if len(matches) == 3 {
		width, _ := strconv.Atoi(matches[1])
		height, _ := strconv.Atoi(matches[2])
		return fmt.Sprintf("%dx%d", width, height)
	}
	return ""
}

// parseResolutionFromURL 从视频URL中解析分辨率
func parseResolutionFromURL(url string) string {
	// 常见的视频URL中可能包含分辨率信息
	// 这里只是一个简单的实现，针对特定URL模式可以扩展
	return parseResolutionFromFormatString(url)
}

// formatHeightToResolution 将视频高度转换为分辨率字符串
func formatHeightToResolution(height int64) string {
	if height >= 2160 {
		return "4K"
	}
	if height >= 1440 {
		return "2K"
	}
	if height >= 1080 {
		return "1080p"
	}
	if height >= 720 {
		return "720p"
	}
	if height >= 480 {
		return "480p"
	}
	return fmt.Sprintf("%dp", height)
}
