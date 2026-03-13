package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"wx_channel/internal/database"
	"wx_channel/internal/response"
)

// RadarServiceAPI 处理雷达监控相关的 API
type RadarServiceAPI struct {
	repo *database.RadarRepository
}

// NewRadarServiceAPI 创建雷达服务 API 处理器
func NewRadarServiceAPI() *RadarServiceAPI {
	return &RadarServiceAPI{
		repo: database.NewRadarRepository(),
	}
}

// GetTargets 获取所有监控目标
func (h *RadarServiceAPI) GetTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := h.repo.GetAll()
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "获取监控目标失败")
		return
	}
	response.Success(w, targets)
}

// AddTarget 添加监控目标
func (h *RadarServiceAPI) AddTarget(w http.ResponseWriter, r *http.Request) {
	var target database.RadarTarget
	if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
		response.Error(w, http.StatusBadRequest, "请求参数解析失败")
		return
	}

	target.Username = strings.TrimSpace(target.Username)
	target.AuthorName = strings.TrimSpace(target.AuthorName)

	if target.Username == "" || target.AuthorName == "" {
		response.Error(w, http.StatusBadRequest, "账号ID和账号名称不能为空")
		return
	}

	if target.IntervalMinutes < 5 {
		target.IntervalMinutes = 5 // 最少5分钟
	}

	if target.Status == "" {
		target.Status = database.RadarStatusActive
	}

	if err := h.repo.Add(&target); err != nil {
		// 判断是否是唯一键冲突
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			response.Error(w, http.StatusConflict, "该账号已在监控列表中")
			return
		}
		response.Error(w, http.StatusInternalServerError, "添加监控目标失败")
		return
	}

	response.Success(w, target)
}

// UpdateTarget 更新监控目标
func (h *RadarServiceAPI) UpdateTarget(w http.ResponseWriter, r *http.Request) {
	// 从路径中获取 ID
	pathParts := strings.Split(r.URL.Path, "/")
	id := pathParts[len(pathParts)-1]

	var target database.RadarTarget
	if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
		response.Error(w, http.StatusBadRequest, "请求参数解析失败")
		return
	}

	target.ID = id
	if target.IntervalMinutes < 5 {
		target.IntervalMinutes = 5
	}

	// 保留之前的检测时间
	existing, err := h.repo.GetByID(id)
	if err == nil && existing != nil {
		target.LastCheckTime = existing.LastCheckTime
	}

	if err := h.repo.Update(&target); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			response.Error(w, http.StatusConflict, "该账号已被其他记录占用")
			return
		}
		response.Error(w, http.StatusInternalServerError, "更新监控目标失败")
		return
	}

	response.Success(w, target)
}

// UpdateTargetStatus 更新监控状态 (暂停/恢复)
func (h *RadarServiceAPI) UpdateTargetStatus(w http.ResponseWriter, r *http.Request) {
	// 从路径中获取 ID
	pathParts := strings.Split(r.URL.Path, "/")
	// /api/v1/radar/targets/{id}/status
	if len(pathParts) < 2 {
		response.Error(w, http.StatusBadRequest, "无效的请求路径")
		return
	}
	id := pathParts[len(pathParts)-2]

	var req struct {
		Status database.RadarTargetStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "请求参数解析失败")
		return
	}

	if req.Status != database.RadarStatusActive && req.Status != database.RadarStatusPaused {
		response.Error(w, http.StatusBadRequest, "无效的状态值")
		return
	}

	if err := h.repo.UpdateStatus(id, req.Status); err != nil {
		response.Error(w, http.StatusInternalServerError, "更新状态失败")
		return
	}

	response.Success(w, nil)
}

// DeleteTarget 删除监控目标
func (h *RadarServiceAPI) DeleteTarget(w http.ResponseWriter, r *http.Request) {
	// 从路径中获取 ID
	pathParts := strings.Split(r.URL.Path, "/")
	id := pathParts[len(pathParts)-1]

	if err := h.repo.Delete(id); err != nil {
		response.Error(w, http.StatusInternalServerError, "删除监控目标失败")
		return
	}

	response.Success(w, nil)
}

// GetRadarLogs 获取监控目标的执行日志
func (h *RadarServiceAPI) GetRadarLogs(w http.ResponseWriter, r *http.Request) {
	// 从路径中获取 ID (/api/v1/radar/targets/{id}/logs)
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 2 {
		response.Error(w, http.StatusBadRequest, "无效的请求路径")
		return
	}
	id := pathParts[len(pathParts)-2]

	logs, err := h.repo.GetLogsByTargetID(id, 50)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "获取日志失败")
		return
	}

	if logs == nil {
		logs = []database.RadarLog{}
	}

	response.Success(w, logs)
}

// RegisterRoutes 注册雷达相关的 API 路由
func (h *RadarServiceAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/radar/targets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.GetTargets(w, r)
		case http.MethodPost:
			h.AddTarget(w, r)
		default:
			response.Error(w, http.StatusMethodNotAllowed, "不允许的请求方法")
		}
	})

	mux.HandleFunc("/api/v1/radar/targets/", func(w http.ResponseWriter, r *http.Request) {
		// 处理 /api/v1/radar/targets/{id} 和 /api/v1/radar/targets/{id}/status 和 /logs
		path := r.URL.Path
		if strings.HasSuffix(path, "/status") && r.Method == http.MethodPut {
			h.UpdateTargetStatus(w, r)
			return
		}
		if strings.HasSuffix(path, "/logs") && r.Method == http.MethodGet {
			h.GetRadarLogs(w, r)
			return
		}

		switch r.Method {
		case http.MethodPut:
			h.UpdateTarget(w, r)
		case http.MethodDelete:
			h.DeleteTarget(w, r)
		default:
			response.Error(w, http.StatusMethodNotAllowed, "不允许的请求方法")
		}
	})
}
