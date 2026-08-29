package officialaccount

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetricSyncHTTPContract(t *testing.T) {
	service := NewMemoryService()
	store := newSyncCatalogStub()
	if err := service.Upsert(Account{Biz: "biz-metric-http", Key: "key-metric-http"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)

	post := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, "/api/mp/metrics/sync", bytes.NewBufferString(`{"biz":"biz-metric-http","force":true}`))
	postRequest.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(post, postRequest)
	if post.Code != http.StatusAccepted {
		t.Fatalf("metric sync POST status = %d: %s", post.Code, post.Body.String())
	}
	var started struct {
		Code int           `json:"code"`
		Data MetricSyncRun `json:"data"`
	}
	if err := json.Unmarshal(post.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode metric sync POST: %v", err)
	}
	if started.Code != 0 || started.Data.ID == "" || !started.Data.Force {
		t.Fatalf("unexpected metric sync POST response: %+v", started)
	}
	completed := waitMetricSyncRun(t, store, started.Data.ID)
	if completed.Status != SyncStatusCompleted || completed.Total != 0 {
		t.Fatalf("empty metric sync did not complete cleanly: %+v", completed)
	}

	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/mp/metrics/sync?biz=biz-metric-http", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("metric sync GET status = %d: %s", get.Code, get.Body.String())
	}
	var status struct {
		Code int           `json:"code"`
		Data MetricSyncRun `json:"data"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode metric sync GET: %v", err)
	}
	if status.Code != 0 || status.Data.ID != started.Data.ID || status.Data.Status != SyncStatusCompleted {
		t.Fatalf("unexpected metric sync GET response: %+v", status)
	}

	deleteRecorder := httptest.NewRecorder()
	mux.ServeHTTP(deleteRecorder, httptest.NewRequest(http.MethodDelete, "/api/mp/metrics/sync/"+started.Data.ID, nil))
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("terminal metric sync DELETE status = %d: %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}

	missingBiz := httptest.NewRecorder()
	mux.ServeHTTP(missingBiz, httptest.NewRequest(http.MethodGet, "/api/mp/metrics/sync", nil))
	if missingBiz.Code != http.StatusBadRequest {
		t.Fatalf("missing biz status = %d: %s", missingBiz.Code, missingBiz.Body.String())
	}
	method := httptest.NewRecorder()
	mux.ServeHTTP(method, httptest.NewRequest(http.MethodPut, "/api/mp/metrics/sync", nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unsupported metric sync method status = %d: %s", method.Code, method.Body.String())
	}
}

func TestMetricSyncStartIsIdempotentWhileRunIsActive(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var requestsMu sync.Mutex
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requestCount++
		requestsMu.Unlock()
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":12,"like_num":3,"comment_num":1}}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-metric-idempotent", Key: "key-idempotent"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-metric-idempotent", ArticleItem{
		ContentURL: "https://mp.weixin.qq.com/s/idempotent?mid=990&idx=1",
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := store.UpsertArticles("biz-metric-idempotent", []ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}

	first, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-idempotent", Force: true})
	if err != nil {
		t.Fatalf("start first metric sync: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first metric request did not start")
	}

	second, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-idempotent", Force: true})
	if err != nil {
		t.Fatalf("duplicate metric sync should be idempotent: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate metric sync ID = %q, want %q", second.ID, first.ID)
	}

	close(release)
	completed := waitMetricSyncRun(t, store, first.ID)
	if completed.Status != SyncStatusCompleted || completed.Attempted != 1 || completed.Stored != 1 {
		t.Fatalf("unexpected idempotent metric run: %+v", completed)
	}
	requestsMu.Lock()
	gotRequests := requestCount
	requestsMu.Unlock()
	if gotRequests != 1 {
		t.Fatalf("duplicate metric sync sent %d upstream requests, want 1", gotRequests)
	}
}

func TestMetricSyncCollectsNestedAppmsgStatsAndPersistsState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mp/getappmsgext" {
			t.Fatalf("unexpected metric path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("metric request method = %s, want POST", r.Method)
		}
		for key, want := range map[string]string{
			"__biz": "biz-metric-sync", "clientversion": "f2541c37", "appmsg_token": "token-sync",
			"f": "json", "mock": "7", "x5": "0",
		} {
			if got := r.URL.Query().Get(key); got != want {
				t.Fatalf("query %s = %q, want %q", key, got, want)
			}
		}
		if got := r.Header.Get("Cookie"); got != "slave_user=metric-sync" {
			t.Fatalf("metric request cookie = %q", got)
		}
		if got := r.Header.Get("X-Requested-With"); got != "XMLHttpRequest" {
			t.Fatalf("metric request X-Requested-With = %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Fatalf("metric request content type = %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse metric form: %v", err)
		}
		for key, want := range map[string]string{
			"appmsg_type": "9", "mid": "900", "sn": "sn-900", "idx": "2", "ct": "100",
			"devicetype": "UnifiedPCWindows", "version": "f2541c37", "msg_daily_idx": "1",
			"is_only_read": "1", "item_show_type": "0", "appmsg_like_type": "2",
			"pass_ticket": "ticket-sync", "comment_id": "", "req_id": "",
		} {
			if got := r.PostForm.Get(key); got != want {
				t.Fatalf("form %s = %q, want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":1200,"old_like_num":630,"like_num":426,"comment_num":12,"favorite_num":240,"collect_num":174,"share_num":8,"reward_num":2}}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-metric-sync", Key: "key-sync", Uin: "uin-sync", PassTicket: "ticket-sync", AppmsgToken: "token-sync", Cookie: "slave_user=metric-sync"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-metric-sync", ArticleItem{
		Title:       "指标文章",
		ContentURL:  "https://mp.weixin.qq.com/s/metric?__biz=biz-metric-sync&mid=900&idx=2&sn=sn-900",
		PublishTime: 100,
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := store.UpsertArticles("biz-metric-sync", []ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}

	run, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-sync"})
	if err != nil {
		t.Fatalf("start metric sync: %v", err)
	}
	completed := waitMetricSyncRun(t, store, run.ID)
	if completed.Status != SyncStatusCompleted || completed.Total != 1 || completed.Attempted != 1 || completed.Stored != 1 || completed.Unknown != 0 || completed.Failed != 0 {
		t.Fatalf("unexpected metric sync result: %+v", completed)
	}

	store.mu.Lock()
	state := store.metricStates[article.Key]
	if len(store.metrics) != 1 {
		store.mu.Unlock()
		t.Fatalf("metric snapshot count = %d, want 1", len(store.metrics))
	}
	snapshot := store.metrics[0]
	store.mu.Unlock()
	if state.Status != MetricStateSuccess || state.AttemptCount != 1 || state.SuccessCount != 1 || state.LastSuccessAt == 0 {
		t.Fatalf("unexpected metric state: %+v", state)
	}
	if snapshot.ViewCount == nil || *snapshot.ViewCount != 1200 || snapshot.LikeCount == nil || *snapshot.LikeCount != 426 ||
		snapshot.CommentCount == nil || *snapshot.CommentCount != 12 || snapshot.ShareCount == nil || *snapshot.ShareCount != 8 ||
		snapshot.CollectCount == nil || *snapshot.CollectCount != 174 || snapshot.RewardCount == nil || *snapshot.RewardCount != 2 {
		t.Fatalf("unexpected metric snapshot: %+v", snapshot)
	}

	second, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-sync"})
	if err != nil {
		t.Fatalf("start second metric sync: %v", err)
	}
	secondCompleted := waitMetricSyncRun(t, store, second.ID)
	if secondCompleted.Status != SyncStatusCompleted || secondCompleted.Total != 0 || secondCompleted.Attempted != 0 {
		t.Fatalf("successful article was not skipped: %+v", secondCompleted)
	}
}

func TestMetricSyncRecollectsSuccessCapturedByLegacyNetworkPath(t *testing.T) {
	var requestsMu sync.Mutex
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requestCount++
		requestsMu.Unlock()
		if r.Method != http.MethodPost {
			t.Fatalf("metric request method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":88,"like_num":9,"comment_num":2}}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-metric-legacy", Key: "key-legacy"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-metric-legacy", ArticleItem{
		ContentURL:  "https://mp.weixin.qq.com/s/legacy?mid=902&idx=1",
		PublishTime: 100,
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := store.UpsertArticles("biz-metric-legacy", []ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	store.mu.Lock()
	store.metricStates[article.Key] = ArticleMetricState{
		ArticleKey: article.Key, Status: MetricStateSuccess, AttemptCount: 1,
		SuccessCount: 1, LastSource: "network", LastObservedAt: 90,
	}
	store.mu.Unlock()

	run, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-legacy"})
	if err != nil {
		t.Fatalf("start metric sync: %v", err)
	}
	completed := waitMetricSyncRun(t, store, run.ID)
	if completed.Status != SyncStatusCompleted || completed.Total != 1 || completed.Attempted != 1 || completed.Stored != 1 {
		t.Fatalf("legacy network metric was not recollected: %+v", completed)
	}
	requestsMu.Lock()
	gotRequests := requestCount
	requestsMu.Unlock()
	if gotRequests != 1 {
		t.Fatalf("legacy network metric request count = %d, want 1", gotRequests)
	}
	store.mu.Lock()
	state := store.metricStates[article.Key]
	store.mu.Unlock()
	if state.LastSource != "getappmsgext" || state.Status != MetricStateSuccess {
		t.Fatalf("legacy metric state was not upgraded: %+v", state)
	}
}

func TestMetricSyncPreservesHistoricalMetricFieldsWhenUpstreamOmitsThem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/s/history":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<script>window.cgiDataNew={comment_id:"comment-history",appmsgid:"991",idx:1};</script>`))
		case "/mp/appmsg_comment":
			_, _ = w.Write([]byte(`{"ret":0,"elected_comment_total_cnt":17}`))
		default:
			_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":1200,"like_num":426,"share_num":8}}`))
		}
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-metric-history", Key: "key-metric-history"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-metric-history", ArticleItem{
		ContentURL: "https://mp.weixin.qq.com/s/history?mid=991&idx=1",
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := store.UpsertArticles("biz-metric-history", []ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	oldView, oldComment, oldCollect := int64(900), int64(17), int64(6)
	store.mu.Lock()
	store.metricStates[article.Key] = ArticleMetricState{
		ArticleKey: article.Key, Status: MetricStateSuccess, AttemptCount: 1,
		SuccessCount: 1, LastSource: "network", LastObservedAt: 90,
	}
	store.metrics = append(store.metrics, ArticleMetricSnapshot{
		ArticleKey: article.Key, ObservedAt: 90, Source: "network",
		ViewCount: &oldView, CommentCount: &oldComment, CollectCount: &oldCollect,
	})
	store.mu.Unlock()

	run, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-history"})
	if err != nil {
		t.Fatalf("start metric sync: %v", err)
	}
	completed := waitMetricSyncRun(t, store, run.ID)
	if completed.Status != SyncStatusCompleted || completed.Stored != 1 {
		t.Fatalf("unexpected historical metric run: %+v", completed)
	}
	store.mu.Lock()
	snapshot := store.metrics[len(store.metrics)-1]
	store.mu.Unlock()
	if snapshot.ViewCount == nil || *snapshot.ViewCount != 1200 || snapshot.LikeCount == nil || *snapshot.LikeCount != 426 ||
		snapshot.CommentCount == nil || *snapshot.CommentCount != 17 || snapshot.CollectCount == nil || *snapshot.CollectCount != 6 ||
		snapshot.ShareCount == nil || *snapshot.ShareCount != 8 {
		t.Fatalf("historical non-empty metric fields were not preserved: %+v", snapshot)
	}
}

func TestMetricSyncMarksMissingUpstreamMetricsAsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{}}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-metric-unknown", Key: "key-unknown"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-metric-unknown", ArticleItem{ContentURL: "https://mp.weixin.qq.com/s/unknown?mid=901&idx=1"}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := store.UpsertArticles("biz-metric-unknown", []ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	run, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-unknown"})
	if err != nil {
		t.Fatalf("start metric sync: %v", err)
	}
	completed := waitMetricSyncRun(t, store, run.ID)
	if completed.Status != SyncStatusPartial || completed.Total != 1 || completed.Attempted != 1 || completed.Unknown != 1 {
		t.Fatalf("unexpected unknown metric run: %+v", completed)
	}
	store.mu.Lock()
	state := store.metricStates[article.Key]
	snapshotCount := len(store.metrics)
	store.mu.Unlock()
	if state.Status != MetricStateUnknown || state.UnknownCount != 1 || snapshotCount != 0 {
		t.Fatalf("missing metrics should be unknown without empty snapshot: state=%+v snapshots=%d", state, snapshotCount)
	}
}

func TestMetricSyncForceResumeUsesPersistedCursor(t *testing.T) {
	var requestsMu sync.Mutex
	var mids []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse resumed metric form: %v", err)
		}
		requestsMu.Lock()
		mids = append(mids, r.PostForm.Get("mid"))
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":100,"old_like_num":10,"comment_num":3}}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-metric-resume", Key: "key-metric-resume"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	for _, item := range []ArticleItem{
		{Title: "第一篇", ContentURL: "https://mp.weixin.qq.com/s/metric-one?mid=1&idx=1", PublishTime: 200},
		{Title: "第二篇", ContentURL: "https://mp.weixin.qq.com/s/metric-two?mid=2&idx=1", PublishTime: 100},
	} {
		article, ok := ArticleRecordFromItem("biz-metric-resume", item, time.Unix(item.PublishTime, 0))
		if !ok {
			t.Fatalf("article identity was not built for %+v", item)
		}
		if _, err := store.UpsertArticles("biz-metric-resume", []ArticleRecord{article}, time.Unix(item.PublishTime, 0)); err != nil {
			t.Fatalf("insert article: %v", err)
		}
	}

	run, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-resume", Force: true})
	if err != nil {
		t.Fatalf("start metric sync: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		requestsMu.Lock()
		requestCount := len(mids)
		requestsMu.Unlock()
		stored, _ := store.GetMetricSyncRun(run.ID)
		if requestCount == 1 && stored != nil && stored.AfterArticleKey != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	requestsMu.Lock()
	firstCount := len(mids)
	requestsMu.Unlock()
	if firstCount != 1 {
		t.Fatalf("first metric run requests = %d, want 1 before cancellation", firstCount)
	}
	service.syncMu.Lock()
	cancel := service.metricSyncCancels[run.ID]
	service.syncMu.Unlock()
	if cancel == nil {
		t.Fatal("metric sync cancellation handle was not registered")
	}
	cancel()
	cancelled := waitMetricSyncRun(t, store, run.ID)
	if cancelled.Status != SyncStatusCancelled || cancelled.AfterArticleKey == "" {
		t.Fatalf("cancelled metric run did not retain cursor: %+v", cancelled)
	}

	resumed, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-metric-resume", Force: true, Resume: true})
	if err != nil {
		t.Fatalf("resume metric sync: %v", err)
	}
	if resumed.ID != run.ID {
		t.Fatalf("resumed metric run ID = %q, want %q", resumed.ID, run.ID)
	}
	completed := waitMetricSyncRun(t, store, resumed.ID)
	if completed.Status != SyncStatusCompleted || completed.Attempted != 2 || completed.Stored != 2 {
		t.Fatalf("unexpected resumed metric run: %+v", completed)
	}
	requestsMu.Lock()
	gotMids := append([]string(nil), mids...)
	requestsMu.Unlock()
	if strings.Join(gotMids, ",") != "1,2" {
		t.Fatalf("force resume replayed wrong metric articles: %v", gotMids)
	}
}

func waitMetricSyncRun(t *testing.T, store *syncCatalogStub, id string) *MetricSyncRun {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := store.GetMetricSyncRun(id)
		if run != nil && metricSyncTerminal(run.Status) {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, _ := store.GetMetricSyncRun(id)
	if run == nil {
		t.Fatalf("metric sync run %s was not persisted", id)
	}
	t.Fatalf("metric sync run %s did not finish: %s", id, mustJSON(run))
	return nil
}

func mustJSON(value interface{}) string {
	data, _ := json.Marshal(value)
	return strings.TrimSpace(string(data))
}
