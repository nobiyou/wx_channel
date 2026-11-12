package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"wx_channel/internal/database"
	"wx_channel/internal/services"
)

func TestBatchSaveDownloadRecordPersistsInteractionMetrics(t *testing.T) {
	t.Cleanup(databaseTestSetupForHandlers(t))
	filePath := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(filePath, []byte("video"), 0600); err != nil {
		t.Fatalf("write test video: %v", err)
	}

	h := &BatchHandler{downloadService: services.NewDownloadRecordService()}
	h.saveDownloadRecord(&BatchTask{
		ID:                "batch-video",
		Title:             "批量视频",
		Author:            "测试作者",
		MetricsSource:     "wechat_dom",
		MetricsObservedAt: "2026-11-12T00:00:00Z",
		MetricValues: map[string]any{
			"likeCount": float64(7), "commentCount": float64(2),
			"favCount": float64(3), "forwardCount": float64(4),
		},
	}, filePath, database.DownloadStatusCompleted)

	record, err := database.NewDownloadRecordRepository().GetByID("batch-video")
	if err != nil || record == nil {
		t.Fatalf("read batch download record: %v, %+v", err, record)
	}
	if record.LikeCount != 7 || record.CommentCount != 2 || record.FavCount != 3 || record.ForwardCount != 4 {
		t.Fatalf("batch metrics = %+v, want reported values", record)
	}
	if snapshot, err := database.ParseVideoMetrics(record.MetricsSnapshot); err != nil || !snapshot.ObservedAt.Equal(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("batch metric snapshot = %+v, %v", snapshot, err)
	}
}
