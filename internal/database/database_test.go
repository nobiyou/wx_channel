package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) func() {
	// 创建测试数据库的临时目录
	tmpDir, err := os.MkdirTemp("", "wx_channel_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")

	// 直接打开数据库进行测试（绕过 once）
	testDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	// 设置全局数据库
	db = testDB

	// 运行迁移
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
		// 重置初始化状态以便下次初始化
		initialized = false
	}
}

func TestBrowseHistoryRepository(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewBrowseHistoryRepository()

	// 测试创建
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

	// 测试根据 ID 获取
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

	// 测试更新
	record.Title = "Updated Title"
	err = repo.Update(record)
	if err != nil {
		t.Fatalf("Failed to update browse record: %v", err)
	}

	retrieved, _ = repo.GetByID("test-video-1")
	if retrieved.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", retrieved.Title)
	}

	// 测试列表
	result, err := repo.List(&PaginationParams{Page: 1, PageSize: 10, SortDesc: true})
	if err != nil {
		t.Fatalf("Failed to list browse records: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Expected 1 record, got %d", result.Total)
	}

	// 测试搜索
	searchResult, err := repo.Search("Updated", &PaginationParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to search browse records: %v", err)
	}
	if searchResult.Total != 1 {
		t.Errorf("Expected 1 search result, got %d", searchResult.Total)
	}

	// 测试删除
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

	// 测试创建
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

	// 测试根据 ID 获取
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

	// 测试带过滤的列表
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

	// 测试统计今天
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

	// 测试添加
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

	// 测试根据 ID 获取
	retrieved, err := repo.GetByID("queue-1")
	if err != nil {
		t.Fatalf("Failed to get queue item: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected item, got nil")
	}

	// 测试更新状态
	err = repo.UpdateStatus("queue-1", QueueStatusDownloading)
	if err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	retrieved, _ = repo.GetByID("queue-1")
	if retrieved.Status != QueueStatusDownloading {
		t.Errorf("Expected status '%s', got '%s'", QueueStatusDownloading, retrieved.Status)
	}

	// 测试重新排序
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

	// 测试列表（应按优先级排序）
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

	// 测试加载（默认设置）
	settings, err := repo.Load()
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}
	if settings.DownloadDir != "downloads" {
		t.Errorf("Expected default download dir 'downloads', got '%s'", settings.DownloadDir)
	}

	// 测试保存
	settings.DownloadDir = "/custom/downloads"
	settings.DownloadFilenameWithVideoID = false
	settings.ConcurrentLimit = 5
	err = repo.Save(settings)
	if err != nil {
		t.Fatalf("Failed to save settings: %v", err)
	}

	// 测试保存后加载
	loaded, err := repo.Load()
	if err != nil {
		t.Fatalf("Failed to load settings after save: %v", err)
	}
	if loaded.DownloadDir != "/custom/downloads" {
		t.Errorf("Expected download dir '/custom/downloads', got '%s'", loaded.DownloadDir)
	}
	if loaded.DownloadFilenameWithVideoID {
		t.Errorf("Expected downloadFilenameWithVideoId false, got true")
	}
	if loaded.ConcurrentLimit != 5 {
		t.Errorf("Expected concurrent limit 5, got %d", loaded.ConcurrentLimit)
	}

	// 测试验证
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
