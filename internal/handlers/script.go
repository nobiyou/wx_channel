package handlers

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/officialaccount"
	"wx_channel/internal/utils"

	"wx_channel/pkg/util"

	"github.com/andybalholm/brotli"
	"github.com/qtgolang/SunnyNet/SunnyNet"
	sunnyPublic "github.com/qtgolang/SunnyNet/public"
)

// ScriptHandler JavaScript注入处理器
type ScriptHandler struct {
	coreJS                []byte
	decryptJS             []byte
	downloadJS            []byte
	homeJS                []byte
	feedJS                []byte
	profileJS             []byte
	batchDownloadJS       []byte
	zipJS                 []byte
	fileSaverJS           []byte
	mittJS                []byte
	eventbusJS            []byte
	utilsJS               []byte
	apiClientJS           []byte
	keepAliveJS           []byte
	liveJS                []byte
	officialAccountJS     []byte
	officialAccountOrigin string
	officialAccountToken  string
	version               string
	liveContextMu         sync.Mutex
	liveContexts          map[string]time.Time
	lastLivePageAt        time.Time
}

var officialAccountCSPNoncePattern = regexp.MustCompile(`(?i)(?:'nonce-|nonce-)([^'";\s]+)`)

// livePageContextUntil is process-wide because SunnyNet can deliver the live
// HTML and its shared JS assets through different callback instances and
// connection identifiers. The marker only covers the short asset-loading
// window and is refreshed by each live HTML navigation.
var livePageContextUntil atomic.Int64

// SetOfficialAccountScript enables the isolated public-account page injection.
func (h *ScriptHandler) SetOfficialAccountScript(script []byte, origin, token string) {
	if h == nil {
		return
	}
	h.officialAccountJS = script
	h.officialAccountOrigin = strings.TrimRight(strings.TrimSpace(origin), "/")
	h.officialAccountToken = strings.TrimSpace(token)
}

// SetLiveScript enables the isolated live-page capture and download UI.
func (h *ScriptHandler) SetLiveScript(script []byte) {
	if h == nil {
		return
	}
	h.liveJS = script
}

// NewScriptHandler 创建脚本处理器
func NewScriptHandler(cfg *config.Config, coreJS, decryptJS, downloadJS, homeJS, feedJS, profileJS, batchDownloadJS, zipJS, fileSaverJS, mittJS, eventbusJS, utilsJS, apiClientJS, keepAliveJS []byte, version string) *ScriptHandler {
	return &ScriptHandler{
		coreJS:          coreJS,
		decryptJS:       decryptJS,
		downloadJS:      downloadJS,
		homeJS:          homeJS,
		feedJS:          feedJS,
		profileJS:       profileJS,
		batchDownloadJS: batchDownloadJS,
		zipJS:           zipJS,
		fileSaverJS:     fileSaverJS,
		mittJS:          mittJS,
		eventbusJS:      eventbusJS,
		utilsJS:         utilsJS,
		apiClientJS:     apiClientJS,
		keepAliveJS:     keepAliveJS,
		version:         version,
		liveContexts:    make(map[string]time.Time),
	}
}

// getConfig 获取当前配置（动态获取最新配置）
func (h *ScriptHandler) getConfig() *config.Config {
	return config.Get()
}

// Handle implements router.Interceptor
func (h *ScriptHandler) Handle(Conn *SunnyNet.HttpConn) bool {

	if Conn.Type != sunnyPublic.HttpResponseOK {
		return false
	}

	// 防御性检查
	if Conn.Request == nil || Conn.Request.URL == nil {
		return false
	}

	// 只有响应成功且有内容才处理
	if Conn.Response == nil || Conn.Response.Body == nil {
		return false
	}

	// 读取响应体
	// 注意：这里读取了Body，如果未被修改，需要重新赋值回去
	body, err := io.ReadAll(Conn.Response.Body)
	if err != nil {
		return false
	}
	_ = Conn.Response.Body.Close()

	decodedBody, decoded, decodeErr := decodeHTTPResponseBody(Conn.Response.Header, body)
	if decodeErr != nil {
		// Do not send a malformed or partially decoded response. Returning the
		// original bytes lets SunnyNet forward the upstream response with its
		// original Content-Encoding header intact.
		utils.LogFileInfo("[响应解压] 跳过响应改写: Content-Encoding=%q | error=%v", Conn.Response.Header.Get("Content-Encoding"), decodeErr)
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer(body))
		return false
	}
	if decoded {
		encodingHeader := Conn.Response.Header.Get("Content-Encoding")
		body = decodedBody
		Conn.Response.Header.Del("Content-Encoding")
		Conn.Response.Header.Del("Content-Length")
		Conn.Response.ContentLength = int64(len(body))
		Conn.Response.Uncompressed = true
		utils.LogFileInfo("[响应解压] 已解压上游响应: Content-Encoding=%q | bytes=%d", encodingHeader, len(body))
	}

	host := Conn.Request.URL.Hostname()
	path := Conn.Request.URL.Path

	// 记录所有JS文件的加载（简略日志）
	if strings.HasSuffix(path, ".js") {
		contentType := strings.ToLower(Conn.Response.Header.Get("content-type"))
		utils.LogFileInfo("[响应] Path=%s | ContentType=%s", path, contentType)
	}

	if h.HandleHTMLResponse(Conn, host, path, body) {
		return true
	}

	if h.HandleJavaScriptResponse(Conn, host, path, body) {
		return true
	}

	// 如果没有处理，恢复Body
	Conn.Response.Body = io.NopCloser(bytes.NewBuffer(body))
	return false
}

// HandleHTMLResponse 处理HTML响应，注入JavaScript代码
func (h *ScriptHandler) HandleHTMLResponse(Conn *SunnyNet.HttpConn, host, path string, body []byte) bool {
	contentType := strings.ToLower(Conn.Response.Header.Get("content-type"))
	if !isHTMLContentType(contentType) {
		return false
	}

	html := string(body)
	originalHTML := html
	pagePath := strings.TrimRight(path, "/")
	if pagePath == "" {
		pagePath = "/"
	}

	// 添加版本号到JS引用
	scriptReg1 := regexp.MustCompile(`src="([^"]{1,})\.js"`)
	html = scriptReg1.ReplaceAllString(html, `src="$1.js`+h.version+`"`)
	scriptReg2 := regexp.MustCompile(`href="([^"]{1,})\.js"`)
	html = scriptReg2.ReplaceAllString(html, `href="$1.js`+h.version+`"`)
	Conn.Response.Header.Set("__debug", "append_script")

	// The proxy has already consumed the upstream body. Once script URLs or
	// inline injection change its size, stale compression and length headers can
	// make the WebView render a blank page. This is especially visible on the
	// standalone live page, which does not receive the download UI injection.
	isLivePage := host == "channels.weixin.qq.com" && pagePath == "/web/pages/live"
	if host == "channels.weixin.qq.com" {
		h.rememberLivePageContext(Conn, pagePath)
	}
	if host == "channels.weixin.qq.com" && (html != originalHTML || isLivePage) {
		resetModifiedResponseHeaders(Conn.Response.Header)
	}

	if host == "channels.weixin.qq.com" && (pagePath == "/web/pages/feed" || pagePath == "/web/pages/home" || pagePath == "/web/pages/profile" || pagePath == "/web/pages/account/like" || pagePath == "/web/pages/live") {
		// 根据页面路径注入不同的脚本
		injectedScripts := h.buildInjectedScripts(pagePath)
		var injected bool
		html, injected = injectScriptsIntoHTMLHead(html, injectedScripts)
		if !injected {
			utils.LogFileInfo("[脚本注入] ⚠️ 未找到可用 HTML 注入点 | Host=%s | Path=%s", host, path)
		}
		utils.LogFileInfo("页面已成功加载！")
		utils.LogFileInfo("已添加视频缓存监控和提醒功能")
		utils.LogFileInfo("[页面加载] 视频号页面已加载 | Host=%s | Path=%s", host, path)
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
		return true
	}

	if host == "mp.weixin.qq.com" && h.hasOfficialAccountPage(path) {
		biz := ""
		if Conn.Request != nil && Conn.Request.URL != nil {
			query := Conn.Request.URL.Query()
			biz = firstNonEmpty(query.Get("__biz"), query.Get("biz"))
		}
		nonce := officialAccountCSPNonce(Conn.Response.Header)
		injected := h.buildOfficialAccountInjection(biz, nonce)
		lowerHTML := strings.ToLower(html)
		if bodyIndex := strings.LastIndex(lowerHTML, "</body>"); bodyIndex >= 0 {
			html = html[:bodyIndex] + injected + html[bodyIndex:]
		} else if headIndex := strings.Index(lowerHTML, "</head>"); headIndex >= 0 {
			html = html[:headIndex] + injected + html[headIndex:]
		} else {
			html = injected + html
		}
		Conn.Response.Header.Set("__debug", "append_official_account_script")
		if nonce != "" {
			utils.LogFileInfo("[脚本注入] 公众号页面检测到 CSP nonce，已应用到采集脚本")
		}
		utils.LogFileInfo("[脚本注入] 公众号文章页面已注入采集脚本 | Host=%s | Path=%s", host, path)
		// The body and its byte length changed. Prevent the WebView from
		// reusing an uninstrumented cached article or truncating the injected
		// script because of upstream compression/length headers.
		resetModifiedResponseHeaders(Conn.Response.Header)
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
		return true
	}

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
	return true
}

func (h *ScriptHandler) hasOfficialAccountPage(path string) bool {
	if len(h.officialAccountJS) == 0 {
		return false
	}
	return officialaccount.IsOfficialAccountPath(path)
}

func officialAccountCSPNonce(headers http.Header) string {
	if headers == nil {
		return ""
	}
	for _, name := range []string{"Content-Security-Policy", "Content-Security-Policy-Report-Only"} {
		if match := officialAccountCSPNoncePattern.FindStringSubmatch(headers.Get(name)); len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

func (h *ScriptHandler) buildOfficialAccountInjection(biz, nonce string) string {
	config, _ := json.Marshal(map[string]string{
		"origin": h.officialAccountOrigin,
		"token":  h.officialAccountToken,
		"biz":    strings.TrimSpace(biz),
	})
	attrs := ""
	if nonce != "" {
		attrs = ` nonce="` + htmlstd.EscapeString(nonce) + `"`
	}
	return fmt.Sprintf(`<script%s>window.__wx_channels_mp_config__=%s;</script><script%s>%s</script>`, attrs, config, attrs, string(h.officialAccountJS))
}

// HandleJavaScriptResponse 处理JavaScript响应，修改JavaScript代码
func (h *ScriptHandler) HandleJavaScriptResponse(Conn *SunnyNet.HttpConn, host, path string, body []byte) bool {
	contentType := strings.ToLower(Conn.Response.Header.Get("content-type"))
	if !isJavaScriptContentType(contentType) {
		return false
	}

	// 记录所有JS文件的加载（用于调试）
	utils.LogFileInfo("[JS文件] %s", path)

	// 保存关键的 JS 文件到本地以便分析
	h.saveJavaScriptFile(path, body)

	content := string(body)
	originalContent := content
	resetHeaders := func() {
		if content != originalContent {
			resetModifiedResponseHeaders(Conn.Response.Header)
		}
	}

	// 添加版本号到JS引用
	depReg := regexp.MustCompile(`"js/([^"]{1,})\.js"`)
	fromReg := regexp.MustCompile(`from {0,1}"([^"]{1,})\.js"`)
	lazyImportReg := regexp.MustCompile(`import\("([^"]{1,})\.js"\)`)
	importReg := regexp.MustCompile(`import {0,1}"([^"]{1,})\.js"`)
	content = fromReg.ReplaceAllString(content, `from"$1.js`+h.version+`"`)
	content = depReg.ReplaceAllString(content, `"js/$1.js`+h.version+`"`)
	content = lazyImportReg.ReplaceAllString(content, `import("$1.js`+h.version+`")`)
	content = importReg.ReplaceAllString(content, `import"$1.js`+h.version+`"`)
	Conn.Response.Header.Set("__debug", "replace_script")

	// The standalone live page shares common bundles with Home/Profile. Keep
	// dependency versioning above, but skip the page-specific source rewrites:
	// those rewrites assume the Home/Profile runtime and can break live-page
	// initialization before the detail view is rendered.
	if h.shouldSkipPageSpecificRewrites(Conn, host) {
		var handled bool
		content, handled = h.handleLivePageApis(path, content)
		if handled {
			resetHeaders()
		}
		mode := "直播页"
		if !isLivePageRequest(Conn) {
			mode = "来源未知页"
		}
		utils.LogFileInfo("[直播页注入] ✅ %s保留原始页面运行时并跳过主页专用 JS 改写 | Host=%s | Path=%s | Referer=%s", mode, host, path, requestReferer(Conn))
	} else {
		// 处理不同的JS文件
		content, handled := h.handleIndexPublish(path, content)
		if handled {
			resetHeaders()
			Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
			return true
		}
		content, handled = h.handleVirtualSvgIcons(path, content)
		if handled {
			resetHeaders()
			Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
			return true
		}

		content, handled = h.handleWorkerRelease(path, content)
		if handled {
			resetHeaders()
			Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
			return true
		}
		content, handled = h.handleConnectPublish(Conn, path, content)
		if handled {
			resetHeaders()
			return true
		}
	}

	resetHeaders()
	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return true
}

// buildInjectedScripts 构建所有需要注入的脚本（根据页面路径注入不同脚本）
func (h *ScriptHandler) buildInjectedScripts(path string) string {
	if path == "/web/pages/live" {
		// 直播页使用隔离运行时，不能加载 Home/Profile 的基础脚本；但仍
		// 需要页面快照脚本，否则直播页无法通过“保存页面”进入后端保存接口。
		return fmt.Sprintf(`<script>%s</script>%s`, string(h.liveJS), h.getSavePageContentScript())
	}

	// 事件系统脚本（mitt + eventbus + utils）- 必须在主脚本之前加载
	mittScript := fmt.Sprintf(`<script>%s</script>`, string(h.mittJS))
	eventbusScript := fmt.Sprintf(`<script>%s</script>`, string(h.eventbusJS))
	utilsScript := fmt.Sprintf(`<script>%s</script>`, string(h.utilsJS))

	// API 客户端脚本 - 必须在其他脚本之前加载
	apiClientScript := fmt.Sprintf(`<script>%s</script>`, string(h.apiClientJS))

	// 页面保活脚本 - 防止页面休眠
	keepAliveScript := fmt.Sprintf(`<script>%s</script>`, string(h.keepAliveJS))

	// 模块化脚本 - 按依赖顺序加载
	fileSaverInlineScript := fmt.Sprintf(`<script>%s</script>`, string(h.fileSaverJS))
	coreScript := fmt.Sprintf(`<script>%s</script>`, string(h.coreJS))
	decryptScript := fmt.Sprintf(`<script>%s</script>`, string(h.decryptJS))
	downloadScript := fmt.Sprintf(`<script>%s</script>`, string(h.downloadJS))
	batchDownloadScript := fmt.Sprintf(`<script>%s</script>`, string(h.batchDownloadJS))
	feedScript := fmt.Sprintf(`<script>%s</script>`, string(h.feedJS))
	profileScript := fmt.Sprintf(`<script>%s</script>`, string(h.profileJS))
	homeScript := fmt.Sprintf(`<script>%s</script>`, string(h.homeJS))

	// 预加载FileSaver.js库 - 所有页面都需要
	preloadScript := h.getPreloadScript()

	// 下载记录功能 - 所有页面都需要
	downloadTrackerScript := h.getDownloadTrackerScript()

	// 捕获URL脚本 - 所有页面都需要
	captureUrlScript := h.getCaptureUrlScript()

	// 保存页面内容脚本 - 所有页面都需要（用于保存快照）
	savePageContentScript := h.getSavePageContentScript()

	// 基础脚本（所有页面都需要）
	baseScripts := mittScript + eventbusScript + utilsScript + apiClientScript + keepAliveScript + fileSaverInlineScript + coreScript + decryptScript + downloadScript + batchDownloadScript + feedScript + profileScript + homeScript + preloadScript + downloadTrackerScript + captureUrlScript + savePageContentScript

	// 根据页面路径决定是否注入特定脚本
	var pageSpecificScripts string

	switch path {
	case "/web/pages/home":
		pageSpecificScripts = h.getVideoCacheNotificationScript()
		utils.LogFileInfo("[脚本注入] Home页面 - 注入视频缓存监控脚本")

	case "/web/pages/profile":
		// Profile页面（视频列表）：不需要特定脚本
		pageSpecificScripts = ""
		utils.LogFileInfo("[脚本注入] Profile页面 - 仅注入基础脚本")

	case "/web/pages/account/like":
		// 赞过页面会加载被全局改写的公共 bundle，需要基础脚本环境避免 WXU/WXE 未定义
		pageSpecificScripts = ""
		utils.LogFileInfo("[脚本注入] Account Like页面 - 注入基础脚本以兼容公共 JS 事件")

	case "/web/pages/feed":
		pageSpecificScripts = h.getVideoCacheNotificationScript()
		utils.LogFileInfo("[脚本注入] Feed页面 - 注入视频缓存监控脚本")

	case "/web/pages/live":
		// 保留分支文档；实际脚本在函数开头以隔离模式直接返回。
		pageSpecificScripts = ""

	default:
		// 其他页面：不注入页面特定脚本
		pageSpecificScripts = ""
		utils.LogInfo("[脚本注入] 其他页面 - 仅注入基础脚本")
	}

	// 初始化脚本（延迟执行）
	initScript := `<script>
console.log('[init] 开始初始化...');
setTimeout(function() {
	console.log('[init] 启动页面注入生命周期');
	if (typeof __wx_start_page_injection_lifecycle__ === 'function') {
		__wx_start_page_injection_lifecycle__();
	} else if (typeof insert_download_btn === 'function') {
		insert_download_btn();
	} else {
		console.error('[init] 页面注入入口未定义');
	}
}, 800);
</script>`

	return baseScripts + pageSpecificScripts + initScript
}

// injectScriptsIntoHTMLHead inserts inline scripts after the opening head tag.
// We cannot assume the upstream page uses the exact literal "<head>": the
// live page has returned attributes and different casing across deployments.
// If a response has no head element, fall back to the body or document start
// so diagnostics and the isolated live runtime still become available.
func injectScriptsIntoHTMLHead(html, scripts string) (string, bool) {
	if strings.TrimSpace(scripts) == "" {
		return html, false
	}

	headPattern := regexp.MustCompile(`(?is)<head\b[^>]*>`)
	if loc := headPattern.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + "\n" + scripts + html[loc[1]:], true
	}

	bodyPattern := regexp.MustCompile(`(?is)<body\b[^>]*>`)
	if loc := bodyPattern.FindStringIndex(html); loc != nil {
		return html[:loc[1]] + "\n" + scripts + html[loc[1]:], true
	}

	return scripts + html, true
}

// getPreloadScript 获取预加载FileSaver.js库的脚本
func (h *ScriptHandler) getPreloadScript() string {
	return `<script>
	// FileSaver 已内联注入，保留空预加载占位避免旧逻辑报错
	</script>`
}

// getDownloadTrackerScript 获取下载记录功能的脚本
func (h *ScriptHandler) getDownloadTrackerScript() string {
	return `<script>
	// 跟踪已记录的下载，防止重复记录
	window.__wx_channels_recorded_downloads = {};

	// 添加下载记录功能
	window.__wx_channels_record_download = function(data) {
		// 检查是否已经记录过这个下载
		const recordKey = data.id;
		if (window.__wx_channels_recorded_downloads[recordKey]) {
			console.log("已经记录过此下载，跳过记录");
			return;
		}
		
		// 标记为已记录
		window.__wx_channels_recorded_downloads[recordKey] = true;
		
		// 发送到记录API
		fetch("/__wx_channels_api/record_download", {
			method: "POST",
			headers: {
				"Content-Type": "application/json"
			},
			body: JSON.stringify(data)
		});
	};
	
	// 暂停视频的辅助函数（只暂停，不阻止自动切换）
	window.__wx_channels_pause_video__ = function() {
		console.log('[视频助手] 暂停视频（下载期间）...');
		try {
			let pausedCount = 0;
			const pausedVideos = [];
			
			// 方法1: 使用 Video.js API
			if (typeof videojs !== 'undefined') {
				const players = videojs.getAllPlayers?.() || [];
				players.forEach((player, index) => {
					if (player && typeof player.pause === 'function' && !player.paused()) {
						player.pause();
						pausedVideos.push({ type: 'videojs', player, index });
						pausedCount++;
						console.log('[视频助手] Video.js 播放器', index, '已暂停');
					}
				});
			}
			
			// 方法2: 查找所有 video 元素
			const videos = document.querySelectorAll('video');
			videos.forEach((video, index) => {
				// 尝试通过 Video.js 获取播放器实例
				let player = null;
				if (typeof videojs !== 'undefined') {
					try {
						player = videojs(video);
					} catch (e) {
						// 不是 Video.js 播放器
					}
				}
				
				if (player && typeof player.pause === 'function') {
					if (!player.paused()) {
						player.pause();
						pausedVideos.push({ type: 'videojs', player, index });
						pausedCount++;
						console.log('[视频助手] Video.js 播放器', index, '已暂停');
					}
				} else {
					if (!video.paused) {
						video.pause();
						pausedVideos.push({ type: 'native', video, index });
						pausedCount++;
						console.log('[视频助手] 原生视频', index, '已暂停');
					}
				}
			});
			
			console.log('[视频助手] 共暂停', pausedCount, '个视频');
			
			// 返回暂停的视频列表，用于后续恢复
			return pausedVideos;
		} catch (e) {
			console.error('[视频助手] 暂停视频失败:', e);
			return [];
		}
	};
	
	// 恢复视频播放的辅助函数
	window.__wx_channels_resume_video__ = function(pausedVideos) {
		if (!pausedVideos || pausedVideos.length === 0) return;
		
		console.log('[视频助手] 恢复视频播放...');
		try {
			pausedVideos.forEach(item => {
				if (item.type === 'videojs' && item.player) {
					item.player.play();
					console.log('[视频助手] Video.js 播放器', item.index, '已恢复');
				} else if (item.type === 'native' && item.video) {
					item.video.play();
					console.log('[视频助手] 原生视频', item.index, '已恢复');
				}
			});
		} catch (e) {
			console.error('[视频助手] 恢复视频失败:', e);
		}
	};
	
	// 覆盖原有的下载处理函数
	const originalHandleClick = window.__wx_channels_handle_click_download__;
	if (originalHandleClick) {
		window.__wx_channels_handle_click_download__ = function(sp) {
			// 暂停视频
			const pausedVideos = window.__wx_channels_pause_video__();
			
			// 调用原始函数进行下载
			originalHandleClick(sp);
			
			// 注意：不再手动记录下载，因为后端API已经处理了记录保存
			// 移除重复的记录调用以避免CSV中出现重复记录
			
			// 3秒后恢复播放（给下载一些时间开始）
			setTimeout(() => {
				window.__wx_channels_resume_video__(pausedVideos);
			}, 5000);
		};
	}
	
	// 覆盖当前视频下载函数
	const originalDownloadCur = window.__wx_channels_download_cur__;
	if (originalDownloadCur) {
		window.__wx_channels_download_cur__ = function() {
			// 暂停视频
			const pausedVideos = window.__wx_channels_pause_video__();
			
			// 调用原始函数进行下载
			originalDownloadCur();
			
			// 注意：不再手动记录下载，因为后端API已经处理了记录保存
			// 移除重复的记录调用以避免CSV中出现重复记录
			
			// 3秒后恢复播放（给下载一些时间开始）
			setTimeout(() => {
				window.__wx_channels_resume_video__(pausedVideos);
			}, 3000);
		};
	}
	
	// 优化封面下载函数：使用后端API保存到服务器
	window.__wx_channels_handle_download_cover = function() {
		if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
			const profile = window.__wx_channels_store__.profile;
			// 优先使用thumbUrl，然后是fullThumbUrl，最后才是coverUrl
			const coverUrl = profile.thumbUrl || profile.fullThumbUrl || profile.coverUrl;
			
			if (!coverUrl) {
				alert("未找到封面图片");
				return;
			}
			
			// 记录日志
			if (window.__wx_log) {
				window.__wx_log({
					msg: '正在保存封面到服务器...\n' + coverUrl
				});
			}
			
			// 构建请求数据
			const requestData = {
				coverUrl: coverUrl,
				videoId: profile.id || '',
				title: profile.title || '',
				author: profile.nickname || (profile.contact && profile.contact.nickname) || '未知作者',
				forceSave: false
			};
			
			// 添加授权头
			const headers = {
				'Content-Type': 'application/json'
			};
			if (window.__WX_LOCAL_TOKEN__) {
				headers['X-Local-Auth'] = window.__WX_LOCAL_TOKEN__;
			}
			
			// 发送到后端API保存封面
			fetch('/__wx_channels_api/save_cover', {
				method: 'POST',
				headers: headers,
				body: JSON.stringify(requestData)
			})
			.then(response => response.json())
			.then(data => {
				if (data.success) {
					const msg = data.message || '封面已保存';
					const path = data.relativePath || data.path || '';
					if (window.__wx_log) {
						window.__wx_log({
							msg: '✓ ' + msg
						});
					}
					console.log('✓ [封面下载] 封面已保存:', path);
				} else {
					const errorMsg = data.error || '保存封面失败';
					if (window.__wx_log) {
						window.__wx_log({
							msg: '❌ ' + errorMsg
						});
					}
					alert('保存封面失败: ' + errorMsg);
				}
			})
			.catch(error => {
				console.error("保存封面失败:", error);
				if (window.__wx_log) {
					window.__wx_log({
						msg: '❌ 保存封面失败: ' + error.message
					});
				}
				alert("保存封面失败: " + error.message);
			});
		} else {
			alert("未找到视频信息");
		}
	};
	</script>`
}

// getCaptureUrlScript 获取捕获完整URL的脚本
func (h *ScriptHandler) getCaptureUrlScript() string {
	return `<script>
	setTimeout(function() {
		// 获取完整的URL
		var fullUrl = window.location.href;
		// 发送到我们的API端点
		fetch("/__wx_channels_api/page_url", {
			method: "POST",
			headers: {
				"Content-Type": "application/json"
			},
			body: JSON.stringify({
				url: fullUrl
			})
		});
	}, 2000); // 延迟2秒执行，确保页面完全加载
	</script>`
}

// getSavePageContentScript 获取保存页面内容的脚本
func (h *ScriptHandler) getSavePageContentScript() string {
	return `<script>
	// 简单的字符串哈希函数 (djb2算法)
	function computeHash(str) {
		var hash = 5381;
		var i = str.length;
		while(i) {
			hash = (hash * 33) ^ str.charCodeAt(--i);
		}
		return hash >>> 0; // 强制转换为无符号32位整数
	}

	// 状态变量
	window.__wx_last_saved_hash = 0;
	window.__wx_save_timer = null;

	// 保存当前页面完整内容的函数 (带去重和防抖)
	window.__wx_channels_save_page_content = function(force) {
		try {
			// 清除之前的定时器
			if (window.__wx_save_timer) {
				clearTimeout(window.__wx_save_timer);
				window.__wx_save_timer = null;
			}

			// 获取当前完整的HTML内容
			var fullHtml = document.documentElement.outerHTML;
			
			// 计算哈希
			var currentHash = computeHash(fullHtml);

			// 如果不是强制保存，且哈希值与上次相同，则跳过
			if (!force && currentHash === window.__wx_last_saved_hash) {
				// console.log("[PageSave] 内容未变化，跳过保存");
				return;
			}

			var currentUrl = window.location.href;
			
			// 发送到保存API
			fetch("/__wx_channels_api/save_page_content", {
				method: "POST",
				headers: {
					"Content-Type": "application/json"
				},
				body: JSON.stringify({
					url: currentUrl,
					html: fullHtml,
					timestamp: new Date().getTime()
				})
			}).then(response => {
				if (response.ok) {
					console.log("[PageSave] 页面内容已保存");
					window.__wx_last_saved_hash = currentHash;
				}
			}).catch(error => {
				console.error("[PageSave] 保存页面内容失败:", error);
			});
		} catch (error) {
			console.error("[PageSave] 获取页面内容失败:", error);
		}
	};
	
	// 触发带防抖的保存 (默认延迟2秒)
	window.__wx_trigger_save_page = function(delay) {
		if (typeof delay === 'undefined') delay = 2000;
		
		if (window.__wx_save_timer) {
			clearTimeout(window.__wx_save_timer);
		}
		
		window.__wx_save_timer = setTimeout(function() {
			window.__wx_channels_save_page_content(false);
		}, delay);
	};

	// 监听URL变化，自动保存页面内容
	let currentPageUrl = window.location.href;
	const checkUrlChange = () => {
		if (window.location.href !== currentPageUrl) {
			currentPageUrl = window.location.href;
			// URL变化后延迟保存，等待内容加载
			window.__wx_trigger_save_page(5000);
		}
	};
	
	// 定期检查URL变化（适用于SPA）
	setInterval(checkUrlChange, 1000);
	
	// 监听历史记录变化
	window.addEventListener('popstate', () => {
		window.__wx_trigger_save_page(3000);
	});
	
	// 在页面加载完成后也保存一次
	setTimeout(() => {
		window.__wx_trigger_save_page(2000);
	}, 8000);
	</script>`
}

// getVideoCacheNotificationScript 获取视频缓存监控脚本
func (h *ScriptHandler) getVideoCacheNotificationScript() string {
	return `<script>
	// 初始化视频缓存监控
	window.__wx_channels_video_cache_monitor = {
		isBuffering: false,
		lastBufferTime: 0,
		totalBufferSize: 0,
		videoSize: 0,
		completeThreshold: 0.98, // 认为98%缓冲完成时视频已缓存完成
		checkInterval: null,
		notificationShown: false, // 防止重复显示通知
		
		// 开始监控缓存
		startMonitoring: function(expectedSize) {
			console.log('=== 开始启动视频缓存监控 ===');
			
			// 检查播放器状态
			const vjsPlayer = document.querySelector('.video-js');
			const video = vjsPlayer ? vjsPlayer.querySelector('video') : document.querySelector('video');
			
			if (!video) {
				console.error('未找到视频元素，无法启动监控');
				return;
			}
			
			console.log('视频元素状态:');
			console.log('- readyState:', video.readyState);
			console.log('- duration:', video.duration);
			console.log('- buffered.length:', video.buffered ? video.buffered.length : 0);
			
			if (this.checkInterval) {
				clearInterval(this.checkInterval);
			}
			
			this.isBuffering = true;
			this.lastBufferTime = Date.now();
			this.totalBufferSize = 0;
			this.videoSize = expectedSize || 0;
			this.notificationShown = false; // 重置通知状态
			
			console.log('视频缓存监控已启动');
			console.log('- 视频大小:', (this.videoSize / (1024 * 1024)).toFixed(2) + 'MB');
			console.log('- 监控间隔: 2秒');
			
			// 定期检查缓冲状态 - 增加检查频率
			this.checkInterval = setInterval(() => this.checkBufferStatus(), 2000);
			
			// 添加可见的缓存状态指示器
			this.addStatusIndicator();
			
			// 监听视频播放完成事件
			this.setupVideoEndedListener();
			
			// 延迟开始监控，让播放器有时间初始化
			setTimeout(() =>{
				this.monitorNativeBuffering();
			}, 1000);
		},
		
		// 监控Video.js播放器和原生视频元素的缓冲状态
		monitorNativeBuffering: function() {
			let firstCheck = true; // 标记是否是第一次检查
			const checkBufferedProgress = () => {
				// 优先检查Video.js播放器
				const vjsPlayer = document.querySelector('.video-js');
				let video = null;
				
				if (vjsPlayer) {
					// 从Video.js播放器中获取video元素
					video = vjsPlayer.querySelector('video');
					if (firstCheck) {
						console.log('找到Video.js播放器，开始监控');
						firstCheck = false;
					}
				} else {
					// 回退到查找普通video元素
					const videoElements = document.querySelectorAll('video');
					if (videoElements.length > 0) {
						video = videoElements[0];
						if (firstCheck) {
							console.log('使用普通video元素监控');
							firstCheck = false;
						}
					}
				}
				
				if (video) {
					// 获取预加载进度条数据
					if (video.buffered && video.buffered.length > 0 && video.duration) {
						// 获取最后缓冲时间范围的结束位置
						const bufferedEnd = video.buffered.end(video.buffered.length - 1);
						// 计算缓冲百分比
						const bufferedPercent = (bufferedEnd / video.duration) * 100;
						
						// 更新页面指示器
						const indicator = document.getElementById('video-cache-indicator');
						if (indicator) {
							indicator.innerHTML = '<div>视频缓存中: ' + bufferedPercent.toFixed(1) + '% (Video.js播放器)</div>';
							
							// 高亮显示接近完成的状态
							if (bufferedPercent >= 95) {
								indicator.style.backgroundColor = 'rgba(0,128,0,0.8)';
							}
						}
						
						// 检查Video.js播放器的就绪状态（只在第一次检查时输出）
						if (vjsPlayer && typeof vjsPlayer.readyState !== 'undefined' && firstCheck) {
							console.log('Video.js播放器就绪状态:', vjsPlayer.readyState);
						}
						
						// 检查是否缓冲完成
						if (bufferedPercent >= 98) {
							console.log('根据Video.js播放器数据，视频已缓存完成 (' + bufferedPercent.toFixed(1) + '%)');
							this.showNotification();
							this.stopMonitoring();
							return true; // 缓存完成，停止监控
						}
					}
				}
				return false; // 继续监控
			};
			
			// 立即检查一次
			if (!checkBufferedProgress()) {
				// 每秒检查一次预加载进度
				const bufferCheckInterval = setInterval(() => {
					if (checkBufferedProgress() || !this.isBuffering) {
						clearInterval(bufferCheckInterval);
					}
				}, 1000);
			}
		},
		
		// 设置Video.js播放器和视频播放结束监听
		setupVideoEndedListener: function() {
			// 尝试查找Video.js播放器和视频元素
			setTimeout(() => {
				const vjsPlayer = document.querySelector('.video-js');
				let video = null;
				
				if (vjsPlayer) {
					// 从Video.js播放器中获取video元素
					video = vjsPlayer.querySelector('video');
					console.log('为Video.js播放器设置事件监听');
					
					// 尝试监听Video.js特有的事件
					if (vjsPlayer.addEventListener) {
						vjsPlayer.addEventListener('ended', () => {
							console.log('Video.js播放器播放结束，标记为缓存完成');
							this.showNotification();
							this.stopMonitoring();
						});
						
						vjsPlayer.addEventListener('loadeddata', () => {
							console.log('Video.js播放器数据加载完成');
						});
					}
				} else {
					// 回退到查找普通video元素
					const videoElements = document.querySelectorAll('video');
					if (videoElements.length > 0) {
						video = videoElements[0];
						console.log('为普通video元素设置事件监听');
					}
				}
				
				if (video) {
					// 监听视频播放结束事件
					video.addEventListener('ended', () => {
						console.log('视频播放已结束，标记为缓存完成');
						this.showNotification();
						this.stopMonitoring();
					});
					
					// 如果视频已在播放中，添加定期检查播放状态
					if (!video.paused) {
						const playStateInterval = setInterval(() => {
							// 如果视频已经播放完或接近结束（剩余小于2秒）
							if (video.ended || (video.duration && video.currentTime > 0 && video.duration - video.currentTime < 2)) {
								console.log('视频接近或已播放完成，标记为缓存完成');
								this.showNotification();
								this.stopMonitoring();
								clearInterval(playStateInterval);
							}
						}, 1000);
					}
				}
			}, 3000); // 延迟3秒再查找视频元素，确保Video.js播放器完全初始化
		},
		
		// 添加缓冲状态指示器
		addStatusIndicator: function() {
			console.log('正在创建缓存状态指示器...');
			
			// 移除现有指示器
			const existingIndicator = document.getElementById('video-cache-indicator');
			if (existingIndicator) {
				console.log('移除现有指示器');
				existingIndicator.remove();
			}
			
			// 创建新指示器
			const indicator = document.createElement('div');
			indicator.id = 'video-cache-indicator';
			indicator.style.cssText = "position:fixed;bottom:20px;left:20px;background-color:rgba(0,0,0,0.8);color:white;padding:10px 15px;border-radius:6px;z-index:99999;font-size:14px;font-family:Arial,sans-serif;border:2px solid rgba(255,255,255,0.3);";
			indicator.innerHTML = '<div>🔄 视频缓存中: 0%</div>';
			document.body.appendChild(indicator);
			
			console.log('缓存状态指示器已创建并添加到页面');
			
			// 初始化进度跟踪变量
			this.lastLoggedProgress = 0;
			this.stuckCheckCount = 0;
			this.maxStuckCount = 30; // 30秒不变则认为停滞
			
			// 每秒更新进度
			const updateInterval = setInterval(() => {
				if (!this.isBuffering) {
					clearInterval(updateInterval);
					indicator.remove();
					return;
				}
				
				let progress = 0;
				let progressSource = 'unknown';
				
				// 优先方案：从video元素实时读取（最准确）
				const vjsPlayer = document.querySelector('.video-js');
				let video = vjsPlayer ? vjsPlayer.querySelector('video') : null;
				
				if (!video) {
					const videoElements = document.querySelectorAll('video');
					if (videoElements.length > 0) {
						video = videoElements[0];
					}
				}
				
				if (video && video.buffered && video.buffered.length > 0) {
					try {
						const bufferedEnd = video.buffered.end(video.buffered.length - 1);
						const duration = video.duration;
						if (duration > 0 && !isNaN(duration) && isFinite(duration)) {
							progress = (bufferedEnd / duration) * 100;
							progressSource = 'video.buffered';
						}
					} catch (e) {
						// 忽略读取错误
					}
				}
				
				// 备用方案：使用 totalBufferSize
				if (progress === 0 && this.videoSize > 0 && this.totalBufferSize > 0) {
					progress = (this.totalBufferSize / this.videoSize) * 100;
					progressSource = 'totalBufferSize';
				}
				
				// 限制进度范围
				progress = Math.min(Math.max(progress, 0), 100);
				
				// 检测进度是否停滞
				const progressChanged = Math.abs(progress - this.lastLoggedProgress) >= 0.1;
				
				if (!progressChanged) {
					this.stuckCheckCount++;
				} else {
					this.stuckCheckCount = 0;
				}
				
				// 更新指示器
				if (progress > 0) {
					// 根据停滞状态显示不同的图标
					let icon = '🔄';
					let statusText = '视频缓存中';
					
					if (this.stuckCheckCount >= this.maxStuckCount) {
						icon = '⏸️';
						statusText = '缓存暂停';
						indicator.style.backgroundColor = 'rgba(128,128,128,0.8)';
					} else if (progress >= 95) {
						icon = '✅';
						statusText = '缓存接近完成';
						indicator.style.backgroundColor = 'rgba(0,128,0,0.8)';
					} else if (progress >= 50) {
						indicator.style.backgroundColor = 'rgba(255,165,0,0.8)';
					} else {
						indicator.style.backgroundColor = 'rgba(0,0,0,0.8)';
					}
					
					indicator.innerHTML = '<div>' + icon + ' ' + statusText + ': ' + progress.toFixed(1) + '%</div>';
					
					// 只在进度变化≥1%时输出日志
					if (Math.abs(progress - this.lastLoggedProgress) >= 1) {
						console.log('缓存进度更新:', progress.toFixed(1) + '% (来源:' + progressSource + ')');
						this.lastLoggedProgress = progress;
					}
					
					// 停滞提示（只输出一次）
					if (this.stuckCheckCount === this.maxStuckCount) {
						console.log('⏸️ 缓存进度长时间未变化 (' + progress.toFixed(1) + '%)，可能原因：');
						console.log('  - 视频已暂停播放');
						console.log('  - 网络速度慢或连接中断');
						console.log('  - 浏览器缓存策略限制');
						console.log('  提示：继续播放视频可能会恢复缓存');
					}
				} else {
					indicator.innerHTML = '<div>⏳ 等待视频数据...</div>';
				}
				
				// 如果进度达到98%以上，检查是否完成
				if (progress >= 98) {
					this.checkCompletion();
				}
			}, 1000);
		},
		
		// 添加缓冲块
		addBuffer: function(buffer) {
			if (!this.isBuffering) return;
			
			// 更新最后缓冲时间
			this.lastBufferTime = Date.now();
			
			// 累计缓冲大小
			if (buffer && buffer.byteLength) {
				this.totalBufferSize += buffer.byteLength;
				
				// 输出调试信息到控制台
				if (this.videoSize > 0) {
					const percent = ((this.totalBufferSize / this.videoSize) * 100).toFixed(1);
					console.log('视频缓存进度: ' + percent + '% (' + (this.totalBufferSize / (1024 * 1024)).toFixed(2) + 'MB/' + (this.videoSize / (1024 * 1024)).toFixed(2) + 'MB)');
				}
			}
			
			// 检查是否接近完成
			this.checkCompletion();
		},
		
		// 检查Video.js播放器和原生视频的缓冲状态
		checkBufferStatus: function() {
			if (!this.isBuffering) return;
			
			// 优先检查Video.js播放器
			const vjsPlayer = document.querySelector('.video-js');
			let video = null;
			
			if (vjsPlayer) {
				// 从Video.js播放器中获取video元素
				video = vjsPlayer.querySelector('video');
				
				// 检查Video.js播放器特有的状态（只在状态变化时输出日志）
				if (vjsPlayer.classList.contains('vjs-has-started')) {
					if (!this._vjsStartedLogged) {
						console.log('Video.js播放器已开始播放');
						this._vjsStartedLogged = true;
					}
				}
				
				if (vjsPlayer.classList.contains('vjs-waiting')) {
					if (!this._vjsWaitingLogged) {
						console.log('Video.js播放器正在等待数据');
						this._vjsWaitingLogged = true;
					}
				} else {
					this._vjsWaitingLogged = false; // 重置标记，以便下次等待时再次输出
				}
				
				if (vjsPlayer.classList.contains('vjs-ended')) {
					console.log('Video.js播放器播放结束，标记为缓存完成');
					this.checkCompletion(true);
					return;
				}
			} else {
				// 回退到查找普通video元素
				const videoElements = document.querySelectorAll('video');
				if (videoElements.length > 0) {
					video = videoElements[0];
				}
			}
			
			if (video) {
				if (video.buffered && video.buffered.length > 0 && video.duration) {
					// 获取最后缓冲时间范围的结束位置
					const bufferedEnd = video.buffered.end(video.buffered.length - 1);
					// 计算缓冲百分比
					const bufferedPercent = (bufferedEnd / video.duration) * 100;
					
					// 如果预加载接近完成，触发完成检测（只输出一次日志）
					if (bufferedPercent >= 95 && !this._preloadNearCompleteLogged) {
						console.log('检测到视频预加载接近完成 (' + bufferedPercent.toFixed(1) + '%)');
						this._preloadNearCompleteLogged = true;
						this.checkCompletion(true);
					}
				}
				
				// 只在readyState为4且缓冲百分比较高时才认为完成
				if (video.readyState >= 4 && video.buffered && video.buffered.length > 0 && video.duration) {
					const bufferedEnd = video.buffered.end(video.buffered.length - 1);
					const bufferedPercent = (bufferedEnd / video.duration) * 100;
					if (bufferedPercent >= 98 && !this._readyStateCompleteLogged) {
						console.log('视频readyState为4且缓冲98%以上，标记为缓存完成');
						this._readyStateCompleteLogged = true;
						this.checkCompletion(true);
					}
				}
			}
			
			// 如果超过10秒没有新的缓冲数据且已经缓冲了部分数据，可能表示视频已暂停或缓冲完成
			const timeSinceLastBuffer = Date.now() - this.lastBufferTime;
			if (timeSinceLastBuffer > 10000 && this.totalBufferSize > 0) {
				this.checkCompletion(true);
			}
		},
		
		// 检查是否完成
		checkCompletion: function(forcedCheck) {
			if (!this.isBuffering) return;
			
			let isComplete = false;
			
			// 优先检查Video.js播放器是否已播放完成
			const vjsPlayer = document.querySelector('.video-js');
			let video = null;
			
			if (vjsPlayer) {
				video = vjsPlayer.querySelector('video');
				
				// 检查Video.js播放器的完成状态
				if (vjsPlayer.classList.contains('vjs-ended')) {
					console.log('Video.js播放器已播放完毕，认为缓存完成');
					isComplete = true;
				}
			} else {
				// 回退到查找普通video元素
				const videoElements = document.querySelectorAll('video');
				if (videoElements.length > 0) {
					video = videoElements[0];
				}
			}
			
			if (video && !isComplete) {
				// 如果视频已经播放完毕或接近结束，直接认为完成
				if (video.ended || (video.duration && video.currentTime > 0 && video.duration - video.currentTime < 2)) {
					console.log('视频已播放完毕或接近结束，认为缓存完成');
					isComplete = true;
				}
				
				// 只在readyState为4且缓冲百分比较高时才认为完成
				if (video.readyState >= 4 && video.buffered && video.buffered.length > 0 && video.duration) {
					const bufferedEnd = video.buffered.end(video.buffered.length - 1);
					const bufferedPercent = (bufferedEnd / video.duration) * 100;
					if (bufferedPercent >= 98) {
						console.log('视频readyState为4且缓冲98%以上，认为缓存完成');
						isComplete = true;
					}
				}
			}
			
			// 如果未通过播放状态判断完成，再检查缓冲大小
			if (!isComplete) {
				// 如果知道视频大小，则根据百分比判断
				if (this.videoSize > 0) {
					const ratio = this.totalBufferSize / this.videoSize;
					// 对短视频降低阈值要求
					const threshold = this.videoSize < 5 * 1024 * 1024 ? 0.9 : this.completeThreshold; // 5MB以下视频降低阈值到90%
					isComplete = ratio >= threshold;
				} 
				// 强制检查：如果长时间没有新数据且视频元素可以播放到最后，也认为已完成
				else if (forcedCheck && video) {
					if (video.readyState >= 3 && video.buffered.length > 0) {
						const bufferedEnd = video.buffered.end(video.buffered.length - 1);
						const duration = video.duration;
						isComplete = duration > 0 && (bufferedEnd / duration) >= 0.95; // 降低阈值到95%
						
						if (isComplete) {
							console.log('强制检查：根据缓冲数据判断视频缓存完成');
						}
					}
				}
			}
			
			// 如果完成，显示通知
			if (isComplete) {
				this.showNotification();
				this.stopMonitoring();
			}
		},
		
		// 显示通知
		showNotification: function() {
			// 防止重复显示通知
			if (this.notificationShown) {
				console.log('通知已经显示过，跳过重复显示');
				return;
			}
			
			console.log('显示缓存完成通知');
			this.notificationShown = true;
			
			// 移除进度指示器
			const indicator = document.getElementById('video-cache-indicator');
			if (indicator) {
				indicator.remove();
			}
			
			// 创建桌面通知
			if ("Notification" in window && Notification.permission === "granted") {
				new Notification("视频缓存完成", {
					body: "视频已缓存完成，可以进行下载操作",
					icon: window.__wx_channels_store__?.profile?.coverUrl
				});
			}
			
			// 在页面上显示通知
			const notification = document.createElement('div');
			notification.style.cssText = "position:fixed;bottom:20px;right:20px;background-color:rgba(0,128,0,0.9);color:white;padding:15px 25px;border-radius:8px;z-index:99999;animation:fadeInOut 12s forwards;box-shadow:0 4px 12px rgba(0,0,0,0.3);font-size:16px;font-weight:bold;";
			notification.innerHTML = '<div style="display:flex;align-items:center;"><span style="font-size:24px;margin-right:12px;">🎉</span> <span>视频缓存完成，可以下载了！</span></div>';
			
			// 添加动画样式 - 延长显示时间到12秒
			const style = document.createElement('style');
			style.textContent = '@keyframes fadeInOut {0% {opacity:0;transform:translateY(20px);} 8% {opacity:1;transform:translateY(0);} 85% {opacity:1;} 100% {opacity:0;}}';
			document.head.appendChild(style);
			
			document.body.appendChild(notification);
			
			// 12秒后移除通知
			setTimeout(() => {
				notification.remove();
			}, 12000);
			
			// 发送通知事件
			fetch("/__wx_channels_api/tip", {
				method: "POST",
				headers: {
					"Content-Type": "application/json"
				},
				body: JSON.stringify({
					msg: "视频缓存完成，可以下载了！"
				})
			});
			
			console.log("视频缓存完成通知已显示");
		},
		
		// 停止监控
		stopMonitoring: function() {
			console.log('停止视频缓存监控');
			if (this.checkInterval) {
				clearInterval(this.checkInterval);
				this.checkInterval = null;
			}
			this.isBuffering = false;
			// 注意：不重置notificationShown，保持通知状态直到下次startMonitoring
		}
	};
	
	// 请求通知权限
	if ("Notification" in window && Notification.permission !== "granted" && Notification.permission !== "denied") {
		// 用户操作后再请求权限
		document.addEventListener('click', function requestPermission() {
			Notification.requestPermission();
			document.removeEventListener('click', requestPermission);
		}, {once: true});
	}
	</script>`
}

// handleIndexPublish 处理index.publish JS文件
func (h *ScriptHandler) handleIndexPublish(path string, content string) (string, bool) {
	if !util.Includes(path, "/t/wx_fed/finder/web/web-finder/res/js/index.publish") {
		return content, false
	}

	utils.LogFileInfo("[Home数据采集] 正在处理 index.publish 文件")

	regexp1 := regexp.MustCompile(`this.sourceBuffer.appendBuffer\(h\),`)
	replaceStr1 := `(() => {
if (window.__wx_channels_store__) {
window.__wx_channels_store__.buffers.push(h);
// 添加缓存监控
if (window.__wx_channels_video_cache_monitor) {
    window.__wx_channels_video_cache_monitor.addBuffer(h);
}
}
})(),this.sourceBuffer.appendBuffer(h),`
	if regexp1.MatchString(content) {
		utils.LogFileInfo("视频播放已成功加载！")
		utils.LogFileInfo("视频缓冲将被监控，完成时会有提醒")
		utils.LogFileInfo("[视频播放] 视频播放器已加载 | Path=%s", path)
	}
	logInjectPatternResult("sourceBuffer.appendBuffer", path, regexp1.MatchString(content))
	content = regexp1.ReplaceAllString(content, replaceStr1)
	regexp2 := regexp.MustCompile(`if\(f.cmd===re.MAIN_THREAD_CMD.AUTO_CUT`)
	replaceStr2 := `if(f.cmd==="CUT"){
	if (window.__wx_channels_store__) {
	// console.log("CUT", f, __wx_channels_store__.profile.key);
	window.__wx_channels_store__.keys[__wx_channels_store__.profile.key]=f.decryptor_array;
	}
}
if(f.cmd===re.MAIN_THREAD_CMD.AUTO_CUT`
	logInjectPatternResult("AUTO_CUT", path, regexp2.MatchString(content))
	content = regexp2.ReplaceAllString(content, replaceStr2)

	return content, true
}

// handleVirtualSvgIcons 处理virtual_svg-icons-register JS文件
func (h *ScriptHandler) handleVirtualSvgIcons(path string, content string) (string, bool) {
	if !util.Includes(path, "/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register") {
		return content, false
	}

	// 2026-03-13: 首页改版后，finderPcFlow/finderStream/finderGetRecommend 所在链路
	// 对首屏当前标签的初始化更敏感。这里暂时停用首页流相关的源码重写，
	// 先保证 Home 页面刷新和首屏加载稳定，保留 feed/profile/search 等稳定链路。
	if strings.Contains(content, "finderPcFlow(") {
		utils.LogFileInfo("[API拦截] ⏭️ 跳过 finderPcFlow 重写，避免干扰新版 Home 首屏初始化")
	}
	if strings.Contains(content, "finderStream(") {
		utils.LogFileInfo("[API拦截] ⏭️ 跳过 finderStream 重写，避免干扰新版 Home 首屏初始化")
	}

	// 拦截 finderGetCommentDetail - 视频详情（参考 wx_channels_download 项目）
	feedProfileRegex := regexp.MustCompile(`(?s)async\s+finderGetCommentDetail\s*\(([^)]+)\)\s*\{(.*?)\}\s*async`)
	feedProfileMatched := feedProfileRegex.MatchString(content)
	logInjectPatternResult("finderGetCommentDetail", path, feedProfileMatched)
	if feedProfileMatched {
		utils.LogFileInfo("[API拦截] ✅ 在virtual_svg-icons-register中成功拦截 finderGetCommentDetail 函数")
		feedProfileReplace := `async finderGetCommentDetail($1){var result=await(async()=>{$2})();var feed=result.data.object;console.log("[API拦截] finderGetCommentDetail 触发 FeedProfileLoaded");WXU.emit(WXU.Events.FeedProfileLoaded,feed);return result;}async`
		content = feedProfileRegex.ReplaceAllString(content, feedProfileReplace)
	}

	commentListRegex := regexp.MustCompile(`(?s)async\s+finderGetCommentList\s*\(([^)]+)\)\s*\{(.*?)\}\s*async`)
	commentListMatched := commentListRegex.MatchString(content)
	logInjectPatternResult("finderGetCommentList", path, commentListMatched)
	if commentListMatched {
		utils.LogFileInfo("[API拦截] ✅ 在virtual_svg-icons-register中成功拦截 finderGetCommentList 函数")
		commentListReplace := `async finderGetCommentList($1){var result=await(async()=>{$2})();console.log("[API拦截] finderGetCommentList 返回评论列表");WXU.emit(WXU.Events.FeedCommentListLoaded,result.data);return result;}async`
		content = commentListRegex.ReplaceAllString(content, commentListReplace)
	}

	// 拦截 Profile 页面的视频列表数据 - 使用事件系统（参考 wx_channels_download 项目）
	profileListRegex := regexp.MustCompile(`(?s)async\s+finderUserPage\s*\(([^)]+)\)\s*\{return(.*?)\}\s*async`)
	profileListMatched := profileListRegex.MatchString(content)
	logInjectPatternResult("finderUserPage", path, profileListMatched)
	if profileListMatched {
		utils.LogFileInfo("[API拦截] ✅ 在virtual_svg-icons-register中成功拦截 finderUserPage 函数")
		profileListReplace := `async finderUserPage($1){console.log("[Profile API] finderUserPage 调用参数:",$1);var result=await(async()=>{return$2})();console.log("[Profile API] finderUserPage 原始结果:",result);if(result&&result.data&&result.data.object){var feeds=result.data.object;console.log("[Profile API] 提取到",feeds.length,"个视频");WXU.emit(WXU.Events.UserFeedsLoaded,feeds);}else{console.warn("[Profile API] result.data.object 为空",result);}return result;}async`
		content = profileListRegex.ReplaceAllString(content, profileListReplace)
	}

	// 拦截 Profile 页面的直播回放列表数据 - 使用事件系统
	liveListRegex := regexp.MustCompile(`(?s)async\s+finderLiveUserPage\s*\(([^)]+)\)\s*\{return(.*?)\}\s*async`)
	liveListMatched := liveListRegex.MatchString(content)
	logInjectPatternResult("finderLiveUserPage", path, liveListMatched)
	if liveListMatched {
		utils.LogFileInfo("[API拦截] ✅ 在virtual_svg-icons-register中成功拦截 finderLiveUserPage 函数")
		liveListReplace := `async finderLiveUserPage($1){console.log("[Profile API] finderLiveUserPage 调用参数:",$1);var result=await(async()=>{return$2})();console.log("[Profile API] finderLiveUserPage 原始结果:",result);if(result&&result.data&&result.data.object){var feeds=result.data.object;console.log("[Profile API] 提取到",feeds.length,"个直播回放");WXU.emit(WXU.Events.UserLiveReplayLoaded,feeds);}else{console.warn("[Profile API] result.data.object 为空",result);}return result;}async`
		content = liveListRegex.ReplaceAllString(content, liveListReplace)
	}

	if strings.Contains(content, "finderGetRecommend(") {
		utils.LogFileInfo("[API拦截] ⏭️ 跳过 finderGetRecommend 重写，避免干扰新版 Home 首屏初始化")
	}

	// 拦截 export 语句，提取所有导出的 API 函数
	// 格式: export{xxx as yyy,zzz as www,...}
	exportBlockRegex := regexp.MustCompile(`export\s*\{([^}]+)\}`)
	exportRegex := regexp.MustCompile(`export\s*\{`)

	exportBlockMatched := exportBlockRegex.MatchString(content)
	logInjectPatternResult("export block", path, exportBlockMatched)
	if exportBlockMatched {
		utils.LogFileInfo("[API拦截] ✅ 在virtual_svg-icons-register中找到 export 语句")

		// 提取 export 块中的内容
		matches := exportBlockRegex.FindStringSubmatch(content)
		if len(matches) >= 2 {
			exportContent := matches[1]
			utils.LogFileInfo("[API拦截] Export 内容: %s", exportContent[:min(100, len(exportContent))])

			// 解析导出的函数名
			items := strings.Split(exportContent, ",")
			var locals []string
			for _, item := range items {
				p := strings.TrimSpace(item)
				if p == "" {
					continue
				}
				// 处理 "xxx as yyy" 格式
				idx := strings.Index(p, " as ")
				local := p
				if idx != -1 {
					local = strings.TrimSpace(p[:idx])
				}
				if local != "" && local != " " {
					locals = append(locals, local)
				}
			}

			if len(locals) > 0 {
				utils.LogFileInfo("[API拦截] 提取到 %d 个导出函数", len(locals))
				apiMethods := "{" + strings.Join(locals, ",") + "}"
				// 转义 $ 符号
				apiMethodsEscaped := strings.ReplaceAll(apiMethods, "$", "$$")

				// 在 export 之前插入 API 加载事件
				jsWXAPI := ";WXU.emit(WXU.Events.APILoaded," + apiMethodsEscaped + ");export{"
				content = exportRegex.ReplaceAllString(content, jsWXAPI)
				utils.LogFileInfo("[API拦截] ✅ 已注入 APILoaded 事件")
			}
		}
	}

	return content, true
}

// handleLivePageApis captures only the two APIs needed by the standalone live
// page. The upstream application owns the rest of this shared bundle; adding
// the Home/Profile event wrappers here can break live-page initialization.
func (h *ScriptHandler) handleLivePageApis(path, content string) (string, bool) {
	if !strings.Contains(strings.ToLower(path), "virtual_svg-icons-register") {
		return content, false
	}

	changed := false
	wrap := func(label string, re *regexp.Regexp, valueExpression string) {
		if !re.MatchString(content) {
			return
		}
		replacement := `async ` + label + `($1){var __wx_live_result__=await(async()=>{$2})();try{if(typeof window.__wx_channels_capture_live_data__==="function"){window.__wx_channels_capture_live_data__("` + label + `",` + valueExpression + `);}}catch(__wx_live_capture_error__){}return __wx_live_result__;}async`
		content = re.ReplaceAllString(content, replacement)
		changed = true
		utils.LogFileInfo("[直播页注入] ✅ 捕获 %s API", label)
	}

	wrap("finderGetCommentDetail", regexp.MustCompile(`(?s)async\s+finderGetCommentDetail\s*\(([^)]+)\)\s*\{(.*?)\}\s*async`), `__wx_live_result__&&__wx_live_result__.data`)
	wrap("finderGetLiveInfo", regexp.MustCompile(`(?s)async\s+finderGetLiveInfo\s*\(([^)]+)\)\s*\{(.*?)\}\s*async`), `__wx_live_result__&&__wx_live_result__.data`)
	wrap("joinLive", regexp.MustCompile(`(?s)async\s+joinLive\s*\(([^)]+)\)\s*\{(.*?)\}\s*async`), `__wx_live_result__&&__wx_live_result__.data`)
	return content, changed
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleWorkerRelease 处理worker_release JS文件
func (h *ScriptHandler) handleWorkerRelease(path string, content string) (string, bool) {
	if !util.Includes(path, "worker_release") {
		return content, false
	}

	regex := regexp.MustCompile(`fmp4Index:p.fmp4Index`)
	replaceStr := `decryptor_array:p.decryptor_array,fmp4Index:p.fmp4Index`
	logInjectPatternResult("worker_release fmp4Index", path, regex.MatchString(content))
	content = regex.ReplaceAllString(content, replaceStr)
	return content, true
}

func isHTMLContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/html")
}

func isJavaScriptContentType(contentType string) bool {
	lower := strings.ToLower(contentType)
	return strings.Contains(lower, "javascript") || strings.Contains(lower, "ecmascript") || strings.Contains(lower, "text/js")
}

func isLivePageRequest(conn *SunnyNet.HttpConn) bool {
	refererURL, ok := requestRefererURL(conn)
	if !ok {
		return false
	}
	pagePath := strings.TrimRight(refererURL.Path, "/")
	return strings.EqualFold(refererURL.Hostname(), "channels.weixin.qq.com") && pagePath == "/web/pages/live"
}

// shouldSkipPageSpecificRewrites keeps the upstream page runtime intact for
// live pages. Some Windows WebView requests omit Referer on shared bundles;
// those requests are deliberately treated as unknown rather than being sent
// through Home/Profile source rewrites, which can blank the live page.
func (h *ScriptHandler) shouldSkipPageSpecificRewrites(conn *SunnyNet.HttpConn, host string) bool {
	if isLivePageRequest(conn) {
		return true
	}
	// Shared bundles are commonly served from res.wx.qq.com rather than
	// channels.weixin.qq.com. Check the active live navigation before the host
	// filter, otherwise those cross-host assets still receive Home rewrites.
	if h.hasRecentLivePageContext(conn) {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(host), "channels.weixin.qq.com") {
		return false
	}
	if strings.TrimSpace(requestReferer(conn)) == "" {
		return true
	}
	_, ok := requestRefererURL(conn)
	return !ok
}

// rememberLivePageContext handles WebView requests whose Referer remains the
// Home route while the browser is loading the standalone live route. Without
// this context, shared bundles are incorrectly sent through Home/Profile
// rewrites and the live page can render blank.
func (h *ScriptHandler) rememberLivePageContext(conn *SunnyNet.HttpConn, pagePath string) {
	if h == nil || conn == nil {
		return
	}
	key := requestContextKey(conn)
	h.liveContextMu.Lock()
	defer h.liveContextMu.Unlock()
	if pagePath == "/web/pages/live" {
		if h.liveContexts == nil {
			h.liveContexts = make(map[string]time.Time)
		}
		now := time.Now()
		h.liveContexts[key] = now
		livePageContextUntil.Store(now.Add(45 * time.Second).UnixNano())
		utils.LogFileInfo("[直播上下文] 已记录直播导航，后续公共 JS 进入直播保护窗口 | Path=%s", pagePath)
		// SunnyNet may expose different PID/client values for the HTML request
		// and the following shared-script requests. Keep a short fallback window
		// so the live route is still recognized across those connections.
		h.lastLivePageAt = now
		return
	}
	delete(h.liveContexts, key)
}

func (h *ScriptHandler) hasRecentLivePageContext(conn *SunnyNet.HttpConn) bool {
	if h == nil || conn == nil {
		return false
	}
	key := requestContextKey(conn)
	h.liveContextMu.Lock()
	defer h.liveContextMu.Unlock()
	when, ok := h.liveContexts[key]
	if ok && time.Since(when) <= 2*time.Minute {
		return true
	}
	if ok {
		delete(h.liveContexts, key)
	}
	if !h.lastLivePageAt.IsZero() && time.Since(h.lastLivePageAt) <= 45*time.Second {
		return true
	}
	return livePageContextUntil.Load() > time.Now().UnixNano()
}

func requestContextKey(conn *SunnyNet.HttpConn) string {
	if conn == nil {
		return "unknown"
	}
	if conn.PID != 0 {
		return fmt.Sprintf("pid:%d", conn.PID)
	}
	if strings.TrimSpace(conn.ClientIP) != "" {
		return "client:" + strings.TrimSpace(conn.ClientIP)
	}
	return "unknown"
}

func requestReferer(conn *SunnyNet.HttpConn) string {
	if conn == nil || conn.Request == nil || conn.Request.Header == nil {
		return ""
	}
	return strings.TrimSpace(conn.Request.Header.Get("Referer"))
}

func requestRefererURL(conn *SunnyNet.HttpConn) (*url.URL, bool) {
	referer := requestReferer(conn)
	if referer == "" {
		return nil, false
	}
	refererURL, err := url.Parse(referer)
	if err != nil || refererURL == nil || refererURL.Hostname() == "" {
		return nil, false
	}
	return refererURL, true
}

// decodeHTTPResponseBody decodes the response before any HTML/JavaScript
// rewrite. SunnyNet uses a custom HTTP reader and does not transparently
// decompress upstream gzip/br responses like net/http.Transport does.
func decodeHTTPResponseBody(headers http.Header, body []byte) ([]byte, bool, error) {
	if headers == nil || len(body) == 0 {
		return body, false, nil
	}
	encodingHeader := strings.TrimSpace(strings.ToLower(headers.Get("Content-Encoding")))
	if encodingHeader == "" || encodingHeader == "identity" {
		return body, false, nil
	}

	encodings := strings.Split(encodingHeader, ",")
	decoded := body
	for i := len(encodings) - 1; i >= 0; i-- {
		encodingName := strings.TrimSpace(encodings[i])
		var reader io.Reader
		switch encodingName {
		case "gzip", "x-gzip":
			gzipReader, err := gzip.NewReader(bytes.NewReader(decoded))
			if err != nil {
				return body, false, err
			}
			reader = gzipReader
			decoded, err = io.ReadAll(reader)
			closeErr := gzipReader.Close()
			if err != nil {
				return body, false, err
			}
			if closeErr != nil {
				return body, false, closeErr
			}
		case "deflate":
			zlibReader, err := zlib.NewReader(bytes.NewReader(decoded))
			if err != nil {
				return body, false, err
			}
			decoded, err = io.ReadAll(zlibReader)
			closeErr := zlibReader.Close()
			if err != nil {
				return body, false, err
			}
			if closeErr != nil {
				return body, false, closeErr
			}
		case "br":
			brotliReader := brotli.NewReader(bytes.NewReader(decoded))
			var err error
			decoded, err = io.ReadAll(brotliReader)
			if err != nil {
				return body, false, err
			}
		case "":
			continue
		default:
			return body, false, fmt.Errorf("unsupported content encoding %q", encodingName)
		}
	}
	return decoded, true, nil
}

func resetModifiedResponseHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	headers.Del("Content-Encoding")
	headers.Del("Content-Length")
	headers.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	headers.Set("Pragma", "no-cache")
	headers.Set("Expires", "0")
}

func logInjectPatternResult(label string, path string, matched bool) {
	if matched {
		utils.LogFileInfo("[注入检查] ✅ %s 命中: %s", label, path)
		return
	}
	utils.LogFileInfo("[注入检查] ❌ %s 未命中: %s", label, path)
}

// handleConnectPublish 处理connect.publish JS文件（参考 wx_channels_download 项目的实现）
func (h *ScriptHandler) handleConnectPublish(Conn *SunnyNet.HttpConn, path string, content string) (string, bool) {
	if !util.Includes(path, "connect.publish") {
		return content, false
	}

	utils.LogFileInfo("[Home数据采集] ✅ 正在处理 connect.publish 文件")
	utils.LogFileInfo("[Home数据采集] ⏭️ 跳过 goToNextFlowFeed/goToPrevFlowFeed 重写，避免干扰新版 Home 状态机")

	// 禁用浏览器缓存，确保每次都能拦截到最新的代码
	Conn.Response.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	Conn.Response.Header.Set("Pragma", "no-cache")
	Conn.Response.Header.Set("Expires", "0")

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return content, true
}

// saveJavaScriptFile 保存页面加载的 JavaScript 文件到本地以便分析
func (h *ScriptHandler) saveJavaScriptFile(path string, content []byte) {
	// 检查是否启用JS文件保存
	if h.getConfig() != nil && !h.getConfig().SavePageJS {
		return
	}

	// 只保存 .js 文件
	if !strings.HasSuffix(strings.Split(path, "?")[0], ".js") {
		return
	}

	// 获取基础目录
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		return
	}

	// 根据JS文件路径识别页面类型
	pageType := "common"
	pathLower := strings.ToLower(path)
	if strings.Contains(pathLower, "home") || strings.Contains(pathLower, "finderhome") {
		pageType = "home"
	} else if strings.Contains(pathLower, "profile") {
		pageType = "profile"
	} else if strings.Contains(pathLower, "feed") {
		pageType = "feed"
	} else if strings.Contains(pathLower, "search") {
		pageType = "search"
	} else if strings.Contains(pathLower, "live") {
		pageType = "live"
	}

	// 创建按页面类型分类的保存目录
	jsDir := filepath.Join(baseDir, h.getConfig().DownloadsDir, "cached_js", pageType)
	if err := utils.EnsureDir(jsDir); err != nil {
		return
	}

	// 从路径中提取文件名
	fileName := filepath.Base(path)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = strings.ReplaceAll(path, "/", "_")
		fileName = strings.ReplaceAll(fileName, "\\", "_")
	}

	// 移除版本号后缀（如 .js?v=xxx）
	fileName = strings.Split(fileName, "?")[0]

	// 检查文件是否已存在（避免重复保存相同内容）
	filePath := filepath.Join(jsDir, fileName)
	if _, err := os.Stat(filePath); err == nil {
		// 文件已存在，跳过
		return
	}

	// 保存文件
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		utils.LogInfo("[JS保存] 保存失败: %s - %v", fileName, err)
		return
	}

	utils.LogInfo("[JS保存] ✅ 已保存: %s/%s", pageType, fileName)
}
