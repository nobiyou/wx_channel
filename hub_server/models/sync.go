package models

import (
	"time"
)

// HubBrowseHistory Hub端浏览记录
type HubBrowseHistory struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	MachineID       string    `json:"machine_id" gorm:"index:idx_browse_machine_updated"`
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	AuthorID        string    `json:"author_id"`
	Duration        int       `json:"duration"`
	Size            int64     `json:"size"`
	Resolution      string    `json:"resolution"`
	CoverURL        string    `json:"cover_url"`
	VideoURL        string    `json:"video_url"`
	DecryptKey      string    `json:"decrypt_key"`
	BrowseTime      time.Time `json:"browse_time"`
	LikeCount       int       `json:"like_count"`
	CommentCount    int       `json:"comment_count"`
	FavCount        int       `json:"fav_count"`
	ForwardCount    int       `json:"forward_count"`
	PageURL         string    `json:"page_url"`
	SourceCreatedAt time.Time `json:"source_created_at"`
	SourceUpdatedAt time.Time `json:"source_updated_at" gorm:"index:idx_browse_machine_updated"`
	SyncedAt        time.Time `json:"synced_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// HubDownloadRecord Hub端下载记录
type HubDownloadRecord struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	MachineID       string    `json:"machine_id" gorm:"index:idx_download_machine_updated"`
	VideoID         string    `json:"video_id"`
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	CoverURL        string    `json:"cover_url"`
	Duration        int       `json:"duration"`
	FileSize        int64     `json:"file_size"`
	FilePath        string    `json:"file_path"`
	Format          string    `json:"format"`
	Resolution      string    `json:"resolution"`
	Status          string    `json:"status"`
	DownloadTime    time.Time `json:"download_time"`
	ErrorMessage    string    `json:"error_message"`
	LikeCount       int       `json:"like_count"`
	CommentCount    int       `json:"comment_count"`
	ForwardCount    int       `json:"forward_count"`
	FavCount        int       `json:"fav_count"`
	SourceCreatedAt time.Time `json:"source_created_at"`
	SourceUpdatedAt time.Time `json:"source_updated_at" gorm:"index:idx_download_machine_updated"`
	SyncedAt        time.Time `json:"synced_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SyncStatus 同步状态记录
type SyncStatus struct {
	ID                   uint      `json:"id" gorm:"primaryKey"`
	MachineID            string    `json:"machine_id" gorm:"uniqueIndex"`
	LastBrowseSyncTime   time.Time `json:"last_browse_sync_time"`
	LastDownloadSyncTime time.Time `json:"last_download_sync_time"`
	BrowseRecordCount    int64     `json:"browse_record_count"`
	DownloadRecordCount  int64     `json:"download_record_count"`
	LastSyncStatus       string    `json:"last_sync_status"` // success, failed, in_progress, never
	LastSyncError        string    `json:"last_sync_error"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// SyncHistory 同步历史记录
type SyncHistory struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	MachineID     string    `json:"machine_id" gorm:"index"`
	SyncTime      time.Time `json:"sync_time"`
	SyncType      string    `json:"sync_type"` // browse, download
	RecordsSynced int       `json:"records_synced"`
	Status        string    `json:"status"` // success, failed
	ErrorMessage  string    `json:"error_message"`
	CreatedAt     time.Time `json:"created_at"`
}

// TableName 指定表名
func (HubBrowseHistory) TableName() string {
	return "hub_browse_history"
}

func (HubDownloadRecord) TableName() string {
	return "hub_download_records"
}

func (SyncStatus) TableName() string {
	return "sync_status"
}

func (SyncHistory) TableName() string {
	return "sync_history"
}
