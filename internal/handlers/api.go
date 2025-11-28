package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/utils"
	"wx_channel/pkg/util"

	"github.com/fatih/color"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// APIHandler API请求处理器
type APIHandler struct {
	config     *config.Config
	currentURL string
}

// NewAPIHandler 创建API处理器
func NewAPIHandler(cfg *config.Config) *APIHandler {
	return &APIHandler{
		config: cfg,
	}
}

// SetCurrentURL 设置当前页面URL
func (h *APIHandler) SetCurrentURL(url string) {
	h.currentURL = url
}

// GetCurrentURL 获取当前页面URL
func (h *APIHandler) GetCurrentURL() string {
	return h.currentURL
}

// HandleProfile 处理视频信息请求
func (h *APIHandler) HandleProfile(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/profile" {
		return false
	}

    // 授权与来源校验（可选）
    if h.config != nil && h.config.SecretToken != "" {
        if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
            headers := http.Header{}
            headers.Set("Content-Type", "application/json")
            headers.Set("X-Content-Type-Options", "nosniff")
            Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
            return true
        }
    }
    if h.config != nil && len(h.config.AllowedOrigins) > 0 {
        origin := Conn.Request.Header.Get("Origin")
        if origin != "" {
            allowed := false
            for _, o := range h.config.AllowedOrigins {
                if o == origin {
                    allowed = true
                    break
                }
            }
            if !allowed {
                headers := http.Header{}
                headers.Set("Content-Type", "application/json")
                headers.Set("X-Content-Type-Options", "nosniff")
                Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
                return true
            }
        }
    }

	var data map[string]interface{}
	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取profile请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		utils.HandleError(err, "解析profile JSON数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 处理视频数据
	h.processVideoData(data)

	// 返回空响应
	h.sendEmptyResponse(Conn)
	return true
}

// processVideoData 处理视频数据并显示
func (h *APIHandler) processVideoData(data map[string]interface{}) {
	// 打印提醒
	utils.Info("💡 [提醒] 视频已成功播放")
	utils.Info("💡 [提醒] 可以在「更多」菜单中下载视频啦！")
	
	// 记录视频信息到日志文件
	videoID := ""
	if id, ok := data["id"].(string); ok {
		videoID = id
	}
	title := ""
	if t, ok := data["title"].(string); ok {
		title = t
	}
	author := ""
	if n, ok := data["nickname"].(string); ok {
		author = n
	}
	sizeMB := 0.0
	if size, ok := data["size"].(float64); ok {
		sizeMB = size / (1024 * 1024)
	}
	url := ""
	if u, ok := data["url"].(string); ok {
		url = u
	}
	
	utils.LogInfo("[视频信息] ID=%s | 标题=%s | 作者=%s | 大小=%.2fMB | URL=%s",
		videoID, title, author, sizeMB, url)
	
	color.Yellow("\n")

	// 打印视频详细信息
	utils.PrintSeparator()
	color.Blue("📊 视频详细信息")
	utils.PrintSeparator()

	if nickname, ok := data["nickname"].(string); ok {
		utils.PrintLabelValue("👤", "视频号名称", nickname)
	}
	if title, ok := data["title"].(string); ok {
		utils.PrintLabelValue("📝", "视频标题", title)
	}

	if duration, ok := data["duration"].(float64); ok {
		utils.PrintLabelValue("⏱️", "视频时长", utils.FormatDuration(duration))
	}
	if size, ok := data["size"].(float64); ok {
		sizeMB := size / (1024 * 1024)
		utils.PrintLabelValue("📦", "视频大小", fmt.Sprintf("%.2f MB", sizeMB))
	}

	// 添加互动数据显示（显示所有数据，包括0）
	if likeCount, ok := data["likeCount"].(float64); ok {
		utils.PrintLabelValue("👍", "点赞量", utils.FormatNumber(likeCount))
	}
	if commentCount, ok := data["commentCount"].(float64); ok {
		utils.PrintLabelValue("💬", "评论量", utils.FormatNumber(commentCount))
	}
	if favCount, ok := data["favCount"].(float64); ok {
		utils.PrintLabelValue("🔖", "收藏数", utils.FormatNumber(favCount))
	}
	if forwardCount, ok := data["forwardCount"].(float64); ok {
		utils.PrintLabelValue("🔄", "转发数", utils.FormatNumber(forwardCount))
	}

	// 添加创建时间
	if createtime, ok := data["createtime"].(float64); ok {
		t := time.Unix(int64(createtime), 0)
		utils.PrintLabelValue("📅", "创建时间", t.Format("2006-01-02 15:04:05"))
	}

	// 添加IP所在地（从多个来源获取）
	locationFound := false

	// 方法1：从 ipRegionInfo 获取
	if ipRegionInfo, ok := data["ipRegionInfo"].(map[string]interface{}); ok {
		if regionText, ok := ipRegionInfo["regionText"].(string); ok && regionText != "" {
			utils.PrintLabelValue("🌍", "IP所在地", regionText)
			locationFound = true
		}
	}

	// 方法2：从 contact.extInfo 获取
	if !locationFound {
		if contact, ok := data["contact"].(map[string]interface{}); ok {
			if extInfo, ok := contact["extInfo"].(map[string]interface{}); ok {
				var location string
				if province, ok := extInfo["province"].(string); ok && province != "" {
					location = province
					if city, ok := extInfo["city"].(string); ok && city != "" {
						location += " " + city
					}
					utils.PrintLabelValue("🌍", "地理位置", location)
					locationFound = true
				}
			}
		}
	}

	if fileFormat, ok := data["fileFormat"].([]interface{}); ok && len(fileFormat) > 0 {
		utils.PrintLabelValue("🎞️", "视频格式", fileFormat)
	}
	if coverUrl, ok := data["coverUrl"].(string); ok {
		utils.PrintLabelValue("🖼️", "视频封面", coverUrl)
	}
	if url, ok := data["url"].(string); ok {
		utils.PrintLabelValue("🔗", "原始链接", url)
	}
	utils.PrintSeparator()
	color.Yellow("\n\n")
}

// HandleTip 处理前端提示请求
func (h *APIHandler) HandleTip(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/tip" {
		return false
	}

    if h.config != nil && h.config.SecretToken != "" {
        if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
            headers := http.Header{}
            headers.Set("Content-Type", "application/json")
            headers.Set("X-Content-Type-Options", "nosniff")
            Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
            return true
        }
    }
    if h.config != nil && len(h.config.AllowedOrigins) > 0 {
        origin := Conn.Request.Header.Get("Origin")
        if origin != "" {
            allowed := false
            for _, o := range h.config.AllowedOrigins {
                if o == origin {
                    allowed = true
                    break
                }
            }
            if !allowed {
                headers := http.Header{}
                headers.Set("Content-Type", "application/json")
                headers.Set("X-Content-Type-Options", "nosniff")
                Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
                return true
            }
        }
    }

	var data struct {
		Msg string `json:"msg"`
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取tip请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	// 检查body是否为空
	if len(body) == 0 {
		utils.Warn("tip请求体为空，跳过处理")
		h.sendEmptyResponse(Conn)
		return true
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		utils.HandleError(err, "解析tip JSON数据")
		// 即使JSON解析失败，也返回空响应，避免重复处理
		h.sendEmptyResponse(Conn)
		return true
	}

	utils.PrintLabelValue("💡", "[提醒]", data.Msg)
	
	// 记录关键操作到日志文件
	msg := data.Msg
	if strings.Contains(msg, "下载封面") {
		// 提取封面URL
		lines := strings.Split(msg, "\n")
		if len(lines) > 1 {
			coverURL := lines[1]
			utils.LogInfo("[下载封面] URL=%s", coverURL)
		}
	} else if strings.Contains(msg, "下载文件名") {
		// 提取文件名，判断是否为不同格式
		filename := strings.TrimPrefix(msg, "下载文件名<")
		filename = strings.TrimSuffix(filename, ">")
		
		// 检查是否包含格式标识（如 xWT111_1280x720）
		if strings.Contains(filename, "xWT") || strings.Contains(filename, "_") {
			parts := strings.Split(filename, "_")
			if len(parts) > 1 {
				format := parts[len(parts)-2] // 格式标识
				resolution := ""
				if len(parts) > 2 {
					resolution = parts[len(parts)-1] // 分辨率
				}
				utils.LogInfo("[格式下载] 文件名=%s | 格式=%s | 分辨率=%s", filename, format, resolution)
			} else {
				utils.LogInfo("[视频下载] 文件名=%s", filename)
			}
		} else {
			utils.LogInfo("[视频下载] 文件名=%s", filename)
		}
	} else if strings.Contains(msg, "视频链接") {
		// 提取视频链接
		videoURL := strings.TrimPrefix(msg, "视频链接<")
		videoURL = strings.TrimSuffix(videoURL, ">")
		utils.LogInfo("[视频链接] URL=%s", videoURL)
	} else if strings.Contains(msg, "页面链接") {
		// 提取页面链接
		pageURL := strings.TrimPrefix(msg, "页面链接<")
		pageURL = strings.TrimSuffix(pageURL, ">")
		utils.LogInfo("[页面链接] URL=%s", pageURL)
	} else if strings.Contains(msg, "搜索页面已加载") {
		// 记录搜索页面加载
		utils.LogInfo("[搜索页面] 页面已加载")
	} else if strings.Contains(msg, "搜索关键词:") {
		// 提取搜索关键词
		keyword := strings.TrimPrefix(msg, "搜索关键词: ")
		keyword = strings.TrimSpace(keyword)
		utils.LogInfo("[搜索关键词] 关键词=%s", keyword)
	} else if strings.Contains(msg, "导出动态:") {
		// 提取导出信息
		// 格式: "导出动态: 格式=JSON, 视频数=10"
		parts := strings.Split(msg, ",")
		format := ""
		count := ""
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.Contains(part, "格式=") {
				format = strings.TrimPrefix(part, "格式=")
				format = strings.TrimPrefix(format, "导出动态: 格式=")
			} else if strings.Contains(part, "视频数=") {
				count = strings.TrimPrefix(part, "视频数=")
			}
		}
		utils.LogInfo("[导出动态] 格式=%s | 视频数=%s", format, count)
	} else if strings.Contains(msg, "[Profile自动下载]") {
		// Profile 页面批量下载日志
		if strings.Contains(msg, "开始自动下载") {
			// 提取视频数量
			// 格式: "🚀 [Profile自动下载] 开始自动下载 10 个视频"
			parts := strings.Split(msg, " ")
			for i, part := range parts {
				if part == "个视频" && i > 0 {
					count := parts[i-1]
					utils.LogInfo("[Profile批量下载] 开始 | 视频数=%s", count)
					break
				}
			}
		} else if strings.Contains(msg, "完成") {
			// 提取统计信息
			// 格式: "✅ [Profile自动下载] 完成！共处理 10 个视频，成功 8 个，失败 2 个"
			var total, success, failed string
			parts := strings.Split(msg, " ")
			for i, part := range parts {
				if part == "个视频，成功" && i > 0 {
					total = parts[i-1]
				} else if part == "个，失败" && i > 0 {
					success = parts[i-1]
				} else if part == "个" && i > 0 && strings.Contains(parts[i-1], "失败") {
					// 已经在上面处理了
				} else if strings.HasSuffix(part, "个") && i > 0 && success != "" {
					failed = strings.TrimSuffix(part, "个")
				}
			}
			if total != "" {
				utils.LogInfo("[Profile批量下载] 完成 | 总数=%s | 成功=%s | 失败=%s", total, success, failed)
			}
		} else if strings.Contains(msg, "进度:") {
			// 进度日志
			// 格式: "📥 [Profile自动下载] 进度: 5/10"
			progress := strings.TrimSpace(strings.Split(msg, "进度:")[1])
			utils.LogInfo("[Profile批量下载] 进度=%s", progress)
		}
	} else if strings.Contains(msg, "Profile视频采集:") {
		// Profile 页面视频采集日志
		// 格式: "Profile视频采集: 采集到 10 个视频"
		parts := strings.Split(msg, " ")
		for i, part := range parts {
			if part == "个视频" && i > 0 {
				count := parts[i-1]
				utils.LogInfo("[Profile视频采集] 采集数=%s", count)
				break
			}
		}
	}
	
	h.sendEmptyResponse(Conn)
	return true
}

// HandlePageURL 处理页面URL请求
func (h *APIHandler) HandlePageURL(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/page_url" {
		return false
	}

    if h.config != nil && h.config.SecretToken != "" {
        if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
            headers := http.Header{}
            headers.Set("Content-Type", "application/json")
            headers.Set("X-Content-Type-Options", "nosniff")
            Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
            return true
        }
    }
    if h.config != nil && len(h.config.AllowedOrigins) > 0 {
        origin := Conn.Request.Header.Get("Origin")
        if origin != "" {
            allowed := false
            for _, o := range h.config.AllowedOrigins {
                if o == origin {
                    allowed = true
                    break
                }
            }
            if !allowed {
                headers := http.Header{}
                headers.Set("Content-Type", "application/json")
                headers.Set("X-Content-Type-Options", "nosniff")
                Conn.StopRequest(403, `{"success":false,"error":"forbidden_origin"}`, headers)
                return true
            }
        }
    }

	var urlData struct {
		URL string `json:"url"`
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取page_url请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	err = json.Unmarshal(body, &urlData)
	if err != nil {
		utils.HandleError(err, "解析page_url JSON数据")
		h.sendErrorResponse(Conn, err)
		return true
	}

	h.currentURL = urlData.URL

	// 显示页面链接
	utils.PrintSeparator()
	color.Blue("📋 页面完整链接")
	utils.PrintSeparator()
	utils.PrintLabelValue("🔗", "分享链接", h.currentURL)
	utils.PrintSeparator()
	fmt.Println()
	fmt.Println()

	h.sendEmptyResponse(Conn)
	return true
}

// HandleStaticFiles 处理静态文件请求（jszip, FileSaver等）
func HandleStaticFiles(Conn *SunnyNet.HttpConn, zipJS, fileSaverJS []byte) bool {
	path := Conn.Request.URL.Path

	if util.Includes(path, "jszip") {
		headers := http.Header{}
		headers.Set("Content-Type", "application/javascript")
		headers.Set("__debug", "local_file")
		Conn.StopRequest(200, zipJS, headers)
		return true
	}

	if util.Includes(path, "FileSaver.min") {
		headers := http.Header{}
		headers.Set("Content-Type", "application/javascript")
		headers.Set("__debug", "local_file")
		Conn.StopRequest(200, fileSaverJS, headers)
		return true
	}

	return false
}

// sendEmptyResponse 发送空JSON响应
func (h *APIHandler) sendEmptyResponse(Conn *SunnyNet.HttpConn) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
    headers.Set("X-Content-Type-Options", "nosniff")
    // CORS
    if h.config != nil && len(h.config.AllowedOrigins) > 0 {
        origin := Conn.Request.Header.Get("Origin")
        if origin != "" {
            for _, o := range h.config.AllowedOrigins {
                if o == origin {
                    headers.Set("Access-Control-Allow-Origin", origin)
                    headers.Set("Vary", "Origin")
                    headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
                    headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
                    break
                }
            }
        }
    }
	headers.Set("__debug", "fake_resp")
	Conn.StopRequest(200, "{}", headers)
}

// sendErrorResponse 发送错误响应
func (h *APIHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
    headers.Set("X-Content-Type-Options", "nosniff")
    if h.config != nil && len(h.config.AllowedOrigins) > 0 {
        origin := Conn.Request.Header.Get("Origin")
        if origin != "" {
            for _, o := range h.config.AllowedOrigins {
                if o == origin {
                    headers.Set("Access-Control-Allow-Origin", origin)
                    headers.Set("Vary", "Origin")
                    headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
                    headers.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
                    break
                }
            }
        }
    }
	errorMsg := fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error())
	Conn.StopRequest(500, errorMsg, headers)
}
