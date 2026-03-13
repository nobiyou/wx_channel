package database

import (
	"time"
)

// BrowseRecord 表示视频浏览历史记录
type BrowseRecord struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	AuthorID     string    `json:"authorId"`
	Duration     int64     `json:"duration"`
	Size         int64     `json:"size"`
	Resolution   string    `json:"resolution"` // 视频分辨率（例如 "1080p"）
	FileFormat   string    `json:"fileFormat"` // 视频格式标识（例如 "xWT128", "xWT111"）
	CoverURL     string    `json:"coverUrl"`
	VideoURL     string    `json:"videoUrl"`
	DecryptKey   string    `json:"decryptKey"` // 加密视频的解密密钥
	BrowseTime   time.Time `json:"browseTime"`
	LikeCount    int64     `json:"likeCount"`
	CommentCount int64     `json:"commentCount"`
	FavCount     int64     `json:"favCount"`
	ForwardCount int64     `json:"forwardCount"`
	PageURL      string    `json:"pageUrl"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// DownloadRecord 表示视频下载记录
type DownloadRecord struct {
	ID           string    `json:"id"`
	VideoID      string    `json:"videoId"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	CoverURL     string    `json:"coverUrl"` // 封面图片 URL
	Duration     int64     `json:"duration"`
	FileSize     int64     `json:"fileSize"`
	FilePath     string    `json:"filePath"`
	Format       string    `json:"format"`
	Resolution   string    `json:"resolution"`
	Status       string    `json:"status"` // pending, in_progress, completed, failed
	DownloadTime time.Time `json:"downloadTime"`
	ErrorMessage string    `json:"errorMessage"`
	LikeCount    int64     `json:"likeCount"`
	CommentCount int64     `json:"commentCount"`
	ForwardCount int64     `json:"forwardCount"`
	FavCount     int64     `json:"favCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// DownloadStatus 常量
const (
	DownloadStatusPending    = "pending"
	DownloadStatusInProgress = "in_progress"
	DownloadStatusCompleted  = "completed"
	DownloadStatusFailed     = "failed"
)

// QueueItem 表示下载队列项目
type QueueItem struct {
	ID              string    `json:"id"`
	VideoID         string    `json:"videoId"`
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	CoverURL        string    `json:"coverUrl"` // 封面图片 URL
	VideoURL        string    `json:"videoUrl"`
	DecryptKey      string    `json:"decryptKey"` // 加密视频的解密密钥
	Duration        int64     `json:"duration"`   // 视频时长（秒）
	Resolution      string    `json:"resolution"` // 视频分辨率（例如 "1080p"）
	TotalSize       int64     `json:"totalSize"`
	DownloadedSize  int64     `json:"downloadedSize"`
	Status          string    `json:"status"` // pending, downloading, paused, completed, failed
	Priority        int       `json:"priority"`
	AddedTime       time.Time `json:"addedTime"`
	StartTime       time.Time `json:"startTime"`
	Speed           int64     `json:"speed"`
	ChunkSize       int64     `json:"chunkSize"`
	ChunksTotal     int       `json:"chunksTotal"`
	ChunksCompleted int       `json:"chunksCompleted"`
	RetryCount      int       `json:"retryCount"`
	ErrorMessage    string    `json:"errorMessage"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// QueueStatus 常量
const (
	QueueStatusPending     = "pending"
	QueueStatusDownloading = "downloading"
	QueueStatusPaused      = "paused"
	QueueStatusCompleted   = "completed"
	QueueStatusFailed      = "failed"
)

// Settings 表示应用程序设置
type Settings struct {
	DownloadDir        string `json:"downloadDir"`
	ChunkSize          int64  `json:"chunkSize"`
	ConcurrentLimit    int    `json:"concurrentLimit"`
	AutoCleanupEnabled bool   `json:"autoCleanupEnabled"`
	AutoCleanupDays    int    `json:"autoCleanupDays"`
	MaxRetries         int    `json:"maxRetries"`
	Theme              string `json:"theme"`
}

// DefaultSettings 返回默认设置
func DefaultSettings() *Settings {
	return &Settings{
		DownloadDir:        "downloads",
		ChunkSize:          10 * 1024 * 1024, // 10MB
		ConcurrentLimit:    3,
		AutoCleanupEnabled: false,
		AutoCleanupDays:    30,
		MaxRetries:         3,
		Theme:              "light",
	}
}

// PaginationParams 表示分页参数
type PaginationParams struct {
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	SortBy   string `json:"sortBy"`
	SortDesc bool   `json:"sortDesc"`
}

// FilterParams 表示下载记录的过滤参数
type FilterParams struct {
	PaginationParams
	StartDate *time.Time `json:"startDate"`
	EndDate   *time.Time `json:"endDate"`
	Status    string     `json:"status"`
	Query     string     `json:"query"`
}

// PagedResult 表示分页结果
type PagedResult[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	TotalPages int   `json:"totalPages"`
}

// NewPagedResult 创建一个新的分页结果
func NewPagedResult[T any](items []T, total int64, page, pageSize int) *PagedResult[T] {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}
	return &PagedResult[T]{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// RadarLog 表示雷达监控的单次执行记录
type RadarLog struct {
	ID           string    `json:"id"`
	TargetID     string    `json:"target_id"`
	CheckTime    time.Time `json:"check_time"`
	FoundVideos  int       `json:"found_videos"`
	NewVideos    int       `json:"new_videos"`
	Status       string    `json:"status"` // success 或 error
	ErrorMessage string    `json:"error_message"`
	VideoList    string    `json:"video_list"` // JSON 数组，存储每个视频的详情摘要
}

// RadarVideoSummary 单次扫描中某个视频的摘要信息
type RadarVideoSummary struct {
	VideoID string `json:"video_id"`
	Title   string `json:"title"`
	IsNew   bool   `json:"is_new"` // true=新视频并已加入队列，false=已存在
}
