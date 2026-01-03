package services

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
	"wx_channel/internal/utils"

	"github.com/google/uuid"
)

// QueueService handles download queue management operations
type QueueService struct {
	repo     *database.QueueRepository
	settings *database.SettingsRepository
	mu       sync.RWMutex
}

// NewQueueService creates a new QueueService
func NewQueueService() *QueueService {
	return &QueueService{
		repo:     database.NewQueueRepository(),
		settings: database.NewSettingsRepository(),
	}
}

// VideoInfo represents video information for adding to queue
type VideoInfo struct {
	VideoID    string `json:"videoId"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	CoverURL   string `json:"coverUrl"`
	VideoURL   string `json:"videoUrl"`
	DecryptKey string `json:"decryptKey"`
	Duration   int64  `json:"duration"`
	Resolution string `json:"resolution"`
	Size       int64  `json:"size"`
}

// AddToQueue adds videos to the download queue
func (s *QueueService) AddToQueue(videos []VideoInfo) ([]database.QueueItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load settings for chunk size
	settings, err := s.settings.Load()
	if err != nil {
		settings = database.DefaultSettings()
	}

	// Get current max priority
	items, err := s.repo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to get queue items: %w", err)
	}
	maxPriority := 0
	for _, item := range items {
		if item.Priority > maxPriority {
			maxPriority = item.Priority
		}
	}

	addedItems := make([]database.QueueItem, 0, len(videos))
	now := time.Now()

	for i, video := range videos {
		// Calculate chunks
		chunkSize := settings.ChunkSize
		chunksTotal := CalculateChunkCount(video.Size, chunkSize)

		item := &database.QueueItem{
			ID:              uuid.New().String(),
			VideoID:         video.VideoID,
			Title:           video.Title,
			Author:          video.Author,
			CoverURL:        video.CoverURL,
			VideoURL:        video.VideoURL,
			DecryptKey:      video.DecryptKey,
			Duration:        video.Duration,
			Resolution:      video.Resolution,
			TotalSize:       video.Size,
			DownloadedSize:  0,
			Status:          database.QueueStatusPending,
			Priority:        maxPriority + len(videos) - i, // Higher priority for earlier items
			AddedTime:       now,
			Speed:           0,
			ChunkSize:       chunkSize,
			ChunksTotal:     chunksTotal,
			ChunksCompleted: 0,
			RetryCount:      0,
		}

		if err := s.repo.Add(item); err != nil {
			return nil, fmt.Errorf("failed to add item to queue: %w", err)
		}
		addedItems = append(addedItems, *item)
	}

	return addedItems, nil
}


// RemoveFromQueue removes an item from the queue
// Note: This does not delete any partial download data (per requirement 10.5)
func (s *QueueService) RemoveFromQueue(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.Remove(id)
}

// RemoveMany removes multiple items from the queue
func (s *QueueService) RemoveMany(ids []string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.RemoveMany(ids)
}

// Pause pauses a downloading item
func (s *QueueService) Pause(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", id)
	}

	// Can only pause downloading items
	if item.Status != database.QueueStatusDownloading {
		return fmt.Errorf("can only pause downloading items, current status: %s", item.Status)
	}

	return s.repo.UpdateStatus(id, database.QueueStatusPaused)
}

// Resume resumes a paused item
func (s *QueueService) Resume(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", id)
	}

	// Can only resume paused items
	if item.Status != database.QueueStatusPaused {
		return fmt.Errorf("can only resume paused items, current status: %s", item.Status)
	}

	return s.repo.UpdateStatus(id, database.QueueStatusPending)
}

// Reorder reorders the queue based on the provided order of IDs
func (s *QueueService) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.Reorder(ids)
}

// SetPriority sets the priority of a queue item
func (s *QueueService) SetPriority(id string, priority int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", id)
	}

	item.Priority = priority
	return s.repo.Update(item)
}

// GetQueue returns all queue items sorted by priority
func (s *QueueService) GetQueue() ([]database.QueueItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.repo.List()
}

// GetByID returns a queue item by ID
func (s *QueueService) GetByID(id string) (*database.QueueItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.repo.GetByID(id)
}

// GetByStatus returns queue items with a specific status
func (s *QueueService) GetByStatus(status string) ([]database.QueueItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.repo.ListByStatus(status)
}

// GetNextPending returns the next pending item to download
func (s *QueueService) GetNextPending() (*database.QueueItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.repo.GetNextPending()
}

// UpdateProgress updates the download progress of a queue item
func (s *QueueService) UpdateProgress(id string, downloadedSize int64, chunksCompleted int, speed int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.UpdateProgress(id, downloadedSize, chunksCompleted, speed)
}

// UpdateStatus updates the status of a queue item
func (s *QueueService) UpdateStatus(id string, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.UpdateStatus(id, status)
}

// StartDownload marks an item as downloading and sets the start time
func (s *QueueService) StartDownload(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.repo.UpdateStatus(id, database.QueueStatusDownloading); err != nil {
		return err
	}
	return s.repo.SetStartTime(id, time.Now())
}

// CompleteDownload marks an item as completed and creates a download record
func (s *QueueService) CompleteDownload(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", id)
	}

	// Check if already completed to avoid duplicate records
	if item.Status == database.QueueStatusCompleted {
		// Already completed, no need to create another record
		return nil
	}

	item.Status = database.QueueStatusCompleted
	item.DownloadedSize = item.TotalSize
	item.ChunksCompleted = item.ChunksTotal
	item.Speed = 0

	if err := s.repo.Update(item); err != nil {
		return err
	}

	// Calculate file path based on batch download convention
	// Path format: {baseDir}/downloads/{authorFolder}/{cleanFilename}.mp4
	filePath := calculateDownloadFilePath(item.Author, item.Title)

	// Create download record
	downloadRecord := &database.DownloadRecord{
		ID:           uuid.New().String(),
		VideoID:      item.VideoID,
		Title:        item.Title,
		Author:       item.Author,
		CoverURL:     item.CoverURL,
		Duration:     item.Duration,
		FileSize:     item.TotalSize,
		FilePath:     filePath,
		Format:       "mp4",
		Resolution:   item.Resolution, // Use resolution from queue item
		Status:       database.DownloadStatusCompleted,
		DownloadTime: time.Now(),
	}

	downloadRepo := database.NewDownloadRecordRepository()
	if err := downloadRepo.Create(downloadRecord); err != nil {
		// Log error but don't fail the completion
		fmt.Printf("Warning: failed to create download record: %v\n", err)
	}

	return nil
}

// calculateDownloadFilePath calculates the expected file path for a downloaded video
func calculateDownloadFilePath(author, title string) string {
	// Get download directory from current configuration
	cfg := config.Get()
	var downloadsDir string
	var err error
	
	if cfg != nil {
		downloadsDir, err = cfg.GetResolvedDownloadsDir()
	}
	
	if err != nil || downloadsDir == "" {
		// Fallback to software base directory + downloads
		baseDir, baseErr := utils.GetBaseDir()
		if baseErr != nil {
			baseDir = "."
		}
		downloadsDir = filepath.Join(baseDir, "downloads")
	}
	
	// Clean author name for folder
	authorFolder := cleanFolderName(author)
	if authorFolder == "" {
		authorFolder = "未知作者"
	}
	
	// Clean title for filename
	cleanTitle := cleanFilename(title)
	if cleanTitle == "" {
		cleanTitle = "未命名视频"
	}
	
	// Ensure .mp4 extension
	if !strings.HasSuffix(strings.ToLower(cleanTitle), ".mp4") {
		cleanTitle = cleanTitle + ".mp4"
	}
	
	// Return absolute path using the correct download directory
	// Path format: {downloadsDir}/{author}/{title}.mp4
	return filepath.Join(downloadsDir, authorFolder, cleanTitle)
}

// cleanFolderName removes invalid characters from folder name
func cleanFolderName(name string) string {
	if name == "" {
		return ""
	}
	// Remove characters that are invalid in folder names
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	// Trim spaces
	result = strings.TrimSpace(result)
	// Windows 文件系统会自动去除文件夹名称末尾的点（.）
	// 为了确保创建文件夹和查找路径时使用相同的名称，我们需要手动去除末尾的点
	result = strings.TrimRight(result, ".")
	// 如果去除末尾点后为空，返回空字符串（调用方会处理）
	return result
}

// cleanFilename removes invalid characters from filename
func cleanFilename(name string) string {
	if name == "" {
		return ""
	}
	// Remove characters that are invalid in filenames
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	// Trim spaces
	result = strings.TrimSpace(result)
	// Limit length
	if len(result) > 200 {
		result = result[:200]
	}
	return result
}

// FailDownload marks an item as failed with an error message
func (s *QueueService) FailDownload(id string, errorMessage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.SetError(id, errorMessage)
}

// IncrementRetryCount increments the retry count for an item
func (s *QueueService) IncrementRetryCount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.IncrementRetryCount(id)
}

// ClearQueue removes all items from the queue
func (s *QueueService) ClearQueue() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.Clear()
}

// GetQueueStats returns queue statistics
func (s *QueueService) GetQueueStats() (*QueueStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total, err := s.repo.Count()
	if err != nil {
		return nil, err
	}

	pending, err := s.repo.CountByStatus(database.QueueStatusPending)
	if err != nil {
		return nil, err
	}

	downloading, err := s.repo.CountByStatus(database.QueueStatusDownloading)
	if err != nil {
		return nil, err
	}

	paused, err := s.repo.CountByStatus(database.QueueStatusPaused)
	if err != nil {
		return nil, err
	}

	completed, err := s.repo.CountByStatus(database.QueueStatusCompleted)
	if err != nil {
		return nil, err
	}

	failed, err := s.repo.CountByStatus(database.QueueStatusFailed)
	if err != nil {
		return nil, err
	}

	return &QueueStats{
		Total:       total,
		Pending:     pending,
		Downloading: downloading,
		Paused:      paused,
		Completed:   completed,
		Failed:      failed,
	}, nil
}

// QueueStats represents queue statistics
type QueueStats struct {
	Total       int64 `json:"total"`
	Pending     int64 `json:"pending"`
	Downloading int64 `json:"downloading"`
	Paused      int64 `json:"paused"`
	Completed   int64 `json:"completed"`
	Failed      int64 `json:"failed"`
}

// CalculateChunkCount calculates the number of chunks needed for a file
// Formula: ceil(fileSize / chunkSize)
func CalculateChunkCount(fileSize, chunkSize int64) int {
	if chunkSize <= 0 {
		return 1
	}
	if fileSize <= 0 {
		return 0
	}
	chunks := fileSize / chunkSize
	if fileSize%chunkSize > 0 {
		chunks++
	}
	return int(chunks)
}

// ResetRetryCount resets the retry count for a queue item
func (s *QueueService) ResetRetryCount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", id)
	}

	item.RetryCount = 0
	return s.repo.Update(item)
}

// UpdateItem updates a queue item
func (s *QueueService) UpdateItem(item *database.QueueItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.repo.Update(item)
}
