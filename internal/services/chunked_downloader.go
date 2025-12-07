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

// ChunkedDownloader handles large file downloads with chunking support
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

// DownloadState tracks the state of an active download
type DownloadState struct {
	QueueItem       *database.QueueItem
	CurrentChunk    int
	ChunkProgress   int64 // bytes downloaded in current chunk
	LastUpdateTime  time.Time
	BytesPerSecond  int64
	IsPaused        bool
	CancelFunc      context.CancelFunc
}

// ProgressUpdate represents a download progress update
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

// NewChunkedDownloader creates a new ChunkedDownloader
func NewChunkedDownloader(queueService *QueueService) *ChunkedDownloader {
	ctx, cancel := context.WithCancel(context.Background())
	settingsRepo := database.NewSettingsRepository()

	// Load settings
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

// ProgressChannel returns the channel for progress updates
func (d *ChunkedDownloader) ProgressChannel() <-chan ProgressUpdate {
	return d.progressChan
}


// StartDownload starts downloading a queue item
func (d *ChunkedDownloader) StartDownload(item *database.QueueItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if already downloading
	if _, exists := d.activeItems[item.ID]; exists {
		return fmt.Errorf("download already in progress for item: %s", item.ID)
	}

	// Create download context
	ctx, cancel := context.WithCancel(d.ctx)

	state := &DownloadState{
		QueueItem:      item,
		CurrentChunk:   item.ChunksCompleted, // Resume from last completed chunk
		LastUpdateTime: time.Now(),
		CancelFunc:     cancel,
	}

	d.activeItems[item.ID] = state

	// Start download in goroutine
	go d.downloadItem(ctx, state)

	return nil
}

// downloadItem performs the actual download
func (d *ChunkedDownloader) downloadItem(ctx context.Context, state *DownloadState) {
	item := state.QueueItem

	// Mark as downloading
	if err := d.queueService.StartDownload(item.ID); err != nil {
		d.handleError(item.ID, fmt.Errorf("failed to start download: %w", err))
		return
	}

	// Prepare download directory
	downloadPath, err := d.prepareDownloadPath(item)
	if err != nil {
		d.handleError(item.ID, fmt.Errorf("failed to prepare download path: %w", err))
		return
	}

	// Download chunks
	err = d.downloadChunks(ctx, state, downloadPath)
	if err != nil {
		// Check if it was cancelled/paused
		if ctx.Err() != nil {
			return
		}
		d.handleError(item.ID, err)
		return
	}

	// Verify file integrity
	if err := d.verifyFileIntegrity(downloadPath, item.TotalSize); err != nil {
		d.handleError(item.ID, fmt.Errorf("file integrity check failed: %w", err))
		return
	}

	// Mark as completed
	if err := d.queueService.CompleteDownload(item.ID); err != nil {
		d.handleError(item.ID, fmt.Errorf("failed to mark download as completed: %w", err))
		return
	}

	// Send completion update
	d.sendProgress(ProgressUpdate{
		QueueID:         item.ID,
		DownloadedSize:  item.TotalSize,
		TotalSize:       item.TotalSize,
		ChunksCompleted: item.ChunksTotal,
		ChunksTotal:     item.ChunksTotal,
		Speed:           0,
		Status:          database.QueueStatusCompleted,
	})

	// Remove from active items
	d.mu.Lock()
	delete(d.activeItems, item.ID)
	d.mu.Unlock()
}

// downloadChunks downloads all chunks for an item
func (d *ChunkedDownloader) downloadChunks(ctx context.Context, state *DownloadState, downloadPath string) error {
	item := state.QueueItem
	chunkSize := item.ChunkSize
	totalChunks := item.ChunksTotal

	// Open or create the file
	file, err := os.OpenFile(downloadPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Resume from last completed chunk
	startChunk := state.CurrentChunk
	startOffset := int64(startChunk) * chunkSize

	// Seek to the correct position
	if _, err := file.Seek(startOffset, 0); err != nil {
		return fmt.Errorf("failed to seek to position: %w", err)
	}

	downloadedSize := startOffset
	lastSpeedCalcTime := time.Now()
	lastDownloadedSize := downloadedSize

	for chunkIndex := startChunk; chunkIndex < totalChunks; chunkIndex++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if paused
		d.mu.RLock()
		isPaused := state.IsPaused
		d.mu.RUnlock()
		if isPaused {
			return nil
		}

		// Calculate chunk range
		chunkStart := int64(chunkIndex) * chunkSize
		chunkEnd := chunkStart + chunkSize - 1
		if chunkEnd >= item.TotalSize {
			chunkEnd = item.TotalSize - 1
		}

		// Download chunk with retry
		chunkBytes, err := d.downloadChunkWithRetry(ctx, item.VideoURL, chunkStart, chunkEnd)
		if err != nil {
			return fmt.Errorf("failed to download chunk %d: %w", chunkIndex, err)
		}

		// Write chunk to file
		if _, err := file.Write(chunkBytes); err != nil {
			return fmt.Errorf("failed to write chunk %d: %w", chunkIndex, err)
		}

		downloadedSize += int64(len(chunkBytes))
		state.CurrentChunk = chunkIndex + 1

		// Calculate speed
		now := time.Now()
		elapsed := now.Sub(lastSpeedCalcTime).Seconds()
		if elapsed >= 1.0 {
			bytesDownloaded := downloadedSize - lastDownloadedSize
			state.BytesPerSecond = int64(float64(bytesDownloaded) / elapsed)
			lastSpeedCalcTime = now
			lastDownloadedSize = downloadedSize
		}

		// Update progress in database
		if err := d.queueService.UpdateProgress(item.ID, downloadedSize, state.CurrentChunk, state.BytesPerSecond); err != nil {
			utils.Warn("[ChunkedDownloader] Failed to update progress: %v", err)
		}

		// Send progress update
		d.sendProgress(ProgressUpdate{
			QueueID:         item.ID,
			DownloadedSize:  downloadedSize,
			TotalSize:       item.TotalSize,
			ChunksCompleted: state.CurrentChunk,
			ChunksTotal:     totalChunks,
			Speed:           state.BytesPerSecond,
			Status:          database.QueueStatusDownloading,
		})
	}

	return nil
}


// downloadChunkWithRetry downloads a single chunk with retry logic
func (d *ChunkedDownloader) downloadChunkWithRetry(ctx context.Context, url string, start, end int64) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if attempt > 0 {
			// Wait before retry with exponential backoff
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

// downloadChunk downloads a single chunk using HTTP Range request
func (d *ChunkedDownloader) downloadChunk(ctx context.Context, url string, start, end int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Range header for partial content
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Accept both 200 (full content) and 206 (partial content)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return data, nil
}

// prepareDownloadPath prepares the download path for an item
func (d *ChunkedDownloader) prepareDownloadPath(item *database.QueueItem) (string, error) {
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		return "", err
	}

	// Create author folder
	authorFolder := utils.CleanFolderName(item.Author)
	downloadDir := filepath.Join(baseDir, d.downloadDir, authorFolder)

	if err := utils.EnsureDir(downloadDir); err != nil {
		return "", fmt.Errorf("failed to create download directory: %w", err)
	}

	// Clean filename
	filename := utils.CleanFilename(item.Title)
	filename = utils.EnsureExtension(filename, ".mp4")

	return filepath.Join(downloadDir, filename), nil
}

// verifyFileIntegrity verifies the downloaded file size matches expected size
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

// handleError handles download errors
func (d *ChunkedDownloader) handleError(itemID string, err error) {
	utils.Error("[ChunkedDownloader] Download error for %s: %v", itemID, err)

	// Get item details for logging
	if item, getErr := d.queueService.GetByID(itemID); getErr == nil && item != nil {
		utils.LogDownloadError(item.ID, item.Title, item.Author, item.VideoURL, err, d.maxRetries)
	}
	
	// Mark as failed
	if markErr := d.queueService.FailDownload(itemID, err.Error()); markErr != nil {
		utils.Error("[ChunkedDownloader] Failed to mark download as failed: %v", markErr)
	}

	// Send error update
	d.sendProgress(ProgressUpdate{
		QueueID:      itemID,
		Status:       database.QueueStatusFailed,
		ErrorMessage: err.Error(),
	})

	// Remove from active items
	d.mu.Lock()
	delete(d.activeItems, itemID)
	d.mu.Unlock()
}

// sendProgress sends a progress update to the channel
func (d *ChunkedDownloader) sendProgress(update ProgressUpdate) {
	select {
	case d.progressChan <- update:
	default:
		// Channel full, skip update
	}
}

// PauseDownload pauses an active download
func (d *ChunkedDownloader) PauseDownload(itemID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, exists := d.activeItems[itemID]
	if !exists {
		return fmt.Errorf("no active download for item: %s", itemID)
	}

	state.IsPaused = true
	state.CancelFunc()

	// Update status in database
	if err := d.queueService.Pause(itemID); err != nil {
		return err
	}

	delete(d.activeItems, itemID)
	return nil
}

// ResumeDownload resumes a paused download
func (d *ChunkedDownloader) ResumeDownload(itemID string) error {
	// Get the item from queue
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", itemID)
	}

	// Resume in queue service
	if err := d.queueService.Resume(itemID); err != nil {
		return err
	}

	// Start download
	return d.StartDownload(item)
}

// CancelDownload cancels an active download
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

// GetActiveDownloads returns the list of active download IDs
func (d *ChunkedDownloader) GetActiveDownloads() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ids := make([]string, 0, len(d.activeItems))
	for id := range d.activeItems {
		ids = append(ids, id)
	}
	return ids
}

// GetDownloadState returns the state of an active download
func (d *ChunkedDownloader) GetDownloadState(itemID string) (*DownloadState, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state, exists := d.activeItems[itemID]
	return state, exists
}

// Stop stops the downloader and cancels all active downloads
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

// GetResumePosition calculates the byte position to resume from
// Based on completed chunks: position = chunksCompleted * chunkSize
func GetResumePosition(chunksCompleted int, chunkSize int64) int64 {
	return int64(chunksCompleted) * chunkSize
}


// ResumeInfo contains information needed to resume a download
type ResumeInfo struct {
	QueueID         string `json:"queueId"`
	ChunksCompleted int    `json:"chunksCompleted"`
	ChunksTotal     int    `json:"chunksTotal"`
	DownloadedSize  int64  `json:"downloadedSize"`
	TotalSize       int64  `json:"totalSize"`
	ChunkSize       int64  `json:"chunkSize"`
	ResumePosition  int64  `json:"resumePosition"`
}

// GetResumeInfo retrieves resume information for a paused download
func (d *ChunkedDownloader) GetResumeInfo(itemID string) (*ResumeInfo, error) {
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("queue item not found: %s", itemID)
	}

	// Can only get resume info for paused or failed items
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

// CanResume checks if a download can be resumed
func (d *ChunkedDownloader) CanResume(itemID string) (bool, error) {
	item, err := d.queueService.GetByID(itemID)
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}

	// Can resume paused or failed items that have partial progress
	if item.Status == database.QueueStatusPaused || item.Status == database.QueueStatusFailed {
		return item.ChunksCompleted > 0 && item.ChunksCompleted < item.ChunksTotal, nil
	}

	return false, nil
}

// SaveProgress explicitly saves the current progress to database
// This is called periodically during download and when pausing
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

// GetPausedDownloads returns all paused downloads that can be resumed
func (d *ChunkedDownloader) GetPausedDownloads() ([]database.QueueItem, error) {
	return d.queueService.GetByStatus(database.QueueStatusPaused)
}

// ResumeAllPaused resumes all paused downloads
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


// FileIntegrityResult contains the result of a file integrity check
type FileIntegrityResult struct {
	IsValid      bool   `json:"isValid"`
	ExpectedSize int64  `json:"expectedSize"`
	ActualSize   int64  `json:"actualSize"`
	FilePath     string `json:"filePath"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// VerifyDownloadedFile verifies the integrity of a downloaded file
// This checks that the actual file size matches the expected size
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

	// Check if file exists
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

// VerifyFileSize is a standalone function to verify file size matches expected
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

// VerifyAllCompletedDownloads verifies all completed downloads
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


// RetryConfig contains retry configuration
type RetryConfig struct {
	MaxRetries      int           `json:"maxRetries"`
	InitialDelay    time.Duration `json:"initialDelay"`
	MaxDelay        time.Duration `json:"maxDelay"`
	BackoffFactor   float64       `json:"backoffFactor"`
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:    3,
		InitialDelay:  time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
	}
}

// RetryResult contains the result of a retry operation
type RetryResult struct {
	Success      bool   `json:"success"`
	Attempts     int    `json:"attempts"`
	LastError    string `json:"lastError,omitempty"`
	TotalTime    int64  `json:"totalTimeMs"`
}

// downloadChunkWithRetryTracked downloads a chunk with retry and returns detailed result
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

			// Calculate next delay with backoff
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

// RetryFailedDownload retries a failed download from the beginning or last checkpoint
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
