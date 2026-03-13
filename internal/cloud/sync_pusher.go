package cloud

import (
	"encoding/json"
	"fmt"
	"time"

	"wx_channel/internal/database"
	"wx_channel/internal/utils"
)

// SyncPusher 同步数据推送器
type SyncPusher struct {
	connector         *Connector
	browseRepo        *database.BrowseHistoryRepository
	downloadRepo      *database.DownloadRecordRepository
	syncInterval      time.Duration
	lastBrowseSync    time.Time
	lastDownloadSync  time.Time
	running           bool
	stopChan          chan struct{}
	batchSize         int
}

// NewSyncPusher 创建同步推送器
func NewSyncPusher(connector *Connector) *SyncPusher {
	cfg := connector.cfg
	
	// 从配置读取同步间隔和批量大小
	syncInterval := cfg.HubSync.PushInterval
	if syncInterval == 0 {
		syncInterval = 5 * time.Minute // 默认5分钟
	}
	
	batchSize := cfg.HubSync.PushBatchSize
	if batchSize == 0 {
		batchSize = 1000 // 默认1000条
	}
	
	return &SyncPusher{
		connector:    connector,
		browseRepo:   database.NewBrowseHistoryRepository(),
		downloadRepo: database.NewDownloadRecordRepository(),
		syncInterval: syncInterval,
		stopChan:     make(chan struct{}),
		batchSize:    batchSize,
	}
}

// Start 启动同步推送器
func (sp *SyncPusher) Start() {
	if sp.running {
		return
	}
	sp.running = true

	utils.LogInfo("[SyncPusher] 启动同步推送器 (间隔: %v)", sp.syncInterval)

	// 立即执行一次同步
	go sp.pushSyncData()

	// 定时推送
	ticker := time.NewTicker(sp.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go sp.pushSyncData()
		case <-sp.stopChan:
			utils.LogInfo("[SyncPusher] 停止同步推送器")
			sp.running = false
			return
		}
	}
}

// Stop 停止同步推送器
func (sp *SyncPusher) Stop() {
	if !sp.running {
		return
	}
	close(sp.stopChan)
}

// pushSyncData 推送同步数据
func (sp *SyncPusher) pushSyncData() {
	// 推送浏览记录
	if err := sp.pushBrowseHistory(); err != nil {
		utils.LogWarn("[SyncPusher] 推送浏览记录失败: %v", err)
	}

	// 推送下载记录
	if err := sp.pushDownloadRecords(); err != nil {
		utils.LogWarn("[SyncPusher] 推送下载记录失败: %v", err)
	}
}

// pushBrowseHistory 推送浏览记录
func (sp *SyncPusher) pushBrowseHistory() error {
	// 获取增量数据
	records, err := sp.browseRepo.GetRecordsSince(sp.lastBrowseSync, sp.batchSize)
	if err != nil {
		return fmt.Errorf("获取浏览记录失败: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	// 构建消息
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("序列化浏览记录失败: %w", err)
	}

	payload := SyncDataPayload{
		SyncType: "browse",
		Records:  recordsJSON,
		Count:    len(records),
		HasMore:  len(records) >= sp.batchSize,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化同步载荷失败: %w", err)
	}

	// 发送消息
	msg := CloudMessage{
		ID:        fmt.Sprintf("sync-browse-%d", time.Now().UnixNano()),
		Type:      MsgTypeSyncData,
		ClientID:  sp.connector.clientID,
		Payload:   payloadJSON,
		Timestamp: time.Now().Unix(),
	}

	if err := sp.connector.send(msg); err != nil {
		return fmt.Errorf("发送浏览记录失败: %w", err)
	}

	// 更新最后同步时间
	sp.lastBrowseSync = time.Now()
	utils.LogInfo("[SyncPusher] 推送 %d 条浏览记录", len(records))

	return nil
}

// pushDownloadRecords 推送下载记录
func (sp *SyncPusher) pushDownloadRecords() error {
	// 获取增量数据
	records, err := sp.downloadRepo.GetRecordsSince(sp.lastDownloadSync, sp.batchSize)
	if err != nil {
		return fmt.Errorf("获取下载记录失败: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	// 构建消息
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("序列化下载记录失败: %w", err)
	}

	payload := SyncDataPayload{
		SyncType: "download",
		Records:  recordsJSON,
		Count:    len(records),
		HasMore:  len(records) >= sp.batchSize,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化同步载荷失败: %w", err)
	}

	// 发送消息
	msg := CloudMessage{
		ID:        fmt.Sprintf("sync-download-%d", time.Now().UnixNano()),
		Type:      MsgTypeSyncData,
		ClientID:  sp.connector.clientID,
		Payload:   payloadJSON,
		Timestamp: time.Now().Unix(),
	}

	if err := sp.connector.send(msg); err != nil {
		return fmt.Errorf("发送下载记录失败: %w", err)
	}

	// 更新最后同步时间
	sp.lastDownloadSync = time.Now()
	utils.LogInfo("[SyncPusher] 推送 %d 条下载记录", len(records))

	return nil
}

// SetSyncInterval 设置同步间隔
func (sp *SyncPusher) SetSyncInterval(interval time.Duration) {
	sp.syncInterval = interval
}

// SetBatchSize 设置批量大小
func (sp *SyncPusher) SetBatchSize(size int) {
	sp.batchSize = size
}
