package database

import (
	"time"
)

// BrowseRecord represents a video browse history record
type BrowseRecord struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	AuthorID     string    `json:"authorId"`
	Duration     int64     `json:"duration"`
	Size         int64     `json:"size"`
	Resolution   string    `json:"resolution"`   // Video resolution (e.g., "1080p")
	CoverURL     string    `json:"coverUrl"`
	VideoURL     string    `json:"videoUrl"`
	DecryptKey   string    `json:"decryptKey"`   // Decrypt key for encrypted videos
	BrowseTime   time.Time `json:"browseTime"`
	LikeCount    int64     `json:"likeCount"`
	CommentCount int64     `json:"commentCount"`
	FavCount     int64     `json:"favCount"`
	ForwardCount int64     `json:"forwardCount"`
	PageURL      string    `json:"pageUrl"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// DownloadRecord represents a video download record
type DownloadRecord struct {
	ID           string    `json:"id"`
	VideoID      string    `json:"videoId"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	CoverURL     string    `json:"coverUrl"`     // Cover image URL
	Duration     int64     `json:"duration"`
	FileSize     int64     `json:"fileSize"`
	FilePath     string    `json:"filePath"`
	Format       string    `json:"format"`
	Resolution   string    `json:"resolution"`
	Status       string    `json:"status"` // pending, in_progress, completed, failed
	DownloadTime time.Time `json:"downloadTime"`
	ErrorMessage string    `json:"errorMessage"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// DownloadStatus constants
const (
	DownloadStatusPending    = "pending"
	DownloadStatusInProgress = "in_progress"
	DownloadStatusCompleted  = "completed"
	DownloadStatusFailed     = "failed"
)

// QueueItem represents a download queue item
type QueueItem struct {
	ID              string    `json:"id"`
	VideoID         string    `json:"videoId"`
	Title           string    `json:"title"`
	Author          string    `json:"author"`
	CoverURL        string    `json:"coverUrl"`        // Cover image URL
	VideoURL        string    `json:"videoUrl"`
	DecryptKey      string    `json:"decryptKey"`      // Decrypt key for encrypted videos
	Duration        int64     `json:"duration"`        // Video duration in seconds
	Resolution      string    `json:"resolution"`      // Video resolution (e.g., "1080p")
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

// QueueStatus constants
const (
	QueueStatusPending     = "pending"
	QueueStatusDownloading = "downloading"
	QueueStatusPaused      = "paused"
	QueueStatusCompleted   = "completed"
	QueueStatusFailed      = "failed"
)

// Settings represents application settings
type Settings struct {
	DownloadDir        string `json:"downloadDir"`
	ChunkSize          int64  `json:"chunkSize"`
	ConcurrentLimit    int    `json:"concurrentLimit"`
	AutoCleanupEnabled bool   `json:"autoCleanupEnabled"`
	AutoCleanupDays    int    `json:"autoCleanupDays"`
	MaxRetries         int    `json:"maxRetries"`
	Theme              string `json:"theme"`
}

// DefaultSettings returns default settings
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

// PaginationParams represents pagination parameters
type PaginationParams struct {
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	SortBy   string `json:"sortBy"`
	SortDesc bool   `json:"sortDesc"`
}

// FilterParams represents filter parameters for download records
type FilterParams struct {
	PaginationParams
	StartDate *time.Time `json:"startDate"`
	EndDate   *time.Time `json:"endDate"`
	Status    string     `json:"status"`
	Query     string     `json:"query"`
}

// PagedResult represents a paginated result
type PagedResult[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	TotalPages int   `json:"totalPages"`
}

// NewPagedResult creates a new paged result
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
