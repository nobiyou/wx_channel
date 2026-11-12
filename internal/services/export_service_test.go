package services

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"wx_channel/internal/database"
)

func TestExportDownloadRecordsBackfillsBrowseMetrics(t *testing.T) {
	_ = database.Close()
	if err := database.Initialize(&database.Config{DBPath: filepath.Join(t.TempDir(), "test.db")}); err != nil {
		t.Fatalf("initialize test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	browseSnapshot := database.CaptureVideoMetrics(map[string]any{
		"likeCount": float64(9), "commentCount": float64(2), "favCount": float64(3), "forwardCount": float64(4),
	}, "wechat_dom", at)
	if err := database.NewBrowseHistoryRepository().Create(&database.BrowseRecord{
		ID: "video-1", BrowseTime: at, LikeCount: 9, CommentCount: 2, FavCount: 3, ForwardCount: 4,
		MetricsSnapshot: browseSnapshot,
	}); err != nil {
		t.Fatalf("create browse record: %v", err)
	}
	if err := database.NewDownloadRecordRepository().Create(&database.DownloadRecord{
		ID: "download-1", VideoID: "video-1", DownloadTime: at, Status: database.DownloadStatusCompleted,
	}); err != nil {
		t.Fatalf("create download record: %v", err)
	}

	result, err := NewExportService().ExportDownloadRecords(ExportFormatJSON, nil)
	if err != nil {
		t.Fatalf("export download records: %v", err)
	}
	var exported []database.DownloadRecord
	if err := json.Unmarshal(result.Data, &exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if len(exported) != 1 {
		t.Fatalf("exported %d records, want 1", len(exported))
	}
	if exported[0].LikeCount != 9 || exported[0].CommentCount != 2 || exported[0].FavCount != 3 || exported[0].ForwardCount != 4 {
		t.Fatalf("exported metrics = %+v, want browse metrics", exported[0])
	}

	persisted, err := database.NewDownloadRecordRepository().GetByID("download-1")
	if err != nil {
		t.Fatalf("reload download record: %v", err)
	}
	if persisted.LikeCount != 9 || persisted.CommentCount != 2 || persisted.FavCount != 3 || persisted.ForwardCount != 4 {
		t.Fatalf("persisted metrics = %+v, want repaired metrics", persisted)
	}
}
