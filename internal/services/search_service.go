package services

import (
	"wx_channel/internal/database"
)

// SearchResult represents global search results
// Requirements: 12.2 - group results by source with counts
type SearchResult struct {
	BrowseResults   []database.BrowseRecord   `json:"browseResults"`
	DownloadResults []database.DownloadRecord `json:"downloadResults"`
	BrowseCount     int64                     `json:"browseCount"`
	DownloadCount   int64                     `json:"downloadCount"`
	TotalCount      int64                     `json:"totalCount"`
}

// SearchService handles global search business logic
type SearchService struct {
	browseRepo   *database.BrowseHistoryRepository
	downloadRepo *database.DownloadRecordRepository
}

// NewSearchService creates a new SearchService
func NewSearchService() *SearchService {
	return &SearchService{
		browseRepo:   database.NewBrowseHistoryRepository(),
		downloadRepo: database.NewDownloadRecordRepository(),
	}
}

// Search performs a global search across browse and download records
// Requirements: 12.1 - search both browse and download records
// Requirements: 12.2 - group results by source with counts
func (s *SearchService) Search(query string, limit int) (*SearchResult, error) {
	if limit < 1 {
		limit = 20
	}

	result := &SearchResult{
		BrowseResults:   []database.BrowseRecord{},
		DownloadResults: []database.DownloadRecord{},
	}

	// Search browse records
	browseParams := &database.PaginationParams{
		Page:     1,
		PageSize: limit,
		SortBy:   "browse_time",
		SortDesc: true,
	}
	browseResult, err := s.browseRepo.Search(query, browseParams)
	if err != nil {
		return nil, err
	}
	result.BrowseResults = browseResult.Items
	result.BrowseCount = browseResult.Total


	// Search download records
	downloadParams := &database.FilterParams{
		PaginationParams: database.PaginationParams{
			Page:     1,
			PageSize: limit,
			SortBy:   "download_time",
			SortDesc: true,
		},
		Query: query,
	}
	downloadResult, err := s.downloadRepo.List(downloadParams)
	if err != nil {
		return nil, err
	}
	result.DownloadResults = downloadResult.Items
	result.DownloadCount = downloadResult.Total

	// Calculate total count
	result.TotalCount = result.BrowseCount + result.DownloadCount

	return result, nil
}

// SearchBrowse searches only browse records
func (s *SearchService) SearchBrowse(query string, params *database.PaginationParams) (*database.PagedResult[database.BrowseRecord], error) {
	if params == nil {
		params = &database.PaginationParams{
			Page:     1,
			PageSize: 20,
			SortBy:   "browse_time",
			SortDesc: true,
		}
	}
	return s.browseRepo.Search(query, params)
}

// SearchDownload searches only download records
func (s *SearchService) SearchDownload(query string, params *database.FilterParams) (*database.PagedResult[database.DownloadRecord], error) {
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
	params.Query = query
	return s.downloadRepo.List(params)
}
