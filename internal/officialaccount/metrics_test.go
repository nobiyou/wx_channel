package officialaccount

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExtractArticleMetricsReadsScriptValuesAndChineseUnits(t *testing.T) {
	payload, raw := ExtractArticleMetrics(`<html><head><script>
        window.read_num = "12,345";
        var like_num = "1.2万";
        var comment_count = 23;
        window.share_count = "456";
        window.collect_count = "789";
        window.reward_count = "10";
    </script></head><body>正文</body></html>`)

	if payload.ViewCount == nil || *payload.ViewCount != 12345 {
		t.Fatalf("view count = %v, want 12345", payload.ViewCount)
	}
	if payload.LikeCount == nil || *payload.LikeCount != 12000 {
		t.Fatalf("like count = %v, want 12000", payload.LikeCount)
	}
	if payload.CommentCount == nil || *payload.CommentCount != 23 {
		t.Fatalf("comment count = %v, want 23", payload.CommentCount)
	}
	if payload.ShareCount == nil || *payload.ShareCount != 456 {
		t.Fatalf("share count = %v, want 456", payload.ShareCount)
	}
	if payload.CollectCount == nil || *payload.CollectCount != 789 {
		t.Fatalf("collect count = %v, want 789", payload.CollectCount)
	}
	if payload.RewardCount == nil || *payload.RewardCount != 10 {
		t.Fatalf("reward count = %v, want 10", payload.RewardCount)
	}
	if !strings.Contains(raw, `"view_count"`) || strings.Contains(raw, "正文") {
		t.Fatalf("unexpected metric evidence: %s", raw)
	}
}

func TestExtractArticleMetricsScriptPrefersCurrentInteractionFields(t *testing.T) {
	payload, raw := ExtractArticleMetrics(`<script>
		window.old_like_num = 630;
		window.like_num = 426;
	</script>`)
	if payload.LikeCount == nil || *payload.LikeCount != 426 {
		t.Fatalf("like_num should take precedence over old_like_num in scripts: payload=%+v raw=%s", payload, raw)
	}
}

func TestExtractArticleMetricsReadsDOMAndLeavesUnavailableFieldsUnknown(t *testing.T) {
	payload, _ := ExtractArticleMetrics(`<div id="readNum3">2.5万</div>
        <span id="likeNum">18</span>
        <span class="comment_count">4</span>`)
	if payload.ViewCount == nil || *payload.ViewCount != 25000 {
		t.Fatalf("DOM view count = %v, want 25000", payload.ViewCount)
	}
	if payload.LikeCount == nil || *payload.LikeCount != 18 {
		t.Fatalf("DOM like count = %v, want 18", payload.LikeCount)
	}
	if payload.CommentCount == nil || *payload.CommentCount != 4 {
		t.Fatalf("DOM comment count = %v, want 4", payload.CommentCount)
	}
	if payload.ShareCount != nil || payload.CollectCount != nil || payload.RewardCount != nil {
		t.Fatalf("unavailable counters must remain nil: %+v", payload)
	}
}

func TestExtractArticleMetricsJSONReadsNestedAndEncodedAppmsgStats(t *testing.T) {
	payload, raw := ExtractArticleMetricsJSON(`{"data":{"appmsg_stat":"{\"read_num\":321,\"old_like_num\":17,\"comment_num\":8,\"share_num\":5,\"favorite_num\":3,\"reward_num\":1}"}}`)
	if payload.ViewCount == nil || *payload.ViewCount != 321 || payload.LikeCount == nil || *payload.LikeCount != 17 ||
		payload.CommentCount == nil || *payload.CommentCount != 8 || payload.ShareCount == nil || *payload.ShareCount != 5 ||
		payload.CollectCount == nil || *payload.CollectCount != 3 || payload.RewardCount == nil || *payload.RewardCount != 1 {
		t.Fatalf("unexpected nested appmsg stats: %+v", payload)
	}
	if !strings.Contains(raw, `"like_count"`) || strings.Contains(raw, "appmsg_stat") {
		t.Fatalf("unexpected metric evidence: %s", raw)
	}
}

func TestExtractArticleMetricsJSONPrefersCurrentInteractionFields(t *testing.T) {
	payload, _ := ExtractArticleMetricsJSON(`{"appmsgstat":{"old_like_num":630,"like_num":426,"favorite_num":240,"collect_num":174}}`)
	if payload.LikeCount == nil || *payload.LikeCount != 426 {
		t.Fatalf("like_num should take precedence over old_like_num: %+v", payload)
	}
	if payload.CollectCount == nil || *payload.CollectCount != 174 {
		t.Fatalf("collect_num should take precedence over favorite_num: %+v", payload)
	}
}

func TestExtractArticleMetricsJSONReadsCommentTotalField(t *testing.T) {
	payload, _ := ExtractArticleMetricsJSON(`{"elected_comment_total_cnt":37}`)
	if payload.CommentCount == nil || *payload.CommentCount != 37 {
		t.Fatalf("comment total field was not parsed: %+v", payload)
	}
}

func TestMergeMetricPayloadWithSnapshotKeepsExplicitZero(t *testing.T) {
	oldComment := int64(8)
	currentComment := int64(0)
	merged := mergeMetricPayloadWithSnapshot(
		ArticleMetricPayload{CommentCount: &currentComment},
		&ArticleMetricSnapshot{CommentCount: &oldComment},
	)
	if merged.CommentCount == nil || *merged.CommentCount != 0 {
		t.Fatalf("explicit zero was replaced by historical value: %+v", merged)
	}
}

func TestExtractArticleMetricsFindsEmbeddedJSONInArticleHTML(t *testing.T) {
	payload, _ := ExtractArticleMetrics(`<script>window.cgiData = {data:{appmsgstat:{read_num:900,old_like_num:12,comment_num:4,share_num:2}}};</script>`)
	if payload.ViewCount == nil || *payload.ViewCount != 900 || payload.LikeCount == nil || *payload.LikeCount != 12 ||
		payload.CommentCount == nil || *payload.CommentCount != 4 || payload.ShareCount == nil || *payload.ShareCount != 2 {
		t.Fatalf("embedded JSON metrics = %+v", payload)
	}
}

func TestHandleArticleMetricsPersistsArticleAndSnapshot(t *testing.T) {
	service := NewMemoryService()
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/mp/metrics", strings.NewReader(`{
        "biz":"biz-metrics",
        "article":{"title":"指标文章","content_url":"https://mp.weixin.qq.com/s/metrics?mid=9&idx=1"},
        "metrics":{"like_count":99},
        "html":"<script>var read_num=123; var like_num=1; var comment_count=7;</script>"
    }`))
	recorder := httptest.NewRecorder()
	service.HandleArticleMetrics(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code int                 `json:"code"`
		Data MetricCaptureResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if response.Code != 0 || !response.Data.Stored || response.Data.Metrics.ViewCount == nil || *response.Data.Metrics.ViewCount != 123 {
		t.Fatalf("unexpected capture response: %+v", response)
	}
	if response.Data.Metrics.LikeCount == nil || *response.Data.Metrics.LikeCount != 99 {
		t.Fatalf("explicit metric did not win over HTML: %+v", response.Data.Metrics)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.articles) != 1 || len(store.metrics) != 1 {
		t.Fatalf("article and metric were not persisted: articles=%d metrics=%d", len(store.articles), len(store.metrics))
	}
	state := store.metricStates[response.Data.ArticleKey]
	if response.Data.State == nil || state.Status != MetricStateSuccess || state.AttemptCount != 1 || state.SuccessCount != 1 || state.LastAttemptAt == 0 || state.LastSuccessAt == 0 {
		t.Fatalf("manual metric capture did not persist success state: response=%+v state=%+v", response.Data, state)
	}
	if store.metrics[0].ShareCount != nil || store.metrics[0].CollectCount != nil {
		t.Fatalf("missing metrics should remain nil: %+v", store.metrics[0])
	}
}

func TestHandleArticleMetricsDoesNotCreateEmptySnapshot(t *testing.T) {
	service := NewMemoryService()
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/mp/metrics", strings.NewReader(`{
        "biz":"biz-empty",
        "article":{"content_url":"https://mp.weixin.qq.com/s/empty?mid=10&idx=1"}
    }`))
	recorder := httptest.NewRecorder()
	service.HandleArticleMetrics(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected empty metrics 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(store.metrics) != 0 {
		t.Fatalf("empty metrics should not create a snapshot: %+v", store.metrics)
	}
	store.mu.Lock()
	var state ArticleMetricState
	for _, candidate := range store.metricStates {
		state = candidate
		break
	}
	store.mu.Unlock()
	if state.Status != MetricStateUnknown || state.AttemptCount != 1 || state.UnknownCount != 1 {
		t.Fatalf("empty metrics should persist unknown state: %+v", state)
	}
}

func TestHandleArticleMetricsPreservesHistoricalMissingFields(t *testing.T) {
	service := NewMemoryService()
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-metrics-history", ArticleItem{
		ContentURL: "https://mp.weixin.qq.com/s/history?mid=11&idx=1",
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	oldComment := int64(8)
	store.mu.Lock()
	store.metrics = append(store.metrics, ArticleMetricSnapshot{
		ArticleKey: article.Key, ObservedAt: 100, Source: "network", CommentCount: &oldComment,
	})
	store.mu.Unlock()

	request := httptest.NewRequest(http.MethodPost, "/api/mp/metrics", strings.NewReader(`{
        "biz":"biz-metrics-history",
        "article":{"content_url":"https://mp.weixin.qq.com/s/history?mid=11&idx=1"},
        "metrics":{"view_count":21},
        "observed_at":200
    }`))
	recorder := httptest.NewRecorder()
	service.HandleArticleMetrics(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected historical metrics 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code int                 `json:"code"`
		Data MetricCaptureResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode historical metrics response: %v", err)
	}
	if response.Data.Metrics.ViewCount == nil || *response.Data.Metrics.ViewCount != 21 ||
		response.Data.Metrics.CommentCount == nil || *response.Data.Metrics.CommentCount != 8 {
		t.Fatalf("historical missing field was not preserved: %+v", response.Data.Metrics)
	}
}
