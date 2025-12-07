package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) func() {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "wx_channel_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	
	// Directly open database for testing (bypass once)
	testDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	// Set the global db
	db = testDB

	// Run migrations
	if err := runMigrations(); err != nil {
		testDB.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return func() {
		if db != nil {
			db.Close()
			db = nil
		}
		os.RemoveAll(tmpDir)
		// Reset once for next initialization
		once = sync.Once{}
	}
}

func TestBrowseHistoryRepository(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewBrowseHistoryRepository()

	// Test Create
	record := &BrowseRecord{
		ID:           "test-video-1",
		Title:        "Test Video",
		Author:       "Test Author",
		AuthorID:     "author-1",
		Duration:     120,
		Size:         1024000,
		CoverURL:     "https://example.com/cover.jpg",
		VideoURL:     "https://example.com/video.mp4",
		BrowseTime:   time.Now(),
		LikeCount:    100,
		CommentCount: 50,
		FavCount:     25,
		ForwardCount: 30,
		PageURL:      "https://example.com/page",
	}

	err := repo.Create(record)
	if err != nil {
		t.Fatalf("Failed to create browse record: %v", err)
	}

	// Test GetByID
	retrieved, err := repo.GetByID("test-video-1")
	if err != nil {
		t.Fatalf("Failed to get browse record: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected record, got nil")
	}
	if retrieved.Title != "Test Video" {
		t.Errorf("Expected title 'Test Video', got '%s'", retrieved.Title)
	}

	// Test Update
	record.Title = "Updated Title"
	err = repo.Update(record)
	if err != nil {
		t.Fatalf("Failed to update browse record: %v", err)
	}

	retrieved, _ = repo.GetByID("test-video-1")
	if retrieved.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", retrieved.Title)
	}

	// Test List
	result, err := repo.List(&PaginationParams{Page: 1, PageSize: 10, SortDesc: true})
	if err != nil {
		t.Fatalf("Failed to list browse records: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Expected 1 record, got %d", result.Total)
	}

	// Test Search
	searchResult, err := repo.Search("Updated", &PaginationParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to search browse records: %v", err)
	}
	if searchResult.Total != 1 {
		t.Errorf("Expected 1 search result, got %d", searchResult.Total)
	}

	// Test Delete
	err = repo.Delete("test-video-1")
	if err != nil {
		t.Fatalf("Failed to delete browse record: %v", err)
	}

	retrieved, _ = repo.GetByID("test-video-1")
	if retrieved != nil {
		t.Error("Expected nil after delete, got record")
	}
}

func TestDownloadRecordRepository(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDownloadRecordRepository()

	// Test Create
	record := &DownloadRecord{
		ID:           "download-1",
		VideoID:      "video-1",
		Title:        "Downloaded Video",
		Author:       "Author",
		Duration:     300,
		FileSize:     5000000,
		FilePath:     "/downloads/video.mp4",
		Format:       "mp4",
		Resolution:   "1080p",
		Status:       DownloadStatusCompleted,
		DownloadTime: time.Now(),
	}

	err := repo.Create(record)
	if err != nil {
		t.Fatalf("Failed to create download record: %v", err)
	}

	// Test GetByID
	retrieved, err := repo.GetByID("download-1")
	if err != nil {
		t.Fatalf("Failed to get download record: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected record, got nil")
	}
	if retrieved.Status != DownloadStatusCompleted {
		t.Errorf("Expected status '%s', got '%s'", DownloadStatusCompleted, retrieved.Status)
	}

	// Test List with filter
	result, err := repo.List(&FilterParams{
		PaginationParams: PaginationParams{Page: 1, PageSize: 10, SortDesc: true},
		Status:           DownloadStatusCompleted,
	})
	if err != nil {
		t.Fatalf("Failed to list download records: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Expected 1 record, got %d", result.Total)
	}

	// Test CountToday
	count, err := repo.CountToday()
	if err != nil {
		t.Fatalf("Failed to count today's downloads: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 today's download, got %d", count)
	}
}

func TestQueueRepository(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewQueueRepository()

	// Test Add
	item := &QueueItem{
		ID:        "queue-1",
		VideoID:   "video-1",
		Title:     "Queue Item",
		Author:    "Author",
		VideoURL:  "https://example.com/video.mp4",
		TotalSize: 10000000,
		Status:    QueueStatusPending,
		Priority:  1,
		AddedTime: time.Now(),
		ChunkSize: 10485760,
	}

	err := repo.Add(item)
	if err != nil {
		t.Fatalf("Failed to add queue item: %v", err)
	}

	// Test GetByID
	retrieved, err := repo.GetByID("queue-1")
	if err != nil {
		t.Fatalf("Failed to get queue item: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected item, got nil")
	}

	// Test UpdateStatus
	err = repo.UpdateStatus("queue-1", QueueStatusDownloading)
	if err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	retrieved, _ = repo.GetByID("queue-1")
	if retrieved.Status != QueueStatusDownloading {
		t.Errorf("Expected status '%s', got '%s'", QueueStatusDownloading, retrieved.Status)
	}

	// Test Reorder
	item2 := &QueueItem{
		ID:        "queue-2",
		VideoID:   "video-2",
		Title:     "Queue Item 2",
		Author:    "Author",
		VideoURL:  "https://example.com/video2.mp4",
		TotalSize: 20000000,
		Status:    QueueStatusPending,
		Priority:  0,
		AddedTime: time.Now(),
		ChunkSize: 10485760,
	}
	repo.Add(item2)

	err = repo.Reorder([]string{"queue-2", "queue-1"})
	if err != nil {
		t.Fatalf("Failed to reorder queue: %v", err)
	}

	// Test List (should be ordered by priority)
	items, err := repo.List()
	if err != nil {
		t.Fatalf("Failed to list queue: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}
}

func TestSettingsRepository(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSettingsRepository()

	// Test Load (default settings)
	settings, err := repo.Load()
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}
	if settings.DownloadDir != "downloads" {
		t.Errorf("Expected default download dir 'downloads', got '%s'", settings.DownloadDir)
	}

	// Test Save
	settings.DownloadDir = "/custom/downloads"
	settings.ConcurrentLimit = 5
	err = repo.Save(settings)
	if err != nil {
		t.Fatalf("Failed to save settings: %v", err)
	}

	// Test Load after save
	loaded, err := repo.Load()
	if err != nil {
		t.Fatalf("Failed to load settings after save: %v", err)
	}
	if loaded.DownloadDir != "/custom/downloads" {
		t.Errorf("Expected download dir '/custom/downloads', got '%s'", loaded.DownloadDir)
	}
	if loaded.ConcurrentLimit != 5 {
		t.Errorf("Expected concurrent limit 5, got %d", loaded.ConcurrentLimit)
	}

	// Test Validate
	invalidSettings := &Settings{
		ChunkSize:       500000, // Too small (< 1MB)
		ConcurrentLimit: 3,
		Theme:           "light",
	}
	err = repo.Validate(invalidSettings)
	if err == nil {
		t.Error("Expected validation error for small chunk size")
	}

	invalidSettings.ChunkSize = 10 * 1024 * 1024
	invalidSettings.ConcurrentLimit = 10 // Too high (> 5)
	err = repo.Validate(invalidSettings)
	if err == nil {
		t.Error("Expected validation error for high concurrent limit")
	}
}
