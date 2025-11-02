package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	h.sendEmptyResponse(Conn)
	return true
}

// HandlePageURL 处理页面URL请求
func (h *APIHandler) HandlePageURL(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/page_url" {
		return false
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
	headers.Set("__debug", "fake_resp")
	Conn.StopRequest(200, "{}", headers)
}

// sendErrorResponse 发送错误响应
func (h *APIHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	errorMsg := fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error())
	Conn.StopRequest(500, errorMsg, headers)
}
