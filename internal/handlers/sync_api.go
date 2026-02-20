package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
	"wx_channel/internal/response"
	"wx_channel/internal/utils"
)

// SyncAPIHandler 数据同步API处理器（供Hub拉取）
type SyncAPIHandler struct {
	browseRepo   *database.BrowseHistoryRepository
	downloadRepo *database.DownloadRecordRepository
}

// NewSyncAPIHandler 创建同步API处理器
func NewSyncAPIHandler() *SyncAPIHandler {
	return &SyncAPIHandler{
		browseRepo:   database.NewBrowseHistoryRepository(),
		downloadRepo: database.NewDownloadRecordRepository(),
	}
}

// HandleGetBrowseHistory 获取浏览记录（分页）
// GET /api/sync/browse?since=timestamp&limit=100
func (h *SyncAPIHandler) HandleGetBrowseHistory(w http.ResponseWriter, r *http.Request) {
	// 验证Hub的访问权限
	if !h.validateAccess(w, r) {
		return
	}

	// 获取查询参数
	sinceStr := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")

	// 解析参数
	var since time.Time
	if sinceStr != "" {
		timestamp, err := strconv.ParseInt(sinceStr, 10, 64)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid since parameter")
			return
		}
		since = time.Unix(timestamp, 0)
	}

	limit := 1000 // 默认限制
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 10000 {
			limit = l
		}
	}

	// 查询增量数据
	records, err := h.browseRepo.GetRecordsSince(since, limit)
	if err != nil {
		utils.Error("Failed to get browse records: %v", err)
		response.Error(w, http.StatusInternalServerError, "Failed to get records")
		return
	}

	// 返回数据
	response.Success(w, map[string]interface{}{
		"records":   records,
		"count":     len(records),
		"timestamp": time.Now().Unix(),
	})

	utils.Info("[Sync API] Returned %d browse records (since: %v)", len(records), since)
}

// HandleGetDownloadRecords 获取下载记录（分页）
// GET /api/sync/download?since=timestamp&limit=100
func (h *SyncAPIHandler) HandleGetDownloadRecords(w http.ResponseWriter, r *http.Request) {
	// 验证Hub的访问权限
	if !h.validateAccess(w, r) {
		return
	}

	// 获取查询参数
	sinceStr := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")

	// 解析参数
	var since time.Time
	if sinceStr != "" {
		timestamp, err := strconv.ParseInt(sinceStr, 10, 64)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid since parameter")
			return
		}
		since = time.Unix(timestamp, 0)
	}

	limit := 1000 // 默认限制
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 10000 {
			limit = l
		}
	}

	// 查询增量数据
	records, err := h.downloadRepo.GetRecordsSince(since, limit)
	if err != nil {
		utils.Error("Failed to get download records: %v", err)
		response.Error(w, http.StatusInternalServerError, "Failed to get records")
		return
	}

	// 返回数据
	response.Success(w, map[string]interface{}{
		"records":   records,
		"count":     len(records),
		"timestamp": time.Now().Unix(),
	})

	utils.Info("[Sync API] Returned %d download records (since: %v)", len(records), since)
}

// HandleGetStats 获取统计信息
// GET /api/sync/stats
func (h *SyncAPIHandler) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	// 验证Hub的访问权限
	if !h.validateAccess(w, r) {
		return
	}

	// 获取统计信息
	browseCount, _ := h.browseRepo.Count()
	downloadCount, _ := h.downloadRepo.Count()
	lastBrowse, _ := h.browseRepo.GetLatestTimestamp()
	lastDownload, _ := h.downloadRepo.GetLatestTimestamp()

	stats := map[string]interface{}{
		"browse_count":    browseCount,
		"download_count":  downloadCount,
		"last_browse":     lastBrowse.Unix(),
		"last_download":   lastDownload.Unix(),
		"machine_id":      config.Get().MachineID,
		"version":         config.Get().Version,
	}

	response.Success(w, stats)
	utils.Info("[Sync API] Returned stats")
}

// validateAccess 验证Hub的访问权限
func (h *SyncAPIHandler) validateAccess(w http.ResponseWriter, r *http.Request) bool {
	cfg := config.Get()

	// 检查是否启用同步API
	if !cfg.HubSync.Enabled {
		response.Error(w, http.StatusForbidden, "Sync API is disabled")
		return false
	}

	// 验证令牌
	token := r.Header.Get("X-Hub-Token")
	if cfg.HubSync.Token != "" && token != cfg.HubSync.Token {
		utils.Warn("[Sync API] Invalid token from %s", r.RemoteAddr)
		response.Error(w, http.StatusUnauthorized, "Invalid token")
		return false
	}

	// 验证IP白名单（如果配置了）
	if len(cfg.HubSync.AllowedIPs) > 0 {
		clientIP := getClientIP(r)
		allowed := false
		for _, allowedIP := range cfg.HubSync.AllowedIPs {
			if clientIP == allowedIP || allowedIP == "*" {
				allowed = true
				break
			}
		}
		if !allowed {
			utils.Warn("[Sync API] Access denied for IP: %s", clientIP)
			response.Error(w, http.StatusForbidden, "IP not allowed")
			return false
		}
	}

	return true
}

// getClientIP 获取客户端真实IP
func getClientIP(r *http.Request) string {
	// 尝试从 X-Forwarded-For 获取
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// 尝试从 X-Real-IP 获取
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// 从 RemoteAddr 获取
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
