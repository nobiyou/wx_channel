package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DownloadRecordRepository 处理下载记录数据库操作
type DownloadRecordRepository struct {
	db *sql.DB
}

// NewDownloadRecordRepository 创建一个新的 DownloadRecordRepository
func NewDownloadRecordRepository() *DownloadRecordRepository {
	return &DownloadRecordRepository{db: GetDB()}
}

// Create 插入新的下载记录
func (r *DownloadRecordRepository) Create(record *DownloadRecord) error {
	now := time.Now()
	record.CreatedAt = now
	record.UpdatedAt = now

	query := `
		INSERT OR REPLACE INTO download_records (
			id, video_id, title, author, cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			like_count, comment_count, forward_count, fav_count,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		record.ID, record.VideoID, record.Title, record.Author, record.CoverURL,
		record.Duration, record.FileSize, record.FilePath, record.Format,
		record.Resolution, record.Status, record.DownloadTime,
		record.ErrorMessage,
		record.LikeCount, record.CommentCount, record.ForwardCount, record.FavCount,
		record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create download record: %w", err)
	}
	return nil
}

// GetByID 根据 ID 获取下载记录
func (r *DownloadRecordRepository) GetByID(id string) (*DownloadRecord, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			like_count, comment_count, forward_count, fav_count,
			created_at, updated_at
		FROM download_records WHERE id = ?
	`
	record := &DownloadRecord{}
	var filePath, format, resolution, errorMessage, coverURL sql.NullString
	err := r.db.QueryRow(query, id).Scan(
		&record.ID, &record.VideoID, &record.Title, &record.Author, &coverURL,
		&record.Duration, &record.FileSize, &filePath, &format,
		&resolution, &record.Status, &record.DownloadTime,
		&errorMessage,
		&record.LikeCount, &record.CommentCount, &record.ForwardCount, &record.FavCount,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get download record: %w", err)
	}
	record.CoverURL = coverURL.String
	record.FilePath = filePath.String
	record.Format = format.String
	record.Resolution = resolution.String
	record.ErrorMessage = errorMessage.String
	return record, nil
}

// Update 更新现有的下载记录
func (r *DownloadRecordRepository) Update(record *DownloadRecord) error {
	record.UpdatedAt = time.Now()

	query := `
		UPDATE download_records SET
			video_id = ?, title = ?, author = ?, cover_url = ?, duration = ?, file_size = ?,
			file_path = ?, format = ?, resolution = ?, status = ?,
			download_time = ?, error_message = ?, updated_at = ?
		WHERE id = ?
	`
	result, err := r.db.Exec(query,
		record.VideoID, record.Title, record.Author, record.CoverURL, record.Duration,
		record.FileSize, record.FilePath, record.Format, record.Resolution,
		record.Status, record.DownloadTime, record.ErrorMessage,
		record.UpdatedAt, record.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update download record: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("download record not found: %s", record.ID)
	}
	return nil
}

// Delete 根据 ID 删除下载记录
func (r *DownloadRecordRepository) Delete(id string) error {
	query := "DELETE FROM download_records WHERE id = ?"
	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete download record: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("download record not found: %s", id)
	}
	return nil
}

// DeleteMany 根据 ID 删除多条下载记录
func (r *DownloadRecordRepository) DeleteMany(ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM download_records WHERE id IN (%s)", strings.Join(placeholders, ","))
	result, err := r.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete download records: %w", err)
	}
	return result.RowsAffected()
}

// Clear 删除所有下载记录
func (r *DownloadRecordRepository) Clear() error {
	_, err := r.db.Exec("DELETE FROM download_records")
	if err != nil {
		return fmt.Errorf("failed to clear download records: %w", err)
	}
	return nil
}

// List 获取分页、过滤和排序的下载记录
func (r *DownloadRecordRepository) List(params *FilterParams) (*PagedResult[DownloadRecord], error) {
	// Set defaults
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	if params.SortBy == "" {
		params.SortBy = "download_time"
	}

	// Validate sort column
	validColumns := map[string]bool{
		"download_time": true, "title": true, "author": true,
		"file_size": true, "status": true, "created_at": true,
	}
	if !validColumns[params.SortBy] {
		params.SortBy = "download_time"
	}

	// Build WHERE clause
	var conditions []string
	var args []interface{}

	if params.StartDate != nil {
		conditions = append(conditions, "download_time >= ?")
		args = append(args, *params.StartDate)
	}
	if params.EndDate != nil {
		conditions = append(conditions, "download_time <= ?")
		args = append(args, *params.EndDate)
	}
	if params.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, params.Status)
	}
	if params.Query != "" {
		conditions = append(conditions, "(title LIKE ? OR author LIKE ?)")
		searchPattern := "%" + params.Query + "%"
		args = append(args, searchPattern, searchPattern)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM download_records %s", whereClause)
	err := r.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count download records: %w", err)
	}

	// Build query
	sortOrder := "DESC"
	if !params.SortDesc {
		sortOrder = "ASC"
	}
	offset := (params.Page - 1) * params.PageSize

	query := fmt.Sprintf(`
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			like_count, comment_count, forward_count, fav_count,
			created_at, updated_at
		FROM download_records
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, params.SortBy, sortOrder)

	args = append(args, params.PageSize, offset)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list download records: %w", err)
	}
	defer rows.Close()

	var records []DownloadRecord
	for rows.Next() {
		var record DownloadRecord
		var filePath, format, resolution, errorMessage, coverURL sql.NullString
		err := rows.Scan(
			&record.ID, &record.VideoID, &record.Title, &record.Author, &coverURL,
			&record.Duration, &record.FileSize, &filePath, &format,
			&resolution, &record.Status, &record.DownloadTime,
			&errorMessage,
			&record.LikeCount, &record.CommentCount, &record.ForwardCount, &record.FavCount,
			&record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan download record: %w", err)
		}
		record.CoverURL = coverURL.String
		record.FilePath = filePath.String
		record.Format = format.String
		record.Resolution = resolution.String
		record.ErrorMessage = errorMessage.String
		records = append(records, record)
	}

	if records == nil {
		records = []DownloadRecord{}
	}

	return NewPagedResult(records, total, params.Page, params.PageSize), nil
}

// Count 返回下载记录的总数
func (r *DownloadRecordRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM download_records").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count download records: %w", err)
	}
	return count, nil
}

// CountByStatus 返回指定状态的记录数
func (r *DownloadRecordRepository) CountByStatus(status string) (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM download_records WHERE status = ?", status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count download records by status: %w", err)
	}
	return count, nil
}

// CountToday 返回今天下载的记录数
func (r *DownloadRecordRepository) CountToday() (int64, error) {
	var count int64
	today := time.Now().Format("2006-01-02")
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM download_records WHERE date(download_time) = ?",
		today,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count today's download records: %w", err)
	}
	return count, nil
}

// GetRecent 获取最近的下载记录
func (r *DownloadRecordRepository) GetRecent(limit int) ([]DownloadRecord, error) {
	if limit < 1 {
		limit = 5
	}

	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			like_count, comment_count, forward_count, fav_count,
			created_at, updated_at
		FROM download_records
		ORDER BY download_time DESC
		LIMIT ?
	`

	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent download records: %w", err)
	}
	defer rows.Close()

	var records []DownloadRecord
	for rows.Next() {
		var record DownloadRecord
		var filePath, format, resolution, errorMessage, coverURL sql.NullString
		err := rows.Scan(
			&record.ID, &record.VideoID, &record.Title, &record.Author, &coverURL,
			&record.Duration, &record.FileSize, &filePath, &format,
			&resolution, &record.Status, &record.DownloadTime,
			&errorMessage,
			&record.LikeCount, &record.CommentCount, &record.ForwardCount, &record.FavCount,
			&record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan download record: %w", err)
		}
		record.CoverURL = coverURL.String
		record.FilePath = filePath.String
		record.Format = format.String
		record.Resolution = resolution.String
		record.ErrorMessage = errorMessage.String
		records = append(records, record)
	}

	if records == nil {
		records = []DownloadRecord{}
	}

	return records, nil
}

// DeleteBefore 删除指定日期前的所有记录
func (r *DownloadRecordRepository) DeleteBefore(date time.Time) (int64, error) {
	result, err := r.db.Exec("DELETE FROM download_records WHERE download_time < ?", date)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old download records: %w", err)
	}
	return result.RowsAffected()
}

// GetAll 获取所有下载记录（用于导出）
func (r *DownloadRecordRepository) GetAll() ([]DownloadRecord, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			like_count, comment_count, forward_count, fav_count,
			created_at, updated_at
		FROM download_records
		ORDER BY download_time DESC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all download records: %w", err)
	}
	defer rows.Close()

	var records []DownloadRecord
	for rows.Next() {
		var record DownloadRecord
		var filePath, format, resolution, errorMessage, coverURL sql.NullString
		err := rows.Scan(
			&record.ID, &record.VideoID, &record.Title, &record.Author, &coverURL,
			&record.Duration, &record.FileSize, &filePath, &format,
			&resolution, &record.Status, &record.DownloadTime,
			&errorMessage,
			&record.LikeCount, &record.CommentCount, &record.ForwardCount, &record.FavCount,
			&record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan download record: %w", err)
		}
		record.CoverURL = coverURL.String
		record.FilePath = filePath.String
		record.Format = format.String
		record.Resolution = resolution.String
		record.ErrorMessage = errorMessage.String
		records = append(records, record)
	}

	if records == nil {
		records = []DownloadRecord{}
	}

	return records, nil
}

// GetByIDs 根据 ID 获取下载记录
func (r *DownloadRecordRepository) GetByIDs(ids []string) ([]DownloadRecord, error) {
	if len(ids) == 0 {
		return []DownloadRecord{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			like_count, comment_count, forward_count, fav_count,
			created_at, updated_at
		FROM download_records
		WHERE id IN (%s)
		ORDER BY download_time DESC
	`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get download records by IDs: %w", err)
	}
	defer rows.Close()

	var records []DownloadRecord
	for rows.Next() {
		var record DownloadRecord
		var filePath, format, resolution, errorMessage, coverURL sql.NullString
		err := rows.Scan(
			&record.ID, &record.VideoID, &record.Title, &record.Author, &coverURL,
			&record.Duration, &record.FileSize, &filePath, &format,
			&resolution, &record.Status, &record.DownloadTime,
			&errorMessage,
			&record.LikeCount, &record.CommentCount, &record.ForwardCount, &record.FavCount,
			&record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan download record: %w", err)
		}
		record.CoverURL = coverURL.String
		record.FilePath = filePath.String
		record.Format = format.String
		record.Resolution = resolution.String
		record.ErrorMessage = errorMessage.String
		records = append(records, record)
	}

	if records == nil {
		records = []DownloadRecord{}
	}

	return records, nil
}

// GetChartData 返回过去 N 天的下载统计
func (r *DownloadRecordRepository) GetChartData(days int) ([]string, []int64, error) {
	if days < 1 {
		days = 7
	}

	labels := make([]string, days)
	values := make([]int64, days)

	for i := days - 1; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		labels[days-1-i] = dateStr

		var count int64
		err := r.db.QueryRow(
			"SELECT COUNT(*) FROM download_records WHERE date(download_time) = ?",
			dateStr,
		).Scan(&count)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get chart data: %w", err)
		}
		values[days-1-i] = count
	}

	return labels, values, nil
}

// GetTotalFileSize 返回所有已完成下载的总文件大小
func (r *DownloadRecordRepository) GetTotalFileSize() (int64, error) {
	var total sql.NullInt64
	err := r.db.QueryRow(
		"SELECT SUM(file_size) FROM download_records WHERE status = ?",
		DownloadStatusCompleted,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total file size: %w", err)
	}
	return total.Int64, nil
}

// GetRecordsSince 获取指定时间之后的下载记录（用于增量同步）
func (r *DownloadRecordRepository) GetRecordsSince(since time.Time, limit int) ([]DownloadRecord, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			like_count, comment_count, forward_count, fav_count,
			created_at, updated_at
		FROM download_records
		WHERE updated_at > ?
		ORDER BY updated_at ASC
		LIMIT ?
	`

	rows, err := r.db.Query(query, since, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get records since: %w", err)
	}
	defer rows.Close()

	var records []DownloadRecord
	for rows.Next() {
		var record DownloadRecord
		var filePath, format, resolution, errorMessage, coverURL sql.NullString
		err := rows.Scan(
			&record.ID, &record.VideoID, &record.Title, &record.Author, &coverURL,
			&record.Duration, &record.FileSize, &filePath, &format,
			&resolution, &record.Status, &record.DownloadTime,
			&errorMessage,
			&record.LikeCount, &record.CommentCount, &record.ForwardCount, &record.FavCount,
			&record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan download record: %w", err)
		}
		record.CoverURL = coverURL.String
		record.FilePath = filePath.String
		record.Format = format.String
		record.Resolution = resolution.String
		record.ErrorMessage = errorMessage.String
		records = append(records, record)
	}

	if records == nil {
		records = []DownloadRecord{}
	}

	return records, nil
}

// GetLatestTimestamp 获取最新记录的时间戳（用于增量同步）
func (r *DownloadRecordRepository) GetLatestTimestamp() (time.Time, error) {
	var timestamp time.Time
	err := r.db.QueryRow("SELECT COALESCE(MAX(updated_at), '1970-01-01') FROM download_records").Scan(&timestamp)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get latest timestamp: %w", err)
	}
	return timestamp, nil
}
