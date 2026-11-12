package database

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

// VideoMetricsSnapshot records supported MCP interaction metrics separately
// from the legacy GUI counters. Play count is intentionally outside this
// snapshot because the page does not return it consistently. A nil count means
// the capture did not return a usable value, including when a DOM label was
// abbreviated. Never infer a count from a missing field.
type VideoMetricsSnapshot struct {
	Source        string    `json:"source"`
	ObservedAt    time.Time `json:"observed_at"`
	LikeCount     *int64    `json:"like_count"`
	CommentCount  *int64    `json:"comment_count"`
	FavoriteCount *int64    `json:"favorite_count"`
	ForwardCount  *int64    `json:"forward_count"`
}

func validVideoMetricsSource(source string) bool {
	return source == "wechat_api" || source == "wechat_dom"
}

// CaptureReportedVideoMetrics preserves the page's original observation time
// when a cached profile is downloaded later. Tags and timestamps are client
// reports, not independent verification of a WeChat server response.
func CaptureReportedVideoMetrics(values map[string]any, source, observedAt string, receivedAt time.Time) string {
	stamp, err := time.Parse(time.RFC3339Nano, observedAt)
	if err != nil || stamp.IsZero() || stamp.After(receivedAt) {
		return ""
	}
	return CaptureVideoMetrics(values, source, stamp)
}

// CaptureVideoMetrics accepts only tagged producers that preserve absence.
// Unsupported fields, including the retired play count, are ignored.
// Old clients defaulted missing counters to zero and cannot supply provenance.
func CaptureVideoMetrics(values map[string]any, source string, observedAt time.Time) string {
	if !validVideoMetricsSource(source) || observedAt.IsZero() {
		return ""
	}
	snapshot := VideoMetricsSnapshot{
		Source: source, ObservedAt: observedAt.UTC(),
		LikeCount:    videoMetricCount(values["likeCount"]),
		CommentCount: videoMetricCount(values["commentCount"]), FavoriteCount: videoMetricCount(values["favCount"]),
		ForwardCount: videoMetricCount(values["forwardCount"]),
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func videoMetricCount(value any) *int64 {
	var count int64
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 9007199254740991 || math.Trunc(v) != v {
			return nil
		}
		count = int64(v)
	case json.Number:
		var err error
		count, err = v.Int64()
		if err != nil {
			return nil
		}
	case string:
		if v == "" {
			return nil
		}
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				return nil
			}
		}
		var err error
		count, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil
		}
	case int64:
		count = v
	default:
		return nil
	}
	if count < 0 {
		return nil
	}
	return &count
}

// ParseVideoMetrics validates the small allowlisted projection before MCP uses
// it. Invalid/legacy snapshots must remain unknown, never raw output.
func ParseVideoMetrics(raw string) (*VideoMetricsSnapshot, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return nil, errors.New("invalid video metric snapshot")
	}
	var snapshot VideoMetricsSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil || !validVideoMetricsSource(snapshot.Source) || snapshot.ObservedAt.IsZero() {
		return nil, errors.New("invalid video metric snapshot")
	}
	for _, count := range []*int64{snapshot.LikeCount, snapshot.CommentCount, snapshot.FavoriteCount, snapshot.ForwardCount} {
		if count != nil && *count < 0 {
			return nil, errors.New("invalid video metric snapshot")
		}
	}
	return &snapshot, nil
}

// MergeBrowseMetricsIntoDownload fills a download record from its matching
// browse record when the download path did not receive interaction metrics.
// A valid download snapshot remains authoritative, including explicit zeroes;
// this keeps a later browse observation from being combined into a different
// observation. Legacy records without a snapshot may use the browse counters
// as a compatibility fallback.
func MergeBrowseMetricsIntoDownload(download *DownloadRecord, browse *BrowseRecord) bool {
	if download == nil || browse == nil {
		return false
	}

	currentSnapshot, currentValid := parseVideoMetricsOrNil(download.MetricsSnapshot)
	browseSnapshot, browseValid := parseVideoMetricsOrNil(browse.MetricsSnapshot)
	changed := false

	if currentValid {
		changed = fillDownloadMetricFromSnapshot(&download.LikeCount, currentSnapshot.LikeCount) || changed
		changed = fillDownloadMetricFromSnapshot(&download.CommentCount, currentSnapshot.CommentCount) || changed
		changed = fillDownloadMetricFromSnapshot(&download.FavCount, currentSnapshot.FavoriteCount) || changed
		changed = fillDownloadMetricFromSnapshot(&download.ForwardCount, currentSnapshot.ForwardCount) || changed
		return changed
	}

	if browseValid {
		if download.MetricsSnapshot != browse.MetricsSnapshot {
			download.MetricsSnapshot = browse.MetricsSnapshot
			changed = true
		}
		changed = fillDownloadMetricFromSnapshot(&download.LikeCount, browseSnapshot.LikeCount) || changed
		changed = fillDownloadMetricFromSnapshot(&download.CommentCount, browseSnapshot.CommentCount) || changed
		changed = fillDownloadMetricFromSnapshot(&download.FavCount, browseSnapshot.FavoriteCount) || changed
		changed = fillDownloadMetricFromSnapshot(&download.ForwardCount, browseSnapshot.ForwardCount) || changed
		return changed
	}

	changed = fillDownloadMetricFromLegacy(&download.LikeCount, browse.LikeCount) || changed
	changed = fillDownloadMetricFromLegacy(&download.CommentCount, browse.CommentCount) || changed
	changed = fillDownloadMetricFromLegacy(&download.FavCount, browse.FavCount) || changed
	changed = fillDownloadMetricFromLegacy(&download.ForwardCount, browse.ForwardCount) || changed
	return changed
}

func parseVideoMetricsOrNil(raw string) (*VideoMetricsSnapshot, bool) {
	snapshot, err := ParseVideoMetrics(raw)
	return snapshot, err == nil
}

func fillDownloadMetricFromSnapshot(destination *int64, source *int64) bool {
	if destination == nil || source == nil || *destination == *source {
		return false
	}
	*destination = *source
	return true
}

func fillDownloadMetricFromLegacy(destination *int64, source int64) bool {
	if destination == nil || *destination != 0 || source == 0 {
		return false
	}
	*destination = source
	return true
}
