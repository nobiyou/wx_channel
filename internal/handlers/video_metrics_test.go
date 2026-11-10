package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"wx_channel/internal/database"
)

func TestProfileMetricPresencePersists(t *testing.T) {
	t.Cleanup(databaseTestSetupForHandlers(t))
	h := &APIHandler{}
	observed := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	h.processVideoData(map[string]any{"id": "metrics-test", "title": "test", "metricsSource": "wechat_api", "metricsObservedAt": observed.Format(time.RFC3339), "likeCount": float64(0)})
	record, err := database.NewBrowseHistoryRepository().GetByID("metrics-test")
	if err != nil || record == nil {
		t.Fatalf("missing record: %v", err)
	}
	snapshot, err := database.ParseVideoMetrics(record.MetricsSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.LikeCount == nil || *snapshot.LikeCount != 0 || snapshot.CommentCount != nil || !snapshot.ObservedAt.Equal(observed) {
		t.Fatalf("profile presence/time lost: %+v", snapshot)
	}
	// A later partial capture supersedes the earlier snapshot, without combining
	// counters from different observations or turning missing fields into zero.
	h.processVideoData(map[string]any{"id": "metrics-test", "metricsSource": "wechat_dom", "metricsObservedAt": observed.Format(time.RFC3339), "commentCount": float64(2)})
	record, _ = database.NewBrowseHistoryRepository().GetByID("metrics-test")
	snapshot, err = database.ParseVideoMetrics(record.MetricsSnapshot)
	if err != nil || snapshot.LikeCount != nil || snapshot.CommentCount == nil || *snapshot.CommentCount != 2 || snapshot.Source != "wechat_dom" {
		t.Fatalf("partial update combined observations: %+v, %v", snapshot, err)
	}
}

func TestDownloadMetricCaptureSeparatesLegacyDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		known      bool
	}{
		{"new", `{"metricsSource":"wechat_api","metricsObservedAt":"2026-11-06T10:00:00Z","metricValues":{"likeCount":0,"commentCount":null},"likeCount":0,"commentCount":0}`, true},
		{"old", `{"likeCount":0,"commentCount":0}`, false},
		{"untimed", `{"metricsSource":"wechat_api","metricValues":{"likeCount":0},"likeCount":0}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request DownloadVideoRequest
			if err := json.Unmarshal([]byte(tc.body), &request); err != nil {
				t.Fatal(err)
			}
			raw := database.CaptureReportedVideoMetrics(request.MetricValues, request.MetricsSource, request.MetricsObservedAt, time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC))
			if !tc.known {
				if raw != "" {
					t.Fatalf("legacy defaults became observations: %s", raw)
				}
				return
			}
			snapshot, err := database.ParseVideoMetrics(raw)
			if err != nil || snapshot.LikeCount == nil || *snapshot.LikeCount != 0 || snapshot.CommentCount != nil {
				t.Fatalf("download presence lost: %+v, %v", snapshot, err)
			}
		})
	}
}
