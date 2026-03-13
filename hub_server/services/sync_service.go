package services

import (
	"fmt"
	"log"
	"strings"
	"time"
	"wx_channel/hub_server/database"
	"wx_channel/hub_server/models"

	"gorm.io/gorm"
)

// SyncService 同步服务
type SyncService struct {
	db           *gorm.DB
	syncInterval time.Duration
	maxRetries   int
	running      bool
	stopChan     chan struct{}
	hub          interface{} // WebSocket Hub，用于反向同步
}

// SyncConfig 同步配置
type SyncConfig struct {
	Enabled    bool          `json:"enabled"`
	Interval   time.Duration `json:"interval"`
	MaxRetries int           `json:"max_retries"`
	Hub        interface{}   `json:"-"` // WebSocket Hub
}

var globalSyncService *SyncService

// NewSyncService 创建同步服务
func NewSyncService(config SyncConfig) *SyncService {
	return &SyncService{
		db:           database.DB,
		syncInterval: config.Interval,
		maxRetries:   config.MaxRetries,
		stopChan:     make(chan struct{}),
		hub:          config.Hub,
	}
}

// Start 启动同步服务
func (s *SyncService) Start() {
	if s.running {
		return
	}
	s.running = true
	log.Println("[SyncService] Starting sync service...")

	// 立即执行一次同步
	go s.syncAllDevices()

	// 定时同步
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go s.syncAllDevices()
		case <-s.stopChan:
			log.Println("[SyncService] Stopping sync service...")
			s.running = false
			return
		}
	}
}

// Stop 停止同步服务
func (s *SyncService) Stop() {
	if !s.running {
		return
	}
	close(s.stopChan)
}

// syncAllDevices 同步所有在线设备
func (s *SyncService) syncAllDevices() {
	log.Println("[SyncService] Starting sync for all devices...")

	// 获取所有在线设备
	nodes, err := database.GetActiveNodes(10 * time.Minute)
	if err != nil {
		log.Printf("[SyncService] Failed to get active nodes: %v", err)
		return
	}

	log.Printf("[SyncService] Found %d active devices", len(nodes))

	for _, node := range nodes {
		// 异步同步每个设备
		go func(n models.Node) {
			if err := s.SyncDevice(n.ID); err != nil {
				log.Printf("[SyncService] Failed to sync device %s: %v", n.ID, err)
			}
		}(node)
	}
}

// SyncDevice 同步单个设备（仅用于 WebSocket 推送模式）
// 注意：此方法现在只是更新同步状态，实际数据由客户端主动推送
func (s *SyncService) SyncDevice(machineID string) error {
	log.Printf("[SyncService] Checking sync status for device: %s", machineID)

	// 获取设备信息
	node, err := database.GetNodeByID(machineID)
	if err != nil {
		return fmt.Errorf("device not found: %w", err)
	}

	// 检查设备是否在线
	if node.Status != "online" {
		return fmt.Errorf("device is offline")
	}

	// WebSocket 推送模式：客户端会主动推送数据
	// 这里只记录检查时间
	log.Printf("[SyncService] Device %s is online, waiting for client push", machineID)
	
	return nil
}

// getOrCreateSyncStatus 获取或创建同步状态
func (s *SyncService) getOrCreateSyncStatus(machineID string) (*models.SyncStatus, error) {
	var status models.SyncStatus
	
	// 先尝试查找
	err := s.db.Where("machine_id = ?", machineID).First(&status).Error
	if err == nil {
		// 找到了，直接返回
		return &status, nil
	}
	
	// 如果是其他错误（不是记录不存在），返回错误
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	
	// 记录不存在，创建新记录
	status = models.SyncStatus{
		MachineID:      machineID,
		LastSyncStatus: "never",
	}
	
	// 使用事务创建，如果失败（可能是并发创建），再次查询
	err = s.db.Create(&status).Error
	if err != nil {
		// 如果是唯一约束冲突，说明其他 goroutine 已经创建了，再次查询
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || 
		   strings.Contains(err.Error(), "Duplicate entry") {
			err = s.db.Where("machine_id = ?", machineID).First(&status).Error
			if err != nil {
				return nil, fmt.Errorf("failed to get sync status after conflict: %w", err)
			}
			return &status, nil
		}
		return nil, err
	}
	
	return &status, nil
}

// recordSyncHistory 记录同步历史
func (s *SyncService) recordSyncHistory(machineID, syncType string, recordsSynced int, status, errorMsg string) {
	history := &models.SyncHistory{
		MachineID:     machineID,
		SyncTime:      time.Now(),
		SyncType:      syncType,
		RecordsSynced: recordsSynced,
		Status:        status,
		ErrorMessage:  errorMsg,
	}
	s.db.Create(history)
}

// BrowseRecord 浏览记录（客户端响应格式）
// 注意：JSON标签必须与客户端的 database.BrowseRecord 一致（驼峰命名）
type BrowseRecord struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	AuthorID     string    `json:"authorId"`      // 驼峰命名
	Duration     int64     `json:"duration"`      // 改为int64匹配客户端
	Size         int64     `json:"size"`
	Resolution   string    `json:"resolution"`
	CoverURL     string    `json:"coverUrl"`      // 驼峰命名
	VideoURL     string    `json:"videoUrl"`      // 驼峰命名
	DecryptKey   string    `json:"decryptKey"`    // 驼峰命名
	BrowseTime   time.Time `json:"browseTime"`    // 驼峰命名
	LikeCount    int64     `json:"likeCount"`     // 驼峰命名，改为int64
	CommentCount int64     `json:"commentCount"`  // 驼峰命名，改为int64
	FavCount     int64     `json:"favCount"`      // 驼峰命名，改为int64
	ForwardCount int64     `json:"forwardCount"`  // 驼峰命名，改为int64
	PageURL      string    `json:"pageUrl"`       // 驼峰命名
	CreatedAt    time.Time `json:"createdAt"`     // 驼峰命名
	UpdatedAt    time.Time `json:"updatedAt"`     // 驼峰命名
}

// DownloadRecord 下载记录（客户端响应格式）
// 注意：JSON标签必须与客户端的 database.DownloadRecord 一致（驼峰命名）
type DownloadRecord struct {
	ID           string    `json:"id"`
	VideoID      string    `json:"videoId"`       // 驼峰命名
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	CoverURL     string    `json:"coverUrl"`      // 驼峰命名
	Duration     int64     `json:"duration"`      // 改为int64匹配客户端
	FileSize     int64     `json:"fileSize"`      // 驼峰命名
	FilePath     string    `json:"filePath"`      // 驼峰命名
	Format       string    `json:"format"`
	Resolution   string    `json:"resolution"`
	Status       string    `json:"status"`
	DownloadTime time.Time `json:"downloadTime"`  // 驼峰命名
	ErrorMessage string    `json:"errorMessage"`  // 驼峰命名
	LikeCount    int64     `json:"likeCount"`     // 驼峰命名，改为int64
	CommentCount int64     `json:"commentCount"`  // 驼峰命名，改为int64
	ForwardCount int64     `json:"forwardCount"`  // 驼峰命名，改为int64
	FavCount     int64     `json:"favCount"`      // 驼峰命名，改为int64
	CreatedAt    time.Time `json:"createdAt"`     // 驼峰命名
	UpdatedAt    time.Time `json:"updatedAt"`     // 驼峰命名
}

// InitSyncService 初始化全局同步服务
func InitSyncService(config SyncConfig) {
	if !config.Enabled {
		log.Println("[SyncService] Sync service is disabled")
		return
	}

	globalSyncService = NewSyncService(config)
	go globalSyncService.Start()
}

// GetSyncService 获取全局同步服务
func GetSyncService() *SyncService {
	return globalSyncService
}

// HandleSyncDataFromClient 处理客户端推送的同步数据
// 这个方法应该从 WebSocket 消息处理器中调用
func (s *SyncService) HandleSyncDataFromClient(machineID string, syncType string, records interface{}) error {
	log.Printf("[SyncService] Received sync data from client: %s, type: %s", machineID, syncType)
	
	// 获取同步状态
	syncStatus, err := s.getOrCreateSyncStatus(machineID)
	if err != nil {
		return fmt.Errorf("failed to get sync status: %w", err)
	}
	
	switch syncType {
	case "browse":
		// 处理浏览记录
		browseRecords, ok := records.([]BrowseRecord)
		if !ok {
			return fmt.Errorf("invalid browse records format")
		}
		return s.saveBrowseRecords(machineID, browseRecords, syncStatus)
		
	case "download":
		// 处理下载记录
		downloadRecords, ok := records.([]DownloadRecord)
		if !ok {
			return fmt.Errorf("invalid download records format")
		}
		return s.saveDownloadRecords(machineID, downloadRecords, syncStatus)
		
	default:
		return fmt.Errorf("unknown sync type: %s", syncType)
	}
}

// saveBrowseRecords 保存浏览记录（从 syncBrowseHistory 提取的逻辑）
func (s *SyncService) saveBrowseRecords(machineID string, records []BrowseRecord, syncStatus *models.SyncStatus) error {
	if len(records) == 0 {
		return nil
	}
	
	savedCount := 0
	for _, record := range records {
		hubRecord := &models.HubBrowseHistory{
			ID:              record.ID,
			MachineID:       machineID,
			Title:           record.Title,
			Author:          record.Author,
			AuthorID:        record.AuthorID,
			Duration:        record.Duration,
			Size:            record.Size,
			Resolution:      record.Resolution,
			CoverURL:        record.CoverURL,
			VideoURL:        record.VideoURL,
			DecryptKey:      record.DecryptKey,
			BrowseTime:      record.BrowseTime,
			LikeCount:       record.LikeCount,
			CommentCount:    record.CommentCount,
			FavCount:        record.FavCount,
			ForwardCount:    record.ForwardCount,
			PageURL:         record.PageURL,
			SourceCreatedAt: record.CreatedAt,
			SourceUpdatedAt: record.UpdatedAt,
			SyncedAt:        time.Now(),
		}

		result := s.db.Where("id = ? AND machine_id = ?", hubRecord.ID, hubRecord.MachineID).
			FirstOrCreate(hubRecord)
		
		if result.Error == nil && result.RowsAffected > 0 {
			savedCount++
		}
	}

	// 更新同步状态
	syncStatus.LastBrowseSyncTime = time.Now()
	syncStatus.BrowseRecordCount += int64(savedCount)
	syncStatus.LastSyncStatus = "success"
	syncStatus.LastSyncError = ""
	
	// 保存到数据库
	if err := s.db.Save(syncStatus).Error; err != nil {
		log.Printf("[SyncService] Failed to update sync status: %v", err)
	}
	
	s.recordSyncHistory(machineID, "browse", savedCount, "success", "")
	
	log.Printf("[SyncService] Saved %d browse records for device: %s", savedCount, machineID)
	return nil
}

// saveDownloadRecords 保存下载记录（从 syncDownloadRecords 提取的逻辑）
func (s *SyncService) saveDownloadRecords(machineID string, records []DownloadRecord, syncStatus *models.SyncStatus) error {
	if len(records) == 0 {
		return nil
	}
	
	savedCount := 0
	for _, record := range records {
		hubRecord := &models.HubDownloadRecord{
			ID:              record.ID,
			MachineID:       machineID,
			VideoID:         record.VideoID,
			Title:           record.Title,
			Author:          record.Author,
			CoverURL:        record.CoverURL,
			Duration:        record.Duration,
			FileSize:        record.FileSize,
			FilePath:        record.FilePath,
			Format:          record.Format,
			Resolution:      record.Resolution,
			Status:          record.Status,
			DownloadTime:    record.DownloadTime,
			ErrorMessage:    record.ErrorMessage,
			LikeCount:       record.LikeCount,
			CommentCount:    record.CommentCount,
			ForwardCount:    record.ForwardCount,
			FavCount:        record.FavCount,
			SourceCreatedAt: record.CreatedAt,
			SourceUpdatedAt: record.UpdatedAt,
			SyncedAt:        time.Now(),
		}

		result := s.db.Where("id = ? AND machine_id = ?", hubRecord.ID, hubRecord.MachineID).
			FirstOrCreate(hubRecord)
		
		if result.Error == nil && result.RowsAffected > 0 {
			savedCount++
		}
	}

	// 更新同步状态
	syncStatus.LastDownloadSyncTime = time.Now()
	syncStatus.DownloadRecordCount += int64(savedCount)
	syncStatus.LastSyncStatus = "success"
	syncStatus.LastSyncError = ""
	
	// 保存到数据库
	if err := s.db.Save(syncStatus).Error; err != nil {
		log.Printf("[SyncService] Failed to update sync status: %v", err)
	}
	
	s.recordSyncHistory(machineID, "download", savedCount, "success", "")
	
	log.Printf("[SyncService] Saved %d download records for device: %s", savedCount, machineID)
	return nil
}
