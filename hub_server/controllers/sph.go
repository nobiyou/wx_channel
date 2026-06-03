package controllers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"wx_channel/hub_server/database"
	"wx_channel/internal/services"
)

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
		"data":    services.BuildSharedFeedProfileCompatResponse(resp),
	})
}

func GetSphSettings(w http.ResponseWriter, r *http.Request) {
	cfg := loadHubSphConfig()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"enabled":          strings.TrimSpace(cfg.SphCookie) != "" || strings.TrimSpace(cfg.SphHostname) != "",
			"hasCookie":        strings.TrimSpace(cfg.SphCookie) != "",
			"cookieMasked":     maskSecret(cfg.SphCookie),
			"hostname":         cfg.SphHostname,
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
