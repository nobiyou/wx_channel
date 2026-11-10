package mcpserver

import (
	"strings"
	"testing"
	"time"

	"wx_channel/internal/database"
)

func TestAnalysisDetailsProjectsMetricSnapshot(t *testing.T) {
	at := time.Date(2026, 11, 10, 2, 3, 4, 0, time.UTC)
	record := &storedRecord{MetricsSnapshot: database.CaptureVideoMetrics(map[string]any{
		"playCount": float64(100), "likeCount": float64(20), "commentCount": float64(3),
		"favCount": float64(4), "forwardCount": float64(5),
	}, "wechat_api", at)}
	details := videoAnalysisDetails{}
	applyStoredMetricsToAnalysisDetails(&details, record)

	if details.MetricsStatus != "observed" || details.MetricsSource != "wechat_api" || details.MetricsVerification != "client_reported" {
		t.Fatalf("metric status = %#v", details)
	}
	if details.MetricsObservedAt == nil || *details.MetricsObservedAt != at.Format(time.RFC3339) {
		t.Fatalf("observed at = %#v", details.MetricsObservedAt)
	}
	for name, value := range map[string]*int64{
		"like": details.LikeCount, "comment": details.CommentCount,
		"favorite": details.FavoriteCount, "forward": details.ForwardCount,
	} {
		if value == nil {
			t.Errorf("%s metric is nil", name)
		}
	}
	if details.LikeCount == nil || *details.LikeCount != 20 || details.CommentCount == nil || *details.CommentCount != 3 {
		t.Fatalf("metric values = %#v", details)
	}
}

func TestAnalysisDetailsKeepsUnassociatedMetricsUnknown(t *testing.T) {
	details := videoAnalysisDetails{}
	applyStoredMetricsToAnalysisDetails(&details, nil)
	if details.MetricsStatus != "unassociated" || details.LikeCount != nil || details.CommentCount != nil {
		t.Fatalf("unassociated metrics = %#v", details)
	}
	if details.MetricsAvailability["like_count"] != "unavailable" {
		t.Fatalf("availability = %#v", details.MetricsAvailability)
	}
	var html strings.Builder
	writeVideoDetailsTable(&html, details)
	page := html.String()
	for _, term := range []string{"点赞量</th><td>未关联采集记录", "互动数据状态</th><td>待确认（未关联采集记录）"} {
		if !strings.Contains(page, term) {
			t.Errorf("details table missing %q", term)
		}
	}
}
