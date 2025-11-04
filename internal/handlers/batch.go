package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"encoding/base64"
	"wx_channel/internal/config"
	"wx_channel/internal/services"
	"wx_channel/internal/storage"
	"wx_channel/internal/utils"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

type BatchHandler struct {
	cfg        *config.Config
	csv        *storage.CSVManager
	downloader *services.Downloader
}

func NewBatchHandler(cfg *config.Config, csv *storage.CSVManager) *BatchHandler {
	return &BatchHandler{cfg: cfg, csv: csv, downloader: services.NewDownloader(cfg, csv)}
}

// HandleBatchStart 接收任务并入队
func (h *BatchHandler) HandleBatchStart(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_start" {
		return false
	}

	if h.cfg != nil && h.cfg.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.cfg.SecretToken {
			headers := http.Header{}
			headers.Set("Content-Type", "application/json")
			headers.Set("X-Content-Type-Options", "nosniff")
			Conn.StopRequest(401, `{"success":false,"error":"unauthorized"}`, headers)
			return true
		}
	}

	var req struct {
		Videos []struct {
			ID              string `json:"id"`
			URL             string `json:"url"`
			Title           string `json:"title"`
			Filename        string `json:"filename"`
			AuthorName      string `json:"authorName"`
			DecryptorPrefix string `json:"decryptorPrefix"`
			PrefixLen       int    `json:"prefixLen"`
		} `json:"videos"`
	}
	body, _ := io.ReadAll(Conn.Request.Body)
	_ = Conn.Request.Body.Close()
	_ = json.Unmarshal(body, &req)

	tasks := make([]services.DownloadTask, 0, len(req.Videos))
	for _, v := range req.Videos {
		name := v.Filename
		if name == "" {
			name = v.Title
		}
		var dec []byte
		if v.DecryptorPrefix != "" {
			if b, err := base64.StdEncoding.DecodeString(v.DecryptorPrefix); err == nil {
				if v.PrefixLen > 0 && v.PrefixLen <= len(b) {
					dec = b[:v.PrefixLen]
				} else {
					dec = b
				}
			}
		}
		tasks = append(tasks, services.DownloadTask{ID: v.ID, URL: v.URL, Filename: name, AuthorName: v.AuthorName, Decryptor: dec})
	}
	h.downloader.Enqueue(tasks)

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	Conn.StopRequest(200, `{"success":true}`, headers)
	return true
}

// HandleBatchProgress 查询进度
func (h *BatchHandler) HandleBatchProgress(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_progress" {
		return false
	}
	total, done, failed, running := h.downloader.Progress()
	resp := map[string]interface{}{"success": true, "total": total, "done": done, "failed": failed, "running": running}
	b, _ := json.Marshal(resp)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	Conn.StopRequest(200, string(b), headers)
	return true
}

// HandleBatchCancel 取消任务
func (h *BatchHandler) HandleBatchCancel(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_cancel" {
		return false
	}
	h.downloader.Cancel()
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	Conn.StopRequest(200, `{"success":true}`, headers)
	return true
}

// HandleBatchFailed 导出失败清单到 downloads，并返回路径
func (h *BatchHandler) HandleBatchFailed(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_failed" {
		return false
	}

	failed := h.downloader.FailedResults()
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		headers := http.Header{}
		headers.Set("Content-Type", "application/json")
		Conn.StopRequest(500, `{"success":false}`, headers)
		return true
	}
	exportDir := filepath.Join(baseDir, h.cfg.DownloadsDir)
	_ = utils.EnsureDir(exportDir)

	// 导出 JSON
	payload := make([]map[string]interface{}, 0, len(failed))
	for _, r := range failed {
		payload = append(payload, map[string]interface{}{
			"id":         r.Task.ID,
			"url":        r.Task.URL,
			"filename":   r.Task.Filename,
			"authorName": r.Task.AuthorName,
			"error": func() string {
				if r.Err != nil {
					return r.Err.Error()
				}
				return ""
			}(),
		})
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	jsonFile := filepath.Join(exportDir, "batch_failed_"+time.Now().Format("20060102_150405")+".json")
	_ = os.WriteFile(jsonFile, b, 0644)

	resp := map[string]interface{}{"success": true, "failed": len(failed), "json": jsonFile}
	rb, _ := json.Marshal(resp)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	Conn.StopRequest(200, string(rb), headers)
	return true
}
