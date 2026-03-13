package controllers

import (
	"encoding/json"
	"net/http"

	"wx_channel/hub_server/database"
	"wx_channel/hub_server/middleware"
	"wx_channel/hub_server/models"
	"wx_channel/hub_server/services"
)

// GenerateBindToken returns a short code for the user to input in the client
func GenerateBindToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	token, err := services.Binder.GenerateToken(userID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}

// GetUserDevices returns all devices bound to the current user
func GetUserDevices(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	user, err := database.GetUserByID(userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    -1,
			"message": "User not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"devices": user.Devices,
	})
}

// GetUserStats returns user statistics (device count, subscription count, etc.)
func GetUserStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	// Get user with devices
	user, err := database.GetUserByID(userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    -1,
			"message": "User not found",
		})
		return
	}

	// Count subscriptions
	var subscriptionCount int64
	database.DB.Model(&models.Subscription{}).Where("user_id = ?", userID).Count(&subscriptionCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"device_count":       len(user.Devices),
			"subscription_count": subscriptionCount,
			"credits":            user.Credits,
		},
	})
}

// UnbindDevice removes the binding between a device and the user
func UnbindDevice(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	// 解析请求参数
	var req struct {
		DeviceID string `json:"device_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// 获取设备信息
	node, err := database.GetNodeByID(req.DeviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// 检查设备是否属于当前用户
	if node.UserID != userID {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// 解绑设备
	if err := database.UnbindNode(req.DeviceID); err != nil {
		http.Error(w, "Failed to unbind device", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Device unbound successfully",
	})
}

// DeleteDevice permanently deletes a device from the database
func DeleteDevice(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	// 解析请求参数
	var req struct {
		DeviceID string `json:"device_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// 获取设备信息
	node, err := database.GetNodeByID(req.DeviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// 检查设备是否属于当前用户
	if node.UserID != userID {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// 删除设备
	if err := database.DeleteNode(req.DeviceID); err != nil {
		http.Error(w, "Failed to delete device", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Device deleted successfully",
	})
}

// RenameDevice updates the display name of a device
func RenameDevice(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	var req struct {
		DeviceID    string `json:"device_id"`
		DisplayName string `json:"display_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" || req.DisplayName == "" {
		http.Error(w, "device_id and display_name are required", http.StatusBadRequest)
		return
	}

	// 获取设备信息
	node, err := database.GetNodeByID(req.DeviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// 检查设备是否属于当前用户
	if node.UserID != userID {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// 更新设备名称
	if err := database.UpdateNodeDisplayName(req.DeviceID, req.DisplayName); err != nil {
		http.Error(w, "Failed to rename device", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Device renamed successfully",
	})
}

// LockDevice locks a device to prevent transfer
func LockDevice(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	var req struct {
		DeviceID string `json:"device_id"`
		IsLocked bool   `json:"is_locked"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// 获取设备信息
	node, err := database.GetNodeByID(req.DeviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// 检查设备是否属于当前用户
	if node.UserID != userID {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// 更新锁定状态
	if err := database.UpdateNodeLockStatus(req.DeviceID, req.IsLocked); err != nil {
		http.Error(w, "Failed to update lock status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Device lock status updated",
	})
}

// SetDeviceGroup sets the group for a device
func SetDeviceGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	var req struct {
		DeviceID    string `json:"device_id"`
		DeviceGroup string `json:"device_group"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// 获取设备信息
	node, err := database.GetNodeByID(req.DeviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// 检查设备是否属于当前用户
	if node.UserID != userID {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// 更新设备分组
	if err := database.UpdateNodeGroup(req.DeviceID, req.DeviceGroup); err != nil {
		http.Error(w, "Failed to update device group", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Device group updated",
	})
}

// TransferDevice transfers a device to another user
func TransferDevice(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	var req struct {
		DeviceID     string `json:"device_id"`
		TargetUserID uint   `json:"target_user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" || req.TargetUserID == 0 {
		http.Error(w, "device_id and target_user_id are required", http.StatusBadRequest)
		return
	}

	// 获取设备信息
	node, err := database.GetNodeByID(req.DeviceID)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// 检查设备是否属于当前用户
	if node.UserID != userID {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// 检查设备是否被锁定
	if node.IsLocked {
		http.Error(w, "Device is locked and cannot be transferred", http.StatusForbidden)
		return
	}

	// 检查目标用户是否存在
	_, err = database.GetUserByID(req.TargetUserID)
	if err != nil {
		http.Error(w, "Target user not found", http.StatusNotFound)
		return
	}

	// 转移设备
	if err := database.TransferNode(req.DeviceID, req.TargetUserID); err != nil {
		http.Error(w, "Failed to transfer device", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Device transferred successfully",
	})
}

// UpdateDeviceConfig 更新设备配置（包括同步 API URL）
func UpdateDeviceConfig(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	var req struct {
		DeviceID   string `json:"device_id"`
		SyncAPIURL string `json:"sync_api_url"`
		Port       int    `json:"port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Invalid request body",
		})
		return
	}

	if req.DeviceID == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "device_id is required",
		})
		return
	}

	// 获取设备并验证所有权
	node, err := database.GetNodeByID(req.DeviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Device not found",
		})
		return
	}

	if node.UserID != userID {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    1,
			"message": "Permission denied",
		})
		return
	}

	// 更新配置
	updates := make(map[string]interface{})
	if req.SyncAPIURL != "" {
		updates["sync_api_url"] = req.SyncAPIURL
	}
	if req.Port > 0 {
		updates["port"] = req.Port
	}

	if len(updates) > 0 {
		if err := database.DB.Model(&models.Node{}).Where("id = ?", req.DeviceID).Updates(updates).Error; err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    1,
				"message": "Failed to update device config",
			})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "Device config updated successfully",
	})
}
