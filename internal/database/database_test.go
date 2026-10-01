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

func TestBrowseHistoryRepositoryPreservesFileFormat(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewBrowseHistoryRepository()
	browseTime := time.Now()
	records := []*BrowseRecord{
		{
			ID:         "browse-format-1",
			Title:      "Format One",
			Author:     "Author One",
			AuthorID:   "author-1",
			Duration:   60,
			Size:       1024,
			Resolution: "720p",
			FileFormat: "xWT111",
			CoverURL:   "https://example.com/cover-1.jpg",
			VideoURL:   "https://example.com/video-1.mp4",
			BrowseTime: browseTime.Add(-time.Minute),
		},
		{
			ID:         "browse-format-2",
			Title:      "Format Two",
			Author:     "Author Two",
			AuthorID:   "author-2",
			Duration:   120,
			Size:       2048,
			Resolution: "1080p",
			FileFormat: "xWT128",
			CoverURL:   "https://example.com/cover-2.jpg",
			VideoURL:   "https://example.com/video-2.mp4",
			BrowseTime: browseTime,
		},
	}

	for _, record := range records {
		if err := repo.Create(record); err != nil {
			t.Fatalf("create %s: %v", record.ID, err)
		}
	}

	assertRecordFormat := func(context string, record *BrowseRecord, want string) {
		t.Helper()
		if record == nil {
			t.Fatalf("%s: got nil record", context)
		}
		if record.FileFormat != want {
			t.Errorf("%s: file format = %q, want %q", context, record.FileFormat, want)
		}
	}

	got, err := repo.GetByID(records[0].ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	assertRecordFormat("get by id", got, "xWT111")

	got, err = repo.GetByID(records[1].ID)
	if err != nil {
		t.Fatalf("get second by id: %v", err)
	}
	assertRecordFormat("get second by id", got, "xWT128")

	records[0].Title = "Format One Updated"
	records[0].FileFormat = "xWT111-updated"
	if err := repo.Update(records[0]); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err = repo.GetByID(records[0].ID)
	if err != nil {
		t.Fatalf("get updated by id: %v", err)
	}
	assertRecordFormat("get updated by id", got, "xWT111-updated")

	list, err := repo.List(&PaginationParams{Page: 1, PageSize: 10, SortDesc: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("list returned %d records, want 2", len(list.Items))
	}
	listFormats := map[string]string{}
	for _, record := range list.Items {
		listFormats[record.ID] = record.FileFormat
	}
	if listFormats[records[0].ID] != "xWT111-updated" || listFormats[records[1].ID] != "xWT128" {
		t.Fatalf("list formats = %#v", listFormats)
	}

	search, err := repo.Search("Updated", &PaginationParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if search.Total != 1 || len(search.Items) != 1 {
		t.Fatalf("search returned total=%d items=%d, want one", search.Total, len(search.Items))
	}
	assertRecordFormat("search", &search.Items[0], "xWT111-updated")

	byIDs, err := repo.GetByIDs([]string{records[0].ID, records[1].ID})
	if err != nil {
		t.Fatalf("get by ids: %v", err)
	}
	byIDFormats := map[string]string{}
	for _, record := range byIDs {
		byIDFormats[record.ID] = record.FileFormat
	}
	if byIDFormats[records[0].ID] != "xWT111-updated" || byIDFormats[records[1].ID] != "xWT128" {
		t.Fatalf("get by ids formats = %#v", byIDFormats)
	}

	since, err := repo.GetRecordsSince(browseTime.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("get records since: %v", err)
	}
	if len(since) != 2 {
		t.Fatalf("get records since returned %d records, want 2", len(since))
	}
	sinceFormats := map[string]string{}
	for _, record := range since {
		sinceFormats[record.ID] = record.FileFormat
	}
	if sinceFormats[records[0].ID] != "xWT111-updated" || sinceFormats[records[1].ID] != "xWT128" {
		t.Fatalf("get records since formats = %#v", sinceFormats)
	}

	recent, err := repo.GetRecent(10)
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("get recent returned %d records, want 2", len(recent))
	}
	assertRecordFormat("get recent", &recent[0], "xWT128")

	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("get all returned %d records, want 2", len(all))
	}
	allFormats := map[string]string{}
	for _, record := range all {
		allFormats[record.ID] = record.FileFormat
	}
	if allFormats[records[0].ID] != "xWT111-updated" || allFormats[records[1].ID] != "xWT128" {
		t.Fatalf("get all formats = %#v", allFormats)
	}
}

func TestQueueRepositoryPreservesResolutionAndFileFormat(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewQueueRepository()
	item := &QueueItem{
		ID:         "queue-format-1",
		VideoID:    "video-format-1",
		Title:      "Queue Format",
		Author:     "Queue Author",
		VideoURL:   "https://example.com/video.mp4",
		Duration:   90,
		Resolution: "720p",
		FileFormat: "xWT111",
		TotalSize:  4096,
		Status:     QueueStatusPending,
		Priority:   2,
		AddedTime:  time.Now(),
		ChunkSize:  1024,
	}
	if err := repo.Add(item); err != nil {
		t.Fatalf("add: %v", err)
	}

	assertItemFormat := func(context string, got *QueueItem, wantResolution, wantFormat string) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: got nil item", context)
		}
		if got.Resolution != wantResolution || got.FileFormat != wantFormat {
			t.Errorf("%s: resolution=%q file format=%q, want resolution=%q file format=%q", context, got.Resolution, got.FileFormat, wantResolution, wantFormat)
		}
	}

	got, err := repo.GetByID(item.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	assertItemFormat("get by id", got, "720p", "xWT111")

	got, err = repo.GetByVideoID(item.VideoID)
	if err != nil {
		t.Fatalf("get by video id: %v", err)
	}
	assertItemFormat("get by video id", got, "720p", "xWT111")

	item.Resolution = "1080p"
	item.FileFormat = "xWT128"
	if err := repo.Update(item); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err = repo.GetByID(item.ID)
	if err != nil {
		t.Fatalf("get updated by id: %v", err)
	}
	assertItemFormat("get updated by id", got, "1080p", "xWT128")

	list, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list returned %d items, want 1", len(list))
	}
	assertItemFormat("list", &list[0], "1080p", "xWT128")

	byStatus, err := repo.ListByStatus(QueueStatusPending)
	if err != nil {
		t.Fatalf("list by status: %v", err)
	}
	if len(byStatus) != 1 {
		t.Fatalf("list by status returned %d items, want 1", len(byStatus))
	}
	assertItemFormat("list by status", &byStatus[0], "1080p", "xWT128")

	next, err := repo.GetNextPending()
	if err != nil {
		t.Fatalf("get next pending: %v", err)
	}
	assertItemFormat("get next pending", next, "1080p", "xWT128")
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

func TestDownloadRecordRepositoryCountsStoredLocalDate(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDownloadRecordRepository()
	localTime := time.Date(2026, time.August, 24, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	record := &DownloadRecord{
		ID:           "download-local-date",
		VideoID:      "video-local-date",
		Title:        "Local Date Boundary",
		Status:       DownloadStatusCompleted,
		DownloadTime: localTime,
	}
	if err := repo.Create(record); err != nil {
		t.Fatalf("Failed to create local-date download record: %v", err)
	}
	var storedDownloadTime string
	if err := repo.db.QueryRow("SELECT download_time FROM download_records WHERE id = ?", record.ID).Scan(&storedDownloadTime); err != nil {
		t.Fatalf("Failed to read stored download time: %v", err)
	}

	count, err := repo.countByLocalDate("2026-08-24")
	if err != nil {
		t.Fatalf("Failed to count downloads by stored local date: %v", err)
	}
	if count != 1 {
		t.Fatalf("Expected 1 download on stored local date, got %d (stored value %q)", count, storedDownloadTime)
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
