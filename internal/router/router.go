package router

import (
	"net/http"
	"strings"
	"time"

	"wx_channel/internal/response"
	"wx_channel/internal/utils"
)

// 重新导出 response 包的函数，保持向后兼容
var (
	Success         = response.Success
	SuccessPaged    = response.SuccessPaged
	Error           = response.Error
	ErrorWithStatus = response.ErrorWithStatus
)

// Response 类型别名
type Response = response.Response
type PagedData = response.PagedData

// LoggerMiddleware 日志中间件
func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 包装 ResponseWriter 以捕获状态码
		wrapped := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		utils.LogAPI(r.Method, r.URL.Path, wrapped.statusCode, duration)
	})
}

// statusResponseWriter 包装 ResponseWriter 以捕获状态码
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// CORSMiddleware 跨域中间件
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// 检查 origin 是否在允许列表中
			allowed := false
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Local-Auth")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}

			// 处理预检请求
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware 基于配置的 token 进行可选认证。
// 当 token 为空时，表示不启用认证。
func AuthMiddleware(secretToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 不启用 token 时直接放行
			if secretToken == "" {
				next.ServeHTTP(w, r)
				return
			}

			// 公共端点放行：用于服务探活和控制台令牌验证
			if isPublicAPIPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// 允许 CORS 预检请求直接通过
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			token := r.Header.Get("X-Local-Auth")
			if token == "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
					token = strings.TrimSpace(auth[len("Bearer "):])
				}
			}
			if token == "" {
				token = r.URL.Query().Get("token")
			}

			if token != secretToken {
				response.ErrorWithStatus(w, http.StatusUnauthorized, 401, "unauthorized")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isPublicAPIPath(path string) bool {
	switch path {
	case "/api/health", "/api/console/verify-token", "/api/system/health", "/api/v1/system/health":
		return true
	default:
		return false
	}
}

// RecoveryMiddleware 异常恢复中间件
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				utils.GetLogger().Error("Panic recovered: %v, path: %s", err, r.URL.Path)
				response.ErrorWithStatus(w, http.StatusInternalServerError, 500, "Internal Server Error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Chain 将多个中间件链接起来
func Chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
