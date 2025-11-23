package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/storage"
	"wx_channel/internal/utils"
	"wx_channel/pkg/util"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// BatchHandler 批量下载处理器
type BatchHandler struct {
	config          *config.Config
	csvManager      *storage.CSVManager
	mu              sync.RWMutex
	tasks           []BatchTask
	running         bool
	cancelChan      chan struct{}
	currentTaskIdx  int     // 当前任务索引
	currentProgress float64 // 当前任务进度
}

// BatchTask 批量下载任务
type BatchTask struct {
	ID              string  `json:"id"`
	URL             string  `json:"url"`
	Title           string  `json:"title"`
	AuthorName      string  `json:"authorName"`
	DecryptorPrefix string  `json:"decryptorPrefix,omitempty"`
	PrefixLen       int     `json:"prefixLen,omitempty"`
	Status          string  `json:"status"` // pending, downloading, done, failed
	Error           string  `json:"error,omitempty"`
	Progress        float64 `json:"progress,omitempty"`        // 下载进度 (0-100)
	DownloadedMB    float64 `json:"downloadedMB,omitempty"`    // 已下载大小(MB)
	TotalMB         float64 `json:"totalMB,omitempty"`         // 总大小(MB)
}

// NewBatchHandler 创建批量下载处理器
func NewBatchHandler(cfg *config.Config, csvManager *storage.CSVManager) *BatchHandler {
	return &BatchHandler{
		config:     cfg,
		csvManager: csvManager,
		tasks:      make([]BatchTask, 0),
	}
}

// HandleBatchStart 处理批量下载开始请求
func (h *BatchHandler) HandleBatchStart(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_start" {
		return false
	}

	// 授权校验
	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			h.sendErrorResponse(Conn, fmt.Errorf("unauthorized"))
			return true
		}
	}

	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取batch_start请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}
	defer Conn.Request.Body.Close()

	var req struct {
		Videos          []BatchTask `json:"videos"`
		ForceRedownload bool        `json:"forceRedownload"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		utils.HandleError(err, "解析batch_start JSON")
		h.sendErrorResponse(Conn, err)
		return true
	}

	if len(req.Videos) == 0 {
		h.sendErrorResponse(Conn, fmt.Errorf("视频列表为空"))
		return true
	}

	// 初始化任务
	h.mu.Lock()
	h.tasks = make([]BatchTask, len(req.Videos))
	for i, v := range req.Videos {
		h.tasks[i] = BatchTask{
			ID:              v.ID,
			URL:             v.URL,
			Title:           v.Title,
			AuthorName:      v.AuthorName,
			DecryptorPrefix: v.DecryptorPrefix,
			PrefixLen:       v.PrefixLen,
			Status:          "pending",
		}
	}
	h.running = true
	h.cancelChan = make(chan struct{})
	h.mu.Unlock()

	utils.Info("🚀 [批量下载] 开始下载 %d 个视频", len(req.Videos))

	// 启动后台下载
	go h.startBatchDownload(req.ForceRedownload)

	h.sendSuccessResponse(Conn, map[string]interface{}{
		"total": len(req.Videos),
	})
	return true
}

// startBatchDownload 开始批量下载
func (h *BatchHandler) startBatchDownload(forceRedownload bool) {
	defer func() {
		h.mu.Lock()
		h.running = false
		h.mu.Unlock()
	}()

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录")
		return
	}

	downloadsDir := filepath.Join(baseDir, h.config.DownloadsDir)

	for i := range h.tasks {
		// 检查是否取消
		select {
		case <-h.cancelChan:
			utils.Info("⏹️ [批量下载] 已取消")
			return
		default:
		}

		task := &h.tasks[i]
		h.mu.Lock()
		task.Status = "downloading"
		h.currentTaskIdx = i
		h.currentProgress = 0
		h.mu.Unlock()

		utils.Info("📥 [批量下载] 进度: %d/%d - %s", i+1, len(h.tasks), task.Title)

		// 下载视频
		err := h.downloadVideo(task, downloadsDir, forceRedownload, i)
		
		h.mu.Lock()
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
			task.Progress = 0
			utils.Error("❌ [批量下载] 失败: %s - %v", task.Title, err)
		} else {
			task.Status = "done"
			task.Progress = 100
			utils.Info("✅ [批量下载] 完成: %s", task.Title)
		}
		h.mu.Unlock()
	}

	// 统计结果
	h.mu.RLock()
	done := 0
	failed := 0
	for _, t := range h.tasks {
		if t.Status == "done" {
			done++
		} else if t.Status == "failed" {
			failed++
		}
	}
	h.mu.RUnlock()

	utils.Info("✅ [批量下载] 全部完成！成功: %d, 失败: %d", done, failed)
}

// downloadVideo 下载单个视频
func (h *BatchHandler) downloadVideo(task *BatchTask, downloadsDir string, forceRedownload bool, taskIdx int) error {
	// 创建作者目录
	authorFolder := utils.CleanFolderName(task.AuthorName)
	savePath := filepath.Join(downloadsDir, authorFolder)
	if err := utils.EnsureDir(savePath); err != nil {
		return fmt.Errorf("创建作者目录失败: %v", err)
	}

	// 生成文件名
	cleanFilename := utils.CleanFilename(task.Title)
	cleanFilename = utils.EnsureExtension(cleanFilename, ".mp4")
	filePath := filepath.Join(savePath, cleanFilename)

	// 检查文件是否已存在
	if !forceRedownload {
		if _, err := os.Stat(filePath); err == nil {
			utils.Info("⏭️ [批量下载] 文件已存在，跳过: %s", cleanFilename)
			return nil
		}
	}

	// 重试下载（最多3次）
	maxRetries := 3
	var lastErr error
	
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			// 递增延迟，给服务器和网络恢复时间
			delay := time.Second * time.Duration(retry*2)
			utils.Info("🔄 [批量下载] 等待 %v 后重试 (%d/%d): %s", delay, retry, maxRetries-1, task.Title)
			time.Sleep(delay)
		}
		
		// 使用带超时的context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		err := h.downloadVideoOnceWithContext(ctx, task, filePath, downloadsDir, taskIdx)
		cancel() // 确保释放资源
		
		if err == nil {
			return nil // 成功
		}
		
		lastErr = err
		utils.Warn("⚠️ [批量下载] 下载失败 (尝试 %d/%d): %v", retry+1, maxRetries, err)
		
		// 清理可能存在的临时文件
		tmpPath := filePath + ".tmp"
		os.Remove(tmpPath)
	}
	
	return fmt.Errorf("下载失败（已重试 %d 次）: %v", maxRetries, lastErr)
}

// downloadVideoOnceWithContext 执行一次下载尝试（带context）
func (h *BatchHandler) downloadVideoOnceWithContext(ctx context.Context, task *BatchTask, filePath string, downloadsDir string, taskIdx int) error {
	// 创建HTTP客户端，使用context控制超时
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       30 * time.Second,
			DisableKeepAlives:     false,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}

	// 创建带context的请求
	req, err := http.NewRequestWithContext(ctx, "GET", task.URL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 下载视频
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 创建临时文件
	tmpPath := filePath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}

	// 判断是否需要解密
	needDecrypt := task.DecryptorPrefix != "" && task.PrefixLen > 0

	// 获取文件大小（如果有）
	contentLength := resp.ContentLength
	if contentLength > 0 {
		sizeMB := float64(contentLength) / (1024 * 1024)
		utils.Info("📦 [批量下载] 文件大小: %.2f MB", sizeMB)
		
		// 更新任务信息
		h.mu.Lock()
		if taskIdx >= 0 && taskIdx < len(h.tasks) {
			h.tasks[taskIdx].TotalMB = sizeMB
		}
		h.mu.Unlock()
	}

	// 下载并写入文件
	var writeErr error
	if needDecrypt {
		// 解密下载
		utils.Info("🔐 [批量下载] 开始解密下载...")
		writeErr = h.downloadAndDecrypt(resp.Body, out, task.DecryptorPrefix, task.PrefixLen, taskIdx, contentLength)
	} else {
		// 直接下载，使用带缓冲的复制
		utils.Info("📥 [批量下载] 开始下载...")
		writeErr = h.downloadWithProgress(resp.Body, out, taskIdx, contentLength)
	}
	
	if writeErr != nil {
		utils.Error("❌ [批量下载] 写入失败: %v", writeErr)
	} else {
		utils.Info("✓ [批量下载] 写入完成")
	}

	// 关闭文件（必须在重命名之前关闭）
	closeErr := out.Close()

	// 检查写入错误
	if writeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("写入文件失败: %v", writeErr)
	}

	// 检查关闭错误
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("关闭文件失败: %v", closeErr)
	}

	// 验证文件大小
	stat, err := os.Stat(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("验证文件失败: %v", err)
	}

	if stat.Size() == 0 {
		os.Remove(tmpPath)
		return fmt.Errorf("下载的文件为空")
	}

	// 重命名为最终文件
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("重命名文件失败: %v", err)
	}

	// 显示成功信息
	sizeMB := float64(stat.Size()) / (1024 * 1024)
	if needDecrypt {
		utils.Info("✓ 视频已保存（已解密）: %s (%.2f MB)", filePath, sizeMB)
	} else {
		utils.Info("✓ 视频已保存: %s (%.2f MB)", filePath, sizeMB)
	}

	return nil
}

// downloadWithProgress 带进度的下载
func (h *BatchHandler) downloadWithProgress(reader io.Reader, writer io.Writer, taskIdx int, totalSize int64) error {
	buf := make([]byte, 32*1024)
	totalCopied := int64(0)
	lastLog := time.Now()
	
	for {
		nr, er := reader.Read(buf)
		if nr > 0 {
			nw, ew := writer.Write(buf[0:nr])
			if nw > 0 {
				totalCopied += int64(nw)
				
				// 更新进度
				if totalSize > 0 {
					progress := float64(totalCopied) / float64(totalSize) * 100
					downloadedMB := float64(totalCopied) / (1024 * 1024)
					
					h.mu.Lock()
					if taskIdx >= 0 && taskIdx < len(h.tasks) {
						h.tasks[taskIdx].Progress = progress
						h.tasks[taskIdx].DownloadedMB = downloadedMB
					}
					h.mu.Unlock()
				}
			}
			if ew != nil {
				return fmt.Errorf("写入数据失败: %v", ew)
			}
			if nr != nw {
				return fmt.Errorf("写入不完整")
			}
			
			// 每5秒输出一次进度
			if time.Since(lastLog) > 5*time.Second {
				utils.Info("📊 [批量下载] 已下载: %.2f MB", float64(totalCopied)/(1024*1024))
				lastLog = time.Now()
			}
		}
		if er != nil {
			if er != io.EOF {
				return fmt.Errorf("读取数据失败: %v", er)
			}
			break
		}
	}
	
	return nil
}

// downloadAndDecrypt 下载并解密视频
func (h *BatchHandler) downloadAndDecrypt(reader io.Reader, writer io.Writer, decryptorPrefixB64 string, prefixLen int, taskIdx int, totalSize int64) error {
	// 解码 Base64 密钥
	decryptorPrefix, err := base64.StdEncoding.DecodeString(decryptorPrefixB64)
	if err != nil {
		return fmt.Errorf("解码密钥失败: %v", err)
	}

	utils.Info("🔑 [批量下载] 密钥长度: %d bytes", len(decryptorPrefix))

	// 读取前缀数据
	prefixData := make([]byte, prefixLen)
	n, err := io.ReadFull(reader, prefixData)
	if err != nil && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("读取前缀失败: %v", err)
	}
	prefixData = prefixData[:n]

	utils.Info("📖 [批量下载] 读取前缀: %d bytes", n)

	// 解密前缀
	decryptedPrefix := util.XorDecrypt(prefixData, decryptorPrefix)

	// 写入解密后的前缀
	if _, err := writer.Write(decryptedPrefix); err != nil {
		return fmt.Errorf("写入解密前缀失败: %v", err)
	}

	utils.Info("✓ [批量下载] 前缀解密完成")

	// 复制剩余数据（带进度）
	buf := make([]byte, 32*1024)
	totalCopied := int64(n) // 包括前缀
	lastLog := time.Now()
	
	for {
		nr, er := reader.Read(buf)
		if nr > 0 {
			nw, ew := writer.Write(buf[0:nr])
			if nw > 0 {
				totalCopied += int64(nw)
				
				// 更新进度
				if totalSize > 0 {
					progress := float64(totalCopied) / float64(totalSize) * 100
					downloadedMB := float64(totalCopied) / (1024 * 1024)
					
					h.mu.Lock()
					if taskIdx >= 0 && taskIdx < len(h.tasks) {
						h.tasks[taskIdx].Progress = progress
						h.tasks[taskIdx].DownloadedMB = downloadedMB
					}
					h.mu.Unlock()
				}
			}
			if ew != nil {
				return fmt.Errorf("写入数据失败: %v", ew)
			}
			if nr != nw {
				return fmt.Errorf("写入不完整")
			}
			
			// 每5秒输出一次进度
			if time.Since(lastLog) > 5*time.Second {
				utils.Info("📊 [批量下载] 已下载: %.2f MB", float64(totalCopied)/(1024*1024))
				lastLog = time.Now()
			}
		}
		if er != nil {
			if er != io.EOF {
				return fmt.Errorf("读取数据失败: %v", er)
			}
			break
		}
	}

	utils.Info("✓ [批量下载] 剩余数据复制完成: %.2f MB", float64(totalCopied)/(1024*1024))
	return nil
}

// HandleBatchProgress 处理批量下载进度查询请求
func (h *BatchHandler) HandleBatchProgress(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_progress" {
		return false
	}

	// 授权校验
	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			h.sendErrorResponse(Conn, fmt.Errorf("unauthorized"))
			return true
		}
	}

	h.mu.RLock()
	total := len(h.tasks)
	done := 0
	failed := 0
	running := 0
	var currentTask *BatchTask

	for i, t := range h.tasks {
		switch t.Status {
		case "done":
			done++
		case "failed":
			failed++
		case "downloading":
			running++
			if i == h.currentTaskIdx {
				currentTask = &t
			}
		}
	}
	h.mu.RUnlock()

	response := map[string]interface{}{
		"total":   total,
		"done":    done,
		"failed":  failed,
		"running": running,
	}

	// 添加当前任务信息
	if currentTask != nil {
		response["currentTask"] = map[string]interface{}{
			"title":        currentTask.Title,
			"progress":     currentTask.Progress,
			"downloadedMB": currentTask.DownloadedMB,
			"totalMB":      currentTask.TotalMB,
		}
	}

	h.sendSuccessResponse(Conn, response)
	return true
}

// HandleBatchCancel 处理批量下载取消请求
func (h *BatchHandler) HandleBatchCancel(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_cancel" {
		return false
	}

	// 授权校验
	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			h.sendErrorResponse(Conn, fmt.Errorf("unauthorized"))
			return true
		}
	}

	h.mu.Lock()
	if h.running && h.cancelChan != nil {
		close(h.cancelChan)
		h.running = false
	}
	h.mu.Unlock()

	utils.Info("⏹️ [批量下载] 用户取消下载")

	h.sendSuccessResponse(Conn, map[string]interface{}{
		"message": "下载已取消",
	})
	return true
}

// HandleBatchFailed 处理导出失败清单请求
func (h *BatchHandler) HandleBatchFailed(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_failed" {
		return false
	}

	// 授权校验
	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			h.sendErrorResponse(Conn, fmt.Errorf("unauthorized"))
			return true
		}
	}

	h.mu.RLock()
	failedTasks := make([]BatchTask, 0)
	for _, t := range h.tasks {
		if t.Status == "failed" {
			failedTasks = append(failedTasks, t)
		}
	}
	h.mu.RUnlock()

	if len(failedTasks) == 0 {
		h.sendSuccessResponse(Conn, map[string]interface{}{
			"failed": 0,
		})
		return true
	}

	// 导出失败清单
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		h.sendErrorResponse(Conn, err)
		return true
	}

	downloadsDir := filepath.Join(baseDir, h.config.DownloadsDir)
	timestamp := time.Now().Format("20060102_150405")
	exportFile := filepath.Join(downloadsDir, fmt.Sprintf("failed_videos_%s.json", timestamp))

	data, err := json.MarshalIndent(failedTasks, "", "  ")
	if err != nil {
		h.sendErrorResponse(Conn, err)
		return true
	}

	if err := os.WriteFile(exportFile, data, 0644); err != nil {
		h.sendErrorResponse(Conn, err)
		return true
	}

	utils.Info("📄 [批量下载] 失败清单已导出: %s", exportFile)

	h.sendSuccessResponse(Conn, map[string]interface{}{
		"failed": len(failedTasks),
		"json":   exportFile,
	})
	return true
}

// sendSuccessResponse 发送成功响应
func (h *BatchHandler) sendSuccessResponse(Conn *SunnyNet.HttpConn, data map[string]interface{}) {
	data["success"] = true
	
	responseBytes, err := json.Marshal(data)
	if err != nil {
		h.sendErrorResponse(Conn, err)
		return
	}

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
					headers.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
					break
				}
			}
		}
	}
	
	Conn.StopRequest(200, string(responseBytes), headers)
}

// sendErrorResponse 发送错误响应
func (h *BatchHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
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
					headers.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
					break
				}
			}
		}
	}
	
	errorMsg := fmt.Sprintf(`{"success":false,"error":"%s"}`, strings.ReplaceAll(err.Error(), `"`, `\"`))
	Conn.StopRequest(500, errorMsg, headers)
}
