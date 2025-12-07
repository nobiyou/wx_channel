package services

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"wx_channel/internal/database"
)

// ExportFormat represents the export file format
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

// ExportResult contains the exported data and metadata
type ExportResult struct {
	Data        []byte    `json:"data"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	RecordCount int       `json:"recordCount"`
	ExportTime  time.Time `json:"exportTime"`
}

// ExportService handles data export operations
// Requirements: 4.1, 4.2, 4.3, 9.4
type ExportService struct {
	browseRepo   *database.BrowseHistoryRepository
	downloadRepo *database.DownloadRecordRepository
}

// NewExportService creates a new ExportService
func NewExportService() *ExportService {
	return &ExportService{
		browseRepo:   database.NewBrowseHistoryRepository(),
		downloadRepo: database.NewDownloadRecordRepository(),
	}
}

// GenerateTimestampFilename generates a filename with timestamp
// Requirements: 4.3 - include timestamp in export filename
func GenerateTimestampFilename(prefix string, format ExportFormat) string {
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s.%s", prefix, timestamp, format)
}

// ExportBrowseHistory exports browse history records
// Requirements: 4.1 - export browse history in JSON or CSV format
func (s *ExportService) ExportBrowseHistory(format ExportFormat, ids []string) (*ExportResult, error) {
	var records []database.BrowseRecord
	var err error

	// Get records - either all or by specific IDs
	// Requirements: 9.4 - selective export by IDs
	if len(ids) > 0 {
		records, err = s.browseRepo.GetByIDs(ids)
	} else {
		records, err = s.browseRepo.GetAll()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get browse records: %w", err)
	}

	var data []byte
	var contentType string

	switch format {
	case ExportFormatJSON:
		data, err = s.exportBrowseRecordsToJSON(records)
		contentType = "application/json"
	case ExportFormatCSV:
		data, err = s.exportBrowseRecordsToCSV(records)
		contentType = "text/csv"
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}

	if err != nil {
		return nil, err
	}

	return &ExportResult{
		Data:        data,
		Filename:    GenerateTimestampFilename("browse_history", format),
		ContentType: contentType,
		RecordCount: len(records),
		ExportTime:  time.Now(),
	}, nil
}


// ExportDownloadRecords exports download records
// Requirements: 4.2 - export download records in JSON or CSV format
func (s *ExportService) ExportDownloadRecords(format ExportFormat, ids []string) (*ExportResult, error) {
	var records []database.DownloadRecord
	var err error

	// Get records - either all or by specific IDs
	// Requirements: 9.4 - selective export by IDs
	if len(ids) > 0 {
		records, err = s.downloadRepo.GetByIDs(ids)
	} else {
		records, err = s.downloadRepo.GetAll()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get download records: %w", err)
	}

	var data []byte
	var contentType string

	switch format {
	case ExportFormatJSON:
		data, err = s.exportDownloadRecordsToJSON(records)
		contentType = "application/json"
	case ExportFormatCSV:
		data, err = s.exportDownloadRecordsToCSV(records)
		contentType = "text/csv"
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}

	if err != nil {
		return nil, err
	}

	return &ExportResult{
		Data:        data,
		Filename:    GenerateTimestampFilename("download_records", format),
		ContentType: contentType,
		RecordCount: len(records),
		ExportTime:  time.Now(),
	}, nil
}

// exportBrowseRecordsToJSON exports browse records to JSON format
func (s *ExportService) exportBrowseRecordsToJSON(records []database.BrowseRecord) ([]byte, error) {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal browse records to JSON: %w", err)
	}
	return data, nil
}

// exportBrowseRecordsToCSV exports browse records to CSV format
func (s *ExportService) exportBrowseRecordsToCSV(records []database.BrowseRecord) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"ID", "Title", "Author", "AuthorID", "Duration", "Size", "Resolution",
		"CoverURL", "VideoURL", "DecryptKey", "BrowseTime", "LikeCount",
		"CommentCount", "FavCount", "ForwardCount", "PageURL", "CreatedAt", "UpdatedAt",
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write records
	for _, record := range records {
		row := []string{
			record.ID,
			record.Title,
			record.Author,
			record.AuthorID,
			fmt.Sprintf("%d", record.Duration),
			fmt.Sprintf("%d", record.Size),
			record.Resolution,
			record.CoverURL,
			record.VideoURL,
			record.DecryptKey,
			record.BrowseTime.Format(time.RFC3339),
			fmt.Sprintf("%d", record.LikeCount),
			fmt.Sprintf("%d", record.CommentCount),
			fmt.Sprintf("%d", record.FavCount),
			fmt.Sprintf("%d", record.ForwardCount),
			record.PageURL,
			record.CreatedAt.Format(time.RFC3339),
			record.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	return buf.Bytes(), nil
}

// exportDownloadRecordsToJSON exports download records to JSON format
func (s *ExportService) exportDownloadRecordsToJSON(records []database.DownloadRecord) ([]byte, error) {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal download records to JSON: %w", err)
	}
	return data, nil
}

// exportDownloadRecordsToCSV exports download records to CSV format
func (s *ExportService) exportDownloadRecordsToCSV(records []database.DownloadRecord) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"ID", "VideoID", "Title", "Author", "Duration", "FileSize",
		"FilePath", "Format", "Resolution", "Status", "DownloadTime",
		"ErrorMessage", "CreatedAt", "UpdatedAt",
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write records
	for _, record := range records {
		row := []string{
			record.ID,
			record.VideoID,
			record.Title,
			record.Author,
			fmt.Sprintf("%d", record.Duration),
			fmt.Sprintf("%d", record.FileSize),
			record.FilePath,
			record.Format,
			record.Resolution,
			record.Status,
			record.DownloadTime.Format(time.RFC3339),
			record.ErrorMessage,
			record.CreatedAt.Format(time.RFC3339),
			record.UpdatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	return buf.Bytes(), nil
}

// ParseBrowseRecordsFromJSON parses browse records from JSON data
// Used for import/round-trip testing
func ParseBrowseRecordsFromJSON(data []byte) ([]database.BrowseRecord, error) {
	var records []database.BrowseRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("failed to parse browse records from JSON: %w", err)
	}
	return records, nil
}

// ParseDownloadRecordsFromJSON parses download records from JSON data
// Used for import/round-trip testing
func ParseDownloadRecordsFromJSON(data []byte) ([]database.DownloadRecord, error) {
	var records []database.DownloadRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("failed to parse download records from JSON: %w", err)
	}
	return records, nil
}
