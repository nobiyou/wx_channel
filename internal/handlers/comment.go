package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/utils"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// CommentHandler 评论数据处理器
type CommentHandler struct {
	config *config.Config
}

// NewCommentHandler 创建评论处理器
func NewCommentHandler(cfg *config.Config) *CommentHandler {
	return &CommentHandler{
		config: cfg,
	}
}

// HandleSaveCommentData 处理保存评论数据请求
func (h *CommentHandler) HandleSaveCommentData(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/save_comment_data" {
		return false
	}

	// 授权校验
	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			// 记录认证失败
			clientIP := Conn.Request.RemoteAddr
			utils.LogAuthFailed(path, clientIP)
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}

	// CORS校验
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				// 记录CORS拦截
				utils.LogCORSBlocked(origin, path)
				headers := http.Header{}
				headers.Set("Content-Type", "application/json")
				headers.Set("X-Content-Type-Options", "nosniff")
				Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
				return true
			}
		}
	}

	var requestData struct {
		Comments             []map[string]interface{} `json:"comments"`
		VideoID              string                   `json:"videoId"`
		VideoTitle           string                   `json:"videoTitle"`
		OriginalCommentCount int                      `json:"originalCommentCount"`
		Timestamp            int64                    `json:"timestamp"`
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取save_comment_data请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	// 检查body是否为空
	if len(body) == 0 {
		utils.Warn("save_comment_data请求体为空，跳过处理")
		h.sendEmptyResponse(Conn)
		return true
	}

	if err := json.Unmarshal(body, &requestData); err != nil {
		utils.HandleError(err, "解析评论数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 保存评论数据
	if err := h.saveCommentData(requestData.Comments, requestData.VideoID, requestData.VideoTitle, requestData.OriginalCommentCount, requestData.Timestamp); err != nil {
		utils.HandleError(err, "保存评论数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	h.sendEmptyResponse(Conn)
	return true
}

// saveCommentData 保存评论数据到文件
func (h *CommentHandler) saveCommentData(comments []map[string]interface{}, videoID, videoTitle string, originalCommentCount int, timestamp int64) error {
	if len(comments) == 0 {
		return nil
	}

	// 获取基础目录
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		return fmt.Errorf("获取基础目录失败: %v", err)
	}

	// 创建评论数据目录
	downloadsDir := filepath.Join(baseDir, h.config.DownloadsDir)
	commentDataRoot := filepath.Join(downloadsDir, "comment_data")
	if err := utils.EnsureDir(commentDataRoot); err != nil {
		return fmt.Errorf("创建评论数据根目录失败: %v", err)
	}

	// 按日期组织目录
	saveTime := time.Now()
	if timestamp > 0 {
		saveTime = time.Unix(0, timestamp*int64(time.Millisecond))
	}

	dateDir := filepath.Join(commentDataRoot, saveTime.Format("2006-01-02"))
	if err := utils.EnsureDir(dateDir); err != nil {
		return fmt.Errorf("创建评论数据日期目录失败: %v", err)
	}

	// 构建文件名
	var fileName string
	if videoTitle != "" {
		// 清理标题作为文件名
		cleanTitle := utils.CleanFilename(videoTitle)
		// CleanFilename 已经处理了长度限制（100字符），这里不需要再次限制
		fileName = fmt.Sprintf("%s_%s_%s.json", 
			saveTime.Format("150405"), 
			videoID, 
			cleanTitle)
	} else {
		fileName = fmt.Sprintf("%s_%s_video_%s.json", 
			saveTime.Format("150405"), 
			videoID,
			saveTime.Format("20060102_150405"))
	}

	targetPath := utils.GenerateUniqueFilename(dateDir, fileName, 100)

	// 计算实际总评论数（一级 + 二级）
	totalComments := len(comments)
	for _, comment := range comments {
		if levelTwo, ok := comment["levelTwoComment"].([]interface{}); ok {
			totalComments += len(levelTwo)
		}
	}

	// 构建数据结构
	commentData := map[string]interface{}{
		"videoId":              videoID,
		"videoTitle":           videoTitle,
		"comments":             comments,
		"commentCount":         totalComments,
		"originalCommentCount": originalCommentCount,
		"saved_at":             saveTime.Format(time.RFC3339),
		"timestamp":            timestamp,
	}

	// 保存JSON数据
	dataBytes, err := json.MarshalIndent(commentData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化评论数据失败: %v", err)
	}

	if err := os.WriteFile(targetPath, dataBytes, 0644); err != nil {
		return fmt.Errorf("保存评论数据文件失败: %v", err)
	}

	relativePath, err := filepath.Rel(downloadsDir, targetPath)
	if err != nil {
		relativePath = targetPath
	}

	if originalCommentCount > 0 {
		utils.Info("评论数据已保存: %s (%d/%d条评论) -> %s", videoTitle, totalComments, originalCommentCount, relativePath)
		utils.LogInfo("[评论保存] 标题=%s | 采集=%d | 原始=%d | 路径=%s", videoTitle, totalComments, originalCommentCount, relativePath)
	} else {
		utils.Info("评论数据已保存: %s (%d条评论) -> %s", videoTitle, totalComments, relativePath)
		utils.LogInfo("[评论保存] 标题=%s | 采集=%d | 路径=%s", videoTitle, totalComments, relativePath)
	}
	
	// 记录详细评论采集日志
	utils.LogComment(videoID, videoTitle, totalComments, true)

	return nil
}

// sendEmptyResponse 发送空JSON响应
func (h *CommentHandler) sendEmptyResponse(Conn *SunnyNet.HttpConn) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Content-Type-Options", "nosniff")

	// CORS
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					headers.Set("Access-Control-Allow-Origin", origin)
					headers.Set("Vary", "Origin")
					headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
					headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
					break
				}
			}
		}
	}

	headers.Set("__debug", "fake_resp")
	Conn.StopRequest(200, `{"success":true}`, headers)
}

// sendErrorResponse 发送错误响应
func (h *CommentHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Content-Type-Options", "nosniff")

	// CORS
	if h.config != nil && len(h.config.AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			for _, o := range h.config.AllowedOrigins {
				if o == origin {
					headers.Set("Access-Control-Allow-Origin", origin)
					headers.Set("Vary", "Origin")
					headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
					headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
					break
				}
			}
		}
	}

	errorMsg := fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error())
	Conn.StopRequest(500, errorMsg, headers)
}
