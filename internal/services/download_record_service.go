package services

import (
	"os"
	"time"

	"wx_channel/internal/database"
)

// DownloadRecordService handles download record business logic
type DownloadRecordService struct {
	repo *database.DownloadRecordRepository
}

// NewDownloadRecordService creates a new DownloadRecordService
func NewDownloadRecordService() *DownloadRecordService {
	return &DownloadRecordService{
		repo: database.NewDownloadRecordRepository(),
	}
}

// List retrieves download records with filtering and pagination
// Requirements: 2.3, 2.4 - filter by date range and status
func (s *DownloadRecordService) List(params *database.FilterParams) (*database.PagedResult[database.DownloadRecord], error) {
	if params == nil {
		params = &database.FilterParams{
			PaginationParams: database.PaginationParams{
				Page:     1,
				PageSize: 20,
				SortBy:   "download_time",
				SortDesc: true,
			},
		}
	}
	return s.repo.List(params)
}

// GetByID retrieves a single download record by ID
func (s *DownloadRecordService) GetByID(id string) (*database.DownloadRecord, error) {
	return s.repo.GetByID(id)
}

// Delete removes a download record by ID, optionally deleting the file
// Requirements: 5.3 - delete with option to keep or delete files
func (s *DownloadRecordService) Delete(id string, deleteFile bool) error {
	if deleteFile {
		record, err := s.repo.GetByID(id)
		if err != nil {
			return err
		}
		if record != nil && record.FilePath != "" {
			// Attempt to delete the file, ignore errors if file doesn't exist
			_ = os.Remove(record.FilePath)
		}
	}
	return s.repo.Delete(id)
}


// DeleteMany removes multiple download records by IDs, optionally deleting files
// Requirements: 5.3 - batch delete with option to keep or delete files
func (s *DownloadRecordService) DeleteMany(ids []string, deleteFiles bool) (int64, error) {
	if deleteFiles {
		records, err := s.repo.GetByIDs(ids)
		if err != nil {
			return 0, err
		}
		for _, record := range records {
			if record.FilePath != "" {
				// Attempt to delete the file, ignore errors if file doesn't exist
				_ = os.Remove(record.FilePath)
			}
		}
	}
	return s.repo.DeleteMany(ids)
}

// Clear removes all download records, optionally deleting files
// Requirements: 5.3 - clear with option to keep or delete files
func (s *DownloadRecordService) Clear(deleteFiles bool) error {
	if deleteFiles {
		records, err := s.repo.GetAll()
		if err != nil {
			return err
		}
		for _, record := range records {
			if record.FilePath != "" {
				// Attempt to delete the file, ignore errors if file doesn't exist
				_ = os.Remove(record.FilePath)
			}
		}
	}
	return s.repo.Clear()
}

// DeleteBefore removes all records before the specified date, optionally deleting files
func (s *DownloadRecordService) DeleteBefore(date time.Time, deleteFiles bool) (int64, error) {
	if deleteFiles {
		// Get records before date to delete their files
		params := &database.FilterParams{
			PaginationParams: database.PaginationParams{
				Page:     1,
				PageSize: 10000, // Large enough to get all
			},
			EndDate: &date,
		}
		result, err := s.repo.List(params)
		if err != nil {
			return 0, err
		}
		for _, record := range result.Items {
			if record.FilePath != "" {
				_ = os.Remove(record.FilePath)
			}
		}
	}
	return s.repo.DeleteBefore(date)
}

// Count returns the total number of download records
func (s *DownloadRecordService) Count() (int64, error) {
	return s.repo.Count()
}

// CountByStatus returns the count of records with a specific status
func (s *DownloadRecordService) CountByStatus(status string) (int64, error) {
	return s.repo.CountByStatus(status)
}

// CountToday returns the count of records downloaded today
func (s *DownloadRecordService) CountToday() (int64, error) {
	return s.repo.CountToday()
}

// GetRecent retrieves the most recent download records
// Requirements: 7.4 - recent 5 downloads on dashboard
func (s *DownloadRecordService) GetRecent(limit int) ([]database.DownloadRecord, error) {
	return s.repo.GetRecent(limit)
}

// GetAll retrieves all download records (for export)
// Requirements: 4.2 - export download records
func (s *DownloadRecordService) GetAll() ([]database.DownloadRecord, error) {
	return s.repo.GetAll()
}

// GetByIDs retrieves download records by IDs (for selective export)
// Requirements: 9.4 - export selected records
func (s *DownloadRecordService) GetByIDs(ids []string) ([]database.DownloadRecord, error) {
	return s.repo.GetByIDs(ids)
}

// GetChartData returns download counts for the last N days
// Requirements: 7.2 - chart data for dashboard
func (s *DownloadRecordService) GetChartData(days int) ([]string, []int64, error) {
	return s.repo.GetChartData(days)
}

// GetTotalFileSize returns the total file size of all completed downloads
func (s *DownloadRecordService) GetTotalFileSize() (int64, error) {
	return s.repo.GetTotalFileSize()
}

// Create adds a new download record
func (s *DownloadRecordService) Create(record *database.DownloadRecord) error {
	return s.repo.Create(record)
}

// Update updates an existing download record
func (s *DownloadRecordService) Update(record *database.DownloadRecord) error {
	return s.repo.Update(record)
}
