package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
)

// ConsoleAPIHandler handles REST API requests for the web console
type ConsoleAPIHandler struct {
	config           *config.Config
	browseService    *services.BrowseHistoryService
	downloadService  *services.DownloadRecordService
	queueService     *services.QueueService
	settingsRepo     *database.SettingsRepository
	statsService     *services.StatisticsService
	exportService    *services.ExportService
	searchService    *services.SearchService
}

// NewConsoleAPIHandler creates a new ConsoleAPIHandler
func NewConsoleAPIHandler(cfg *config.Config) *ConsoleAPIHandler {
	return &ConsoleAPIHandler{
		config:           cfg,
		browseService:    services.NewBrowseHistoryService(),
		downloadService:  services.NewDownloadRecordService(),
		queueService:     services.NewQueueService(),
		settingsRepo:     database.NewSettingsRepository(),
		statsService:     services.NewStatisticsService(),
		exportService:    services.NewExportService(),
		searchService:    services.NewSearchService(),
	}
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// sendJSON sends a JSON response
func (h *ConsoleAPIHandler) sendJSON(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	h.setCORSHeaders(w, r)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// sendSuccess sends a success response
func (h *ConsoleAPIHandler) sendSuccess(w http.ResponseWriter, r *http.Request, data interface{}) {
	h.sendJSON(w, r, http.StatusOK, APIResponse{Success: true, Data: data})
}

// sendSuccessMessage sends a success response with a message
func (h *ConsoleAPIHandler) sendSuccessMessage(w http.ResponseWriter, r *http.Request, message string) {
	h.sendJSON(w, r, http.StatusOK, APIResponse{Success: true, Message: message})
}

// sendError sends an error response
func (h *ConsoleAPIHandler) sendError(w http.ResponseWriter, r *http.Request, status int, message string) {
	h.sendJSON(w, r, status, APIResponse{Success: false, Error: message})
}

// setCORSHeaders sets CORS headers for the response
// Requirements: 14.6 - include CORS headers for remote console
func (h *ConsoleAPIHandler) setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		// Allow all origins for local development, or check against allowed origins
		if h.config != nil && len(h.config.AllowedOrigins) > 0 {
			for _, o := range h.config.AllowedOrigins {
				if o == origin || o == "*" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		} else {
			// Default: allow all origins for local service
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth, Authorization")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// HandleCORS handles CORS preflight requests
func (h *ConsoleAPIHandler) HandleCORS(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == "OPTIONS" {
		h.setCORSHeaders(w, r)
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// parseJSON parses JSON request body
func (h *ConsoleAPIHandler) parseJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.Unmarshal(body, v)
}

// getPaginationParams extracts pagination parameters from query string
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

// getFilterParams extracts filter parameters from query string
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
			// Set to end of day
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

// extractIDFromPath extracts the ID from a URL path like /api/browse/123
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
// Browse History API Handlers
// Requirements: 14.1 - REST API endpoints for browse history CRUD operations
// ============================================================================

// HandleBrowseList handles GET /api/browse - paginated list
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

// HandleBrowseGet handles GET /api/browse/:id - single record
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

// HandleBrowseDelete handles DELETE /api/browse/:id - delete single record
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

// HandleBrowseDeleteMany handles DELETE /api/browse - batch delete
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

// HandleBrowseClear handles DELETE /api/browse/clear - clear all records
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

// HandleBrowseAPI routes browse API requests
func (h *ConsoleAPIHandler) HandleBrowseAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	// DELETE /api/browse/clear - must be checked before extracting ID
	if path == "/api/browse/clear" && r.Method == "DELETE" {
		h.HandleBrowseClear(w, r)
		return
	}

	// Extract ID from path
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
// Download Records API Handlers
// Requirements: 14.2 - REST API endpoints for download records CRUD operations
// ============================================================================

// HandleDownloadsList handles GET /api/downloads - paginated list with filters
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

// HandleDownloadsGet handles GET /api/downloads/:id - single record
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

// HandleDownloadsDelete handles DELETE /api/downloads/:id - delete single record
func (h *ConsoleAPIHandler) HandleDownloadsDelete(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	// Check if files should be deleted
	deleteFiles := r.URL.Query().Get("deleteFiles") == "true"

	err := h.downloadService.Delete(id, deleteFiles)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "record deleted")
}

// HandleDownloadsDeleteMany handles DELETE /api/downloads - batch delete
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

// HandleDownloadsAPI routes download API requests
func (h *ConsoleAPIHandler) HandleDownloadsAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	// Extract ID from path
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
// Download Queue API Handlers
// Requirements: 14.3 - REST API endpoints for download queue management
// ============================================================================

// HandleQueueList handles GET /api/queue - list queue items
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

// HandleQueueAdd handles POST /api/queue - add items to queue
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

	// Broadcast queue changes via WebSocket
	// Requirements: 14.5 - broadcast queue changes
	hub := GetWebSocketHub()
	for i := range items {
		hub.BroadcastQueueAdd(&items[i])
	}

	h.sendSuccess(w, r, items)
}

// HandleQueuePause handles PUT /api/queue/:id/pause - pause download
func (h *ConsoleAPIHandler) HandleQueuePause(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.queueService.Pause(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Broadcast queue update via WebSocket
	item, _ := h.queueService.GetByID(id)
	if item != nil {
		GetWebSocketHub().BroadcastQueueUpdate(item)
	}

	h.sendSuccessMessage(w, r, "download paused")
}

// HandleQueueResume handles PUT /api/queue/:id/resume - resume download
func (h *ConsoleAPIHandler) HandleQueueResume(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.queueService.Resume(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Broadcast queue update via WebSocket
	item, _ := h.queueService.GetByID(id)
	if item != nil {
		GetWebSocketHub().BroadcastQueueUpdate(item)
	}

	h.sendSuccessMessage(w, r, "download resumed")
}

// HandleQueueRemove handles DELETE /api/queue/:id - remove from queue
func (h *ConsoleAPIHandler) HandleQueueRemove(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.queueService.RemoveFromQueue(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Broadcast queue removal via WebSocket
	GetWebSocketHub().BroadcastQueueRemove(id)

	h.sendSuccessMessage(w, r, "item removed from queue")
}

// HandleQueueReorder handles PUT /api/queue/reorder - reorder queue
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

	// Broadcast queue reorder via WebSocket
	queue, _ := h.queueService.GetQueue()
	GetWebSocketHub().BroadcastQueueReorder(queue)

	h.sendSuccessMessage(w, r, "queue reordered")
}

// HandleQueueComplete handles PUT /api/queue/:id/complete - mark download as completed
func (h *ConsoleAPIHandler) HandleQueueComplete(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	err := h.queueService.CompleteDownload(id)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Get updated item for WebSocket broadcast
	item, _ := h.queueService.GetByID(id)
	if item != nil {
		GetWebSocketHub().BroadcastQueueUpdate(item)
	}

	h.sendSuccessMessage(w, r, "download completed")
}

// HandleQueueFail handles PUT /api/queue/:id/fail - mark download as failed
func (h *ConsoleAPIHandler) HandleQueueFail(w http.ResponseWriter, r *http.Request, id string) {
	if h.HandleCORS(w, r) {
		return
	}

	var req struct {
		Error string `json:"error"`
	}
	h.parseJSON(r, &req)

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

// HandleQueueAPI routes queue API requests
func (h *ConsoleAPIHandler) HandleQueueAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	// Handle reorder endpoint
	if path == "/api/queue/reorder" && r.Method == "PUT" {
		h.HandleQueueReorder(w, r)
		return
	}

	// Extract ID and action from path
	// Path format: /api/queue/:id or /api/queue/:id/pause or /api/queue/:id/resume
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
// Settings API Handlers
// Requirements: 14.4 - REST API endpoints for settings management
// ============================================================================

// HandleSettingsGet handles GET /api/settings - get settings
func (h *ConsoleAPIHandler) HandleSettingsGet(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	settings, err := h.settingsRepo.Load()
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccess(w, r, settings)
}

// HandleSettingsUpdate handles PUT /api/settings - update settings
func (h *ConsoleAPIHandler) HandleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	var settings database.Settings
	if err := h.parseJSON(r, &settings); err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate and save settings
	// Requirements: 11.3, 11.4 - validate chunk size (1-100MB) and concurrent limit (1-5)
	if err := h.settingsRepo.SaveAndValidate(&settings); err != nil {
		h.sendError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "settings updated")
}

// HandleSettingsAPI routes settings API requests
func (h *ConsoleAPIHandler) HandleSettingsAPI(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
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
// Statistics API Handlers
// Requirements: 7.1, 7.2 - statistics and chart data endpoints
// ============================================================================

// HandleStatsGet handles GET /api/stats - get statistics
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

// HandleStatsChart handles GET /api/stats/chart - get chart data
func (h *ConsoleAPIHandler) HandleStatsChart(w http.ResponseWriter, r *http.Request) {
	if h.HandleCORS(w, r) {
		return
	}

	// Default to 7 days
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

// HandleStatsAPI routes stats API requests
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
// Export API Handlers
// Requirements: 4.1, 4.2 - export browse and download records
// ============================================================================

// HandleExportBrowse handles GET /api/export/browse - export browse records
func (h *ConsoleAPIHandler) HandleExportBrowse(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.exportService.ExportBrowseHistory(format, ids)
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

// HandleExportDownloads handles GET /api/export/downloads - export download records
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

// HandleExportAPI routes export API requests
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
// Search API Handlers
// Requirements: 12.1, 12.2 - global search across browse and download records
// ============================================================================

// HandleSearch handles GET /api/search - global search
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

	// Minimum 2 characters for search
	// Requirements: 12.4 - show suggestions after 2+ characters
	if len(query) < 2 {
		h.sendError(w, r, http.StatusBadRequest, "query must be at least 2 characters")
		return
	}

	// Get limit (default: 20)
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
// Health Check API Handler
// Requirements: 14.7 - health check endpoint returning service status and version
// ============================================================================

// HealthStatus represents the health check response
type HealthStatus struct {
	Status      string `json:"status"`
	Version     string `json:"version"`
	Timestamp   string `json:"timestamp"`
	WebSocketPort int  `json:"webSocketPort,omitempty"`
}

// HandleHealth handles GET /api/health - health check
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
	if h.config != nil {
		version = h.config.Version
		// WebSocket runs on proxy port + 1
		wsPort = h.config.Port + 1
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
// Main Router
// Requirements: 14.6 - CORS middleware for all API responses
// ============================================================================

// HandleAPIRequest is the main router for all /api/* requests
func (h *ConsoleAPIHandler) HandleAPIRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle CORS preflight for all API endpoints
	if r.Method == "OPTIONS" {
		h.setCORSHeaders(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Route to appropriate handler based on path
	switch {
	case path == "/api/health":
		h.HandleHealth(w, r)
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
	default:
		h.sendError(w, r, http.StatusNotFound, "endpoint not found")
	}
}

// ============================================================================
// Files API Handlers - Open folder and play video
// ============================================================================

// HandleFilesAPI routes file operation API requests
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

// HandleOpenFolder handles POST /api/files/open-folder - open file folder in explorer
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

	// Open folder in file explorer
	if err := openFileExplorer(req.Path); err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "folder opened")
}

// HandlePlayVideo handles POST /api/files/play - play video with default player
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

	// Play video with default player
	if err := openWithDefaultApp(req.Path); err != nil {
		h.sendError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	h.sendSuccessMessage(w, r, "video player opened")
}

// CORSMiddleware wraps an http.Handler with CORS support
// Requirements: 14.6 - include CORS headers in all responses
func (h *ConsoleAPIHandler) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle preflight requests
		if r.Method == "OPTIONS" {
			h.setCORSHeaders(w, r)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Set CORS headers for all responses
		h.setCORSHeaders(w, r)

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}


// ============================================================================
// Platform-specific file operations
// ============================================================================

// openFileExplorer opens the folder containing the file in the system file explorer
func openFileExplorer(filePath string) error {
	// Get the directory containing the file
	dir := filepath.Dir(filePath)
	
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// On Windows, use explorer to open the folder and select the file
		// Convert to Windows path format (backslashes)
		winPath := filepath.FromSlash(filePath)
		cmd = exec.Command("explorer", "/select,", winPath)
	case "darwin":
		// On macOS, use open -R to reveal the file in Finder
		cmd = exec.Command("open", "-R", filePath)
	default:
		// On Linux, use xdg-open to open the folder
		cmd = exec.Command("xdg-open", dir)
	}
	
	return cmd.Start()
}

// openWithDefaultApp opens a file with the system's default application
func openWithDefaultApp(filePath string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Convert to Windows path format (backslashes)
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

// HandleVideoStream handles GET /api/video/stream - stream video file
func (h *ConsoleAPIHandler) HandleVideoStream(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if h.HandleCORS(w, r) {
		return
	}

	if r.Method != "GET" {
		h.sendError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Get file path from query parameter
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		h.sendError(w, r, http.StatusBadRequest, "path parameter is required")
		return
	}

	// Security check: ensure the path is within allowed directories
	// Convert to absolute path and check if it exists
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		h.sendError(w, r, http.StatusBadRequest, "invalid path")
		return
	}

	// Check if file exists
	fileInfo, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		h.sendError(w, r, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "failed to access file")
		return
	}
	if fileInfo.IsDir() {
		h.sendError(w, r, http.StatusBadRequest, "path is a directory")
		return
	}

	// Open the file
	file, err := os.Open(absPath)
	if err != nil {
		h.sendError(w, r, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer file.Close()

	// Determine content type based on file extension
	ext := strings.ToLower(filepath.Ext(absPath))
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

	// Set CORS headers
	h.setCORSHeaders(w, r)

	// Handle range requests for video seeking
	fileSize := fileInfo.Size()
	rangeHeader := r.Header.Get("Range")

	if rangeHeader != "" {
		// Parse range header
		var start, end int64
		_, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
		if err != nil {
			// Try parsing without end
			_, err = fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
			if err != nil {
				h.sendError(w, r, http.StatusBadRequest, "invalid range header")
				return
			}
			end = fileSize - 1
		}

		// Validate range
		if start < 0 || start >= fileSize || end >= fileSize || start > end {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		// Seek to start position
		_, err = file.Seek(start, 0)
		if err != nil {
			h.sendError(w, r, http.StatusInternalServerError, "failed to seek file")
			return
		}

		// Set headers for partial content
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)

		// Copy the requested range
		io.CopyN(w, file, end-start+1)
	} else {
		// Full file request
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)

		// Copy the entire file
		io.Copy(w, file)
	}
}
