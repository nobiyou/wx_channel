package officialaccount

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"wx_channel/internal/response"
)

func (s *Service) CaptureArticleMetrics(request MetricRequest) (MetricCaptureResult, error) {
	if s == nil {
		return MetricCaptureResult{}, ErrCatalogUnavailable
	}
	repository := s.catalogRepository()
	if repository == nil {
		return MetricCaptureResult{}, ErrCatalogUnavailable
	}
	biz := strings.TrimSpace(request.Biz)
	if biz == "" {
		return MetricCaptureResult{}, ErrMissingBiz
	}
	capturedAt := s.now()
	now := capturedAt.Unix()
	observedAt := request.ObservedAt
	if observedAt <= 0 {
		observedAt = now
	}
	if observedAt > now+int64((5*time.Minute).Seconds()) {
		observedAt = now
	}
	article, ok := ArticleRecordFromItem(biz, request.Article, capturedAt)
	if !ok {
		return MetricCaptureResult{}, fmt.Errorf("%w: content URL, source URL, or file ID is required", ErrArticleIdentity)
	}

	extracted, rawMetadata := ExtractArticleMetrics(request.HTML)
	metrics := mergeMetricPayload(request.Metrics, extracted)
	if !metricPayloadEmpty(metrics) {
		latest, err := repository.LatestArticleMetrics([]string{article.Key})
		if err != nil {
			return MetricCaptureResult{}, err
		}
		if previous, ok := latest[article.Key]; ok {
			metrics = mergeMetricPayloadWithSnapshot(metrics, &previous)
		}
	}
	result := MetricCaptureResult{
		ArticleKey: article.Key,
		ObservedAt: observedAt,
		Source:     normalizeMetricSource(request.Source),
		Metrics:    metrics,
	}
	if result.Source == "" {
		result.Source = "article_page"
	}
	if _, err := repository.UpsertArticles(biz, []ArticleRecord{article}, capturedAt); err != nil {
		return MetricCaptureResult{}, err
	}
	states, err := repository.ListArticleMetricStates([]string{article.Key})
	if err != nil {
		return MetricCaptureResult{}, err
	}
	state := states[article.Key]
	if state.ArticleKey == "" {
		state = ArticleMetricState{ArticleKey: article.Key, Status: MetricStatePending}
	}
	state.ArticleKey = article.Key
	state.AttemptCount++
	state.LastAttemptAt = now
	state.LastSource = result.Source
	state.LastError = ""
	state.NextRetryAt = 0
	var snapshot *ArticleMetricSnapshot
	if metricPayloadEmpty(metrics) {
		state.Status = MetricStateUnknown
		state.UnknownCount++
		state.LastObservedAt = observedAt
		state.LastError = sanitizeMetricError("article page did not expose article metrics")
	} else {
		state.Status = MetricStateSuccess
		state.SuccessCount++
		state.LastSuccessAt = observedAt
		state.LastObservedAt = observedAt
		snapshot = &ArticleMetricSnapshot{
			ArticleKey:   article.Key,
			ObservedAt:   observedAt,
			Source:       result.Source,
			ViewCount:    metrics.ViewCount,
			LikeCount:    metrics.LikeCount,
			CommentCount: metrics.CommentCount,
			ShareCount:   metrics.ShareCount,
			CollectCount: metrics.CollectCount,
			RewardCount:  metrics.RewardCount,
			RawMetadata:  rawMetadata,
		}
	}
	state.UpdatedAt = now
	if err := repository.SaveArticleMetricResult(state, snapshot); err != nil {
		return MetricCaptureResult{}, err
	}
	result.Stored = snapshot != nil
	result.State = &state
	if snapshot == nil {
		log.Printf("[公众号指标] 未发现文章指标 | biz=%s | article=%s | source=%s", biz, article.Key, result.Source)
		return result, nil
	}
	log.Printf("[公众号指标] 已记录文章指标 | biz=%s | article=%s | source=%s | view=%s like=%s comment=%s share=%s collect=%s reward=%s",
		biz, article.Key, result.Source, formatMetricValue(metrics.ViewCount), formatMetricValue(metrics.LikeCount),
		formatMetricValue(metrics.CommentCount), formatMetricValue(metrics.ShareCount), formatMetricValue(metrics.CollectCount),
		formatMetricValue(metrics.RewardCount))
	return result, nil
}

func (s *Service) HandleArticleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	defer r.Body.Close()
	var request MetricRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody))
	if err := decoder.Decode(&request); err != nil {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid article metrics payload")
		return
	}
	result, err := s.CaptureArticleMetrics(request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, result)
}

func normalizeMetricSource(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var builder strings.Builder
	for _, value := range raw {
		if value < 0x20 || value == 0x7f {
			continue
		}
		builder.WriteRune(value)
		if builder.Len() >= 64 {
			break
		}
	}
	return strings.TrimSpace(builder.String())
}

func formatMetricValue(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *value)
}
