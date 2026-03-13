package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"wx_channel/hub_server/database"
	"wx_channel/hub_server/middleware"
	"wx_channel/hub_server/models"
	"wx_channel/hub_server/ws"

	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

// CreateSubscription 订阅一个视频号用户
func CreateSubscription(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	var req struct {
		WxUsername  string `json:"wx_username"`
		WxNickname  string `json:"wx_nickname"`
		WxHeadUrl   string `json:"wx_head_url"`
		WxSignature string `json:"wx_signature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.WxUsername == "" {
		http.Error(w, "wx_username is required", http.StatusBadRequest)
		return
	}

	// Check if already subscribed
	var existing models.Subscription
	err := database.DB.Where("user_id = ? AND wx_username = ?", userID, req.WxUsername).First(&existing).Error
	if err == nil {
		// Already subscribed
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "Already subscribed",
			"data":    existing,
		})
		return
	}

	// Create new subscription
	subscription := models.Subscription{
		UserID:      userID,
		WxUsername:  req.WxUsername,
		WxNickname:  req.WxNickname,
		WxHeadUrl:   req.WxHeadUrl,
		WxSignature: req.WxSignature,
		Status:      "active",
	}

	if err := database.DB.Create(&subscription).Error; err != nil {
		http.Error(w, "Failed to create subscription", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "Subscription created",
		"data":    subscription,
	})
}

// GetSubscriptions 获取当前用户的所有订阅
func GetSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)

	var subscriptions []models.Subscription
	if err := database.DB.Where("user_id = ?", userID).Order("created_at desc").Find(&subscriptions).Error; err != nil {
		http.Error(w, "Failed to fetch subscriptions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 0,
		"data": subscriptions,
	})
}

// FetchVideos 获取或更新某个订阅的视频数据
func FetchVideos(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(middleware.ContextKeyUserID).(uint)
		vars := mux.Vars(r)
		subID, err := strconv.ParseUint(vars["id"], 10, 32)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    -1,
				"message": "Invalid subscription ID",
			})
			return
		}

		// Get subscription
		var subscription models.Subscription
		if err := database.DB.Where("id = ? AND user_id = ?", subID, userID).First(&subscription).Error; err != nil {
			w.Header().Set("Content-Type", "application/json")
			if err == gorm.ErrRecordNotFound {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code":    -1,
					"message": "Subscription not found",
				})
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code":    -1,
					"message": "Database error",
				})
			}
			return
		}

		// Get current user's devices to find an active client
		// If no device bound to user, try to find any online device
		var clientID string

		user, err := database.GetUserByID(userID)
		if err == nil && len(user.Devices) > 0 {
			// Use first online device bound to user
			for _, device := range user.Devices {
				if device.Status == "online" {
					clientID = device.ID
					break
				}
			}
		}

		// If no bound device found, try to find any online device (auto-select)
		if clientID == "" {
			// Get all online nodes
			var nodes []models.Node
			if err := database.DB.Where("status = ?", "online").Find(&nodes).Error; err == nil && len(nodes) > 0 {
				clientID = nodes[0].ID
			}
		}

		if clientID == "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    -1,
				"message": "No online device found",
			})
			return
		}

		// Fetch videos with pagination (fetch all pages until no more data)
		allVideos := []map[string]interface{}{}
		nextMarker := ""
		maxPages := 50 // Increased limit to fetch more videos

		for page := 0; page < maxPages; page++ {
			payload := map[string]interface{}{
				"username":    subscription.WxUsername,
				"next_marker": nextMarker,
			}

			response, err := hub.Call(userID, clientID, "api_call", map[string]interface{}{
				"key":  "key:channels:feed_list",
				"body": payload,
			}, 30*time.Second)

			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code":    -1,
					"message": fmt.Sprintf("Failed to fetch videos: %v", err),
				})
				return
			}

			// Parse response data
			var responseData map[string]interface{}
			if err := json.Unmarshal(response.Data, &responseData); err != nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code":    -1,
					"message": "Failed to parse response",
				})
				return
			}

			// Extract data field
			data, ok := responseData["data"].(map[string]interface{})
			if !ok {
				// No data field, stop pagination
				break
			}

			// Extract video list - try both "object" and "list" fields
			var videos []map[string]interface{}
			if objects, ok := data["object"].([]interface{}); ok {
				for _, obj := range objects {
					if videoMap, ok := obj.(map[string]interface{}); ok {
						videos = append(videos, videoMap)
					}
				}
			} else if list, ok := data["list"].([]interface{}); ok {
				for _, obj := range list {
					if videoMap, ok := obj.(map[string]interface{}); ok {
						videos = append(videos, videoMap)
					}
				}
			}

			// If no videos in this page, stop pagination
			if len(videos) == 0 {
				fmt.Printf("[Subscription] Page %d: No videos, stopping pagination\n", page)
				break
			}

			fmt.Printf("[Subscription] Page %d: Found %d videos, total so far: %d\n", page, len(videos), len(allVideos)+len(videos))
			allVideos = append(allVideos, videos...)

			// Check for next page marker - try multiple locations
			var nextMarkerStr string

			// Method 1: Check data.payload.lastBuffer
			if payload_data, ok := data["payload"].(map[string]interface{}); ok {
				if marker, ok := payload_data["lastBuffer"].(string); ok && marker != "" {
					nextMarkerStr = marker
				}
			}

			// Method 2: Check data.lastBuffer (direct)
			if nextMarkerStr == "" {
				if marker, ok := data["lastBuffer"].(string); ok && marker != "" {
					nextMarkerStr = marker
				}
			}

			// Method 3: Check response continueFlag
			if nextMarkerStr == "" {
				if continueFlag, ok := data["continueFlag"].(float64); ok && continueFlag == 0 {
					// continueFlag = 0 means no more data
					fmt.Printf("[Subscription] Page %d: continueFlag=0, stopping pagination\n", page)
					break
				}
			}

			if nextMarkerStr == "" {
				// No more pages
				fmt.Printf("[Subscription] Page %d: No nextMarker found, stopping pagination\n", page)
				break
			}

			nextMarker = nextMarkerStr
			fmt.Printf("[Subscription] Page %d: Next marker: %s\n", page, nextMarker)
		}

		fmt.Printf("[Subscription] Total videos fetched: %d\n", len(allVideos))

		// Process and save videos to database
		newCount := 0
		for _, videoData := range allVideos {
			// Extract the actual video object (might be wrapped in "object" field)
			actualVideo := videoData
			if objectField, ok := videoData["object"].(map[string]interface{}); ok {
				actualVideo = objectField
			}

			objectID := getStringField(actualVideo, "id")
			if objectID == "" {
				objectID = getStringField(actualVideo, "objectId")
			}
			if objectID == "" {
				objectID = getStringField(actualVideo, "displayid")
			}
			if objectID == "" {
				continue
			}

			// Check if video already exists
			var existing models.SubscribedVideo
			existsErr := database.DB.Where("subscription_id = ? AND object_id = ?", subscription.ID, objectID).First(&existing).Error
			
			videoExists := existsErr == nil
			shouldUpdate := false
			
			// 如果视频已存在，检查是否需要更新
			if videoExists {
				// 检查 URL 是否完整（完整的 URL 通常超过 500 字符）
				if len(existing.VideoURL) > 500 {
					// URL 完整，跳过
					continue
				}
				// URL 不完整，需要更新
				shouldUpdate = true
				fmt.Printf("[Subscription] Updating incomplete video: %s (URL length: %d)\n", objectID, len(existing.VideoURL))
			}

			// Extract video information
			desc := getMapField(actualVideo, "objectDesc")
			if desc == nil {
				desc = getMapField(actualVideo, "desc")
			}

			// Extract media information
			media := []map[string]interface{}{}
			if desc != nil {
				if mediaList, ok := desc["media"].([]interface{}); ok {
					for _, m := range mediaList {
						if mediaMap, ok := m.(map[string]interface{}); ok {
							media = append(media, mediaMap)
						}
					}
				}
			}

			var firstMedia map[string]interface{}
			if len(media) > 0 {
				firstMedia = media[0]
			}

			// Extract cover URL with fallback
			coverURL := getStringField(actualVideo, "coverUrl")
			if coverURL == "" && firstMedia != nil {
				coverURL = getStringField(firstMedia, "thumbUrl")
			}

			// Extract duration with fallback
			duration := getIntField(actualVideo, "videoPlayLen")
			if duration == 0 && firstMedia != nil {
				duration = getIntField(firstMedia, "videoPlayLen")
			}

			// Extract create time - could be "createtime" or "createTime"
			createTime := getIntField(actualVideo, "createTime")
			if createTime == 0 {
				createTime = getIntField(actualVideo, "createtime")
			}

			// Create new video record
			// 构建完整的视频 URL（包含 urlToken）
			videoURL := getStringField(firstMedia, "url")
			urlToken := getStringField(firstMedia, "urlToken")
			if videoURL != "" && urlToken != "" {
				videoURL = videoURL + urlToken
				fmt.Printf("[Subscription] Video URL with token: %s (length: %d)\n", videoURL[:50], len(videoURL))
			} else {
				fmt.Printf("[Subscription] Warning: Missing URL or token for video %s\n", objectID)
			}
			
			video := models.SubscribedVideo{
				SubscriptionID: subscription.ID,
				ObjectID:       objectID,
				ObjectNonceID:  getStringField(actualVideo, "objectNonceId"),
				Title:          getStringField(desc, "description"),
				CoverURL:       coverURL,
				Description:    getStringField(desc, "description"),
				Duration:       duration,
				Width:          getIntField(firstMedia, "width"),
				Height:         getIntField(firstMedia, "height"),
				LikeCount:      getIntField(actualVideo, "likeCount"),
				CommentCount:   getIntField(actualVideo, "commentCount"),
				VideoURL:       videoURL, // 使用完整的 URL
				DecryptKey:     getStringField(firstMedia, "decodeKey"),
				PublishedAt:    time.Unix(int64(createTime), 0),
			}

			// 如果视频已存在且需要更新，保留原有的 ID 和创建时间
			if shouldUpdate {
				video.ID = existing.ID
				video.CreatedAt = existing.CreatedAt
			}
			
			if err := database.DB.Save(&video).Error; err != nil {
				// Log error but continue
				fmt.Printf("[Subscription] Failed to save/update video %s: %v\n", objectID, err)
				continue
			}
			
			// 只有新视频才计入 newCount
			if !videoExists {
				newCount++
			}
		}

		// Update subscription metadata
		database.DB.Model(&subscription).Updates(map[string]interface{}{
			"video_count":     gorm.Expr("video_count + ?", newCount),
			"last_fetched_at": time.Now(),
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"new_videos":   newCount,
				"total_videos": subscription.VideoCount + newCount,
			},
		})
	}
}

// GetSubscriptionVideos 获取某个订阅的视频列表
func GetSubscriptionVideos(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)
	vars := mux.Vars(r)
	subID, err := strconv.ParseUint(vars["id"], 10, 32)
	if err != nil {
		http.Error(w, "Invalid subscription ID", http.StatusBadRequest)
		return
	}

	// Pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize := 20

	// Verify subscription belongs to user
	var subscription models.Subscription
	if err := database.DB.Where("id = ? AND user_id = ?", subID, userID).First(&subscription).Error; err != nil {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}

	// Get videos
	var videos []models.SubscribedVideo
	var total int64

	database.DB.Model(&models.SubscribedVideo{}).Where("subscription_id = ?", subID).Count(&total)
	if err := database.DB.Where("subscription_id = ?", subID).
		Order("published_at desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&videos).Error; err != nil {
		http.Error(w, "Failed to fetch videos", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"videos": videos,
			"total":  total,
			"page":   page,
		},
	})
}

// DeleteSubscription 取消订阅
func DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.ContextKeyUserID).(uint)
	vars := mux.Vars(r)
	subID, err := strconv.ParseUint(vars["id"], 10, 32)
	if err != nil {
		http.Error(w, "Invalid subscription ID", http.StatusBadRequest)
		return
	}

	// Verify subscription belongs to user, then delete
	result := database.DB.Where("id = ? AND user_id = ?", subID, userID).Delete(&models.Subscription{})
	if result.Error != nil {
		http.Error(w, "Failed to delete subscription", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}

	// Also delete associated videos
	database.DB.Where("subscription_id = ?", subID).Delete(&models.SubscribedVideo{})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "Subscription deleted",
	})
}

// Helper functions
func getStringField(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getIntField(data map[string]interface{}, key string) int {
	if val, ok := data[key].(float64); ok {
		return int(val)
	}
	if val, ok := data[key].(int); ok {
		return val
	}
	return 0
}

func getMapField(data map[string]interface{}, key string) map[string]interface{} {
	if val, ok := data[key].(map[string]interface{}); ok {
		return val
	}
	return nil
}
