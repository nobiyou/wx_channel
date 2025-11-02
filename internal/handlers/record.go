package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"wx_channel/internal/config"
	"wx_channel/internal/models"
	"wx_channel/internal/storage"
	"wx_channel/internal/utils"
	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// RecordHandler 下载记录处理器
type RecordHandler struct {
	config     *config.Config
	csvManager *storage.CSVManager
	currentURL string
}

// NewRecordHandler 创建记录处理器
func NewRecordHandler(cfg *config.Config, csvManager *storage.CSVManager) *RecordHandler {
	return &RecordHandler{
		config:     cfg,
		csvManager: csvManager,
	}
}

// SetCurrentURL 设置当前页面URL
func (h *RecordHandler) SetCurrentURL(url string) {
	h.currentURL = url
}

// GetCurrentURL 获取当前页面URL
func (h *RecordHandler) GetCurrentURL() string {
	return h.currentURL
}

// HandleRecordDownload 处理记录下载信息请求
func (h *RecordHandler) HandleRecordDownload(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/record_download" {
		return false
	}

	var data map[string]interface{}
	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取record_download请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	// 检查body是否为空
	if len(body) == 0 {
		utils.Warn("record_download请求体为空，跳过处理")
		h.sendEmptyResponse(Conn)
		return true
	}

	if err := json.Unmarshal(body, &data); err != nil {
		utils.HandleError(err, "记录下载信息")
		h.sendEmptyResponse(Conn)
		return true
	}

	// 创建下载记录
	record := &models.VideoDownloadRecord{
		ID:         fmt.Sprintf("%v", data["id"]),
		Title:      fmt.Sprintf("%v", data["title"]),
		Author:     "", // 将在后面从contact中获取
		URL:        fmt.Sprintf("%v", data["url"]),
		PageURL:    h.currentURL,
		DownloadAt: time.Now(),
	}

	// 从正确的位置获取作者昵称
	// 优先从顶层获取（Feed页）
	if nickname, ok := data["nickname"].(string); ok && nickname != "" {
		record.Author = nickname
	} else {
		// 从 contact.nickname 获取（Home页）
		if contact, ok := data["contact"].(map[string]interface{}); ok {
			if nickname, ok := contact["nickname"].(string); ok {
				record.Author = nickname
			}
		}
	}

	// 添加可选字段
	if size, ok := data["size"].(float64); ok {
		record.FileSize = fmt.Sprintf("%.2f MB", size/(1024*1024))
	}
	if duration, ok := data["duration"].(float64); ok {
		record.Duration = utils.FormatDuration(duration)
	}

	// 添加互动数据
	if readCount, ok := data["readCount"].(float64); ok {
		record.PlayCount = utils.FormatNumber(readCount)
	}
	if likeCount, ok := data["likeCount"].(float64); ok {
		record.LikeCount = utils.FormatNumber(likeCount)
	}
	if commentCount, ok := data["commentCount"].(float64); ok {
		record.CommentCount = utils.FormatNumber(commentCount)
	}
	if favCount, ok := data["favCount"].(float64); ok {
		record.FavCount = utils.FormatNumber(favCount)
	}
	if forwardCount, ok := data["forwardCount"].(float64); ok {
		record.ForwardCount = utils.FormatNumber(forwardCount)
	}

	// 添加创建时间
	if createtime, ok := data["createtime"].(float64); ok {
		t := time.Unix(int64(createtime), 0)
		record.CreateTime = t.Format("2006-01-02 15:04:05")
	}

	// 添加视频号分类和公众号名称
	if contact, ok := data["contact"].(map[string]interface{}); ok {
		if authInfo, ok := contact["authInfo"].(map[string]interface{}); ok {
			if authProfession, ok := authInfo["authProfession"].(string); ok {
				record.AuthorType = authProfession
			}
		}

		// 尝试获取公众号名称
		if bindInfo, ok := contact["bindInfo"].([]interface{}); ok && len(bindInfo) > 0 {
			for _, bind := range bindInfo {
				if bindMap, ok := bind.(map[string]interface{}); ok {
					if bizInfo, ok := bindMap["bizInfo"].(map[string]interface{}); ok {
						if info, ok := bizInfo["info"].([]interface{}); ok && len(info) > 0 {
							if infoMap, ok := info[0].(map[string]interface{}); ok {
								if bizNickname, ok := infoMap["bizNickname"].(string); ok {
									record.OfficialName = bizNickname
									break
								}
							}
						}
					}
				}
			}
		}
	}

	// 添加IP所在地
	if ipRegionInfo, ok := data["ipRegionInfo"].(map[string]interface{}); ok {
		if regionText, ok := ipRegionInfo["regionText"].(string); ok {
			record.IPRegion = regionText
		}
	}

	// 保存记录
	if h.csvManager != nil {
		if err := h.csvManager.AddRecord(record); err != nil {
			utils.HandleError(err, "保存下载记录")
		} else {
			utils.PrintSeparator()
			color.Green("✅ 下载记录已保存")
			utils.PrintSeparator()
		}
	}

	h.sendEmptyResponse(Conn)
	return true
}

// HandleExportVideoList 处理批量导出视频链接请求
func (h *RecordHandler) HandleExportVideoList(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/export_video_list" {
		return false
	}

	var requestData struct {
		Videos []map[string]interface{} `json:"videos"`
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取export_video_list请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	if err := json.Unmarshal(body, &requestData); err != nil {
		utils.HandleError(err, "解析批量导出请求")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 生成视频链接列表
	var videoList []string
	for i, video := range requestData.Videos {
		title := fmt.Sprintf("%v", video["title"])
		videoId := fmt.Sprintf("%v", video["id"])
		url := fmt.Sprintf("%v", video["url"])

		videoList = append(videoList, fmt.Sprintf("%d. %s\n   ID: %s\n   URL: %s\n",
			i+1, title, videoId, url))
	}

	content := fmt.Sprintf("主页页面视频列表导出\n生成时间: %s\n总计: %d 个视频\n\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		len(requestData.Videos),
		strings.Join(videoList, "\n"))

	// 保存到文件
	baseDir, err := utils.GetBaseDir()
	if err == nil {
		exportDir := filepath.Join(baseDir, h.config.DownloadsDir)
		if err := utils.EnsureDir(exportDir); err == nil {
			exportFile := filepath.Join(exportDir, fmt.Sprintf("profile_videos_export_%s.txt",
				time.Now().Format("20060102_150405")))
			err = os.WriteFile(exportFile, []byte(content), 0644)
			if err == nil {
				utils.PrintSeparator()
				color.Green("📄 视频列表已导出")
				utils.PrintSeparator()
				utils.PrintLabelValue("📁", "导出文件", exportFile)
				utils.PrintLabelValue("📊", "视频数量", len(requestData.Videos))
				utils.PrintSeparator()
			} else {
				utils.HandleError(err, "保存导出文件")
			}
		}
	}

	h.sendEmptyResponse(Conn)
	return true
}

// HandleBatchDownloadStatus 处理批量下载状态查询请求
func (h *RecordHandler) HandleBatchDownloadStatus(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_download_status" {
		return false
	}

	var statusData struct {
		Current int    `json:"current"`
		Total   int    `json:"total"`
		Status  string `json:"status"`
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取batch_download_status请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := Conn.Request.Body.Close(); err != nil {
		utils.HandleError(err, "关闭请求体")
	}

	if err := json.Unmarshal(body, &statusData); err != nil {
		utils.HandleError(err, "解析批量下载状态")
		h.sendErrorResponse(Conn, err)
		return true
	}

	// 显示批量下载进度
	if statusData.Total > 0 {
		percentage := float64(statusData.Current) / float64(statusData.Total) * 100
		utils.PrintSeparator()
		color.Blue("📥 批量下载进度")
		utils.PrintSeparator()
		utils.PrintLabelValue("📊", "进度", fmt.Sprintf("%d/%d (%.1f%%)",
			statusData.Current, statusData.Total, percentage))
		utils.PrintLabelValue("🔄", "状态", statusData.Status)
		utils.PrintSeparator()
	}

	h.sendEmptyResponse(Conn)
	return true
}

// sendEmptyResponse 发送空JSON响应
func (h *RecordHandler) sendEmptyResponse(Conn *SunnyNet.HttpConn) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("__debug", "fake_resp")
	Conn.StopRequest(200, "{}", headers)
}

// sendErrorResponse 发送错误响应
func (h *RecordHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	errorMsg := fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error())
	Conn.StopRequest(500, errorMsg, headers)
}

