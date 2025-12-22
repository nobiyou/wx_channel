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
	"wx_channel/internal/database"
	"wx_channel/internal/models"
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
	AuthorName      string  `json:"authorName,omitempty"`      // 兼容旧格式
	Author          string  `json:"author,omitempty"`          // 新格式
	Key             string  `json:"key,omitempty"`             // 加密密钥（新方式，后端生成解密数组）
	DecryptorPrefix string  `json:"decryptorPrefix,omitempty"` // 解密前缀（旧方式，前端传递）
	PrefixLen       int     `json:"prefixLen,omitempty"`
	Status          string  `json:"status"` // pending, downloading, done, failed
	Error           string  `json:"error,omitempty"`
	Progress        float64 `json:"progress,omitempty"`
	DownloadedMB    float64 `json:"downloadedMB,omitempty"`
	TotalMB         float64 `json:"totalMB,omitempty"`
	// 额外字段用于下载记录（批量下载JSON格式）
	Duration   string `json:"duration,omitempty"`   // 时长字符串，如 "00:22"
	SizeMB     string `json:"sizeMB,omitempty"`     // 大小字符串，如 "28.77MB"
	Cover      string `json:"cover,omitempty"`      // 封面URL（批量下载格式）
	Resolution string `json:"resolution,omitempty"` // 分辨率
	PageSource string `json:"pageSource,omitempty"` // 页面来源（batch_console/batch_feed/batch_home等）
	// 统计数据字段
	PlayCount    string `json:"playCount,omitempty"`    // 播放量（字符串格式）
	LikeCount    string `json:"likeCount,omitempty"`    // 点赞数（字符串格式）
	CommentCount string `json:"commentCount,omitempty"` // 评论数（字符串格式）
	FavCount     string `json:"favCount,omitempty"`     // 收藏数（字符串格式）
	ForwardCount string `json:"forwardCount,omitempty"` // 转发数（字符串格式）
	CreateTime   string `json:"createTime,omitempty"`   // 创建时间
	IPRegion     string `json:"ipRegion,omitempty"`     // IP所在地
	// 兼容数据库导出格式
	VideoURL   string `json:"videoUrl,omitempty"`   // 视频URL（数据库格式）
	CoverURL   string `json:"coverUrl,omitempty"`   // 封面URL（数据库格式）
	DecryptKey string `json:"decryptKey,omitempty"` // 解密密钥（数据库格式）
	DurationMs int64  `json:"durationMs,omitempty"` // 时长毫秒（数据库格式，字段名为duration但类型是int64）
	Size       int64  `json:"size,omitempty"`       // 大小字节（数据库格式）
}

// GetAuthor 获取作者名称，兼容两种字段
func (t *BatchTask) GetAuthor() string {
	if t.Author != "" {
		return t.Author
	}
	return t.AuthorName
}

// GetURL 获取视频URL，兼容两种格式
func (t *BatchTask) GetURL() string {
	if t.URL != "" {
		return t.URL
	}
	return t.VideoURL
}

// GetKey 获取解密密钥，兼容两种格式
func (t *BatchTask) GetKey() string {
	if t.Key != "" {
		return t.Key
	}
	return t.DecryptKey
}

// GetCover 获取封面URL，兼容两种格式
func (t *BatchTask) GetCover() string {
	if t.Cover != "" {
		return t.Cover
	}
	return t.CoverURL
}

// NewBatchHandler 创建批量下载处理器
func NewBatchHandler(cfg *config.Config, csvManager *storage.CSVManager) *BatchHandler {
	return &BatchHandler{
		csvManager: csvManager,
		tasks:      make([]BatchTask, 0),
	}
}

// getConfig 获取当前配置（动态获取最新配置）
func (h *BatchHandler) getConfig() *config.Config {
	return config.Get()
}

// getDownloadsDir 获取解析后的下载目录
func (h *BatchHandler) getDownloadsDir() (string, error) {
	cfg := h.getConfig()
	return cfg.GetResolvedDownloadsDir()
}

// HandleBatchStart 处理批量下载开始请求
func (h *BatchHandler) HandleBatchStart(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_start" {
		return false
	}

	utils.Info("📥 [批量下载] 收到 batch_start 请求")

	// 处理 CORS 预检请求
	if Conn.Request.Method == "OPTIONS" {
		h.sendSuccessResponse(Conn, map[string]interface{}{"message": "OK"})
		return true
	}

	// 只处理 POST 请求
	if Conn.Request.Method != "POST" {
		h.sendErrorResponse(Conn, fmt.Errorf("method not allowed: %s", Conn.Request.Method))
		return true
	}

	// 授权校验
	if h.getConfig() != nil && h.getConfig().SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.getConfig().SecretToken {
			h.sendErrorResponse(Conn, fmt.Errorf("unauthorized"))
			return true
		}
	}

	utils.Info("📥 [批量下载] 开始读取请求体...")
	
	// 检查请求体是否为空
	if Conn.Request.Body == nil {
		err := fmt.Errorf("request body is nil")
		utils.HandleError(err, "读取batch_start请求体")
		h.sendErrorResponse(Conn, err)
		return true
	}
	
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
		PageSource      string      `json:"pageSource,omitempty"` // 页面来源
	}

	utils.Info("📥 [批量下载] 开始解析 JSON...")
	if err := json.Unmarshal(body, &req); err != nil {
		utils.HandleError(err, "解析batch_start JSON")
		h.sendErrorResponse(Conn, err)
		return true
	}
	utils.Info("📥 [批量下载] JSON 解析完成，视频数: %d", len(req.Videos))

	// 判断批量下载来源
	pageSource := req.PageSource
	if pageSource == "" {
		// 如果请求体中没有指定，则通过请求头判断
		origin := Conn.Request.Header.Get("Origin")
		referer := Conn.Request.Header.Get("Referer")

		if strings.Contains(origin, "channels.weixin.qq.com") || strings.Contains(referer, "channels.weixin.qq.com") {
			// 从视频号页面发起的请求，尝试从Referer中提取页面类型
			if strings.Contains(referer, "/web/pages/feed") {
				pageSource = "batch_feed"
			} else if strings.Contains(referer, "/web/pages/home") {
				pageSource = "batch_home"
			} else if strings.Contains(referer, "/web/pages/profile") {
				pageSource = "batch_profile"
			} else if strings.Contains(referer, "/web/pages/s") {
				pageSource = "batch_search" // 搜索页面批量下载
			} else {
				pageSource = "batch_channels" // 默认标记为视频号批量下载
			}
		} else {
			// 从Web控制台发起的请求
			pageSource = "batch_console"
		}
	}
	utils.Info("📥 [批量下载] 来源: %s", pageSource)

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
			AuthorName:      v.GetAuthor(), // 兼容 author 和 authorName
			Author:          v.Author,
			Key:             v.Key,
			DecryptorPrefix: v.DecryptorPrefix,
			PrefixLen:       v.PrefixLen,
			Status:          "pending",
			// 保留额外字段
			Duration:     v.Duration,
			SizeMB:       v.SizeMB,
			Cover:        v.Cover,
			Resolution:   v.Resolution,
			PageSource:   pageSource, // 保存页面来源
			PlayCount:    v.PlayCount,
			LikeCount:    v.LikeCount,
			CommentCount: v.CommentCount,
			FavCount:     v.FavCount,
			ForwardCount: v.ForwardCount,
			CreateTime:   v.CreateTime,
			IPRegion:     v.IPRegion,
		}
	}
	h.running = true
	h.mu.Unlock()

	// 获取并发数配置
	concurrency := 5 // 默认值（与配置默认值一致）
	if h.getConfig() != nil && h.getConfig().DownloadConcurrency > 0 {
		concurrency = h.getConfig().DownloadConcurrency
	}

	utils.Info("🚀 [批量下载] 开始下载 %d 个视频，并发数: %d", len(req.Videos), concurrency)

	// 启动后台下载
	go h.startBatchDownload(req.ForceRedownload)

	h.sendSuccessResponse(Conn, map[string]interface{}{
		"total":       len(req.Videos),
		"concurrency": concurrency,
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

	// 获取下载目录
	downloadsDir, err := h.getDownloadsDir()
	if err != nil {
		utils.HandleError(err, "获取下载目录")
		return
	}

	// 获取并发数
	concurrency := 5 // 默认值（与配置默认值一致）
	if h.getConfig() != nil && h.getConfig().DownloadConcurrency > 0 {
		concurrency = h.getConfig().DownloadConcurrency
	}
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
	authorFolder := utils.CleanFolderName(task.GetAuthor())
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
			// 文件已存在也保存记录（标记为已完成）
			h.saveDownloadRecord(task, filePath, "completed")
			return nil
		}
	}

	// 使用配置的重试次数
	maxRetries := 3
	if h.getConfig() != nil {
		maxRetries = h.getConfig().DownloadRetryCount
	}
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
		timeout := 10 * time.Minute
		if h.getConfig() != nil && h.getConfig().DownloadTimeout > 0 {
			timeout = h.getConfig().DownloadTimeout
		}
		downloadCtx, cancel := context.WithTimeout(ctx, timeout)
		err := h.downloadVideoOnce(downloadCtx, task, filePath, taskIdx)
		cancel()

		if err == nil {
			// 下载成功，保存到下载记录数据库
			h.saveDownloadRecord(task, filePath, "completed")
			return nil
		}

		lastErr = err
		utils.LogDownloadRetry(task.ID, task.Title, retry+1, maxRetries, err)
		utils.Warn("⚠️ [批量下载] 下载失败 (尝试 %d/%d): %v", retry+1, maxRetries, err)

		// 如果不支持断点续传或是加密视频，清理临时文件
		resumeEnabled := h.getConfig() != nil && h.getConfig().DownloadResumeEnabled
		if task.DecryptorPrefix != "" || !resumeEnabled {
			os.Remove(filePath + ".tmp")
		}
	}

	// 记录最终失败的详细错误
	utils.LogDownloadError(task.ID, task.Title, task.GetAuthor(), task.URL, lastErr, maxRetries)
	return fmt.Errorf("下载失败（已重试 %d 次）: %v", maxRetries, lastErr)
}

// downloadVideoOnce 执行一次下载尝试（支持断点续传）
func (h *BatchHandler) downloadVideoOnce(ctx context.Context, task *BatchTask, filePath string, taskIdx int) error {
	tmpPath := filePath + ".tmp"
	
	// 判断是否需要解密：优先使用 key（新方式），其次使用 decryptorPrefix（旧方式）
	needDecrypt := task.Key != "" || (task.DecryptorPrefix != "" && task.PrefixLen > 0)

	// 断点续传：检查已下载的部分（仅非加密视频支持）
	var resumeOffset int64 = 0
	resumeEnabled := h.getConfig() != nil && h.getConfig().DownloadResumeEnabled
	if !needDecrypt && resumeEnabled {
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
		resumeEnabled := h.getConfig() != nil && h.getConfig().DownloadResumeEnabled
		if !resumeEnabled || needDecrypt {
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

// saveDownloadRecord 保存下载记录到数据库
func (h *BatchHandler) saveDownloadRecord(task *BatchTask, filePath string, status string) {
	// 检查CSV中是否已存在记录（避免重复记录）
	if h.csvManager != nil {
		if exists, err := h.csvManager.RecordExists(task.ID); err == nil && exists {
			utils.Info("📝 [下载记录] 记录已存在，跳过保存: %s - %s", task.Title, task.GetAuthor())
			return
		}
	}

	// 获取文件大小
	var fileSize int64 = 0
	if stat, err := os.Stat(filePath); err == nil {
		fileSize = stat.Size()
	}

	// 解析时长字符串为毫秒 (格式: "00:22" 或 "1:23:45")
	duration := parseDurationToMs(task.Duration)

	// 尝试从浏览记录获取更多信息（分辨率、封面等）
	resolution := task.Resolution
	coverURL := task.Cover
	if resolution == "" || coverURL == "" {
		browseRepo := database.NewBrowseHistoryRepository()
		if browseRecord, err := browseRepo.GetByID(task.ID); err == nil && browseRecord != nil {
			if resolution == "" {
				resolution = browseRecord.Resolution
			}
			if coverURL == "" {
				coverURL = browseRecord.CoverURL
			}
			// 如果时长为0，也从浏览记录获取
			if duration == 0 {
				duration = browseRecord.Duration
			}
		}
	}

	// 创建下载记录
	// 使用格式化后的文件名作为标题，确保与实际文件名一致
	cleanTitle := utils.CleanFilename(task.Title)
	record := &database.DownloadRecord{
		ID:           task.ID,
		VideoID:      task.ID,
		Title:        cleanTitle,
		Author:       task.GetAuthor(),
		CoverURL:     coverURL,
		Duration:     duration,
		FileSize:     fileSize,
		FilePath:     filePath,
		Format:       "mp4",
		Resolution:   resolution,
		Status:       status,
		DownloadTime: time.Now(),
	}

	// 保存到数据库
	repo := database.NewDownloadRecordRepository()
	if err := repo.Create(record); err != nil {
		// 如果是重复记录，尝试更新
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			if updateErr := repo.Update(record); updateErr != nil {
				utils.Warn("更新下载记录失败: %v", updateErr)
			}
		} else {
			utils.Warn("保存下载记录失败: %v", err)
		}
	} else {
		utils.Info("📝 [下载记录] 已保存: %s - %s", task.Title, task.GetAuthor())
	}

	// 保存到CSV文件
	if h.csvManager != nil {
		// 格式化文件大小为字符串
		fileSizeStr := fmt.Sprintf("%.2f MB", float64(fileSize)/(1024*1024))

		// 格式化时长为字符串（从毫秒转换为 HH:MM:SS 或 MM:SS）
		durationStr := ""
		if duration > 0 {
			totalSeconds := duration / 1000
			hours := totalSeconds / 3600
			minutes := (totalSeconds % 3600) / 60
			secs := totalSeconds % 60
			if hours > 0 {
				durationStr = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
			} else {
				durationStr = fmt.Sprintf("%02d:%02d", minutes, secs)
			}
		}

		// 创建CSV记录
		// 使用任务中的PageSource，如果没有则默认为"batch"
		pageSource := task.PageSource
		if pageSource == "" {
			pageSource = "batch" // 默认标记为批量下载
		}

		csvRecord := &models.VideoDownloadRecord{
			ID:            task.ID,
			Title:         task.Title,
			Author:        task.GetAuthor(),
			AuthorType:    "",
			OfficialName:  "",
			URL:           task.URL,
			PageURL:       "",
			FileSize:      fileSizeStr,
			Duration:      durationStr,
			PlayCount:     task.PlayCount,    // 使用任务中的播放量
			LikeCount:     task.LikeCount,    // 使用任务中的点赞数
			CommentCount:  task.CommentCount, // 使用任务中的评论数
			FavCount:      task.FavCount,     // 使用任务中的收藏数
			ForwardCount:  task.ForwardCount, // 使用任务中的转发数
			CreateTime:    task.CreateTime,   // 使用任务中的创建时间
			IPRegion:      task.IPRegion,     // 使用任务中的IP所在地
			DownloadAt:    time.Now(),
			PageSource:    pageSource, // 使用实际的页面来源
			SearchKeyword: "",
		}

		// 保存到CSV
		if err := h.csvManager.AddRecord(csvRecord); err != nil {
			utils.Warn("保存CSV记录失败: %v", err)
		} else {
			utils.Info("📄 [CSV记录] 已保存: %s - %s", task.Title, task.GetAuthor())
		}
	}
}

// parseDurationToMs 解析时长字符串为毫秒
// 支持格式: "00:22", "1:23", "1:23:45"
func parseDurationToMs(duration string) int64 {
	if duration == "" {
		return 0
	}

	parts := strings.Split(duration, ":")
	var totalSeconds int64 = 0

	switch len(parts) {
	case 2: // MM:SS
		minutes, _ := strconv.ParseInt(parts[0], 10, 64)
		seconds, _ := strconv.ParseInt(parts[1], 10, 64)
		totalSeconds = minutes*60 + seconds
	case 3: // HH:MM:SS
		hours, _ := strconv.ParseInt(parts[0], 10, 64)
		minutes, _ := strconv.ParseInt(parts[1], 10, 64)
		seconds, _ := strconv.ParseInt(parts[2], 10, 64)
		totalSeconds = hours*3600 + minutes*60 + seconds
	}

	return totalSeconds * 1000 // 转换为毫秒
}

// HandleBatchProgress 处理批量下载进度查询请求
func (h *BatchHandler) HandleBatchProgress(Conn *SunnyNet.HttpConn) bool {
	path := Conn.Request.URL.Path
	if path != "/__wx_channels_api/batch_progress" {
		return false
	}

	// 处理 CORS 预检请求
	if Conn.Request.Method == "OPTIONS" {
		h.sendSuccessResponse(Conn, map[string]interface{}{"message": "OK"})
		return true
	}

	// 授权校验
	if h.getConfig() != nil && h.getConfig().SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.getConfig().SecretToken {
			h.sendErrorResponse(Conn, fmt.Errorf("unauthorized"))
			return true
		}
	}

	h.mu.RLock()
	total := len(h.tasks)
	done, failed, running := 0, 0, 0
	var downloadingTasks []map[string]interface{}
	var allTasks []map[string]interface{}

	for _, t := range h.tasks {
		taskInfo := map[string]interface{}{
			"id":           t.ID,
			"title":        t.Title,
			"authorName":   t.GetAuthor(),
			"status":       t.Status,
			"progress":     t.Progress,
			"downloadedMB": t.DownloadedMB,
			"totalMB":      t.TotalMB,
			"error":        t.Error,
		}
		allTasks = append(allTasks, taskInfo)

		switch t.Status {
		case "done":
			done++
		case "failed":
			failed++
		case "downloading":
			running++
			downloadingTasks = append(downloadingTasks, taskInfo)
		}
	}
	h.mu.RUnlock()

	response := map[string]interface{}{
		"total":   total,
		"done":    done,
		"failed":  failed,
		"running": running,
		"tasks":   allTasks,
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

	// 处理 CORS 预检请求
	if Conn.Request.Method == "OPTIONS" {
		h.sendSuccessResponse(Conn, map[string]interface{}{"message": "OK"})
		return true
	}

	// 授权校验
	if h.getConfig() != nil && h.getConfig().SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.getConfig().SecretToken {
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

	// 处理 CORS 预检请求
	if Conn.Request.Method == "OPTIONS" {
		h.sendSuccessResponse(Conn, map[string]interface{}{"message": "OK"})
		return true
	}

	// 授权校验
	if h.getConfig() != nil && h.getConfig().SecretToken != "" {
		if Conn.Request.Header.Get("X-Local-Auth") != h.getConfig().SecretToken {
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
	// 获取下载目录
	downloadsDir, err := h.getDownloadsDir()
	if err != nil {
		h.sendErrorResponse(Conn, err)
		return true
	}
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

	// CORS - 允许所有来源（因为是本地服务）
	origin := Conn.Request.Header.Get("Origin")
	if origin != "" {
		headers.Set("Access-Control-Allow-Origin", origin)
		headers.Set("Vary", "Origin")
		headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
		headers.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		headers.Set("Access-Control-Max-Age", "86400") // 24小时
	}

	Conn.StopRequest(200, string(responseBytes), headers)
}

// sendErrorResponse 发送错误响应
func (h *BatchHandler) sendErrorResponse(Conn *SunnyNet.HttpConn, err error) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Content-Type-Options", "nosniff")

	// CORS - 允许所有来源（因为是本地服务）
	origin := Conn.Request.Header.Get("Origin")
	if origin != "" {
		headers.Set("Access-Control-Allow-Origin", origin)
		headers.Set("Vary", "Origin")
		headers.Set("Access-Control-Allow-Headers", "Content-Type, X-Local-Auth")
		headers.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		headers.Set("Access-Control-Max-Age", "86400") // 24小时
	}

	errorMsg := fmt.Sprintf(`{"success":false,"error":"%s"}`, strings.ReplaceAll(err.Error(), `"`, `\"`))
	Conn.StopRequest(500, errorMsg, headers)
}
