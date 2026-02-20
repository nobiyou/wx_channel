package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// BrowseHistoryRepository 处理浏览历史数据库操作
type BrowseHistoryRepository struct {
	db *sql.DB
}

// NewBrowseHistoryRepository 创建一个新的 BrowseHistoryRepository
func NewBrowseHistoryRepository() *BrowseHistoryRepository {
	return &BrowseHistoryRepository{db: GetDB()}
}

// Create 插入新的浏览记录
func (r *BrowseHistoryRepository) Create(record *BrowseRecord) error {
	now := time.Now()
	record.CreatedAt = now
	record.UpdatedAt = now

	query := `
		INSERT INTO browse_history (
			id, title, author, author_id, duration, size, resolution, cover_url, video_url,
			decrypt_key, browse_time, like_count, comment_count, fav_count, forward_count, page_url,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		record.ID, record.Title, record.Author, record.AuthorID,
		record.Duration, record.Size, record.Resolution, record.CoverURL, record.VideoURL,
		record.DecryptKey, record.BrowseTime, record.LikeCount, record.CommentCount,
		record.FavCount, record.ForwardCount, record.PageURL, record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create browse record: %w", err)
	}
	return nil
}

// GetByID 根据 ID 获取浏览记录
func (r *BrowseHistoryRepository) GetByID(id string) (*BrowseRecord, error) {
	query := `
		SELECT id, title, author, author_id, duration, size, COALESCE(resolution, '') as resolution, cover_url, video_url,
			decrypt_key, browse_time, like_count, comment_count, 
			COALESCE(fav_count, 0) as fav_count, COALESCE(forward_count, 0) as forward_count, page_url,
			created_at, updated_at
		FROM browse_history WHERE id = ?
	`
	record := &BrowseRecord{}
	err := r.db.QueryRow(query, id).Scan(
		&record.ID, &record.Title, &record.Author, &record.AuthorID,
		&record.Duration, &record.Size, &record.Resolution, &record.CoverURL, &record.VideoURL,
		&record.DecryptKey, &record.BrowseTime, &record.LikeCount, &record.CommentCount,
		&record.FavCount, &record.ForwardCount, &record.PageURL, &record.CreatedAt, &record.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get browse record: %w", err)
	}
	return record, nil
}

// Update 更新现有的浏览记录
func (r *BrowseHistoryRepository) Update(record *BrowseRecord) error {
	record.UpdatedAt = time.Now()

	query := `
		UPDATE browse_history SET
			title = ?, author = ?, author_id = ?, duration = ?, size = ?, resolution = ?,
			cover_url = ?, video_url = ?, decrypt_key = ?, browse_time = ?, like_count = ?,
			comment_count = ?, fav_count = ?, forward_count = ?, page_url = ?, updated_at = ?
		WHERE id = ?
	`
	result, err := r.db.Exec(query,
		record.Title, record.Author, record.AuthorID, record.Duration,
		record.Size, record.Resolution, record.CoverURL, record.VideoURL, record.DecryptKey, record.BrowseTime,
		record.LikeCount, record.CommentCount, record.FavCount, record.ForwardCount,
		record.PageURL, record.UpdatedAt, record.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update browse record: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("browse record not found: %s", record.ID)
	}
	return nil
}

// Delete 根据 ID 删除浏览记录
func (r *BrowseHistoryRepository) Delete(id string) error {
	query := "DELETE FROM browse_history WHERE id = ?"
	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete browse record: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("browse record not found: %s", id)
	}
	return nil
}

// DeleteMany 根据 ID 删除多条浏览记录
func (r *BrowseHistoryRepository) DeleteMany(ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM browse_history WHERE id IN (%s)", strings.Join(placeholders, ","))
	result, err := r.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete browse records: %w", err)
	}
	return result.RowsAffected()
}

// Clear 删除所有浏览记录
func (r *BrowseHistoryRepository) Clear() error {
	_, err := r.db.Exec("DELETE FROM browse_history")
	if err != nil {
		return fmt.Errorf("failed to clear browse history: %w", err)
	}
	return nil
}

// List 获取分页和排序的浏览记录
func (r *BrowseHistoryRepository) List(params *PaginationParams) (*PagedResult[BrowseRecord], error) {
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
		params.SortBy = "browse_time"
	}

	// Validate sort column
	validColumns := map[string]bool{
		"browse_time": true, "title": true, "author": true,
		"duration": true, "size": true, "created_at": true,
	}
	if !validColumns[params.SortBy] {
		params.SortBy = "browse_time"
	}

	// Count total
	var total int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM browse_history").Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count browse records: %w", err)
	}

	// Build query
	sortOrder := "DESC"
	if !params.SortDesc {
		sortOrder = "ASC"
	}
	offset := (params.Page - 1) * params.PageSize

	query := fmt.Sprintf(`
		SELECT id, title, author, author_id, duration, size, COALESCE(resolution, '') as resolution, cover_url, video_url,
			decrypt_key, browse_time, like_count, comment_count, 
			COALESCE(fav_count, 0) as fav_count, COALESCE(forward_count, 0) as forward_count, page_url,
			created_at, updated_at
		FROM browse_history
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, params.SortBy, sortOrder)

	rows, err := r.db.Query(query, params.PageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list browse records: %w", err)
	}
	defer rows.Close()

	var records []BrowseRecord
	for rows.Next() {
		var record BrowseRecord
		err := rows.Scan(
			&record.ID, &record.Title, &record.Author, &record.AuthorID,
			&record.Duration, &record.Size, &record.Resolution, &record.CoverURL, &record.VideoURL,
			&record.DecryptKey, &record.BrowseTime, &record.LikeCount, &record.CommentCount,
			&record.FavCount, &record.ForwardCount, &record.PageURL, &record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan browse record: %w", err)
		}
		records = append(records, record)
	}

	if records == nil {
		records = []BrowseRecord{}
	}

	return NewPagedResult(records, total, params.Page, params.PageSize), nil
}

// Search 根据标题或作者搜索浏览记录
func (r *BrowseHistoryRepository) Search(query string, params *PaginationParams) (*PagedResult[BrowseRecord], error) {
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

	searchPattern := "%" + query + "%"

	// Count total
	var total int64
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM browse_history WHERE title LIKE ? OR author LIKE ?",
		searchPattern, searchPattern,
	).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count search results: %w", err)
	}

	// Build query
	offset := (params.Page - 1) * params.PageSize
	sqlQuery := `
		SELECT id, title, author, author_id, duration, size, COALESCE(resolution, '') as resolution, cover_url, video_url,
			decrypt_key, browse_time, like_count, comment_count, 
			COALESCE(fav_count, 0) as fav_count, COALESCE(forward_count, 0) as forward_count, page_url,
			created_at, updated_at
		FROM browse_history
		WHERE title LIKE ? OR author LIKE ?
		ORDER BY browse_time DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.Query(sqlQuery, searchPattern, searchPattern, params.PageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to search browse records: %w", err)
	}
	defer rows.Close()

	var records []BrowseRecord
	for rows.Next() {
		var record BrowseRecord
		err := rows.Scan(
			&record.ID, &record.Title, &record.Author, &record.AuthorID,
			&record.Duration, &record.Size, &record.Resolution, &record.CoverURL, &record.VideoURL,
			&record.DecryptKey, &record.BrowseTime, &record.LikeCount, &record.CommentCount,
			&record.FavCount, &record.ForwardCount, &record.PageURL, &record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan browse record: %w", err)
		}
		records = append(records, record)
	}

	if records == nil {
		records = []BrowseRecord{}
	}

	return NewPagedResult(records, total, params.Page, params.PageSize), nil
}

// Count 返回浏览记录的总数
func (r *BrowseHistoryRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM browse_history").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count browse records: %w", err)
	}
	return count, nil
}

// GetRecent 获取最近的浏览记录
func (r *BrowseHistoryRepository) GetRecent(limit int) ([]BrowseRecord, error) {
	if limit < 1 {
		limit = 5
	}

	query := `
		SELECT id, title, author, author_id, duration, size, COALESCE(resolution, '') as resolution, cover_url, video_url,
			decrypt_key, browse_time, like_count, comment_count, 
			COALESCE(fav_count, 0) as fav_count, COALESCE(forward_count, 0) as forward_count, page_url,
			created_at, updated_at
		FROM browse_history
		ORDER BY browse_time DESC
		LIMIT ?
	`

	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent browse records: %w", err)
	}
	defer rows.Close()

	var records []BrowseRecord
	for rows.Next() {
		var record BrowseRecord
		err := rows.Scan(
			&record.ID, &record.Title, &record.Author, &record.AuthorID,
			&record.Duration, &record.Size, &record.Resolution, &record.CoverURL, &record.VideoURL,
			&record.DecryptKey, &record.BrowseTime, &record.LikeCount, &record.CommentCount,
			&record.FavCount, &record.ForwardCount, &record.PageURL, &record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan browse record: %w", err)
		}
		records = append(records, record)
	}

	if records == nil {
		records = []BrowseRecord{}
	}

	return records, nil
}

// DeleteBefore 删除指定日期前的所有记录
func (r *BrowseHistoryRepository) DeleteBefore(date time.Time) (int64, error) {
	result, err := r.db.Exec("DELETE FROM browse_history WHERE browse_time < ?", date)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old browse records: %w", err)
	}
	return result.RowsAffected()
}

// GetAll 获取所有浏览记录（用于导出）
func (r *BrowseHistoryRepository) GetAll() ([]BrowseRecord, error) {
	query := `
		SELECT id, title, author, author_id, duration, size, COALESCE(resolution, '') as resolution, cover_url, video_url,
			decrypt_key, browse_time, like_count, comment_count, 
			COALESCE(fav_count, 0) as fav_count, COALESCE(forward_count, 0) as forward_count, page_url,
			created_at, updated_at
		FROM browse_history
		ORDER BY browse_time DESC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all browse records: %w", err)
	}
	defer rows.Close()

	var records []BrowseRecord
	for rows.Next() {
		var record BrowseRecord
		err := rows.Scan(
			&record.ID, &record.Title, &record.Author, &record.AuthorID,
			&record.Duration, &record.Size, &record.Resolution, &record.CoverURL, &record.VideoURL,
			&record.DecryptKey, &record.BrowseTime, &record.LikeCount, &record.CommentCount,
			&record.FavCount, &record.ForwardCount, &record.PageURL, &record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan browse record: %w", err)
		}
		records = append(records, record)
	}

	if records == nil {
		records = []BrowseRecord{}
	}

	return records, nil
}

// GetByIDs 根据 ID 获取浏览记录
func (r *BrowseHistoryRepository) GetByIDs(ids []string) ([]BrowseRecord, error) {
	if len(ids) == 0 {
		return []BrowseRecord{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, title, author, author_id, duration, size, COALESCE(resolution, '') as resolution, cover_url, video_url,
			decrypt_key, browse_time, like_count, comment_count, 
			COALESCE(fav_count, 0) as fav_count, COALESCE(forward_count, 0) as forward_count, page_url,
			created_at, updated_at
		FROM browse_history
		WHERE id IN (%s)
		ORDER BY browse_time DESC
	`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get browse records by IDs: %w", err)
	}
	defer rows.Close()

	var records []BrowseRecord
	for rows.Next() {
		var record BrowseRecord
		err := rows.Scan(
			&record.ID, &record.Title, &record.Author, &record.AuthorID,
			&record.Duration, &record.Size, &record.Resolution, &record.CoverURL, &record.VideoURL,
			&record.DecryptKey, &record.BrowseTime, &record.LikeCount, &record.CommentCount,
			&record.FavCount, &record.ForwardCount, &record.PageURL, &record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan browse record: %w", err)
		}
		records = append(records, record)
	}

	if records == nil {
		records = []BrowseRecord{}
	}

	return records, nil
}

// GetRecordsSince 获取指定时间之后的浏览记录（用于增量同步）
func (r *BrowseHistoryRepository) GetRecordsSince(since time.Time, limit int) ([]BrowseRecord, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT id, title, author, author_id, duration, size, COALESCE(resolution, '') as resolution, cover_url, video_url,
			decrypt_key, browse_time, like_count, comment_count, 
			COALESCE(fav_count, 0) as fav_count, COALESCE(forward_count, 0) as forward_count, page_url,
			created_at, updated_at
		FROM browse_history
		WHERE updated_at > ?
		ORDER BY updated_at ASC
		LIMIT ?
	`

	rows, err := r.db.Query(query, since, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get records since: %w", err)
	}
	defer rows.Close()

	var records []BrowseRecord
	for rows.Next() {
		var record BrowseRecord
		err := rows.Scan(
			&record.ID, &record.Title, &record.Author, &record.AuthorID,
			&record.Duration, &record.Size, &record.Resolution, &record.CoverURL, &record.VideoURL,
			&record.DecryptKey, &record.BrowseTime, &record.LikeCount, &record.CommentCount,
			&record.FavCount, &record.ForwardCount, &record.PageURL, &record.CreatedAt, &record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan browse record: %w", err)
		}
		records = append(records, record)
	}

	if records == nil {
		records = []BrowseRecord{}
	}

	return records, nil
}

// GetLatestTimestamp 获取最新记录的时间戳（用于增量同步）
func (r *BrowseHistoryRepository) GetLatestTimestamp() (time.Time, error) {
	var timestamp time.Time
	err := r.db.QueryRow("SELECT COALESCE(MAX(updated_at), '1970-01-01') FROM browse_history").Scan(&timestamp)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get latest timestamp: %w", err)
	}
	return timestamp, nil
}
