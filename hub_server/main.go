package main

import (
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"wx_channel/hub_server/controllers"
	"wx_channel/hub_server/database"
	"wx_channel/hub_server/middleware"
	"wx_channel/hub_server/services"
	"wx_channel/hub_server/ws"

	"github.com/gorilla/mux"
)

// recoveryMiddleware 全局 panic 恢复中间件
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v\nStack: %s", err, string(debug.Stack()))
				http.Error(w, "Internal Server Error", 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// authMiddleware 认证中间件（适配 gorilla/mux）
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		middleware.AuthRequired(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})(w, r)
	})
}

// adminMiddleware 管理员权限中间件
func adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		middleware.AdminRequired(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})(w, r)
	})
}

func main() {
	if err := middleware.InitJWTSecretFromEnv(); err != nil {
		log.Fatalf("Invalid JWT secret configuration: %v", err)
	}

	// 1. 初始化数据库
	if err := database.InitDB("hub_server.db"); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}

	// 2. 初始化 WebSocket Hub
	hub := ws.NewHub()
	go hub.Run()

	// 2.5 启动积分矿工服务 (在线时长统计)
	services.StartMiningService()

	// 2.6 启动同步服务
	services.InitSyncService(services.SyncConfig{
		Enabled:    true,
		Interval:   5 * 60 * 1000000000, // 5 minutes
		Token:      "",                   // 从环境变量或配置文件读取
		MaxRetries: 3,
		Timeout:    30 * 1000000000, // 30 seconds
		BatchSize:  1000,
	})

	// 2.7 初始化 API 指标采集
	metricsStore := middleware.InitMetricsStore()

	// 3. 创建路由器（全局 panic recovery + 指标采集）
	router := mux.NewRouter()
	router.Use(recoveryMiddleware)
	router.Use(middleware.MetricsMiddleware(metricsStore))

	// WebSocket 接入点（无需认证）
	router.HandleFunc("/ws/client", hub.ServeWs).Methods("GET")

	// ─── 公开 API ───
	router.HandleFunc("/api/auth/register", controllers.Register).Methods("POST")
	router.HandleFunc("/api/auth/login", controllers.Login).Methods("POST")
	router.HandleFunc("/api/video/play", controllers.PlayVideo).Methods("GET")

	// ─── 需要认证的 API（Auth Subrouter）───
	auth := router.PathPrefix("").Subrouter()
	auth.Use(authMiddleware)

	auth.HandleFunc("/api/auth/profile", controllers.GetProfile).Methods("GET")
	auth.HandleFunc("/api/user/change-password", controllers.ChangePassword).Methods("POST")
	auth.HandleFunc("/api/user/stats", controllers.GetUserStats).Methods("GET")
	auth.HandleFunc("/api/user/transactions", controllers.GetTransactions).Methods("GET")

	// Device
	auth.HandleFunc("/api/device/bind_token", controllers.GenerateBindToken).Methods("POST")
	auth.HandleFunc("/api/device/list", controllers.GetUserDevices).Methods("GET")
	auth.HandleFunc("/api/device/unbind", controllers.UnbindDevice).Methods("POST")
	auth.HandleFunc("/api/device/delete", controllers.DeleteDevice).Methods("POST")
	auth.HandleFunc("/api/device/rename", controllers.RenameDevice).Methods("POST")
	auth.HandleFunc("/api/device/lock", controllers.LockDevice).Methods("POST")
	auth.HandleFunc("/api/device/group", controllers.SetDeviceGroup).Methods("POST")
	auth.HandleFunc("/api/device/transfer", controllers.TransferDevice).Methods("POST")
	auth.HandleFunc("/api/device/config", controllers.UpdateDeviceConfig).Methods("POST")

	// Subscription
	auth.HandleFunc("/api/subscriptions", controllers.CreateSubscription).Methods("POST")
	auth.HandleFunc("/api/subscriptions", controllers.GetSubscriptions).Methods("GET")
	auth.HandleFunc("/api/subscriptions/{id}/fetch", controllers.FetchVideos(hub)).Methods("POST")
	auth.HandleFunc("/api/subscriptions/{id}/videos", controllers.GetSubscriptionVideos).Methods("GET")
	auth.HandleFunc("/api/subscriptions/{id}", controllers.DeleteSubscription).Methods("DELETE")

	// Task & Remote Call
	auth.HandleFunc("/api/clients", controllers.GetNodes).Methods("GET")
	auth.HandleFunc("/api/tasks", controllers.GetTasks).Methods("GET")
	auth.HandleFunc("/api/tasks/detail", controllers.GetTaskDetail).Methods("GET")
	auth.HandleFunc("/api/remoteCall", controllers.RemoteCall(hub)).Methods("POST")
	auth.HandleFunc("/api/call", controllers.RemoteCall(hub)).Methods("POST")

	// Metrics & WS Stats
	auth.HandleFunc("/api/metrics/summary", controllers.GetMetricsSummary).Methods("GET")
	auth.HandleFunc("/api/metrics/timeseries", controllers.GetTimeSeriesData).Methods("GET")
	auth.HandleFunc("/api/ws/stats", controllers.GetWSStats(hub)).Methods("GET")

	// Sync Management
	auth.HandleFunc("/api/sync/status", controllers.GetSyncStatus).Methods("GET")
	auth.HandleFunc("/api/sync/status/{machine_id}", controllers.GetDeviceSyncStatus).Methods("GET")
	auth.HandleFunc("/api/sync/trigger", controllers.TriggerSync).Methods("POST")
	auth.HandleFunc("/api/sync/history/{machine_id}", controllers.GetSyncHistory).Methods("GET")
	auth.HandleFunc("/api/sync/browse", controllers.GetBrowseRecords).Methods("GET")
	auth.HandleFunc("/api/sync/download", controllers.GetDownloadRecords).Methods("GET")

	// ─── 管理员 API（Admin Subrouter）───
	admin := auth.PathPrefix("/api/admin").Subrouter()
	admin.Use(adminMiddleware)

	admin.HandleFunc("/users", controllers.GetUserList).Methods("GET")
	admin.HandleFunc("/stats", controllers.GetStats).Methods("GET")
	admin.HandleFunc("/user/credits", controllers.UpdateUserCredits).Methods("POST")
	admin.HandleFunc("/user/role", controllers.UpdateUserRole).Methods("POST")
	admin.HandleFunc("/user/{id}", controllers.DeleteUser).Methods("DELETE")
	admin.HandleFunc("/devices", controllers.GetAllDevices).Methods("GET")
	admin.HandleFunc("/device/unbind", controllers.AdminUnbindDevice).Methods("POST")
	admin.HandleFunc("/device/{id}", controllers.AdminDeleteDevice).Methods("DELETE")
	admin.HandleFunc("/tasks", controllers.GetAllTasks).Methods("GET")
	admin.HandleFunc("/task/{id}", controllers.AdminDeleteTask).Methods("DELETE")
	admin.HandleFunc("/subscriptions", controllers.GetAllSubscriptions).Methods("GET")
	admin.HandleFunc("/subscription/{id}", controllers.AdminDeleteSubscription).Methods("DELETE")

	// ─── 静态文件服务 - Vue SPA 支持 ───
	fs := http.FileServer(http.Dir("frontend/dist"))
	router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 如果是 API 调用或 WebSocket，不处理
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.NotFound(w, r)
			return
		}

		path := r.URL.Path
		// 检查文件是否存在于 dist 目录
		if _, err := os.Stat("frontend/dist" + path); os.IsNotExist(err) {
			// 文件不存在，返回 index.html (SPA History Mode)
			http.ServeFile(w, r, "frontend/dist/index.html")
			return
		}

		// 文件存在，直接服务
		fs.ServeHTTP(w, r)
	})

	log.Println("Hub Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}
