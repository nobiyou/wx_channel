package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wx_channel/internal/database"
	"wx_channel/internal/utils"
)

// ChunkedDownloader 处理支持分片的大文件下载
type ChunkedDownloader struct {
	queueService *QueueService
	settings     *database.SettingsRepository
	client       *http.Client
	downloadDir  string

	mu            sync.RWMutex
	activeItems   map[string]*DownloadState
	progressChan  chan ProgressUpdate
	ctx           context.Context
	cancel        context.CancelFunc
	maxConcurrent int
	maxRetries    int
}

// DownloadState 跟踪活动下载的状态
type DownloadState struct {
	QueueItem      *database.QueueItem
	CurrentChunk   int
	ChunkProgress  int64 // bytes downloaded in current chunk
	LastUpdateTime time.Time
	BytesPerSecond int64
	IsPaused       bool
	CancelFunc     context.CancelFunc
}

// ProgressUpdate 表示下载进度更新
type ProgressUpdate struct {
	QueueID         string `json:"queueId"`
	DownloadedSize  int64  `json:"downloadedSize"`
	TotalSize       int64  `json:"totalSize"`
	ChunksCompleted int    `json:"chunksCompleted"`
	ChunksTotal     int    `json:"chunksTotal"`
	Speed           int64  `json:"speed"`
	Status          string `json:"status"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
}

// NewChunkedDownloader 创建一个新的 ChunkedDownloader
func NewChunkedDownloader(queueService *QueueService) *ChunkedDownloader {
	ctx, cancel := context.WithCancel(context.Background())
	settingsRepo := database.NewSettingsRepository()

	// 加载设置
	settings, err := settingsRepo.Load()
	if err != nil {
		settings = database.DefaultSettings()
	}

	return &ChunkedDownloader{
		queueService:  queueService,
		settings:      settingsRepo,
		client:        &http.Client{Timeout: 0}, // No timeout for large downloads
		downloadDir:   settings.DownloadDir,
		activeItems:   make(map[string]*DownloadState),
		progressChan:  make(chan ProgressUpdate, 100),
		ctx:           ctx,
		cancel:        cancel,
		maxConcurrent: settings.ConcurrentLimit,
		maxRetries:    settings.MaxRetries,
	}
}

// ProgressChannel 返回进度更新通道
func (d *ChunkedDownloader) ProgressChannel() <-chan ProgressUpdate {
	return d.progressChan
}

// StartDownload 开始下载队列项目
func (d *ChunkedDownloader) StartDownload(item *database.QueueItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 检查是否已在下载
	if _, exists := d.activeItems[item.ID]; exists {
		return fmt.Errorf("download already in progress for item: %s", item.ID)
	}

	// 创建下载上下文
	ctx, cancel := context.WithCancel(d.ctx)

	state := &DownloadState{
		QueueItem:      item,
		CurrentChunk:   item.ChunksCompleted, // Resume from last completed chunk
		LastUpdateTime: time.Now(),
		CancelFunc:     cancel,
	}

	d.activeItems[item.ID] = state

	// 在 goroutine 中开始下载
	go d.downloadItem(ctx, state)

	return nil
}

// downloadItem 执行实际下载
func (d *ChunkedDownloader) downloadItem(ctx context.Context, state *DownloadState) {
	item := state.QueueItem

	// 标记为正在下载
	if err := d.queueService.StartDownload(item.ID); err != nil {
		d.handleError(item.ID, fmt.Errorf("failed to start download: %w", err))
		return
	}

	// 准备下载目录
	downloadPath, err := d.prepareDownloadPath(item)
	if err != nil {
		d.handleError(item.ID, fmt.Errorf("failed to prepare download path: %w", err))
		return
	}

	// 下载分片
	err = d.downloadChunks(ctx, state, downloadPath)
	if err != nil {
		// 检查是否被取消/暂停
		if ctx.Err() != nil {
			return
		}
		d.handleError(item.ID, err)
		return
	}

	// 验证文件完整性
	if err := d.verifyFileIntegrity(downloadPath, item.TotalSize); err != nil {
		d.handleError(item.ID, fmt.Errorf("file integrity check failed: %w", err))
		return
	}

	// 标记为完成
	if err := d.queueService.CompleteDownload(item.ID); err != nil {
		d.handleError(item.ID, fmt.Errorf("failed to mark download as completed: %w", err))
		return
	}

	// 发送完成更新
	d.sendProgress(ProgressUpdate{
		QueueID:         item.ID,
		DownloadedSize:  item.TotalSize,
		TotalSize:       item.TotalSize,
		ChunksCompleted: item.ChunksTotal,
		ChunksTotal:     item.ChunksTotal,
		Speed:           0,
		Status:          database.QueueStatusCompleted,
	})

	// 从活动项目中移除
	d.mu.Lock()
	delete(d.activeItems, item.ID)
	d.mu.Unlock()
}

// downloadChunks 下载项目的所有分片（并发下载）
func (d *ChunkedDownloader) downloadChunks(ctx context.Context, state *DownloadState, downloadPath string) error {
	item := state.QueueItem
	chunkSize := item.ChunkSize
	totalChunks := item.ChunksTotal

	// 打开或创建文件
	file, err := os.OpenFile(downloadPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 预分配文件大小
	if err := file.Truncate(item.TotalSize); err != nil {
		return fmt.Errorf("failed to truncate file: %w", err)
	}

	startChunk := state.CurrentChunk
	downloadedSize := int64(startChunk) * chunkSize

	// 获取最大并发数
	concurrentLimit := d.maxConcurrent
	if concurrentLimit <= 0 {
		concurrentLimit = 3 // 默认并发 3
	}

	var wg sync.WaitGroup
	errChan := make(chan error, totalChunks)
	chunkChan := make(chan int, totalChunks)

	// 填充任务通道
	for i := startChunk; i < totalChunks; i++ {
		chunkChan <- i
	}
	close(chunkChan)

	var mu sync.Mutex // 保护共享状态
	lastSpeedCalcTime := time.Now()
	lastDownloadedSize := downloadedSize

	// 启动 worker
	for i := 0; i < concurrentLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chunkIndex := range chunkChan {
				select {
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				default:
				}

				// 检查是否暂停
				d.mu.RLock()
				isPaused := state.IsPaused
				d.mu.RUnlock()
				if isPaused {
					return
				}

				// 计算分片范围
				chunkStart := int64(chunkIndex) * chunkSize
				chunkEnd := chunkStart + chunkSize - 1
				if chunkEnd >= item.TotalSize {
					chunkEnd = item.TotalSize - 1
				}

				// 带重试下载并写入分片
				written, err := d.downloadAndWriteChunkWithRetry(ctx, item.VideoURL, chunkStart, chunkEnd, file)
				if err != nil {
					errChan <- fmt.Errorf("failed to download chunk %d: %w", chunkIndex, err)
					return
				}

				// 更新状态
				mu.Lock()
				downloadedSize += written
				// 这里简单地增加 CurrentChunk，实际上多线程下 Completed 计数才是准确的
				// 但为了兼容，保持每次加 1
				state.CurrentChunk++

				// 计算速度
				now := time.Now()
				elapsed := now.Sub(lastSpeedCalcTime).Seconds()
				if elapsed >= 1.0 {
					bytesDownloaded := downloadedSize - lastDownloadedSize
					state.BytesPerSecond = int64(float64(bytesDownloaded) / elapsed)
					lastSpeedCalcTime = now
					lastDownloadedSize = downloadedSize
				}

				// 复制状态用于后续使用
				currChunk := state.CurrentChunk
				bytesPerSec := state.BytesPerSecond

				mu.Unlock()

				// 更新数据库中的进度（只做一次 DB 调用）
				if err := d.queueService.UpdateProgress(item.ID, downloadedSize, currChunk, bytesPerSec); err != nil {
					utils.Warn("[ChunkedDownloader] Failed to update progress: %v", err)
				}

				// 发送进度更新
				d.sendProgress(ProgressUpdate{
					QueueID:         item.ID,
					DownloadedSize:  downloadedSize,
					TotalSize:       item.TotalSize,
					ChunksCompleted: currChunk,
					ChunksTotal:     totalChunks,
					Speed:           bytesPerSec,
					Status:          database.QueueStatusDownloading,
				})
			}
		}()
	}

	// 等待所有 worker 完成
	wg.Wait()
	close(errChan)

	// 检查是否有错误
	for e := range errChan {
		if e != nil && e != context.Canceled {
			return e
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

// downloadAndWriteChunkWithRetry 带重试逻辑下载并写入单个分片
func (d *ChunkedDownloader) downloadAndWriteChunkWithRetry(ctx context.Context, url string, start, end int64, file *os.File) (int64, error) {
	var lastErr error

	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		if attempt > 0 {
			// 重试前等待（指数退避）
			waitTime := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(waitTime):
			}
		}

		written, err := d.downloadAndWriteChunk(ctx, url, start, end, file)
		if err == nil {
			return written, nil
		}

		lastErr = err
		utils.Warn("[ChunkedDownloader] Chunk download failed (attempt %d/%d): %v", attempt+1, d.maxRetries+1, err)
	}

	return 0, fmt.Errorf("chunk download failed after %d retries: %w", d.maxRetries+1, lastErr)
}

// downloadAndWriteChunk 使用 HTTP Range 请求下载单个分片并直接写入文件缓冲
func (d *ChunkedDownloader) downloadAndWriteChunk(ctx context.Context, url string, start, end int64, file *os.File) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置部分内容的 Range 头
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 接受 200 (完整内容) 和 206 (部分内容)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 使用 io.Copy 代替 io.ReadAll，避免内存暴涨
	// 创建一个专门用于此分片写入的 SectionWriter
	writer := io.NewOffsetWriter(file, start)

	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		return written, fmt.Errorf("failed to write response to file: %w", err)
	}

	return written, nil
}

// downloadChunkWithRetry 带重试逻辑下载单个分片
func (d *ChunkedDownloader) downloadChunkWithRetry(ctx context.Context, url string, start, end int64) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if attempt > 0 {
			// 重试前等待（指数退避）
			waitTime := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(waitTime):
			}
		}

		data, err := d.downloadChunk(ctx, url, start, end)
		if err == nil {
			return data, nil
		}

		lastErr = err
		utils.Warn("[ChunkedDownloader] Chunk download failed (attempt %d/%d): %v", attempt+1, d.maxRetries+1, err)
	}

	return nil, fmt.Errorf("chunk download failed after %d retries: %w", d.maxRetries+1, lastErr)
}

// downloadChunk 使用 HTTP Range 请求下载单个分片
func (d *ChunkedDownloader) downloadChunk(ctx context.Context, url string, start, end int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置部分内容的 Range 头
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 接受 200 (完整内容) 和 206 (部分内容)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return data, nil
}

// prepareDownloadPath 准备项目的下载路径
func (d *ChunkedDownloader) prepareDownloadPath(item *database.QueueItem) (string, error) {
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		return "", err
	}

	// 创建作者文件夹
	authorFolder := utils.CleanFolderName(item.Author)
	downloadDir := filepath.Join(baseDir, d.downloadDir, authorFolder)

	if err := utils.EnsureDir(downloadDir); err != nil {
		return "", fmt.Errorf("failed to create download directory: %w", err)
	}

	// 清理文件名
	filename := utils.CleanFilename(item.Title)
	filename = utils.EnsureExtension(filename, ".mp4")

	return filepath.Join(downloadDir, filename), nil
}

// verifyFileIntegrity 验证下载的文件大小是否与预期大小匹配
func (d *ChunkedDownloader) verifyFileIntegrity(filePath string, expectedSize int64) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	actualSize := fileInfo.Size()
	if actualSize != expectedSize {
		return fmt.Errorf("file size mismatch: expected %d bytes, got %d bytes", expectedSize, actualSize)
	}

	return nil
}

// handleError 处理下载错误
func (d *ChunkedDownloader) handleError(itemID string, err error) {
	utils.Error("[ChunkedDownloader] Download error for %s: %v", itemID, err)

	// 获取项目详细信息以进行日志记录
	if item, getErr := d.queueService.GetByID(itemID); getErr == nil && item != nil {
		utils.LogDownloadError(item.ID, item.Title, item.Author, item.VideoURL, err, d.maxRetries)
	}

	// 标记为失败
	if markErr := d.queueService.FailDownload(itemID, err.Error()); markErr != nil {
		utils.Error("[ChunkedDownloader] Failed to mark download as failed: %v", markErr)
	}

	// 发送错误更新
	d.sendProgress(ProgressUpdate{
		QueueID:      itemID,
		Status:       database.QueueStatusFailed,
		ErrorMessage: err.Error(),
	})

	// 从活动项目中移除
	d.mu.Lock()
	delete(d.activeItems, itemID)
	d.mu.Unlock()
}

// sendProgress 发送进度更新到通道
func (d *ChunkedDownloader) sendProgress(update ProgressUpdate) {
	select {
	case d.progressChan <- update:
	default:
		// 通道已满，跳过更新
	}
}

// PauseDownload 暂停活动下载
func (d *ChunkedDownloader) PauseDownload(itemID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, exists := d.activeItems[itemID]
	if !exists {
		return fmt.Errorf("no active download for item: %s", itemID)
	}

	state.IsPaused = true
	state.CancelFunc()

	// 更新数据库中的状态
	if err := d.queueService.Pause(itemID); err != nil {
		return err
	}

	delete(d.activeItems, itemID)
	return nil
}

// ResumeDownload 恢复暂停的下载
func (d *ChunkedDownloader) ResumeDownload(itemID string) error {
	// 从队列获取项目
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", itemID)
	}

	// 在队列服务中恢复
	if err := d.queueService.Resume(itemID); err != nil {
		return err
	}

	// 开始下载
	return d.StartDownload(item)
}

// CancelDownload 取消活动下载
func (d *ChunkedDownloader) CancelDownload(itemID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, exists := d.activeItems[itemID]
	if !exists {
		return fmt.Errorf("no active download for item: %s", itemID)
	}

	state.CancelFunc()
	delete(d.activeItems, itemID)

	return nil
}

// GetActiveDownloads 返回活动下载 ID 列表
func (d *ChunkedDownloader) GetActiveDownloads() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ids := make([]string, 0, len(d.activeItems))
	for id := range d.activeItems {
		ids = append(ids, id)
	}
	return ids
}

// GetDownloadState 返回活动下载的状态
func (d *ChunkedDownloader) GetDownloadState(itemID string) (*DownloadState, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state, exists := d.activeItems[itemID]
	return state, exists
}

// Stop 停止下载器并取消所有活动下载
func (d *ChunkedDownloader) Stop() {
	d.cancel()

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, state := range d.activeItems {
		state.CancelFunc()
	}
	d.activeItems = make(map[string]*DownloadState)

	close(d.progressChan)
}

// GetResumePosition 计算恢复的字节位置
// 基于已完成的分片: 位置 = 已完成分片数 * 分片大小
func GetResumePosition(chunksCompleted int, chunkSize int64) int64 {
	return int64(chunksCompleted) * chunkSize
}

// ResumeInfo 包含恢复下载所需的信息
type ResumeInfo struct {
	QueueID         string `json:"queueId"`
	ChunksCompleted int    `json:"chunksCompleted"`
	ChunksTotal     int    `json:"chunksTotal"`
	DownloadedSize  int64  `json:"downloadedSize"`
	TotalSize       int64  `json:"totalSize"`
	ChunkSize       int64  `json:"chunkSize"`
	ResumePosition  int64  `json:"resumePosition"`
}

// GetResumeInfo 获取暂停下载的恢复信息
func (d *ChunkedDownloader) GetResumeInfo(itemID string) (*ResumeInfo, error) {
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("queue item not found: %s", itemID)
	}

	// 只能获取暂停或失败项目的恢复信息
	if item.Status != database.QueueStatusPaused && item.Status != database.QueueStatusFailed {
		return nil, fmt.Errorf("item is not paused or failed, current status: %s", item.Status)
	}

	resumePosition := GetResumePosition(item.ChunksCompleted, item.ChunkSize)

	return &ResumeInfo{
		QueueID:         item.ID,
		ChunksCompleted: item.ChunksCompleted,
		ChunksTotal:     item.ChunksTotal,
		DownloadedSize:  item.DownloadedSize,
		TotalSize:       item.TotalSize,
		ChunkSize:       item.ChunkSize,
		ResumePosition:  resumePosition,
	}, nil
}

// CanResume 检查下载是否可以恢复
func (d *ChunkedDownloader) CanResume(itemID string) (bool, error) {
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}

	// 可以恢复有部分进度的暂停或失败项目
	if item.Status == database.QueueStatusPaused || item.Status == database.QueueStatusFailed {
		return item.ChunksCompleted > 0 && item.ChunksCompleted < item.ChunksTotal, nil
	}

	return false, nil
}

// SaveProgress 显式保存当前进度到数据库
// 这在下载期间和暂停时定期调用
func (d *ChunkedDownloader) SaveProgress(itemID string) error {
	d.mu.RLock()
	state, exists := d.activeItems[itemID]
	d.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no active download for item: %s", itemID)
	}

	downloadedSize := int64(state.CurrentChunk) * state.QueueItem.ChunkSize
	if state.ChunkProgress > 0 {
		downloadedSize += state.ChunkProgress
	}

	return d.queueService.UpdateProgress(
		itemID,
		downloadedSize,
		state.CurrentChunk,
		state.BytesPerSecond,
	)
}

// GetPausedDownloads 返回所有可恢复的暂停下载
func (d *ChunkedDownloader) GetPausedDownloads() ([]database.QueueItem, error) {
	return d.queueService.GetByStatus(database.QueueStatusPaused)
}

// ResumeAllPaused 恢复所有暂停的下载
func (d *ChunkedDownloader) ResumeAllPaused() error {
	paused, err := d.GetPausedDownloads()
	if err != nil {
		return err
	}

	for _, item := range paused {
		itemCopy := item // Create a copy to avoid closure issues
		if err := d.ResumeDownload(itemCopy.ID); err != nil {
			utils.Warn("[ChunkedDownloader] Failed to resume download %s: %v", itemCopy.ID, err)
		}
	}

	return nil
}

// FileIntegrityResult 包含文件完整性检查的结果
type FileIntegrityResult struct {
	IsValid      bool   `json:"isValid"`
	ExpectedSize int64  `json:"expectedSize"`
	ActualSize   int64  `json:"actualSize"`
	FilePath     string `json:"filePath"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// VerifyDownloadedFile 验证下载文件的完整性
// 这检查实际文件大小是否与预期大小匹配
func (d *ChunkedDownloader) VerifyDownloadedFile(itemID string) (*FileIntegrityResult, error) {
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("queue item not found: %s", itemID)
	}

	// Get the file path
	filePath, err := d.prepareDownloadPath(item)
	if err != nil {
		return &FileIntegrityResult{
			IsValid:      false,
			ExpectedSize: item.TotalSize,
			ErrorMessage: fmt.Sprintf("failed to get file path: %v", err),
		}, nil
	}

	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileIntegrityResult{
				IsValid:      false,
				ExpectedSize: item.TotalSize,
				FilePath:     filePath,
				ErrorMessage: "file does not exist",
			}, nil
		}
		return &FileIntegrityResult{
			IsValid:      false,
			ExpectedSize: item.TotalSize,
			FilePath:     filePath,
			ErrorMessage: fmt.Sprintf("failed to stat file: %v", err),
		}, nil
	}

	actualSize := fileInfo.Size()
	isValid := actualSize == item.TotalSize

	result := &FileIntegrityResult{
		IsValid:      isValid,
		ExpectedSize: item.TotalSize,
		ActualSize:   actualSize,
		FilePath:     filePath,
	}

	if !isValid {
		result.ErrorMessage = fmt.Sprintf("file size mismatch: expected %d bytes, got %d bytes", item.TotalSize, actualSize)
	}

	return result, nil
}

// VerifyFileSize 是一个独立函数，用于验证文件大小是否匹配预期
func VerifyFileSize(filePath string, expectedSize int64) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	actualSize := fileInfo.Size()
	if actualSize != expectedSize {
		return fmt.Errorf("file size mismatch: expected %d bytes, got %d bytes", expectedSize, actualSize)
	}

	return nil
}

// VerifyAllCompletedDownloads 验证所有已完成的下载
func (d *ChunkedDownloader) VerifyAllCompletedDownloads() ([]FileIntegrityResult, error) {
	completed, err := d.queueService.GetByStatus(database.QueueStatusCompleted)
	if err != nil {
		return nil, err
	}

	results := make([]FileIntegrityResult, 0, len(completed))
	for _, item := range completed {
		result, err := d.VerifyDownloadedFile(item.ID)
		if err != nil {
			results = append(results, FileIntegrityResult{
				IsValid:      false,
				ExpectedSize: item.TotalSize,
				ErrorMessage: err.Error(),
			})
			continue
		}
		results = append(results, *result)
	}

	return results, nil
}

// RetryConfig 包含重试配置
type RetryConfig struct {
	MaxRetries    int           `json:"maxRetries"`
	InitialDelay  time.Duration `json:"initialDelay"`
	MaxDelay      time.Duration `json:"maxDelay"`
	BackoffFactor float64       `json:"backoffFactor"`
}

// DefaultRetryConfig 返回默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:    3,
		InitialDelay:  time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
	}
}

// RetryResult 包含重试操作的结果
type RetryResult struct {
	Success   bool   `json:"success"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"lastError,omitempty"`
	TotalTime int64  `json:"totalTimeMs"`
}

// downloadChunkWithRetryTracked 带重试下载分片并返回详细结果
func (d *ChunkedDownloader) downloadChunkWithRetryTracked(ctx context.Context, url string, start, end int64, config *RetryConfig) ([]byte, *RetryResult) {
	if config == nil {
		config = DefaultRetryConfig()
	}

	result := &RetryResult{
		Success:  false,
		Attempts: 0,
	}

	startTime := time.Now()
	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			result.LastError = ctx.Err().Error()
			result.TotalTime = time.Since(startTime).Milliseconds()
			return nil, result
		default:
		}

		result.Attempts = attempt + 1

		if attempt > 0 {
			// Wait before retry with exponential backoff
			select {
			case <-ctx.Done():
				result.LastError = ctx.Err().Error()
				result.TotalTime = time.Since(startTime).Milliseconds()
				return nil, result
			case <-time.After(delay):
			}

			// 计算下一次退避延迟
			delay = time.Duration(float64(delay) * config.BackoffFactor)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}

		data, err := d.downloadChunk(ctx, url, start, end)
		if err == nil {
			result.Success = true
			result.TotalTime = time.Since(startTime).Milliseconds()
			return data, result
		}

		lastErr = err
		utils.Warn("[ChunkedDownloader] Chunk download failed (attempt %d/%d): %v", attempt+1, config.MaxRetries+1, err)
	}

	result.LastError = lastErr.Error()
	result.TotalTime = time.Since(startTime).Milliseconds()
	return nil, result
}

// RetryFailedDownload 从头开始或上一个检查点重试失败的下载
func (d *ChunkedDownloader) RetryFailedDownload(itemID string) error {
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", itemID)
	}

	// Can only retry failed items
	if item.Status != database.QueueStatusFailed {
		return fmt.Errorf("can only retry failed items, current status: %s", item.Status)
	}

	// Check retry count
	settings, err := d.settings.Load()
	if err != nil {
		settings = database.DefaultSettings()
	}

	if item.RetryCount >= settings.MaxRetries {
		return fmt.Errorf("max retries (%d) exceeded for item: %s", settings.MaxRetries, itemID)
	}

	// Increment retry count
	if err := d.queueService.IncrementRetryCount(itemID); err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	// Reset status to pending
	if err := d.queueService.UpdateStatus(itemID, database.QueueStatusPending); err != nil {
		return fmt.Errorf("failed to reset status: %w", err)
	}

	// Start download (will resume from last checkpoint)
	return d.StartDownload(item)
}

// GetRetryCount returns the current retry count for an item
func (d *ChunkedDownloader) GetRetryCount(itemID string) (int, error) {
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return 0, err
	}
	if item == nil {
		return 0, fmt.Errorf("queue item not found: %s", itemID)
	}
	return item.RetryCount, nil
}

// CanRetry checks if an item can be retried
func (d *ChunkedDownloader) CanRetry(itemID string) (bool, error) {
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}

	// Can only retry failed items
	if item.Status != database.QueueStatusFailed {
		return false, nil
	}

	// Check retry count
	settings, err := d.settings.Load()
	if err != nil {
		settings = database.DefaultSettings()
	}

	return item.RetryCount < settings.MaxRetries, nil
}

// ResetRetryCount resets the retry count for an item
func (d *ChunkedDownloader) ResetRetryCount(itemID string) error {
	return d.queueService.ResetRetryCount(itemID)
}
