package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/storage"
	"wx_channel/internal/utils"
	"wx_channel/pkg/util"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// parseKey 解析密钥字符串为 uint64
func parseKey(key string) (uint64, error) {
	// 尝试直接解析为数字
	if seed, err := strconv.ParseUint(key, 10, 64); err == nil {
		return seed, nil
	}
	// 如果不是纯数字，可能是其他格式，暂不支持
	return 0, fmt.Errorf("无效的密钥格式: %s", key)
}

// BatchHandler 批量下载处理器
type BatchHandler struct {
	config     *config.Config
	csvManager *storage.CSVManager
	mu         sync.RWMutex
	tasks      []BatchTask
	running    bool
	cancelFunc context.CancelFunc // 用于取消时立即中断下载
}

// BatchTask 批量下载任务
type BatchTask struct {
	ID              string  `json:"id"`
	URL             string  `json:"url"`
	Title           string  `json:"title"`
	AuthorName      string  `json:"authorName"`
	Key             string  `json:"key,omitempty"`             // 加密密钥（新方式，后端生成解密数组）
	DecryptorPrefix string  `json:"decryptorPrefix,omitempty"` // 解密前缀（旧方式，前端传递）
	PrefixLen       int     `json:"prefixLen,omitempty"`
	Status          string  `json:"status"` // pending, downloading, done, failed
	Error           string  `json:"error,omitempty"`
	Progress        float64 `json:"progress,omitempty"`
	DownloadedMB    float64 `json:"downloadedMB,omitempty"`
	TotalMB         float64 `json:"totalMB,omitempty"`
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

	utils.Info("📥 [批量下载] 收到 batch_start 请求")

	// 授权校验
	if h.config != nil && h.config.SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.config.SecretToken {
			h.sendErrorResponse(Conn, fmt.Errorf("unauthorized"))
			return true
		}
	}

	utils.Info("📥 [批量下载] 开始读取请求体...")
	body, err := io.ReadAll(Conn.Request.Body)
	if err != nil {
		utils.HandleError(err, "读取batch_start请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}
	defer Conn.Request.Body.Close()

	bodySize := len(body)
	utils.Info("📥 [批量下载] 请求体大小: %.2f MB", float64(bodySize)/(1024*1024))

	var req struct {
		Videos          []BatchTask `json:"videos"`
		ForceRedownload bool        `json:"forceRedownload"`
	}

	utils.Info("📥 [批量下载] 开始解析 JSON...")
	if err := json.Unmarshal(body, &req); err != nil {
		utils.HandleError(err, "解析batch_start JSON")
		h.sendErrorResponse(Conn, err)
		return true
	}
	utils.Info("📥 [批量下载] JSON 解析完成，视频数: %d", len(req.Videos))

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
			Key:             v.Key,
			DecryptorPrefix: v.DecryptorPrefix,
			PrefixLen:       v.PrefixLen,
			Status:          "pending",
		}
	}
	h.running = true
	h.mu.Unlock()

	utils.Info("🚀 [批量下载] 开始下载 %d 个视频，并发数: %d", len(req.Videos), h.config.DownloadConcurrency)

	// 启动后台下载
	go h.startBatchDownload(req.ForceRedownload)

	h.sendSuccessResponse(Conn, map[string]interface{}{
		"total":       len(req.Videos),
		"concurrency": h.config.DownloadConcurrency,
	})
	return true
}

// startBatchDownload 开始批量下载（并发版本）
func (h *BatchHandler) startBatchDownload(forceRedownload bool) {
	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.cancelFunc = cancel
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		h.running = false
		h.cancelFunc = nil
		h.mu.Unlock()
		cancel() // 确保释放资源
	}()

	baseDir, err := utils.GetBaseDir()
	if err != nil {
		utils.HandleError(err, "获取基础目录")
		return
	}
	downloadsDir := filepath.Join(baseDir, h.config.DownloadsDir)

	// 获取并发数
	concurrency := h.config.DownloadConcurrency
	if concurrency < 1 {
		concurrency = 1
	}

	// 创建任务通道
	taskChan := make(chan int, len(h.tasks))
	var wg sync.WaitGroup

	// 启动 worker
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for taskIdx := range taskChan {
				// 检查是否取消
				select {
				case <-ctx.Done():
					return
				default:
				}

				h.mu.Lock()
				task := &h.tasks[taskIdx]
				task.Status = "downloading"
				h.mu.Unlock()

				utils.Info("📥 [Worker %d] 开始下载: %s", workerID, task.Title)

				// 下载视频
				err := h.downloadVideo(ctx, task, downloadsDir, forceRedownload, taskIdx)

				h.mu.Lock()
				if err != nil {
					task.Status = "failed"
					task.Error = err.Error()
					task.Progress = 0
					utils.Error("❌ [Worker %d] 失败: %s - %v", workerID, task.Title, err)
				} else {
					task.Status = "done"
					task.Progress = 100
					utils.Info("✅ [Worker %d] 完成: %s", workerID, task.Title)
				}
				h.mu.Unlock()
			}
		}(w)
	}

	// 分发任务
	for i := range h.tasks {
		select {
		case <-ctx.Done():
			close(taskChan)
			wg.Wait()
			utils.Info("⏹️ [批量下载] 已取消")
			return
		case taskChan <- i:
		}
	}
	close(taskChan)

	// 等待所有 worker 完成
	wg.Wait()

	// 统计结果
	h.mu.RLock()
	done, failed := 0, 0
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


// downloadVideo 下载单个视频（带重试和断点续传）
func (h *BatchHandler) downloadVideo(ctx context.Context, task *BatchTask, downloadsDir string, forceRedownload bool, taskIdx int) error {
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

	// 使用配置的重试次数
	maxRetries := h.config.DownloadRetryCount
	if maxRetries < 1 {
		maxRetries = 3
	}
	var lastErr error

	for retry := 0; retry < maxRetries; retry++ {
		// 检查是否取消
		select {
		case <-ctx.Done():
			return fmt.Errorf("下载已取消")
		default:
		}

		if retry > 0 {
			// 指数退避 + 随机抖动
			baseDelay := time.Duration(1<<uint(retry)) * time.Second // 2s, 4s, 8s...
			jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
			delay := baseDelay + jitter
			utils.Info("🔄 [批量下载] 等待 %v 后重试 (%d/%d): %s", delay, retry, maxRetries-1, task.Title)
			
			select {
			case <-ctx.Done():
				return fmt.Errorf("下载已取消")
			case <-time.After(delay):
			}
		}

		// 使用配置的超时时间
		timeout := h.config.DownloadTimeout
		if timeout == 0 {
			timeout = 10 * time.Minute
		}
		downloadCtx, cancel := context.WithTimeout(ctx, timeout)
		err := h.downloadVideoOnce(downloadCtx, task, filePath, taskIdx)
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err
		utils.Warn("⚠️ [批量下载] 下载失败 (尝试 %d/%d): %v", retry+1, maxRetries, err)

		// 如果不支持断点续传或是加密视频，清理临时文件
		if task.DecryptorPrefix != "" || !h.config.DownloadResumeEnabled {
			os.Remove(filePath + ".tmp")
		}
	}

	return fmt.Errorf("下载失败（已重试 %d 次）: %v", maxRetries, lastErr)
}

// downloadVideoOnce 执行一次下载尝试（支持断点续传）
func (h *BatchHandler) downloadVideoOnce(ctx context.Context, task *BatchTask, filePath string, taskIdx int) error {
	tmpPath := filePath + ".tmp"
	
	// 判断是否需要解密：优先使用 key（新方式），其次使用 decryptorPrefix（旧方式）
	needDecrypt := task.Key != "" || (task.DecryptorPrefix != "" && task.PrefixLen > 0)

	// 断点续传：检查已下载的部分（仅非加密视频支持）
	var resumeOffset int64 = 0
	if !needDecrypt && h.config.DownloadResumeEnabled {
		if stat, err := os.Stat(tmpPath); err == nil {
			resumeOffset = stat.Size()
			utils.Info("📍 [批量下载] 断点续传，从 %.2f MB 继续", float64(resumeOffset)/(1024*1024))
		}
	}

	// 创建HTTP客户端
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

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", task.URL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 断点续传：设置 Range 头
	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 如果服务器不支持 Range，重新下载
	if resumeOffset > 0 && resp.StatusCode != 206 {
		utils.Warn("⚠️ [批量下载] 服务器不支持断点续传，重新下载")
		resumeOffset = 0
		os.Remove(tmpPath)
	}

	// 计算总大小
	var totalSize int64
	if resp.StatusCode == 206 {
		// 断点续传：总大小 = 已下载 + Content-Length
		totalSize = resumeOffset + resp.ContentLength
	} else {
		totalSize = resp.ContentLength
	}

	if totalSize > 0 {
		sizeMB := float64(totalSize) / (1024 * 1024)
		utils.Info("📦 [批量下载] 文件大小: %.2f MB", sizeMB)
		h.mu.Lock()
		if taskIdx >= 0 && taskIdx < len(h.tasks) {
			h.tasks[taskIdx].TotalMB = sizeMB
		}
		h.mu.Unlock()
	}

	// 打开/创建文件
	var out *os.File
	if resumeOffset > 0 {
		out, err = os.OpenFile(tmpPath, os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		out, err = os.Create(tmpPath)
	}
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}

	// 下载并写入
	var writeErr error
	if needDecrypt {
		utils.Info("🔐 [批量下载] 开始解密下载...")
		writeErr = h.downloadAndDecrypt(ctx, resp.Body, out, task, taskIdx, totalSize)
	} else {
		utils.Info("📥 [批量下载] 开始下载...")
		writeErr = h.downloadWithProgress(ctx, resp.Body, out, taskIdx, totalSize, resumeOffset)
	}

	closeErr := out.Close()

	if writeErr != nil {
		// 断点续传模式下不删除临时文件
		if !h.config.DownloadResumeEnabled || needDecrypt {
			os.Remove(tmpPath)
		}
		return fmt.Errorf("写入文件失败: %v", writeErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("关闭文件失败: %v", closeErr)
	}

	// 验证文件
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

	sizeMB := float64(stat.Size()) / (1024 * 1024)
	if needDecrypt {
		utils.Info("✓ 视频已保存（已解密）: %s (%.2f MB)", filePath, sizeMB)
	} else {
		utils.Info("✓ 视频已保存: %s (%.2f MB)", filePath, sizeMB)
	}

	return nil
}


// downloadWithProgress 带进度的下载（支持断点续传）
func (h *BatchHandler) downloadWithProgress(ctx context.Context, reader io.Reader, writer io.Writer, taskIdx int, totalSize int64, resumeOffset int64) error {
	buf := make([]byte, 32*1024)
	totalCopied := resumeOffset
	lastLog := time.Now()

	for {
		// 检查是否取消
		select {
		case <-ctx.Done():
			return fmt.Errorf("下载已取消")
		default:
		}

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
func (h *BatchHandler) downloadAndDecrypt(ctx context.Context, reader io.Reader, writer io.Writer, task *BatchTask, taskIdx int, totalSize int64) error {
	var decryptorPrefix []byte
	var prefixLen int
	
	// 优先使用 key 生成解密数组（新方式）
	if task.Key != "" {
		// 解析 key 为 uint64
		seed, err := parseKey(task.Key)
		if err != nil {
			return fmt.Errorf("解析密钥失败: %v", err)
		}
		// 生成 128KB 解密数组
		prefixLen = 131072
		decryptorPrefix = util.GenerateDecryptorArray(seed, prefixLen)
		utils.Info("🔑 [批量下载] 从 key 生成解密数组，长度: %d bytes", len(decryptorPrefix))
	} else if task.DecryptorPrefix != "" && task.PrefixLen > 0 {
		// 使用前端传递的解密数组（旧方式）
		var err error
		decryptorPrefix, err = base64.StdEncoding.DecodeString(task.DecryptorPrefix)
		if err != nil {
			return fmt.Errorf("解码密钥失败: %v", err)
		}
		prefixLen = task.PrefixLen
		utils.Info("🔑 [批量下载] 使用前端解密数组，长度: %d bytes", len(decryptorPrefix))
	} else {
		return fmt.Errorf("缺少解密密钥")
	}

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

	// 复制剩余数据（带进度和取消检查）
	buf := make([]byte, 32*1024)
	totalCopied := int64(n)
	lastLog := time.Now()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("下载已取消")
		default:
		}

		nr, er := reader.Read(buf)
		if nr > 0 {
			nw, ew := writer.Write(buf[0:nr])
			if nw > 0 {
				totalCopied += int64(nw)

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
	done, failed, running := 0, 0, 0
	var downloadingTasks []map[string]interface{}

	for _, t := range h.tasks {
		switch t.Status {
		case "done":
			done++
		case "failed":
			failed++
		case "downloading":
			running++
			downloadingTasks = append(downloadingTasks, map[string]interface{}{
				"title":        t.Title,
				"progress":     t.Progress,
				"downloadedMB": t.DownloadedMB,
				"totalMB":      t.TotalMB,
			})
		}
	}
	h.mu.RUnlock()

	response := map[string]interface{}{
		"total":   total,
		"done":    done,
		"failed":  failed,
		"running": running,
	}

	// 返回所有正在下载的任务（并发模式下可能有多个）
	if len(downloadingTasks) > 0 {
		response["currentTasks"] = downloadingTasks
		// 兼容旧版本，返回第一个
		response["currentTask"] = downloadingTasks[0]
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
	if h.running && h.cancelFunc != nil {
		h.cancelFunc() // 立即取消所有正在进行的下载
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
