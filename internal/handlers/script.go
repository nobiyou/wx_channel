package handlers

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wx_channel/internal/config"
	"wx_channel/internal/utils"

	"wx_channel/pkg/util"

	"github.com/qtgolang/SunnyNet/SunnyNet"
	sunnyPublic "github.com/qtgolang/SunnyNet/public"
)

// ScriptHandler JavaScript注入处理器
type ScriptHandler struct {
	coreJS          []byte
	decryptJS       []byte
	downloadJS      []byte
	homeJS          []byte
	feedJS          []byte
	profileJS       []byte
	batchDownloadJS []byte
	zipJS           []byte
	fileSaverJS     []byte
	mittJS          []byte
	eventbusJS      []byte
	utilsJS         []byte
	apiClientJS     []byte
	keepAliveJS     []byte
	version         string
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

	// 添加版本号到JS引用
	scriptReg1 := regexp.MustCompile(`src="([^"]{1,})\.js"`)
	html = scriptReg1.ReplaceAllString(html, `src="$1.js`+h.version+`"`)
	scriptReg2 := regexp.MustCompile(`href="([^"]{1,})\.js"`)
	html = scriptReg2.ReplaceAllString(html, `href="$1.js`+h.version+`"`)
	Conn.Response.Header.Set("__debug", "append_script")

	if host == "channels.weixin.qq.com" && (path == "/web/pages/feed" || path == "/web/pages/home" || path == "/web/pages/profile" || path == "/web/pages/account/like") {
		// 根据页面路径注入不同的脚本
		injectedScripts := h.buildInjectedScripts(path)
		html = strings.Replace(html, "<head>", "<head>\n"+injectedScripts, 1)
		utils.LogFileInfo("页面已成功加载！")
		utils.LogFileInfo("已添加视频缓存监控和提醒功能")
		utils.LogFileInfo("[页面加载] 视频号页面已加载 | Host=%s | Path=%s", host, path)
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
		return true
	}

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
	return true
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

	// 处理不同的JS文件
	content, handled := h.handleIndexPublish(path, content)
	if handled {
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
		return true
	}
	content, handled = h.handleVirtualSvgIcons(path, content)
	if handled {
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
		return true
	}

	content, handled = h.handleWorkerRelease(path, content)
	if handled {
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
		return true
	}
	content, handled = h.handleConnectPublish(Conn, path, content)
	if handled {
		return true
	}

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return true
}

// buildInjectedScripts 构建所有需要注入的脚本（根据页面路径注入不同脚本）
func (h *ScriptHandler) buildInjectedScripts(path string) string {
	// 日志面板脚本（必须在最前面，以便拦截所有console输出）- 所有页面都需要
	logPanelScript := h.getLogPanelScript()

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
	baseScripts := logPanelScript + mittScript + eventbusScript + utilsScript + apiClientScript + keepAliveScript + fileSaverInlineScript + coreScript + decryptScript + downloadScript + batchDownloadScript + feedScript + profileScript + homeScript + preloadScript + downloadTrackerScript + captureUrlScript + savePageContentScript

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

	default:
		// 其他页面：不注入页面特定脚本
		pageSpecificScripts = ""
		utils.LogInfo("[脚本注入] 其他页面 - 仅注入基础脚本")
	}

	// 初始化脚本（延迟执行）
	initScript := `<script>
console.log('[init] 开始初始化...');
setTimeout(function() {
	console.log('[init] 执行 insert_download_btn');
	if (typeof insert_download_btn === 'function') {
		insert_download_btn();
	} else {
		console.error('[init] insert_download_btn 函数未定义');
	}
}, 800);
</script>`

	return baseScripts + pageSpecificScripts + initScript
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

// getLogPanelScript 获取日志面板脚本，用于在页面上显示日志（替代控制台）
func (h *ScriptHandler) getLogPanelScript() string {
	// 根据配置决定是否显示日志按钮
	showLogButton := "false"
	if h.getConfig().ShowLogButton {
		showLogButton = "true"
	}

	// 根据配置决定是否拦截日志（默认禁用以节省内存）
	enableLogInterception := "false"
	if h.getConfig().EnableLogInterception {
		enableLogInterception = "true"
	}

	return `<script>
// 日志按钮显示配置
window.__wx_channels_show_log_button__ = ` + showLogButton + `;
// 日志拦截配置（禁用可节省内存）
window.__wx_channels_enable_log_interception__ = ` + enableLogInterception + `;
</script>
<script>
(function() {
	'use strict';
	
	// 防止重复初始化
	if (window.__wx_channels_log_panel_initialized__) {
		return;
	}
	window.__wx_channels_log_panel_initialized__ = true;
	
	// 日志存储（优化版 - 减少内存占用）
	const logStore = {
		logs: [],
		maxLogs: 100, // 最多保存100条日志（从500降低）
		updatePending: false,
		lastCleanupTime: Date.now(),
		cleanupInterval: 5 * 60 * 1000, // 每5分钟自动清理一次
		
		addLog: function(level, args) {
			// 过滤掉过于频繁的日志（防止刷屏）
			const message = Array.from(args).map(arg => {
				if (typeof arg === 'object') {
					try {
						// 限制对象序列化深度，避免大对象占用过多内存
						return JSON.stringify(arg, this.jsonReplacer, 2);
					} catch (e) {
						return String(arg);
					}
				}
				return String(arg);
			}).join(' ');
			
			// 跳过重复的日志（连续相同的日志只保留一条）
			if (this.logs.length > 0) {
				const lastLog = this.logs[this.logs.length - 1];
				if (lastLog.level === level && lastLog.message === message) {
					// 更新重复计数
					lastLog.count = (lastLog.count || 1) + 1;
					lastLog.timestamp = new Date().toLocaleTimeString('zh-CN', { hour12: false });
					this.scheduleUpdate();
					return;
				}
			}
			
			const timestamp = new Date().toLocaleTimeString('zh-CN', { hour12: false });
			this.logs.push({
				level: level,
				message: message,
				timestamp: timestamp,
				count: 1
			});
			
			// 限制日志数量（移除最旧的日志）
			if (this.logs.length > this.maxLogs) {
				this.logs.shift();
			}
			
			// 定期自动清理（防止内存累积）
			const now = Date.now();
			if (now - this.lastCleanupTime > this.cleanupInterval) {
				this.autoCleanup();
				this.lastCleanupTime = now;
			}
			
			// 批量更新显示（防止频繁DOM操作）
			this.scheduleUpdate();
		},
		
		// JSON序列化限制器（防止大对象）
		jsonReplacer: function(key, value) {
			// 限制字符串长度
			if (typeof value === 'string' && value.length > 500) {
				return value.substring(0, 500) + '... (truncated)';
			}
			// 限制数组长度
			if (Array.isArray(value) && value.length > 10) {
				return value.slice(0, 10).concat(['... (' + (value.length - 10) + ' more items)']);
			}
			return value;
		},
		
		// 自动清理旧日志（保留最近50条）
		autoCleanup: function() {
			if (this.logs.length > 50) {
				const removed = this.logs.length - 50;
				this.logs = this.logs.slice(-50);
				console.log('[日志面板] 自动清理了 ' + removed + ' 条旧日志');
			}
		},
		
		// 批量更新显示（防抖）
		scheduleUpdate: function() {
			if (this.updatePending) return;
			this.updatePending = true;
			
			// 使用 requestAnimationFrame 批量更新
			requestAnimationFrame(() => {
				this.updatePending = false;
				if (window.__wx_channels_log_panel) {
					window.__wx_channels_log_panel.updateDisplay();
				}
			});
		},
		
		clear: function() {
			this.logs = [];
			if (window.__wx_channels_log_panel) {
				window.__wx_channels_log_panel.updateDisplay();
			}
		}
	};
	
	// 创建日志面板
	function createLogPanel() {
		const panel = document.createElement('div');
		panel.id = '__wx_channels_log_panel';
		// 检测是否为移动设备
		const isMobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent) || window.innerWidth < 768;
		
		// 面板位置：在按钮旁边，向上展开
		const btnBottom = isMobile ? 80 : 20;
		const btnLeft = isMobile ? 15 : 20;
		const btnSize = isMobile ? 56 : 50;
		const panelWidth = isMobile ? 'calc(100% - 30px)' : '400px';
		const panelMaxWidth = isMobile ? '100%' : '500px';
		const panelMaxHeight = isMobile ? 'calc(100vh - ' + (btnBottom + btnSize + 20) + 'px)' : '500px';
		const panelFontSize = isMobile ? '11px' : '12px';
		const panelBottom = btnBottom + btnSize + 10; // 按钮上方10px
		
		panel.style.cssText = 'position: fixed;' +
			'bottom: ' + panelBottom + 'px;' +
			'left: ' + btnLeft + 'px;' +
			'width: ' + panelWidth + ';' +
			'max-width: ' + panelMaxWidth + ';' +
			'max-height: ' + panelMaxHeight + ';' +
			'height: 0;' +
			'background: rgba(0, 0, 0, 0.95);' +
			'border: 1px solid #333;' +
			'border-radius: 8px 8px 0 0;' +
			'box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.5);' +
			'z-index: 999999;' +
			'font-family: "Consolas", "Monaco", "Courier New", monospace;' +
			'font-size: ' + panelFontSize + ';' +
			'color: #fff;' +
			'display: none;' +
			'flex-direction: column;' +
			'overflow: hidden;' +
			'transition: height 0.3s ease, opacity 0.3s ease;' +
			'opacity: 0;';
		
		// 标题栏
		const header = document.createElement('div');
		header.style.cssText = 'background: #1a1a1a;' +
			'padding: 8px 12px;' +
			'border-bottom: 1px solid #333;' +
			'display: flex;' +
			'justify-content: space-between;' +
			'align-items: center;' +
			'cursor: move;' +
			'user-select: none;';
		
		const title = document.createElement('span');
		title.textContent = '📋 日志面板';
		title.style.cssText = 'font-weight: bold; color: #4CAF50;';
		
		const controls = document.createElement('div');
		controls.style.cssText = 'display: flex; gap: 8px;';
		
		// 清空按钮
		const clearBtn = document.createElement('button');
		clearBtn.textContent = '清空';
		clearBtn.style.cssText = 'background: #f44336;' +
			'color: white;' +
			'border: none;' +
			'padding: 4px 12px;' +
			'border-radius: 4px;' +
			'cursor: pointer;' +
			'font-size: 11px;';
		clearBtn.onclick = function(e) {
			e.stopPropagation();
			logStore.clear();
		};
		
		// 复制日志按钮
		const copyBtn = document.createElement('button');
		copyBtn.textContent = '复制';
		copyBtn.style.cssText = 'background: #4CAF50;' +
			'color: white;' +
			'border: none;' +
			'padding: 4px 12px;' +
			'border-radius: 4px;' +
			'cursor: pointer;' +
			'font-size: 11px;';
		copyBtn.onclick = function(e) {
			e.stopPropagation();
			try {
				// 构建日志文本
				var logText = '';
				logStore.logs.forEach(function(log) {
					var levelPrefix = '';
					switch(log.level) {
						case 'log': levelPrefix = '[LOG]'; break;
						case 'info': levelPrefix = '[INFO]'; break;
						case 'warn': levelPrefix = '[WARN]'; break;
						case 'error': levelPrefix = '[ERROR]'; break;
						default: levelPrefix = '[LOG]';
					}
					logText += '[' + log.timestamp + '] ' + levelPrefix + ' ' + log.message + '\n';
				});
				
				if (logText === '') {
					alert('日志为空，无需复制');
					return;
				}
				
				// 使用 Clipboard API 复制
				if (navigator.clipboard && navigator.clipboard.writeText) {
					navigator.clipboard.writeText(logText).then(function() {
						copyBtn.textContent = '已复制';
						setTimeout(function() {
							copyBtn.textContent = '复制';
						}, 2000);
					}).catch(function(err) {
						console.error('复制失败:', err);
						// 降级方案：使用传统方法
						copyToClipboardFallback(logText);
					});
				} else {
					// 降级方案：使用传统方法
					copyToClipboardFallback(logText);
				}
			} catch (error) {
				console.error('复制日志失败:', error);
				alert('复制失败: ' + error.message);
			}
		};
		
		// 复制到剪贴板的降级方案
		function copyToClipboardFallback(text) {
			var textArea = document.createElement('textarea');
			textArea.value = text;
			textArea.style.position = 'fixed';
			textArea.style.top = '-999px';
			textArea.style.left = '-999px';
			document.body.appendChild(textArea);
			textArea.select();
			try {
				var successful = document.execCommand('copy');
				if (successful) {
					copyBtn.textContent = '已复制';
					setTimeout(function() {
						copyBtn.textContent = '复制';
					}, 2000);
				} else {
					alert('复制失败，请手动选择文本复制');
				}
			} catch (err) {
				console.error('复制失败:', err);
				alert('复制失败: ' + err.message);
			}
			document.body.removeChild(textArea);
		}
		
		// 导出日志按钮
		const exportBtn = document.createElement('button');
		exportBtn.textContent = '导出';
		exportBtn.style.cssText = 'background: #FF9800;' +
			'color: white;' +
			'border: none;' +
			'padding: 4px 12px;' +
			'border-radius: 4px;' +
			'cursor: pointer;' +
			'font-size: 11px;';
		exportBtn.onclick = function(e) {
			e.stopPropagation();
			try {
				// 构建日志文本
				var logText = '';
				logStore.logs.forEach(function(log) {
					var levelPrefix = '';
					switch(log.level) {
						case 'log': levelPrefix = '[LOG]'; break;
						case 'info': levelPrefix = '[INFO]'; break;
						case 'warn': levelPrefix = '[WARN]'; break;
						case 'error': levelPrefix = '[ERROR]'; break;
						default: levelPrefix = '[LOG]';
					}
					logText += '[' + log.timestamp + '] ' + levelPrefix + ' ' + log.message + '\n';
				});
				
				if (logText === '') {
					alert('日志为空，无需导出');
					return;
				}
				
				// 创建 Blob 并下载
				var blob = new Blob([logText], { type: 'text/plain;charset=utf-8' });
				var url = URL.createObjectURL(blob);
				var a = document.createElement('a');
				var timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5);
				a.href = url;
				a.download = 'wx_channels_logs_' + timestamp + '.txt';
				document.body.appendChild(a);
				a.click();
				document.body.removeChild(a);
				URL.revokeObjectURL(url);
				
				exportBtn.textContent = '已导出';
				setTimeout(function() {
					exportBtn.textContent = '导出';
				}, 2000);
			} catch (error) {
				console.error('导出日志失败:', error);
				alert('导出失败: ' + error.message);
			}
		};
		
		// 最小化/最大化按钮
		const toggleBtn = document.createElement('button');
		toggleBtn.textContent = '−';
		toggleBtn.style.cssText = 'background: #2196F3;' +
			'color: white;' +
			'border: none;' +
			'padding: 4px 12px;' +
			'border-radius: 4px;' +
			'cursor: pointer;' +
			'font-size: 11px;';
		toggleBtn.onclick = function(e) {
			e.stopPropagation();
			const content = panel.querySelector('.log-content');
			if (content.style.display === 'none') {
				content.style.display = 'flex';
				toggleBtn.textContent = '−';
			} else {
				content.style.display = 'none';
				toggleBtn.textContent = '+';
			}
		};
		
		// 关闭按钮
		const closeBtn = document.createElement('button');
		closeBtn.textContent = '×';
		closeBtn.style.cssText = 'background: #666;' +
			'color: white;' +
			'border: none;' +
			'padding: 4px 12px;' +
			'border-radius: 4px;' +
			'cursor: pointer;' +
			'font-size: 14px;' +
			'line-height: 1;';
		closeBtn.onclick = function(e) {
			e.stopPropagation();
			panel.style.display = 'none';
		};
		
		controls.appendChild(clearBtn);
		controls.appendChild(copyBtn);
		controls.appendChild(exportBtn);
		controls.appendChild(toggleBtn);
		controls.appendChild(closeBtn);
		header.appendChild(title);
		header.appendChild(controls);
		
		// 日志内容区域
		const content = document.createElement('div');
		content.className = 'log-content';
		content.style.cssText = 'flex: 1;' +
			'overflow-y: auto;' +
			'padding: 8px;' +
			'display: flex;' +
			'flex-direction: column;' +
			'gap: 2px;';
		
		// 滚动条样式
		content.style.scrollbarWidth = 'thin';
		content.style.scrollbarColor = '#555 #222';
		
		// 更新显示（优化版 - 减少DOM操作）
		function updateDisplay() {
			// 使用 DocumentFragment 批量更新DOM
			const fragment = document.createDocumentFragment();
			
			logStore.logs.forEach(log => {
				const logItem = document.createElement('div');
				logItem.style.cssText = 'padding: 4px 8px;' +
					'border-radius: 4px;' +
					'word-break: break-all;' +
					'line-height: 1.4;' +
					'background: rgba(255, 255, 255, 0.05);';
				
				// 根据日志级别设置颜色
				let levelColor = '#fff';
				let levelPrefix = '';
				switch(log.level) {
					case 'log':
						levelColor = '#4CAF50';
						levelPrefix = '[LOG]';
						break;
					case 'info':
						levelColor = '#2196F3';
						levelPrefix = '[INFO]';
						break;
					case 'warn':
						levelColor = '#FF9800';
						levelPrefix = '[WARN]';
						break;
					case 'error':
						levelColor = '#f44336';
						levelPrefix = '[ERROR]';
						logItem.style.background = 'rgba(244, 67, 54, 0.2)';
						break;
					default:
						levelPrefix = '[LOG]';
				}
				
				// 显示重复计数
				const countBadge = log.count > 1 ? 
					'<span style="background: rgba(255,255,255,0.2); padding: 2px 6px; border-radius: 10px; font-size: 10px; margin-left: 4px;">×' + log.count + '</span>' : '';
				
				logItem.innerHTML = '<span style="color: #888; font-size: 10px;">[' + log.timestamp + ']</span>' +
					'<span style="color: ' + levelColor + '; font-weight: bold; margin: 0 4px;">' + levelPrefix + '</span>' +
					countBadge +
					'<span style="color: #fff;">' + escapeHtml(log.message) + '</span>';
				
				fragment.appendChild(logItem);
			});
			
			// 一次性更新DOM
			content.innerHTML = '';
			content.appendChild(fragment);
			
			// 自动滚动到底部
			content.scrollTop = content.scrollHeight;
		}
		
		// HTML转义
		function escapeHtml(text) {
			const div = document.createElement('div');
			div.textContent = text;
			return div.innerHTML;
		}
		
		panel.appendChild(header);
		panel.appendChild(content);
		document.body.appendChild(panel);
		
		// 移除拖拽功能，面板位置固定在按钮旁边
		
		// 计算面板高度
		function getPanelHeight() {
			// 临时显示以计算高度
			const wasHidden = panel.style.display === 'none';
			if (wasHidden) {
				panel.style.display = 'flex';
				panel.style.height = 'auto';
				panel.style.opacity = '0';
			}
			
			const maxHeight = parseInt(panel.style.maxHeight) || 500;
			const headerHeight = header.offsetHeight || 40;
			const contentHeight = content.scrollHeight || 0;
			const totalHeight = headerHeight + contentHeight + 16; // 16px padding
			const finalHeight = Math.min(maxHeight, totalHeight);
			
			if (wasHidden) {
				panel.style.display = 'none';
				panel.style.height = '0';
			}
			
			return finalHeight;
		}
		
		// 暴露更新方法
		window.__wx_channels_log_panel = {
			panel: panel,
			updateDisplay: updateDisplay,
			show: function() {
				panel.style.display = 'flex';
				// 使用requestAnimationFrame确保DOM已更新
				requestAnimationFrame(function() {
					const targetHeight = getPanelHeight();
					panel.style.height = targetHeight + 'px';
					panel.style.opacity = '1';
				});
			},
			hide: function() {
				panel.style.height = '0';
				panel.style.opacity = '0';
				// 动画结束后隐藏
				setTimeout(function() {
					if (panel.style.opacity === '0') {
						panel.style.display = 'none';
					}
				}, 300);
			},
			toggle: function() {
				if (panel.style.display === 'none' || panel.style.opacity === '0') {
					this.show();
				} else {
					this.hide();
				}
			}
		};
	}
	
	// 保存原始的console方法
	const originalConsole = {
		log: console.log.bind(console),
		info: console.info.bind(console),
		warn: console.warn.bind(console),
		error: console.error.bind(console),
		debug: console.debug.bind(console)
	};
	
	// 重写console方法（可选 - 根据配置决定是否拦截）
	// 如果不需要日志面板，可以完全禁用拦截以节省内存
	const enableLogInterception = window.__wx_channels_enable_log_interception__ || false;
	
	if (enableLogInterception) {
		console.log = function(...args) {
			originalConsole.log.apply(console, args);
			logStore.addLog('log', args);
		};
		
		console.info = function(...args) {
			originalConsole.info.apply(console, args);
			logStore.addLog('info', args);
		};
		
		console.warn = function(...args) {
			originalConsole.warn.apply(console, args);
			logStore.addLog('warn', args);
		};
		
		console.error = function(...args) {
			originalConsole.error.apply(console, args);
			logStore.addLog('error', args);
		};
		
		console.debug = function(...args) {
			originalConsole.debug.apply(console, args);
			logStore.addLog('log', args);
		};
		
		console.log('[日志面板] 日志拦截已启用（可能占用内存）');
	} else {
		console.log('[日志面板] 日志拦截已禁用（节省内存模式）');
	}
	
	// 创建浮动触发按钮（用于微信浏览器等无法使用快捷键的场景）
	function createToggleButton() {
		const btn = document.createElement('div');
		btn.id = '__wx_channels_log_toggle_btn';
		btn.innerHTML = '📋';
		// 检测是否为移动设备
		const isMobileBtn = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent) || window.innerWidth < 768;
		
		const btnBottom = isMobileBtn ? '80px' : '20px';
		const btnLeft = isMobileBtn ? '15px' : '20px';
		const btnWidth = isMobileBtn ? '56px' : '50px';
		const btnHeight = isMobileBtn ? '56px' : '50px';
		const btnFontSize = isMobileBtn ? '28px' : '24px';
		
		btn.style.cssText = 'position: fixed;' +
			'bottom: ' + btnBottom + ';' +
			'left: ' + btnLeft + ';' +
			'width: ' + btnWidth + ';' +
			'height: ' + btnHeight + ';' +
			'background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);' +
			'border-radius: 50%;' +
			'box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);' +
			'z-index: 999998;' +
			'cursor: pointer;' +
			'display: flex;' +
			'align-items: center;' +
			'justify-content: center;' +
			'font-size: ' + btnFontSize + ';' +
			'user-select: none;' +
			'transition: all 0.3s ease;' +
			'border: 2px solid rgba(255, 255, 255, 0.3);' +
			'touch-action: manipulation;' +
			'-webkit-tap-highlight-color: transparent;';
		
		btn.addEventListener('mouseenter', function() {
			btn.style.transform = 'scale(1.1)';
			btn.style.boxShadow = '0 6px 16px rgba(0, 0, 0, 0.4)';
		});
		
		btn.addEventListener('mouseleave', function() {
			btn.style.transform = 'scale(1)';
			btn.style.boxShadow = '0 4px 12px rgba(0, 0, 0, 0.3)';
		});
		
		// 切换面板显示的函数
		function togglePanel() {
			if (window.__wx_channels_log_panel) {
				const isVisible = window.__wx_channels_log_panel.panel.style.display !== 'none' && 
				                  window.__wx_channels_log_panel.panel.style.opacity !== '0';
				window.__wx_channels_log_panel.toggle();
				// 延迟更新按钮状态，等待动画完成
				setTimeout(function() {
					const nowVisible = window.__wx_channels_log_panel.panel.style.display !== 'none' && 
					                  window.__wx_channels_log_panel.panel.style.opacity !== '0';
					if (nowVisible) {
						btn.style.opacity = '1';
						btn.title = '点击隐藏日志面板';
					} else {
						btn.style.opacity = '0.6';
						btn.title = '点击显示日志面板';
					}
				}, 100);
			}
		}
		
		// 支持点击和触摸事件
		btn.addEventListener('click', togglePanel);
		btn.addEventListener('touchend', function(e) {
			e.preventDefault();
			togglePanel();
		});
		
		btn.title = '点击显示/隐藏日志面板';
		document.body.appendChild(btn);
		
		// 初始状态：面板默认不显示，按钮半透明
		btn.style.opacity = '0.6';
	}
	
	// 页面加载完成后创建面板和按钮
	if (document.readyState === 'loading') {
		document.addEventListener('DOMContentLoaded', function() {
			createLogPanel();
			// 根据配置决定是否创建日志按钮
			if (window.__wx_channels_show_log_button__) {
				createToggleButton();
			}
		});
	} else {
		createLogPanel();
		// 根据配置决定是否创建日志按钮
		if (window.__wx_channels_show_log_button__) {
			createToggleButton();
		}
	}
	
	// 添加快捷键：Ctrl+Shift+L 显示/隐藏日志面板（桌面浏览器可用）
	document.addEventListener('keydown', function(e) {
		if (e.ctrlKey && e.shiftKey && e.key === 'L') {
			e.preventDefault();
			if (window.__wx_channels_log_panel) {
				window.__wx_channels_log_panel.toggle();
				// 同步更新按钮状态
				const btn = document.getElementById('__wx_channels_log_toggle_btn');
				if (btn) {
					setTimeout(function() {
						const isVisible = window.__wx_channels_log_panel.panel.style.display !== 'none' && 
						                  window.__wx_channels_log_panel.panel.style.opacity !== '0';
						if (isVisible) {
							btn.style.opacity = '1';
						} else {
							btn.style.opacity = '0.6';
						}
					}, 100);
				}
			}
		}
	});
	
	// 面板默认不显示，需要点击按钮才会显示
})();
</script>`
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
