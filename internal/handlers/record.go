package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
	"wx_channel/internal/services"
	"wx_channel/internal/utils"

	"github.com/fatih/color"
	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// RecordHandler 下载记录处理器
type RecordHandler struct {
	downloadService *services.DownloadRecordService
	currentURL      string
}

// NewRecordHandler 创建记录处理器
func NewRecordHandler(cfg *config.Config) *RecordHandler {
	return &RecordHandler{
		downloadService: services.NewDownloadRecordService(),
	}
}

// getConfig 获取当前配置（动态获取最新配置）
func (h *RecordHandler) getConfig() *config.Config {
	return config.Get()
}

// SetCurrentURL 设置当前页面URL
func (h *RecordHandler) SetCurrentURL(url string) {
	h.currentURL = url
}

// GetCurrentURL 获取当前页面URL
func (h *RecordHandler) GetCurrentURL() string {
	return h.currentURL
}

// Handle implements router.Interceptor
func (h *RecordHandler) Handle(Conn *SunnyNet.HttpConn) bool {

	if h.HandleRecordDownload(Conn) {
		return true
	}
	if h.HandleBatchDownloadStatus(Conn) {
		return true
	}
	return false
}

// HandleRecordDownload 处理记录下载信息请求
func (h *RecordHandler) HandleRecordDownload(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/record_download" {
		return false
	}

	if h.getConfig() != nil && h.getConfig().SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.getConfig().SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}

	var data map[string]interface{}
	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取record_download请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	// 检查body是否为空
	if len(body) == 0 {
		utils.Warn("record_download请求体为空，跳过处理")
		h.sendEmptyResponse(Conn)
		return true
	}

	if err := json.Unmarshal(body, &data); err != nil {
		utils.HandleError(err, "记录下载信息")
		h.sendEmptyResponse(Conn)
		return true
	}

	// 映射到数据库模型
	record := &database.DownloadRecord{
		ID:           fmt.Sprintf("%v", data["id"]),
		Title:        fmt.Sprintf("%v", data["title"]),
		Author:       "", // 将在后面从contact中获取
		VideoID:      fmt.Sprintf("%v", data["id"]),
		DownloadTime: time.Now(),
		Status:       database.DownloadStatusCompleted, // 假设调用此接口时下载已完成或仅作为记录
	}

	// 从正确的位置获取作者昵称
	if nickname, ok := data["nickname"].(string); ok && nickname != "" {
		record.Author = nickname
	} else {
		// 从 contact.nickname 获取（Home页）
		if contact, ok := data["contact"].(map[string]interface{}); ok {
			if nickname, ok := contact["nickname"].(string); ok {
				record.Author = nickname
			}
		}
	}

	// 添加可选字段
	if size, ok := data["size"].(float64); ok {
		record.FileSize = int64(size)
	}
	if duration, ok := data["duration"].(float64); ok {
		record.Duration = int64(duration)
	}

	// 尝试解析格式
	record.Format = "unknown"
	if urlStr, ok := data["url"].(string); ok {
		// 简单的格式推断，或者不存URL直接存元数据
		if strings.Contains(urlStr, ".mp4") {
			record.Format = "mp4"
		}
	}
	if browseRecord, browseErr := database.NewBrowseHistoryRepository().GetByID(record.VideoID); browseErr == nil && browseRecord != nil {
		database.MergeBrowseMetricsIntoDownload(record, browseRecord)
	}

	// 保存记录到数据库
	if h.downloadService != nil {
		// 检查重复 (GetByID)
		existing, err := h.downloadService.GetByID(record.ID)
		if err == nil && existing != nil {
			utils.Info("[下载记录] 记录已存在(DB)，跳过保存: ID=%s, 标题=%s", record.ID, record.Title)
			h.sendEmptyResponse(Conn)
			return true
		}

		if err := h.downloadService.Create(record); err != nil {
			utils.Error("[下载记录] DB保存失败: ID=%s, 标题=%s, 错误=%v", record.ID, record.Title, err)
		} else {
			// 格式化大小用于日志显示
			sizeMB := float64(record.FileSize) / (1024 * 1024)
			durationStr := utils.FormatDuration(float64(record.Duration))

			utils.Info("[下载记录] 已保存到DB: ID=%s, 标题=%s, 作者=%s, 大小=%.2f MB, 时长=%s",
				record.ID, record.Title, record.Author, sizeMB, durationStr)

			utils.PrintSeparator()
			color.Green("✅ 下载记录已保存 (数据库)")
			utils.PrintSeparator()
		}
	}

	h.sendEmptyResponse(Conn)
	return true
}

// HandleBatchDownloadStatus 处理批量下载状态查询请求
func (h *RecordHandler) HandleBatchDownloadStatus(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_download_status" {
		return false
	}

	if h.getConfig() != nil && h.getConfig().SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.getConfig().SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}

	var statusData struct {
		Current int    `json:"current"`
		Total   int    `json:"total"`
		Status  string `json:"status"`
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取batch_download_status请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	if err := json.Unmarshal(body, &statusData); err != nil {
		utils.HandleError(err, "解析批量下载状态")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 显示批量下载进度
	if statusData.Total > 0 {
		percentage := float64(statusData.Current) / float64(statusData.Total) * 100
		utils.PrintSeparator()
		color.Blue("📥 批量下载进度")
		utils.PrintSeparator()
		utils.PrintLabelValue("📊", "进度", fmt.Sprintf("%d/%d (%.1f%%)",
			statusData.Current, statusData.Total, percentage))
		utils.PrintLabelValue("🔄", "状态", statusData.Status)
		utils.PrintSeparator()
	}

	h.sendEmptyResponse(Conn)
	return true
}

// inferPageSource 从URL推断页面来源
func (h *RecordHandler) inferPageSource(url string) string {
	if strings.Contains(url, "/pages/feed") {
		return "feed"
	} else if strings.Contains(url, "/pages/home") {
		return "home"
	} else if strings.Contains(url, "/pages/profile") {
		return "profile"
	}
	return "unknown"
}

// sendEmptyResponse 发送空JSON响应
func (h *RecordHandler) sendEmptyResponse(Conn *SunnyNet.HttpConn) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Content-Type-Options", "nosniff")
	if h.getConfig() != nil && len(h.getConfig().AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			for _, o := range h.getConfig().AllowedOrigins {
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
	Conn.StopRequest(200, "{}", headers)
}

// sendErrorResponse 发送错误响应
func (h *RecordHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Content-Type-Options", "nosniff")
	if h.getConfig() != nil && len(h.getConfig().AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			for _, o := range h.getConfig().AllowedOrigins {
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
