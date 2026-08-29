package officialaccount

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestExtractArticleCommentIdentityFromArticleHTML(t *testing.T) {
	identity, ok := extractArticleCommentIdentity(`
		<html><script>
			window.cgiDataNew = { comment_id: "comment-987", appmsgid: "2650828073", idx: 2 };
		</script></html>`, ArticleRecord{Mid: "fallback-mid", Idx: 1})
	if !ok {
		t.Fatal("expected article comment identity")
	}
	if identity.AppmsgID != "2650828073" || identity.Idx != "2" || identity.CommentID != "comment-987" {
		t.Fatalf("unexpected article comment identity: %+v", identity)
	}
}

func TestMetricSyncFetchesCommentTotalWhenGetAppmsgextOmitsIt(t *testing.T) {
	var requestsMu sync.Mutex
	paths := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		paths[r.URL.Path]++
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mp/getappmsgext":
			_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":1200,"like_num":426,"share_num":8}}`))
		case "/s/article":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<script>window.cgiDataNew={comment_id:"comment-900",appmsgid:"900",idx:1};</script>`))
		case "/mp/appmsg_comment":
			if got := r.URL.Query().Get("appmsgid"); got != "900" {
				t.Errorf("comment appmsgid = %q, want 900", got)
			}
			if got := r.URL.Query().Get("idx"); got != "1" {
				t.Errorf("comment idx = %q, want 1", got)
			}
			if got := r.URL.Query().Get("comment_id"); got != "comment-900" {
				t.Errorf("comment comment_id = %q, want comment-900", got)
			}
			_, _ = w.Write([]byte(`{"ret":0,"elected_comment_total_cnt":37}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{
		Biz: "biz-comment-sync", Key: "key-comment-sync", Uin: "uin-comment-sync",
		PassTicket: "ticket-comment-sync", AppmsgToken: "token-comment-sync",
		Cookie: "slave_user=comment-sync",
	}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-comment-sync", ArticleItem{
		ContentURL:  "https://mp.weixin.qq.com/s/article?__biz=biz-comment-sync&mid=900&idx=1&sn=sn-900",
		PublishTime: 100,
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := store.UpsertArticles("biz-comment-sync", []ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}

	run, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-comment-sync"})
	if err != nil {
		t.Fatalf("start metric sync: %v", err)
	}
	completed := waitMetricSyncRun(t, store, run.ID)
	if completed.Status != SyncStatusCompleted || completed.Total != 1 || completed.Attempted != 1 || completed.Stored != 1 {
		t.Fatalf("unexpected comment metric sync result: %+v", completed)
	}
	store.mu.Lock()
	state := store.metricStates[article.Key]
	snapshot := store.metrics[len(store.metrics)-1]
	store.mu.Unlock()
	if state.Status != MetricStateSuccess || state.LastError != "" {
		t.Fatalf("comment metric state was not complete: %+v", state)
	}
	if snapshot.CommentCount == nil || *snapshot.CommentCount != 37 {
		t.Fatalf("comment total was not persisted: %+v", snapshot)
	}
	requestsMu.Lock()
	gotPaths := map[string]int{"metrics": paths["/mp/getappmsgext"], "article": paths["/s/article"], "comments": paths["/mp/appmsg_comment"]}
	requestsMu.Unlock()
	if gotPaths["metrics"] != 1 || gotPaths["article"] != 1 || gotPaths["comments"] != 1 {
		t.Fatalf("unexpected comment collection requests: %+v", gotPaths)
	}
}

func TestMetricSyncRecollectsSuccessWhenCommentSnapshotIsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mp/getappmsgext":
			_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":1200,"like_num":426}}`))
		case "/s/article":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<script>window.cgiDataNew={comment_id:"comment-retry",appmsgid:"901",idx:1};</script>`))
		case "/mp/appmsg_comment":
			_, _ = w.Write([]byte(`{"ret":0,"elected_comment_total_cnt":11}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-comment-retry", Key: "key-comment-retry"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-comment-retry", ArticleItem{
		ContentURL:  "https://mp.weixin.qq.com/s/article?mid=901&idx=1",
		PublishTime: 100,
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := store.UpsertArticles("biz-comment-retry", []ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	view, like := int64(100), int64(3)
	store.mu.Lock()
	store.metricStates[article.Key] = ArticleMetricState{
		ArticleKey: article.Key, Status: MetricStateSuccess, AttemptCount: 1,
		SuccessCount: 1, LastSource: "getappmsgext", LastObservedAt: 90,
	}
	store.metrics = append(store.metrics, ArticleMetricSnapshot{
		ArticleKey: article.Key, ObservedAt: 90, Source: "getappmsgext",
		ViewCount: &view, LikeCount: &like,
	})
	store.mu.Unlock()

	run, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-comment-retry"})
	if err != nil {
		t.Fatalf("start metric sync: %v", err)
	}
	completed := waitMetricSyncRun(t, store, run.ID)
	if completed.Status != SyncStatusCompleted || completed.Total != 1 || completed.Attempted != 1 {
		t.Fatalf("incomplete success state was not retried: %+v", completed)
	}
	store.mu.Lock()
	state := store.metricStates[article.Key]
	snapshot := store.metrics[len(store.metrics)-1]
	store.mu.Unlock()
	if state.Status != MetricStateSuccess || snapshot.CommentCount == nil || *snapshot.CommentCount != 11 {
		t.Fatalf("retried comment metric was not persisted: state=%+v snapshot=%+v", state, snapshot)
	}
}

func TestMetricSyncRetainsCommentRetryAfterFallbackFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mp/getappmsgext" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":1200,"like_num":426}}`))
			return
		}
		http.Error(w, "comment page unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	service := NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(Account{Biz: "biz-comment-failure", Key: "key-comment-failure"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	store := newSyncCatalogStub()
	if err := service.SetCatalogRepository(store); err != nil {
		t.Fatalf("set catalog repository: %v", err)
	}
	article, ok := ArticleRecordFromItem("biz-comment-failure", ArticleItem{
		ContentURL:  "https://mp.weixin.qq.com/s/article?mid=902&idx=1",
		PublishTime: 100,
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := store.UpsertArticles("biz-comment-failure", []ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}

	run, err := service.StartMetricSync(MetricSyncRequest{Biz: "biz-comment-failure"})
	if err != nil {
		t.Fatalf("start metric sync: %v", err)
	}
	completed := waitMetricSyncRun(t, store, run.ID)
	if completed.Status != SyncStatusPartial || completed.Stored != 1 || completed.Failed != 1 {
		t.Fatalf("comment fallback failure was hidden from sync run: %+v", completed)
	}

	store.mu.Lock()
	state := store.metricStates[article.Key]
	snapshot := store.metrics[len(store.metrics)-1]
	store.mu.Unlock()
	if state.Status != MetricStateSuccess || state.LastError == "" || state.NextRetryAt <= time.Now().Unix() {
		t.Fatalf("comment retry state was not retained: %+v", state)
	}
	if snapshot.ViewCount == nil || *snapshot.ViewCount != 1200 || snapshot.CommentCount != nil {
		t.Fatalf("partial metric snapshot was not preserved correctly: %+v", snapshot)
	}
}
