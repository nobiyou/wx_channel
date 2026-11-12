package services

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"wx_channel/internal/database"
)

// ExportFormat 表示导出文件格式
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

// ExportResult 包含导出的数据和元数据
type ExportResult struct {
	Data        []byte    `json:"data"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	RecordCount int       `json:"recordCount"`
	ExportTime  time.Time `json:"exportTime"`
}

// ExportService 处理数据导出操作
// Requirements: 4.1, 4.2, 4.3, 9.4
type ExportService struct {
	browseRepo   *database.BrowseHistoryRepository
	downloadRepo *database.DownloadRecordRepository
}

// NewExportService 创建一个新的 ExportService
func NewExportService() *ExportService {
	return &ExportService{
		browseRepo:   database.NewBrowseHistoryRepository(),
		downloadRepo: database.NewDownloadRecordRepository(),
	}
}

// GenerateTimestampFilename 生成带时间戳的文件名
// Requirements: 4.3 - 导出文件名包含时间戳
func GenerateTimestampFilename(prefix string, format ExportFormat) string {
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s.%s", prefix, timestamp, format)
}

// ExportBrowseHistory 导出浏览历史记录
// Requirements: 4.1 - 以 JSON 或 CSV 格式导出浏览历史
func (s *ExportService) ExportBrowseHistory(format ExportFormat, ids []string) (*ExportResult, error) {
	var records []database.BrowseRecord
	var err error

	// 获取记录 - 所有或按特定 ID
	// Requirements: 9.4 - 按 ID 选择性导出
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

// ExportDownloadRecords 导出下载记录
// Requirements: 4.2 - 以 JSON 或 CSV 格式导出下载记录
func (s *ExportService) ExportDownloadRecords(format ExportFormat, ids []string) (*ExportResult, error) {
	var records []database.DownloadRecord
	var err error

	// 获取记录 - 所有或按特定 ID
	// Requirements: 9.4 - 按 ID 选择性导出
	if len(ids) > 0 {
		records, err = s.downloadRepo.GetByIDs(ids)
	} else {
		records, err = s.downloadRepo.GetAll()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get download records: %w", err)
	}
	if err := s.enrichDownloadRecordsFromBrowse(records); err != nil {
		return nil, err
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

// enrichDownloadRecordsFromBrowse repairs legacy download rows whose download
// path did not persist interaction metrics. The repaired row is also written
// back so subsequent exports and the download history use the same values.
func (s *ExportService) enrichDownloadRecordsFromBrowse(records []database.DownloadRecord) error {
	for i := range records {
		fullRecord, err := s.downloadRepo.GetByID(records[i].ID)
		if err != nil {
			return fmt.Errorf("failed to reload download record %q: %w", records[i].ID, err)
		}
		if fullRecord == nil {
			continue
		}
		records[i] = *fullRecord

		browse, err := s.browseRepo.GetByID(records[i].VideoID)
		if err != nil {
			return fmt.Errorf("failed to read browse metrics for download %q: %w", records[i].ID, err)
		}
		if browse == nil && records[i].ID != records[i].VideoID {
			browse, err = s.browseRepo.GetByID(records[i].ID)
			if err != nil {
				return fmt.Errorf("failed to read browse metrics for download %q: %w", records[i].ID, err)
			}
		}
		if browse == nil || !database.MergeBrowseMetricsIntoDownload(&records[i], browse) {
			continue
		}
		if err := s.downloadRepo.Update(&records[i]); err != nil {
			return fmt.Errorf("failed to persist metrics for download %q: %w", records[i].ID, err)
		}
	}
	return nil
}

// exportBrowseRecordsToJSON 将浏览记录导出为 JSON 格式
func (s *ExportService) exportBrowseRecordsToJSON(records []database.BrowseRecord) ([]byte, error) {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal browse records to JSON: %w", err)
	}
	return data, nil
}

// exportBrowseRecordsToCSV 将浏览记录导出为 CSV 格式
func (s *ExportService) exportBrowseRecordsToCSV(records []database.BrowseRecord) ([]byte, error) {
	var buf bytes.Buffer
	// 写入 UTF-8 BOM 以兼容 Excel
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(&buf)

	// 写入表头
	header := []string{
		"ID", "Title", "Author", "AuthorID", "Duration", "Size", "Resolution",
		"CoverURL", "VideoURL", "DecryptKey", "BrowseTime", "LikeCount",
		"CommentCount", "FavCount", "ForwardCount", "PageURL", "CreatedAt", "UpdatedAt",
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// 写入记录
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

// exportDownloadRecordsToJSON 将下载记录导出为 JSON 格式
func (s *ExportService) exportDownloadRecordsToJSON(records []database.DownloadRecord) ([]byte, error) {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal download records to JSON: %w", err)
	}
	return data, nil
}

// exportDownloadRecordsToCSV 将下载记录导出为 CSV 格式
func (s *ExportService) exportDownloadRecordsToCSV(records []database.DownloadRecord) ([]byte, error) {
	var buf bytes.Buffer
	// 写入 UTF-8 BOM 以兼容 Excel
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(&buf)

	// 写入表头
	header := []string{
		"ID", "VideoID", "Title", "Author", "Duration", "FileSize",
		"FilePath", "Format", "Resolution", "Status", "DownloadTime",
		"LikeCount", "CommentCount", "ForwardCount", "FavCount",
		"ErrorMessage", "CreatedAt", "UpdatedAt",
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// 写入记录
	for _, record := range records {
		row := []string{
			record.ID,
			record.VideoID,
			record.Title,
			record.Author,
			formatDuration(record.Duration),
			formatFileSize(record.FileSize),
			record.FilePath,
			record.Format,
			record.Resolution,
			record.Status,
			record.DownloadTime.Format(time.RFC3339),
			fmt.Sprintf("%d", record.LikeCount),
			fmt.Sprintf("%d", record.CommentCount),
			fmt.Sprintf("%d", record.ForwardCount),
			fmt.Sprintf("%d", record.FavCount),
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

// formatDuration 将毫秒持续时间格式化为 MM:SS 字符串
func formatDuration(ms int64) string {
	seconds := ms / 1000
	minutes := seconds / 60
	remainingSeconds := seconds % 60
	return fmt.Sprintf("%02d:%02d", minutes, remainingSeconds)
}

// formatFileSize 将字节大小格式化为人类可读字符串 (MB/KB)
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ParseBrowseRecordsFromJSON 从 JSON 数据解析浏览记录
// 用于导入/往返测试
func ParseBrowseRecordsFromJSON(data []byte) ([]database.BrowseRecord, error) {
	var records []database.BrowseRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("failed to parse browse records from JSON: %w", err)
	}
	return records, nil
}

// ParseDownloadRecordsFromJSON 从 JSON 数据解析下载记录
// 用于导入/往返测试
func ParseDownloadRecordsFromJSON(data []byte) ([]database.DownloadRecord, error) {
	var records []database.DownloadRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("failed to parse download records from JSON: %w", err)
	}
	return records, nil
}
