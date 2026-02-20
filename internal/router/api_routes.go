package router

import (
	"net/http"

	"wx_channel/internal/api"
	"wx_channel/internal/config"
	"wx_channel/internal/handlers"
	"wx_channel/internal/websocket"

	"strings"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// APIRouter 管理所有 API 路由
type APIRouter struct {
	mux                *http.ServeMux
	consoleHandler     *handlers.ConsoleAPIHandler
	searchService      *api.SearchService
	systemService      *api.SystemService
	logsService        *api.LogsService
	exportService      *api.ExportAPI
	proxyService       *api.ProxyService
	certificateService *api.CertificateService
	versionService     *api.VersionAPI
	allowedOrigins     []string
	secretToken        string
}

// Handle implements Interceptor
func (r *APIRouter) Handle(Conn *SunnyNet.HttpConn) bool {

	// 防御性检查
	if r == nil {
		return false
	}
	if Conn == nil || Conn.Request == nil || Conn.Request.URL == nil {
		return false
	}

	// 仅处理 /api/ 请求
	if !strings.HasPrefix(Conn.Request.URL.Path, "/api/") {
		return false
	}

	w := NewSunnyNetResponseWriter(Conn)
	r.ServeHTTP(w, Conn.Request)
	w.Flush()
	return true
}

// NewAPIRouter 创建 API 路由器
func NewAPIRouter(cfg *config.Config, hub *websocket.Hub, sunny *SunnyNet.Sunny) *APIRouter {
	mux := http.NewServeMux()

	router := &APIRouter{
		mux:                mux,
		consoleHandler:     handlers.NewConsoleAPIHandler(cfg, hub),
		searchService:      api.NewSearchService(hub),
		systemService:      api.NewSystemService(),
		logsService:        api.NewLogsService(cfg),
		exportService:      api.NewExportAPI(),
		proxyService:       api.NewProxyService(sunny, cfg.Port),
		certificateService: api.NewCertificateService(sunny),
		versionService:     api.NewVersionAPI(),
		allowedOrigins:     cfg.AllowedOrigins,
		secretToken:        cfg.SecretToken,
	}

	router.registerRoutes()

	return router
}

// registerRoutes 注册所有 API 路由
func (r *APIRouter) registerRoutes() {
	// 搜索 API (v1)
	r.searchService.RegisterRoutes(r.mux)

	// 系统管理 API (v1)
	// 系统管理
	r.systemService.RegisterRoutes(r.mux)
	r.logsService.RegisterRoutes(r.mux)
	r.proxyService.RegisterRoutes(r.mux)
	r.certificateService.RegisterRoutes(r.mux)
	r.versionService.RegisterRoutes(r.mux)

	// 控制台 API - 浏览历史
	r.mux.HandleFunc("/api/browse", r.consoleHandler.HandleBrowseAPI)
	r.mux.HandleFunc("/api/browse/", r.consoleHandler.HandleBrowseAPI)

	// 控制台 API - 下载记录
	r.mux.HandleFunc("/api/downloads", r.consoleHandler.HandleDownloadsAPI)
	r.mux.HandleFunc("/api/downloads/", r.consoleHandler.HandleDownloadsAPI)

	// 控制台 API - 队列管理
	r.mux.HandleFunc("/api/queue", r.consoleHandler.HandleQueueAPI)
	r.mux.HandleFunc("/api/queue/", r.consoleHandler.HandleQueueAPI)

	// Console API - Settings
	// 设置管理
	r.mux.HandleFunc("/api/settings", r.consoleHandler.HandleSettingsAPI)

	// 健康检查
	r.mux.HandleFunc("/api/health", r.consoleHandler.HandleHealth)

	// 统计信息
	r.mux.HandleFunc("/api/stats", r.consoleHandler.HandleStatsAPI)
	r.mux.HandleFunc("/api/stats/", r.consoleHandler.HandleStatsAPI)

	// 搜索
	r.mux.HandleFunc("/api/search", r.consoleHandler.HandleSearch)

	// 文件操作
	r.mux.HandleFunc("/api/files/", r.consoleHandler.HandleFilesAPI)

	// 系统信息

	// 控制台 API - 导出功能
	r.mux.HandleFunc("/api/export/browse", r.exportService.HandleExportBrowseHistory)
	r.mux.HandleFunc("/api/export/downloads", r.exportService.HandleExportDownloadRecords)

	// 控制台 API - 视频相关
	r.mux.HandleFunc("/api/video/stream", r.consoleHandler.HandleVideoStream)
	r.mux.HandleFunc("/api/video/play", r.consoleHandler.HandleVideoPlay)

	// 控制台 API - 触发评论采集
	r.mux.HandleFunc("/api/control/comment/start", r.consoleHandler.HandleStartCommentCollection)

	// Hub 同步 API - 供 Hub Server 拉取数据
	syncHandler := handlers.NewSyncAPIHandler()
	r.mux.HandleFunc("/api/sync/browse", syncHandler.HandleGetBrowseHistory)
	r.mux.HandleFunc("/api/sync/download", syncHandler.HandleGetDownloadRecords)
	r.mux.HandleFunc("/api/sync/stats", syncHandler.HandleGetStats)

	// v1 版本化路由（别名）
	r.mux.HandleFunc("/api/v1/browse", r.consoleHandler.HandleBrowseAPI)
	r.mux.HandleFunc("/api/v1/browse/", r.consoleHandler.HandleBrowseAPI)
	r.mux.HandleFunc("/api/v1/downloads", r.consoleHandler.HandleDownloadsAPI)
	r.mux.HandleFunc("/api/v1/downloads/", r.consoleHandler.HandleDownloadsAPI)
	r.mux.HandleFunc("/api/v1/queue", r.consoleHandler.HandleQueueAPI)
	r.mux.HandleFunc("/api/v1/queue/", r.consoleHandler.HandleQueueAPI)
	r.mux.HandleFunc("/api/v1/settings", r.consoleHandler.HandleSettingsAPI)
	r.mux.HandleFunc("/api/v1/stats", r.consoleHandler.HandleStatsAPI)
	r.mux.HandleFunc("/api/v1/stats/", r.consoleHandler.HandleStatsAPI)
}

// Handler 返回带中间件的 HTTP Handler
func (r *APIRouter) Handler() http.Handler {
	// 应用中间件链
	return Chain(
		r.mux,
		RecoveryMiddleware,
		LoggerMiddleware,
		CORSMiddleware(r.allowedOrigins),
		AuthMiddleware(r.secretToken),
	)
}

// ServeHTTP 实现 http.Handler 接口
func (r *APIRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.Handler().ServeHTTP(w, req)
}
