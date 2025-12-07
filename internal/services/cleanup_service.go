package services

import (
	"fmt"
	"os"
	"time"

	"wx_channel/internal/database"
)

// CleanupResult contains the result of a cleanup operation
type CleanupResult struct {
	BrowseRecordsDeleted   int64     `json:"browseRecordsDeleted"`
	DownloadRecordsDeleted int64     `json:"downloadRecordsDeleted"`
	FilesDeleted           int64     `json:"filesDeleted"`
	SpaceFreed             int64     `json:"spaceFreed"`
	CleanupTime            time.Time `json:"cleanupTime"`
	Errors                 []string  `json:"errors,omitempty"`
}

// CleanupService handles data cleanup operations
// Requirements: 5.1, 5.2, 5.3, 5.5, 11.5
type CleanupService struct {
	browseRepo   *database.BrowseHistoryRepository
	downloadRepo *database.DownloadRecordRepository
	settingsRepo *database.SettingsRepository
}

// NewCleanupService creates a new CleanupService
func NewCleanupService() *CleanupService {
	return &CleanupService{
		browseRepo:   database.NewBrowseHistoryRepository(),
		downloadRepo: database.NewDownloadRecordRepository(),
		settingsRepo: database.NewSettingsRepository(),
	}
}

// ClearBrowseHistory clears all browse history records
// Requirements: 5.1, 5.2 - clear all browse history with confirmation
func (s *CleanupService) ClearBrowseHistory() (*CleanupResult, error) {
	// Get count before clearing
	count, err := s.browseRepo.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count browse records: %w", err)
	}

	// Clear all records
	if err := s.browseRepo.Clear(); err != nil {
		return nil, fmt.Errorf("failed to clear browse history: %w", err)
	}

	return &CleanupResult{
		BrowseRecordsDeleted: count,
		CleanupTime:          time.Now(),
	}, nil
}


// ClearDownloadRecords clears all download records with optional file deletion
// Requirements: 5.3 - clear download records with option to delete files
func (s *CleanupService) ClearDownloadRecords(deleteFiles bool) (*CleanupResult, error) {
	result := &CleanupResult{
		CleanupTime: time.Now(),
		Errors:      []string{},
	}

	// Get all records before clearing (for file deletion)
	records, err := s.downloadRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get download records: %w", err)
	}

	// Delete files if requested
	if deleteFiles {
		for _, record := range records {
			if record.FilePath != "" {
				fileInfo, err := os.Stat(record.FilePath)
				if err == nil {
					result.SpaceFreed += fileInfo.Size()
					if err := os.Remove(record.FilePath); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("failed to delete file %s: %v", record.FilePath, err))
					} else {
						result.FilesDeleted++
					}
				}
			}
		}
	}

	// Clear all records
	if err := s.downloadRepo.Clear(); err != nil {
		return nil, fmt.Errorf("failed to clear download records: %w", err)
	}

	result.DownloadRecordsDeleted = int64(len(records))
	return result, nil
}

// DeleteBrowseRecordsBefore deletes browse records before the specified date
// Requirements: 5.5 - date-based cleanup
func (s *CleanupService) DeleteBrowseRecordsBefore(date time.Time) (*CleanupResult, error) {
	count, err := s.browseRepo.DeleteBefore(date)
	if err != nil {
		return nil, fmt.Errorf("failed to delete browse records before %v: %w", date, err)
	}

	return &CleanupResult{
		BrowseRecordsDeleted: count,
		CleanupTime:          time.Now(),
	}, nil
}

// DeleteDownloadRecordsBefore deletes download records before the specified date
// Requirements: 5.5 - date-based cleanup
func (s *CleanupService) DeleteDownloadRecordsBefore(date time.Time, deleteFiles bool) (*CleanupResult, error) {
	result := &CleanupResult{
		CleanupTime: time.Now(),
		Errors:      []string{},
	}

	// If we need to delete files, we need to get the records first
	if deleteFiles {
		// Get all records and filter by date
		records, err := s.downloadRepo.GetAll()
		if err != nil {
			return nil, fmt.Errorf("failed to get download records: %w", err)
		}

		for _, record := range records {
			if record.DownloadTime.Before(date) && record.FilePath != "" {
				fileInfo, err := os.Stat(record.FilePath)
				if err == nil {
					result.SpaceFreed += fileInfo.Size()
					if err := os.Remove(record.FilePath); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("failed to delete file %s: %v", record.FilePath, err))
					} else {
						result.FilesDeleted++
					}
				}
			}
		}
	}

	// Delete records from database
	count, err := s.downloadRepo.DeleteBefore(date)
	if err != nil {
		return nil, fmt.Errorf("failed to delete download records before %v: %w", date, err)
	}

	result.DownloadRecordsDeleted = count
	return result, nil
}

// RunAutoCleanup runs automatic cleanup based on settings
// Requirements: 11.5 - auto cleanup based on settings
func (s *CleanupService) RunAutoCleanup() (*CleanupResult, error) {
	// Load settings
	settings, err := s.settingsRepo.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load settings: %w", err)
	}

	// Check if auto cleanup is enabled
	if !settings.AutoCleanupEnabled {
		return &CleanupResult{
			CleanupTime: time.Now(),
		}, nil
	}

	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, 0, -settings.AutoCleanupDays)

	// Delete old browse records
	browseResult, err := s.DeleteBrowseRecordsBefore(cutoffDate)
	if err != nil {
		return nil, fmt.Errorf("failed to cleanup browse records: %w", err)
	}

	return &CleanupResult{
		BrowseRecordsDeleted: browseResult.BrowseRecordsDeleted,
		CleanupTime:          time.Now(),
	}, nil
}

// DeleteSelectedBrowseRecords deletes specific browse records by IDs
// Requirements: 5.4 - selective deletion
func (s *CleanupService) DeleteSelectedBrowseRecords(ids []string) (*CleanupResult, error) {
	count, err := s.browseRepo.DeleteMany(ids)
	if err != nil {
		return nil, fmt.Errorf("failed to delete selected browse records: %w", err)
	}

	return &CleanupResult{
		BrowseRecordsDeleted: count,
		CleanupTime:          time.Now(),
	}, nil
}

// DeleteSelectedDownloadRecords deletes specific download records by IDs
// Requirements: 5.4 - selective deletion
func (s *CleanupService) DeleteSelectedDownloadRecords(ids []string, deleteFiles bool) (*CleanupResult, error) {
	result := &CleanupResult{
		CleanupTime: time.Now(),
		Errors:      []string{},
	}

	// If we need to delete files, get the records first
	if deleteFiles {
		records, err := s.downloadRepo.GetByIDs(ids)
		if err != nil {
			return nil, fmt.Errorf("failed to get download records: %w", err)
		}

		for _, record := range records {
			if record.FilePath != "" {
				fileInfo, err := os.Stat(record.FilePath)
				if err == nil {
					result.SpaceFreed += fileInfo.Size()
					if err := os.Remove(record.FilePath); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("failed to delete file %s: %v", record.FilePath, err))
					} else {
						result.FilesDeleted++
					}
				}
			}
		}
	}

	// Delete records from database
	count, err := s.downloadRepo.DeleteMany(ids)
	if err != nil {
		return nil, fmt.Errorf("failed to delete selected download records: %w", err)
	}

	result.DownloadRecordsDeleted = count
	return result, nil
}

// CleanupAll clears all data (browse and download records)
// Requirements: 5.1, 5.2, 5.3 - comprehensive cleanup
func (s *CleanupService) CleanupAll(deleteFiles bool) (*CleanupResult, error) {
	result := &CleanupResult{
		CleanupTime: time.Now(),
		Errors:      []string{},
	}

	// Clear browse history
	browseResult, err := s.ClearBrowseHistory()
	if err != nil {
		return nil, fmt.Errorf("failed to clear browse history: %w", err)
	}
	result.BrowseRecordsDeleted = browseResult.BrowseRecordsDeleted

	// Clear download records
	downloadResult, err := s.ClearDownloadRecords(deleteFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to clear download records: %w", err)
	}
	result.DownloadRecordsDeleted = downloadResult.DownloadRecordsDeleted
	result.FilesDeleted = downloadResult.FilesDeleted
	result.SpaceFreed = downloadResult.SpaceFreed
	result.Errors = append(result.Errors, downloadResult.Errors...)

	return result, nil
}
