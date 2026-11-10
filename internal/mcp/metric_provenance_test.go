package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"wx_channel/internal/database"
)

func TestLocalMetricProvenanceAndObservedZero(t *testing.T) {
	s := localContractServer(t)
	at := time.Date(2026, 11, 6, 1, 2, 3, 0, time.UTC)
	for _, source := range []string{"browse", "download"} {
		raw := database.CaptureVideoMetrics(map[string]any{"likeCount": float64(0), "playCount": float64(18)}, "wechat_api", at)
		if source == "browse" {
			err := database.NewBrowseHistoryRepository().Create(&database.BrowseRecord{ID: source, BrowseTime: at.Add(time.Hour), CommentCount: 99, MetricsSnapshot: raw})
			if err != nil {
				t.Fatal(err)
			}
		} else {
			err := database.NewDownloadRecordRepository().Create(&database.DownloadRecord{ID: source, DownloadTime: at.Add(time.Hour), CommentCount: 99, Status: database.DownloadStatusCompleted, MetricsSnapshot: raw})
			if err != nil {
				t.Fatal(err)
			}
		}
		_, result, err := s.getLocalVideo(context.Background(), nil, getLocalVideoInput{Source: source, ID: source})
		if err != nil {
			t.Fatal(err)
		}
		data := result["data"].(map[string]any)
		if data["like_count"] != int64(0) || data["comment_count"] != nil || data["metrics_source"] != "wechat_api" || data["metrics_observed_at"] != at.Format(time.RFC3339) {
			t.Fatalf("snapshot projection incorrect: %v", data)
		}
		if _, exists := data["play_count"]; exists {
			t.Fatalf("retired play_count leaked: %v", data)
		}
		availability := data["metrics_availability"].(map[string]any)
		if data["metrics_source_verification"] != "client_reported" {
			t.Fatalf("client provenance was presented as independently verified: %v", data)
		}
		if availability["like_count"] != "observed" || availability["comment_count"] != "not_returned" {
			t.Fatalf("availability = %v", availability)
		}
		if strings.Contains(string(localContractJSON(t, result)), "stored_zero_metrics_unknown") {
			t.Fatal("observed zero treated as ambiguous")
		}
	}
	_, list, err := s.listLocalVideos(context.Background(), nil, listLocalVideosInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list["data"].(map[string]any)["items"].([]map[string]any)) != 2 {
		t.Fatal("missing snapshot list items")
	}
}

func TestLocalInvalidSnapshotStaysSafeAndUnknown(t *testing.T) {
	s := &Server{}
	data := s.recordMap(storedRecord{MetricsSnapshot: `{"source":"C:\\private\\token=secret"}`, LikeCount: 4})
	if data["comment_count"] != nil || data["metrics_observed_at"] != nil || data["metrics_source"] != "legacy_unknown" {
		t.Fatalf("invalid snapshot acquired provenance: %v", data)
	}
	if _, exists := data["play_count"]; exists {
		t.Fatalf("retired play_count leaked: %v", data)
	}
	if strings.Contains(string(localContractJSON(t, data)), "secret") {
		t.Fatal("raw snapshot leaked")
	}
}
