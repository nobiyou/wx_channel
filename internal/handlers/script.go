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

	"github.com/fatih/color"
	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// ScriptHandler JavaScript注入处理器
type ScriptHandler struct {
	config      *config.Config
	mainJS      []byte
	zipJS       []byte
	fileSaverJS []byte
	version     string
}

// NewScriptHandler 创建脚本处理器
func NewScriptHandler(cfg *config.Config, mainJS, zipJS, fileSaverJS []byte, version string) *ScriptHandler {
	return &ScriptHandler{
		config:      cfg,
		mainJS:      mainJS,
		zipJS:       zipJS,
		fileSaverJS: fileSaverJS,
		version:     version,
	}
}

// HandleHTMLResponse 处理HTML响应，注入JavaScript代码
func (h *ScriptHandler) HandleHTMLResponse(Conn *SunnyNet.HttpConn, host, path string, body []byte) bool {
	contentType := strings.ToLower(Conn.Response.Header.Get("content-type"))
	if contentType != "text/html; charset=utf-8" {
		return false
	}

	html := string(body)

	// 添加版本号到JS引用
	scriptReg1 := regexp.MustCompile(`src="([^"]{1,})\.js"`)
	html = scriptReg1.ReplaceAllString(html, `src="$1.js`+h.version+`"`)
	scriptReg2 := regexp.MustCompile(`href="([^"]{1,})\.js"`)
	html = scriptReg2.ReplaceAllString(html, `href="$1.js`+h.version+`"`)
	Conn.Response.Header.Set("__debug", "append_script")

	if host == "channels.weixin.qq.com" && (path == "/web/pages/feed" || path == "/web/pages/home" || path == "/web/pages/profile" || path == "/web/pages/s") {
		// 根据页面路径注入不同的脚本
		injectedScripts := h.buildInjectedScripts(path)
		html = strings.Replace(html, "<head>", "<head>\n"+injectedScripts, 1)
		utils.Info("页面已成功加载！")
		utils.Info("已添加视频缓存监控和提醒功能")
		utils.LogInfo("[页面加载] 视频号页面已加载 | Host=%s | Path=%s", host, path)
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
		return true
	}

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(html)))
	return true
}

// HandleJavaScriptResponse 处理JavaScript响应，修改JavaScript代码
func (h *ScriptHandler) HandleJavaScriptResponse(Conn *SunnyNet.HttpConn, host, path string, body []byte) bool {
	contentType := strings.ToLower(Conn.Response.Header.Get("content-type"))
	if contentType != "application/javascript" {
		return false
	}

	// 记录所有JS文件的加载（用于调试）
	utils.LogInfo("[JS文件] %s", path)

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
	content, handled = h.handleFeedDetail(path, content)
	if handled {
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
		return true
	}
	content, handled = h.handleWorkerRelease(path, content)
	if handled {
		Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
		return true
	}
	content, handled = h.handleVuexStores(Conn, path, content)
	if handled {
		return true
	}
	content, handled = h.handleGlobalPublish(Conn, path, content)
	if handled {
		return true
	}
	content, handled = h.handleConnectPublish(Conn, path, content)
	if handled {
		return true
	}
	content, handled = h.handleFinderHomePublish(Conn, path, content)
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

	// 主脚本 - 所有页面都需要
	script := fmt.Sprintf(`<script>%s</script>`, string(h.mainJS))

	// 预加载FileSaver.js库 - 所有页面都需要
	preloadScript := h.getPreloadScript()

	// 下载记录功能 - 所有页面都需要
	downloadTrackerScript := h.getDownloadTrackerScript()

	// 捕获URL脚本 - 所有页面都需要
	captureUrlScript := h.getCaptureUrlScript()

	// 保存页面内容脚本 - 所有页面都需要（用于保存快照）
	savePageContentScript := h.getSavePageContentScript()

	// 根据页面路径决定是否注入特定脚本
	var pageSpecificScripts string

	switch path {
	case "/web/pages/home":
		// Home页面：只注入视频缓存监控脚本
		pageSpecificScripts = h.getVideoCacheNotificationScript()
		utils.LogInfo("[脚本注入] Home页面 - 注入视频缓存监控脚本")

	case "/web/pages/profile":
		// Profile页面（视频列表）：不需要特定脚本
		pageSpecificScripts = ""
		utils.LogInfo("[脚本注入] Profile页面 - 仅注入基础脚本")

	case "/web/pages/feed":
		// Feed页面（视频详情）：注入视频缓存监控和评论采集脚本
		pageSpecificScripts = h.getVideoCacheNotificationScript() + h.getCommentCaptureScript()
		utils.LogInfo("[脚本注入] Feed页面 - 注入视频缓存监控和评论采集脚本")

	case "/web/pages/s":
		// 搜索页面：不需要额外的特定脚本（搜索数据采集已包含在savePageContentScript中）
		pageSpecificScripts = ""
		utils.LogInfo("[脚本注入] 搜索页面 - 注入基础脚本（含搜索数据采集）")

	default:
		// 其他页面：不注入页面特定脚本
		pageSpecificScripts = ""
		utils.LogInfo("[脚本注入] 其他页面 - 仅注入基础脚本")
	}

	return logPanelScript + script + preloadScript + downloadTrackerScript + captureUrlScript + savePageContentScript + pageSpecificScripts
}

// getPreloadScript 获取预加载FileSaver.js库的脚本
func (h *ScriptHandler) getPreloadScript() string {
	return `<script>
	// 预加载FileSaver.js库
	(function() {
		const script = document.createElement('script');
		script.src = '/FileSaver.min.js';
		document.head.appendChild(script);
	})();
	</script>`
}

// getDownloadTrackerScript 获取下载记录功能的脚本
func (h *ScriptHandler) getDownloadTrackerScript() string {
	return `<script>
	// 确保FileSaver.js库已加载
	if (typeof saveAs === 'undefined') {
		console.log('加载FileSaver.js库');
		const script = document.createElement('script');
		script.src = '/FileSaver.min.js';
		script.onload = function() {
			console.log('FileSaver.js库加载成功');
		};
		document.head.appendChild(script);
	}

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
			
			// 记录下载
			if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
				const profile = {...window.__wx_channels_store__.profile};
				window.__wx_channels_record_download(profile);
			}
			
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
			
			// 记录下载
			if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
				const profile = {...window.__wx_channels_store__.profile};
				window.__wx_channels_record_download(profile);
			}
			
			// 3秒后恢复播放（给下载一些时间开始）
			setTimeout(() => {
				window.__wx_channels_resume_video__(pausedVideos);
			}, 3000);
		};
	}
	
	// 修复封面下载函数
	window.__wx_channels_handle_download_cover = function() {
		if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
			const profile = window.__wx_channels_store__.profile;
			// 优先使用thumbUrl，然后是fullThumbUrl，最后才是coverUrl
			const coverUrl = profile.thumbUrl || profile.fullThumbUrl || profile.coverUrl;
			
			if (!coverUrl) {
				alert("未找到封面图片");
				return;
			}
			
			// 注意：下载封面不记录到CSV，只记录视频下载
			
			// 记录日志
			if (window.__wx_log) {
				window.__wx_log({
					msg: '下载封面\n' + coverUrl
				});
			}
			
			// 创建一个隐藏的a标签来下载图片，避免使用saveAs可能导致的确认框问题
			const downloadLink = document.createElement('a');
			downloadLink.href = coverUrl;
			downloadLink.download = "cover_" + profile.id + ".jpg";
			downloadLink.target = "_blank";
			
			// 添加到文档中并模拟点击
			document.body.appendChild(downloadLink);
			downloadLink.click();
			
			// 清理DOM
			setTimeout(() => {
				document.body.removeChild(downloadLink);
			}, 100);
			
			// 备用方法：如果直接下载失败，尝试使用fetch和saveAs
			setTimeout(() => {
				if (typeof saveAs !== 'undefined') {
					fetch(coverUrl)
						.then(response => response.blob())
						.then(blob => {
							saveAs(blob, "cover_" + profile.id + ".jpg");
						})
						.catch(error => {
							console.error("下载封面失败:", error);
							alert("下载封面失败，请重试");
						});
				}
			}, 1000); // 延迟1秒执行备用方法
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
	// 保存当前页面完整内容的函数
	window.__wx_channels_save_page_content = function() {
		try {
			// 获取当前完整的HTML内容
			var fullHtml = document.documentElement.outerHTML;
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
					console.log("页面内容已保存");
				}
			}).catch(error => {
				console.error("保存页面内容失败:", error);
			});
		} catch (error) {
			console.error("获取页面内容失败:", error);
		}
	};
	
	// 监听URL变化，自动保存页面内容
	let currentPageUrl = window.location.href;
	const checkUrlChange = () => {
		if (window.location.href !== currentPageUrl) {
			currentPageUrl = window.location.href;
			// URL变化后延迟保存，等待内容加载（增加到8秒，确保下载菜单已注入）
			setTimeout(() => {
				window.__wx_channels_save_page_content();
			}, 8000);
		}
	};
	
	// 定期检查URL变化（适用于SPA）
	setInterval(checkUrlChange, 1000);
	
	// 监听历史记录变化
	window.addEventListener('popstate', () => {
		setTimeout(() => {
			window.__wx_channels_save_page_content();
		}, 8000);
	});
	
	// 在页面加载完成后也保存一次（增加到10秒，确保所有内容都已加载）
	setTimeout(() => {
		window.__wx_channels_save_page_content();
	}, 10000);
	
	// 搜索页面数据采集功能
	window.__wx_channels_collect_search_data = function() {
		try {
			// 检查是否是搜索页面
			var isSearchPage = window.location.pathname.includes('/pages/s');
			if (!isSearchPage) {
				return;
			}
			
			console.log('[搜索数据采集] 检测到搜索页面，开始初始化...');
			
			// 记录日志
			if (window.__wx_log) {
				window.__wx_log({
					msg: '搜索页面已加载'
				});
			}
			
			// HTML标签清理函数
			var cleanHtmlTags = function(text) {
				if (!text || typeof text !== 'string') return text || '';
				// 创建临时DOM元素来移除HTML标签
				var tempDiv = document.createElement('div');
				tempDiv.innerHTML = text;
				var cleaned = tempDiv.textContent || tempDiv.innerText || '';
				// 处理HTML实体
				var htmlEntities = {
					'&nbsp;': ' ',
					'&amp;': '&',
					'&lt;': '<',
					'&gt;': '>',
					'&quot;': '"',
					'&apos;': "'",
					'&#39;': "'",
					'&#34;': '"'
				};
				for (var entity in htmlEntities) {
					cleaned = cleaned.replace(new RegExp(entity, 'g'), htmlEntities[entity]);
				}
				// 移除剩余的HTML实体
				cleaned = cleaned.replace(/&[a-zA-Z0-9#]+;/g, '');
				return cleaned.trim();
			};
			
			// 初始化批量下载面板（复用主页的批量下载功能）
			try {
				if (typeof window.__wx_channels_profile_collector !== 'undefined') {
					// 设置页面类型为搜索页面
					window.__wx_channels_profile_collector.pageType = 'search';
					// 初始化批量下载UI（如果还没有初始化）
					if (!document.getElementById('wx-channels-batch-download-ui')) {
						window.__wx_channels_profile_collector.addBatchDownloadUI();
						console.log('[搜索数据采集] ✓ 批量下载面板已初始化');
					}
				} else {
					console.warn('[搜索数据采集] 批量下载面板未加载，请确保 inject/main.js 已加载');
				}
			} catch (e) {
				console.error('[搜索数据采集] 初始化批量下载面板失败:', e);
			}
			
			// 从URL中提取搜索关键词
			var urlParams = new URLSearchParams(window.location.search);
			var keyword = urlParams.get('q') || '';
			if (keyword) {
				keyword = decodeURIComponent(keyword);
			}
			console.log('[搜索数据采集] 搜索关键词:', keyword);
			
			// 记录搜索关键词
			if (window.__wx_log && keyword) {
				window.__wx_log({
					msg: '搜索关键词: ' + keyword
				});
			}
			
			// 存储拦截到的搜索数据
			var interceptedSearchData = {
				profiles: [],
				liveResults: [],
				feedResults: [],
				lastUpdate: 0
			};
			
			// 拦截fetch请求（备用方案，主要依赖Store采集）
			var originalFetch = window.fetch;
			window.fetch = function() {
				var args = Array.prototype.slice.call(arguments);
				var url = args[0];
				
				// 检查是否是搜索相关的API（静默拦截，不输出日志）
				if (typeof url === 'string' && (url.includes('/search') || url.includes('/finder/search') || url.includes('searchResult'))) {
					return originalFetch.apply(this, args).then(function(response) {
						// 克隆响应以便读取
						var clonedResponse = response.clone();
						clonedResponse.json().then(function(data) {
							try {
								// 静默处理API响应（备用方案）
								// 尝试从响应中提取数据
								if (data && typeof data === 'object') {
									// 检查各种可能的数据结构
									if (data.data) {
										var responseData = data.data;
										if (Array.isArray(responseData.profileResults) || Array.isArray(responseData.accountResults)) {
											var accounts = responseData.profileResults || responseData.accountResults || [];
											accounts.forEach(function(account) {
												if (account && account.id && !interceptedSearchData.profiles.find(function(p) { return p.id === account.id; })) {
													interceptedSearchData.profiles.push(JSON.parse(JSON.stringify(account)));
												}
											});
										}
										if (Array.isArray(responseData.liveResults)) {
											responseData.liveResults.forEach(function(live) {
												if (live && live.id && !interceptedSearchData.liveResults.find(function(l) { return l.id === live.id; })) {
													interceptedSearchData.liveResults.push(JSON.parse(JSON.stringify(live)));
												}
											});
										}
										if (Array.isArray(responseData.feedResults)) {
											responseData.feedResults.forEach(function(feed) {
												if (feed && feed.id && !interceptedSearchData.feedResults.find(function(f) { return f.id === feed.id; })) {
													interceptedSearchData.feedResults.push(JSON.parse(JSON.stringify(feed)));
												}
											});
										}
										// 如果响应数据直接是数组
										if (Array.isArray(responseData)) {
											responseData.forEach(function(item) {
												if (item && item.id) {
													if (item.type === 'profile' || item.type === 'account') {
														if (!interceptedSearchData.profiles.find(function(p) { return p.id === item.id; })) {
															interceptedSearchData.profiles.push(JSON.parse(JSON.stringify(item)));
														}
													} else if (item.type === 'live') {
														if (!interceptedSearchData.liveResults.find(function(l) { return l.id === item.id; })) {
															interceptedSearchData.liveResults.push(JSON.parse(JSON.stringify(item)));
														}
													} else if (item.type === 'feed' || item.type === 'video') {
														if (!interceptedSearchData.feedResults.find(function(f) { return f.id === item.id; })) {
															interceptedSearchData.feedResults.push(JSON.parse(JSON.stringify(item)));
														}
													}
												}
											});
										}
									}
									// 如果数据直接在根级别
									if (Array.isArray(data.profileResults) || Array.isArray(data.accountResults)) {
										var accounts = data.profileResults || data.accountResults || [];
										accounts.forEach(function(account) {
											if (account && account.id && !interceptedSearchData.profiles.find(function(p) { return p.id === account.id; })) {
												interceptedSearchData.profiles.push(JSON.parse(JSON.stringify(account)));
											}
										});
									}
									if (Array.isArray(data.liveResults)) {
										data.liveResults.forEach(function(live) {
											if (live && live.id && !interceptedSearchData.liveResults.find(function(l) { return l.id === live.id; })) {
												interceptedSearchData.liveResults.push(JSON.parse(JSON.stringify(live)));
											}
										});
									}
									if (Array.isArray(data.feedResults)) {
										data.feedResults.forEach(function(feed) {
											if (feed && feed.id && !interceptedSearchData.feedResults.find(function(f) { return f.id === feed.id; })) {
												interceptedSearchData.feedResults.push(JSON.parse(JSON.stringify(feed)));
											}
										});
									}
									
									interceptedSearchData.lastUpdate = Date.now();
									// 自动保存拦截到的数据（仅在Store采集失败时作为备用）
									// 注意：Store采集是主要方式，API拦截仅作为备用
									if (interceptedSearchData.profiles.length > 0 || 
									    interceptedSearchData.liveResults.length > 0 || 
									    interceptedSearchData.feedResults.length > 0) {
										console.log('[搜索数据采集] [API备用] 从API提取到数据 - 账号:', interceptedSearchData.profiles.length, 
										            ', 直播:', interceptedSearchData.liveResults.length, 
										            ', 动态:', interceptedSearchData.feedResults.length);
										saveInterceptedSearchData();
									}
								}
							} catch (err) {
								console.error('[搜索数据采集] 解析API响应失败:', err);
							}
						}).catch(function(err) {
							console.warn('[搜索数据采集] 读取API响应失败:', err);
						});
						return response;
					});
				}
				return originalFetch.apply(this, args);
			};
			
			// 拦截XMLHttpRequest（备用方案，主要依赖Store采集）
			var originalXHROpen = XMLHttpRequest.prototype.open;
			var originalXHRSend = XMLHttpRequest.prototype.send;
			XMLHttpRequest.prototype.open = function(method, url) {
				this._url = url;
				return originalXHROpen.apply(this, arguments);
			};
			XMLHttpRequest.prototype.send = function() {
				var xhr = this;
				var url = xhr._url;
				if (url && (url.includes('/search') || url.includes('/finder/search') || url.includes('searchResult'))) {
					// 静默拦截XHR请求（备用方案）
					xhr.addEventListener('load', function() {
						if (xhr.readyState === 4 && xhr.status === 200) {
							try {
								var data = JSON.parse(xhr.responseText);
								// 使用与fetch相同的解析逻辑
								if (data && typeof data === 'object') {
									if (data.data) {
										var responseData = data.data;
										if (Array.isArray(responseData.profileResults) || Array.isArray(responseData.accountResults)) {
											var accounts = responseData.profileResults || responseData.accountResults || [];
											accounts.forEach(function(account) {
												if (account && account.id && !interceptedSearchData.profiles.find(function(p) { return p.id === account.id; })) {
													interceptedSearchData.profiles.push(JSON.parse(JSON.stringify(account)));
												}
											});
										}
										if (Array.isArray(responseData.liveResults)) {
											responseData.liveResults.forEach(function(live) {
												if (live && live.id && !interceptedSearchData.liveResults.find(function(l) { return l.id === live.id; })) {
													interceptedSearchData.liveResults.push(JSON.parse(JSON.stringify(live)));
												}
											});
										}
										if (Array.isArray(responseData.feedResults)) {
											responseData.feedResults.forEach(function(feed) {
												if (feed && feed.id && !interceptedSearchData.feedResults.find(function(f) { return f.id === feed.id; })) {
													interceptedSearchData.feedResults.push(JSON.parse(JSON.stringify(feed)));
												}
											});
										}
									}
									interceptedSearchData.lastUpdate = Date.now();
									// 仅在Store采集失败时作为备用
									if (interceptedSearchData.profiles.length > 0 || 
									    interceptedSearchData.liveResults.length > 0 || 
									    interceptedSearchData.feedResults.length > 0) {
										console.log('[搜索数据采集] [XHR备用] 从XHR提取到数据');
										saveInterceptedSearchData();
									}
								}
							} catch (err) {
								console.error('[搜索数据采集] 解析XHR响应失败:', err);
							}
						}
					});
				}
				return originalXHRSend.apply(this, arguments);
			};
			
			// 保存拦截到的搜索数据
			// 记录上次保存的数据数量，用于判断是否有变化
			var lastSavedCount = {
				profiles: 0,
				liveResults: 0,
				feedResults: 0
			};
			
			var saveInterceptedSearchData = function(forceSave) {
				// 检查数据是否有变化
				var hasChange = forceSave || 
					interceptedSearchData.profiles.length !== lastSavedCount.profiles ||
					interceptedSearchData.liveResults.length !== lastSavedCount.liveResults ||
					interceptedSearchData.feedResults.length !== lastSavedCount.feedResults;
				
				if (!hasChange) {
					// 数据没有变化，不保存
					return;
				}
				
				if (interceptedSearchData.profiles.length > 0 || 
				    interceptedSearchData.liveResults.length > 0 || 
				    interceptedSearchData.feedResults.length > 0) {
					console.log('[搜索数据采集] 保存拦截到的数据 - 账号:', interceptedSearchData.profiles.length, 
					            ', 直播:', interceptedSearchData.liveResults.length, 
					            ', 动态:', interceptedSearchData.feedResults.length);
					fetch('/__wx_channels_api/save_search_data', {
						method: 'POST',
						headers: {
							'Content-Type': 'application/json'
						},
						body: JSON.stringify({
							url: window.location.href,
							keyword: keyword,
							profiles: interceptedSearchData.profiles,
							liveResults: interceptedSearchData.liveResults,
							feedResults: interceptedSearchData.feedResults,
							timestamp: Date.now()
						})
					}).then(function(response) {
						if (response.ok) {
							console.log('[搜索数据采集] ✓ 拦截数据已保存');
							// 更新最后保存的数据数量
							lastSavedCount = {
								profiles: interceptedSearchData.profiles.length,
								liveResults: interceptedSearchData.liveResults.length,
								feedResults: interceptedSearchData.feedResults.length
							};
						}
					}).catch(function(error) {
						console.error('[搜索数据采集] ✗ 保存拦截数据失败:', error);
					});
				}
			};
			
			// 从Store收集搜索数据的函数（主要方式：直接访问Pinia Store）
			var collectSearchData = function() {
				console.log('[搜索数据采集] 尝试从Store采集数据...');
				
				// 优先方法：直接检查 appContext.$pinia.state._value.search（已知的数据路径）
				var searchStore = null;
				try {
					// 快速路径：使用与完整搜索相同的选择器
					var rootElements = document.querySelectorAll('[data-v-app], [id="app"], [id="__nuxt"], [class*="app"], [class*="root"], body > div');
					for (var re = 0; re < Math.min(rootElements.length, 4); re++) {
						var el = rootElements[re];
						// 检查所有可能的Vue实例属性（与完整搜索保持一致）
						var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
						if (vueInstance) {
							// 检查是否是VNode（虚拟节点）
							var isVNode = vueInstance.__v_isVNode || vueInstance.type !== undefined && vueInstance.props !== undefined;
							var componentInstance = null;
							
							if (isVNode) {
								// 从VNode的component属性获取组件实例
								if (vueInstance.component) {
									componentInstance = vueInstance.component;
								}
							} else {
								// 可能是组件实例
								componentInstance = vueInstance;
							}
							
							// 获取appContext（多种方式尝试）
							var appContext = null;
							if (componentInstance) {
								if (componentInstance.appContext) {
									appContext = componentInstance.appContext;
								} else if (componentInstance.setupState && componentInstance.setupState.__appContext) {
									appContext = componentInstance.setupState.__appContext;
								} else if (componentInstance.ctx && componentInstance.ctx.appContext) {
									appContext = componentInstance.ctx.appContext;
								}
							}
							// 如果componentInstance没有appContext，尝试从vueInstance本身获取
							if (!appContext && vueInstance.appContext) {
								appContext = vueInstance.appContext;
							}
							
							// 检查appContext中的Pinia
							if (appContext && appContext.config && appContext.config.globalProperties && appContext.config.globalProperties.$pinia) {
								var pinia = appContext.config.globalProperties.$pinia;
								if (pinia.state && pinia.state._value && pinia.state._value.search) {
									console.log('[搜索数据采集] ✓ 快速路径找到 $pinia.state._value.search');
									searchStore = pinia.state._value.search;
									break;
								}
							}
						}
					}
				} catch (e) {
					console.warn('[搜索数据采集] 快速路径检查失败:', e.message);
				}
				
				// 如果快速路径找到了，直接提取数据，跳过所有其他检查
				if (searchStore) {
					console.log('[搜索数据采集] ✓ 使用快速路径找到搜索Store，跳过其他检查');
					// 直接跳转到数据提取部分，不执行后续的完整搜索
				} else {
					// 如果快速路径没找到，使用完整搜索（保留作为备用）
					console.log('[搜索数据采集] 快速路径未找到，使用完整搜索...');
					
					// 方法1: 尝试从全局 store 中获取搜索数据
				var findSearchStore = function(obj, depth, path) {
					if (depth > 5 || !obj || typeof obj !== 'object' || obj === window) return null;
					if (!path) path = '';
					
					// 检查是否包含搜索结果的典型字段
					var hasFeedResults = Array.isArray(obj.feedResults);
					var hasProfileResults = Array.isArray(obj.profileResults);
					var hasLiveResults = Array.isArray(obj.liveResults);
					var hasAccountResults = Array.isArray(obj.accountResults);
					var hasSearchResults = Array.isArray(obj.searchResults);
					var hasFeeds = Array.isArray(obj.feeds);
					var hasSearchFields = typeof obj.lastContent !== 'undefined' || 
					                      typeof obj.isSearchingMoreFeeds !== 'undefined' || 
					                      typeof obj.noMoreFeeds !== 'undefined' ||
					                      typeof obj.searchKeyword !== 'undefined' ||
					                      typeof obj.query !== 'undefined';
					
					// 特殊检查：如果对象有 search 属性，检查 search 对象内部
					if (obj.search && typeof obj.search === 'object') {
						var searchObj = obj.search;
						if (Array.isArray(searchObj.feedResults) || Array.isArray(searchObj.profileResults) || 
						    Array.isArray(searchObj.results) || Array.isArray(searchObj.feeds) ||
						    Array.isArray(searchObj.liveResults) || Array.isArray(searchObj.accountResults)) {
							console.log('[搜索数据采集] 在路径', path + '.search', '发现搜索数据');
							return searchObj; // 返回 search 对象
						}
					}
					
					// 调试：记录找到的数组字段
					if (hasFeedResults || hasProfileResults || hasLiveResults || hasAccountResults || hasSearchResults || hasFeeds) {
						var foundFields = [];
						if (hasFeedResults) foundFields.push('feedResults(' + obj.feedResults.length + ')');
						if (hasProfileResults) foundFields.push('profileResults(' + obj.profileResults.length + ')');
						if (hasLiveResults) foundFields.push('liveResults(' + obj.liveResults.length + ')');
						if (hasAccountResults) foundFields.push('accountResults(' + obj.accountResults.length + ')');
						if (hasSearchResults) foundFields.push('searchResults(' + obj.searchResults.length + ')');
						if (hasFeeds) foundFields.push('feeds(' + obj.feeds.length + ')');
						console.log('[搜索数据采集] 在路径', path, '发现搜索结果字段:', foundFields.join(', '), hasSearchFields ? '(包含搜索字段)' : '(缺少搜索字段)');
					}
					
					// 放宽条件：只要有搜索结果字段就返回（即使没有搜索字段）
					if (hasFeedResults || hasProfileResults || hasLiveResults || hasAccountResults || hasSearchResults || hasFeeds) {
						console.log('[搜索数据采集] 找到搜索Store，路径:', path);
						return obj;
					}
					
					// 递归查找
					try {
						var keys = Object.keys(obj);
						for (var i = 0; i < Math.min(keys.length, 50); i++) {
							var k = keys[i];
							if (k === '__proto__' || k === 'constructor' || k === 'prototype') continue;
							try {
								var result = findSearchStore(obj[k], depth + 1, path + '.' + k);
								if (result) return result;
							} catch (e) {}
						}
					} catch (e) {}
					return null;
				};
				
					// 跳过全局对象检查（已知不包含搜索数据，直接进入组件实例检查）
				
					// 方法2: 尝试从 Vuex/Pinia store 实例获取（静默检查）
					if (!searchStore && window.__VUE_DEVTOOLS_GLOBAL_HOOK__) {
						var instances = window.__VUE_DEVTOOLS_GLOBAL_HOOK__.apps || [];
						for (var i = 0; i < instances.length; i++) {
							var app = instances[i];
							if (app && app.config && app.config.globalProperties) {
								// 优先检查 $pinia
								if (app.config.globalProperties.$pinia) {
									var pinia = app.config.globalProperties.$pinia;
									if (pinia.state && pinia.state._value && pinia.state._value.search) {
										searchStore = pinia.state._value.search;
										break;
									}
									if (pinia._s) {
										for (var storeId in pinia._s) {
											var piniaStore = pinia._s[storeId];
											if (piniaStore && piniaStore.$state) {
												searchStore = findSearchStore(piniaStore.$state, 0, 'pinia.' + storeId);
												if (searchStore) break;
											}
										}
									}
								}
								// 尝试 $store
								if (!searchStore && app.config.globalProperties.$store) {
									var store = app.config.globalProperties.$store;
									if (store.state && store.state.search && typeof store.state.search === 'object') {
										searchStore = findSearchStore(store.state.search, 0, 'store.state.search');
									}
									if (!searchStore && store.state) {
										searchStore = findSearchStore(store.state, 0, 'store.state');
									}
								}
								if (searchStore) break;
							}
						}
					}
				
					// 方法3: 尝试从Vue组件实例中查找（静默检查，减少日志）
					if (!searchStore) {
					try {
						// 扩展选择器，查找更多可能的根元素
						var rootElements = document.querySelectorAll('[data-v-app], [id="app"], [id="__nuxt"], [class*="app"], [class*="root"], body > div');
						for (var e = 0; e < Math.min(rootElements.length, 4); e++) {
							var el = rootElements[e];
							// 检查所有可能的Vue实例属性
							var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
							if (vueInstance) {
								// 检查是否是VNode（虚拟节点）
								var isVNode = vueInstance.__v_isVNode || vueInstance.type !== undefined && vueInstance.props !== undefined;
								var componentInstance = null;
								
								if (isVNode) {
									// 从VNode的component属性获取组件实例
									if (vueInstance.component) {
										componentInstance = vueInstance.component;
									}
								} else {
									// 可能是组件实例
									componentInstance = vueInstance;
								}
								
								// 如果找到了组件实例，检查它（优先检查Pinia）
								if (componentInstance) {
									// 尝试多种方式访问store
									if (componentInstance.$store) {
										console.log('[搜索数据采集] 找到 $store');
										var store = componentInstance.$store;
										// 输出store的结构信息
										try {
											var storeKeys = [];
											for (var sk in store) {
												storeKeys.push(sk);
											}
											console.log('[搜索数据采集] $store键:', storeKeys.slice(0, 20).join(', '));
										} catch (err) {}
										
										// 检查Vuex store的模块（递归检查所有嵌套模块）
										var checkVuexModules = function(moduleNode, modulePath) {
											if (!moduleNode || !moduleNode._children) return;
											var moduleKeys = Object.keys(moduleNode._children);
											for (var mk = 0; mk < moduleKeys.length; mk++) {
												var moduleName = moduleKeys[mk];
												var module = moduleNode._children[moduleName];
												var currentPath = modulePath ? modulePath + '.' + moduleName : moduleName;
												if (module && module.state) {
													console.log('[搜索数据采集] 检查模块', currentPath);
													// 输出模块state的键（用于调试）
													try {
														var stateKeys = Object.keys(module.state);
														console.log('[搜索数据采集] 模块', currentPath, 'state键:', stateKeys.slice(0, 15).join(', '), '(共', stateKeys.length, '个)');
													} catch (e) {}
													searchStore = findSearchStore(module.state, 0, 'component.$store.module.' + currentPath);
													if (searchStore) return true;
													// 递归检查嵌套模块
													if (module._children) {
														if (checkVuexModules(module, currentPath)) return true;
													}
												}
											}
											return false;
										};
										
										if (store._modules && store._modules.root) {
											console.log('[搜索数据采集] 检查Vuex模块...');
											if (checkVuexModules(store._modules.root, '')) {
												if (searchStore) break;
											}
										}
										
										// 检查store.state（特别检查 state.search 模块）
										if (!searchStore && store.state) {
											console.log('[搜索数据采集] 检查 $store.state');
											// 优先检查 state.search（根据旧快照文件的线索）
											if (store.state.search && typeof store.state.search === 'object') {
												console.log('[搜索数据采集] 发现 state.search 对象，检查其内容');
												searchStore = findSearchStore(store.state.search, 0, 'component.$store.state.search');
											}
											if (!searchStore) {
												searchStore = findSearchStore(store.state, 0, 'component.$store.state');
											}
										}
										if (searchStore) break;
									}
									if (componentInstance.$pinia) {
										console.log('[搜索数据采集] 找到 $pinia');
										var pinia = componentInstance.$pinia;
										// 输出pinia的结构信息
										try {
											var piniaKeys = [];
											for (var pk in pinia) {
												piniaKeys.push(pk);
											}
											console.log('[搜索数据采集] $pinia键:', piniaKeys.slice(0, 20).join(', '));
										} catch (err) {}
										
										// 优先检查 pinia.state._value.search（根据实际找到的路径）
										if (pinia.state && pinia.state._value && pinia.state._value.search) {
											console.log('[搜索数据采集] 发现 $pinia.state._value.search，直接检查');
											searchStore = findSearchStore(pinia.state._value.search, 0, 'component.$pinia.state._value.search');
											if (searchStore) break;
										}
										
										// 检查Pinia store
										if (!searchStore && pinia._s) {
											console.log('[搜索数据采集] 检查Pinia stores...');
											var storeIds = Object.keys(pinia._s);
											console.log('[搜索数据采集] Pinia store IDs:', storeIds.join(', '));
											for (var storeId in pinia._s) {
												var piniaStore = pinia._s[storeId];
												if (piniaStore && piniaStore.$state) {
													console.log('[搜索数据采集] 检查Pinia store:', storeId);
													searchStore = findSearchStore(piniaStore.$state, 0, 'component.$pinia.' + storeId);
													if (searchStore) break;
												}
											}
										}
										
										// 如果还没找到，递归查找整个 pinia 对象
										if (!searchStore) {
											searchStore = findSearchStore(pinia, 0, 'component.$pinia');
										}
										if (searchStore) break;
									}
									
									// 尝试从组件实例本身查找
									searchStore = findSearchStore(componentInstance, 0, 'component.instance');
									if (searchStore) break;
									
									// 尝试从组件的setupState查找（Vue 3 Composition API）
									if (componentInstance.setupState) {
										console.log('[搜索数据采集] 找到 setupState');
										try {
											var setupKeys = Object.keys(componentInstance.setupState);
											console.log('[搜索数据采集] setupState键:', setupKeys.slice(0, 20).join(', '), '(共', setupKeys.length, '个)');
										} catch (e) {}
										searchStore = findSearchStore(componentInstance.setupState, 0, 'component.setupState');
										if (searchStore) break;
									}
									
									// 尝试从ctx查找（Vue 3 Options API）
									if (componentInstance.ctx) {
										console.log('[搜索数据采集] 找到 ctx');
										try {
											var ctxKeys = Object.keys(componentInstance.ctx);
											console.log('[搜索数据采集] ctx键:', ctxKeys.slice(0, 20).join(', '), '(共', ctxKeys.length, '个)');
										} catch (e) {}
										searchStore = findSearchStore(componentInstance.ctx, 0, 'component.ctx');
										if (searchStore) break;
									}
									
									// 尝试从exposed查找（Vue 3 expose）
									if (componentInstance.exposed) {
										console.log('[搜索数据采集] 找到 exposed');
										searchStore = findSearchStore(componentInstance.exposed, 0, 'component.exposed');
										if (searchStore) break;
									}
									
									// 尝试从parent查找（向上遍历组件树）
									if (componentInstance.parent) {
										console.log('[搜索数据采集] 找到 parent，尝试向上查找');
										var parent = componentInstance.parent;
										for (var p = 0; p < 5 && parent; p++) {
											if (parent.$store) {
												searchStore = findSearchStore(parent.$store.state, 0, 'component.parent[' + p + '].$store');
												if (searchStore) break;
											}
											if (parent.$pinia) {
												searchStore = findSearchStore(parent.$pinia, 0, 'component.parent[' + p + '].$pinia');
												if (searchStore) break;
											}
											parent = parent.parent;
										}
										if (searchStore) break;
									}
								}
								
								// 尝试从appContext查找（Vue 3）- 无论是否是VNode都有appContext
								if (vueInstance.appContext) {
									console.log('[搜索数据采集] 找到 appContext');
									var appContext = vueInstance.appContext;
									
									// 输出appContext的结构
									try {
										var acKeys = [];
										for (var ack in appContext) {
											acKeys.push(ack);
										}
										console.log('[搜索数据采集] appContext键:', acKeys.join(', '));
									} catch (err) {}
									
									// 检查config.globalProperties
									if (appContext.config && appContext.config.globalProperties) {
										console.log('[搜索数据采集] 检查 appContext.config.globalProperties');
										if (appContext.config.globalProperties.$store) {
											console.log('[搜索数据采集] 找到 $store (appContext)');
											var store = appContext.config.globalProperties.$store;
											
											// 检查Vuex store的模块（递归检查所有嵌套模块）
											var checkVuexModules = function(moduleNode, modulePath) {
												if (!moduleNode || !moduleNode._children) return;
												var moduleKeys = Object.keys(moduleNode._children);
												for (var mk = 0; mk < moduleKeys.length; mk++) {
													var moduleName = moduleKeys[mk];
													var module = moduleNode._children[moduleName];
													var currentPath = modulePath ? modulePath + '.' + moduleName : moduleName;
													if (module && module.state) {
														console.log('[搜索数据采集] 检查模块', currentPath);
														// 输出模块state的键（用于调试）
														try {
															var stateKeys = Object.keys(module.state);
															console.log('[搜索数据采集] 模块', currentPath, 'state键:', stateKeys.slice(0, 15).join(', '), '(共', stateKeys.length, '个)');
														} catch (e) {}
														searchStore = findSearchStore(module.state, 0, 'appContext.$store.module.' + currentPath);
														if (searchStore) return true;
														// 递归检查嵌套模块
														if (module._children) {
															if (checkVuexModules(module, currentPath)) return true;
														}
													}
												}
												return false;
											};
											
											if (store._modules && store._modules.root) {
												console.log('[搜索数据采集] 检查Vuex模块 (appContext)...');
												if (checkVuexModules(store._modules.root, '')) {
													if (searchStore) break;
												}
											}
											
											// 检查store.state（输出state的键用于调试，特别检查 state.search）
											if (!searchStore && store.state) {
												console.log('[搜索数据采集] 检查 $store.state (appContext)');
												try {
													var stateKeys = Object.keys(store.state);
													console.log('[搜索数据采集] $store.state键:', stateKeys.slice(0, 20).join(', '), '(共', stateKeys.length, '个)');
													// 优先检查 state.search（根据旧快照文件的线索）
													if (store.state.search && typeof store.state.search === 'object') {
														console.log('[搜索数据采集] 发现 state.search 对象 (appContext)，检查其内容');
														searchStore = findSearchStore(store.state.search, 0, 'appContext.$store.state.search');
													}
												} catch (e) {}
												if (!searchStore) {
													searchStore = findSearchStore(store.state, 0, 'appContext.$store.state');
												}
											}
											if (searchStore) break;
										}
										// 优先检查 $pinia（已知路径）
										if (appContext.config.globalProperties.$pinia) {
											var pinia = appContext.config.globalProperties.$pinia;
											if (pinia.state && pinia.state._value && pinia.state._value.search) {
												searchStore = pinia.state._value.search;
												break;
											}
											if (pinia._s) {
												for (var storeId in pinia._s) {
													var piniaStore = pinia._s[storeId];
													if (piniaStore && piniaStore.$state) {
														searchStore = findSearchStore(piniaStore.$state, 0, 'appContext.pinia.' + storeId);
														if (searchStore) break;
													}
												}
											}
											if (searchStore) break;
										}
									}
									
									// 检查provides（Vue 3依赖注入）
									if (appContext.provides) {
										console.log('[搜索数据采集] 检查 appContext.provides');
										try {
											var providesKeys = Object.keys(appContext.provides);
											console.log('[搜索数据采集] provides键:', providesKeys.slice(0, 20).join(', '), '(共', providesKeys.length, '个)');
											// 明确检查 provides.store（如果有的话）
											if (appContext.provides.store) {
												console.log('[搜索数据采集] 找到 appContext.provides.store');
												var providesStore = appContext.provides.store;
												// 检查是否是Vuex store
												if (providesStore.state) {
													console.log('[搜索数据采集] provides.store 是Vuex store，检查state');
													// 检查Vuex模块
													if (providesStore._modules && providesStore._modules.root) {
														var checkVuexModules = function(moduleNode, modulePath) {
															if (!moduleNode || !moduleNode._children) return;
															var moduleKeys = Object.keys(moduleNode._children);
															for (var mk = 0; mk < moduleKeys.length; mk++) {
																var moduleName = moduleKeys[mk];
																var module = moduleNode._children[moduleName];
																var currentPath = modulePath ? modulePath + '.' + moduleName : moduleName;
																if (module && module.state) {
																	console.log('[搜索数据采集] 检查provides.store模块', currentPath);
																	searchStore = findSearchStore(module.state, 0, 'provides.store.module.' + currentPath);
																	if (searchStore) return true;
																	if (module._children) {
																		if (checkVuexModules(module, currentPath)) return true;
																	}
																}
															}
															return false;
														};
														if (checkVuexModules(providesStore._modules.root, '')) {
															if (searchStore) break;
														}
													}
													// 检查state本身（特别检查 state.search）
													if (!searchStore && providesStore.state) {
														// 优先检查 state.search（根据旧快照文件的线索）
														if (providesStore.state.search && typeof providesStore.state.search === 'object') {
															console.log('[搜索数据采集] 发现 state.search 对象 (provides.store)，检查其内容');
															searchStore = findSearchStore(providesStore.state.search, 0, 'provides.store.state.search');
														}
														if (!searchStore) {
															searchStore = findSearchStore(providesStore.state, 0, 'provides.store.state');
														}
														if (searchStore) break;
													}
												} else {
													// 可能是其他类型的store，直接查找
													searchStore = findSearchStore(providesStore, 0, 'provides.store');
													if (searchStore) break;
												}
											}
										} catch (e) {
											console.log('[搜索数据采集] 检查provides时出错:', e.message);
										}
										// 如果还没找到，递归查找整个provides对象
										if (!searchStore) {
											searchStore = findSearchStore(appContext.provides, 0, 'component.appContext.provides');
											if (searchStore) break;
										}
									}
									
									// 检查appContext本身
									searchStore = findSearchStore(appContext, 0, 'component.appContext');
									if (searchStore) break;
									
									// 检查app（Vue应用实例）
									if (appContext.app) {
										console.log('[搜索数据采集] 检查 appContext.app');
										var app = appContext.app;
										if (app.config && app.config.globalProperties) {
											if (app.config.globalProperties.$store) {
												searchStore = findSearchStore(app.config.globalProperties.$store.state, 0, 'component.appContext.app.$store');
												if (searchStore) break;
											}
											if (app.config.globalProperties.$pinia) {
												var pinia = app.config.globalProperties.$pinia;
												if (pinia._s) {
													for (var storeId in pinia._s) {
														var piniaStore = pinia._s[storeId];
														if (piniaStore && piniaStore.$state) {
															searchStore = findSearchStore(piniaStore.$state, 0, 'component.appContext.app.pinia.' + storeId);
															if (searchStore) break;
														}
													}
												}
												if (searchStore) break;
											}
										}
									}
								}
								
								// 如果还没找到，尝试从VNode本身查找（可能数据在props或其他地方）
								if (!searchStore && isVNode) {
									console.log('[搜索数据采集] 尝试从VNode本身查找');
									searchStore = findSearchStore(vueInstance, 0, 'vnode');
									if (searchStore) break;
									if (vueInstance.props) {
										searchStore = findSearchStore(vueInstance.props, 0, 'vnode.props');
										if (searchStore) break;
									}
								}
							}
						}
					} catch (err) {
						console.warn('[搜索数据采集] 从组件实例查找失败:', err);
					}
				}
				
					// 方法4: 尝试从window对象中深度查找（静默检查，最后手段）
					if (!searchStore) {
						var windowKeys = ['__APP__', '__VUE__', '__NUXT__', '__INITIAL_STATE__', 'app', 'store', 'vue', 'Vue', 'vueApp', 'vuex', 'pinia'];
						for (var wk = 0; wk < windowKeys.length; wk++) {
							var key = windowKeys[wk];
							if (window[key]) {
								searchStore = findSearchStore(window[key], 0, 'window.' + key);
								if (searchStore) break;
							}
						}
					}
				}
				
				// 从 store 中提取数据并合并到拦截数据中
				if (searchStore) {
					console.log('[搜索数据采集] 找到搜索Store，开始提取数据...');
					
					// 收集账号信息 (profileResults/accountResults)
					if (Array.isArray(searchStore.profileResults)) {
						console.log('[搜索数据采集] 从Store找到profileResults，数量:', searchStore.profileResults.length);
						searchStore.profileResults.forEach(function(profile) {
							if (profile && profile.id && !interceptedSearchData.profiles.find(function(p) { return p.id === profile.id; })) {
								interceptedSearchData.profiles.push(JSON.parse(JSON.stringify(profile)));
							}
						});
					}
					if (Array.isArray(searchStore.accountResults)) {
						console.log('[搜索数据采集] 从Store找到accountResults，数量:', searchStore.accountResults.length);
						var addedCount = 0;
						var skippedCount = 0;
						searchStore.accountResults.forEach(function(account, index) {
							if (!account || typeof account !== 'object') {
								skippedCount++;
								return;
							}
							// 检查多种可能的ID字段（包括contact对象中的ID）
							var accountId = account.id || account.accountId || account.username || account.nickname || account.finderUsername;
							// 如果account对象有contact属性，检查contact中的ID字段
							if (!accountId && account.contact && typeof account.contact === 'object') {
								accountId = account.contact.id || account.contact.username || account.contact.finderUsername || 
								           account.contact.accountId || account.contact.nickname;
							}
							
							if (!accountId) {
								// 如果前3个都没有ID，输出调试信息
								if (index < 3) {
									var accountKeys = Object.keys(account);
									console.log('[搜索数据采集] accountResults[' + index + '] 没有找到标准ID字段，键:', accountKeys.slice(0, 15).join(', '));
									if (account.contact) {
										var contactKeys = Object.keys(account.contact);
										console.log('[搜索数据采集] accountResults[' + index + '].contact 键:', contactKeys.slice(0, 10).join(', '));
									}
								}
								// 使用其他唯一标识符进行去重（如contact.username的组合）
								var uniqueKey = null;
								if (account.contact && account.contact.username) {
									uniqueKey = 'contact_username_' + account.contact.username;
								} else if (account.highlightNickname) {
									uniqueKey = 'nickname_' + account.highlightNickname;
								} else if (account.reqIndex !== undefined) {
									uniqueKey = 'reqIndex_' + account.reqIndex;
								}
								
								if (uniqueKey) {
									// 检查是否已存在（使用唯一键）
									var exists = interceptedSearchData.profiles.find(function(p) {
										if (p.contact && p.contact.username && account.contact && account.contact.username) {
											return p.contact.username === account.contact.username;
										}
										if (p.highlightNickname && account.highlightNickname) {
											return p.highlightNickname === account.highlightNickname;
										}
										if (p.reqIndex !== undefined && account.reqIndex !== undefined) {
											return p.reqIndex === account.reqIndex;
										}
										return false;
									});
									if (!exists) {
										interceptedSearchData.profiles.push(JSON.parse(JSON.stringify(account)));
										addedCount++;
									} else {
										skippedCount++;
									}
								} else {
									// 如果连唯一键都没有，也添加（但标记为可能重复）
									interceptedSearchData.profiles.push(JSON.parse(JSON.stringify(account)));
									addedCount++;
								}
								return;
							}
							// 检查是否已存在（使用多种ID字段匹配）
							var exists = interceptedSearchData.profiles.find(function(p) {
								var pId = p.id || p.accountId || p.username || p.finderUsername;
								if (!pId && p.contact && typeof p.contact === 'object') {
									pId = p.contact.id || p.contact.username || p.contact.finderUsername || 
									      p.contact.accountId || p.contact.nickname;
								}
								return pId === accountId;
							});
							if (!exists) {
								interceptedSearchData.profiles.push(JSON.parse(JSON.stringify(account)));
								addedCount++;
							} else {
								skippedCount++;
							}
						});
						console.log('[搜索数据采集] accountResults处理完成 - 添加:', addedCount, ', 跳过:', skippedCount, ', profiles总数:', interceptedSearchData.profiles.length);
					}
					
					// 收集直播数据 (liveResults)
					if (Array.isArray(searchStore.liveResults)) {
						console.log('[搜索数据采集] 从Store找到liveResults，数量:', searchStore.liveResults.length);
						var liveAddedCount = 0;
						searchStore.liveResults.forEach(function(live) {
							if (live && live.id && !interceptedSearchData.liveResults.find(function(l) { return l.id === live.id; })) {
								interceptedSearchData.liveResults.push(JSON.parse(JSON.stringify(live)));
								liveAddedCount++;
							}
						});
						console.log('[搜索数据采集] liveResults处理完成 - 添加:', liveAddedCount, ', liveResults总数:', interceptedSearchData.liveResults.length);
					}
					
					// 收集动态数据 (feedResults)
					if (Array.isArray(searchStore.feedResults)) {
						console.log('[搜索数据采集] 从Store找到feedResults，数量:', searchStore.feedResults.length);
						var feedAddedCount = 0;
						searchStore.feedResults.forEach(function(feed) {
							if (feed && feed.id && !interceptedSearchData.feedResults.find(function(f) { return f.id === feed.id; })) {
								interceptedSearchData.feedResults.push(JSON.parse(JSON.stringify(feed)));
								feedAddedCount++;
							}
						});
						console.log('[搜索数据采集] feedResults处理完成 - 添加:', feedAddedCount, ', feedResults总数:', interceptedSearchData.feedResults.length);
					}
					
					// 如果从Store获取到数据，保存一次
					console.log('[搜索数据采集] Store数据提取完成 - profiles:', interceptedSearchData.profiles.length, 
					            ', liveResults:', interceptedSearchData.liveResults.length, 
					            ', feedResults:', interceptedSearchData.feedResults.length);
					
					// 将搜索数据暴露到全局对象，供前端导出使用
					if (!window.__wx_channels_search_data) {
						window.__wx_channels_search_data = {};
					}
					window.__wx_channels_search_data.profiles = interceptedSearchData.profiles;
					window.__wx_channels_search_data.liveResults = interceptedSearchData.liveResults;
					window.__wx_channels_search_data.feedResults = interceptedSearchData.feedResults;
					window.__wx_channels_search_data.keyword = keyword;
					
					// 只在数据有变化时才保存（在checkAndUpdateData中会处理保存）
					// 这里不保存，避免重复保存
					
					// 将 feedResults 转换为批量下载面板需要的格式，并添加到批量下载面板
					if (interceptedSearchData.feedResults.length > 0) {
						try {
							// 确保批量下载面板已初始化
							if (typeof window.__wx_channels_profile_collector === 'undefined') {
								console.log('[搜索数据采集] 批量下载面板未初始化，等待初始化...');
								// 延迟重试，等待批量下载面板初始化
								var retryCount = 0;
								var maxRetries = 10;
								var retryInterval = setInterval(function() {
									retryCount++;
									if (typeof window.__wx_channels_profile_collector !== 'undefined') {
										clearInterval(retryInterval);
										console.log('[搜索数据采集] 批量下载面板已初始化，开始添加视频');
										// 重新处理feedResults
										var videoCount = 0;
										interceptedSearchData.feedResults.forEach(function(feed) {
											if (feed && feed.id && feed.objectDesc && feed.objectDesc.media && feed.objectDesc.media.length > 0) {
												var media = feed.objectDesc.media[0];
												if (media.mediaType === 4 && media.spec && media.spec.length > 0) {
													var videoUrl = '';
													if (media.url) {
														videoUrl = media.url + (media.urlToken || '');
													} else if (media.fullUrl) {
														videoUrl = media.fullUrl;
													}
													
													var videoData = {
														type: 'media',
														id: feed.id,
														nonce_id: feed.objectNonceId || feed.id,
														title: cleanHtmlTags(feed.objectDesc.description) || '未命名视频',
														coverUrl: media.coverUrl || media.fullCoverUrl || '',
														thumbUrl: media.thumbUrl || media.fullThumbUrl || '',
														fullThumbUrl: media.fullThumbUrl || '',
														url: videoUrl,
														size: parseInt(media.fileSize || media.cdnFileSize || '0'),
														key: media.decodeKey || '',
														duration: media.spec[0].durationMs || 0,
														spec: media.spec.map(function(s) {
															return {
																width: s.width || 0,
																height: s.height || 0,
																bitrate: s.videoBitrate || s.bitRate || 0,
																fileFormat: s.fileFormat || '',
																durationMs: s.durationMs || 0
															};
														}),
														fileFormat: media.spec.map(function(o) { return o.fileFormat; }),
														nickname: feed.contact ? feed.contact.nickname : '',
														username: feed.contact ? feed.contact.username : '',
														createtime: feed.createtime || 0,
														contact: feed.contact || {},
														readCount: feed.readCount || 0,
														likeCount: feed.likeCount || 0,
														commentCount: feed.commentCount || 0,
														favCount: feed.favCount || 0,
														forwardCount: feed.forwardCount || 0,
														ipRegionInfo: feed.ipRegionInfo || {},
														mediaType: media.mediaType,
														timestamp: Date.now()
													};
													
													window.__wx_channels_profile_collector.addVideoFromAPI(videoData);
													videoCount++;
												}
											}
										});
										if (videoCount > 0) {
											console.log('[搜索数据采集] ✓ 延迟添加：已将', videoCount, '个视频添加到批量下载面板');
										}
									} else if (retryCount >= maxRetries) {
										clearInterval(retryInterval);
										console.warn('[搜索数据采集] 批量下载面板初始化超时，无法添加视频');
									}
								}, 500);
							} else {
								var videoCount = 0;
								interceptedSearchData.feedResults.forEach(function(feed) {
									if (feed && feed.id && feed.objectDesc && feed.objectDesc.media && feed.objectDesc.media.length > 0) {
										var media = feed.objectDesc.media[0];
										// 只处理视频类型（mediaType === 4）
										if (media.mediaType === 4 && media.spec && media.spec.length > 0) {
											// 转换为批量下载面板需要的格式
											// 拼接视频URL（url + urlToken）
											var videoUrl = '';
											if (media.url) {
												videoUrl = media.url + (media.urlToken || '');
											} else if (media.fullUrl) {
												videoUrl = media.fullUrl;
											}
											
											var videoData = {
												type: 'media',
												id: feed.id,
												nonce_id: feed.objectNonceId || feed.id,
												title: cleanHtmlTags(feed.objectDesc.description) || '未命名视频',
												coverUrl: media.coverUrl || media.fullCoverUrl || '',
												thumbUrl: media.thumbUrl || media.fullThumbUrl || '',
												fullThumbUrl: media.fullThumbUrl || '',
												url: videoUrl,
												size: parseInt(media.fileSize || media.cdnFileSize || '0'),
												key: media.decodeKey || '',
												duration: media.spec[0].durationMs || 0,
												spec: media.spec.map(function(s) {
													return {
														width: s.width || 0,
														height: s.height || 0,
														bitrate: s.videoBitrate || s.bitRate || 0,
														fileFormat: s.fileFormat || '',
														durationMs: s.durationMs || 0
													};
												}),
												fileFormat: media.spec.map(function(o) { return o.fileFormat; }),
												nickname: feed.contact ? feed.contact.nickname : '',
												username: feed.contact ? feed.contact.username : '',
												createtime: feed.createtime || 0,
												contact: feed.contact || {},
												readCount: feed.readCount || 0,
												likeCount: feed.likeCount || 0,
												commentCount: feed.commentCount || 0,
												favCount: feed.favCount || 0,
												forwardCount: feed.forwardCount || 0,
												ipRegionInfo: feed.ipRegionInfo || {},
												mediaType: media.mediaType,
												timestamp: Date.now()
											};
											
											// 添加到批量下载面板
											window.__wx_channels_profile_collector.addVideoFromAPI(videoData);
											videoCount++;
										}
									}
								});
								if (videoCount > 0) {
									console.log('[搜索数据采集] ✓ 已将', videoCount, '个视频添加到批量下载面板');
								}
							}
						} catch (e) {
							console.error('[搜索数据采集] 转换视频数据失败:', e);
						}
					}
					
					// 检查是否有任何数据
					if (interceptedSearchData.profiles.length === 0 && 
					    interceptedSearchData.liveResults.length === 0 && 
					    interceptedSearchData.feedResults.length === 0) {
						console.warn('[搜索数据采集] 警告：从Store提取数据后，所有数组都为空！');
					}
				} else {
					console.log('[搜索数据采集] 未找到搜索Store，可能Store尚未加载或结构不同');
				}
			};
			
			// 注意：DOM提取已移除，因为Pinia Store已经能稳定获取完整数据
			
			// 页面加载后尝试从Store收集数据（智能重试：如果成功找到数据就停止重试）
			var retryCount = 0;
			var maxRetries = 3; // 减少到3次重试
			var retryDelays = [3000, 5000, 8000]; // 减少延迟次数
			var dataFound = false; // 标记是否已找到数据
			
			var tryCollectSearchData = function() {
				if (retryCount < maxRetries && !dataFound) {
					console.log('[搜索数据采集] 第', (retryCount + 1), '次尝试从Store采集数据...');
					
					// 检查是否成功找到数据
					var beforeProfiles = interceptedSearchData.profiles.length;
					var beforeLive = interceptedSearchData.liveResults.length;
					var beforeFeed = interceptedSearchData.feedResults.length;
					
					collectSearchData();
					
					// 检查数据是否有增加
					var afterProfiles = interceptedSearchData.profiles.length;
					var afterLive = interceptedSearchData.liveResults.length;
					var afterFeed = interceptedSearchData.feedResults.length;
					
					// 如果找到了数据（账号、直播或动态），标记为成功
					if (afterProfiles > 0 || afterLive > 0 || afterFeed > 0) {
						dataFound = true;
						console.log('[搜索数据采集] ✓ 已成功采集到数据，停止重试');
						return; // 停止重试
					}
					
					retryCount++;
					if (retryCount < maxRetries && !dataFound) {
						setTimeout(tryCollectSearchData, retryDelays[retryCount - 1] || 3000);
					}
				}
			};
			
			if (document.readyState === 'complete') {
				setTimeout(tryCollectSearchData, retryDelays[0]);
			} else {
				window.addEventListener('load', function() {
					setTimeout(tryCollectSearchData, retryDelays[0]);
				});
			}
			
			// 监听 URL 变化（搜索页是 SPA），重新初始化
			var lastUrl = window.location.href;
			setInterval(function() {
				if (window.location.href !== lastUrl) {
					lastUrl = window.location.href;
					if (window.location.pathname.includes('/pages/s')) {
						// URL变化时清空已拦截的数据，重新开始
						interceptedSearchData.profiles = [];
						interceptedSearchData.liveResults = [];
						interceptedSearchData.feedResults = [];
						setTimeout(collectSearchData, 2000);
					}
				}
			}, 1000);
			
			// 监听页面变化并自动更新数据
			var updateTimer = null;
			var lastUpdateTime = 0;
			var lastDataCount = {
				profiles: 0,
				liveResults: 0,
				feedResults: 0
			};
			
			var checkAndUpdateData = function(source) {
				// 记录当前数据数量
				var currentDataCount = {
					profiles: interceptedSearchData.profiles.length,
					liveResults: interceptedSearchData.liveResults.length,
					feedResults: interceptedSearchData.feedResults.length
				};
				
				// 重新采集数据
				collectSearchData();
				
				// 检查是否有新数据
				var newProfiles = interceptedSearchData.profiles.length - currentDataCount.profiles;
				var newLives = interceptedSearchData.liveResults.length - currentDataCount.liveResults;
				var newFeeds = interceptedSearchData.feedResults.length - currentDataCount.feedResults;
				
				if (newProfiles > 0 || newLives > 0 || newFeeds > 0) {
					console.log('[搜索数据采集] [' + (source || '检测') + '] 检测到新数据 - 账户:', newProfiles, ', 直播:', newLives, ', 动态:', newFeeds);
					
					// 更新全局搜索数据
					if (!window.__wx_channels_search_data) {
						window.__wx_channels_search_data = {};
					}
					window.__wx_channels_search_data.profiles = interceptedSearchData.profiles;
					window.__wx_channels_search_data.liveResults = interceptedSearchData.liveResults;
					window.__wx_channels_search_data.feedResults = interceptedSearchData.feedResults;
					window.__wx_channels_search_data.keyword = keyword;
					
					// 如果有新的feedResults，添加到批量下载面板
					if (newFeeds > 0 && typeof window.__wx_channels_profile_collector !== 'undefined') {
						try {
							var videoCount = 0;
							// 只处理新增的feedResults
							var processedFeeds = interceptedSearchData.feedResults.slice(currentDataCount.feedResults);
							processedFeeds.forEach(function(feed) {
								if (feed && feed.id && feed.objectDesc && feed.objectDesc.media && feed.objectDesc.media.length > 0) {
									var media = feed.objectDesc.media[0];
									if (media.mediaType === 4 && media.spec && media.spec.length > 0) {
										var videoUrl = '';
										if (media.url) {
											videoUrl = media.url + (media.urlToken || '');
										} else if (media.fullUrl) {
											videoUrl = media.fullUrl;
										}
										
										var videoData = {
											type: 'media',
											id: feed.id,
											nonce_id: feed.objectNonceId || feed.id,
											title: cleanHtmlTags(feed.objectDesc.description) || '未命名视频',
											coverUrl: media.coverUrl || media.fullCoverUrl || '',
											thumbUrl: media.thumbUrl || media.fullThumbUrl || '',
											fullThumbUrl: media.fullThumbUrl || '',
											url: videoUrl,
											size: parseInt(media.fileSize || media.cdnFileSize || '0'),
											key: media.decodeKey || '',
											duration: media.spec[0].durationMs || 0,
											spec: media.spec.map(function(s) {
												return {
													width: s.width || 0,
													height: s.height || 0,
													bitrate: s.videoBitrate || s.bitRate || 0,
													fileFormat: s.fileFormat || '',
													durationMs: s.durationMs || 0
												};
											}),
											fileFormat: media.spec.map(function(o) { return o.fileFormat; }),
											nickname: feed.contact ? feed.contact.nickname : '',
											username: feed.contact ? feed.contact.username : '',
											createtime: feed.createtime || 0,
											contact: feed.contact || {},
											readCount: feed.readCount || 0,
											likeCount: feed.likeCount || 0,
											commentCount: feed.commentCount || 0,
											favCount: feed.favCount || 0,
											forwardCount: feed.forwardCount || 0,
											ipRegionInfo: feed.ipRegionInfo || {},
											mediaType: media.mediaType,
											timestamp: Date.now()
										};
										
										window.__wx_channels_profile_collector.addVideoFromAPI(videoData);
										videoCount++;
									}
								}
							});
							if (videoCount > 0) {
								console.log('[搜索数据采集] ✓ [' + (source || '更新') + '] 已将', videoCount, '个新视频添加到批量下载面板');
							}
						} catch (e) {
							console.error('[搜索数据采集] 更新视频数据失败:', e);
						}
					}
					
					// 保存更新的数据
					saveInterceptedSearchData();
					
					// 更新最后的数据计数
					lastDataCount = {
						profiles: interceptedSearchData.profiles.length,
						liveResults: interceptedSearchData.liveResults.length,
						feedResults: interceptedSearchData.feedResults.length
					};
				}
			};
			
			// 防抖函数
			var debounceCheck = function(source) {
				clearTimeout(updateTimer);
				updateTimer = setTimeout(function() {
					var now = Date.now();
					// 限制更新频率：至少间隔1.5秒
					if (now - lastUpdateTime > 1500) {
						lastUpdateTime = now;
						checkAndUpdateData(source);
					}
				}, 300);
			};
			
			// 1. 监听DOM变化（MutationObserver）
			try {
				var observer = new MutationObserver(function(mutations) {
					var shouldCheck = false;
					mutations.forEach(function(mutation) {
						if (mutation.addedNodes && mutation.addedNodes.length > 0) {
							// 检测到新节点添加，可能是新内容加载
							shouldCheck = true;
						}
						if (mutation.type === 'childList' && mutation.addedNodes.length > 0) {
							shouldCheck = true;
						}
					});
					if (shouldCheck) {
						debounceCheck('DOM变化');
					}
				});
				
				// 观察整个文档的变化
				observer.observe(document.body, {
					childList: true,
					subtree: true
				});
				console.log('[搜索数据采集] ✓ 已启动DOM变化监听');
			} catch (e) {
				console.warn('[搜索数据采集] MutationObserver初始化失败:', e);
			}
			
			// 2. 监听滚动事件（作为补充）
			window.addEventListener('scroll', function() {
				var scrollTop = window.pageYOffset || document.documentElement.scrollTop;
				var windowHeight = window.innerHeight;
				var documentHeight = document.documentElement.scrollHeight;
				
				// 当滚动到底部附近时触发检查
				if (documentHeight - scrollTop - windowHeight < 200) {
					debounceCheck('滚动到底部');
				}
			}, { passive: true });
			
			// 3. 定期检查Store数据变化（每3秒）
			setInterval(function() {
				debounceCheck('定期检查');
			}, 3000);
			
			console.log('[搜索数据采集] ✓ 已启动页面变化监听（DOM变化、滚动、定期检查）');
		} catch (error) {
			console.error('搜索数据采集初始化失败:', error);
		}
	};
	
	// 初始化搜索数据采集
	if (document.readyState === 'complete') {
		window.__wx_channels_collect_search_data();
	} else {
		window.addEventListener('load', function() {
			window.__wx_channels_collect_search_data();
		});
	}
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

	utils.LogInfo("[Home数据采集] 正在处理 index.publish 文件")

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
		utils.Info("视频播放已成功加载！")
		utils.Info("视频缓冲将被监控，完成时会有提醒")
		utils.LogInfo("[视频播放] 视频播放器已加载 | Path=%s", path)
	}
	content = regexp1.ReplaceAllString(content, replaceStr1)
	regexp2 := regexp.MustCompile(`if\(f.cmd===re.MAIN_THREAD_CMD.AUTO_CUT`)
	replaceStr2 := `if(f.cmd==="CUT"){
	if (window.__wx_channels_store__) {
	// console.log("CUT", f, __wx_channels_store__.profile.key);
	window.__wx_channels_store__.keys[__wx_channels_store__.profile.key]=f.decryptor_array;
	}
}
if(f.cmd===re.MAIN_THREAD_CMD.AUTO_CUT`
	content = regexp2.ReplaceAllString(content, replaceStr2)

	// 尝试在index.publish中查找并拦截视频切换函数
	// 策略1：拦截 goToNextFlowFeed (下一个视频)
	callNextRegex := regexp.MustCompile(`(\w)\.goToNextFlowFeed\(\{goBackWhenEnd:[^,]+,eleInfo:\{[^}]+\}[^)]*\}\)`)
	// 策略2：拦截 goToPrevFlowFeed (上一个视频)
	callPrevRegex := regexp.MustCompile(`(\w)\.goToPrevFlowFeed\(\{eleInfo:\{[^}]+\}\}\)`)

	// 数据采集代码（通用，包含互动数据）- 精简日志版本
	captureCode := `setTimeout(function(){try{var __tab=Ue.value;if(__tab&&__tab.currentFeed){var __feed=__tab.currentFeed;if(__feed.objectDesc){var __media=__feed.objectDesc.media[0];var __duration=0;if(__media&&__media.spec&&__media.spec[0]&&__media.spec[0].durationMs){__duration=__media.spec[0].durationMs;}var __profile={type:"media",media:__media,duration:__duration,spec:__media.spec.map(function(s){return{width:s.width||s.videoWidth,height:s.height||s.videoHeight,bitrate:s.bitrate,fileFormat:s.fileFormat}}),title:__feed.objectDesc.description,coverUrl:__media.thumbUrl,url:__media.url+__media.urlToken,size:__media.fileSize,key:__media.decodeKey,id:__feed.id,nonce_id:__feed.objectNonceId,nickname:(__feed.contact&&__feed.contact.nickname)?__feed.contact.nickname:"",createtime:__feed.createtime,fileFormat:__media.spec.map(function(o){return o.fileFormat}),contact:__feed.contact,readCount:__feed.readCount,likeCount:__feed.likeCount,commentCount:__feed.commentCount,favCount:__feed.favCount,forwardCount:__feed.forwardCount,ipRegionInfo:__feed.ipRegionInfo};fetch("/__wx_channels_api/profile",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(__profile)});window.__wx_channels_store__=window.__wx_channels_store__||{profile:null,buffers:[],keys:{}};window.__wx_channels_store__.profile=__profile;fetch("/__wx_channels_api/tip",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({msg:"📹 [Home] "+(__profile.nickname||"未知作者")+" - "+(__profile.title||"").substring(0,30)+"..."})}).catch(function(){});}}}catch(__e){console.error("[Home] 采集失败:",__e)}},500)`

	// 替换 goToNextFlowFeed
	if callNextRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] 在index.publish中成功拦截 goToNextFlowFeed 函数")
		replaceNext := `$1.goToNextFlowFeed({goBackWhenEnd:f.goBackWhenEnd,eleInfo:{type:f.source,tagId:Ct.value},ignoreCoolDown:f.ignoreCoolDown});` + captureCode
		content = callNextRegex.ReplaceAllString(content, replaceNext)
	} else {
		utils.LogInfo("[Home数据采集] 在index.publish中未找到 goToNextFlowFeed 函数")
	}

	// 替换 goToPrevFlowFeed
	if callPrevRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] 在index.publish中成功拦截 goToPrevFlowFeed 函数")
		replacePrev := `$1.goToPrevFlowFeed({eleInfo:{type:f.source,tagId:Ct.value}});` + captureCode
		content = callPrevRegex.ReplaceAllString(content, replacePrev)
	} else {
		utils.LogInfo("[Home数据采集] 在index.publish中未找到 goToPrevFlowFeed 函数")
	}

	return content, true
}

// handleVirtualSvgIcons 处理virtual_svg-icons-register JS文件
func (h *ScriptHandler) handleVirtualSvgIcons(path string, content string) (string, bool) {
	if !util.Includes(path, "/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register") {
		return content, false
	}

	// 拦截 Profile 页面的视频列表数据
	profileListRegex := regexp.MustCompile(`async finderUserPage\((\w+)\)\{return(.*?)\}async`)
	profileListReplace := `async finderUserPage($1) {
		var profileResult = await$2;
		
		// 检查当前页面类型
		var isProfilePage = window.location.pathname.includes('/pages/profile') && 
		                    !window.location.pathname.includes('/pages/home') && 
		                    !window.location.pathname.includes('/pages/feed');
		
		// 如果不是Profile页面，静默返回（不输出日志，不采集数据）
		if (!isProfilePage) {
			return profileResult;
		}
		
		// HTML标签清理函数
		var cleanHtmlTags = function(text) {
			if (!text || typeof text !== 'string') return text || '';
			var tempDiv = document.createElement('div');
			tempDiv.innerHTML = text;
			var cleaned = tempDiv.textContent || tempDiv.innerText || '';
			var htmlEntities = {
				'&nbsp;': ' ',
				'&amp;': '&',
				'&lt;': '<',
				'&gt;': '>',
				'&quot;': '"',
				'&apos;': "'",
				'&#39;': "'",
				'&#34;': '"'
			};
			for (var entity in htmlEntities) {
				cleaned = cleaned.replace(new RegExp(entity, 'g'), htmlEntities[entity]);
			}
			cleaned = cleaned.replace(/&[a-zA-Z0-9#]+;/g, '');
			return cleaned.trim();
		};
		
		// Profile页面视频列表数据采集
		if (profileResult && profileResult.data && profileResult.data.object) {
			var videoCount = profileResult.data.object.length;
			console.log('[API拦截] 获取到视频列表，数量:', videoCount);
			
			// 发送日志到后端终端
			fetch('/__wx_channels_api/tip', {
				method: 'POST',
				headers: {'Content-Type': 'application/json'},
				body: JSON.stringify({msg: '📊 [API拦截] 获取到当前页视频列表，数量: ' + videoCount})
			}).catch(() => {});
			
			// 处理视频列表中的每个视频
			profileResult.data.object.forEach((item, index) => {
				try {
					var data_object = item;
					if (!data_object || !data_object.objectDesc) {
						return;
					}
					
					var media = data_object.objectDesc.media[0];
					if (!media) return;
					
					var profile = media.mediaType !== 4 ? {
						type: "picture",
						id: data_object.id,
						title: cleanHtmlTags(data_object.objectDesc.description),
						files: data_object.objectDesc.media,
						spec: [],
						contact: data_object.contact
					} : {
						type: "media",
						duration: media.spec[0].durationMs,
						spec: media.spec.map(s => ({
							...s,
							width: s.width || s.videoWidth,
							height: s.height || s.videoHeight
						})),
						title: cleanHtmlTags(data_object.objectDesc.description),
						coverUrl: media.thumbUrl || media.coverUrl,
						thumbUrl: media.thumbUrl,
						fullThumbUrl: media.fullThumbUrl,
						url: media.url+media.urlToken,
						size: media.fileSize,
						key: media.decodeKey,
						id: data_object.id,
						nonce_id: data_object.objectNonceId,
						nickname: data_object.nickname,
						username: data_object.contact?.username || '',
						createtime: data_object.createtime,
						fileFormat: media.spec.map(o => o.fileFormat),
						contact: data_object.contact,
						readCount: data_object.readCount || 0,
						likeCount: data_object.likeCount || 0,
						commentCount: data_object.commentCount || 0,
						favCount: data_object.favCount || 0,
						forwardCount: data_object.forwardCount || 0,
						ipRegionInfo: data_object.ipRegionInfo || {},
						// 新增字段
						mediaType: media.mediaType,
						videoWidth: media.spec[0]?.width || media.spec[0]?.videoWidth || 0,
						videoHeight: media.spec[0]?.height || media.spec[0]?.videoHeight || 0,
						videoBitrate: media.spec[0]?.bitrate || 0,
						videoCodec: media.spec[0]?.codec || '',
						audioCodec: media.spec[0]?.audioCodec || '',
						frameRate: media.spec[0]?.fps || 0,
						location: data_object.location || '',
						latitude: data_object.latitude || 0,
						longitude: data_object.longitude || 0,
						poi: data_object.poi || '',
						extInfo: data_object.extInfo || {},
						timestamp: Date.now()
					};
					
				// 添加到profile采集器（使用等待机制）
				(function(profileData) {
					// 尝试立即添加
					if (window.__wx_channels_profile_collector) {
						window.__wx_channels_profile_collector.addVideoFromAPI(profileData);
					} else {
						// 如果采集器还未初始化，等待最多5秒
						var waitCount = 0;
						var waitInterval = setInterval(function() {
							waitCount++;
							if (window.__wx_channels_profile_collector) {
								clearInterval(waitInterval);
								window.__wx_channels_profile_collector.addVideoFromAPI(profileData);
								console.log('✓ 延迟添加视频到采集器:', profileData.title?.substring(0, 30));
							} else if (waitCount > 50) {
								// 超时5秒
								clearInterval(waitInterval);
								console.warn('⚠️ 采集器初始化超时，数据已保存到临时存储');
								// 保存到临时存储
								window.__wx_channels_temp_profiles = window.__wx_channels_temp_profiles || [];
								window.__wx_channels_temp_profiles.push(profileData);
							}
						}, 100);
					}
				})(profile);
				
				// 同时添加到全局存储
				if (window.__wx_channels_store__) {
					window.__wx_channels_store__.profiles = window.__wx_channels_store__.profiles || [];
					window.__wx_channels_store__.profiles.push(profile);
				}
					
					// 输出前3个视频的日志到控制台和后端
					if (index < 3) {
						var logMsg = '[API拦截] 视频' + (index+1) + ': ' + profile.title.substring(0, 30) + '...';
						console.log(logMsg);
						fetch('/__wx_channels_api/tip', {
							method: 'POST',
							headers: {'Content-Type': 'application/json'},
							body: JSON.stringify({msg: '📹 ' + logMsg})
						}).catch(() => {});
					}
					
					// 采集完成后发送总结日志
					if (index === profileResult.data.object.length - 1) {
						fetch('/__wx_channels_api/tip', {
							method: 'POST',
							headers: {'Content-Type': 'application/json'},
							body: JSON.stringify({msg: '✅ [API拦截] 当前页采集完成，共 ' + profileResult.data.object.length + ' 个视频'})
						}).catch(() => {});
					}
				} catch (error) {
					console.error('[主页采集] 处理视频失败:', error);
				}
			});
		}
		
		return profileResult;
	}async`

	if profileListRegex.MatchString(content) {
		utils.PrintSeparator()
		color.Green("✅ [主页页面] 视频列表API拦截器已注入")
		utils.PrintSeparator()
		content = profileListRegex.ReplaceAllString(content, profileListReplace)
	}

	regexp1 := regexp.MustCompile(`async finderGetCommentDetail\((\w+)\)\{return(.*?)\}async`)
	replaceStr1 := `async finderGetCommentDetail($1) {
		var feedResult = await$2;
		var data_object = feedResult.data.object;
		if (!data_object.objectDesc) {
			return feedResult;
		}
		
		// 不再输出调试信息
		// console.log("原始视频数据对象:", data_object);
		
		var media = data_object.objectDesc.media[0];
		var profile = media.mediaType !== 4 ? {
			type: "picture",
			id: data_object.id,
			title: data_object.objectDesc.description,
			files: data_object.objectDesc.media,
			spec: [],
			contact: data_object.contact
		} : {
			type: "media",
			duration: media.spec[0].durationMs,
			spec: media.spec.map(s => ({
				...s,
				width: s.width || s.videoWidth,
				height: s.height || s.videoHeight
			})),
			title: data_object.objectDesc.description,
			coverUrl: media.thumbUrl || media.coverUrl, // 使用thumbUrl作为主要封面，如果不存在则使用coverUrl
			thumbUrl: media.thumbUrl, // 添加thumbUrl字段
			fullThumbUrl: media.fullThumbUrl, // 添加fullThumbUrl字段
			url: media.url+media.urlToken,
			size: media.fileSize,
			key: media.decodeKey,
			id: data_object.id,
			nonce_id: data_object.objectNonceId,
			nickname: data_object.nickname,
			createtime: data_object.createtime,
			fileFormat: media.spec.map(o => o.fileFormat),
			contact: data_object.contact,
			// 互动数据
			readCount: data_object.readCount || 0,
			likeCount: data_object.likeCount || 0,
			commentCount: data_object.commentCount || 0,
			favCount: data_object.favCount || 0,
			forwardCount: data_object.forwardCount || 0,
			// IP区域信息
			ipRegionInfo: data_object.ipRegionInfo || {}
		};
		
		// 如果存在对象扩展信息，添加到profile
		if (data_object.objectExtend && data_object.objectExtend.monotonicData) {
			const monotonicData = data_object.objectExtend.monotonicData;
			if (monotonicData.countInfo) {
				profile.readCount = monotonicData.countInfo.readCount || profile.readCount;
				profile.likeCount = monotonicData.countInfo.likeCount || profile.likeCount;
				profile.commentCount = monotonicData.countInfo.commentCount || profile.commentCount;
				profile.favCount = monotonicData.countInfo.favCount || profile.favCount;
				profile.forwardCount = monotonicData.countInfo.forwardCount || profile.forwardCount;
			}
		}
		
		fetch("/__wx_channels_api/profile", {
			method: "POST",
			headers: {
				"Content-Type": "application/json"
			},
			body: JSON.stringify(profile)
		});
		if (window.__wx_channels_store__) {
		__wx_channels_store__.profile = profile;
		window.__wx_channels_store__.profiles.push(profile);
		
		// 启动视频缓存监控
		if (window.__wx_channels_video_cache_monitor && profile.type === "media" && profile.size) {
			console.log("正在初始化视频缓存监控系统...");
			console.log("视频大小:", (profile.size / (1024 * 1024)).toFixed(2) + 'MB');
			console.log("视频标题:", profile.title);
			setTimeout(() => {
				// 确保Video.js播放器已经加载
				const vjsPlayer = document.querySelector('.video-js');
				const video = vjsPlayer ? vjsPlayer.querySelector('video') : document.querySelector('video');
				
				if (video) {
					console.log("找到视频元素，启动缓存监控");
					console.log("视频readyState:", video.readyState);
					console.log("视频duration:", video.duration);
					window.__wx_channels_video_cache_monitor.startMonitoring(profile.size);
				} else {
					console.log("未找到视频元素，延迟重试");
					setTimeout(() => {
						window.__wx_channels_video_cache_monitor.startMonitoring(profile.size);
					}, 2000); // 再延迟2秒重试
				}
			}, 3000); // 延迟3秒启动，确保Video.js播放器完全初始化
		}
		}
		return feedResult;
	}async`
	if regexp1.MatchString(content) {
		utils.Info("视频详情数据已获取成功！")
	utils.LogInfo("[视频详情] 视频详情API已拦截 | Path=%s", path)
	}
	content = regexp1.ReplaceAllString(content, replaceStr1)
	regex2 := regexp.MustCompile(`i.default={dialog`)
	replaceStr2 := `i.default=window.window.__wx_channels_tip__={dialog`
	content = regex2.ReplaceAllString(content, replaceStr2)
	regex5 := regexp.MustCompile(`this.updateDetail\(o\)`)
	replaceStr5 := `(() => {
		if (Object.keys(o).length===0){
		return;
		}
		
		// 不再输出调试信息
		// console.log("updateDetail原始数据:", o);
		
		var data_object = o;
		var media = data_object.objectDesc.media[0];
		var profile = media.mediaType !== 4 ? {
			type: "picture",
			id: data_object.id,
			title: data_object.objectDesc.description,
			files: data_object.objectDesc.media,
			spec: [],
			contact: data_object.contact
		} : {
			type: "media",
			duration: media.spec[0].durationMs,
			spec: media.spec.map(s => ({
				...s,
				width: s.width || s.videoWidth,
				height: s.height || s.videoHeight
			})),
			title: data_object.objectDesc.description,
			coverUrl: media.thumbUrl || media.coverUrl, // 使用thumbUrl作为主要封面，如果不存在则使用coverUrl
			thumbUrl: media.thumbUrl, // 添加thumbUrl字段
			fullThumbUrl: media.fullThumbUrl, // 添加fullThumbUrl字段
			url: media.url+media.urlToken,
			size: media.fileSize,
			key: media.decodeKey,
			id: data_object.id,
			nonce_id: data_object.objectNonceId,
			nickname: data_object.nickname,
			createtime: data_object.createtime,
			fileFormat: media.spec.map(o => o.fileFormat),
			contact: data_object.contact,
			// 互动数据
			readCount: data_object.readCount || 0,
			likeCount: data_object.likeCount || 0,
			commentCount: data_object.commentCount || 0,
			favCount: data_object.favCount || 0,
			forwardCount: data_object.forwardCount || 0,
			// IP区域信息
			ipRegionInfo: data_object.ipRegionInfo || {}
		};
		
		// 如果存在对象扩展信息，添加到profile
		if (data_object.objectExtend && data_object.objectExtend.monotonicData) {
			const monotonicData = data_object.objectExtend.monotonicData;
			if (monotonicData.countInfo) {
				profile.readCount = monotonicData.countInfo.readCount || profile.readCount;
				profile.likeCount = monotonicData.countInfo.likeCount || profile.likeCount;
				profile.commentCount = monotonicData.countInfo.commentCount || profile.commentCount;
				profile.favCount = monotonicData.countInfo.favCount || profile.favCount;
				profile.forwardCount = monotonicData.countInfo.forwardCount || profile.forwardCount;
			}
		}
		
		if (window.__wx_channels_store__) {
	window.__wx_channels_store__.profiles.push(profile);
		}
		})(),this.updateDetail(o)`
	content = regex5.ReplaceAllString(content, replaceStr5)
	return content, true
}

// handleFeedDetail 处理FeedDetail.publish JS文件
func (h *ScriptHandler) handleFeedDetail(path string, content string) (string, bool) {
	if !util.Includes(path, "/t/wx_fed/finder/web/web-finder/res/js/FeedDetail.publish") {
		return content, false
	}

	regex := regexp.MustCompile(`,"投诉"\)]`)
	replaceStr := `,"投诉"),...(() => {
	if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
		return window.__wx_channels_store__.profile.spec.map((sp) => {
			return f("div",{class:"context-item",role:"button",onClick:() => __wx_channels_handle_click_download__(sp)},__wx_format_quality_option(sp));
		});
	}
	})(),f("div",{class:"context-item",role:"button",onClick:()=>__wx_channels_handle_click_download__()},"原始视频"),f("div",{class:"context-item",role:"button",onClick:__wx_channels_download_cur__},"当前视频"),f("div",{class:"context-item",role:"button",onClick:()=>__wx_channels_handle_download_cover()},"下载封面"),f("div",{class:"context-item",role:"button",onClick:()=>window.__wx_channels_start_comment_collection&&window.__wx_channels_start_comment_collection()},"采集评论")]`
	content = regex.ReplaceAllString(content, replaceStr)
	return content, true
}

// handleWorkerRelease 处理worker_release JS文件
func (h *ScriptHandler) handleWorkerRelease(path string, content string) (string, bool) {
	if !util.Includes(path, "worker_release") {
		return content, false
	}

	regex := regexp.MustCompile(`fmp4Index:p.fmp4Index`)
	replaceStr := `decryptor_array:p.decryptor_array,fmp4Index:p.fmp4Index`
	content = regex.ReplaceAllString(content, replaceStr)
	return content, true
}

// handleVuexStores 处理vuexStores.publish JS文件
func (h *ScriptHandler) handleVuexStores(Conn *SunnyNet.HttpConn, path string, content string) (string, bool) {
	if !util.Includes(path, "vuexStores.publish") {
		return content, false
	}

	utils.LogInfo("[Home数据采集] 正在处理 vuexStores.publish 文件")

	// 策略1：拦截 goToNextFlowFeed (下一个视频)
	callNextRegex := regexp.MustCompile(`(\w)\.goToNextFlowFeed\(\{goBackWhenEnd:[^,]+,eleInfo:\{[^}]+\}[^)]*\}\)`)
	// 策略2：拦截 goToPrevFlowFeed (上一个视频)
	callPrevRegex := regexp.MustCompile(`(\w)\.goToPrevFlowFeed\(\{eleInfo:\{[^}]+\}\}\)`)

	// 数据采集代码（通用，包含互动数据）- 精简日志版本
	captureCode := `setTimeout(function(){try{var __tab=Ue.value;if(__tab&&__tab.currentFeed){var __feed=__tab.currentFeed;if(__feed.objectDesc){var __media=__feed.objectDesc.media[0];var __duration=0;if(__media&&__media.spec&&__media.spec[0]&&__media.spec[0].durationMs){__duration=__media.spec[0].durationMs;}var __profile={type:"media",media:__media,duration:__duration,spec:__media.spec.map(function(s){return{width:s.width||s.videoWidth,height:s.height||s.videoHeight,bitrate:s.bitrate,fileFormat:s.fileFormat}}),title:__feed.objectDesc.description,coverUrl:__media.thumbUrl,url:__media.url+__media.urlToken,size:__media.fileSize,key:__media.decodeKey,id:__feed.id,nonce_id:__feed.objectNonceId,nickname:(__feed.contact&&__feed.contact.nickname)?__feed.contact.nickname:"",createtime:__feed.createtime,fileFormat:__media.spec.map(function(o){return o.fileFormat}),contact:__feed.contact,readCount:__feed.readCount,likeCount:__feed.likeCount,commentCount:__feed.commentCount,favCount:__feed.favCount,forwardCount:__feed.forwardCount,ipRegionInfo:__feed.ipRegionInfo};fetch("/__wx_channels_api/profile",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(__profile)});window.__wx_channels_store__=window.__wx_channels_store__||{profile:null,buffers:[],keys:{}};window.__wx_channels_store__.profile=__profile;fetch("/__wx_channels_api/tip",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({msg:"📹 [Home] "+(__profile.nickname||"未知作者")+" - "+(__profile.title||"").substring(0,30)+"..."})}).catch(function(){});}}}catch(__e){console.error("[Home] 采集失败:",__e)}},500)`

	// 替换 goToNextFlowFeed
	if callNextRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] 成功拦截 goToNextFlowFeed 函数")
		replaceNext := `$1.goToNextFlowFeed({goBackWhenEnd:f.goBackWhenEnd,eleInfo:{type:f.source,tagId:Ct.value},ignoreCoolDown:f.ignoreCoolDown});` + captureCode
		content = callNextRegex.ReplaceAllString(content, replaceNext)
	} else {
		utils.LogInfo("[Home数据采集] 未找到 goToNextFlowFeed 函数")
	}

	// 替换 goToPrevFlowFeed
	if callPrevRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] 成功拦截 goToPrevFlowFeed 函数")
		replacePrev := `$1.goToPrevFlowFeed({eleInfo:{type:f.source,tagId:Ct.value}});` + captureCode
		content = callPrevRegex.ReplaceAllString(content, replacePrev)
	} else {
		utils.LogInfo("[Home数据采集] 未找到 goToPrevFlowFeed 函数")
	}

	// 禁用浏览器缓存，确保每次都能拦截到最新的代码
	Conn.Response.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	Conn.Response.Header.Set("Pragma", "no-cache")
	Conn.Response.Header.Set("Expires", "0")

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return content, true
}

// handleGlobalPublish 处理global.publish JS文件
func (h *ScriptHandler) handleGlobalPublish(Conn *SunnyNet.HttpConn, path string, content string) (string, bool) {
	if !util.Includes(path, "global.publish") {
		return content, false
	}

	utils.LogInfo("[Home数据采集] 正在处理 global.publish 文件")

	// 策略1：拦截 goToNextFlowFeed (下一个视频)
	callNextRegex := regexp.MustCompile(`(\w)\.goToNextFlowFeed\(\{goBackWhenEnd:[^,]+,eleInfo:\{[^}]+\}[^)]*\}\)`)
	// 策略2：拦截 goToPrevFlowFeed (上一个视频)
	callPrevRegex := regexp.MustCompile(`(\w)\.goToPrevFlowFeed\(\{eleInfo:\{[^}]+\}\}\)`)

	// 数据采集代码（通用，包含互动数据）- 精简日志版本
	captureCode := `setTimeout(function(){try{var __tab=Ue.value;if(__tab&&__tab.currentFeed){var __feed=__tab.currentFeed;if(__feed.objectDesc){var __media=__feed.objectDesc.media[0];var __duration=0;if(__media&&__media.spec&&__media.spec[0]&&__media.spec[0].durationMs){__duration=__media.spec[0].durationMs;}var __profile={type:"media",media:__media,duration:__duration,spec:__media.spec.map(function(s){return{width:s.width||s.videoWidth,height:s.height||s.videoHeight,bitrate:s.bitrate,fileFormat:s.fileFormat}}),title:__feed.objectDesc.description,coverUrl:__media.thumbUrl,url:__media.url+__media.urlToken,size:__media.fileSize,key:__media.decodeKey,id:__feed.id,nonce_id:__feed.objectNonceId,nickname:(__feed.contact&&__feed.contact.nickname)?__feed.contact.nickname:"",createtime:__feed.createtime,fileFormat:__media.spec.map(function(o){return o.fileFormat}),contact:__feed.contact,readCount:__feed.readCount,likeCount:__feed.likeCount,commentCount:__feed.commentCount,favCount:__feed.favCount,forwardCount:__feed.forwardCount,ipRegionInfo:__feed.ipRegionInfo};fetch("/__wx_channels_api/profile",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(__profile)});window.__wx_channels_store__=window.__wx_channels_store__||{profile:null,buffers:[],keys:{}};window.__wx_channels_store__.profile=__profile;fetch("/__wx_channels_api/tip",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({msg:"📹 [Home] "+(__profile.nickname||"未知作者")+" - "+(__profile.title||"").substring(0,30)+"..."})}).catch(function(){});}}}catch(__e){console.error("[Home] 采集失败:",__e)}},500)`

	// 替换 goToNextFlowFeed
	if callNextRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] 在global.publish中成功拦截 goToNextFlowFeed 函数")
		replaceNext := `$1.goToNextFlowFeed({goBackWhenEnd:f.goBackWhenEnd,eleInfo:{type:f.source,tagId:Ct.value},ignoreCoolDown:f.ignoreCoolDown});` + captureCode
		content = callNextRegex.ReplaceAllString(content, replaceNext)
	} else {
		utils.LogInfo("[Home数据采集] 在global.publish中未找到 goToNextFlowFeed 函数")
	}

	// 替换 goToPrevFlowFeed
	if callPrevRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] 在global.publish中成功拦截 goToPrevFlowFeed 函数")
		replacePrev := `$1.goToPrevFlowFeed({eleInfo:{type:f.source,tagId:Ct.value}});` + captureCode
		content = callPrevRegex.ReplaceAllString(content, replacePrev)
	} else {
		utils.LogInfo("[Home数据采集] 在global.publish中未找到 goToPrevFlowFeed 函数")
	}

	// 禁用浏览器缓存，确保每次都能拦截到最新的代码
	Conn.Response.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	Conn.Response.Header.Set("Pragma", "no-cache")
	Conn.Response.Header.Set("Expires", "0")

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return content, true
}

// handleConnectPublish 处理connect.publish JS文件（可能是新的vuexStores）
func (h *ScriptHandler) handleConnectPublish(Conn *SunnyNet.HttpConn, path string, content string) (string, bool) {
	if !util.Includes(path, "connect.publish") {
		return content, false
	}

	utils.LogInfo("[Home数据采集] ✅ 正在处理 connect.publish 文件（可能是新的状态管理文件）")

	// 策略1：拦截 goToNextFlowFeed (下一个视频)
	// 修复正则：允许 eleInfo 后面有更多参数（如 ignoreCoolDown）
	callNextRegex := regexp.MustCompile(`(\w)\.goToNextFlowFeed\(\{[^}]*goBackWhenEnd:[^,}]+[^}]*eleInfo:\{[^}]+\}[^}]*\}\)`)
	// 策略2：拦截 goToPrevFlowFeed (上一个视频)
	callPrevRegex := regexp.MustCompile(`(\w)\.goToPrevFlowFeed\(\{[^}]*eleInfo:\{[^}]+\}[^}]*\}\)`)

	// 数据采集代码（智能查找变量名：yt, Dt, ae, Ue）- 带调试日志
	captureCode := `setTimeout(function(){try{console.log("[Home采集] 开始执行...");var __tab=null;if(typeof yt!=="undefined"&&yt&&yt.value){__tab=yt.value;console.log("[Home采集] 使用yt.value");}else if(typeof Dt!=="undefined"&&Dt&&Dt.value){__tab=Dt.value;console.log("[Home采集] 使用Dt.value");}else if(typeof ae!=="undefined"&&ae&&ae.value){__tab=ae.value;console.log("[Home采集] 使用ae.value");}else if(typeof Ue!=="undefined"&&Ue&&Ue.value){__tab=Ue.value;console.log("[Home采集] 使用Ue.value");}else{console.log("[Home采集] 未找到tab变量");return;}if(__tab&&__tab.currentFeed){var __feed=__tab.currentFeed;console.log("[Home采集] 找到currentFeed");if(__feed.objectDesc){var __media=__feed.objectDesc.media[0];console.log("[Home采集] media高度:",__media.height);var __duration=0;if(__media&&__media.spec&&__media.spec[0]&&__media.spec[0].durationMs){__duration=__media.spec[0].durationMs;}var __profile={type:"media",media:__media,duration:__duration,spec:__media.spec.map(function(s){return{width:s.width||s.videoWidth,height:s.height||s.videoHeight,bitrate:s.bitrate,fileFormat:s.fileFormat}}),title:__feed.objectDesc.description,coverUrl:__media.thumbUrl,url:__media.url+__media.urlToken,size:__media.fileSize,key:__media.decodeKey,id:__feed.id,nonce_id:__feed.objectNonceId,nickname:(__feed.contact&&__feed.contact.nickname)?__feed.contact.nickname:"",createtime:__feed.createtime,fileFormat:__media.spec.map(function(o){return o.fileFormat}),contact:__feed.contact,readCount:__feed.readCount,likeCount:__feed.likeCount,commentCount:__feed.commentCount,favCount:__feed.favCount,forwardCount:__feed.forwardCount,ipRegionInfo:__feed.ipRegionInfo};console.log("[Home采集] 发送profile请求...");fetch("/__wx_channels_api/profile",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(__profile)}).then(function(){console.log("[Home采集] profile请求成功");}).catch(function(e){console.error("[Home采集] profile请求失败:",e);});window.__wx_channels_store__=window.__wx_channels_store__||{profile:null,buffers:[],keys:{}};window.__wx_channels_store__.profile=__profile;fetch("/__wx_channels_api/tip",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({msg:"📹 [Home] "+(__profile.nickname||"未知作者")+" - "+(__profile.title||"").substring(0,30)+"..."})}).catch(function(){});}}}catch(__e){console.error("[Home采集] 失败:",__e)}},500)`

	// 替换 goToNextFlowFeed
	if callNextRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] ✅ 在connect.publish中成功拦截 goToNextFlowFeed 函数")
		// 保留原始的函数调用，只在后面添加采集代码
		replaceNext := `$0;` + captureCode
		content = callNextRegex.ReplaceAllString(content, replaceNext)
	} else {
		utils.LogInfo("[Home数据采集] ❌ 在connect.publish中未找到 goToNextFlowFeed 函数")
	}

	// 替换 goToPrevFlowFeed
	if callPrevRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] ✅ 在connect.publish中成功拦截 goToPrevFlowFeed 函数")
		// 保留原始的函数调用，只在后面添加采集代码
		replacePrev := `$0;` + captureCode
		content = callPrevRegex.ReplaceAllString(content, replacePrev)
	} else {
		utils.LogInfo("[Home数据采集] ❌ 在connect.publish中未找到 goToPrevFlowFeed 函数")
	}

	// 禁用浏览器缓存，确保每次都能拦截到最新的代码
	Conn.Response.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	Conn.Response.Header.Set("Pragma", "no-cache")
	Conn.Response.Header.Set("Expires", "0")

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return content, true
}

// handleFinderHomePublish 处理FinderHome.publish JS文件（Home页面主逻辑）
func (h *ScriptHandler) handleFinderHomePublish(Conn *SunnyNet.HttpConn, path string, content string) (string, bool) {
	if !util.Includes(path, "FinderHome.publish") {
		return content, false
	}

	utils.LogInfo("[Home数据采集] 🎯 正在处理 FinderHome.publish 文件（Home页面主逻辑文件）")

	// 策略1：拦截 goToNextFlowFeed (下一个视频)
	callNextRegex := regexp.MustCompile(`(\w)\.goToNextFlowFeed\(\{goBackWhenEnd:[^,]+,eleInfo:\{[^}]+\}[^)]*\}\)`)
	// 策略2：拦截 goToPrevFlowFeed (上一个视频)
	callPrevRegex := regexp.MustCompile(`(\w)\.goToPrevFlowFeed\(\{eleInfo:\{[^}]+\}\}\)`)

	// 数据采集代码（通用，包含互动数据）- 精简日志版本
	captureCode := `setTimeout(function(){try{var __tab=Ue.value;if(__tab&&__tab.currentFeed){var __feed=__tab.currentFeed;if(__feed.objectDesc){var __media=__feed.objectDesc.media[0];var __duration=0;if(__media&&__media.spec&&__media.spec[0]&&__media.spec[0].durationMs){__duration=__media.spec[0].durationMs;}var __profile={type:"media",media:__media,duration:__duration,spec:__media.spec.map(function(s){return{width:s.width||s.videoWidth,height:s.height||s.videoHeight,bitrate:s.bitrate,fileFormat:s.fileFormat}}),title:__feed.objectDesc.description,coverUrl:__media.thumbUrl,url:__media.url+__media.urlToken,size:__media.fileSize,key:__media.decodeKey,id:__feed.id,nonce_id:__feed.objectNonceId,nickname:(__feed.contact&&__feed.contact.nickname)?__feed.contact.nickname:"",createtime:__feed.createtime,fileFormat:__media.spec.map(function(o){return o.fileFormat}),contact:__feed.contact,readCount:__feed.readCount,likeCount:__feed.likeCount,commentCount:__feed.commentCount,favCount:__feed.favCount,forwardCount:__feed.forwardCount,ipRegionInfo:__feed.ipRegionInfo};fetch("/__wx_channels_api/profile",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(__profile)});window.__wx_channels_store__=window.__wx_channels_store__||{profile:null,buffers:[],keys:{}};window.__wx_channels_store__.profile=__profile;fetch("/__wx_channels_api/tip",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({msg:"📹 [Home] "+(__profile.nickname||"未知作者")+" - "+(__profile.title||"").substring(0,30)+"..."})}).catch(function(){});}}}catch(__e){console.error("[Home] 采集失败:",__e)}},500)`

	// 替换 goToNextFlowFeed
	if callNextRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] ✅ 在FinderHome.publish中成功拦截 goToNextFlowFeed 函数")
		replaceNext := `$1.goToNextFlowFeed({goBackWhenEnd:f.goBackWhenEnd,eleInfo:{type:f.source,tagId:Ct.value},ignoreCoolDown:f.ignoreCoolDown});` + captureCode
		content = callNextRegex.ReplaceAllString(content, replaceNext)
	} else {
		utils.LogInfo("[Home数据采集] ❌ 在FinderHome.publish中未找到 goToNextFlowFeed 函数")
	}

	// 替换 goToPrevFlowFeed
	if callPrevRegex.MatchString(content) {
		utils.LogInfo("[Home数据采集] ✅ 在FinderHome.publish中成功拦截 goToPrevFlowFeed 函数")
		replacePrev := `$1.goToPrevFlowFeed({eleInfo:{type:f.source,tagId:Ct.value}});` + captureCode
		content = callPrevRegex.ReplaceAllString(content, replacePrev)
	} else {
		utils.LogInfo("[Home数据采集] ❌ 在FinderHome.publish中未找到 goToPrevFlowFeed 函数")
	}

	// 禁用浏览器缓存，确保每次都能拦截到最新的代码
	Conn.Response.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	Conn.Response.Header.Set("Pragma", "no-cache")
	Conn.Response.Header.Set("Expires", "0")

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return content, true
}

// getCommentCaptureScript 获取评论采集脚本
func (h *ScriptHandler) getCommentCaptureScript() string {
	return `<script>
(function() {
	'use strict';
	
	console.log('[评论采集] 初始化评论采集系统...');
	
	// 保存评论数据的函数
	function saveCommentData(comments, options) {
		if (!comments || comments.length === 0) {
			console.log('[评论采集] 没有评论数据，跳过保存');
			return;
		}
		
		options = options || {};
		
		// 去重处理：移除重复的二级回复
		var deduplicatedComments = [];
		var totalLevel2Before = 0;
		var totalLevel2After = 0;
		
		for (var i = 0; i < comments.length; i++) {
			var comment = JSON.parse(JSON.stringify(comments[i])); // 深拷贝
			
			if (comment.levelTwoComment && Array.isArray(comment.levelTwoComment)) {
				totalLevel2Before += comment.levelTwoComment.length;
				
				// 使用commentId去重
				var seenIds = {};
				var uniqueReplies = [];
				
				for (var j = 0; j < comment.levelTwoComment.length; j++) {
					var reply = comment.levelTwoComment[j];
					var replyId = reply.commentId;
					
					if (!seenIds[replyId]) {
						seenIds[replyId] = true;
						uniqueReplies.push(reply);
					}
				}
				
				comment.levelTwoComment = uniqueReplies;
				totalLevel2After += uniqueReplies.length;
			}
			
			deduplicatedComments.push(comment);
		}
		
		// 如果有重复，输出日志
		if (totalLevel2Before > totalLevel2After) {
			console.log('[评论采集] 🔧 去重: 二级回复从 ' + totalLevel2Before + ' 条减少到 ' + totalLevel2After + ' 条 (移除 ' + (totalLevel2Before - totalLevel2After) + ' 条重复)');
		}
		
		// 计算实际总评论数（一级 + 二级）
		var actualTotalComments = deduplicatedComments.length + totalLevel2After;
		
		// 获取视频信息
		var videoId = '';
		var videoTitle = '';
		
		// 尝试从当前profile获取视频信息
		if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
			var profile = window.__wx_channels_store__.profile;
			videoId = profile.id || profile.nonce_id || '';
			videoTitle = profile.title || '';
		}
		
		// 如果没有从store获取到，尝试从options获取
		if (!videoId && options.videoId) {
			videoId = options.videoId;
		}
		if (!videoTitle && options.videoTitle) {
			videoTitle = options.videoTitle;
		}
		
		console.log('[评论采集] 准备保存评论数据:', {
			videoId: videoId,
			videoTitle: videoTitle,
			commentCount: actualTotalComments,
			level1Count: deduplicatedComments.length,
			level2Count: totalLevel2After,
			source: options.source || 'unknown'
		});
		
		// 获取原始评论数（从视频信息中）
		var originalCommentCount = 0;
		if (options.totalCount) {
			originalCommentCount = options.totalCount;
		} else if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
			originalCommentCount = window.__wx_channels_store__.profile.commentCount || 0;
		}
		
		// 发送评论数据到后端保存（使用去重后的数据）
		fetch('/__wx_channels_api/save_comment_data', {
			method: 'POST',
			headers: {'Content-Type': 'application/json'},
			body: JSON.stringify({
				comments: deduplicatedComments,
				videoId: videoId,
				videoTitle: videoTitle,
				originalCommentCount: originalCommentCount,
				timestamp: Date.now()
			})
		}).then(function(response) {
			if (response.ok) {
				console.log('[评论采集] ✓ 评论数据已保存到后端');
				
				// 保存成功后返回页面顶部（如果options中指定）
				if (options.scrollToTop !== false) {
					console.log('[评论采集] 📤 返回页面顶部');
					setTimeout(function() {
						// 使用与向下滚动相同的方法：找到第一个评论并滚动到它
						try {
							// 使用与 scrollToLastComment 相同的选择器
							var commentSelectors = [
								'[class*="comment-item"]',
								'[class*="CommentItem"]',
								'[class*="comment"]',
								'[class*="Comment"]'
							];
							
							var firstComment = null;
							var comments = null;
							
							// 尝试所有选择器找到评论
							for (var i = 0; i < commentSelectors.length; i++) {
								comments = document.querySelectorAll(commentSelectors[i]);
								if (comments.length > 0) {
									firstComment = comments[0];
									console.log('[评论采集] ✓ 找到', comments.length, '个评论，滚动到第一个');
									break;
								}
							}
							
							if (firstComment) {
								// 使用 scrollIntoView 滚动到第一个评论（与向下滚动相同的方法）
								console.log('[评论采集] ✓ 找到评论，使用 scrollIntoView 滚动到顶部');
								try {
									firstComment.scrollIntoView({ behavior: 'smooth', block: 'start' });
								} catch (e) {
									firstComment.scrollIntoView(true);
								}
							} else {
								// 如果找不到评论，使用标准方式
								console.log('[评论采集] ⚠️ 未找到评论元素，使用标准方式滚动');
								window.scrollTo({ top: 0, behavior: 'smooth' });
							}
							
							console.log('[评论采集] ✓ 已执行返回顶部操作');
						} catch (e) {
							console.error('[评论采集] 返回顶部失败:', e);
						}
					}, 1000);
				}
			} else {
				console.error('[评论采集] ✗ 保存评论数据失败:', response.status);
			}
		}).catch(function(error) {
			console.error('[评论采集] ✗ 保存评论数据出错:', error);
		});
	}
	
	// 将保存函数暴露到全局，供其他脚本使用
	window.__wx_channels_save_comment_data = saveCommentData;
	
	// 监控评论数据的变化
	var lastCommentSignature = '';
	var commentCheckInterval = null;
	var storeCheckAttempts = 0;
	var maxStoreCheckAttempts = 20; // 最多尝试20次（60秒）
	var isLoadingAllComments = false; // 标记是否正在加载全部评论
	var lastCommentCount = 0; // 记录上次的评论数量
	var pendingSaveTimer = null; // 延迟保存定时器
	var stableCheckCount = 0; // 稳定检查计数
	var autoScrollEnabled = false; // 是否启用自动滚动
	var autoScrollInterval = null; // 自动滚动定时器
	var noChangeCount = 0; // 评论数量未变化的次数
	
	function getCommentSignature(comments) {
		if (!comments || comments.length === 0) return '';
		// 使用评论数量和第一条、最后一条评论的ID生成签名
		var firstId = comments[0].id || comments[0].commentId || '';
		var lastId = comments[comments.length - 1].id || comments[comments.length - 1].commentId || '';
		return comments.length + '_' + firstId + '_' + lastId;
	}
	
	// 获取详细的评论统计信息
	function getCommentStats() {
		try {
			var rootElements = document.querySelectorAll('[data-v-app], #app, [id*="app"], [class*="app"]');
			for (var i = 0; i < Math.min(rootElements.length, 3); i++) {
				var el = rootElements[i];
				var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
				if (vueInstance) {
					var componentInstance = vueInstance.component || vueInstance;
					if (componentInstance) {
						var appContext = componentInstance.appContext || 
						                 (componentInstance.ctx && componentInstance.ctx.appContext);
						
						if (appContext && appContext.config && appContext.config.globalProperties) {
							if (appContext.config.globalProperties.$pinia) {
								var pinia = appContext.config.globalProperties.$pinia;
								var feedStore = null;
								
								if (pinia._s && pinia._s.feed) {
									feedStore = pinia._s.feed;
								} else if (pinia._s && pinia._s.get && typeof pinia._s.get === 'function') {
									feedStore = pinia._s.get('feed');
								} else if (pinia.state && pinia.state._value && pinia.state._value.feed) {
									feedStore = pinia.state._value.feed;
								}
								
								if (feedStore) {
									var commentList = feedStore.commentList || (feedStore.feed && feedStore.feed.commentList);
									if (commentList && commentList.dataList && commentList.dataList.items) {
										var items = commentList.dataList.items;
										var level1Count = items.length; // 一级评论数量
										var level2Count = 0; // 二级回复数量
										
										// 统计二级回复数量
										for (var j = 0; j < items.length; j++) {
											var item = items[j];
											if (item.levelTwoComment && Array.isArray(item.levelTwoComment)) {
												level2Count += item.levelTwoComment.length;
											}
										}
										
										return {
											level1: level1Count,
											level2: level2Count,
											total: level1Count + level2Count
										};
									}
								}
							}
						}
					}
				}
			}
		} catch (e) {
			// 静默失败
		}
		return { level1: 0, level2: 0, total: 0 };
	}
	
	// 获取当前评论数量（包括一级评论和二级回复）
	function getCurrentCommentCount() {
		try {
			var rootElements = document.querySelectorAll('[data-v-app], #app, [id*="app"], [class*="app"]');
			for (var i = 0; i < Math.min(rootElements.length, 3); i++) {
				var el = rootElements[i];
				var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
				if (vueInstance) {
					var componentInstance = vueInstance.component || vueInstance;
					if (componentInstance) {
						var appContext = componentInstance.appContext || 
						                 (componentInstance.ctx && componentInstance.ctx.appContext);
						
						if (appContext && appContext.config && appContext.config.globalProperties) {
							if (appContext.config.globalProperties.$pinia) {
								var pinia = appContext.config.globalProperties.$pinia;
								var feedStore = null;
								
								if (pinia._s && pinia._s.feed) {
									feedStore = pinia._s.feed;
								} else if (pinia._s && pinia._s.get && typeof pinia._s.get === 'function') {
									feedStore = pinia._s.get('feed');
								} else if (pinia.state && pinia.state._value && pinia.state._value.feed) {
									feedStore = pinia.state._value.feed;
								}
								
								if (feedStore) {
									var commentList = feedStore.commentList || (feedStore.feed && feedStore.feed.commentList);
									if (commentList && commentList.dataList && commentList.dataList.items) {
										var items = commentList.dataList.items;
										var totalCount = items.length; // 一级评论数量
										
										// 统计二级回复数量
										for (var j = 0; j < items.length; j++) {
											var item = items[j];
											// 检查是否有二级回复
											if (item.levelTwoComment && Array.isArray(item.levelTwoComment)) {
												totalCount += item.levelTwoComment.length;
											}
										}
										
										return totalCount;
									}
								}
							}
						}
					}
				}
			}
		} catch (e) {
			// 静默失败
		}
		return 0;
	}
	
	// 验证二级评论完整性：检查实际采集的二级评论数量是否与expandCommentCount一致
	function verifySecondaryCommentCompleteness() {
		try {
			var rootElements = document.querySelectorAll('[data-v-app], #app, [id*="app"], [class*="app"]');
			for (var i = 0; i < Math.min(rootElements.length, 3); i++) {
				var el = rootElements[i];
				var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
				if (vueInstance) {
					var componentInstance = vueInstance.component || vueInstance;
					if (componentInstance) {
						var appContext = componentInstance.appContext || 
						                 (componentInstance.ctx && componentInstance.ctx.appContext);
						
						if (appContext && appContext.config && appContext.config.globalProperties) {
							if (appContext.config.globalProperties.$pinia) {
								var pinia = appContext.config.globalProperties.$pinia;
								var feedStore = null;
								
								if (pinia._s && pinia._s.feed) {
									feedStore = pinia._s.feed;
								} else if (pinia._s && pinia._s.get && typeof pinia._s.get === 'function') {
									feedStore = pinia._s.get('feed');
								} else if (pinia.state && pinia.state._value && pinia.state._value.feed) {
									feedStore = pinia.state._value.feed;
								}
								
								if (feedStore) {
									var commentList = feedStore.commentList || (feedStore.feed && feedStore.feed.commentList);
									if (commentList && commentList.dataList && commentList.dataList.items) {
										var items = commentList.dataList.items;
										var totalExpected = 0; // 预期的二级评论总数
										var totalActual = 0;   // 实际采集的二级评论总数
										var incompleteComments = []; // 不完整的评论列表
										
										// 检查每条一级评论
										for (var j = 0; j < items.length; j++) {
											var item = items[j];
											var expected = item.expandCommentCount || 0;
											var actual = (item.levelTwoComment && Array.isArray(item.levelTwoComment)) ? item.levelTwoComment.length : 0;
											
											totalExpected += expected;
											totalActual += actual;
											
											// 如果实际数量少于预期数量，记录下来
											if (expected > 0 && actual < expected) {
												incompleteComments.push({
													commentId: item.commentId,
													content: (item.content || '').substring(0, 30),
													expected: expected,
													actual: actual,
													missing: expected - actual
												});
											}
										}
										
										return {
											totalExpected: totalExpected,
											totalActual: totalActual,
											incompleteComments: incompleteComments,
											isComplete: totalExpected === totalActual,
											completeness: totalExpected > 0 ? (totalActual / totalExpected * 100).toFixed(1) : 100
										};
									}
								}
							}
						}
					}
				}
			}
		} catch (e) {
			console.error('[二级评论验证] 验证失败:', e);
		}
		return {
			totalExpected: 0,
			totalActual: 0,
			incompleteComments: [],
			isComplete: true,
			completeness: 100
		};
	}
	
	// 查找评论滚动容器
	function findCommentScrollContainer() {
		var scrollableContainers = [];
		
		// 查找所有可滚动的元素
		function findScrollableElements(element, depth) {
			if (!element || depth > 10) {
				return;
			}
			
			// 跳过 body 和 html，稍后单独处理
			if (element === document.body || element === document.documentElement) {
				return;
			}
			
			var style = window.getComputedStyle(element);
			var overflowY = style.overflowY || style.overflow;
			var hasScrollStyle = (overflowY === 'auto' || overflowY === 'scroll' || overflowY === 'overlay');
			var hasScroll = hasScrollStyle && element.scrollHeight > element.clientHeight + 5; // 5px容差
			
			if (hasScroll) {
				// 检查是否包含评论项
				var commentItems = element.querySelectorAll('[class*="comment"], [class*="Comment"]');
				if (commentItems.length > 1) {
					var scrollableHeight = element.scrollHeight - element.clientHeight;
					scrollableContainers.push({
						element: element,
						commentCount: commentItems.length,
						scrollHeight: element.scrollHeight,
						clientHeight: element.clientHeight,
						scrollableHeight: scrollableHeight,
						className: element.className || '',
						id: element.id || ''
					});
					console.log('[评论采集] 发现可滚动容器:', element.tagName, element.className || element.id || '', 
					           '评论数:', commentItems.length, 
					           '可滚动高度:', scrollableHeight + 'px');
				}
			}
			
			// 递归查找子元素
			for (var i = 0; i < element.children.length; i++) {
				findScrollableElements(element.children[i], depth + 1);
			}
		}
		
		// 从 body 开始查找
		findScrollableElements(document.body, 0);
		
		// 如果找到可滚动的容器，选择可滚动高度最大且包含评论的
		if (scrollableContainers.length > 0) {
			// 优先选择可滚动高度最大的容器
			scrollableContainers.sort(function(a, b) {
				// 首先按可滚动高度排序
				if (Math.abs(a.scrollableHeight - b.scrollableHeight) > 100) {
					return b.scrollableHeight - a.scrollableHeight;
				}
				// 如果可滚动高度相近，按评论数量排序
				return b.commentCount - a.commentCount;
			});
			
			var bestContainer = scrollableContainers[0].element;
			console.log('[评论采集] ✓ 选择最佳滚动容器:', bestContainer.tagName, 
			           bestContainer.className || bestContainer.id || '', 
			           '包含', scrollableContainers[0].commentCount, '个评论项',
			           '可滚动高度:', scrollableContainers[0].scrollableHeight + 'px');
			return bestContainer;
		}
		
		// 检查页面本身是否可滚动
		var bodyScrollHeight = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
		var viewportHeight = window.innerHeight || document.documentElement.clientHeight;
		if (bodyScrollHeight > viewportHeight + 5) {
			console.log('[评论采集] 使用页面滚动 (window/body), 可滚动高度:', (bodyScrollHeight - viewportHeight) + 'px');
			return document.body;
		}
		
		// 如果都不可滚动，仍然返回body，但给出警告
		console.warn('[评论采集] ⚠️ 未找到可滚动容器，使用body作为默认容器');
		return document.body;
	}
	
	// 强制滚动到容器底部（不使用 smooth，立即执行）
	function scrollToBottom(container) {
		if (!container) return;
		
		// 如果是 body 或 html，使用 window.scrollTo
		if (container === document.body || container === document.documentElement) {
			// 获取页面最大滚动高度
			var maxScroll = Math.max(
				document.body.scrollHeight,
				document.documentElement.scrollHeight,
				document.body.offsetHeight,
				document.documentElement.offsetHeight
			);
			
			// 立即滚动（不使用 smooth）
			window.scrollTo(0, maxScroll);
			document.documentElement.scrollTop = maxScroll;
			document.body.scrollTop = maxScroll;
			
			// 多次尝试确保滚动成功
			setTimeout(function() {
				window.scrollTo(0, maxScroll);
				document.documentElement.scrollTop = maxScroll;
				document.body.scrollTop = maxScroll;
			}, 50);
			
			setTimeout(function() {
				window.scrollTo(0, maxScroll);
				document.documentElement.scrollTop = maxScroll;
				document.body.scrollTop = maxScroll;
			}, 200);
		} else {
			// 滚动容器本身
			var maxScroll = container.scrollHeight - container.clientHeight;
			container.scrollTop = maxScroll;
			
			// 多次尝试确保滚动成功
			setTimeout(function() {
				container.scrollTop = maxScroll;
			}, 50);
			
			setTimeout(function() {
				container.scrollTop = maxScroll;
			}, 200);
		}
	}
	
	// 缓存评论选择器，避免重复查询
	var cachedCommentSelector = null;
	var lastCommentElementCount = 0; // 记录上次找到的评论元素数量
	
	// 尝试找到评论列表的最后一个元素并滚动到它（优化版）
	function scrollToLastComment() {
		// 尝试多种选择器找到评论项
		var commentSelectors = [
			'[class*="comment-item"]',
			'[class*="CommentItem"]',
			'[class*="comment"]',
			'[class*="Comment"]'
		];
		
		var lastComment = null;
		var comments = null;
		var selector = null;
		
		// 如果之前找到过选择器，优先使用缓存的选择器
		if (cachedCommentSelector) {
			comments = document.querySelectorAll(cachedCommentSelector);
			if (comments.length > 0) {
				selector = cachedCommentSelector;
			}
		}
		
		// 如果缓存的选择器无效，尝试所有选择器
		if (!comments || comments.length === 0) {
			for (var i = 0; i < commentSelectors.length; i++) {
				comments = document.querySelectorAll(commentSelectors[i]);
				if (comments.length > 0) {
					selector = commentSelectors[i];
					cachedCommentSelector = selector; // 缓存有效的选择器
					break;
				}
			}
		}
		
		if (comments && comments.length > 0) {
			lastComment = comments[comments.length - 1];
			
			// 只在评论数量变化时输出日志（减少日志量）
			if (comments.length !== lastCommentElementCount) {
				console.log('[评论采集] 找到评论项:', comments.length, '个，滚动到最后一个');
				lastCommentElementCount = comments.length;
			}
			
			// 检查最后一个评论是否已经在视口内（避免不必要的滚动）
			var rect = lastComment.getBoundingClientRect();
			var viewportHeight = window.innerHeight || document.documentElement.clientHeight;
			var isVisible = rect.top >= 0 && rect.top < viewportHeight;
			
			// 如果最后一个评论已经在视口内，滚动到稍微下面一点以触发加载
			if (isVisible) {
				// 滚动到稍微下面一点，确保触发加载更多（增加滚动距离）
				var scrollY = window.pageYOffset || document.documentElement.scrollTop || document.body.scrollTop;
				var targetScroll = scrollY + rect.bottom + 500; // 增加滚动距离到500px，确保触发加载
				
				// 多次尝试滚动，确保生效
				window.scrollTo(0, targetScroll);
				document.documentElement.scrollTop = targetScroll;
				document.body.scrollTop = targetScroll;
				
				// 延迟再次滚动，确保生效
				setTimeout(function() {
					window.scrollTo(0, targetScroll);
					document.documentElement.scrollTop = targetScroll;
					document.body.scrollTop = targetScroll;
				}, 100);
			} else {
				// 如果不在视口内，使用 scrollIntoView 滚动到它
				try {
					lastComment.scrollIntoView({ behavior: 'auto', block: 'end' });
				} catch (e) {
					// 如果不支持参数，使用默认方式
					lastComment.scrollIntoView(false);
				}
				
				// 滚动后再稍微向下滚动一点，确保触发加载（增加滚动距离）
				setTimeout(function() {
					var rect2 = lastComment.getBoundingClientRect();
					var scrollY2 = window.pageYOffset || document.documentElement.scrollTop || document.body.scrollTop;
					var targetScroll2 = scrollY2 + rect2.bottom + 500; // 增加滚动距离到500px
					
					// 多次尝试滚动，确保生效
					window.scrollTo(0, targetScroll2);
					document.documentElement.scrollTop = targetScroll2;
					document.body.scrollTop = targetScroll2;
					
					// 再次延迟滚动
					setTimeout(function() {
						window.scrollTo(0, targetScroll2);
						document.documentElement.scrollTop = targetScroll2;
						document.body.scrollTop = targetScroll2;
					}, 100);
				}, 100);
			}
			
			return true;
		}
		
		// 如果找不到评论，清除缓存
		cachedCommentSelector = null;
		lastCommentElementCount = 0;
		
		return false;
	}
	
	// 尝试直接调用 Vue Store 的加载更多方法
	function tryLoadMoreComments() {
		try {
			var rootElements = document.querySelectorAll('[data-v-app], #app, [id*="app"], [class*="app"]');
			for (var i = 0; i < Math.min(rootElements.length, 3); i++) {
				var el = rootElements[i];
				var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
				if (vueInstance) {
					var componentInstance = vueInstance.component || vueInstance;
					if (componentInstance) {
						var appContext = componentInstance.appContext || 
						                 (componentInstance.ctx && componentInstance.ctx.appContext);
						
						if (appContext && appContext.config && appContext.config.globalProperties) {
							if (appContext.config.globalProperties.$pinia) {
								var pinia = appContext.config.globalProperties.$pinia;
								var feedStore = null;
								
								if (pinia._s && pinia._s.feed) {
									feedStore = pinia._s.feed;
								} else if (pinia._s && pinia._s.get && typeof pinia._s.get === 'function') {
									feedStore = pinia._s.get('feed');
								} else if (pinia.state && pinia.state._value && pinia.state._value.feed) {
									feedStore = pinia.state._value.feed;
								}
								
								if (feedStore) {
									var commentList = feedStore.commentList || (feedStore.feed && feedStore.feed.commentList);
									if (commentList) {
										// 尝试调用加载更多的方法
										var methods = ['loadMore', 'loadMoreComments', 'fetchMore', 'getMore', 'loadNextPage', 'nextPage'];
										for (var j = 0; j < methods.length; j++) {
											if (typeof commentList[methods[j]] === 'function') {
												console.log('[评论采集] 尝试调用方法:', methods[j]);
												try {
													commentList[methods[j]]();
													return true;
												} catch (e) {
													console.log('[评论采集] 调用方法失败:', methods[j], e.message);
												}
											}
										}
										
										// 尝试调用 feedStore 的方法
										for (var j = 0; j < methods.length; j++) {
											if (typeof feedStore[methods[j]] === 'function') {
												console.log('[评论采集] 尝试调用 feedStore 方法:', methods[j]);
												try {
													feedStore[methods[j]]();
													return true;
												} catch (e) {
													console.log('[评论采集] 调用 feedStore 方法失败:', methods[j], e.message);
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		} catch (e) {
			console.log('[评论采集] 尝试调用加载方法失败:', e.message);
		}
		
		return false;
	}
	
	// 自动加载所有评论 - 通过滚动触发加载
	function startAutoScroll(totalCount) {
		if (autoScrollEnabled) {
			console.log('[评论采集] 自动加载已在运行中');
			return;
		}
		
		autoScrollEnabled = true;
		noChangeCount = 0;
		var loadAttempts = 0;
		var maxLoadAttempts = 200; // 最多尝试200次（约10分钟）
		var lastCount = getCurrentCommentCount();
		lastCommentCount = lastCount;
		
		// 初始化时输出当前状态
		if (lastCount > 0) {
			console.log('[评论采集] 初始评论数: ' + lastCount);
		}
		
		console.log('[评论采集] 🚀 开始自动滚动加载评论');
		var initialStats = getCommentStats();
		console.log('[评论采集] 当前评论数: ' + lastCount + ' (一级:' + initialStats.level1 + ' + 二级:' + initialStats.level2 + ')');
		if (totalCount > 0) {
			console.log('[评论采集] 目标评论数: ' + totalCount);
		}
		
		// 查找评论滚动容器
		var scrollContainer = findCommentScrollContainer();
		
		// 检查容器是否可滚动
		var canScroll = false;
		var scrollableHeight = 0;
		if (scrollContainer === document.body || scrollContainer === document.documentElement) {
			var maxScroll = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
			var viewportHeight = window.innerHeight || document.documentElement.clientHeight;
			scrollableHeight = maxScroll - viewportHeight;
			canScroll = scrollableHeight > 5;
			console.log('[评论采集] 页面滚动检查: 总高度=' + maxScroll + 'px, 视口=' + viewportHeight + 'px, 可滚动=' + scrollableHeight + 'px');
		} else {
			scrollableHeight = scrollContainer.scrollHeight - scrollContainer.clientHeight;
			canScroll = scrollableHeight > 5;
			console.log('[评论采集] 容器滚动检查: 总高度=' + scrollContainer.scrollHeight + 'px, 可见=' + scrollContainer.clientHeight + 'px, 可滚动=' + scrollableHeight + 'px');
		}
		
		if (!canScroll) {
			console.warn('[评论采集] ⚠️ 警告: 容器不可滚动（可滚动高度=' + scrollableHeight + 'px），可能无法加载更多评论');
			console.warn('[评论采集] ⚠️ 尝试使用替代方法加载评论...');
			
			// 如果容器不可滚动，尝试直接调用加载方法
			var loadSuccess = tryLoadMoreComments();
			if (!loadSuccess) {
				// 如果调用失败，尝试模拟用户交互
				console.log('[评论采集] 尝试模拟用户交互触发加载...');
				
				// 多次尝试点击按钮，因为点击后可能会出现新的按钮
				var totalClicked = 0;
				for (var attempt = 0; attempt < 3; attempt++) {
					var clicked = clickAllLoadMoreButtons();
					if (clicked) {
						totalClicked++;
						// 等待一小段时间让DOM更新（使用同步延迟）
						var start = Date.now();
						while (Date.now() - start < 500) {
							// 等待500ms
						}
					} else {
						break; // 没有找到按钮，停止尝试
					}
				}
				
				if (totalClicked > 0) {
					console.log('[评论采集] 完成', totalClicked, '轮按钮点击');
				}
			}
		}
		
		// 查找并点击所有"加载更多"按钮
		function clickAllLoadMoreButtons() {
			var clickedCount = 0;
			var clickedButtons = []; // 记录已点击的按钮，避免重复点击
			
			// 查找各种可能的"加载更多"按钮
			var selectors = [
				'[class*="load-more"]',
				'[class*="LoadMore"]',
				'[class*="more-comment"]',
				'[class*="MoreComment"]',
				'[class*="展开"]',
				'[class*="expand"]',
				'[class*="Expand"]',
				'[class*="reply"]',
				'[class*="Reply"]',
				'button',
				'div[role="button"]',
				'span[role="button"]',
				'a'
			];
			
			for (var s = 0; s < selectors.length; s++) {
				var buttons = document.querySelectorAll(selectors[s]);
				for (var i = 0; i < buttons.length; i++) {
					var btn = buttons[i];
					
					// 避免重复点击
					if (clickedButtons.indexOf(btn) !== -1) {
						continue;
					}
					
					var btnText = (btn.textContent || btn.innerText || '').trim();
					
					// 检查按钮文本是否包含加载更多的关键词
					if (btnText && (
						btnText.includes('更多') || 
						btnText.includes('展开') || 
						btnText.includes('加载') ||
						btnText.includes('回复') ||
						btnText.includes('条回复') ||
						btnText.toLowerCase().includes('more') || 
						btnText.toLowerCase().includes('load') ||
						btnText.toLowerCase().includes('expand') ||
						btnText.toLowerCase().includes('show') ||
						btnText.toLowerCase().includes('reply') ||
						btnText.toLowerCase().includes('replies')
					)) {
						// 检查按钮是否可见
						var rect = btn.getBoundingClientRect();
						var isVisible = rect.width > 0 && rect.height > 0;
						
						if (isVisible) {
							console.log('[评论采集] 找到加载按钮:', btnText.substring(0, 50));
							try {
								btn.click();
								clickedButtons.push(btn);
								clickedCount++;
								console.log('[评论采集] ✓ 已点击按钮');
								
								// 点击后等待一小段时间，让DOM更新
								// 注意：这里不能用setTimeout，因为函数是同步的
							} catch (e) {
								console.log('[评论采集] 点击按钮失败:', e.message);
							}
						}
					}
				}
			}
			
			if (clickedCount > 0) {
				console.log('[评论采集] 共点击了', clickedCount, '个加载按钮');
			} else {
				console.log('[评论采集] 未找到可点击的加载按钮');
			}
			
			return clickedCount > 0;
		}
		
		// 展开所有二级评论（回复）的函数
		function expandAllSecondaryComments() {
			var expandedCount = 0;
			var totalAttempts = 0;
			
			// 1. 找到所有一级评论容器
			var commentSelectors = [
				'[class*="comment-item"]',
				'[class*="CommentItem"]',
				'[class*="comment-card"]',
				'[class*="CommentCard"]',
				'[class*="comment"]',
				'[class*="Comment"]'
			];
			
			var commentItems = [];
			for (var i = 0; i < commentSelectors.length; i++) {
				var items = document.querySelectorAll(commentSelectors[i]);
				if (items.length > 0) {
					commentItems = Array.from(items);
					console.log('[二级评论] 使用选择器:', commentSelectors[i], '找到', items.length, '个评论');
					break;
				}
			}
			
			if (commentItems.length === 0) {
				console.log('[二级评论] 未找到评论容器');
				return 0;
			}
			
			console.log('[二级评论] 开始检查', commentItems.length, '个一级评论的回复按钮');
			
			// 调试：输出第一个评论的HTML结构（仅前500字符）
			if (commentItems.length > 0) {
				var firstItemHtml = commentItems[0].innerHTML;
				if (firstItemHtml && firstItemHtml.length > 0) {
					console.log('[二级评论] 第一个评论的HTML片段:', firstItemHtml.substring(0, 500));
				}
			}
			
			// 2. 在每个一级评论中查找并点击回复按钮
			for (var idx = 0; idx < commentItems.length; idx++) {
				var item = commentItems[idx];
				
				// 查找回复按钮的多种可能选择器
				var replyButtonSelectors = [
					'[class*="reply-btn"]',
					'[class*="ReplyBtn"]',
					'[class*="show-reply"]',
					'[class*="ShowReply"]',
					'[class*="view-reply"]',
					'[class*="ViewReply"]',
					'[class*="more-reply"]',
					'[class*="MoreReply"]',
					'[class*="expand-reply"]',
					'[class*="ExpandReply"]',
					'button',
					'div[role="button"]',
					'span[role="button"]',
					'a'
				];
				
				// 调试：记录找到的所有按钮文本（仅第一个评论）
				if (idx === 0) {
					var debugButtons = [];
					for (var ds = 0; ds < replyButtonSelectors.length; ds++) {
						var debugBtns = item.querySelectorAll(replyButtonSelectors[ds]);
						for (var db = 0; db < Math.min(debugBtns.length, 5); db++) {
							var debugText = (debugBtns[db].textContent || debugBtns[db].innerText || '').trim();
							if (debugText && debugText.length > 0 && debugText.length < 100) {
								debugButtons.push(debugText);
							}
						}
					}
					if (debugButtons.length > 0) {
						console.log('[二级评论] 第一个评论中找到的按钮文本:', debugButtons.slice(0, 10).join(' | '));
					} else {
						console.log('[二级评论] 第一个评论中未找到任何按钮');
					}
				}
				
				for (var s = 0; s < replyButtonSelectors.length; s++) {
					var buttons = item.querySelectorAll(replyButtonSelectors[s]);
					
					for (var b = 0; b < buttons.length; b++) {
						var btn = buttons[b];
						var btnText = (btn.textContent || btn.innerText || '').trim();
						
						// 检查是否是回复相关按钮（更精确的匹配）
						var isReplyButton = false;
						if (btnText) {
							// 清理文本：移除多余空格和换行符
							var cleanText = btnText.replace(/\s+/g, ' ').trim();
							
							// 匹配"X条回复"、"查看回复"、"展开回复"等
							if (cleanText.match(/\d+\s*条回复/) ||
							    cleanText.match(/\d+\s*repl(y|ies)/i) ||
							    cleanText.includes('条回复') ||
							    cleanText.includes('查看回复') ||
							    cleanText.includes('展开回复') ||
							    cleanText.includes('更多回复') ||
							    cleanText.includes('显示回复') ||
							    (cleanText.includes('回复') && cleanText.length < 20) || // 单独的"回复"字样，且文本较短
							    (cleanText.toLowerCase().includes('view') && cleanText.toLowerCase().includes('repl')) ||
							    (cleanText.toLowerCase().includes('show') && cleanText.toLowerCase().includes('repl')) ||
							    (cleanText.toLowerCase().includes('more') && cleanText.toLowerCase().includes('repl')) ||
							    (cleanText.toLowerCase().includes('expand') && cleanText.toLowerCase().includes('repl'))) {
								isReplyButton = true;
							}
							
							// 调试：输出未匹配的按钮（仅前3个评论）
							if (!isReplyButton && idx < 3 && cleanText.length > 0 && cleanText.length < 50) {
								console.log('[二级评论] 第', idx + 1, '个评论: 未匹配按钮 "' + cleanText + '"');
							}
						}
						
						if (isReplyButton) {
							totalAttempts++;
							
							// 检查按钮是否可见
							var rect = btn.getBoundingClientRect();
							if (rect.width > 0 && rect.height > 0) {
								try {
									console.log('[二级评论] 第', idx + 1, '个评论: 点击 "' + btnText.substring(0, 30) + '"');
									btn.click();
									expandedCount++;
								} catch (e) {
									console.warn('[二级评论] 点击失败:', e.message);
								}
							}
						}
					}
				}
			}
			
			if (expandedCount > 0) {
				console.log('[二级评论] ✓ 展开操作完成: 尝试', totalAttempts, '次, 成功', expandedCount, '次');
			} else if (totalAttempts > 0) {
				console.log('[二级评论] ⚠️ 找到', totalAttempts, '个回复按钮但都不可见');
			}
			
			return expandedCount;
		}
		
		// 多轮展开二级评论（异步版本，使用回调）
		var isExpandingSecondaryComments = false;
		function expandSecondaryCommentsInRounds(maxRounds, callback) {
			if (isExpandingSecondaryComments) {
				console.log('[二级评论] 已有展开任务在运行中');
				return;
			}
			
			isExpandingSecondaryComments = true;
			var round = 0;
			maxRounds = maxRounds || 3;
			
			function performRound() {
				round++;
				console.log('[二级评论] 🔄 开始第', round, '/', maxRounds, '轮展开...');
				
				var expandCount = expandAllSecondaryComments();
				
				// 等待DOM更新后继续下一轮
				setTimeout(function() {
					// 如果还有按钮被点击，或者还没达到最大轮数，继续下一轮
					if (round < maxRounds && (expandCount > 0 || round === 1)) {
						performRound();
					} else {
						console.log('[二级评论] ✓ 所有轮次完成 (共', round, '轮)');
						isExpandingSecondaryComments = false;
						if (callback) callback();
					}
				}, 1500); // 每轮之间等待1.5秒
			}
			
			performRound();
		}
		
		// 增量滚动距离（像素）
		var scrollStep = 300; // 每次滚动300px（增加初始步长）
		var lastScrollPosition = 0;
		var isScrolling = false; // 防止并发滚动
		var scrollThrottle = 0; // 滚动节流计数器
		
		// 增量滚动加载函数（优化版：每次滚动一小段，检查新数据，添加错误处理）
		function performScrollLoad() {
			// 防止并发执行
			if (isScrolling) {
				return;
			}
			
			try {
				loadAttempts++;
				isScrolling = true;
				
				// 获取当前滚动位置
				var currentScrollPos = 0;
				var maxScroll = 0;
				try {
					if (scrollContainer === document.body || scrollContainer === document.documentElement) {
						currentScrollPos = window.pageYOffset || document.documentElement.scrollTop || document.body.scrollTop;
						maxScroll = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
					} else {
						currentScrollPos = scrollContainer.scrollTop;
						maxScroll = scrollContainer.scrollHeight;
					}
				} catch (e) {
					console.error('[评论采集] 获取滚动位置失败:', e);
					isScrolling = false;
					return;
				}
				
				// 记录当前评论数量（滚动前）
				var countBeforeScroll = 0;
				try {
					countBeforeScroll = getCurrentCommentCount();
				} catch (e) {
					console.error('[评论采集] 获取评论数量失败:', e);
				}
				
				// 优先使用滚动到最后一个评论的方法（这是最有效的方法）
				var scrolledToComment = false;
				try {
					// 总是尝试滚动到最后一个评论（这个方法最有效）
					scrolledToComment = scrollToLastComment();
					
					// 如果滚动到评论失败，尝试增量滚动
					if (!scrolledToComment) {
						var targetScrollPos = currentScrollPos + scrollStep;
						if (scrollContainer === document.body || scrollContainer === document.documentElement) {
							window.scrollTo(0, targetScrollPos);
							document.documentElement.scrollTop = targetScrollPos;
							document.body.scrollTop = targetScrollPos;
						} else {
							scrollContainer.scrollTop = targetScrollPos;
						}
					}
				} catch (e) {
					console.error('[评论采集] 滚动操作失败:', e);
				}
				
				// 触发滚动事件（确保监听器被触发）
				try {
					var scrollEvent = new Event('scroll', { bubbles: true, cancelable: true });
					if (scrollContainer === document.body || scrollContainer === document.documentElement) {
						window.dispatchEvent(scrollEvent);
						document.dispatchEvent(scrollEvent);
					} else {
						scrollContainer.dispatchEvent(scrollEvent);
					}
				} catch (e) {
					console.error('[评论采集] 触发滚动事件失败:', e);
				}
				
				// 验证滚动是否生效（延迟检查）
				setTimeout(function() {
					try {
						var newScrollPos = 0;
						if (scrollContainer === document.body || scrollContainer === document.documentElement) {
							newScrollPos = window.pageYOffset || document.documentElement.scrollTop || document.body.scrollTop;
						} else {
							newScrollPos = scrollContainer.scrollTop;
						}
						
						// 如果滚动位置没有变化，说明滚动可能无效，强制使用 scrollToLastComment
						if (Math.abs(newScrollPos - currentScrollPos) < 10 && loadAttempts > 3) {
							if (loadAttempts % 5 === 0) {
								console.log('[评论采集] ⚠️ 滚动位置未变化，强制使用滚动到评论方法');
							}
							scrollToLastComment();
						}
					} catch (e) {
						// 忽略错误
					}
				}, 100);
			
				// 只在第一次和每10次输出日志（减少日志量）
				if (loadAttempts === 1 || loadAttempts % 10 === 0) {
					var logTargetPos = scrolledToComment ? '滚动到评论' : (currentScrollPos + scrollStep);
					console.log('[评论采集] 🔽 增量滚动 (第' + loadAttempts + '次) - 位置: ' + Math.round(currentScrollPos) + ' -> ' + (scrolledToComment ? '滚动到评论' : Math.round(currentScrollPos + scrollStep)));
				}
				
				// 如果容器不可滚动且滚动位置没有变化，快速进入最终检查
				if (!canScroll && loadAttempts >= 2) {
					console.log('[评论采集] 容器不可滚动且已尝试' + loadAttempts + '次，快速进入最终检查');
					noChangeCount = 20; // 直接设置为触发最终检查的阈值
				}
				
				// 滚动后等待一段时间再检查评论数量（给页面时间加载新内容）
				// 使用多次检查机制，确保捕获到数据变化
				var checkDelay = 2500; // 增加到2.5秒
				var recheckDelay = 1500; // 如果第一次没变化，1.5秒后再检查一次
				
				setTimeout(function() {
					try {
						// 第一次检查：获取当前评论数量（滚动后）
						var currentCount = 0;
						try {
							currentCount = getCurrentCommentCount();
							lastCommentCount = currentCount;
						} catch (e) {
							console.error('[评论采集] 获取评论数量失败:', e);
							isScrolling = false;
							return;
						}
						
						// 如果第一次检查发现有新数据，立即处理
						if (currentCount > countBeforeScroll) {
							console.log('[评论采集] ✓ 第一次检查发现新数据: ' + countBeforeScroll + ' -> ' + currentCount);
							handleCountChange(currentCount, countBeforeScroll);
							return;
						}
						
						// 如果第一次没有新数据，等待后再检查一次（可能数据还在加载中）
						setTimeout(function() {
							try {
								var recheckCount = getCurrentCommentCount();
								if (recheckCount > currentCount) {
									console.log('[评论采集] ✓ 第二次检查发现新数据: ' + currentCount + ' -> ' + recheckCount);
									currentCount = recheckCount;
									lastCommentCount = recheckCount;
								}
								handleCountChange(currentCount, countBeforeScroll);
							} catch (e) {
								console.error('[评论采集] 第二次检查失败:', e);
								handleCountChange(currentCount, countBeforeScroll);
							}
						}, recheckDelay);
						
					} catch (e) {
						console.error('[评论采集] 滚动检查失败:', e);
						isScrolling = false;
					}
				}, checkDelay);
				
				// 处理评论数量变化的函数
				function handleCountChange(currentCount, countBeforeScroll) {
					try {
				
						// 检查是否完成（允许1条误差）
						if (totalCount > 0 && currentCount >= totalCount - 1) {
							console.log('[评论采集] ✅ 已加载全部评论 (' + currentCount + '/' + totalCount + ')');
							isScrolling = false;
							stopAutoScroll(true);
							return;
						}
						
						// 检查是否超时
						if (loadAttempts > maxLoadAttempts) {
							console.log('[评论采集] ⚠️ 达到最大尝试次数 (' + maxLoadAttempts + ')');
							if (totalCount > 0 && currentCount < totalCount) {
								console.warn('[评论采集] ⚠️ 未能加载全部评论: ' + currentCount + '/' + totalCount + ' (差' + (totalCount - currentCount) + '条)');
							}
							isScrolling = false;
							stopAutoScroll(true);
							return;
						}
				
						// 检查是否有新数据（与滚动前比较）
						var hasNewData = currentCount > countBeforeScroll;
						
						// 检查评论数量变化（与上次记录比较）
						if (currentCount !== lastCount) {
							noChangeCount = 0;
							var progress = totalCount > 0 ? Math.round(currentCount / totalCount * 100) : '?';
							var newComments = currentCount - lastCount;
							// 获取详细统计信息
							var stats = getCommentStats();
							console.log('[评论采集] 📊 进度: ' + currentCount + '/' + (totalCount || '?') + ' (' + progress + '%) - 新增: ' + newComments + ' (一级:' + stats.level1 + ' + 二级:' + stats.level2 + ')');
							lastCount = currentCount;
							
							// 发现新数据，继续滚动（保持当前滚动距离）
							scrollStep = 200; // 重置为默认值
						} else {
							// 没有新数据
							noChangeCount++;
							
							// 如果连续多次无新数据，尝试直接调用加载方法和点击按钮
							if (noChangeCount === 2 || noChangeCount === 5 || noChangeCount === 8) {
								console.log('[评论采集] 尝试直接调用加载方法...');
								tryLoadMoreComments();
								
								// 同时尝试点击加载更多按钮
								console.log('[评论采集] 尝试点击加载更多按钮...');
								clickAllLoadMoreButtons();
								
								// 尝试展开二级评论
								if (noChangeCount === 5) {
									console.log('[评论采集] 尝试展开二级评论...');
									expandAllSecondaryComments();
								}
							}
							
							// 如果没有新数据，不要急于增加滚动距离，保持稳定
							// 因为可能是数据还在加载中，而不是需要滚动更多
							if (noChangeCount > 5 && scrollStep < 500) {
								scrollStep = Math.min(scrollStep + 50, 500); // 缓慢增加滚动距离
							}
							
							// 如果连续多次无新数据，强制滚动到最后一个评论和底部
							if (noChangeCount > 3 && noChangeCount % 3 === 0) {
								console.log('[评论采集] 强制滚动到最后一个评论和底部...');
								scrollToLastComment();
								setTimeout(function() {
									scrollToBottom(scrollContainer);
								}, 500);
							}
							
							if (loadAttempts % 5 === 0 || loadAttempts <= 3) {
								// 前3次和每5次输出一次日志
								var progress = totalCount > 0 ? Math.round(currentCount / totalCount * 100) : '?';
								var scrollInfo = '';
								try {
									if (scrollContainer === document.body || scrollContainer === document.documentElement) {
										var currentScroll = window.pageYOffset || document.documentElement.scrollTop || document.body.scrollTop;
										var maxScroll = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
										scrollInfo = ' | 滚动位置: ' + Math.round(currentScroll) + '/' + maxScroll;
									} else {
										scrollInfo = ' | 容器滚动: ' + Math.round(scrollContainer.scrollTop) + '/' + scrollContainer.scrollHeight;
									}
								} catch (e) {
									// 忽略错误
								}
								console.log('[评论采集] 📊 进度: ' + currentCount + '/' + (totalCount || '?') + ' (' + progress + '%) - 无新数据，继续滚动 (步长: ' + scrollStep + 'px, 无变化次数: ' + noChangeCount + ')' + scrollInfo);
							}
						}
						
						// 如果连续20次无变化，进行最终检查（增加阈值，确保完整性）
						if (noChangeCount >= 20) {
							// 只在第一次触发时输出日志，避免重复
							if (noChangeCount === 20) {
								console.log('[评论采集] ⚠️ 评论数量连续20次无变化，进行最终检查...');
								console.log('[评论采集] 🔍 正在进行深度检查，确保不遗漏评论...');
							}
							
							// 如果接近总数但还没达到，进行多次延迟检查（降低阈值到60%，更早触发）
							if (totalCount > 0 && currentCount < totalCount && currentCount >= totalCount * 0.6) {
								// 只在第一次触发时输出日志
								if (noChangeCount === 20) {
									console.log('[评论采集] 接近完成（' + currentCount + '/' + totalCount + '），进行延迟检查...');
								}
								
								// 进行5次延迟检查，每次间隔5秒（增加检查次数和间隔，提高完整度）
								var finalCheckCount = 0;
								var maxFinalChecks = 5;
								
								function performFinalCheck() {
									try {
										finalCheckCount++;
										
										// 第一次最终检查时，先展开所有二级评论
										if (finalCheckCount === 1) {
											console.log('[评论采集] 🔍 最终检查: 展开所有二级评论...');
											expandSecondaryCommentsInRounds(3, function() {
												console.log('[评论采集] ✓ 二级评论展开完成，继续最终检查');
												// 展开完成后继续滚动检查
												continueFinalCheck();
											});
											return; // 等待展开完成
										}
										
										continueFinalCheck();
									} catch (e) {
										console.error('[评论采集] 最终检查失败:', e);
										isScrolling = false;
										stopAutoScroll(true);
									}
								}
								
								function continueFinalCheck() {
									try {
										// 每次检查时都尝试多次滚动，确保触发加载
										scrollToLastComment();
										
										// 尝试点击所有加载更多按钮
										setTimeout(function() {
											console.log('[评论采集] 最终检查: 尝试点击加载按钮...');
											clickAllLoadMoreButtons();
										}, 300);
										
										// 额外滚动到底部，确保触发加载
										setTimeout(function() {
											scrollToBottom(scrollContainer);
										}, 600);
										
										// 再次滚动到最后一个评论
										setTimeout(function() {
											scrollToLastComment();
										}, 1200);
										
										// 等待一段时间后检查评论数量（增加等待时间）
										setTimeout(function() {
											var finalCount = getCurrentCommentCount();
											
											// 验证二级评论完整性
											var verification = verifySecondaryCommentCompleteness();
											if (verification.totalExpected > 0) {
												console.log('[评论采集] 📊 二级评论验证: ' + verification.totalActual + '/' + verification.totalExpected + ' (' + verification.completeness + '%)');
												
												// 如果不完整且还有检查次数，输出详情
												if (!verification.isComplete && verification.incompleteComments.length > 0 && finalCheckCount < maxFinalChecks) {
													console.log('[评论采集] ⚠️ 发现 ' + verification.incompleteComments.length + ' 条评论的回复不完整');
													for (var vi = 0; vi < Math.min(verification.incompleteComments.length, 3); vi++) {
														var inc = verification.incompleteComments[vi];
														console.log('[评论采集]   - "' + inc.content + '..." 缺少 ' + inc.missing + ' 条回复 (' + inc.actual + '/' + inc.expected + ')');
													}
													
													// 如果完整度低于90%，再次尝试展开
													if (parseFloat(verification.completeness) < 90) {
														console.log('[评论采集] 🔄 二级评论完整度低于90%，再次尝试展开...');
														expandAllSecondaryComments();
													}
												} else if (verification.isComplete) {
													console.log('[评论采集] ✓ 二级评论完整度验证通过！');
												}
											}
											
											console.log('[评论采集] 最终检查 ' + finalCheckCount + '/' + maxFinalChecks + ': ' + finalCount + '/' + totalCount);
											
											// 如果第一次检查没有变化，再等待一段时间后再次检查
											if (finalCount === currentCount && finalCheckCount <= maxFinalChecks - 2) {
												setTimeout(function() {
													var recheckCount = getCurrentCommentCount();
													if (recheckCount > finalCount) {
														console.log('[评论采集] ✓ 延迟检查发现新数据: ' + finalCount + ' -> ' + recheckCount);
														finalCount = recheckCount;
													}
													processFinalCheckResult(finalCount);
												}, 2000); // 再等待2秒
												return;
											}
											
											processFinalCheckResult(finalCount);
										}, 2500); // 增加到2.5秒
										
										function processFinalCheckResult(finalCount) {
											
											if (finalCount > currentCount) {
												// 发现新评论，继续加载
												console.log('[评论采集] ✓ 发现新评论 (' + currentCount + ' -> ' + finalCount + ')，继续加载');
												noChangeCount = 0;
												lastCount = finalCount;
												lastCommentCount = finalCount;
												currentCount = finalCount; // 更新当前计数
												
												// 重新启动滚动加载（定时器应该还在运行，只需要重置标志）
												autoScrollEnabled = true; // 确保标志为true
												isScrolling = false; // 释放滚动锁
												
												// 立即滚动到最后一个评论，触发加载
												scrollToLastComment();
												
												// 立即执行一次滚动
												setTimeout(function() {
													performScrollLoad();
												}, 1000);
												return;
											}
											
											if (finalCheckCount < maxFinalChecks) {
												// 继续检查（增加间隔到5秒，给网络更多时间）
												console.log('[评论采集] ⏳ 预计还需要 ' + ((maxFinalChecks - finalCheckCount) * 5) + ' 秒完成检查');
												setTimeout(performFinalCheck, 5000);
											} else {
												// 最终确认停止
												console.log('[评论采集] 最终评论数: ' + finalCount + (totalCount > 0 ? ' / ' + totalCount : ''));
												
												// 如果还是没达到总数，给出警告
												if (totalCount > 0 && finalCount < totalCount) {
													console.warn('[评论采集] ⚠️ 未能加载全部评论: ' + finalCount + '/' + totalCount + ' (差' + (totalCount - finalCount) + '条)');
												}
												
												isScrolling = false;
												stopAutoScroll(true);
											}
										}
									} catch (e) {
										console.error('[评论采集] 最终检查失败:', e);
										isScrolling = false;
										stopAutoScroll(true);
									}
								}
								
								// 延迟5秒后开始最终检查（给予更多时间）
								setTimeout(performFinalCheck, 5000);
								isScrolling = false;
								return;
							}
							
							// 如果不接近总数，直接停止
							console.log('[评论采集] 最终评论数: ' + currentCount + (totalCount > 0 ? ' / ' + totalCount : ''));
							isScrolling = false;
							stopAutoScroll(true);
							return;
						}
						
						// 释放滚动锁，允许下次滚动
						isScrolling = false;
					} catch (e) {
						console.error('[评论采集] handleCountChange失败:', e);
						isScrolling = false;
					}
				}
			} catch (e) {
				console.error('[评论采集] 滚动加载失败:', e);
				isScrolling = false;
			}
		}
		
		// 立即执行第一次滚动
		performScrollLoad();
		
		// 设置定时器，每4秒滚动一次（增加间隔，给予更多时间加载数据）
		// 考虑到每次滚动后会等待2.5秒+1.5秒=4秒来检查数据，所以总周期约8秒
		autoScrollInterval = setInterval(performScrollLoad, 4000);
	}
	
	// 停止自动加载
	function stopAutoScroll(scrollToTop) {
		if (autoScrollInterval) {
			clearInterval(autoScrollInterval);
			autoScrollInterval = null;
		}
		autoScrollEnabled = false;
		noChangeCount = 0;
		
		if (scrollToTop) {
			console.log('[评论采集] 📤 返回顶部');
			window.scrollTo({ top: 0, behavior: 'smooth' });
			
			// 加载完成后，进行检查确保获取到所有评论
			var saveCheckCount = 0;
			var maxSaveChecks = 2; // 最多检查2次（减少重复检查）
			var lastSaveCount = 0;
			
			function performSaveCheck() {
				saveCheckCount++;
				console.log('[评论采集] 保存前检查 ' + saveCheckCount + '/' + maxSaveChecks + '...');
				
				// 获取最新的评论数据
				try {
					var rootElements = document.querySelectorAll('[data-v-app], #app, [id*="app"], [class*="app"]');
					for (var i = 0; i < Math.min(rootElements.length, 3); i++) {
						var el = rootElements[i];
						var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
						if (vueInstance) {
							var componentInstance = vueInstance.component || vueInstance;
							if (componentInstance) {
								var appContext = componentInstance.appContext || 
								                 (componentInstance.ctx && componentInstance.ctx.appContext);
								
								if (appContext && appContext.config && appContext.config.globalProperties) {
									if (appContext.config.globalProperties.$pinia) {
										var pinia = appContext.config.globalProperties.$pinia;
										if (pinia.state && pinia.state._value && pinia.state._value.feed) {
											var feedStore = pinia.state._value.feed;
											
											// 安全地访问评论数据
											var finalComments = null;
											try {
												if (feedStore.commentList && feedStore.commentList.dataList && 
												    feedStore.commentList.dataList.items && 
												    Array.isArray(feedStore.commentList.dataList.items)) {
													finalComments = feedStore.commentList.dataList.items;
												}
											} catch (accessError) {
												console.error('[评论采集] 访问评论数据失败:', accessError.message);
											}
											
											if (finalComments && finalComments.length > 0) {
												var totalCommentCount = 0;
												if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
													totalCommentCount = window.__wx_channels_store__.profile.commentCount || 0;
												}
												
												// 检查评论数量是否有变化
												if (finalComments.length > lastSaveCount) {
													console.log('[评论采集] ✓ 发现新评论: ' + lastSaveCount + ' -> ' + finalComments.length);
													lastSaveCount = finalComments.length;
													
													// 如果还没达到总数，尝试再次滚动到底部触发加载
													if (totalCommentCount > 0 && finalComments.length < totalCommentCount && saveCheckCount < maxSaveChecks) {
														console.log('[评论采集] 尝试再次滚动到底部触发加载...');
														scrollToLastComment();
														setTimeout(performSaveCheck, 3000); // 等待更长时间
														return;
													}
												} else if (lastSaveCount === 0) {
													// 第一次检查，记录初始数量
													lastSaveCount = finalComments.length;
												}
												
												// 最后一次检查或已达到总数，保存评论
												if (saveCheckCount >= maxSaveChecks || (totalCommentCount > 0 && finalComments.length >= totalCommentCount)) {
													console.log('[评论采集] ✅ 加载完成，准备保存评论');
													
													// 统计实际评论数（包括二级回复）
													var actualCommentCount = finalComments.length;
													var level2Count = 0;
													for (var ci = 0; ci < finalComments.length; ci++) {
														if (finalComments[ci].levelTwoComment && Array.isArray(finalComments[ci].levelTwoComment)) {
															level2Count += finalComments[ci].levelTwoComment.length;
														}
													}
													actualCommentCount += level2Count;
													
													console.log('[评论采集] 💾 保存最终评论: ' + actualCommentCount + '/' + totalCommentCount + ' (一级:' + finalComments.length + ' + 二级:' + level2Count + ')');
													
													saveCommentData(finalComments, {
														source: 'auto_scroll_complete',
														totalCount: totalCommentCount,
														loadedCount: actualCommentCount,
														isComplete: actualCommentCount >= totalCommentCount
													});
													
													lastCommentSignature = getCommentSignature(finalComments);
													lastCommentCount = actualCommentCount;
													
													// 标记已通过自动滚动保存，停止Store监控
													isLoadingAllComments = false;
													if (commentCheckInterval) {
														clearInterval(commentCheckInterval);
														commentCheckInterval = null;
														console.log('[评论采集] ✓ 已停止Store监控（自动滚动已完成保存）');
													}
													// 清除待保存的延迟定时器
													if (pendingSaveTimer) {
														clearTimeout(pendingSaveTimer);
														pendingSaveTimer = null;
														console.log('[评论采集] ✓ 已取消待保存的定时器');
													}
													
													// 保存完成后返回页面顶部
													console.log('[评论采集] 📤 返回页面顶部');
													setTimeout(function() {
														window.scrollTo({ top: 0, behavior: 'smooth' });
														console.log('[评论采集] ✅ 评论采集完成');
													}, 500);
													
													return;
												}
												
												// 继续检查
												if (saveCheckCount < maxSaveChecks) {
													setTimeout(performSaveCheck, 2000);
												}
												break;
											} else {
												// 无法获取评论数据
												console.error('[评论采集] 无法获取评论数据，feedStore.commentList 可能不存在');
												if (saveCheckCount >= maxSaveChecks) {
													console.log('[评论采集] ⚠️ 已达最大检查次数，放弃保存');
												} else {
													setTimeout(performSaveCheck, 2000);
												}
												break;
											}
										}
									}
								}
							}
						}
					}
				} catch (e) {
					console.error('[评论采集] 保存评论检查失败:', e);
					console.error('[评论采集] 错误类型:', typeof e);
					console.error('[评论采集] 错误消息:', e.message || '(无消息)');
					console.error('[评论采集] 错误堆栈:', e.stack || '(无堆栈)');
					
					// 如果出错，尝试直接保存当前已有的评论
					if (saveCheckCount >= maxSaveChecks) {
						console.log('[评论采集] ⚠️ 检查失败但已达最大次数，尝试保存当前评论');
						// 尝试从 lastCommentCount 获取评论数
						if (lastCommentCount > 0) {
							console.log('[评论采集] 使用最后已知的评论数:', lastCommentCount);
						}
					} else {
						// 继续重试
						setTimeout(performSaveCheck, 2000);
					}
				}
			}
			
			// 延迟2秒后开始检查
			setTimeout(performSaveCheck, 2000);
		}
	}
	

	
	// 深度探测Store结构的辅助函数
	var deepFindFirstLog = true; // 标记是否是第一次找到
	function deepFindComments(obj, path, maxDepth, currentDepth) {
		if (!obj || typeof obj !== 'object' || currentDepth >= maxDepth) return null;
		
		// 检查当前对象是否包含评论数组
		var possibleArrays = ['comments', 'commentList', 'commentData', 'list', 'items', 'data', 'rootCommentList'];
		for (var i = 0; i < possibleArrays.length; i++) {
			var key = possibleArrays[i];
			if (Array.isArray(obj[key]) && obj[key].length > 0) {
				var firstItem = obj[key][0];
				// 验证是否是评论数据
				if (firstItem && typeof firstItem === 'object' && 
				    (firstItem.content || firstItem.comment || firstItem.text || 
				     firstItem.nickname || firstItem.userName || firstItem.commentId)) {
					// 只在第一次找到时输出日志
					if (deepFindFirstLog) {
						console.log('[评论采集] 🎯 在路径', path + '.' + key, '找到评论数据:', obj[key].length, '条');
						deepFindFirstLog = false;
					}
					return {data: obj[key], path: path + '.' + key};
				}
			}
		}
		
		// 递归搜索子对象
		try {
			var keys = Object.keys(obj);
			for (var i = 0; i < Math.min(keys.length, 30); i++) {
				var key = keys[i];
				if (key === '__proto__' || key === 'constructor' || key === 'prototype') continue;
				try {
					var result = deepFindComments(obj[key], path + '.' + key, maxDepth, currentDepth + 1);
					if (result) return result;
				} catch (e) {}
			}
		} catch (e) {}
		
		return null;
	}
	
	function startCommentMonitoring() {
		if (commentCheckInterval) {
			clearInterval(commentCheckInterval);
		}
		
		console.log('[评论采集] 启动评论监控（仅从Store获取）...');
		
		commentCheckInterval = setInterval(function() {
			storeCheckAttempts++;
			
			// 尝试从Store获取评论数据
			var comments = [];
			var foundStore = false;
			var storePath = '';
			
			try {
				// 第一次检查时输出全局对象信息
				if (storeCheckAttempts === 1) {
					console.log('[评论采集] 🔍 开始探测Store结构...');
					console.log('[评论采集] 检查全局对象:');
					console.log('[评论采集]   - window.__VUE_DEVTOOLS_GLOBAL_HOOK__:', !!window.__VUE_DEVTOOLS_GLOBAL_HOOK__);
					console.log('[评论采集]   - window.$pinia:', !!window.$pinia);
					console.log('[评论采集]   - window.__PINIA__:', !!window.__PINIA__);
					console.log('[评论采集]   - window.$store:', !!window.$store);
					
					// 尝试从DOM元素获取Vue实例
					var rootElements = document.querySelectorAll('[data-v-app], #app, [id*="app"], [class*="app"]');
					console.log('[评论采集]   - 找到可能的根元素:', rootElements.length);
					
					if (rootElements.length > 0) {
						var firstEl = rootElements[0];
						console.log('[评论采集]   - 第一个根元素的Vue属性:');
						console.log('[评论采集]     - __vue__:', !!firstEl.__vue__);
						console.log('[评论采集]     - __vueParentComponent:', !!firstEl.__vueParentComponent);
						console.log('[评论采集]     - _vnode:', !!firstEl._vnode);
						console.log('[评论采集]     - __vnode:', !!firstEl.__vnode);
					}
				}
				
				// 方法1: 从Vue DevTools Hook获取
				if (window.__VUE_DEVTOOLS_GLOBAL_HOOK__ && window.__VUE_DEVTOOLS_GLOBAL_HOOK__.apps) {
					var apps = window.__VUE_DEVTOOLS_GLOBAL_HOOK__.apps;
					
					if (storeCheckAttempts === 1) {
						console.log('[评论采集] ✓ 找到Vue DevTools Hook');
						console.log('[评论采集] ✓ 找到', apps.length, '个Vue应用实例');
					}
					
					for (var i = 0; i < apps.length; i++) {
						var app = apps[i];
						if (app && app.config && app.config.globalProperties) {
							// 检查Pinia
							if (app.config.globalProperties.$pinia) {
								var pinia = app.config.globalProperties.$pinia;
								
								if (storeCheckAttempts === 1) {
									console.log('[评论采集] 找到Pinia实例');
									if (pinia.state && pinia.state._value) {
										var storeKeys = Object.keys(pinia.state._value);
										console.log('[评论采集] Pinia stores:', storeKeys.join(', '));
									}
								}
								
								if (pinia.state && pinia.state._value) {
									// 遍历所有store
									for (var storeKey in pinia.state._value) {
										var store = pinia.state._value[storeKey];
										
										// 第一次检查时输出每个store的结构
										if (storeCheckAttempts === 1 && store) {
											var storeKeys = Object.keys(store);
											console.log('[评论采集] Store "' + storeKey + '" 的字段:', storeKeys.slice(0, 10).join(', '));
										}
										
										// 使用深度搜索查找评论
										if (store) {
											var result = deepFindComments(store, 'pinia.' + storeKey, 3, 0);
											if (result) {
												comments = result.data;
												storePath = result.path;
												foundStore = true;
												console.log('[评论采集] ✓ 从Pinia获取到评论:', comments.length, '条');
												console.log('[评论采集] ✓ 数据路径:', storePath);
												break;
											}
										}
									}
								}
							}
							
							// 检查Vuex
							if (!foundStore && app.config.globalProperties.$store) {
								var store = app.config.globalProperties.$store;
								
								if (storeCheckAttempts === 1) {
									console.log('[评论采集] 找到Vuex store');
									if (store.state) {
										var stateKeys = Object.keys(store.state);
										console.log('[评论采集] Vuex state模块:', stateKeys.join(', '));
									}
								}
								
								if (store.state) {
									var result = deepFindComments(store.state, 'vuex.state', 3, 0);
									if (result) {
										comments = result.data;
										storePath = result.path;
										foundStore = true;
										console.log('[评论采集] ✓ 从Vuex获取到评论:', comments.length, '条');
										console.log('[评论采集] ✓ 数据路径:', storePath);
									}
								}
							}
						}
						
						if (foundStore) break;
					}
				}
				
				// 方法2: 直接从window对象查找
				if (!foundStore && window.$pinia) {
					if (storeCheckAttempts === 1) {
						console.log('[评论采集] ✓ 从window.$pinia查找...');
					}
					
					var pinia = window.$pinia;
					if (pinia.state && pinia.state._value) {
						for (var storeKey in pinia.state._value) {
							var store = pinia.state._value[storeKey];
							if (store) {
								var result = deepFindComments(store, 'window.$pinia.' + storeKey, 3, 0);
								if (result) {
									comments = result.data;
									storePath = result.path;
									foundStore = true;
									console.log('[评论采集] ✓ 从window.$pinia获取到评论:', comments.length, '条');
									console.log('[评论采集] ✓ 数据路径:', storePath);
									break;
								}
							}
						}
					}
				}
				
				// 方法3: 从window.__PINIA__查找
				if (!foundStore && window.__PINIA__) {
					if (storeCheckAttempts === 1) {
						console.log('[评论采集] ✓ 从window.__PINIA__查找...');
					}
					
					var result = deepFindComments(window.__PINIA__, 'window.__PINIA__', 4, 0);
					if (result) {
						comments = result.data;
						storePath = result.path;
						foundStore = true;
						console.log('[评论采集] ✓ 从window.__PINIA__获取到评论:', comments.length, '条');
						console.log('[评论采集] ✓ 数据路径:', storePath);
					}
				}
				
				// 方法4: 从DOM元素的Vue实例获取
				if (!foundStore) {
					if (storeCheckAttempts === 1) {
						console.log('[评论采集] 尝试从DOM元素获取Vue实例...');
					}
					
					var rootElements = document.querySelectorAll('[data-v-app], #app, [id*="app"], [class*="app"]');
					for (var i = 0; i < Math.min(rootElements.length, 3); i++) {
						var el = rootElements[i];
						var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
						
						if (vueInstance) {
							if (storeCheckAttempts === 1) {
								console.log('[评论采集] ✓ 找到Vue实例，尝试获取store...');
							}
							
							// 尝试从Vue实例获取store
							var componentInstance = vueInstance.component || vueInstance;
							if (componentInstance) {
								// 检查appContext
								var appContext = componentInstance.appContext || 
								                (componentInstance.ctx && componentInstance.ctx.appContext);
								
								if (appContext && appContext.config && appContext.config.globalProperties) {
									if (appContext.config.globalProperties.$pinia) {
										var pinia = appContext.config.globalProperties.$pinia;
										if (pinia.state && pinia.state._value) {
											if (storeCheckAttempts === 1) {
												var storeKeys = Object.keys(pinia.state._value);
												console.log('[评论采集] ✓ 从Vue实例找到Pinia stores:', storeKeys.join(', '));
											}
											
											for (var storeKey in pinia.state._value) {
												var store = pinia.state._value[storeKey];
												if (store) {
													var result = deepFindComments(store, 'vue.pinia.' + storeKey, 3, 0);
													if (result) {
														comments = result.data;
														storePath = result.path;
														foundStore = true;
														// 只在第一次找到时输出详细信息
														if (storeCheckAttempts === 1) {
															console.log('[评论采集] ✓ 从Vue实例的Pinia获取到评论:', comments.length, '条');
															console.log('[评论采集] ✓ 数据路径:', storePath);
														}
														break;
													}
												}
											}
										}
									}
								}
							}
						}
						
						if (foundStore) break;
					}
				}
			} catch (e) {
				console.error('[评论采集] ✗ 从store获取评论失败:', e);
				if (storeCheckAttempts === 1) {
					console.error('[评论采集] 错误详情:', e.message);
					console.error('[评论采集] 错误堆栈:', e.stack);
				}
			}
			
			// 如果找到了评论数据，检查是否有变化
			if (foundStore && comments.length > 0) {
				var currentSignature = getCommentSignature(comments);
				
				// 获取总评论数（从视频信息中）
				var totalCommentCount = 0;
				if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
					totalCommentCount = window.__wx_channels_store__.profile.commentCount || 0;
				}
				
				if (currentSignature !== lastCommentSignature) {
					// 检测到变化，重置稳定计数
					stableCheckCount = 0;
					
					console.log('[评论采集] ✓ 检测到评论数据变化');
					console.log('[评论采集]   - 之前签名:', lastCommentSignature || '(无)');
					console.log('[评论采集]   - 当前签名:', currentSignature);
					console.log('[评论采集]   - 当前评论数:', comments.length);
					if (totalCommentCount > 0) {
						console.log('[评论采集]   - 总评论数:', totalCommentCount);
						console.log('[评论采集]   - 完成度:', (comments.length / totalCommentCount * 100).toFixed(1) + '%');
					}
					console.log('[评论采集]   - 示例评论:', JSON.stringify(comments[0]).substring(0, 100) + '...');
					
					lastCommentSignature = currentSignature;
					lastCommentCount = comments.length;
					
					// 第一次找到评论时，先启动自动滚动
					if (storeCheckAttempts === 1) {
						if (totalCommentCount > 0 && comments.length < totalCommentCount) {
							console.log('[评论采集] 💡 评论未完全加载: ' + comments.length + '/' + totalCommentCount);
							console.log('[评论采集] 🤖 启动自动滚动');
							startAutoScroll(totalCommentCount);
						} else if (totalCommentCount === 0) {
							console.log('[评论采集] 💡 未知总数，尝试滚动加载 (当前: ' + comments.length + ')');
							startAutoScroll(0);
						} else if (totalCommentCount > 0 && comments.length >= totalCommentCount) {
							console.log('[评论采集] ✅ 评论已完全加载: ' + comments.length + '/' + totalCommentCount);
						}
					}
					
					// 检查是否已经完成加载（如果正在自动滚动）
					if (autoScrollEnabled && totalCommentCount > 0 && comments.length >= totalCommentCount) {
						console.log('[评论采集] ✅ 检测到评论已完全加载，停止自动滚动');
						stopAutoScroll(true);
						return;
					}
					
					// 如果正在自动滚动，不要设置延迟保存（等滚动完成后再保存）
					if (autoScrollEnabled) {
						console.log('[评论采集] ⏳ 自动滚动中，等待滚动完成后保存...');
						return; // 跳过延迟保存，等自动滚动完成
					}
					
					// 清除之前的延迟保存定时器
					if (pendingSaveTimer) {
						clearTimeout(pendingSaveTimer);
					}
					
					// 延迟保存：等待6秒确保数据稳定
					console.log('[评论采集] ⏳ 等待6秒后保存...');
					pendingSaveTimer = setTimeout(function() {
						// 再次检查签名是否还是一样的
						var finalComments = [];
						var finalSignature = '';
						
						// 重新获取最新的评论数据
						try {
							var rootElements = document.querySelectorAll('[data-v-app], #app, [id*="app"], [class*="app"]');
							for (var i = 0; i < Math.min(rootElements.length, 3); i++) {
								var el = rootElements[i];
								var vueInstance = el.__vue__ || el.__vueParentComponent || el._vnode || el.__vnode;
								
								if (vueInstance) {
									var componentInstance = vueInstance.component || vueInstance;
									if (componentInstance) {
										var appContext = componentInstance.appContext || 
										                (componentInstance.ctx && componentInstance.ctx.appContext);
										
										if (appContext && appContext.config && appContext.config.globalProperties) {
											if (appContext.config.globalProperties.$pinia) {
												var pinia = appContext.config.globalProperties.$pinia;
												if (pinia.state && pinia.state._value && pinia.state._value.feed) {
													var feedStore = pinia.state._value.feed;
													if (feedStore.commentList && feedStore.commentList.dataList && 
													    feedStore.commentList.dataList.items) {
														finalComments = feedStore.commentList.dataList.items;
														finalSignature = getCommentSignature(finalComments);
														break;
													}
												}
											}
										}
									}
								}
							}
						} catch (e) {
							console.error('[评论采集] 获取最新评论数据失败:', e);
						}
						
						if (finalComments.length > 0) {
							console.log('[评论采集] ✓ 数据已稳定，最终评论数:', finalComments.length);
							console.log('[评论采集] 💾 开始保存...');
							
							// 保存最终的评论数据
							saveCommentData(finalComments, {
								source: 'store_monitor', 
								path: storePath,
								totalCount: totalCommentCount,
								loadedCount: finalComments.length,
								isComplete: finalComments.length >= totalCommentCount
							});
							
							// 更新签名
							lastCommentSignature = finalSignature;
							lastCommentCount = finalComments.length;
						}
						
						pendingSaveTimer = null;
					}, 6000); // 6秒延迟
				} else {
					// 签名没有变化，增加稳定计数
					stableCheckCount++;
					
					if (storeCheckAttempts === 2) {
						// 第二次检查时，如果数据没变化，说明监控正常工作
						if (totalCommentCount > 0 && comments.length < totalCommentCount) {
							console.log('[评论采集] ✓ 监控正常，已加载', comments.length, '/', totalCommentCount, '条评论');
						} else {
							console.log('[评论采集] ✓ 监控正常，等待评论变化...');
						}
					}
					
					// 如果数据已经稳定5次检查（15秒），且有待保存的数据，立即保存
					if (stableCheckCount >= 5 && pendingSaveTimer) {
						console.log('[评论采集] ✓ 数据已稳定15秒，立即保存');
						clearTimeout(pendingSaveTimer);
						pendingSaveTimer = null;
						
						saveCommentData(comments, {
							source: 'store_monitor', 
							path: storePath,
							totalCount: totalCommentCount,
							loadedCount: comments.length,
							isComplete: comments.length >= totalCommentCount
						});
						
						stableCheckCount = 0;
					}
				}
			} else if (storeCheckAttempts <= 5) {
				// 前5次尝试时输出调试信息
				console.log('[评论采集] 第', storeCheckAttempts, '次检查，未找到评论Store');
			}
			
			// 如果超过最大尝试次数且没有找到Store，降低检查频率
			if (storeCheckAttempts > maxStoreCheckAttempts && !foundStore) {
				console.log('[评论采集] 已尝试', maxStoreCheckAttempts, '次，未找到评论Store，降低检查频率');
				clearInterval(commentCheckInterval);
				// 改为每30秒检查一次
				commentCheckInterval = setInterval(arguments.callee, 30000);
				storeCheckAttempts = 0; // 重置计数器
			}
		}, 3000); // 每3秒检查一次
	}
	
	// 暴露手动启动评论采集的函数
	window.__wx_channels_start_comment_collection = function() {
		if (window.location.pathname.includes('/pages/feed')) {
			console.log('[评论采集] 🚀 手动启动评论采集');
			startCommentMonitoring();
		} else {
			console.log('[评论采集] ⚠️ 当前不是Feed页面，无法采集评论');
		}
	};
	
	console.log('[评论采集] 评论采集系统初始化完成（手动模式）');
	console.log('[评论采集] 💡 评论按钮将与下载按钮一起显示');
})();
</script>`
}

// getLogPanelScript 获取日志面板脚本，用于在页面上显示日志（替代控制台）
func (h *ScriptHandler) getLogPanelScript() string {
	// 根据配置决定是否显示日志按钮
	showLogButton := "false"
	if h.config.ShowLogButton {
		showLogButton = "true"
	}
	
	return `<script>
// 日志按钮显示配置
window.__wx_channels_show_log_button__ = ` + showLogButton + `;
</script>
<script>
(function() {
	'use strict';
	
	// 防止重复初始化
	if (window.__wx_channels_log_panel_initialized__) {
		return;
	}
	window.__wx_channels_log_panel_initialized__ = true;
	
	// 日志存储
	const logStore = {
		logs: [],
		maxLogs: 500, // 最多保存500条日志
		addLog: function(level, args) {
			const timestamp = new Date().toLocaleTimeString('zh-CN', { hour12: false });
			const message = Array.from(args).map(arg => {
				if (typeof arg === 'object') {
					try {
						return JSON.stringify(arg, null, 2);
					} catch (e) {
						return String(arg);
					}
				}
				return String(arg);
			}).join(' ');
			
			this.logs.push({
				level: level,
				message: message,
				timestamp: timestamp
			});
			
			// 限制日志数量
			if (this.logs.length > this.maxLogs) {
				this.logs.shift();
			}
			
			// 更新面板显示
			if (window.__wx_channels_log_panel) {
				window.__wx_channels_log_panel.updateDisplay();
			}
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
		
		// 更新显示
		function updateDisplay() {
			content.innerHTML = '';
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
				
				logItem.innerHTML = '<span style="color: #888; font-size: 10px;">[' + log.timestamp + ']</span>' +
					'<span style="color: ' + levelColor + '; font-weight: bold; margin: 0 4px;">' + levelPrefix + '</span>' +
					'<span style="color: #fff;">' + escapeHtml(log.message) + '</span>';
				
				content.appendChild(logItem);
			});
			
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
	
	// 重写console方法
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
	if h.config != nil && !h.config.SavePageJS {
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
	jsDir := filepath.Join(baseDir, h.config.DownloadsDir, "cached_js", pageType)
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
