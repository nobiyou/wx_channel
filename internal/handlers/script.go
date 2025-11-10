package handlers

import (
	"bytes"
	"fmt"
	"io"
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

	if host == "channels.weixin.qq.com" && (path == "/web/pages/feed" || path == "/web/pages/home" || path == "/web/pages/profile") {
		// 注入所有脚本
		injectedScripts := h.buildInjectedScripts()
		html = strings.Replace(html, "<head>", "<head>\n"+injectedScripts, 1)
		utils.Info("页面已成功加载！")
		utils.Info("已添加视频缓存监控和提醒功能")
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

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return true
}

// buildInjectedScripts 构建所有需要注入的脚本
func (h *ScriptHandler) buildInjectedScripts() string {
	// 主脚本
	script := fmt.Sprintf(`<script>%s</script>`, string(h.mainJS))

	// 预加载FileSaver.js库
	preloadScript := h.getPreloadScript()

	// 下载记录功能
	downloadTrackerScript := h.getDownloadTrackerScript()

	// 捕获URL脚本
	captureUrlScript := h.getCaptureUrlScript()

	// 保存页面内容脚本
	savePageContentScript := h.getSavePageContentScript()

	// 视频缓存监控脚本
	videoCacheNotificationScript := h.getVideoCacheNotificationScript()

	return script + preloadScript + downloadTrackerScript + captureUrlScript + savePageContentScript + videoCacheNotificationScript
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
	
	// 覆盖原有的下载处理函数
	const originalHandleClick = window.__wx_channels_handle_click_download__;
	if (originalHandleClick) {
		window.__wx_channels_handle_click_download__ = function(sp) {
			// 调用原始函数进行下载
			originalHandleClick(sp);
			
			// 记录下载
			if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
				const profile = {...window.__wx_channels_store__.profile};
				window.__wx_channels_record_download(profile);
			}
		};
	}
	
	// 覆盖当前视频下载函数
	const originalDownloadCur = window.__wx_channels_download_cur__;
	if (originalDownloadCur) {
		window.__wx_channels_download_cur__ = function() {
			// 调用原始函数进行下载
			originalDownloadCur();
			
			// 记录下载
			if (window.__wx_channels_store__ && window.__wx_channels_store__.profile) {
				const profile = {...window.__wx_channels_store__.profile};
				window.__wx_channels_record_download(profile);
			}
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
			
			// 记录下载
			const recordProfile = {...profile};
			window.__wx_channels_record_download(recordProfile);
			
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
			// URL变化后延迟保存，等待内容加载
			setTimeout(() => {
				window.__wx_channels_save_page_content();
			}, 3000);
		}
	};
	
	// 定期检查URL变化（适用于SPA）
	setInterval(checkUrlChange, 1000);
	
	// 监听历史记录变化
	window.addEventListener('popstate', () => {
		setTimeout(() => {
			window.__wx_channels_save_page_content();
		}, 3000);
	});
	
	// 在页面加载完成后也保存一次
	setTimeout(() => {
		window.__wx_channels_save_page_content();
	}, 5000);
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
			const checkBufferedProgress = () => {
				// 优先检查Video.js播放器
				const vjsPlayer = document.querySelector('.video-js');
				let video = null;
				
				if (vjsPlayer) {
					// 从Video.js播放器中获取video元素
					video = vjsPlayer.querySelector('video');
					console.log('找到Video.js播放器，开始监控');
				} else {
					// 回退到查找普通video元素
					const videoElements = document.querySelectorAll('video');
					if (videoElements.length > 0) {
						video = videoElements[0];
						console.log('使用普通video元素监控');
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
						
						// 检查Video.js播放器的就绪状态
						if (vjsPlayer && typeof vjsPlayer.readyState !== 'undefined') {
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
			
			// 每秒更新进度
			const updateInterval = setInterval(() => {
				if (!this.isBuffering) {
					clearInterval(updateInterval);
					indicator.remove();
					return;
				}
				
				let progress = 0;
				if (this.videoSize > 0) {
					progress = (this.totalBufferSize / this.videoSize) * 100;
				} else {
					// 优先使用Video.js播放器
					const vjsPlayer = document.querySelector('.video-js');
					let video = null;
					
					if (vjsPlayer) {
						video = vjsPlayer.querySelector('video');
					} else {
						const videoElements = document.querySelectorAll('video');
						if (videoElements.length > 0) {
							video = videoElements[0];
						}
					}
					
					if (video && video.duration && video.buffered.length > 0) {
						const bufferedEnd = video.buffered.end(video.buffered.length - 1);
						progress = (bufferedEnd / video.duration) * 100;
					}
				}
				
				// 更新指示器
				if (progress > 0) {
					indicator.innerHTML = '<div>🔄 视频缓存中: ' + progress.toFixed(1) + '%</div>';
				} else {
					indicator.innerHTML = '<div>⏳ 等待视频数据...</div>';
				}
				
				// 根据进度改变样式
				if (progress >= 95) {
					indicator.style.backgroundColor = 'rgba(0,128,0,0.8)';
					indicator.innerHTML = '<div>✅ 视频缓存接近完成: ' + progress.toFixed(1) + '%</div>';
				} else if (progress >= 50) {
					indicator.style.backgroundColor = 'rgba(255,165,0,0.8)';
				} else {
					indicator.style.backgroundColor = 'rgba(0,0,0,0.8)';
				}
				
				// 输出调试信息
				if (progress > 0) {
					console.log('缓存进度更新:', progress.toFixed(1) + '%');
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
				
				// 检查Video.js播放器特有的状态
				if (vjsPlayer.classList.contains('vjs-has-started')) {
					console.log('Video.js播放器已开始播放');
				}
				
				if (vjsPlayer.classList.contains('vjs-waiting')) {
					console.log('Video.js播放器正在等待数据');
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
					
					// 如果预加载接近完成，触发完成检测
					if (bufferedPercent >= 95) {
						console.log('检测到视频预加载接近完成 (' + bufferedPercent.toFixed(1) + '%)');
						this.checkCompletion(true);
					}
				}
				
				// 只在readyState为4且缓冲百分比较高时才认为完成
				if (video.readyState >= 4 && video.buffered && video.buffered.length > 0 && video.duration) {
					const bufferedEnd = video.buffered.end(video.buffered.length - 1);
					const bufferedPercent = (bufferedEnd / video.duration) * 100;
					if (bufferedPercent >= 98) {
						console.log('视频readyState为4且缓冲98%以上，标记为缓存完成');
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
	}
	content = regexp1.ReplaceAllString(content, replaceStr1)
	regexp2 := regexp.MustCompile(`if\(f.cmd===re.MAIN_THREAD_CMD.AUTO_CUT`)
	replaceStr2 := `if(f.cmd==="CUT"){
	if (window.__wx_channels_store__) {
	console.log("CUT", f, __wx_channels_store__.profile.key);
	window.__wx_channels_store__.keys[__wx_channels_store__.profile.key]=f.decryptor_array;
	}
}
if(f.cmd===re.MAIN_THREAD_CMD.AUTO_CUT`
	content = regexp2.ReplaceAllString(content, replaceStr2)
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
		
		// Profile页面视频列表数据采集
		if (profileResult && profileResult.data && profileResult.data.object) {
			var videoCount = profileResult.data.object.length;
			console.log('[主页数据采集] 获取到视频列表，数量:', videoCount);
			
			// 发送日志到后端终端
			fetch('/__wx_channels_api/tip', {
				method: 'POST',
				headers: {'Content-Type': 'application/json'},
				body: JSON.stringify({msg: '📊 [主页数据采集] 获取到视频列表，数量: ' + videoCount})
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
						var logMsg = '[主页采集] 视频' + (index+1) + ': ' + profile.title.substring(0, 30) + '...';
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
							body: JSON.stringify({msg: '✅ [主页采集] 完成！共采集 ' + profileResult.data.object.length + ' 个视频'})
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
	})(),f("div",{class:"context-item",role:"button",onClick:()=>__wx_channels_handle_click_download__()},"原始视频"),f("div",{class:"context-item",role:"button",onClick:__wx_channels_download_cur__},"当前视频"),f("div",{class:"context-item",role:"button",onClick:()=>__wx_channels_handle_download_cover()},"下载封面")]`
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

	// 策略1：拦截 goToNextFlowFeed (下一个视频)
	callNextRegex := regexp.MustCompile(`(\w)\.goToNextFlowFeed\(\{goBackWhenEnd:[^,]+,eleInfo:\{[^}]+\}[^)]*\}\)`)
	// 策略2：拦截 goToPrevFlowFeed (上一个视频)
	callPrevRegex := regexp.MustCompile(`(\w)\.goToPrevFlowFeed\(\{eleInfo:\{[^}]+\}\}\)`)

	// 数据采集代码（通用，包含互动数据）
	captureCode := `setTimeout(function(){try{var __tab=Ue.value;if(__tab&&__tab.currentFeed){var __feed=__tab.currentFeed;if(__feed.objectDesc){var __media=__feed.objectDesc.media[0];var __duration=0;if(__media&&__media.spec&&__media.spec[0]&&__media.spec[0].durationMs){__duration=__media.spec[0].durationMs;}var __profile={type:"media",duration:__duration,spec:__media.spec.map(function(s){return{width:s.width||s.videoWidth,height:s.height||s.videoHeight,bitrate:s.bitrate,fileFormat:s.fileFormat}}),title:__feed.objectDesc.description,coverUrl:__media.thumbUrl,url:__media.url+__media.urlToken,size:__media.fileSize,key:__media.decodeKey,id:__feed.id,nonce_id:__feed.objectNonceId,nickname:__feed.nickname,createtime:__feed.createtime,fileFormat:__media.spec.map(function(o){return o.fileFormat}),contact:__feed.contact,readCount:__feed.readCount,likeCount:__feed.likeCount,commentCount:__feed.commentCount,favCount:__feed.favCount,forwardCount:__feed.forwardCount,ipRegionInfo:__feed.ipRegionInfo};fetch("/__wx_channels_api/profile",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(__profile)});window.__wx_channels_store__=window.__wx_channels_store__||{profile:null,buffers:[],keys:{}};window.__wx_channels_store__.profile=__profile;console.log("[Home页面] 视频数据采集成功:",__profile.title,"时长:",__duration)}}}catch(__e){console.error("[Home] 采集失败:",__e)}},500)`

	// 替换 goToNextFlowFeed
	if callNextRegex.MatchString(content) {
		replaceNext := `$1.goToNextFlowFeed({goBackWhenEnd:f.goBackWhenEnd,eleInfo:{type:f.source,tagId:Ct.value},ignoreCoolDown:f.ignoreCoolDown});` + captureCode
		content = callNextRegex.ReplaceAllString(content, replaceNext)
	}

	// 替换 goToPrevFlowFeed
	if callPrevRegex.MatchString(content) {
		replacePrev := `$1.goToPrevFlowFeed({eleInfo:{type:f.source,tagId:Ct.value}});` + captureCode
		content = callPrevRegex.ReplaceAllString(content, replacePrev)
	}

	// 禁用浏览器缓存，确保每次都能拦截到最新的代码
	Conn.Response.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	Conn.Response.Header.Set("Pragma", "no-cache")
	Conn.Response.Header.Set("Expires", "0")

	Conn.Response.Body = io.NopCloser(bytes.NewBuffer([]byte(content)))
	return content, true
}
