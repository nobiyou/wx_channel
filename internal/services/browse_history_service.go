package services

import (
	"time"

	"wx_channel/internal/database"
)

// BrowseHistoryService handles browse history business logic
type BrowseHistoryService struct {
	repo *database.BrowseHistoryRepository
}

// NewBrowseHistoryService creates a new BrowseHistoryService
func NewBrowseHistoryService() *BrowseHistoryService {
	return &BrowseHistoryService{
		repo: database.NewBrowseHistoryRepository(),
	}
}

// Search searches browse records by title or author with pagination
// Requirements: 1.3 - search within 500ms by title or author
func (s *BrowseHistoryService) Search(query string, params *database.PaginationParams) (*database.PagedResult[database.BrowseRecord], error) {
	if params == nil {
		params = &database.PaginationParams{
			Page:     1,
			PageSize: 20,
			SortBy:   "browse_time",
			SortDesc: true,
		}
	}
	return s.repo.Search(query, params)
}

// List retrieves browse records with pagination
// Requirements: 1.1, 1.4 - paginated list sorted by browse time
func (s *BrowseHistoryService) List(params *database.PaginationParams) (*database.PagedResult[database.BrowseRecord], error) {
	if params == nil {
		params = &database.PaginationParams{
			Page:     1,
			PageSize: 20,
			SortBy:   "browse_time",
			SortDesc: true,
		}
	}
	return s.repo.List(params)
}

// GetByID retrieves a single browse record by ID
func (s *BrowseHistoryService) GetByID(id string) (*database.BrowseRecord, error) {
	return s.repo.GetByID(id)
}


// Clear removes all browse records
// Requirements: 5.1, 5.2 - clear all browse history with confirmation
func (s *BrowseHistoryService) Clear() error {
	return s.repo.Clear()
}

// Delete removes a single browse record by ID
// Requirements: 5.4 - selective deletion
func (s *BrowseHistoryService) Delete(id string) error {
	return s.repo.Delete(id)
}

// DeleteMany removes multiple browse records by IDs
// Requirements: 5.4 - selective deletion of selected records
func (s *BrowseHistoryService) DeleteMany(ids []string) (int64, error) {
	return s.repo.DeleteMany(ids)
}

// DeleteBefore removes all records before the specified date
// Requirements: 5.5 - date-based cleanup
func (s *BrowseHistoryService) DeleteBefore(date time.Time) (int64, error) {
	return s.repo.DeleteBefore(date)
}

// Count returns the total number of browse records
func (s *BrowseHistoryService) Count() (int64, error) {
	return s.repo.Count()
}

// GetRecent retrieves the most recent browse records
// Requirements: 7.3 - recent 5 videos on dashboard
func (s *BrowseHistoryService) GetRecent(limit int) ([]database.BrowseRecord, error) {
	return s.repo.GetRecent(limit)
}

// GetAll retrieves all browse records (for export)
// Requirements: 4.1 - export browse history
func (s *BrowseHistoryService) GetAll() ([]database.BrowseRecord, error) {
	return s.repo.GetAll()
}

// GetByIDs retrieves browse records by IDs (for selective export)
// Requirements: 9.4 - export selected records
func (s *BrowseHistoryService) GetByIDs(ids []string) ([]database.BrowseRecord, error) {
	return s.repo.GetByIDs(ids)
}

// Create adds a new browse record
func (s *BrowseHistoryService) Create(record *database.BrowseRecord) error {
	return s.repo.Create(record)
}

// Update updates an existing browse record
func (s *BrowseHistoryService) Update(record *database.BrowseRecord) error {
	return s.repo.Update(record)
}
