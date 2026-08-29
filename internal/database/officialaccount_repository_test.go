package database

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"wx_channel/internal/officialaccount"
)

func TestOfficialAccountRepositoryPersistsCatalogAndSyncState(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	repository := NewOfficialAccountRepository()
	if err := repository.UpsertAccount(officialaccount.Account{
		Biz:         "biz-repository",
		Nickname:    "仓储账号",
		AvatarURL:   "https://example.test/avatar.jpg",
		AuthorID:    "author-repository",
		Key:         "secret-must-not-be-stored-here",
		IsEffective: true,
		CreatedAt:   100,
		UpdateTime:  200,
	}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	accountPage, err := repository.ListAccounts("仓储", 1, 10)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if accountPage.Total != 1 || len(accountPage.Items) != 1 || accountPage.Items[0].Biz != "biz-repository" {
		t.Fatalf("unexpected account page: %+v", accountPage)
	}
	var leakedKey int
	if err := repository.db.QueryRow("SELECT COUNT(*) FROM mp_accounts WHERE nickname = ?", "secret-must-not-be-stored-here").Scan(&leakedKey); err != nil {
		t.Fatalf("check credential separation: %v", err)
	}
	if leakedKey != 0 {
		t.Fatalf("credential was written into account metadata: %d", leakedKey)
	}

	seenAt := time.Unix(300, 0)
	firstItem := officialaccount.ArticleItem{
		Title:         "第一篇",
		Digest:        "摘要",
		Author:        "作者",
		ContentURL:    "https://mp.weixin.qq.com/s/first?__biz=biz-repository&mid=100&idx=1&sessionid=one",
		PublishTime:   250,
		VideoID:       "video-repository",
		Subtype:       7,
		CopyrightStat: 1,
		Duration:      42,
		AudioFileID:   23,
		PlayURL:       "https://vd.example.test/video.mp4?token=short-lived",
	}
	secondItem := officialaccount.ArticleItem{
		Title:       "第二篇",
		ContentURL:  "https://mp.weixin.qq.com/s/second?__biz=biz-repository&mid=101&idx=1",
		PublishTime: 200,
	}
	first, ok := officialaccount.ArticleRecordFromItem("biz-repository", firstItem, seenAt)
	if !ok {
		t.Fatal("first article identity was not built")
	}
	second, ok := officialaccount.ArticleRecordFromItem("biz-repository", secondItem, seenAt)
	if !ok {
		t.Fatal("second article identity was not built")
	}
	stats, err := repository.UpsertArticles("biz-repository", []officialaccount.ArticleRecord{first, second}, seenAt)
	if err != nil {
		t.Fatalf("insert articles: %v", err)
	}
	if stats.Seen != 2 || stats.Inserted != 2 || stats.Updated != 0 {
		t.Fatalf("unexpected insert stats: %+v", stats)
	}

	first.Title = "第一篇修订标题"
	stats, err = repository.UpsertArticles("biz-repository", []officialaccount.ArticleRecord{first}, time.Unix(400, 0))
	if err != nil {
		t.Fatalf("update article: %v", err)
	}
	if stats.Inserted != 0 || stats.Updated != 1 {
		t.Fatalf("unexpected update stats: %+v", stats)
	}

	articlePage, err := repository.ListArticles(officialaccount.ArticleQuery{Biz: "biz-repository", Page: 1, PageSize: 1, Descending: true})
	if err != nil {
		t.Fatalf("list articles: %v", err)
	}
	if articlePage.Total != 2 || len(articlePage.Items) != 1 || articlePage.Items[0].Title != "第一篇修订标题" {
		t.Fatalf("unexpected article page: %+v", articlePage)
	}
	loaded, err := repository.GetArticle(first.Key)
	if err != nil || loaded == nil || loaded.Title != "第一篇修订标题" {
		t.Fatalf("get article = %+v, err = %v", loaded, err)
	}
	if loaded.VideoID != "video-repository" || loaded.Duration != 42 || loaded.AudioFileID != 23 || loaded.PlayURL != "https://vd.example.test/video.mp4" ||
		loaded.Subtype != 7 || loaded.CopyrightStat != 1 {
		t.Fatalf("article media metadata was not persisted: %+v", loaded)
	}

	zero := int64(0)
	if err := repository.RecordArticleMetrics([]officialaccount.ArticleMetricSnapshot{{
		ArticleKey: first.Key,
		ObservedAt: 500,
		Source:     "article_html",
		ViewCount:  &zero,
		LikeCount:  nil,
	}}); err != nil {
		t.Fatalf("record metrics: %v", err)
	}
	var view sql.NullInt64
	var like sql.NullInt64
	if err := repository.db.QueryRow("SELECT view_count, like_count FROM mp_article_metric_snapshots WHERE article_key = ?", first.Key).Scan(&view, &like); err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	if !view.Valid || view.Int64 != 0 || like.Valid {
		t.Fatalf("nullable metric semantics lost: view=%+v like=%+v", view, like)
	}

	assetSize := int64(123)
	if err := repository.UpdateArticleArchive(officialaccount.ArticleArchiveState{
		ArticleKey:   first.Key,
		Status:       officialaccount.ArchiveStatusArchived,
		Directory:    "公众号文章/仓储账号/第一篇",
		HTMLPath:     "公众号文章/仓储账号/第一篇/index.html",
		ManifestPath: "公众号文章/仓储账号/第一篇/article.json",
		ArchivedAt:   600,
		Assets: []officialaccount.ArticleAsset{{
			ArticleKey:  first.Key,
			ResourceKey: "body:html",
			Kind:        officialaccount.ArchiveResourceKindHTML,
			Role:        officialaccount.ArchiveResourceRoleArticleBody,
			LocalPath:   "公众号文章/仓储账号/第一篇/index.html",
			SHA256:      strings.Repeat("b", 64),
			Size:        &assetSize,
			Status:      "downloaded",
		}},
	}); err != nil {
		t.Fatalf("update archive state: %v", err)
	}
	archived, err := repository.GetArticle(first.Key)
	if err != nil || archived == nil || len(archived.Assets) != 1 || archived.Assets[0].SHA256 != strings.Repeat("b", 64) {
		t.Fatalf("archive asset metadata was not persisted: article=%+v err=%v", archived, err)
	}
	if err := repository.UpdateArticleArchive(officialaccount.ArticleArchiveState{
		ArticleKey: first.Key,
		Status:     officialaccount.ArchiveStatusPartial,
		Assets: []officialaccount.ArticleAsset{{
			ArticleKey:  first.Key,
			ResourceKey: "invalid",
			LocalPath:   "../outside",
			Status:      "failed",
		}},
	}); err == nil {
		t.Fatal("unsafe archive asset path should fail")
	}
	var archiveStatus string
	var assetCount int
	if err := repository.db.QueryRow("SELECT archive_status FROM mp_articles WHERE article_key = ?", first.Key).Scan(&archiveStatus); err != nil {
		t.Fatalf("read archive status after rollback: %v", err)
	}
	if err := repository.db.QueryRow("SELECT COUNT(*) FROM mp_article_assets WHERE article_key = ?", first.Key).Scan(&assetCount); err != nil {
		t.Fatalf("read asset count after rollback: %v", err)
	}
	if archiveStatus != officialaccount.ArchiveStatusArchived || assetCount != 1 {
		t.Fatalf("archive update was not atomic: status=%q assets=%d", archiveStatus, assetCount)
	}
	accountPage, err = repository.ListAccounts("", 1, 10)
	if err != nil || accountPage.Items[0].ArticleCount != 2 || accountPage.Items[0].ArchivedCount != 1 {
		t.Fatalf("account counts not refreshed: %+v, err=%v", accountPage, err)
	}

	run := officialaccount.SyncRun{ID: "sync-repository", Biz: "biz-repository", Mode: officialaccount.SyncModeHistory, Status: officialaccount.SyncStatusRunning, Offset: 0, NextOffset: 10, PageSize: 10, PageCount: 1, Fetched: 2, Inserted: 2, CanContinue: true, StartedAt: 700}
	if err := repository.CreateSyncRun(run); err != nil {
		t.Fatalf("create sync run: %v", err)
	}
	run.Status = officialaccount.SyncStatusCompleted
	run.Offset = 10
	run.NextOffset = 10
	run.CanContinue = false
	run.FinishedAt = 800
	if err := repository.UpdateSyncRun(run); err != nil {
		t.Fatalf("update sync run: %v", err)
	}
	loadedRun, err := repository.GetSyncRun(run.ID)
	if err != nil || loadedRun == nil || loadedRun.Status != officialaccount.SyncStatusCompleted || loadedRun.CanContinue {
		t.Fatalf("unexpected sync run: %+v, err=%v", loadedRun, err)
	}
	accountPage, err = repository.ListAccounts("", 1, 10)
	if err != nil || accountPage.Items[0].SyncStatus != officialaccount.SyncStatusCompleted || accountPage.Items[0].LastSyncAt != 800 {
		t.Fatalf("account sync state not refreshed: %+v, err=%v", accountPage, err)
	}

	metricRun := officialaccount.MetricSyncRun{
		ID:               "metric-sync-repository",
		Biz:              "biz-repository",
		Status:           officialaccount.SyncStatusRunning,
		Force:            true,
		Total:            8,
		Attempted:        3,
		Stored:           2,
		AfterPublishTime: 123456,
		AfterArticleKey:  first.Key,
		StartedAt:        900,
	}
	if err := repository.CreateMetricSyncRun(metricRun); err != nil {
		t.Fatalf("create metric sync run: %v", err)
	}
	loadedMetricRun, err := repository.GetMetricSyncRun(metricRun.ID)
	if err != nil || loadedMetricRun == nil || !loadedMetricRun.Force || loadedMetricRun.AfterPublishTime != metricRun.AfterPublishTime || loadedMetricRun.AfterArticleKey != metricRun.AfterArticleKey {
		t.Fatalf("metric sync cursor was not persisted: %+v, err=%v", loadedMetricRun, err)
	}
}

func TestOfficialAccountRepositoryMigrationVersionIncludesCatalog(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	version, err := GetSchemaVersion()
	if err != nil {
		t.Fatalf("get schema version: %v", err)
	}
	if version != 20 {
		t.Fatalf("schema version = %d, want 20", version)
	}
	var videoColumnCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('mp_articles') WHERE name = 'video_id'").Scan(&videoColumnCount); err != nil {
		t.Fatalf("check article video_id column: %v", err)
	}
	if videoColumnCount != 1 {
		t.Fatalf("article video_id column was not created: %d", videoColumnCount)
	}
	for _, table := range []string{"mp_accounts", "mp_articles", "mp_article_assets", "mp_article_metric_snapshots", "mp_article_metric_states", "mp_sync_runs", "mp_metric_sync_runs"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("table %s was not created", table)
		}
	}
	for _, column := range []string{"after_publish_time", "after_article_key"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('mp_metric_sync_runs') WHERE name = ?", column).Scan(&count); err != nil {
			t.Fatalf("check metric sync column %s: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("metric sync column %s was not created", column)
		}
	}
}

func TestOfficialAccountRepositoryBackfillsArticleIdentityOnRead(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	const biz = "biz-legacy-identity"
	repository := NewOfficialAccountRepository()
	article, ok := officialaccount.ArticleRecordFromItem(biz, officialaccount.ArticleItem{
		Title:       "旧文章",
		ContentURL:  "https://mp.weixin.qq.com/s/legacy-identity?mid=903&idx=2",
		PublishTime: 100,
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := repository.UpsertArticles(biz, []officialaccount.ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	if _, err := db.Exec("UPDATE mp_articles SET mid = '', idx = 0 WHERE article_key = ?", article.Key); err != nil {
		t.Fatalf("simulate legacy article row: %v", err)
	}

	loaded, err := repository.GetArticle(article.Key)
	if err != nil {
		t.Fatalf("load legacy article: %v", err)
	}
	if loaded == nil || loaded.Mid != "903" || loaded.Idx != 2 {
		t.Fatalf("legacy article identity was not restored: %+v", loaded)
	}
}

func TestOfficialAccountRepositoryRequeuesNonCanonicalMetricSource(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	const biz = "biz-legacy-metrics"
	repository := NewOfficialAccountRepository()
	article, ok := officialaccount.ArticleRecordFromItem(biz, officialaccount.ArticleItem{
		ContentURL:  "https://mp.weixin.qq.com/s/legacy-metrics?mid=904&idx=1",
		PublishTime: 100,
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := repository.UpsertArticles(biz, []officialaccount.ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	if err := repository.SaveArticleMetricResult(officialaccount.ArticleMetricState{
		ArticleKey: article.Key, Status: officialaccount.MetricStateSuccess,
		AttemptCount: 1, SuccessCount: 1, LastSource: "network", UpdatedAt: 100,
	}, nil); err != nil {
		t.Fatalf("save legacy metric state: %v", err)
	}

	page, err := repository.ListArticleMetricCandidates(biz, false, time.Unix(200, 0), 0, "", 10)
	if err != nil {
		t.Fatalf("list legacy metric candidates: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Key != article.Key {
		t.Fatalf("legacy metric state was not requeued: %+v", page)
	}

	if err := repository.SaveArticleMetricResult(officialaccount.ArticleMetricState{
		ArticleKey: article.Key, Status: officialaccount.MetricStateSuccess,
		AttemptCount: 2, SuccessCount: 2, LastSource: "getappmsgext", UpdatedAt: 200,
	}, &officialaccount.ArticleMetricSnapshot{
		ArticleKey: article.Key, ObservedAt: 200, Source: "getappmsgext",
		CommentCount: func() *int64 { value := int64(4); return &value }(),
	}); err != nil {
		t.Fatalf("save canonical metric state: %v", err)
	}
	count, err := repository.CountArticleMetricCandidates(biz, false, time.Unix(300, 0))
	if err != nil {
		t.Fatalf("count canonical metric candidates: %v", err)
	}
	if count != 0 {
		t.Fatalf("canonical getappmsgext metric was unexpectedly requeued: %d", count)
	}
}

func TestOfficialAccountRepositoryLatestMetricsKeepsLastKnownFields(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	const biz = "biz-metric-last-known"
	repository := NewOfficialAccountRepository()
	article, ok := officialaccount.ArticleRecordFromItem(biz, officialaccount.ArticleItem{
		ContentURL: "https://mp.weixin.qq.com/s/last-known?mid=905&idx=1",
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("article identity was not built")
	}
	if _, err := repository.UpsertArticles(biz, []officialaccount.ArticleRecord{article}, time.Unix(100, 0)); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	firstView, firstComment := int64(10), int64(7)
	secondView := int64(20)
	if err := repository.RecordArticleMetrics([]officialaccount.ArticleMetricSnapshot{
		{ArticleKey: article.Key, ObservedAt: 100, Source: "network", ViewCount: &firstView, CommentCount: &firstComment},
		{ArticleKey: article.Key, ObservedAt: 200, Source: "getappmsgext", ViewCount: &secondView},
	}); err != nil {
		t.Fatalf("record partial metric snapshots: %v", err)
	}
	latest, err := repository.LatestArticleMetrics([]string{article.Key})
	if err != nil {
		t.Fatalf("read latest known metrics: %v", err)
	}
	metric, ok := latest[article.Key]
	if !ok || metric.ViewCount == nil || *metric.ViewCount != 20 || metric.CommentCount == nil || *metric.CommentCount != 7 {
		t.Fatalf("latest known fields were not merged: %+v", latest)
	}
	if metric.ObservedAt != 200 || metric.Source != "getappmsgext" {
		t.Fatalf("latest snapshot metadata changed unexpectedly: %+v", metric)
	}
}

func TestOfficialAccountMetricSyncResumesFromSQLiteAfterServiceRestart(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	const biz = "biz-metric-restart"
	var requestsMu sync.Mutex
	var mids []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse resumed metric form: %v", err)
		}
		mid := r.PostForm.Get("mid")
		requestsMu.Lock()
		mids = append(mids, mid)
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if mid != "2" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"ret":0,"appmsgstat":{"read_num":200,"old_like_num":20,"comment_num":4}}`))
	}))
	defer server.Close()

	repository := NewOfficialAccountRepository()
	first, ok := officialaccount.ArticleRecordFromItem(biz, officialaccount.ArticleItem{
		Title: "第一篇", Mid: "1", Idx: 1,
		ContentURL:  "https://mp.weixin.qq.com/s/metric-first?mid=1&idx=1",
		PublishTime: 200,
	}, time.Unix(200, 0))
	if !ok {
		t.Fatal("first article identity was not built")
	}
	second, ok := officialaccount.ArticleRecordFromItem(biz, officialaccount.ArticleItem{
		Title: "第二篇", Mid: "2", Idx: 1,
		ContentURL:  "https://mp.weixin.qq.com/s/metric-second?mid=2&idx=1",
		PublishTime: 100,
	}, time.Unix(100, 0))
	if !ok {
		t.Fatal("second article identity was not built")
	}
	if _, err := repository.UpsertArticles(biz, []officialaccount.ArticleRecord{first, second}, time.Unix(200, 0)); err != nil {
		t.Fatalf("insert articles: %v", err)
	}

	view, like := int64(100), int64(10)
	if err := repository.SaveArticleMetricResult(officialaccount.ArticleMetricState{
		ArticleKey: first.Key, Status: officialaccount.MetricStateSuccess,
		AttemptCount: 1, SuccessCount: 1, LastSuccessAt: 300,
		LastObservedAt: 300, LastSource: "getappmsgext", UpdatedAt: 300,
	}, &officialaccount.ArticleMetricSnapshot{
		ArticleKey: first.Key, ObservedAt: 300, Source: "getappmsgext",
		ViewCount: &view, LikeCount: &like,
	}); err != nil {
		t.Fatalf("persist completed article state: %v", err)
	}
	if err := repository.CreateMetricSyncRun(officialaccount.MetricSyncRun{
		ID: "metric-sync-restart", Biz: biz, Status: officialaccount.SyncStatusRunning,
		Force: true, Total: 2, Attempted: 1, Stored: 1,
		AfterPublishTime: first.PublishTime, AfterArticleKey: first.Key,
		StartedAt: 301,
	}); err != nil {
		t.Fatalf("persist interrupted metric run: %v", err)
	}

	// A new service instance represents the next process. Loading its captured
	// credential and attaching the existing repository must resume the durable
	// running row without replaying the completed prefix.
	service := officialaccount.NewMemoryService()
	service.SetUpstreamBaseURL(server.URL)
	service.SetHTTPClient(server.Client())
	if err := service.Upsert(officialaccount.Account{
		Biz: biz, Key: "key-restart", Uin: "uin-restart",
		PassTicket: "ticket-restart", AppmsgToken: "token-restart",
	}); err != nil {
		t.Fatalf("load captured account: %v", err)
	}
	if err := service.SetCatalogRepository(repository); err != nil {
		t.Fatalf("attach catalog repository after restart: %v", err)
	}

	deadline := time.Now().Add(4 * time.Second)
	var completed *officialaccount.MetricSyncRun
	for time.Now().Before(deadline) {
		var err error
		completed, err = repository.GetMetricSyncRun("metric-sync-restart")
		if err != nil {
			t.Fatalf("read resumed metric run: %v", err)
		}
		if completed != nil && (completed.Status == officialaccount.SyncStatusCompleted || completed.Status == officialaccount.SyncStatusPartial || completed.Status == officialaccount.SyncStatusFailed || completed.Status == officialaccount.SyncStatusCancelled) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed == nil || completed.Status != officialaccount.SyncStatusCompleted {
		t.Fatalf("resumed metric run did not complete: %+v", completed)
	}
	if completed.Attempted != 2 || completed.Stored != 2 || completed.AfterArticleKey == first.Key {
		t.Fatalf("resumed metric run counters/cursor are wrong: %+v", completed)
	}
	requestsMu.Lock()
	gotMids := append([]string(nil), mids...)
	requestsMu.Unlock()
	if strings.Join(gotMids, ",") != "2" {
		t.Fatalf("restart replayed the completed metric prefix: %v", gotMids)
	}

	loaded, err := repository.GetArticle(second.Key)
	if err != nil {
		t.Fatalf("load resumed article: %v", err)
	}
	if loaded == nil || loaded.MetricState == nil || loaded.MetricState.Status != officialaccount.MetricStateSuccess || loaded.Metrics == nil || loaded.Metrics.ViewCount == nil || *loaded.Metrics.ViewCount != 200 {
		t.Fatalf("resumed article metrics were not persisted: %+v", loaded)
	}
}
