package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
	"wx_channel/internal/services"
	"wx_channel/internal/utils"
	"wx_channel/internal/websocket"
)

// ConsoleAPIHandler 处理 Web 控制台的 REST API 请求
type ConsoleAPIHandler struct {
	cfg             *config.Config
	browseService   *services.BrowseHistoryService
	downloadService *services.DownloadRecordService
	queueService    *services.QueueService
	settingsRepo    *database.SettingsRepository
	statsService    *services.StatisticsService
	exportService   *services.ExportService
	searchService   *services.SearchService
	wsHub           *websocket.Hub
	radarService    *services.RadarService
}

const maxJSONBodyBytes = 8 << 20 // 8MB

// NewConsoleAPIHandler 创建一个新的 ConsoleAPIHandler
func NewConsoleAPIHandler(cfg *config.Config, wsHub *websocket.Hub, radarService *services.RadarService) *ConsoleAPIHandler {
	return &ConsoleAPIHandler{
		cfg:             cfg,
		browseService:   services.NewBrowseHistoryService(),
		downloadService: services.NewDownloadRecordService(),
		queueService:    services.NewQueueService(),
		settingsRepo:    database.NewSettingsRepository(),
		statsService:    services.NewStatisticsService(),
		exportService:   services.NewExportService(),
		searchService:   services.NewSearchService(),
		wsHub:           wsHub,
		radarService:    radarService,
	}
}

// getConfig 获取当前配置（动态获取最新配置）
func (h *ConsoleAPIHandler) getConfig() *config.Config {
	if h.cfg != nil {
		return h.cfg
	}
	return config.Get()
}

func (h *ConsoleAPIHandler) applyConfigOwnedSettings(settings *database.Settings) *database.Settings {
	if settings == nil {
		settings = database.DefaultSettings()
	}

	cfg := h.getConfig()
	if cfg == nil {
		return settings
	}

	copied := *settings
	copied.RadarEnabled = cfg.RadarEnabled
	copied.DownloadFilenameTemplate = cfg.DownloadFilenameTemplate
	copied.SharedFeedBackendEnabled, copied.SharedFeedBackendType = resolveSharedFeedBackendStatus(cfg)
	return &copied
}

func resolveSharedFeedBackendStatus(cfg *config.Config) (bool, string) {
	if cfg == nil {
		return false, "none"
	}

	if strings.TrimSpace(cfg.Cloudflare.SphHostname) != "" {
		return true, "worker"
	}
	if strings.TrimSpace(cfg.Cloudflare.SphCookie) != "" {
		return true, "cookie"
	}
	return false, "none"
}

// APIResponse 表示标准 API 响应
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type pathValidationError struct {
	status int
	msg    string
}

func (e *pathValidationError) Error() string {
	return e.msg
}

// sendJSON 发送 JSON 响应
func (h *ConsoleAPIHandler) sendJSON(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	h.setCORSHeaders(w, r)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// sendSuccess 发送成功响应
func (h *ConsoleAPIHandler) sendSuccess(w http.ResponseWriter, r *http.Request, data interface{}) {
	h.sendJSON(w, r, http.StatusOK, APIResponse{Success: true, Data: data})
}

// sendSuccessMessage 发送带有消息的成功响应
func (h *ConsoleAPIHandler) sendSuccessMessage(w http.ResponseWriter, r *http.Request, message string) {
	h.sendJSON(w, r, http.StatusOK, APIResponse{Success: true, Message: message})
}

// sendError 发送错误响应
func (h *ConsoleAPIHandler) sendError(w http.ResponseWriter, r *http.Request, status int, message string) {
	h.sendJSON(w, r, status, APIResponse{Success: false, Error: message})
}

// setCORSHeaders 设置响应的 CORS 头
// Requirements: 14.6 - 为远程控制台包含 CORS 头
func (h *ConsoleAPIHandler) setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		// 允许本地开发的所有来源，或对照允许的来源列表进行检查
		if h.getConfig() != nil && len(h.getConfig().AllowedOrigins) > 0 {
			for _, o := range h.getConfig().AllowedOrigins {
				if o == origin || o == "*" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		} else {
			// 默认：允许本地服务的所有来源
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth, Authorization")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// HandleCORS 处理 CORS 预检请求
func (h *ConsoleAPIHandler) HandleCORS(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == "OPTIONS" {
		h.setCORSHeaders(w, r)
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// parseJSON 解析 JSON 请求体
func (h *ConsoleAPIHandler) parseJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodyBytes+1))
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if int64(len(body)) > maxJSONBodyBytes {
		return fmt.Errorf("request body too large")
	}
	return json.Unmarshal(body, v)
}

// getPaginationParams 从查询字符串中提取分页参数
func getPaginationParams(r *http.Request) *database.PaginationParams {
	params := &database.PaginationParams{
		Page:     1,
		PageSize: 20,
		SortBy:   "browse_time",
		SortDesc: true,
	}

	if page := r.URL.Query().Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			params.Page = p
		}
	}
	if pageSize := r.URL.Query().Get("pageSize"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 100 {
			params.PageSize = ps
		}
	}
	if sortBy := r.URL.Query().Get("sortBy"); sortBy != "" {
		params.SortBy = sortBy
	}
	if sortDesc := r.URL.Query().Get("sortDesc"); sortDesc != "" {
		params.SortDesc = sortDesc == "true" || sortDesc == "1"
	}

	return params
}

// getFilterParams 从查询字符串中提取过滤参数
func getFilterParams(r *http.Request) *database.FilterParams {
	params := &database.FilterParams{
		PaginationParams: *getPaginationParams(r),
	}
	params.SortBy = "download_time"

	if startDate := r.URL.Query().Get("startDate"); startDate != "" {
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			params.StartDate = &t
		}
	}
	if endDate := r.URL.Query().Get("endDate"); endDate != "" {
		if t, err := time.Parse("2006-01-02", endDate); err == nil {
			// 设置为当天的结束时间
			t = t.Add(24*time.Hour - time.Second)
			params.EndDate = &t
		}
	}
	if status := r.URL.Query().Get("status"); status != "" {
		params.Status = status
	}
	if query := r.URL.Query().Get("query"); query != "" {
		params.Query = query
	}

	return params
}

// extractIDFromPath 从 URL 路径中提取 ID，例如 /api/browse/123
func extractIDFromPath(path, prefix string) string {
	path = strings.TrimPrefix(path, prefix)
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// ============================================================================
// 浏览历史 API 处理器
// Requirements: 14.1 - 浏览历史 CRUD 操作的 REST API 端点
// ============================================================================

// HandleBrowseList 处理 GET /api/browse - 分页列表
func (h *ConsoleAPIHandler) HandleBrowseList(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	params := getPaginationParams(r)
	query := r.URL.Query().Get("query")

	var result *database.PagedResult[database.BrowseRecord]
	var err error

	if query != "" {
		result, err = h.browseService.Search(query, params)
	} else {
		result, err = h.browseService.List(params)
	}

	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, result)
}

// HandleBrowseGet 处理 GET /api/browse/:id - 单条记录
func (h *ConsoleAPIHandler) HandleBrowseGet(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	record, err := h.browseService.GetByID(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if record == nil {
		h.sendError(w, r, http.StatusNotFound, "record not found")
		return
	}

	h.sendSuccess(w, r, record)
}

// HandleBrowseDelete 处理 DELETE /api/browse/:id - 删除单条记录
func (h *ConsoleAPIHandler) HandleBrowseDelete(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.browseService.Delete(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "record deleted")
}

// HandleBrowseDeleteMany 处理 DELETE /api/browse - 批量删除
func (h *ConsoleAPIHandler) HandleBrowseDeleteMany(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	var req struct {
		IDs []string `json:"ids"`
	}
	if err := h.parseJSON(r, &req); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.IDs) == 0 {
		h.sendError(w, r, http.StatusBadRequest, "no IDs provided")
		return
	}

	count, err := h.browseService.DeleteMany(req.IDs)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, map[string]interface{}{
		"deleted": count,
	})
}

// HandleBrowseClear 处理 DELETE /api/browse/clear - 清空所有记录
func (h *ConsoleAPIHandler) HandleBrowseClear(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.browseService.Clear()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "all browse records cleared")
}

// HandleBrowseAPI 路由浏览历史 API 请求
func (h *ConsoleAPIHandler) HandleBrowseAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 处理 CORS 预检请求
	if h.HandleCORS(w, r) {
		return
	}

	// DELETE /api/browse/clear - 必须在提取 ID 之前检查
	if path == "/api/browse/clear" && r.Method == "DELETE" {
		h.HandleBrowseClear(w, r)
		return
	}

	// 从路径提取 ID
	id := extractIDFromPath(path, "/api/browse")

	switch r.Method {
	case "GET":
		if id != "" {
			h.HandleBrowseGet(w, r, id)
		} else {
			h.HandleBrowseList(w, r)
		}
	case "DELETE":
		if id != "" {
			h.HandleBrowseDelete(w, r, id)
		} else {
			h.HandleBrowseDeleteMany(w, r)
		}
	default:
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ============================================================================
// 下载记录 API 处理器
// Requirements: 14.2 - 下载记录 CRUD 操作的 REST API 端点
// ============================================================================

// HandleDownloadsList 处理 GET /api/downloads - 带过滤的分页列表
func (h *ConsoleAPIHandler) HandleDownloadsList(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	params := getFilterParams(r)
	result, err := h.downloadService.List(params)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, result)
}

// HandleDownloadsGet 处理 GET /api/downloads/:id - 单条记录
func (h *ConsoleAPIHandler) HandleDownloadsGet(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	record, err := h.downloadService.GetByID(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if record == nil {
		h.sendError(w, r, http.StatusNotFound, "record not found")
		return
	}

	h.sendSuccess(w, r, record)
}

// HandleDownloadsDelete 处理 DELETE /api/downloads/:id - 删除单条记录
func (h *ConsoleAPIHandler) HandleDownloadsDelete(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	// 检查是否应删除文件
	deleteFiles := r.URL.Query().Get("deleteFiles") == "true"

	err := h.downloadService.Delete(id, deleteFiles)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "record deleted")
}

// HandleDownloadsDeleteMany 处理 DELETE /api/downloads - 批量删除
func (h *ConsoleAPIHandler) HandleDownloadsDeleteMany(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	var req struct {
		IDs         []string `json:"ids"`
		DeleteFiles bool     `json:"deleteFiles"`
	}
	if err := h.parseJSON(r, &req); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.IDs) == 0 {
		h.sendError(w, r, http.StatusBadRequest, "no IDs provided")
		return
	}

	count, err := h.downloadService.DeleteMany(req.IDs, req.DeleteFiles)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, map[string]interface{}{
		"deleted": count,
	})
}

// HandleDownloadsAPI 路由下载记录 API 请求
func (h *ConsoleAPIHandler) HandleDownloadsAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 处理 CORS 预检请求
	if h.HandleCORS(w, r) {
		return
	}

	// 从路径提取 ID
	id := extractIDFromPath(path, "/api/downloads")

	switch r.Method {
	case "GET":
		if id != "" {
			h.HandleDownloadsGet(w, r, id)
		} else {
			h.HandleDownloadsList(w, r)
		}
	case "DELETE":
		if id != "" {
			h.HandleDownloadsDelete(w, r, id)
		} else {
			h.HandleDownloadsDeleteMany(w, r)
		}
	default:
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ============================================================================
// 下载队列 API 处理器
// Requirements: 14.3 - 下载队列管理的 REST API 端点
// ============================================================================

// HandleQueueList 处理 GET /api/queue - 列出队列项目
func (h *ConsoleAPIHandler) HandleQueueList(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	items, err := h.queueService.GetQueue()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, items)
}

// HandleQueueAdd 处理 POST /api/queue - 添加项目到队列
func (h *ConsoleAPIHandler) HandleQueueAdd(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	var req struct {
		Videos []services.VideoInfo `json:"videos"`
	}
	if err := h.parseJSON(r, &req); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Videos) == 0 {
		h.sendError(w, r, http.StatusBadRequest, "no videos provided")
		return
	}

	items, err := h.queueService.AddToQueue(req.Videos)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// 通过 WebSocket 广播队列变更
	// Requirements: 14.5 - 广播队列变更
	hub := GetWebSocketHub()
	for i := range items {
		hub.BroadcastQueueAdd(&items[i])
	}

	h.sendSuccess(w, r, items)
}

// HandleQueuePause 处理 PUT /api/queue/:id/pause - 暂停下载
func (h *ConsoleAPIHandler) HandleQueuePause(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.queueService.Pause(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// 通过 WebSocket 广播队列更新
	item, _ := h.queueService.GetByID(id)
	if item != nil {
		GetWebSocketHub().BroadcastQueueUpdate(item)
	}

	h.sendSuccessMessage(w, r, "download paused")
}

// HandleQueueResume 处理 PUT /api/queue/:id/resume - 恢复下载
func (h *ConsoleAPIHandler) HandleQueueResume(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.queueService.Resume(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// 通过 WebSocket 广播队列更新
	item, _ := h.queueService.GetByID(id)
	if item != nil {
		GetWebSocketHub().BroadcastQueueUpdate(item)
	}

	h.sendSuccessMessage(w, r, "download resumed")
}

// HandleQueueRemove 处理 DELETE /api/queue/:id - 从队列移除
func (h *ConsoleAPIHandler) HandleQueueRemove(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.queueService.RemoveFromQueue(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// 通过 WebSocket 广播队列移除
	GetWebSocketHub().BroadcastQueueRemove(id)

	h.sendSuccessMessage(w, r, "item removed from queue")
}

// HandleQueueReorder 处理 PUT /api/queue/reorder - 重新排序队列
func (h *ConsoleAPIHandler) HandleQueueReorder(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	var req struct {
		IDs []string `json:"ids"`
	}
	if err := h.parseJSON(r, &req); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.IDs) == 0 {
		h.sendError(w, r, http.StatusBadRequest, "no IDs provided")
		return
	}

	err := h.queueService.Reorder(req.IDs)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// 通过 WebSocket 广播队列重新排序
	queue, _ := h.queueService.GetQueue()
	GetWebSocketHub().BroadcastQueueReorder(queue)

	h.sendSuccessMessage(w, r, "queue reordered")
}

// HandleQueueComplete 处理 PUT /api/queue/:id/complete - 标记下载为完成
func (h *ConsoleAPIHandler) HandleQueueComplete(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.queueService.CompleteDownload(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// 获取更新后的项目以进行 WebSocket 广播
	item, _ := h.queueService.GetByID(id)
	if item != nil {
		GetWebSocketHub().BroadcastQueueUpdate(item)
	}

	h.sendSuccessMessage(w, r, "download completed")
}

// HandleQueueFail 处理 PUT /api/queue/:id/fail - 标记下载为失败
func (h *ConsoleAPIHandler) HandleQueueFail(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	var req struct {
		Error string `json:"error"`
	}
	if err := h.parseJSON(r, &req); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	errorMsg := req.Error
	if errorMsg == "" {
		errorMsg = "下载失败"
	}

	err := h.queueService.FailDownload(id, errorMsg)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Get updated item for WebSocket broadcast
	item, _ := h.queueService.GetByID(id)
	if item != nil {
		GetWebSocketHub().BroadcastQueueUpdate(item)
	}

	h.sendSuccessMessage(w, r, "download marked as failed")
}

// HandleQueueAPI 路由队列 API 请求
func (h *ConsoleAPIHandler) HandleQueueAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	// 处理重排序端点
	if path == "/api/queue/reorder" && r.Method == "PUT" {
		h.HandleQueueReorder(w, r)
		return
	}

	// 从路径提取 ID 和操作
	// 路径格式: /api/queue/:id 或 /api/queue/:id/pause 或 /api/queue/:id/resume
	pathParts := strings.Split(strings.TrimPrefix(path, "/api/queue/"), "/")
	id := ""
	action := ""
	if len(pathParts) > 0 && pathParts[0] != "" {
		id = pathParts[0]
	}
	if len(pathParts) > 1 {
		action = pathParts[1]
	}

	switch r.Method {
	case "GET":
		h.HandleQueueList(w, r)
	case "POST":
		h.HandleQueueAdd(w, r)
	case "PUT":
		if id == "" {
			h.sendError(w, r, http.StatusBadRequest, "ID required")
			return
		}
		switch action {
		case "pause":
			h.HandleQueuePause(w, r, id)
		case "resume":
			h.HandleQueueResume(w, r, id)
		case "complete":
			h.HandleQueueComplete(w, r, id)
		case "fail":
			h.HandleQueueFail(w, r, id)
		default:
			h.sendError(w, r, http.StatusBadRequest, "invalid action")
		}
	case "DELETE":
		if id == "" {
			h.sendError(w, r, http.StatusBadRequest, "ID required")
			return
		}
		h.HandleQueueRemove(w, r, id)
	default:
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ============================================================================
// 设置 API 处理器
// Requirements: 14.4 - 设置管理的 REST API 端点
// ============================================================================

// HandleSettingsGet 处理 GET /api/settings - 获取设置
func (h *ConsoleAPIHandler) HandleSettingsGet(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	settings, err := h.settingsRepo.Load()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, h.applyConfigOwnedSettings(settings))
}

// HandleSettingsUpdate 处理 PUT /api/settings - 更新设置
func (h *ConsoleAPIHandler) HandleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	var settings database.Settings
	if err := h.parseJSON(r, &settings); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	settings.RadarEnabled = h.applyConfigOwnedSettings(&settings).RadarEnabled

	// 验证并保存设置
	// Requirements: 11.3, 11.4 - 验证分片大小 (1-100MB) 和并发限制 (1-5)
	if err := h.settingsRepo.SaveAndValidate(&settings); err != nil {
		h.sendError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "settings updated")
}

// HandleSettingsAPI 路由设置 API 请求
func (h *ConsoleAPIHandler) HandleSettingsAPI(w http.ResponseWriter, r *http.Request) {
	// 处理 CORS 预检请求
	if h.HandleCORS(w, r) {
		return
	}

	switch r.Method {
	case "GET":
		h.HandleSettingsGet(w, r)
	case "PUT":
		h.HandleSettingsUpdate(w, r)
	default:
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ============================================================================
// 统计 API 处理器
// Requirements: 7.1, 7.2 - 统计和图表数据端点
// ============================================================================

// HandleStatsGet 处理 GET /api/stats - 获取统计信息
func (h *ConsoleAPIHandler) HandleStatsGet(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	stats, err := h.statsService.GetStatistics()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, stats)
}

// HandleStatsChart 处理 GET /api/stats/chart - 获取图表数据
func (h *ConsoleAPIHandler) HandleStatsChart(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	// 默认为 7 天
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 30 {
			days = parsed
		}
	}

	chartData, err := h.statsService.GetChartData(days)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, chartData)
}

// HandleStatsAPI 路由统计 API 请求
func (h *ConsoleAPIHandler) HandleStatsAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "GET" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if path == "/api/stats/chart" {
		h.HandleStatsChart(w, r)
	} else {
		h.HandleStatsGet(w, r)
	}
}

// ============================================================================
// 导出 API 处理器
// Requirements: 4.1, 4.2 - 导出浏览和下载记录
// ============================================================================

// HandleExportBrowse 处理 GET /api/export/browse - 导出浏览记录
func (h *ConsoleAPIHandler) HandleExportBrowse(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	// 获取格式 (默认: json)
	format := services.ExportFormatJSON
	if f := r.URL.Query().Get("format"); f == "csv" {
		format = services.ExportFormatCSV
	}

	// 获取可选 ID 用于选择性导出
	var ids []string
	if idsParam := r.URL.Query().Get("ids"); idsParam != "" {
		ids = strings.Split(idsParam, ",")
	}

	result, err := h.exportService.ExportBrowseHistory(format, ids)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// 设置文件下载头
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+result.Filename+"\"")
	h.setCORSHeaders(w, r)
	w.WriteHeader(http.StatusOK)
	w.Write(result.Data)
}

// HandleExportDownloads 处理 GET /api/export/downloads - 导出下载记录
func (h *ConsoleAPIHandler) HandleExportDownloads(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	// Get format (default: json)
	format := services.ExportFormatJSON
	if f := r.URL.Query().Get("format"); f == "csv" {
		format = services.ExportFormatCSV
	}

	// Get optional IDs for selective export
	var ids []string
	if idsParam := r.URL.Query().Get("ids"); idsParam != "" {
		ids = strings.Split(idsParam, ",")
	}

	result, err := h.exportService.ExportDownloadRecords(format, ids)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+result.Filename+"\"")
	h.setCORSHeaders(w, r)
	w.WriteHeader(http.StatusOK)
	w.Write(result.Data)
}

// HandleExportAPI 路由导出 API 请求
func (h *ConsoleAPIHandler) HandleExportAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "GET" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch path {
	case "/api/export/browse":
		h.HandleExportBrowse(w, r)
	case "/api/export/downloads":
		h.HandleExportDownloads(w, r)
	default:
		h.sendError(w, r, http.StatusNotFound, "endpoint not found")
	}
}

// ============================================================================
// 搜索 API 处理器
// Requirements: 12.1, 12.2 - 跨浏览和下载记录的全局搜索
// ============================================================================

// HandleSearch 处理 GET /api/search - 全局搜索
func (h *ConsoleAPIHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "GET" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		h.sendError(w, r, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	// 搜索至少需要 2 个字符
	// Requirements: 12.4 - 2+ 字符后显示建议
	if len(query) < 2 {
		h.sendError(w, r, http.StatusBadRequest, "query must be at least 2 characters")
		return
	}

	// 获取限制 (默认: 20)
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	result, err := h.searchService.Search(query, limit)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, result)
}

// ============================================================================
// 健康检查 API 处理器
// Requirements: 14.7 - 返回服务状态和版本的健康检查端点
// ============================================================================

// HealthStatus 表示健康检查响应
type HealthStatus struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	Timestamp     string `json:"timestamp"`
	WebSocketPort int    `json:"webSocketPort,omitempty"`
}

// HandleHealth 处理 GET /api/health - 健康检查
func (h *ConsoleAPIHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "GET" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	version := "unknown"
	wsPort := 0
	if h.getConfig() != nil {
		version = h.getConfig().Version
		// WebSocket 运行在代理端口 + 1
		wsPort = h.getConfig().Port + 1
	}

	status := HealthStatus{
		Status:        "ok",
		Version:       version,
		Timestamp:     time.Now().Format(time.RFC3339),
		WebSocketPort: wsPort,
	}

	h.sendSuccess(w, r, status)
}

// ============================================================================
// 主路由器
// Requirements: 14.6 - 所有 API 响应的 CORS 中间件
// ============================================================================

// HandleAPIRequest 是所有 /api/* 请求的主路由器
func (h *ConsoleAPIHandler) HandleAPIRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 处理所有 API 端点的 CORS 预检请求
	if r.Method == "OPTIONS" {
		h.setCORSHeaders(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 根据路径路由到相应的处理器
	switch {
	case path == "/api/health":
		h.HandleHealth(w, r)
	case path == "/api/console/verify-token":
		h.HandleVerifyToken(w, r)
	case path == "/api/search":
		h.HandleSearch(w, r)
	case path == "/api/settings":
		h.HandleSettingsAPI(w, r)
	case strings.HasPrefix(path, "/api/stats"):
		h.HandleStatsAPI(w, r)
	case strings.HasPrefix(path, "/api/export"):
		h.HandleExportAPI(w, r)
	case strings.HasPrefix(path, "/api/browse"):
		h.HandleBrowseAPI(w, r)
	case strings.HasPrefix(path, "/api/downloads"):
		h.HandleDownloadsAPI(w, r)
	case strings.HasPrefix(path, "/api/queue"):
		h.HandleQueueAPI(w, r)
	case strings.HasPrefix(path, "/api/files"):
		h.HandleFilesAPI(w, r)
	case path == "/api/video/stream":
		h.HandleVideoStream(w, r)
	case path == "/api/video/play":
		h.HandleVideoPlay(w, r)
	default:
		h.sendError(w, r, http.StatusNotFound, "endpoint not found")
	}
}

// ============================================================================
// 控制台令牌验证
// ============================================================================

// HandleVerifyToken 处理 POST /api/console/verify-token - 验证 Web 控制台访问令牌
func (h *ConsoleAPIHandler) HandleVerifyToken(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "POST" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg := h.getConfig()
	// 如果未配置 token，则允许访问
	if cfg == nil || cfg.WebConsoleToken == "" {
		h.sendSuccess(w, r, map[string]interface{}{
			"valid":   true,
			"message": "token not required",
		})
		return
	}

	// 验证 token
	if req.Token == cfg.WebConsoleToken {
		h.sendSuccess(w, r, map[string]interface{}{
			"valid":   true,
			"message": "token verified",
		})
		return
	}

	h.sendError(w, r, http.StatusUnauthorized, "invalid token")
}

// ============================================================================
// 文件 API 处理器 - 打开文件夹和播放视频
// ============================================================================

// HandleFilesAPI 路由文件操作 API 请求
func (h *ConsoleAPIHandler) HandleFilesAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "POST" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch path {
	case "/api/files/open-folder":
		h.HandleOpenFolder(w, r)
	case "/api/files/play":
		h.HandlePlayVideo(w, r)
	default:
		h.sendError(w, r, http.StatusNotFound, "endpoint not found")
	}
}

// HandleOpenFolder 处理 POST /api/files/open-folder - 在资源管理器中打开文件夹
func (h *ConsoleAPIHandler) HandleOpenFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := h.parseJSON(r, &req); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Path == "" {
		h.sendError(w, r, http.StatusBadRequest, "path is required")
		return
	}

	downloadsDir, err := h.getConfig().GetResolvedDownloadsDir()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "failed to resolve downloads directory")
		return
	}
	absPath, err := validatePathInBase(downloadsDir, req.Path, true)
	if err != nil {
		if pe, ok := err.(*pathValidationError); ok {
			h.sendError(w, r, pe.status, pe.msg)
			return
		}
		h.sendError(w, r, http.StatusInternalServerError, "failed to validate path")
		return
	}

	// 在文件管理器中打开文件夹
	if err := openFileExplorer(absPath); err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "folder opened")
}

// HandlePlayVideo 处理 POST /api/files/play - 使用默认播放器播放视频
func (h *ConsoleAPIHandler) HandlePlayVideo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := h.parseJSON(r, &req); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Path == "" {
		h.sendError(w, r, http.StatusBadRequest, "path is required")
		return
	}

	downloadsDir, err := h.getConfig().GetResolvedDownloadsDir()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "failed to resolve downloads directory")
		return
	}
	absPath, err := validatePathInBase(downloadsDir, req.Path, false)
	if err != nil {
		if pe, ok := err.(*pathValidationError); ok {
			h.sendError(w, r, pe.status, pe.msg)
			return
		}
		h.sendError(w, r, http.StatusInternalServerError, "failed to validate path")
		return
	}
	if !isAllowedVideoExtension(absPath) {
		h.sendError(w, r, http.StatusBadRequest, "unsupported video file extension")
		return
	}

	// 使用默认播放器播放视频
	if err := openWithDefaultApp(absPath); err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "video player opened")
}

// CORSMiddleware 包装 http.Handler 以支持 CORS
// Requirements: 14.6 - 在所有响应中包含 CORS 头
func (h *ConsoleAPIHandler) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 处理预检请求
		if r.Method == "OPTIONS" {
			h.setCORSHeaders(w, r)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// 为所有响应设置 CORS 头
		h.setCORSHeaders(w, r)

		// 调用下一个处理器
		next.ServeHTTP(w, r)
	})
}

// ============================================================================
// 特定平台的文件操作
// ============================================================================

// openFileExplorer 在系统文件管理器中打开包含该文件的文件夹
func openFileExplorer(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	isDir := info.IsDir()
	dir := filePath
	if !isDir {
		dir = filepath.Dir(filePath)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if isDir {
			cmd = exec.Command("explorer", filepath.FromSlash(dir))
		} else {
			// 在 Windows 上，使用 explorer 打开文件夹并选择文件
			// 转换为 Windows 路径格式 (反斜杠)
			winPath := filepath.FromSlash(filePath)
			cmd = exec.Command("explorer", "/select,", winPath)
		}
	case "darwin":
		// 在 macOS 上，对目录直接 open，对文件使用 open -R 显示文件
		if isDir {
			cmd = exec.Command("open", dir)
		} else {
			cmd = exec.Command("open", "-R", filePath)
		}
	default:
		// 在 Linux 上，使用 xdg-open 打开文件夹
		cmd = exec.Command("xdg-open", dir)
	}

	return cmd.Start()
}

// openWithDefaultApp 使用系统默认应用程序打开文件
func openWithDefaultApp(filePath string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// 转换为 Windows 路径格式 (反斜杠)
		winPath := filepath.FromSlash(filePath)
		cmd = exec.Command("cmd", "/c", "start", "", winPath)
	case "darwin":
		cmd = exec.Command("open", filePath)
	default:
		cmd = exec.Command("xdg-open", filePath)
	}

	return cmd.Start()
}

// ============================================================================
// Video Stream API Handler
// ============================================================================

// HandleVideoStream 处理 GET /api/video/stream - 视频文件流
func (h *ConsoleAPIHandler) HandleVideoStream(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "GET" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 从查询参数获取文件路径
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		h.sendError(w, r, http.StatusBadRequest, "path parameter is required")
		return
	}

	downloadsDir, err := h.getConfig().GetResolvedDownloadsDir()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "failed to resolve downloads directory")
		return
	}
	resolvedPath, err := validatePathInBase(downloadsDir, filePath, false)
	if err != nil {
		if pe, ok := err.(*pathValidationError); ok {
			h.sendError(w, r, pe.status, pe.msg)
			return
		}
		h.sendError(w, r, http.StatusInternalServerError, "failed to validate path")
		return
	}
	fileInfo, err := os.Stat(resolvedPath)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "failed to access file")
		return
	}

	// 打开文件
	file, err := os.Open(resolvedPath)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer file.Close()

	// 根据文件扩展名确定内容类型
	ext := strings.ToLower(filepath.Ext(resolvedPath))
	contentType := "application/octet-stream"
	switch ext {
	case ".mp4":
		contentType = "video/mp4"
	case ".webm":
		contentType = "video/webm"
	case ".ogg", ".ogv":
		contentType = "video/ogg"
	case ".mov":
		contentType = "video/quicktime"
	case ".avi":
		contentType = "video/x-msvideo"
	case ".mkv":
		contentType = "video/x-matroska"
	}

	// 设置 CORS 头
	h.setCORSHeaders(w, r)

	// 处理视频跳转的范围请求
	fileSize := fileInfo.Size()
	rangeHeader := r.Header.Get("Range")

	if rangeHeader != "" {
		// 解析范围头
		var start, end int64
		_, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
		if err != nil {
			// 尝试不带结束位置解析
			_, err = fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
			if err != nil {
				h.sendError(w, r, http.StatusBadRequest, "invalid range header")
				return
			}
			end = fileSize - 1
		}

		// 验证范围
		if start < 0 || start >= fileSize || end >= fileSize || start > end {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		// 跳转到起始位置
		_, err = file.Seek(start, 0)
		if err != nil {
			h.sendError(w, r, http.StatusInternalServerError, "failed to seek file")
			return
		}

		// 设置部分内容的响应头
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)

		// 复制请求的范围
		io.CopyN(w, file, end-start+1)
	} else {
		// 完整文件请求
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)

		// 复制整个文件
		io.Copy(w, file)
	}
}

// isPathWithinBase returns true when targetPath is within baseDir.
func isPathWithinBase(baseDir, targetPath string) bool {
	rel, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

func validatePathInBase(baseDir, targetPath string, allowDir bool) (string, error) {
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return "", &pathValidationError{status: http.StatusBadRequest, msg: "invalid path"}
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", &pathValidationError{status: http.StatusInternalServerError, msg: "failed to resolve downloads directory"}
	}
	realBaseDir, err := filepath.EvalSymlinks(absBaseDir)
	if err != nil {
		realBaseDir = absBaseDir
	}

	if !isPathWithinBase(realBaseDir, absPath) {
		return "", &pathValidationError{status: http.StatusForbidden, msg: "access to path outside downloads directory is forbidden"}
	}

	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return "", &pathValidationError{status: http.StatusNotFound, msg: "file not found"}
	}
	if err != nil {
		return "", &pathValidationError{status: http.StatusInternalServerError, msg: "failed to access file"}
	}
	if !allowDir && info.IsDir() {
		return "", &pathValidationError{status: http.StatusBadRequest, msg: "path is a directory"}
	}

	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		realPath = absPath
	}
	if !isPathWithinBase(realBaseDir, realPath) {
		return "", &pathValidationError{status: http.StatusForbidden, msg: "access to path outside downloads directory is forbidden"}
	}

	return realPath, nil
}

func isAllowedVideoExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".webm", ".ogg", ".ogv", ".mov", ".avi", ".mkv":
		return true
	default:
		return false
	}
}

// validateVideoPlayTargetURL validates upstream video URL and blocks local/private targets.
func validateVideoPlayTargetURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid target URL")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("only http/https URLs are allowed")
	}
	if u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("invalid target URL")
	}
	if u.User != nil {
		return "", fmt.Errorf("URL userinfo is not allowed")
	}

	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return "", fmt.Errorf("local addresses are not allowed")
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
			return "", fmt.Errorf("local/private addresses are not allowed")
		}
	}

	if ips, err := net.LookupIP(host); err == nil && len(ips) > 0 {
		for _, ip := range ips {
			if addr, ok := netip.AddrFromSlice(ip); ok {
				if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
					return "", fmt.Errorf("local/private addresses are not allowed")
				}
			}
		}
	}

	return u.String(), nil
}

func newVideoProxyHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 0, // 不设置超时，支持长时间流式传输
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if _, err := validateVideoPlayTargetURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

// HandleVideoPlay 处理 GET /api/video/play - 远程视频流式播放（支持加密解密）
// 参数:
//   - url: 视频源 URL（必需）
//   - key: 解密密钥，uint64 格式（可选，仅用于加密视频）
func (h *ConsoleAPIHandler) HandleVideoPlay(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "GET" && r.Method != "HEAD" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 从查询参数获取视频 URL
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		h.sendError(w, r, http.StatusBadRequest, "url parameter is required")
		return
	}

	// 获取可选的解密密钥
	decryptKeyStr := r.URL.Query().Get("key")
	var decryptKey uint64
	var needsDecryption bool

	if decryptKeyStr != "" {
		var err error
		decryptKey, err = strconv.ParseUint(decryptKeyStr, 10, 64)
		if err != nil {
			h.sendError(w, r, http.StatusBadRequest, "invalid decryption key")
			return
		}
		needsDecryption = true
	}

	validatedURL, err := validateVideoPlayTargetURL(targetURL)
	if err != nil {
		h.sendError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// 创建上游请求
	upstreamReq, err := http.NewRequest(r.Method, validatedURL, nil)
	if err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid target URL")
		return
	}

	// 复制 Range 头（支持视频拖动）
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		upstreamReq.Header.Set("Range", rangeHeader)
	}

	// 发起上游请求
	client := newVideoProxyHTTPClient()
	upstreamResp, err := client.Do(upstreamReq)
	if err != nil {
		h.sendError(w, r, http.StatusBadGateway, "failed to fetch video: "+err.Error())
		return
	}
	defer upstreamResp.Body.Close()

	// 设置 CORS 头
	h.setCORSHeaders(w, r)

	// 复制上游响应头
	for k, v := range upstreamResp.Header {
		w.Header()[k] = v
	}

	// 确保设置 Accept-Ranges
	if w.Header().Get("Accept-Ranges") == "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}

	// 如果需要解密
	if needsDecryption {
		// 解析 Content-Range 头以获取起始偏移
		var startOffset uint64 = 0
		if cr := upstreamResp.Header.Get("Content-Range"); cr != "" {
			// Content-Range 格式: "bytes start-end/total"
			parts := strings.Split(cr, " ")
			if len(parts) == 2 {
				rangePart := parts[1]
				dashIdx := strings.Index(rangePart, "-")
				if dashIdx > 0 {
					if v, err := strconv.ParseUint(rangePart[:dashIdx], 10, 64); err == nil {
						startOffset = v
					}
				}
			}
		}

		// 创建解密读取器
		// 加密区域大小为 131072 字节（128KB）
		decryptReader := utils.NewDecryptReader(upstreamResp.Body, decryptKey, startOffset, 131072)

		// 写入状态码
		w.WriteHeader(upstreamResp.StatusCode)

		// 如果是 HEAD 请求，不传输内容
		if r.Method == "HEAD" {
			return
		}

		// 流式复制解密后的数据到客户端
		io.Copy(w, decryptReader)
	} else {
		// 无需解密，直接代理
		w.WriteHeader(upstreamResp.StatusCode)

		// 如果是 HEAD 请求，不传输内容
		if r.Method == "HEAD" {
			return
		}

		// 流式复制数据到客户端
		io.Copy(w, upstreamResp.Body)
	}
}
