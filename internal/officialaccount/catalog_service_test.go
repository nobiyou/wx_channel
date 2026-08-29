package officialaccount

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncCatalogStub struct {
	mu              sync.Mutex
	accounts        map[string]Account
	articles        map[string]ArticleRecord
	runs            map[string]SyncRun
	latest          map[string]string
	metrics         []ArticleMetricSnapshot
	metricStates    map[string]ArticleMetricState
	metricRuns      map[string]MetricSyncRun
	latestMetric    string
	updateRunErrors int
	updateRunCalls  int
}

func newSyncCatalogStub() *syncCatalogStub {
	return &syncCatalogStub{
		accounts:     make(map[string]Account),
		articles:     make(map[string]ArticleRecord),
		runs:         make(map[string]SyncRun),
		latest:       make(map[string]string),
		metricStates: make(map[string]ArticleMetricState),
		metricRuns:   make(map[string]MetricSyncRun),
	}
}

func (s *syncCatalogStub) UpsertAccount(account Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[account.Biz] = account
	return nil
}

func (s *syncCatalogStub) ListAccounts(string, int, int) (AccountPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]AccountSummary, 0, len(s.accounts))
	for _, account := range s.accounts {
		items = append(items, AccountSummary{Biz: account.Biz, Nickname: account.Nickname})
	}
	return AccountPage{Items: items, Total: int64(len(items)), Page: 1, PageSize: 30}, nil
}

func (s *syncCatalogStub) UpsertArticles(biz string, articles []ArticleRecord, _ time.Time) (ArticleUpsertStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := ArticleUpsertStats{}
	for _, article := range articles {
		if article.Biz == "" {
			article.Biz = biz
		}
		if _, exists := s.articles[article.Key]; exists {
			stats.Updated++
		} else {
			stats.Inserted++
		}
		s.articles[article.Key] = article
		stats.Seen++
	}
	return stats, nil
}

func (s *syncCatalogStub) ListArticles(query ArticleQuery) (ArticlePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ArticleRecord, 0, len(s.articles))
	for _, article := range s.articles {
		if query.Biz != "" && article.Biz != query.Biz {
			continue
		}
		items = append(items, article)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return ArticlePage{Items: items, Total: int64(len(items)), Page: 1, PageSize: len(items)}, nil
}

func (s *syncCatalogStub) GetArticle(key string) (*ArticleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	article, ok := s.articles[key]
	if !ok {
		return nil, nil
	}
	return &article, nil
}

func (s *syncCatalogStub) RecordArticleMetrics(metrics []ArticleMetricSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, metric := range metrics {
		s.metrics = append(s.metrics, metric)
	}
	return nil
}

func (s *syncCatalogStub) LatestArticleMetrics(keys []string) (map[string]ArticleMetricSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]ArticleMetricSnapshot, len(keys))
	for _, key := range keys {
		for index := len(s.metrics) - 1; index >= 0; index-- {
			metric := s.metrics[index]
			if metric.ArticleKey != key {
				continue
			}
			result[key] = metric
			break
		}
	}
	return result, nil
}

func (s *syncCatalogStub) ListArticleMetricStates(keys []string) (map[string]ArticleMetricState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]ArticleMetricState, len(keys))
	for _, key := range keys {
		if state, ok := s.metricStates[key]; ok {
			result[key] = state
		}
	}
	return result, nil
}

func (s *syncCatalogStub) ListArticleMetricCandidates(biz string, force bool, now time.Time, afterPublishTime int64, afterArticleKey string, limit int) (ArticleMetricCandidatePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ArticleRecord, 0, len(s.articles))
	for _, article := range s.articles {
		if article.Biz != biz {
			continue
		}
		state, hasState := s.metricStates[article.Key]
		hasComment := false
		for _, metric := range s.metrics {
			if metric.ArticleKey == article.Key && metric.CommentCount != nil {
				hasComment = true
				break
			}
		}
		commentRetryReady := state.NextRetryAt == 0 || state.NextRetryAt <= now.Unix()
		successNeedsRetry := state.Status == MetricStateSuccess &&
			(state.LastSource != "getappmsgext" || state.LastError != "" || !hasComment) && commentRetryReady
		if !force && hasState && ((!successNeedsRetry && state.Status == MetricStateSuccess) ||
			state.Status == MetricStateUnknown || (state.Status == MetricStateFailed && state.NextRetryAt > now.Unix())) {
			continue
		}
		if afterArticleKey != "" && (article.PublishTime > afterPublishTime || (article.PublishTime == afterPublishTime && article.Key <= afterArticleKey)) {
			continue
		}
		items = append(items, article)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PublishTime != items[j].PublishTime {
			return items[i].PublishTime > items[j].PublishTime
		}
		return items[i].Key < items[j].Key
	})
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	page := ArticleMetricCandidatePage{Items: append([]ArticleRecord(nil), items[:limit]...), HasMore: limit < len(items)}
	if limit > 0 {
		last := page.Items[len(page.Items)-1]
		page.NextPublishTime = last.PublishTime
		page.NextArticleKey = last.Key
	}
	return page, nil
}

func (s *syncCatalogStub) CountArticleMetricCandidates(biz string, force bool, now time.Time) (int, error) {
	page, err := s.ListArticleMetricCandidates(biz, force, now, 0, "", 100000)
	return len(page.Items), err
}

func (s *syncCatalogStub) SaveArticleMetricResult(state ArticleMetricState, snapshot *ArticleMetricSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metricStates[state.ArticleKey] = state
	if snapshot != nil {
		s.metrics = append(s.metrics, *snapshot)
	}
	return nil
}

func (s *syncCatalogStub) UpsertArticleAssets(_ string, _ []ArticleAsset) error { return nil }

func (s *syncCatalogStub) ListArticleAssets(keys []string) (map[string][]ArticleAsset, error) {
	return make(map[string][]ArticleAsset, len(keys)), nil
}

func (s *syncCatalogStub) UpdateArticleArchive(ArticleArchiveState) error { return nil }

func (s *syncCatalogStub) CreateSyncRun(run SyncRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	s.latest[run.Biz+"\x00"+run.Mode] = run.ID
	return nil
}

func (s *syncCatalogStub) UpdateSyncRun(run SyncRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateRunCalls++
	if s.updateRunErrors > 0 {
		s.updateRunErrors--
		return errors.New("injected sync progress persistence failure")
	}
	s.runs[run.ID] = run
	s.latest[run.Biz+"\x00"+run.Mode] = run.ID
	return nil
}

func (s *syncCatalogStub) GetSyncRun(id string) (*SyncRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return nil, nil
	}
	return &run, nil
}

func (s *syncCatalogStub) GetLatestSyncRun(biz, mode string) (*SyncRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.latest[biz+"\x00"+mode]
	if id == "" {
		return nil, nil
	}
	run := s.runs[id]
	return &run, nil
}

func (s *syncCatalogStub) CreateMetricSyncRun(run MetricSyncRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metricRuns[run.ID] = run
	s.latestMetric = run.ID
	return nil
}

func (s *syncCatalogStub) UpdateMetricSyncRun(run MetricSyncRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metricRuns[run.ID] = run
	s.latestMetric = run.ID
	return nil
}

func (s *syncCatalogStub) GetMetricSyncRun(id string) (*MetricSyncRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.metricRuns[id]
	if !ok {
		return nil, nil
	}
	return &run, nil
}

func (s *syncCatalogStub) GetLatestMetricSyncRun(biz string) (*MetricSyncRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latestMetric == "" {
		return nil, nil
	}
	run, ok := s.metricRuns[s.latestMetric]
	if !ok || run.Biz != biz {
		return nil, nil
	}
	return &run, nil
}

func TestStartSyncPersistsAllAccessiblePages(t *testing.T) {
	var requestsMu sync.Mutex
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		offsets = append(offsets, r.URL.Query().Get("offset"))
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		page := map[string]interface{}{
			"ret":              0,
			"can_msg_continue": 0,
			"next_offset":      10,
			"general_msg_list": `{"list":[{"app_msg_ext_info":{"title":"第二页","digest":"摘要","content_url":"https://mp.weixin.qq.com/s/page-2?__biz=biz-sync&mid=2&idx=1"},"comm_msg_info":{"datetime":200}}]}`,
		}
		if r.URL.Query().Get("offset") == "0" {
			page["can_msg_continue"] = 1
			page["next_offset"] = 10
			page["general_msg_list"] = `{"list":[{"app_msg_ext_info":{"title":"第一页","content_url":"https://mp.weixin.qq.com/s/page-1?__biz=biz-sync&mid=1&idx=1"},"comm_msg_info":{"datetime":100}}]}`
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-sync", Key: "sync-key", Uin: "sync-uin", PassTicket: "sync-ticket"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}

	run, err := service.StartSync(SyncRequest{Biz: "biz-sync", Mode: SyncModeHistory})
	if err != nil {
		t.Fatalf("start sync: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var completed *SyncRun
	for time.Now().Before(deadline) {
		completed, _ = store.GetSyncRun(run.ID)
		if completed != nil && completed.Status == SyncStatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed == nil || completed.Status != SyncStatusCompleted {
		t.Fatalf("sync did not complete: %+v", completed)
	}
	if completed.PageCount != 2 || completed.Fetched != 2 || completed.Inserted != 2 || completed.CanContinue {
		t.Fatalf("unexpected completed run: %+v", completed)
	}
	if len(store.articles) != 2 {
		t.Fatalf("article count = %d, want 2", len(store.articles))
	}
	requestsMu.Lock()
	gotOffsets := append([]string(nil), offsets...)
	requestsMu.Unlock()
	if strings.Join(gotOffsets, ",") != "0,10" {
		t.Fatalf("upstream offsets = %v, want [0 10]", gotOffsets)
	}
}

func TestStartSyncRejectsConcurrentRunForSameAccount(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closeOnce(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"can_msg_continue":0,"next_offset":0,"general_msg_list":"{\"list\":[]}"}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-concurrent", Key: "key"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	if _, err := service.StartSync(SyncRequest{Biz: "biz-concurrent"}); err != nil {
		t.Fatalf("start first sync: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first sync did not reach upstream")
	}
	if _, err := service.StartSync(SyncRequest{Biz: "biz-concurrent"}); !errors.Is(err, ErrSyncAlreadyRunning) {
		t.Fatalf("second sync error = %v, want ErrSyncAlreadyRunning", err)
	}
	close(release)
}

func TestSyncStatusCanReadLatestRunAndCancelActiveRequest(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closeOnce(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-cancel", Key: "key-cancel"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	run, err := service.StartSync(SyncRequest{Biz: "biz-cancel", Mode: SyncModeHistory})
	if err != nil {
		t.Fatalf("start sync: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("sync did not reach upstream")
	}

	latestRecorder := httptest.NewRecorder()
	latestRequest := httptest.NewRequest(http.MethodGet, "/api/mp/sync?biz=biz-cancel&mode=history", nil)
	service.HandleStartSync(latestRecorder, latestRequest)
	if latestRecorder.Code != http.StatusOK || !strings.Contains(latestRecorder.Body.String(), run.ID) {
		t.Fatalf("latest sync status response = %d: %s", latestRecorder.Code, latestRecorder.Body.String())
	}

	cancelRecorder := httptest.NewRecorder()
	cancelRequest := httptest.NewRequest(http.MethodDelete, "/api/mp/sync/"+run.ID, nil)
	service.HandleSyncStatus(cancelRecorder, cancelRequest)
	if cancelRecorder.Code != http.StatusAccepted {
		t.Fatalf("cancel response = %d: %s", cancelRecorder.Code, cancelRecorder.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	var cancelled *SyncRun
	for time.Now().Before(deadline) {
		cancelled, _ = store.GetSyncRun(run.ID)
		if cancelled != nil && cancelled.Status == SyncStatusCancelled {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if cancelled == nil || cancelled.Status != SyncStatusCancelled {
		t.Fatalf("sync was not marked cancelled: %+v", cancelled)
	}
}

func TestStartSyncResumesCancelledRunFromSavedOffset(t *testing.T) {
	var offsets []string
	var offsetsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsetsMu.Lock()
		offsets = append(offsets, r.URL.Query().Get("offset"))
		offsetsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"can_msg_continue":0,"next_offset":30,"general_msg_list":"{\"list\":[{\"app_msg_ext_info\":{\"title\":\"续跑文章\",\"content_url\":\"https://mp.weixin.qq.com/s/resumed?mid=30&idx=1\"},\"comm_msg_info\":{\"datetime\":300}}]}"}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-resume", Key: "key-resume"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	store.runs["old-run"] = SyncRun{ID: "old-run", Biz: "biz-resume", Mode: SyncModeHistory, Status: SyncStatusCancelled, NextOffset: 30, CanContinue: true}
	store.latest["biz-resume\x00history"] = "old-run"
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	run, err := service.StartSync(SyncRequest{Biz: "biz-resume", Mode: SyncModeHistory, Resume: true})
	if err != nil {
		t.Fatalf("resume sync: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var completed *SyncRun
	for time.Now().Before(deadline) {
		completed, _ = store.GetSyncRun(run.ID)
		if completed != nil && completed.Status == SyncStatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed == nil || completed.Status != SyncStatusCompleted {
		t.Fatalf("resumed sync did not complete: %+v", completed)
	}
	offsetsMu.Lock()
	gotOffsets := strings.Join(offsets, ",")
	offsetsMu.Unlock()
	if gotOffsets != "30" {
		t.Fatalf("resumed sync started at wrong offset: %s", gotOffsets)
	}
}

func TestStartSyncReusesInterruptedRunIDAndSavedOffset(t *testing.T) {
	var offsets []string
	var offsetsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsetsMu.Lock()
		offsets = append(offsets, r.URL.Query().Get("offset"))
		offsetsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"can_msg_continue":0,"next_offset":40,"general_msg_list":"{\"list\":[{\"app_msg_ext_info\":{\"title\":\"恢复文章\",\"content_url\":\"https://mp.weixin.qq.com/s/recovered?mid=40&idx=1\"},\"comm_msg_info\":{\"datetime\":400}}]}"}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-reuse", Key: "key-reuse"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	store.runs["interrupted-run"] = SyncRun{ID: "interrupted-run", Biz: "biz-reuse", Mode: SyncModeHistory, Status: SyncStatusRunning, NextOffset: 40, PageSize: 10, CanContinue: true, StartedAt: time.Now().Add(-time.Hour).Unix()}
	store.latest["biz-reuse\x00history"] = "interrupted-run"

	run, err := service.StartSync(SyncRequest{Biz: "biz-reuse", Mode: SyncModeHistory, Resume: true})
	if err != nil {
		t.Fatalf("start resumed sync: %v", err)
	}
	if run.ID != "interrupted-run" {
		t.Fatalf("resumed run ID = %q, want interrupted-run", run.ID)
	}
	deadline := time.Now().Add(2 * time.Second)
	var completed *SyncRun
	for time.Now().Before(deadline) {
		completed, _ = store.GetSyncRun(run.ID)
		if completed != nil && completed.Status == SyncStatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed == nil || completed.Status != SyncStatusCompleted {
		t.Fatalf("resumed sync did not complete: %+v", completed)
	}
	offsetsMu.Lock()
	gotOffsets := strings.Join(offsets, ",")
	offsetsMu.Unlock()
	if gotOffsets != "40" {
		t.Fatalf("resumed sync started at wrong offset: %s", gotOffsets)
	}
}

func TestSetCatalogRepositoryAutomaticallyResumesQueuedRun(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closeOnce(started)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"can_msg_continue":0,"next_offset":10,"general_msg_list":"{\"list\":[]}"}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-auto-resume", Key: "key-auto-resume"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	store.runs["queued-run"] = SyncRun{ID: "queued-run", Biz: "biz-auto-resume", Mode: SyncModeHistory, Status: SyncStatusQueued, NextOffset: 20, PageSize: 10, CanContinue: true}
	store.latest["biz-auto-resume\x00history"] = "queued-run"
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("queued run was not resumed automatically")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := store.GetSyncRun("queued-run")
		if run != nil && run.Status == SyncStatusCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, _ := store.GetSyncRun("queued-run")
	t.Fatalf("automatic resumed run did not complete: %+v", run)
}

func TestStartSyncStopsAfterPageProgressPersistenceFailure(t *testing.T) {
	var requestsMu sync.Mutex
	requests := 0
	store := newSyncCatalogStub()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests++
		requestsMu.Unlock()
		store.mu.Lock()
		store.updateRunErrors = 1
		store.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"can_msg_continue":1,"next_offset":10,"general_msg_list":"{\"list\":[{\"app_msg_ext_info\":{\"title\":\"第一页\",\"content_url\":\"https://mp.weixin.qq.com/s/progress?mid=1&idx=1\"},\"comm_msg_info\":{\"datetime\":100}}]}"}`))
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-progress-failure", Key: "key-progress-failure"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	run, err := service.StartSync(SyncRequest{Biz: "biz-progress-failure", Mode: SyncModeHistory})
	if err != nil {
		t.Fatalf("start sync: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var partial *SyncRun
	for time.Now().Before(deadline) {
		partial, _ = store.GetSyncRun(run.ID)
		if partial != nil && partial.Status == SyncStatusPartial {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if partial == nil || partial.Status != SyncStatusPartial || !partial.CanContinue {
		t.Fatalf("progress failure did not leave resumable partial state: %+v", partial)
	}
	requestsMu.Lock()
	gotRequests := requests
	requestsMu.Unlock()
	if gotRequests != 1 {
		t.Fatalf("upstream requests after progress failure = %d, want 1", gotRequests)
	}
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

var _ CatalogRepository = (*syncCatalogStub)(nil)
