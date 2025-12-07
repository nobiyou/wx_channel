package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DownloadRecordRepository handles download record database operations
type DownloadRecordRepository struct {
	db *sql.DB
}

// NewDownloadRecordRepository creates a new DownloadRecordRepository
func NewDownloadRecordRepository() *DownloadRecordRepository {
	return &DownloadRecordRepository{db: GetDB()}
}

// Create inserts a new download record
func (r *DownloadRecordRepository) Create(record *DownloadRecord) error {
	now := time.Now()
	record.CreatedAt = now
	record.UpdatedAt = now

	query := `
		INSERT INTO download_records (
			id, video_id, title, author, cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query,
		record.ID, record.VideoID, record.Title, record.Author, record.CoverURL,
		record.Duration, record.FileSize, record.FilePath, record.Format,
		record.Resolution, record.Status, record.DownloadTime,
		record.ErrorMessage, record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create download record: %w", err)
	}
	return nil
}

// GetByID retrieves a download record by ID
func (r *DownloadRecordRepository) GetByID(id string) (*DownloadRecord, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
			created_at, updated_at
		FROM download_records WHERE id = ?
	`
	record := &DownloadRecord{}
	var filePath, format, resolution, errorMessage, coverURL sql.NullString
	err := r.db.QueryRow(query, id).Scan(
		&record.ID, &record.VideoID, &record.Title, &record.Author, &coverURL,
		&record.Duration, &record.FileSize, &filePath, &format,
		&resolution, &record.Status, &record.DownloadTime,
		&errorMessage, &record.CreatedAt, &record.UpdatedAt,
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

// Update updates an existing download record
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

// Delete removes a download record by ID
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

// DeleteMany removes multiple download records by IDs
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

// Clear removes all download records
func (r *DownloadRecordRepository) Clear() error {
	_, err := r.db.Exec("DELETE FROM download_records")
	if err != nil {
		return fmt.Errorf("failed to clear download records: %w", err)
	}
	return nil
}

// List retrieves download records with pagination, filtering, and sorting
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
			&errorMessage, &record.CreatedAt, &record.UpdatedAt,
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

// Count returns the total number of download records
func (r *DownloadRecordRepository) Count() (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM download_records").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count download records: %w", err)
	}
	return count, nil
}

// CountByStatus returns the count of records with a specific status
func (r *DownloadRecordRepository) CountByStatus(status string) (int64, error) {
	var count int64
	err := r.db.QueryRow("SELECT COUNT(*) FROM download_records WHERE status = ?", status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count download records by status: %w", err)
	}
	return count, nil
}

// CountToday returns the count of records downloaded today
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

// GetRecent retrieves the most recent download records
func (r *DownloadRecordRepository) GetRecent(limit int) ([]DownloadRecord, error) {
	if limit < 1 {
		limit = 5
	}

	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
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
			&errorMessage, &record.CreatedAt, &record.UpdatedAt,
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

// DeleteBefore deletes all records before the specified date
func (r *DownloadRecordRepository) DeleteBefore(date time.Time) (int64, error) {
	result, err := r.db.Exec("DELETE FROM download_records WHERE download_time < ?", date)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old download records: %w", err)
	}
	return result.RowsAffected()
}

// GetAll retrieves all download records (for export)
func (r *DownloadRecordRepository) GetAll() ([]DownloadRecord, error) {
	query := `
		SELECT id, video_id, title, author, COALESCE(cover_url, '') as cover_url, duration, file_size, file_path,
			format, resolution, status, download_time, error_message,
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
			&errorMessage, &record.CreatedAt, &record.UpdatedAt,
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

// GetByIDs retrieves download records by IDs
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
			&errorMessage, &record.CreatedAt, &record.UpdatedAt,
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

// GetChartData returns download counts for the last N days
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

// GetTotalFileSize returns the total file size of all completed downloads
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
