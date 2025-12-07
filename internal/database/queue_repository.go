package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// QueueRepository handles download queue database operations
type QueueRepository struct {
	db *sql.DB
}

// NewQueueRepository creates a new QueueRepository
func NewQueueRepository() *QueueRepository {
	return &QueueRepository{db: GetDB()}
}

// Add inserts a new queue item
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

// GetByID retrieves a queue item by ID
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

// Update updates an existing queue item
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

// Remove removes a queue item by ID
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

// RemoveMany removes multiple queue items by IDs
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

// Clear removes all queue items
func (r *QueueRepository) Clear() error {
	_, err := r.db.Exec("DELETE FROM download_queue")
	if err != nil {
		return fmt.Errorf("failed to clear download queue: %w", err)
	}
	return nil
}

// List retrieves all queue items sorted by priority and added time
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

// ListByStatus retrieves queue items with a specific status
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

// UpdateStatus updates the status of a queue item
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

// UpdateProgress updates the download progress of a queue item
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

// Reorder updates the priority of queue items based on the new order
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

// Count returns the total number of queue items
func (r *QueueRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM download_queue").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count queue items: %w", err)
	}
	return count, nil
}

// CountByStatus returns the count of queue items with a specific status
func (r *QueueRepository) CountByStatus(status string) (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM download_queue WHERE status = ?", status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count queue items by status: %w", err)
	}
	return count, nil
}

// GetNextPending retrieves the next pending queue item
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

// IncrementRetryCount increments the retry count for a queue item
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

// SetStartTime sets the start time for a queue item
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

// SetError sets the error message for a queue item
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
