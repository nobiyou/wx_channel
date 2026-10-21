package handlers

import (
	"net/http"
	"path"
	"strings"

	"wx_channel/internal/utils"
	"wx_channel/web"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// StaticFileHandler 处理静态文件请求
type StaticFileHandler struct {
}

// NewStaticFileHandler 创建静态文件处理器
func NewStaticFileHandler() *StaticFileHandler {
	return &StaticFileHandler{}
}

// readWebFile 从内嵌资源读取文件，name 为相对 web/ 的路径
func readWebFile(name string) ([]byte, bool) {
	clean := path.Clean("/" + name)
	if strings.Contains(clean, "..") {
		return nil, false
	}
	data, err := web.FS.ReadFile(strings.TrimPrefix(clean, "/"))
	if err != nil {
		return nil, false
	}
	return data, true
}

// contentTypeByExt 按扩展名返回 Content-Type
func contentTypeByExt(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".md"):
		return "text/plain; charset=utf-8"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// Handle implements router.Interceptor
func (h *StaticFileHandler) Handle(Conn *SunnyNet.HttpConn) bool {

	if Conn.Request == nil || Conn.Request.URL == nil {
		return false
	}
	path := Conn.Request.URL.Path

	// 1. 处理控制台页面（资源内嵌于二进制，与工作目录无关）
	if path == "/console" || path == "/console/" {
		consoleHTML, ok := readWebFile("console.html")
		if !ok {
			utils.Warn("无法读取内嵌控制台资源 console.html")
			Conn.StopRequest(404, "Console not found", http.Header{})
			return true
		}
		headers := http.Header{}
		headers.Set("Content-Type", "text/html; charset=utf-8")
		Conn.StopRequest(200, string(consoleHTML), headers)
		return true
	}

	// 2. 检查是否为微信资源（排除）
	isWeixinResource := strings.Contains(path, "pic_blank.gif") ||
		strings.Contains(path, "we-emoji") ||
		strings.Contains(path, "Expression") ||
		strings.Contains(path, "auth_icon") ||
		strings.Contains(path, "weixin/checkresupdate") ||
		strings.Contains(path, "fed_upload") ||
		strings.HasPrefix(path, "/a/") ||
		strings.HasPrefix(path, "/weixin/")

	if isWeixinResource {
		return false
	}

	// 3. 处理静态资源文件（内嵌资源）
	if strings.HasPrefix(path, "/js/") || strings.HasPrefix(path, "/css/") || strings.HasPrefix(path, "/docs/") ||
		strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpg") ||
		strings.HasSuffix(path, ".jpeg") || strings.HasSuffix(path, ".gif") ||
		strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".ico") ||
		strings.HasSuffix(path, ".md") {

		content, ok := readWebFile(strings.TrimPrefix(path, "/"))
		if !ok {
			// 内嵌资源中没有，交给后续处理或透传
			return false
		}

		headers := http.Header{}
		headers.Set("Content-Type", contentTypeByExt(path))

		Conn.StopRequest(200, string(content), headers)
		return true
	}

	return false
}
