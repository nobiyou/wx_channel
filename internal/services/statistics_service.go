package services

import (
	"wx_channel/internal/database"
)

// Statistics represents dashboard statistics
type Statistics struct {
	TotalBrowseCount   int64                     `json:"totalBrowseCount"`
	TotalDownloadCount int64                     `json:"totalDownloadCount"`
	TodayDownloadCount int64                     `json:"todayDownloadCount"`
	StorageUsed        int64                     `json:"storageUsed"`
	RecentBrowse       []database.BrowseRecord   `json:"recentBrowse"`
	RecentDownload     []database.DownloadRecord `json:"recentDownload"`
}

// ChartData represents chart data for the dashboard
type ChartData struct {
	Labels []string `json:"labels"`
	Values []int64  `json:"values"`
}

// StatisticsService handles statistics business logic
type StatisticsService struct {
	browseRepo   *database.BrowseHistoryRepository
	downloadRepo *database.DownloadRecordRepository
}

// NewStatisticsService creates a new StatisticsService
func NewStatisticsService() *StatisticsService {
	return &StatisticsService{
		browseRepo:   database.NewBrowseHistoryRepository(),
		downloadRepo: database.NewDownloadRecordRepository(),
	}
}

// GetStatistics returns dashboard statistics
// Requirements: 7.1 - total browse, total downloads, today downloads, storage used
func (s *StatisticsService) GetStatistics() (*Statistics, error) {
	stats := &Statistics{}

	// Get total browse count
	browseCount, err := s.browseRepo.Count()
	if err != nil {
		return nil, err
	}
	stats.TotalBrowseCount = browseCount

	// Get total download count
	downloadCount, err := s.downloadRepo.Count()
	if err != nil {
		return nil, err
	}
	stats.TotalDownloadCount = downloadCount

	// Get today's download count
	todayCount, err := s.downloadRepo.CountToday()
	if err != nil {
		return nil, err
	}
	stats.TodayDownloadCount = todayCount


	// Get storage used (total file size of completed downloads)
	storageUsed, err := s.downloadRepo.GetTotalFileSize()
	if err != nil {
		return nil, err
	}
	stats.StorageUsed = storageUsed

	// Get recent browse records
	// Requirements: 7.3 - recent 5 videos
	recentBrowse, err := s.browseRepo.GetRecent(5)
	if err != nil {
		return nil, err
	}
	stats.RecentBrowse = recentBrowse

	// Get recent download records
	// Requirements: 7.4 - recent 5 downloads
	recentDownload, err := s.downloadRepo.GetRecent(5)
	if err != nil {
		return nil, err
	}
	stats.RecentDownload = recentDownload

	return stats, nil
}

// GetChartData returns download counts for the last 7 days
// Requirements: 7.2 - chart showing download activity for past 7 days
func (s *StatisticsService) GetChartData(days int) (*ChartData, error) {
	if days < 1 {
		days = 7
	}

	labels, values, err := s.downloadRepo.GetChartData(days)
	if err != nil {
		return nil, err
	}

	return &ChartData{
		Labels: labels,
		Values: values,
	}, nil
}

// GetRecentBrowse retrieves the most recent browse records
// Requirements: 7.3 - recent 5 videos on dashboard
func (s *StatisticsService) GetRecentBrowse(limit int) ([]database.BrowseRecord, error) {
	if limit < 1 {
		limit = 5
	}
	return s.browseRepo.GetRecent(limit)
}

// GetRecentDownload retrieves the most recent download records
// Requirements: 7.4 - recent 5 downloads on dashboard
func (s *StatisticsService) GetRecentDownload(limit int) ([]database.DownloadRecord, error) {
	if limit < 1 {
		limit = 5
	}
	return s.downloadRepo.GetRecent(limit)
}
