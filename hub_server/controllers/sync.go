package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"wx_channel/hub_server/database"
	"wx_channel/hub_server/models"
	"wx_channel/hub_server/services"

	"github.com/gorilla/mux"
)

// GetSyncStatus 获取所有设备的同步状态
func GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	var statuses []models.SyncStatus
	
	// 查询所有同步状态
	if err := database.DB.Order("updated_at desc").Find(&statuses).Error; err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Failed to get sync status",
			"error":   err.Error(),
		})
		return
	}

	// 获取设备信息，添加设备名称
	type SyncStatusWithDevice struct {
		models.SyncStatus
		DeviceName string `json:"device_name"`
	}

	result := make([]SyncStatusWithDevice, 0, len(statuses))
	for _, status := range statuses {
		node, err := database.GetNodeByID(status.MachineID)
		deviceName := ""
		if err == nil {
			if node.DisplayName != "" {
				deviceName = node.DisplayName
			} else if node.Hostname != "" {
				deviceName = node.Hostname
			}
		}

		result = append(result, SyncStatusWithDevice{
			SyncStatus: status,
			DeviceName: deviceName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

// GetDeviceSyncStatus 获取单个设备的同步状态
func GetDeviceSyncStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	machineID := vars["machine_id"]

	var status models.SyncStatus
	if err := database.DB.Where("machine_id = ?", machineID).First(&status).Error; err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Sync status not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    status,
	})
}

// TriggerSync 触发同步检查
// 注意：WebSocket 推送模式下，客户端会自动推送数据
// 此接口仅用于检查设备状态，不会主动拉取数据
func TriggerSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MachineID string `json:"machine_id"`
		SyncAll   bool   `json:"sync_all"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Invalid request body",
		})
		return
	}

	syncService := services.GetSyncService()
	if syncService == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Sync service is not available",
		})
		return
	}

	if req.SyncAll {
		// 检查所有设备状态
		go func() {
			nodes, err := database.GetActiveNodes(10 * 60 * 1000000000) // 10 minutes
			if err != nil {
				return
			}
			for _, node := range nodes {
				syncService.SyncDevice(node.ID)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "正在检查所有设备状态，客户端会自动推送数据",
		})
		return
	}

	if req.MachineID == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "machine_id is required",
		})
		return
	}

	// 检查单个设备状态
	go syncService.SyncDevice(req.MachineID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "正在检查设备状态，客户端会自动推送数据",
	})
}

// GetSyncHistory 获取同步历史
func GetSyncHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	machineID := vars["machine_id"]

	var history []models.SyncHistory
	if err := database.DB.Where("machine_id = ?", machineID).
		Order("sync_time desc").
		Limit(100).
		Find(&history).Error; err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Failed to get sync history",
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    history,
	})
}

// GetBrowseRecords 获取浏览记录（聚合查询）
func GetBrowseRecords(w http.ResponseWriter, r *http.Request) {
	// 解析查询参数
	query := r.URL.Query()
	machineID := query.Get("machine_id")
	page := parseIntParam(query.Get("page"), 1)
	pageSize := parseIntParam(query.Get("page_size"), 20)
	
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	// 构建查询
	db := database.DB.Model(&models.HubBrowseHistory{})
	if machineID != "" {
		db = db.Where("machine_id = ?", machineID)
	}

	// 获取总数
	var total int64
	db.Count(&total)

	// 获取记录
	var records []models.HubBrowseHistory
	if err := db.Order("browse_time desc").
		Offset(offset).
		Limit(pageSize).
		Find(&records).Error; err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Failed to get browse records",
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"records": records,
			"total":   total,
			"page":    page,
			"size":    pageSize,
		},
	})
}

// GetDownloadRecords 获取下载记录（聚合查询）
func GetDownloadRecords(w http.ResponseWriter, r *http.Request) {
	// 解析查询参数
	query := r.URL.Query()
	machineID := query.Get("machine_id")
	page := parseIntParam(query.Get("page"), 1)
	pageSize := parseIntParam(query.Get("page_size"), 20)
	
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	// 构建查询
	db := database.DB.Model(&models.HubDownloadRecord{})
	if machineID != "" {
		db = db.Where("machine_id = ?", machineID)
	}

	// 获取总数
	var total int64
	db.Count(&total)

	// 获取记录
	var records []models.HubDownloadRecord
	if err := db.Order("download_time desc").
		Offset(offset).
		Limit(pageSize).
		Find(&records).Error; err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Failed to get download records",
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"records": records,
			"total":   total,
			"page":    page,
			"size":    pageSize,
		},
	})
}

// parseIntParam 解析整数参数
func parseIntParam(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return value
}
