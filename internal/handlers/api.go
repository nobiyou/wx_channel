package handlers

import (
	"net/http"
	"strings"

	"wx_channel/internal/config"
	"wx_channel/internal/response"
	"wx_channel/internal/utils"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// APIHandler API请求处理器
type APIHandler struct {
	cfg        *config.Config
	currentURL string
}

// NewAPIHandler 创建API处理器
func NewAPIHandler(cfg *config.Config) *APIHandler {
	return &APIHandler{
		cfg: cfg,
	}
}

// getConfig 获取当前配置
func (h *APIHandler) getConfig() *config.Config {
	if h.cfg != nil {
		return h.cfg
	}
	return config.Get()
}

// SetCurrentURL 设置当前页面URL
func (h *APIHandler) SetCurrentURL(url string) {
	h.currentURL = url
}

// GetCurrentURL 获取当前页面URL
func (h *APIHandler) GetCurrentURL() string {
	return h.currentURL
}

// Handle implements router.Interceptor
func (h *APIHandler) Handle(Conn *SunnyNet.HttpConn) bool {
	// CORS Preflight for all __wx_channels_api requests
	if Conn.Request == nil || Conn.Request.URL == nil {
		return false
	}

	// Add local panic recovery
	defer func() {
		if r := recover(); r != nil {
			utils.Error("APIHandler.Handle panic: %v", r)
		}
	}()

	if strings.HasPrefix(Conn.Request.URL.Path, "/__wx_channels_api/") && Conn.Request.Method == "OPTIONS" {
		h.handleCORS(Conn)
		return true
	}

	if h.HandleProfile(Conn) {
		return true
	}
	if h.HandleTip(Conn) {
		return true
	}
	if h.HandleInjectHealth(Conn) {
		return true
	}
	if h.HandlePageURL(Conn) {
		// HandlePageURL updates state alongside returning true
		return true
	}
	if h.HandleSavePageContent(Conn) {
		return true
	}
	return false
}

// handleCORS 处理CORS预检请求
func (h *APIHandler) handleCORS(Conn *SunnyNet.HttpConn) {
	headers := http.Header{}
	headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
	if h.getConfig() != nil && len(h.getConfig().AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		for _, o := range h.getConfig().AllowedOrigins {
			if o == origin {
				headers.Set("Access-Control-Allow-Origin", origin)
				headers.Set("Vary", "Origin")
				break
			}
		}
	}
	Conn.StopRequest(204, "", headers)
}

// sendEmptyResponse 发送空JSON响应
func (h *APIHandler) sendEmptyResponse(Conn *SunnyNet.HttpConn) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	if h.getConfig() != nil && len(h.getConfig().AllowedOrigins) > 0 {
		origin := Conn.Request.Header.Get("Origin")
		if origin != "" {
			for _, o := range h.getConfig().AllowedOrigins {
				if o == origin {
					headers.Set("Access-Control-Allow-Origin", origin)
					headers.Set("Vary", "Origin")
					break
				}
			}
		}
	}
	Conn.StopRequest(200, string(response.SuccessJSON(nil)), headers)
}

// sendErrorResponse 发送错误响应
func (h *APIHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
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
					break
				}
			}
		}
	}
	// 记录错误但不中断流程（实际响应错误给客户端）
	utils.LogError("API Error: %v", err)
	Conn.StopRequest(500, string(response.ErrorJSON(500, err.Error())), headers)
}
