package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"wx_channel/hub_server/database"
	"wx_channel/hub_server/models"

	"gorm.io/gorm"
)

// SyncService 同步服务
type SyncService struct {
	db           *gorm.DB
	httpClient   *http.Client
	syncToken    string
	syncInterval time.Duration
	maxRetries   int
	running      bool
	stopChan     chan struct{}
}

// SyncConfig 同步配置
type SyncConfig struct {
	Enabled      bool          `json:"enabled"`
	Interval     time.Duration `json:"interval"`
	Token        string        `json:"token"`
	MaxRetries   int           `json:"max_retries"`
	Timeout      time.Duration `json:"timeout"`
	BatchSize    int           `json:"batch_size"`
}

var globalSyncService *SyncService

// NewSyncService 创建同步服务
func NewSyncService(config SyncConfig) *SyncService {
	return &SyncService{
		db: database.DB,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		syncToken:    config.Token,
		syncInterval: config.Interval,
		maxRetries:   config.MaxRetries,
		stopChan:     make(chan struct{}),
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

// SyncDevice 同步单个设备
func (s *SyncService) SyncDevice(machineID string) error {
	log.Printf("[SyncService] Syncing device: %s", machineID)

	// 获取设备信息
	node, err := database.GetNodeByID(machineID)
	if err != nil {
		return fmt.Errorf("device not found: %w", err)
	}

	// 检查设备是否在线
	if node.Status != "online" {
		return fmt.Errorf("device is offline")
	}

	// 获取或创建同步状态
	syncStatus, err := s.getOrCreateSyncStatus(machineID)
	if err != nil {
		return fmt.Errorf("failed to get sync status: %w", err)
	}

	// 更新状态为同步中
	syncStatus.LastSyncStatus = "in_progress"
	s.db.Save(syncStatus)

	// 同步浏览记录
	browseErr := s.syncBrowseHistory(node, syncStatus)
	if browseErr != nil {
		log.Printf("[SyncService] Failed to sync browse history for %s: %v", machineID, browseErr)
	}

	// 同步下载记录
	downloadErr := s.syncDownloadRecords(node, syncStatus)
	if downloadErr != nil {
		log.Printf("[SyncService] Failed to sync download records for %s: %v", machineID, downloadErr)
	}

	// 更新同步状态
	if browseErr != nil || downloadErr != nil {
		syncStatus.LastSyncStatus = "failed"
		if browseErr != nil {
			syncStatus.LastSyncError = browseErr.Error()
		} else {
			syncStatus.LastSyncError = downloadErr.Error()
		}
	} else {
		syncStatus.LastSyncStatus = "success"
		syncStatus.LastSyncError = ""
	}

	s.db.Save(syncStatus)

	log.Printf("[SyncService] Sync completed for device: %s (status: %s)", machineID, syncStatus.LastSyncStatus)
	return nil
}

// syncBrowseHistory 同步浏览记录
func (s *SyncService) syncBrowseHistory(node *models.Node, syncStatus *models.SyncStatus) error {
	// 构建请求 URL
	baseURL := s.getNodeAPIURL(node)
	url := fmt.Sprintf("%s/api/sync/browse", baseURL)
	if syncStatus.LastBrowseSyncTime.IsZero() {
		url += "?limit=1000"
	} else {
		url += fmt.Sprintf("?since=%s&limit=1000", syncStatus.LastBrowseSyncTime.Format(time.RFC3339))
	}

	// 发送请求
	records, err := s.fetchBrowseRecords(url)
	if err != nil {
		s.recordSyncHistory(node.ID, "browse", 0, "failed", err.Error())
		return err
	}

	if len(records) == 0 {
		log.Printf("[SyncService] No new browse records for device: %s", node.ID)
		return nil
	}

	// 保存记录
	savedCount := 0
	for _, record := range records {
		hubRecord := &models.HubBrowseHistory{
			ID:              record.ID,
			MachineID:       node.ID,
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

		// 使用 FirstOrCreate 避免重复
		result := s.db.Where("id = ? AND machine_id = ?", hubRecord.ID, hubRecord.MachineID).
			FirstOrCreate(hubRecord)
		
		if result.Error == nil && result.RowsAffected > 0 {
			savedCount++
		}
	}

	// 更新同步状态
	syncStatus.LastBrowseSyncTime = time.Now()
	syncStatus.BrowseRecordCount += int64(savedCount)

	// 记录同步历史
	s.recordSyncHistory(node.ID, "browse", savedCount, "success", "")

	log.Printf("[SyncService] Synced %d browse records for device: %s", savedCount, node.ID)
	return nil
}

// syncDownloadRecords 同步下载记录
func (s *SyncService) syncDownloadRecords(node *models.Node, syncStatus *models.SyncStatus) error {
	// 构建请求 URL
	baseURL := s.getNodeAPIURL(node)
	url := fmt.Sprintf("%s/api/sync/download", baseURL)
	if syncStatus.LastDownloadSyncTime.IsZero() {
		url += "?limit=1000"
	} else {
		url += fmt.Sprintf("?since=%s&limit=1000", syncStatus.LastDownloadSyncTime.Format(time.RFC3339))
	}

	// 发送请求
	records, err := s.fetchDownloadRecords(url)
	if err != nil {
		s.recordSyncHistory(node.ID, "download", 0, "failed", err.Error())
		return err
	}

	if len(records) == 0 {
		log.Printf("[SyncService] No new download records for device: %s", node.ID)
		return nil
	}

	// 保存记录
	savedCount := 0
	for _, record := range records {
		hubRecord := &models.HubDownloadRecord{
			ID:              record.ID,
			MachineID:       node.ID,
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

		// 使用 FirstOrCreate 避免重复
		result := s.db.Where("id = ? AND machine_id = ?", hubRecord.ID, hubRecord.MachineID).
			FirstOrCreate(hubRecord)
		
		if result.Error == nil && result.RowsAffected > 0 {
			savedCount++
		}
	}

	// 更新同步状态
	syncStatus.LastDownloadSyncTime = time.Now()
	syncStatus.DownloadRecordCount += int64(savedCount)

	// 记录同步历史
	s.recordSyncHistory(node.ID, "download", savedCount, "success", "")

	log.Printf("[SyncService] Synced %d download records for device: %s", savedCount, node.ID)
	return nil
}

// fetchBrowseRecords 从客户端获取浏览记录
func (s *SyncService) fetchBrowseRecords(url string) ([]BrowseRecord, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 添加同步令牌
	if s.syncToken != "" {
		req.Header.Set("X-Sync-Token", s.syncToken)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    struct {
			Records []BrowseRecord `json:"records"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API error: %s", response.Message)
	}

	return response.Data.Records, nil
}

// fetchDownloadRecords 从客户端获取下载记录
func (s *SyncService) fetchDownloadRecords(url string) ([]DownloadRecord, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 添加同步令牌
	if s.syncToken != "" {
		req.Header.Set("X-Sync-Token", s.syncToken)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    struct {
			Records []DownloadRecord `json:"records"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("API error: %s", response.Message)
	}

	return response.Data.Records, nil
}

// getOrCreateSyncStatus 获取或创建同步状态
func (s *SyncService) getOrCreateSyncStatus(machineID string) (*models.SyncStatus, error) {
	var status models.SyncStatus
	result := s.db.Where("machine_id = ?", machineID).FirstOrCreate(&status, models.SyncStatus{
		MachineID:      machineID,
		LastSyncStatus: "never",
	})
	return &status, result.Error
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
type BrowseRecord struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	AuthorID     string    `json:"author_id"`
	Duration     int       `json:"duration"`
	Size         int64     `json:"size"`
	Resolution   string    `json:"resolution"`
	CoverURL     string    `json:"cover_url"`
	VideoURL     string    `json:"video_url"`
	DecryptKey   string    `json:"decrypt_key"`
	BrowseTime   time.Time `json:"browse_time"`
	LikeCount    int       `json:"like_count"`
	CommentCount int       `json:"comment_count"`
	FavCount     int       `json:"fav_count"`
	ForwardCount int       `json:"forward_count"`
	PageURL      string    `json:"page_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DownloadRecord 下载记录（客户端响应格式）
type DownloadRecord struct {
	ID           string    `json:"id"`
	VideoID      string    `json:"video_id"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	CoverURL     string    `json:"cover_url"`
	Duration     int       `json:"duration"`
	FileSize     int64     `json:"file_size"`
	FilePath     string    `json:"file_path"`
	Format       string    `json:"format"`
	Resolution   string    `json:"resolution"`
	Status       string    `json:"status"`
	DownloadTime time.Time `json:"download_time"`
	ErrorMessage string    `json:"error_message"`
	LikeCount    int       `json:"like_count"`
	CommentCount int       `json:"comment_count"`
	ForwardCount int       `json:"forward_count"`
	FavCount     int       `json:"fav_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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

// getNodeAPIURL 获取节点的 API URL
func (s *SyncService) getNodeAPIURL(node *models.Node) string {
	// 优先使用自定义的同步 API URL（用于 NAT 穿透或自定义地址）
	if node.SyncAPIURL != "" {
		return node.SyncAPIURL
	}

	// 使用 IP 和端口构建 URL
	port := node.Port
	if port == 0 {
		port = 2025 // 默认端口
	}

	// 如果 IP 包含端口号，直接使用
	if strings.Contains(node.IP, ":") {
		return fmt.Sprintf("http://%s", node.IP)
	}

	return fmt.Sprintf("http://%s:%d", node.IP, port)
}

// GetSyncService 获取全局同步服务
func GetSyncService() *SyncService {
	return globalSyncService
}
