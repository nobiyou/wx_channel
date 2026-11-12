package database

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVideoMetricsMigrationPreservesLegacyRecords(t *testing.T) {
	testDB, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	previous := db
	db = testDB
	t.Cleanup(func() { _ = testDB.Close(); db = previous })
	for _, migration := range migrations {
		if migration.Version >= 21 {
			break
		}
		if _, err := db.Exec(migration.Up); err != nil {
			t.Fatalf("old migration %d: %v", migration.Version, err)
		}
	}
	if _, err := db.Exec("INSERT INTO schema_migrations(version) VALUES(20)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO browse_history(id,title,author,browse_time,like_count) VALUES('old','title','author',CURRENT_TIMESTAMP,0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO download_records(id,video_id,title,author,download_time,status,like_count) VALUES('old','video','title','author',CURRENT_TIMESTAMP,'completed',12)"); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(); err != nil {
		t.Fatalf("migration not repeatable: %v", err)
	}
	for _, tc := range []struct {
		table string
		count int64
	}{{"browse_history", 0}, {"download_records", 12}} {
		var snapshot string
		var count int64
		if err := db.QueryRow("SELECT metrics_snapshot, like_count FROM "+tc.table+" WHERE id='old'").Scan(&snapshot, &count); err != nil {
			t.Fatal(err)
		}
		if snapshot != "" || count != tc.count {
			t.Fatalf("legacy %s changed: %q, %d", tc.table, snapshot, count)
		}
	}
}

func TestVideoMetricsSnapshotPreservesPresence(t *testing.T) {
	at := time.Date(2026, 11, 6, 1, 2, 3, 0, time.UTC)
	raw := CaptureVideoMetrics(map[string]any{
		"playCount": json.Number("9223372036854775807"), "likeCount": float64(0),
		"commentCount": nil, "favCount": float64(-1), "forwardCount": "1.2万",
	}, "wechat_api", at)
	snapshot, err := ParseVideoMetrics(raw)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.LikeCount == nil || *snapshot.LikeCount != 0 {
		t.Fatalf("observed values lost: %+v", snapshot)
	}
	if snapshot.CommentCount != nil || snapshot.FavoriteCount != nil || snapshot.ForwardCount != nil || !snapshot.ObservedAt.Equal(at) || snapshot.Source != "wechat_api" {
		t.Fatalf("missing/invalid values became known: %+v", snapshot)
	}
	for _, value := range []any{true, 0.5, math.Inf(1), math.NaN(), float64(1 << 53), "-1", "9223372036854775808", "", "1,234"} {
		s, err := ParseVideoMetrics(CaptureVideoMetrics(map[string]any{"likeCount": value}, "wechat_dom", at))
		if err != nil || s.LikeCount != nil {
			t.Errorf("invalid count %v accepted: %+v, %v", value, s, err)
		}
	}
	if got := CaptureVideoMetrics(map[string]any{"likeCount": float64(0)}, "", at); got != "" {
		t.Fatalf("untagged legacy input acquired provenance: %s", got)
	}
	readOnly, err := ParseVideoMetrics(CaptureVideoMetrics(map[string]any{"readCount": float64(99)}, "wechat_api", at))
	if err != nil || readOnly.LikeCount != nil {
		t.Fatalf("readCount acquired a metric value: %+v, %v", readOnly, err)
	}
	if strings.Contains(raw, `"play_count"`) {
		t.Fatal("retired play_count was written to the snapshot")
	}
	legacy, err := ParseVideoMetrics(`{"source":"wechat_api","observed_at":"2026-11-06T00:00:00Z","play_count":100,"like_count":1}`)
	if err != nil || legacy.LikeCount == nil || *legacy.LikeCount != 1 {
		t.Fatalf("legacy play_count snapshot was not read compatibly: %+v, %v", legacy, err)
	}
	for _, raw := range []string{"", "{}", `{"source":"secret","observed_at":"2026-11-06T00:00:00Z"}`, `{"source":"wechat_api","observed_at":"2026-11-06T00:00:00Z","like_count":-1}`} {
		if _, err := ParseVideoMetrics(raw); err == nil {
			t.Errorf("invalid snapshot accepted: %s", raw)
		}
	}
}

func TestReportedVideoMetricsRetainsCaptureTime(t *testing.T) {
	received := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	observed := received.Add(-2 * time.Hour)
	raw := CaptureReportedVideoMetrics(map[string]any{"likeCount": float64(0)}, "wechat_api", observed.Format(time.RFC3339Nano), received)
	snapshot, err := ParseVideoMetrics(raw)
	if err != nil || !snapshot.ObservedAt.Equal(observed) {
		t.Fatalf("cache was re-dated: %+v, %v", snapshot, err)
	}
	for _, stamp := range []string{"", "invalid", received.Add(time.Second).Format(time.RFC3339), received.Add(5 * time.Minute).Format(time.RFC3339), received.Add(10 * time.Minute).Format(time.RFC3339)} {
		if got := CaptureReportedVideoMetrics(map[string]any{"likeCount": float64(0)}, "wechat_api", stamp, received); got != "" {
			t.Errorf("invalid capture acquired current observation: %s", got)
		}
	}
}

func TestVideoMetricsRepositoryRoundTrip(t *testing.T) {
	t.Cleanup(setupTestDB(t))
	at := time.Date(2026, 11, 6, 1, 2, 3, 0, time.UTC)
	raw := CaptureVideoMetrics(map[string]any{"likeCount": float64(0)}, "wechat_api", at)
	browse := &BrowseRecord{ID: "known", BrowseTime: at, MetricsSnapshot: raw}
	br := NewBrowseHistoryRepository()
	if err := br.Create(browse); err != nil {
		t.Fatal(err)
	}
	got, err := br.GetByID(browse.ID)
	if err != nil || got.MetricsSnapshot != raw {
		t.Fatalf("browse round trip: %+v, %v", got, err)
	}
	got.MetricsSnapshot = CaptureVideoMetrics(map[string]any{"commentCount": float64(8)}, "wechat_dom", at.Add(time.Hour))
	if err := br.Update(got); err != nil {
		t.Fatal(err)
	}
	updated, err := br.GetByID(got.ID)
	if err != nil || updated.MetricsSnapshot != got.MetricsSnapshot {
		t.Fatalf("browse update: %+v, %v", updated, err)
	}
	download := &DownloadRecord{ID: "download", VideoID: "known", DownloadTime: at, Status: DownloadStatusCompleted, MetricsSnapshot: raw}
	dr := NewDownloadRecordRepository()
	if err := dr.Create(download); err != nil {
		t.Fatal(err)
	}
	for _, read := range []func() (*DownloadRecord, error){func() (*DownloadRecord, error) { return dr.GetByID(download.ID) }, func() (*DownloadRecord, error) { return dr.GetByVideoID(download.VideoID) }} {
		d, err := read()
		if err != nil || d.MetricsSnapshot != raw {
			t.Fatalf("download round trip: %+v, %v", d, err)
		}
		d.Status = DownloadStatusFailed
		if err := dr.Update(d); err != nil {
			t.Fatal(err)
		}
	}
	if err := br.Create(&BrowseRecord{ID: "legacy", BrowseTime: at, LikeCount: 3}); err != nil {
		t.Fatal(err)
	}
	legacy, err := br.GetByID("legacy")
	if err != nil || legacy.MetricsSnapshot != "" || legacy.LikeCount != 3 {
		t.Fatalf("legacy record changed: %+v, %v", legacy, err)
	}
	b, _ := json.Marshal(browse)
	var fields map[string]any
	_ = json.Unmarshal(b, &fields)
	if _, exists := fields["MetricsSnapshot"]; exists {
		t.Fatal("internal provenance leaked into GUI contract")
	}
}

func TestMergeBrowseMetricsIntoDownloadPreservesExplicitDownloadObservation(t *testing.T) {
	at := time.Date(2026, 11, 6, 1, 2, 3, 0, time.UTC)
	browseSnapshot := CaptureVideoMetrics(map[string]any{
		"likeCount": float64(12), "commentCount": float64(3), "favCount": float64(4), "forwardCount": float64(5),
	}, "wechat_dom", at)
	browse := &BrowseRecord{
		ID: "video-1", LikeCount: 12, CommentCount: 3, FavCount: 4, ForwardCount: 5,
		MetricsSnapshot: browseSnapshot,
	}

	download := &DownloadRecord{VideoID: "video-1"}
	if !MergeBrowseMetricsIntoDownload(download, browse) {
		t.Fatal("expected browse metrics to fill a legacy download record")
	}
	if download.LikeCount != 12 || download.CommentCount != 3 || download.FavCount != 4 || download.ForwardCount != 5 {
		t.Fatalf("download metrics = %+v, want browse metrics", download)
	}
	if download.MetricsSnapshot != browseSnapshot {
		t.Fatalf("download snapshot = %q, want %q", download.MetricsSnapshot, browseSnapshot)
	}

	explicitZero := CaptureVideoMetrics(map[string]any{"likeCount": float64(0)}, "wechat_api", at.Add(time.Hour))
	download = &DownloadRecord{VideoID: "video-1", MetricsSnapshot: explicitZero}
	if MergeBrowseMetricsIntoDownload(download, browse) {
		t.Fatal("explicit download observation should remain authoritative")
	}
	if download.LikeCount != 0 || download.MetricsSnapshot != explicitZero {
		t.Fatalf("explicit download observation changed: %+v", download)
	}
}
