package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// QueueRepository 处理下载队列数据库操作
type QueueRepository struct {
	db *sql.DB
}

// NewQueueRepository 创建一个新的 QueueRepository
func NewQueueRepository() *QueueRepository {
	return &QueueRepository{db: GetDB()}
}

// Add 插入新的队列项目
func (r *QueueRepository) Add(item *QueueItem) error {
	now := time.Now()
	item.CreatedAt = now
	item.UpdatedAt = now

	query := `
		INSERT INTO download_queue (
			id, video_id, title, author, cover_url, video_url, decrypt_key, duration, resolution, total_size, downloaded_size,
			status, priority, added_time, start_time, speed, chunk_size,
			chunks_total, chunks_completed, retry_count, error_message,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		item.ID, item.VideoID, item.Title, item.Author, item.CoverURL, item.VideoURL, item.DecryptKey,
		item.Duration, item.Resolution, item.TotalSize, item.DownloadedSize, item.Status, item.Priority,
		item.AddedTime, item.StartTime, item.Speed, item.ChunkSize,
		item.ChunksTotal, item.ChunksCompleted, item.RetryCount,
		item.ErrorMessage, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to add queue item: %w", err)
	}
	return nil
}

// GetByID 根据 ID 获取队列项目
func (r *QueueRepository) GetByID(id string) (*QueueItem, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, video_url, decrypt_key, 
			COALESCE(duration, 0) as duration, COALESCE(resolution, '') as resolution, total_size, downloaded_size,
			status, priority, added_time, start_time, speed, chunk_size,
			chunks_total, chunks_completed, retry_count, error_message,
			created_at, updated_at
		FROM download_queue WHERE id = ?
	`
	item := &QueueItem{}
	var startTime sql.NullTime
	var errorMessage sql.NullString
	var decryptKey sql.NullString
	var coverURL sql.NullString
	var resolution sql.NullString
	err := r.db.QueryRow(query, id).Scan(
		&item.ID, &item.VideoID, &item.Title, &item.Author, &coverURL, &item.VideoURL, &decryptKey,
		&item.Duration, &resolution, &item.TotalSize, &item.DownloadedSize, &item.Status, &item.Priority,
		&item.AddedTime, &startTime, &item.Speed, &item.ChunkSize,
		&item.ChunksTotal, &item.ChunksCompleted, &item.RetryCount,
		&errorMessage, &item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get queue item: %w", err)
	}
	if startTime.Valid {
		item.StartTime = startTime.Time
	}
	item.CoverURL = coverURL.String
	item.Resolution = resolution.String
	item.ErrorMessage = errorMessage.String
	item.DecryptKey = decryptKey.String
	return item, nil
}

// GetByVideoID 根据 VideoID 获取队列项目
func (r *QueueRepository) GetByVideoID(videoID string) (*QueueItem, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, video_url, decrypt_key, 
			COALESCE(duration, 0) as duration, COALESCE(resolution, '') as resolution, total_size, downloaded_size,
			status, priority, added_time, start_time, speed, chunk_size,
			chunks_total, chunks_completed, retry_count, error_message,
			created_at, updated_at
		FROM download_queue WHERE video_id = ? LIMIT 1
	`
	item := &QueueItem{}
	var startTime sql.NullTime
	var errorMessage sql.NullString
	var decryptKey sql.NullString
	var coverURL sql.NullString
	var resolution sql.NullString
	err := r.db.QueryRow(query, videoID).Scan(
		&item.ID, &item.VideoID, &item.Title, &item.Author, &coverURL, &item.VideoURL, &decryptKey,
		&item.Duration, &resolution, &item.TotalSize, &item.DownloadedSize, &item.Status, &item.Priority,
		&item.AddedTime, &startTime, &item.Speed, &item.ChunkSize,
		&item.ChunksTotal, &item.ChunksCompleted, &item.RetryCount,
		&errorMessage, &item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get queue item by video id: %w", err)
	}
	if startTime.Valid {
		item.StartTime = startTime.Time
	}
	item.CoverURL = coverURL.String
	item.Resolution = resolution.String
	item.ErrorMessage = errorMessage.String
	item.DecryptKey = decryptKey.String
	return item, nil
}

// Update 更新现有的队列项目
func (r *QueueRepository) Update(item *QueueItem) error {
	item.UpdatedAt = time.Now()

	query := `
		UPDATE download_queue SET
			video_id = ?, title = ?, author = ?, cover_url = ?, video_url = ?, decrypt_key = ?, duration = ?, total_size = ?,
			downloaded_size = ?, status = ?, priority = ?, added_time = ?,
			start_time = ?, speed = ?, chunk_size = ?, chunks_total = ?,
			chunks_completed = ?, retry_count = ?, error_message = ?, updated_at = ?
		WHERE id = ?
	`
	result, err := r.db.Exec(query,
		item.VideoID, item.Title, item.Author, item.CoverURL, item.VideoURL, item.DecryptKey, item.Duration, item.TotalSize,
		item.DownloadedSize, item.Status, item.Priority, item.AddedTime,
		item.StartTime, item.Speed, item.ChunkSize, item.ChunksTotal,
		item.ChunksCompleted, item.RetryCount, item.ErrorMessage,
		item.UpdatedAt, item.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update queue item: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("queue item not found: %s", item.ID)
	}
	return nil
}

// Remove 根据 ID 删除队列项目
func (r *QueueRepository) Remove(id string) error {
	query := "DELETE FROM download_queue WHERE id = ?"
	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to remove queue item: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("queue item not found: %s", id)
	}
	return nil
}

// RemoveMany 根据 ID 删除多个队列项目
func (r *QueueRepository) RemoveMany(ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM download_queue WHERE id IN (%s)", strings.Join(placeholders, ","))
	result, err := r.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to remove queue items: %w", err)
	}
	return result.RowsAffected()
}

// Clear 删除所有队列项目
func (r *QueueRepository) Clear() error {
	_, err := r.db.Exec("DELETE FROM download_queue")
	if err != nil {
		return fmt.Errorf("failed to clear download queue: %w", err)
	}
	return nil
}

// List 获取按优先级和添加时间排序的所有队列项目
func (r *QueueRepository) List() ([]QueueItem, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, video_url, decrypt_key, 
			COALESCE(duration, 0) as duration, total_size, downloaded_size,
			status, priority, added_time, start_time, speed, chunk_size,
			chunks_total, chunks_completed, retry_count, error_message,
			created_at, updated_at
		FROM download_queue
		ORDER BY priority DESC, added_time ASC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list queue items: %w", err)
	}
	defer rows.Close()

	var items []QueueItem
	for rows.Next() {
		var item QueueItem
		var startTime sql.NullTime
		var errorMessage sql.NullString
		var decryptKey sql.NullString
		var coverURL sql.NullString
		err := rows.Scan(
			&item.ID, &item.VideoID, &item.Title, &item.Author, &coverURL, &item.VideoURL, &decryptKey,
			&item.Duration, &item.TotalSize, &item.DownloadedSize, &item.Status, &item.Priority,
			&item.AddedTime, &startTime, &item.Speed, &item.ChunkSize,
			&item.ChunksTotal, &item.ChunksCompleted, &item.RetryCount,
			&errorMessage, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue item: %w", err)
		}
		if startTime.Valid {
			item.StartTime = startTime.Time
		}
		item.CoverURL = coverURL.String
		item.ErrorMessage = errorMessage.String
		item.DecryptKey = decryptKey.String
		items = append(items, item)
	}

	if items == nil {
		items = []QueueItem{}
	}

	return items, nil
}

// ListByStatus 获取指定状态的队列项目
func (r *QueueRepository) ListByStatus(status string) ([]QueueItem, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, video_url, decrypt_key, 
			COALESCE(duration, 0) as duration, total_size, downloaded_size,
			status, priority, added_time, start_time, speed, chunk_size,
			chunks_total, chunks_completed, retry_count, error_message,
			created_at, updated_at
		FROM download_queue
		WHERE status = ?
		ORDER BY priority DESC, added_time ASC
	`

	rows, err := r.db.Query(query, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list queue items by status: %w", err)
	}
	defer rows.Close()

	var items []QueueItem
	for rows.Next() {
		var item QueueItem
		var startTime sql.NullTime
		var errorMessage sql.NullString
		var decryptKey sql.NullString
		var coverURL sql.NullString
		err := rows.Scan(
			&item.ID, &item.VideoID, &item.Title, &item.Author, &coverURL, &item.VideoURL, &decryptKey,
			&item.Duration, &item.TotalSize, &item.DownloadedSize, &item.Status, &item.Priority,
			&item.AddedTime, &startTime, &item.Speed, &item.ChunkSize,
			&item.ChunksTotal, &item.ChunksCompleted, &item.RetryCount,
			&errorMessage, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue item: %w", err)
		}
		if startTime.Valid {
			item.StartTime = startTime.Time
		}
		item.CoverURL = coverURL.String
		item.ErrorMessage = errorMessage.String
		item.DecryptKey = decryptKey.String
		items = append(items, item)
	}

	if items == nil {
		items = []QueueItem{}
	}

	return items, nil
}

// UpdateStatus 更新队列项目的状态
func (r *QueueRepository) UpdateStatus(id string, status string) error {
	query := "UPDATE download_queue SET status = ?, updated_at = ? WHERE id = ?"
	result, err := r.db.Exec(query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update queue item status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("queue item not found: %s", id)
	}
	return nil
}

// UpdateProgress 更新队列项目的下载进度
func (r *QueueRepository) UpdateProgress(id string, downloadedSize int64, chunksCompleted int, speed int64) error {
	query := `
		UPDATE download_queue SET
			downloaded_size = ?, chunks_completed = ?, speed = ?, updated_at = ?
		WHERE id = ?
	`
	result, err := r.db.Exec(query, downloadedSize, chunksCompleted, speed, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update queue item progress: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("queue item not found: %s", id)
	}
	return nil
}

// Reorder 根据新顺序更新队列项目的优先级
func (r *QueueRepository) Reorder(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update priorities in reverse order (highest priority first)
	for i, id := range ids {
		priority := len(ids) - i
		_, err := tx.Exec(
			"UPDATE download_queue SET priority = ?, updated_at = ? WHERE id = ?",
			priority, time.Now(), id,
		)
		if err != nil {
			return fmt.Errorf("failed to update queue item priority: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Count 返回队列项目的总数
func (r *QueueRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM download_queue").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count queue items: %w", err)
	}
	return count, nil
}

// CountByStatus 返回指定状态的队列项目数
func (r *QueueRepository) CountByStatus(status string) (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM download_queue WHERE status = ?", status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count queue items by status: %w", err)
	}
	return count, nil
}

// GetQueueStats 返回队列的所有状态统计聚合
func (r *QueueRepository) GetQueueStats() (total, pending, downloading, paused, completed, failed int64, err error) {
	query := `
		SELECT 
			COUNT(*) as total,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as pending,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as downloading,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as paused,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as completed,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) as failed
		FROM download_queue
	`
	err = r.db.QueryRow(query,
		QueueStatusPending,
		QueueStatusDownloading,
		QueueStatusPaused,
		QueueStatusCompleted,
		QueueStatusFailed,
	).Scan(&total, &pending, &downloading, &paused, &completed, &failed)

	if err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("failed to get aggregated queue stats: %w", err)
	}
	return total, pending, downloading, paused, completed, failed, nil
}

// GetNextPending 获取下一个待处理的队列项目
func (r *QueueRepository) GetNextPending() (*QueueItem, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, video_url, decrypt_key, 
			COALESCE(duration, 0) as duration, total_size, downloaded_size,
			status, priority, added_time, start_time, speed, chunk_size,
			chunks_total, chunks_completed, retry_count, error_message,
			created_at, updated_at
		FROM download_queue
		WHERE status = ?
		ORDER BY priority DESC, added_time ASC
		LIMIT 1
	`
	item := &QueueItem{}
	var startTime sql.NullTime
	var errorMessage sql.NullString
	var decryptKey sql.NullString
	var coverURL sql.NullString
	err := r.db.QueryRow(query, QueueStatusPending).Scan(
		&item.ID, &item.VideoID, &item.Title, &item.Author, &coverURL, &item.VideoURL, &decryptKey,
		&item.Duration, &item.TotalSize, &item.DownloadedSize, &item.Status, &item.Priority,
		&item.AddedTime, &startTime, &item.Speed, &item.ChunkSize,
		&item.ChunksTotal, &item.ChunksCompleted, &item.RetryCount,
		&errorMessage, &item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get next pending queue item: %w", err)
	}
	if startTime.Valid {
		item.StartTime = startTime.Time
	}
	item.CoverURL = coverURL.String
	item.ErrorMessage = errorMessage.String
	item.DecryptKey = decryptKey.String
	return item, nil
}

// IncrementRetryCount 增加队列项目的重试计数
func (r *QueueRepository) IncrementRetryCount(id string) error {
	query := "UPDATE download_queue SET retry_count = retry_count + 1, updated_at = ? WHERE id = ?"
	result, err := r.db.Exec(query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("queue item not found: %s", id)
	}
	return nil
}

// SetStartTime 设置队列项目的开始时间
func (r *QueueRepository) SetStartTime(id string, startTime time.Time) error {
	query := "UPDATE download_queue SET start_time = ?, updated_at = ? WHERE id = ?"
	result, err := r.db.Exec(query, startTime, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set start time: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("queue item not found: %s", id)
	}
	return nil
}

// SetError 设置队列项目的错误信息
func (r *QueueRepository) SetError(id string, errorMessage string) error {
	query := "UPDATE download_queue SET error_message = ?, status = ?, updated_at = ? WHERE id = ?"
	result, err := r.db.Exec(query, errorMessage, QueueStatusFailed, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set error: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("queue item not found: %s", id)
	}
	return nil
}
