package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"wx_channel/hub_server/database"
	"wx_channel/hub_server/middleware"
	"wx_channel/hub_server/models"
	"wx_channel/hub_server/ws"
	"wx_channel/internal/services"
)

type hubSharedFeedService interface {
	Enabled() bool
	FetchVideoProfile(ctx context.Context, shareURL string) (*services.SphFeedResponse, error)
}

type sharedFeedProfileRequest struct {
	URL string `json:"url"`
}

var newHubSharedFeedService = func() hubSharedFeedService {
	return services.NewSphServiceWithConfigProvider(loadHubSphConfig)
}

var fetchSharedFeedProfileViaPage = defaultFetchSharedFeedProfileViaPage

func ParseSph(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}

	if r.Method == http.MethodGet {
		req.URL = r.URL.Query().Get("url")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	service := services.NewSphServiceWithConfigProvider(loadHubSphConfig)

	if !service.Enabled() {
		http.Error(w, "sph settings not configured", http.StatusBadRequest)
		return
	}

	resp, err := service.FetchVideoProfile(r.Context(), req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    resp,
	})
}

func GetSharedFeedProfile(w http.ResponseWriter, r *http.Request) {
	handleSharedFeedProfile(nil, w, r)
}

func GetSharedFeedProfileHandler(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleSharedFeedProfile(hub, w, r)
	}
}

func handleSharedFeedProfile(hub *ws.Hub, w http.ResponseWriter, r *http.Request) {
	req, err := decodeSharedFeedProfileRequest(r)
	if err != nil {
		writeSharedFeedProfileError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.URL = normalizeSharedFeedProfileURL(req.URL)
	if req.URL == "" {
		writeSharedFeedProfileError(w, http.StatusBadRequest, "url is required")
		return
	}

	service := newHubSharedFeedService()
	if service != nil && service.Enabled() {
		resp, err := service.FetchVideoProfile(r.Context(), req.URL)
		if err == nil {
			writeSharedFeedProfileSuccess(w, services.BuildSharedFeedProfileCompatResponse(resp))
			return
		}

		log.Printf("[shared_feed_profile] backend parse failed, fallback to page API: %v", err)
	}

	userID := getHubUserID(r)
	result, err := fetchSharedFeedProfileViaPage(r.Context(), hub, userID, req.URL)
	if err != nil {
		if isNoReadySharedFeedPageError(err) {
			writeSharedFeedProfileError(w, http.StatusServiceUnavailable, "No ready WeChat page is available for shared feed profile.")
			return
		}
		writeSharedFeedProfileError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSharedFeedProfileSuccess(w, result)
}

func GetSphSettings(w http.ResponseWriter, r *http.Request) {
	cfg := loadHubSphConfig()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"enabled":           strings.TrimSpace(cfg.SphCookie) != "" || strings.TrimSpace(cfg.SphHostname) != "",
			"hasCookie":         strings.TrimSpace(cfg.SphCookie) != "",
			"cookieMasked":      maskSecret(cfg.SphCookie),
			"hostname":          cfg.SphHostname,
			"sourceFallbackEnv": strings.TrimSpace(os.Getenv("HUB_SPH_COOKIE")) != "" || strings.TrimSpace(os.Getenv("HUB_SPH_HOSTNAME")) != "",
		},
	})
}

func UpdateSphSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled  bool   `json:"enabled"`
		Cookie   string `json:"cookie"`
		Hostname string `json:"hostname"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	cookie := strings.TrimSpace(req.Cookie)
	hostname := strings.TrimSpace(req.Hostname)

	if !req.Enabled {
		cookie = ""
		hostname = ""
	}

	if err := database.SetSetting("sph.enabled", boolString(req.Enabled)); err != nil {
		http.Error(w, "Failed to save sph.enabled", http.StatusInternalServerError)
		return
	}
	if err := database.SetSetting("sph.cookie", cookie); err != nil {
		http.Error(w, "Failed to save sph.cookie", http.StatusInternalServerError)
		return
	}
	if err := database.SetSetting("sph.hostname", hostname); err != nil {
		http.Error(w, "Failed to save sph.hostname", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "Settings updated successfully",
	})
}

func decodeSharedFeedProfileRequest(r *http.Request) (sharedFeedProfileRequest, error) {
	var req sharedFeedProfileRequest

	if r.Method == http.MethodGet {
		req.URL = r.URL.Query().Get("url")
		return req, nil
	}

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return req, err
		}
	}

	return req, nil
}

func normalizeSharedFeedProfileURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	decoded, err := url.QueryUnescape(raw)
	if err == nil {
		return decoded
	}

	return raw
}

func defaultFetchSharedFeedProfileViaPage(ctx context.Context, hub *ws.Hub, userID uint, shareURL string) (interface{}, error) {
	if hub == nil {
		return nil, errors.New("no ready client")
	}

	clientIDs := collectSharedFeedProfileClientIDs(userID)
	if len(clientIDs) == 0 {
		return nil, errors.New("no ready client")
	}

	var lastErr error
	payload := map[string]interface{}{
		"key": "key:channels:shared_feed_profile",
		"body": map[string]interface{}{
			"objectId": "",
			"nonceId":  "",
			"url":      shareURL,
		},
	}

	for _, clientID := range clientIDs {
		resp, err := hub.Call(userID, clientID, "api_call", payload, 60*time.Second)
		if err != nil {
			lastErr = err
			continue
		}

		if !resp.Success {
			if msg := strings.TrimSpace(resp.Error); msg != "" {
				lastErr = errors.New(msg)
			} else {
				lastErr = errors.New("shared feed profile page call failed")
			}
			continue
		}

		var result interface{}
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			raw := append([]byte(nil), resp.Data...)
			return json.RawMessage(raw), nil
		}

		return result, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, errors.New("no ready client")
}

func collectSharedFeedProfileClientIDs(userID uint) []string {
	seen := map[string]struct{}{}
	var clientIDs []string

	appendNode := func(node models.Node) {
		if node.ID == "" || node.Status != "online" {
			return
		}
		if _, ok := seen[node.ID]; ok {
			return
		}
		seen[node.ID] = struct{}{}
		clientIDs = append(clientIDs, node.ID)
	}

	if userID != 0 {
		user, err := database.GetUserByID(userID)
		if err == nil {
			for _, device := range user.Devices {
				if device.Status == "online" && nodeSupportsCapability(device, "profile") {
					appendNode(device)
				}
			}
			for _, device := range user.Devices {
				if device.Status == "online" {
					appendNode(device)
				}
			}
		}
	}

	var nodes []models.Node
	if database.DB != nil {
		if err := database.DB.Where("status = ? AND supports_profile = ?", "online", true).Find(&nodes).Error; err == nil {
			for _, node := range nodes {
				appendNode(node)
			}
		}

		nodes = nil
		if err := database.DB.Where("status = ?", "online").Find(&nodes).Error; err == nil {
			for _, node := range nodes {
				appendNode(node)
			}
		}
	}

	return clientIDs
}

func getHubUserID(r *http.Request) uint {
	userID, _ := r.Context().Value(middleware.ContextKeyUserID).(uint)
	return userID
}

func isNoReadySharedFeedPageError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "no ready client") ||
		strings.Contains(msg, "no available client") ||
		strings.Contains(msg, "no online device") ||
		strings.Contains(msg, "no device found") ||
		strings.Contains(msg, "client offline")
}

func writeSharedFeedProfileSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    data,
	})
}

func writeSharedFeedProfileError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    status,
		"message": message,
	})
}

func loadHubSphConfig() services.SphServiceConfig {
	cookie, _ := database.GetSetting("sph.cookie")
	hostname, _ := database.GetSetting("sph.hostname")
	enabledRaw, _ := database.GetSetting("sph.enabled")
	enabled := parseBoolString(enabledRaw)

	cfg := services.SphServiceConfig{
		SphHostname: strings.TrimSpace(hostname),
		SphCookie:   strings.TrimSpace(cookie),
	}

	if enabled {
		if cfg.SphCookie == "" {
			cfg.SphCookie = strings.TrimSpace(os.Getenv("HUB_SPH_COOKIE"))
		}
		if cfg.SphHostname == "" {
			cfg.SphHostname = strings.TrimSpace(os.Getenv("HUB_SPH_HOSTNAME"))
		}
		return cfg
	}

	if cfg.SphCookie == "" && cfg.SphHostname == "" {
		return services.SphServiceConfig{
			SphHostname: strings.TrimSpace(os.Getenv("HUB_SPH_HOSTNAME")),
			SphCookie:   strings.TrimSpace(os.Getenv("HUB_SPH_COOKIE")),
		}
	}

	return services.SphServiceConfig{}
}

func maskSecret(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 8 {
		return "********"
	}
	return raw[:4] + "********" + raw[len(raw)-4:]
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func parseBoolString(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
