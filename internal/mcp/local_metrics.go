package mcpserver

import (
	"time"

	"wx_channel/internal/database"
)

type storedMetricsProjection struct {
	LikeCount     *int64
	CommentCount  *int64
	FavoriteCount *int64
	ForwardCount  *int64
	Source        string
	ObservedAt    *string
	Verification  string
	Status        string
	Availability  map[string]string
	Warnings      []string
}

func projectStoredMetricValues(record storedRecord) storedMetricsProjection {
	projection := storedMetricsProjection{
		Source:       "legacy_unknown",
		Status:       "unknown",
		Availability: map[string]string{},
	}

	if snapshot, err := database.ParseVideoMetrics(record.MetricsSnapshot); err == nil {
		observedAt := snapshot.ObservedAt.UTC().Format(time.RFC3339)
		projection.Source = snapshot.Source
		projection.ObservedAt = &observedAt
		projection.Verification = "client_reported"
		projection.LikeCount = snapshot.LikeCount
		projection.CommentCount = snapshot.CommentCount
		projection.FavoriteCount = snapshot.FavoriteCount
		projection.ForwardCount = snapshot.ForwardCount
		observed := 0
		for _, metric := range []struct {
			key   string
			value *int64
		}{
			{"like_count", projection.LikeCount},
			{"comment_count", projection.CommentCount}, {"favorite_count", projection.FavoriteCount}, {"forward_count", projection.ForwardCount},
		} {
			if metric.value == nil {
				projection.Availability[metric.key] = "not_returned"
				projection.Warnings = appendLocalWarnings(projection.Warnings, "metrics_not_returned")
				continue
			}
			projection.Availability[metric.key] = "observed"
			observed++
		}
		switch {
		case observed == 0:
			projection.Status = "not_returned"
		case observed < 4:
			projection.Status = "partial"
		default:
			projection.Status = "observed"
		}
		return projection
	}

	for _, metric := range []struct {
		key   string
		value int64
	}{
		{"like_count", record.LikeCount}, {"comment_count", record.CommentCount},
		{"favorite_count", record.FavCount}, {"forward_count", record.ForwardCount},
	} {
		if metric.value > 0 {
			value := metric.value
			switch metric.key {
			case "like_count":
				projection.LikeCount = &value
			case "comment_count":
				projection.CommentCount = &value
			case "favorite_count":
				projection.FavoriteCount = &value
			case "forward_count":
				projection.ForwardCount = &value
			}
			projection.Availability[metric.key] = "stored_without_provenance"
			continue
		}
		projection.Availability[metric.key] = "unknown"
		if metric.value == 0 {
			projection.Warnings = appendLocalWarnings(projection.Warnings, "stored_zero_metrics_unknown")
		} else {
			projection.Warnings = appendLocalWarnings(projection.Warnings, "stored_negative_metrics_unknown")
		}
	}
	return projection
}

func applyStoredMetricsToAnalysisDetails(details *videoAnalysisDetails, record *storedRecord) {
	if details == nil {
		return
	}
	if record == nil {
		details.MetricsStatus = "unassociated"
		details.MetricsAvailability = map[string]string{}
		for _, key := range []string{"like_count", "comment_count", "favorite_count", "forward_count"} {
			details.MetricsAvailability[key] = "unavailable"
		}
		return
	}
	projection := projectStoredMetricValues(*record)
	details.LikeCount = projection.LikeCount
	details.CommentCount = projection.CommentCount
	details.FavoriteCount = projection.FavoriteCount
	details.ForwardCount = projection.ForwardCount
	details.MetricsSource = projection.Source
	details.MetricsObservedAt = projection.ObservedAt
	details.MetricsVerification = projection.Verification
	details.MetricsStatus = projection.Status
	details.MetricsAvailability = projection.Availability
}

func projectStoredMetrics(data map[string]any, record storedRecord) []string {
	projection := projectStoredMetricValues(record)
	data["metrics_availability"] = map[string]any{}
	for key, value := range projection.Availability {
		data["metrics_availability"].(map[string]any)[key] = value
	}
	data["metrics_source"] = projection.Source
	data["metrics_observed_at"] = nil
	if projection.ObservedAt != nil {
		data["metrics_observed_at"] = *projection.ObservedAt
	}
	data["metrics_source_verification"] = nil
	if projection.Verification != "" {
		data["metrics_source_verification"] = projection.Verification
	}
	for _, metric := range []struct {
		key   string
		value *int64
	}{
		{"like_count", projection.LikeCount},
		{"comment_count", projection.CommentCount}, {"favorite_count", projection.FavoriteCount}, {"forward_count", projection.ForwardCount},
	} {
		data[metric.key] = nil
		if metric.value != nil {
			data[metric.key] = *metric.value
		}
	}
	return projection.Warnings
}
