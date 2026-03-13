package services

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
	"wx_channel/internal/utils"

	"github.com/google/uuid"
)

// QueueService 处理下载队列管理操作
type QueueService struct {
	repo     *database.QueueRepository
	settings *database.SettingsRepository
}

// NewQueueService 创建一个新的 QueueService
func NewQueueService() *QueueService {
	return &QueueService{
		repo:     database.NewQueueRepository(),
		settings: database.NewSettingsRepository(),
	}
}

// VideoInfo 表示要添加到队列的视频信息
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

// AddToQueue 将视频添加到下载队列
func (s *QueueService) AddToQueue(videos []VideoInfo) ([]database.QueueItem, error) {

	// 加载设置以获取分片大小
	settings, err := s.settings.Load()
	if err != nil {
		settings = database.DefaultSettings()
	}

	// 获取当前最大优先级
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
		// 计算分片
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
			Priority:        maxPriority + len(videos) - i, // 较早的项目优先级更高
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

// RemoveFromQueue 从队列中移除项目
// 注意：根据需求 10.5，这不会删除任何部分下载数据
func (s *QueueService) RemoveFromQueue(id string) error {

	return s.repo.Remove(id)
}

// RemoveMany 从队列中批量移除项目
func (s *QueueService) RemoveMany(ids []string) (int64, error) {

	return s.repo.RemoveMany(ids)
}

// Pause 暂停正在下载的项目
func (s *QueueService) Pause(id string) error {

	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", id)
	}

	// 只能暂停正在下载的项目
	if item.Status != database.QueueStatusDownloading {
		return fmt.Errorf("can only pause downloading items, current status: %s", item.Status)
	}

	return s.repo.UpdateStatus(id, database.QueueStatusPaused)
}

// Resume 恢复暂停的项目
func (s *QueueService) Resume(id string) error {

	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", id)
	}

	// 只能恢复暂停的项目
	if item.Status != database.QueueStatusPaused {
		return fmt.Errorf("can only resume paused items, current status: %s", item.Status)
	}

	return s.repo.UpdateStatus(id, database.QueueStatusPending)
}

// Reorder 根据提供的 ID 顺序重新排序队列
func (s *QueueService) Reorder(ids []string) error {

	return s.repo.Reorder(ids)
}

// SetPriority 设置队列项目的优先级
func (s *QueueService) SetPriority(id string, priority int) error {

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

// GetQueue 返回按优先级排序的所有队列项目
func (s *QueueService) GetQueue() ([]database.QueueItem, error) {

	return s.repo.List()
}

// GetByID 按 ID 返回队列项目
func (s *QueueService) GetByID(id string) (*database.QueueItem, error) {

	return s.repo.GetByID(id)
}

// GetByVideoID 根据 VideoID 返回队列项目
func (s *QueueService) GetByVideoID(videoID string) (*database.QueueItem, error) {
	return s.repo.GetByVideoID(videoID)
}

// GetByStatus 返回具有特定状态的队列项目
func (s *QueueService) GetByStatus(status string) ([]database.QueueItem, error) {

	return s.repo.ListByStatus(status)
}

// GetNextPending 返回下一个待下载的项目
func (s *QueueService) GetNextPending() (*database.QueueItem, error) {

	return s.repo.GetNextPending()
}

// UpdateProgress 更新队列项目的下载进度
func (s *QueueService) UpdateProgress(id string, downloadedSize int64, chunksCompleted int, speed int64) error {

	return s.repo.UpdateProgress(id, downloadedSize, chunksCompleted, speed)
}

// UpdateStatus 更新队列项目的状态
func (s *QueueService) UpdateStatus(id string, status string) error {

	return s.repo.UpdateStatus(id, status)
}

// StartDownload 标记项目为正在下载并设置开始时间
func (s *QueueService) StartDownload(id string) error {

	if err := s.repo.UpdateStatus(id, database.QueueStatusDownloading); err != nil {
		return err
	}
	return s.repo.SetStartTime(id, time.Now())
}

// CompleteDownload 标记项目为完成并创建下载记录
func (s *QueueService) CompleteDownload(id string) error {

	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("queue item not found: %s", id)
	}

	// 检查是否已完成以避免重复记录
	if item.Status == database.QueueStatusCompleted {
		// 已完成，无需再次创建记录
		return nil
	}

	item.Status = database.QueueStatusCompleted
	item.DownloadedSize = item.TotalSize
	item.ChunksCompleted = item.ChunksTotal
	item.Speed = 0

	if err := s.repo.Update(item); err != nil {
		return err
	}

	// 根据批量下载约定计算文件路径
	// 路径格式: {baseDir}/downloads/{authorFolder}/{cleanFilename}.mp4
	filePath := calculateDownloadFilePath(item.Author, item.Title)

	downloadRepo := database.NewDownloadRecordRepository()

	// 检查是否已经存在该视频的下载记录 (由 batch.go 等其他流程创建)
	if existingRecord, _ := downloadRepo.GetByVideoID(item.VideoID); existingRecord != nil {
		// 已存在记录，仅需确保状态为完成，不需要新建
		if existingRecord.Status != database.DownloadStatusCompleted {
			existingRecord.Status = database.DownloadStatusCompleted
			existingRecord.FilePath = filePath
			_ = downloadRepo.Update(existingRecord)
		}
		return nil
	}

	// 使用 VideoID 作为主键ID (如果有)，否则使用 UUID，确保与 batch.go 行为一致，触发 REPLACE 逻辑
	recordID := item.VideoID
	if recordID == "" {
		recordID = uuid.New().String()
	}

	// 创建全新的下载记录
	downloadRecord := &database.DownloadRecord{
		ID:           recordID,
		VideoID:      item.VideoID,
		Title:        item.Title,
		Author:       item.Author,
		CoverURL:     item.CoverURL,
		Duration:     item.Duration,
		FileSize:     item.TotalSize,
		FilePath:     filePath,
		Format:       "mp4",
		Resolution:   item.Resolution, // 使用队列项目中的分辨率
		Status:       database.DownloadStatusCompleted,
		DownloadTime: time.Now(),
	}

	if err := downloadRepo.Create(downloadRecord); err != nil {
		// 记录错误但不失败完成
		fmt.Printf("Warning: failed to create download record: %v\n", err)
	}

	return nil
}

// calculateDownloadFilePath 计算下载视频的预期文件路径
func calculateDownloadFilePath(author, title string) string {
	// 从当前配置获取下载目录
	cfg := config.Get()
	var downloadsDir string
	var err error

	if cfg != nil {
		downloadsDir, err = cfg.GetResolvedDownloadsDir()
	}

	if err != nil || downloadsDir == "" {
		// 回退到软件基础目录 + downloads
		baseDir, baseErr := utils.GetBaseDir()
		if baseErr != nil {
			baseDir = "."
		}
		downloadsDir = filepath.Join(baseDir, "downloads")
	}

	// 清理作者名作为文件夹名
	authorFolder := cleanFolderName(author)
	if authorFolder == "" {
		authorFolder = "未知作者"
	}

	// 清理标题作为文件名
	cleanTitle := cleanFilename(title)
	if cleanTitle == "" {
		cleanTitle = "未命名视频"
	}

	// 确保 .mp4 扩展名
	if !strings.HasSuffix(strings.ToLower(cleanTitle), ".mp4") {
		cleanTitle = cleanTitle + ".mp4"
	}

	// 使用正确的下载目录返回绝对路径
	// 路径格式: {downloadsDir}/{author}/{title}.mp4
	return filepath.Join(downloadsDir, authorFolder, cleanTitle)
}

// cleanFolderName 从文件夹名称中移除无效字符
func cleanFolderName(name string) string {
	if name == "" {
		return ""
	}
	// 移除文件夹名称中无效的字符
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	// 去除空格
	result = strings.TrimSpace(result)
	// Windows 文件系统会自动去除文件夹名称末尾的点（.）
	// 为了确保创建文件夹和查找路径时使用相同的名称，我们需要手动去除末尾的点
	result = strings.TrimRight(result, ".")
	// 如果去除末尾点后为空，返回空字符串（调用方会处理）
	return result
}

// cleanFilename 从文件名中移除无效字符
func cleanFilename(name string) string {
	if name == "" {
		return ""
	}
	// 移除文件名中无效的字符
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	// 去除空格
	result = strings.TrimSpace(result)
	// 限制长度
	if len(result) > 200 {
		result = result[:200]
	}
	return result
}

// FailDownload 标记项目为失败并附带错误消息
func (s *QueueService) FailDownload(id string, errorMessage string) error {

	return s.repo.SetError(id, errorMessage)
}

// IncrementRetryCount 增加项目的重试计数
func (s *QueueService) IncrementRetryCount(id string) error {

	return s.repo.IncrementRetryCount(id)
}

// ClearQueue 从队列中移除所有项目
func (s *QueueService) ClearQueue() error {

	return s.repo.Clear()
}

// GetQueueStats 返回队列统计信息
func (s *QueueService) GetQueueStats() (*QueueStats, error) {

	total, pending, downloading, paused, completed, failed, err := s.repo.GetQueueStats()
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

// QueueStats 表示队列统计信息
type QueueStats struct {
	Total       int64 `json:"total"`
	Pending     int64 `json:"pending"`
	Downloading int64 `json:"downloading"`
	Paused      int64 `json:"paused"`
	Completed   int64 `json:"completed"`
	Failed      int64 `json:"failed"`
}

// CalculateChunkCount 计算文件所需的分片数
// 公式: ceil(fileSize / chunkSize)
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

// ResetRetryCount 重置队列项目的重试计数
func (s *QueueService) ResetRetryCount(id string) error {

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

// UpdateItem 更新队列项目
func (s *QueueService) UpdateItem(item *database.QueueItem) error {

	return s.repo.Update(item)
}
