package officialaccount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"wx_channel/internal/response"
)

const (
	metricCandidatePageSize = 40
	metricRequestSpacing    = 300 * time.Millisecond
	maxMetricResponseBody   = 16 << 20
	metricClientVersion     = "f2541c37"
)

var (
	ErrMetricSyncAlreadyRunning = errors.New("official account metric sync is already running")
	ErrMetricSyncNotFound       = errors.New("official account metric sync run not found")
	ErrMetricArticleUnfetchable = errors.New("article does not contain enough identity fields for metric collection")
)

type metricFetchResult struct {
	Metrics      ArticleMetricPayload
	Raw          string
	Source       string
	ObservedAt   int64
	Unknown      bool
	CommentError error
}

// StartMetricSync starts a per-article interaction metric collection run. The
// durable keyset cursor and article states make both normal and force-refresh
// runs resumable without keeping the full article list in memory.
func (s *Service) StartMetricSync(request MetricSyncRequest) (*MetricSyncRun, error) {
	if s == nil {
		return nil, ErrCatalogUnavailable
	}
	repository := s.catalogRepository()
	if repository == nil {
		return nil, ErrCatalogUnavailable
	}
	biz := strings.TrimSpace(request.Biz)
	if biz == "" {
		return nil, ErrMissingBiz
	}
	account, ok := s.accountSnapshot(biz)
	if !ok {
		return nil, ErrAccountNotFound
	}
	if account.Key == "" {
		return nil, ErrAccountExpired
	}
	now := s.now()
	previous, err := repository.GetLatestMetricSyncRun(biz)
	if err != nil {
		return nil, err
	}
	reuse := previous != nil && metricSyncRunCanResume(previous.Status, request.Resume)
	run := MetricSyncRun{}
	if reuse {
		run = *previous
		run.Status = SyncStatusQueued
		run.FinishedAt = 0
		run.Error = ""
		run.StartedAt = now.Unix()
	} else {
		total, err := repository.CountArticleMetricCandidates(biz, request.Force, now)
		if err != nil {
			return nil, err
		}
		run = MetricSyncRun{
			ID:        uuid.NewString(),
			Biz:       biz,
			Status:    SyncStatusQueued,
			Force:     request.Force,
			Total:     total,
			StartedAt: now.Unix(),
		}
	}
	run.Biz = biz

	return s.startMetricSyncRun(repository, run, reuse)
}

func metricSyncRunCanResume(status string, explicit bool) bool {
	if status == SyncStatusQueued || status == SyncStatusRunning {
		return true
	}
	return explicit && (status == SyncStatusPartial || status == SyncStatusFailed || status == SyncStatusCancelled)
}

func metricSyncRunNeedsAutoResume(status string) bool {
	return status == SyncStatusQueued || status == SyncStatusRunning
}

func (s *Service) startMetricSyncRun(repository CatalogRepository, run MetricSyncRun, reuse bool) (*MetricSyncRun, error) {
	if repository == nil {
		return nil, ErrCatalogUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.syncMu.Lock()
	if s.activeMetricSyncs == nil {
		s.activeMetricSyncs = make(map[string]string)
	}
	if s.metricSyncCancels == nil {
		s.metricSyncCancels = make(map[string]context.CancelFunc)
	}
	if active := s.activeMetricSyncs[run.Biz]; active != "" {
		current, lookupErr := repository.GetMetricSyncRun(active)
		s.syncMu.Unlock()
		cancel()
		if lookupErr != nil {
			return nil, lookupErr
		}
		if current == nil {
			return nil, ErrMetricSyncAlreadyRunning
		}
		return current, nil
	}
	var err error
	if reuse {
		err = repository.UpdateMetricSyncRun(run)
	} else {
		err = repository.CreateMetricSyncRun(run)
	}
	if err != nil {
		s.syncMu.Unlock()
		cancel()
		return nil, err
	}
	s.activeMetricSyncs[run.Biz] = run.ID
	s.metricSyncCancels[run.ID] = cancel
	s.syncMu.Unlock()

	go s.runMetricSync(ctx, run)
	return &run, nil
}

// resumePendingMetricSync is called when the account credential owner is
// available. Imported catalog metadata alone cannot authorize upstream metric
// requests, so only queued/running runs for a captured account are resumed.
func (s *Service) resumePendingMetricSync(biz string) error {
	repository := s.catalogRepository()
	if repository == nil {
		return nil
	}
	biz = strings.TrimSpace(biz)
	if biz == "" {
		return nil
	}
	run, err := repository.GetLatestMetricSyncRun(biz)
	if err != nil {
		return fmt.Errorf("load pending official account metric sync for %q: %w", biz, err)
	}
	if run == nil || !metricSyncRunNeedsAutoResume(run.Status) {
		return nil
	}
	run.Status = SyncStatusQueued
	run.FinishedAt = 0
	run.Error = ""
	run.StartedAt = s.now().Unix()
	if _, err := s.startMetricSyncRun(repository, *run, true); err != nil {
		if errors.Is(err, ErrMetricSyncAlreadyRunning) {
			return nil
		}
		return fmt.Errorf("resume official account metric sync %q: %w", biz, err)
	}
	return nil
}

func (s *Service) HandleStartMetricSync(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		repository := s.catalogRepository()
		if repository == nil {
			writeServiceError(w, ErrCatalogUnavailable)
			return
		}
		biz := strings.TrimSpace(r.URL.Query().Get("biz"))
		if biz == "" {
			writeServiceError(w, ErrMissingBiz)
			return
		}
		run, err := repository.GetLatestMetricSyncRun(biz)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		response.Success(w, run)
		return
	}
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer r.Body.Close()
	var request MetricSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid metric sync request payload")
		return
	}
	if request.Biz == "" {
		request.Biz = strings.TrimSpace(r.URL.Query().Get("biz"))
	}
	if rawForce := strings.TrimSpace(r.URL.Query().Get("force")); rawForce != "" {
		force, err := strconv.ParseBool(rawForce)
		if err != nil {
			response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid force value")
			return
		}
		request.Force = force
	}
	run, err := s.StartMetricSync(request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.SuccessWithStatus(w, http.StatusAccepted, run)
}

func (s *Service) HandleMetricSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	repository := s.catalogRepository()
	if repository == nil {
		writeServiceError(w, ErrCatalogUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/mp/metrics/sync/")
	id, _ = url.PathUnescape(strings.TrimSpace(id))
	if id == "" || strings.Contains(id, "/") {
		writeServiceError(w, ErrMetricSyncNotFound)
		return
	}
	run, err := repository.GetMetricSyncRun(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if run == nil {
		writeServiceError(w, ErrMetricSyncNotFound)
		return
	}
	if r.Method == http.MethodDelete {
		s.syncMu.Lock()
		cancel := s.metricSyncCancels[id]
		s.syncMu.Unlock()
		if cancel == nil {
			if metricSyncTerminal(run.Status) {
				response.Success(w, run)
				return
			}
			response.ErrorWithStatus(w, http.StatusConflict, http.StatusConflict, "metric sync run is not cancellable in this process")
			return
		}
		cancel()
		response.SuccessWithStatus(w, http.StatusAccepted, map[string]interface{}{"id": id, "status": "cancelling"})
		return
	}
	response.Success(w, run)
}

func metricSyncTerminal(status string) bool {
	return status == SyncStatusCompleted || status == SyncStatusPartial || status == SyncStatusFailed || status == SyncStatusCancelled
}

func (s *Service) runMetricSync(ctx context.Context, run MetricSyncRun) {
	defer func() {
		s.syncMu.Lock()
		if s.activeMetricSyncs[run.Biz] == run.ID {
			delete(s.activeMetricSyncs, run.Biz)
		}
		if cancel := s.metricSyncCancels[run.ID]; cancel != nil {
			delete(s.metricSyncCancels, run.ID)
		}
		s.syncMu.Unlock()
	}()

	repository := s.catalogRepository()
	if repository == nil {
		return
	}
	run.Status = SyncStatusRunning
	if err := repository.UpdateMetricSyncRun(run); err != nil {
		finishMetricSync(repository, &run, SyncStatusFailed, err, s.now())
		return
	}

	var lastRequestAt time.Time
	afterPublishTime := run.AfterPublishTime
	afterArticleKey := strings.TrimSpace(run.AfterArticleKey)
	for {
		if err := ctx.Err(); err != nil {
			finishMetricSync(repository, &run, SyncStatusCancelled, err, s.now())
			return
		}
		page, err := repository.ListArticleMetricCandidates(run.Biz, run.Force, s.now(), afterPublishTime, afterArticleKey, metricCandidatePageSize)
		if err != nil {
			status := SyncStatusFailed
			if run.Attempted > 0 {
				status = SyncStatusPartial
			}
			finishMetricSync(repository, &run, status, err, s.now())
			return
		}
		if len(page.Items) == 0 {
			status := SyncStatusCompleted
			if run.Failed > 0 || run.Unknown > 0 {
				status = SyncStatusPartial
			}
			finishMetricSync(repository, &run, status, nil, s.now())
			return
		}

		pageStartPublishTime := afterPublishTime
		pageStartArticleKey := afterArticleKey
		keys := make([]string, 0, len(page.Items))
		for _, article := range page.Items {
			keys = append(keys, article.Key)
		}
		states, err := repository.ListArticleMetricStates(keys)
		if err != nil {
			status := SyncStatusFailed
			if run.Attempted > 0 {
				status = SyncStatusPartial
			}
			finishMetricSync(repository, &run, status, err, s.now())
			return
		}
		latestMetrics, err := repository.LatestArticleMetrics(keys)
		if err != nil {
			status := SyncStatusFailed
			if run.Attempted > 0 {
				status = SyncStatusPartial
			}
			finishMetricSync(repository, &run, status, err, s.now())
			return
		}

		for _, article := range page.Items {
			if err := ctx.Err(); err != nil {
				finishMetricSync(repository, &run, SyncStatusCancelled, err, s.now())
				return
			}
			if err := waitMetricRequestSpacing(ctx, &lastRequestAt); err != nil {
				finishMetricSync(repository, &run, SyncStatusCancelled, err, s.now())
				return
			}

			state := states[article.Key]
			if state.ArticleKey == "" {
				state = ArticleMetricState{ArticleKey: article.Key, Status: MetricStatePending}
			}
			state.ArticleKey = article.Key
			state.AttemptCount++
			state.LastAttemptAt = s.now().Unix()
			state.LastSource = "getappmsgext"
			state.LastError = ""
			run.Attempted++
			lastRequestAt = s.now()

			result, fetchErr := s.fetchArticleMetrics(ctx, run.Biz, article)
			if errors.Is(fetchErr, context.Canceled) || errors.Is(fetchErr, context.DeadlineExceeded) || ctx.Err() != nil {
				finishMetricSync(repository, &run, SyncStatusCancelled, ctx.Err(), s.now())
				return
			}

			var snapshot *ArticleMetricSnapshot
			if fetchErr != nil {
				state.Status = MetricStateFailed
				state.FailureCount++
				state.NextRetryAt = s.now().Add(metricRetryDelay(state.AttemptCount)).Unix()
				state.LastError = sanitizeMetricError(fetchErr.Error())
				run.Failed++
			} else if result.Unknown {
				state.Status = MetricStateUnknown
				state.UnknownCount++
				state.LastObservedAt = result.ObservedAt
				state.NextRetryAt = 0
				state.LastError = sanitizeMetricError("upstream did not return article metrics")
				run.Unknown++
			} else {
				commentError := result.CommentError
				if previous, ok := latestMetrics[article.Key]; ok {
					result.Metrics = mergeMetricPayloadWithSnapshot(result.Metrics, &previous)
				}
				state.Status = MetricStateSuccess
				state.SuccessCount++
				state.LastSuccessAt = result.ObservedAt
				state.LastObservedAt = result.ObservedAt
				state.NextRetryAt = 0
				state.LastError = ""
				if commentError != nil {
					state.LastError = sanitizeMetricError("comment metrics: " + commentError.Error())
					state.NextRetryAt = s.now().Add(metricRetryDelay(state.AttemptCount)).Unix()
					run.Failed++
				}
				snapshot = &ArticleMetricSnapshot{
					ArticleKey:   article.Key,
					ObservedAt:   result.ObservedAt,
					Source:       result.Source,
					ViewCount:    result.Metrics.ViewCount,
					LikeCount:    result.Metrics.LikeCount,
					CommentCount: result.Metrics.CommentCount,
					ShareCount:   result.Metrics.ShareCount,
					CollectCount: result.Metrics.CollectCount,
					RewardCount:  result.Metrics.RewardCount,
					RawMetadata:  result.Raw,
				}
				run.Stored++
			}
			state.UpdatedAt = s.now().Unix()
			if err := repository.SaveArticleMetricResult(state, snapshot); err != nil {
				status := SyncStatusFailed
				if run.Attempted > 1 {
					status = SyncStatusPartial
				}
				finishMetricSync(repository, &run, status, err, s.now())
				return
			}
			run.AfterPublishTime = article.PublishTime
			run.AfterArticleKey = article.Key
			if err := repository.UpdateMetricSyncRun(run); err != nil {
				finishMetricSync(repository, &run, SyncStatusPartial, fmt.Errorf("persist metric sync cursor: %w", err), s.now())
				return
			}

			if errors.Is(fetchErr, ErrAccountExpired) {
				finishMetricSync(repository, &run, SyncStatusPartial, fetchErr, s.now())
				return
			}
		}

		if !page.HasMore {
			status := SyncStatusCompleted
			if run.Failed > 0 || run.Unknown > 0 {
				status = SyncStatusPartial
			}
			finishMetricSync(repository, &run, status, nil, s.now())
			return
		}
		afterPublishTime = run.AfterPublishTime
		afterArticleKey = run.AfterArticleKey
		if afterArticleKey == "" || (afterArticleKey == pageStartArticleKey && afterPublishTime == pageStartPublishTime) {
			finishMetricSync(repository, &run, SyncStatusPartial, ErrSyncStalled, s.now())
			return
		}
	}
}

func waitMetricRequestSpacing(ctx context.Context, lastRequestAt *time.Time) error {
	if lastRequestAt == nil || lastRequestAt.IsZero() {
		return nil
	}
	delay := metricRequestSpacing - time.Since(*lastRequestAt)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func finishMetricSync(repository CatalogRepository, run *MetricSyncRun, status string, syncErr error, now time.Time) {
	if run == nil {
		return
	}
	run.Status = status
	run.FinishedAt = now.Unix()
	run.Error = ""
	if syncErr != nil {
		run.Error = sanitizeMetricError(syncErr.Error())
	}
	if err := repository.UpdateMetricSyncRun(*run); err != nil {
		// The article-level state remains the recovery source even if the run
		// summary cannot be updated at shutdown.
		fmt.Printf("official account metric sync %s: persist state: %v\n", run.ID, err)
	}
}

func metricRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Minute
	for i := 1; i < attempt && delay < 24*time.Hour; i++ {
		delay *= 2
	}
	if delay > 24*time.Hour {
		return 24 * time.Hour
	}
	return delay
}

func sanitizeMetricError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}

func (s *Service) fetchArticleMetrics(ctx context.Context, biz string, article ArticleRecord) (metricFetchResult, error) {
	account, ok := s.accountSnapshot(strings.TrimSpace(biz))
	if !ok {
		return metricFetchResult{}, ErrAccountNotFound
	}
	if account.Key == "" {
		return metricFetchResult{}, ErrAccountExpired
	}
	req, err := buildArticleMetricsRequest(ctx, s.upstream(), account, article)
	if err != nil {
		return metricFetchResult{Source: "getappmsgext", Unknown: true, ObservedAt: s.now().Unix()}, nil
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return metricFetchResult{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return metricFetchResult{}, fmt.Errorf("%w: metric status %d", ErrUpstream, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetricResponseBody+1))
	if err != nil {
		return metricFetchResult{}, fmt.Errorf("%w: read metric response: %v", ErrUpstream, err)
	}
	if len(body) > maxMetricResponseBody {
		return metricFetchResult{}, fmt.Errorf("%w: metric response is too large", ErrUpstream)
	}
	var envelope struct {
		Ret      int    `json:"ret"`
		ErrMsg   string `json:"errmsg"`
		BaseResp struct {
			Ret int `json:"ret"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return metricFetchResult{}, fmt.Errorf("%w: decode metric response: %v", ErrUpstream, err)
	}
	ret := envelope.Ret
	if ret == 0 {
		ret = envelope.BaseResp.Ret
	}
	if ret != 0 {
		if ret == -3 || ret == -6 {
			_ = s.markAccountInvalid(biz, envelope.ErrMsg)
			return metricFetchResult{}, fmt.Errorf("%w: %s", ErrAccountExpired, firstNonEmpty(envelope.ErrMsg, "credential rejected"))
		}
		return metricFetchResult{}, fmt.Errorf("%w: %s", ErrUpstream, firstNonEmpty(envelope.ErrMsg, "metric request rejected"))
	}
	payload, raw := ExtractArticleMetricsJSON(string(body))
	observedAt := s.now().Unix()
	result := metricFetchResult{Metrics: payload, Raw: raw, Source: "getappmsgext", ObservedAt: observedAt}
	if payload.CommentCount == nil {
		commentPayload, commentRaw, commentErr := s.fetchArticleCommentCount(ctx, account, article)
		if commentErr != nil {
			result.CommentError = commentErr
		} else {
			result.Metrics = mergeMetricPayload(result.Metrics, commentPayload)
			result.Raw = mergeMetricEvidenceRaw(result.Raw, commentRaw)
		}
	}
	if metricPayloadEmpty(result.Metrics) {
		result.Unknown = true
	}
	return result, nil
}

func buildArticleMetricsRequest(ctx context.Context, baseURL string, account Account, article ArticleRecord) (*http.Request, error) {
	var mid, idx, sn string
	var articleCreateTime string
	for _, raw := range []string{article.ContentURL, article.SourceURL} {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		query := u.Query()
		if mid == "" {
			mid = strings.TrimSpace(query.Get("mid"))
		}
		if idx == "" {
			idx = strings.TrimSpace(query.Get("idx"))
		}
		if sn == "" {
			sn = strings.TrimSpace(query.Get("sn"))
		}
		if articleCreateTime == "" {
			articleCreateTime = strings.TrimSpace(query.Get("ct"))
		}
	}
	if mid == "" {
		mid = strings.TrimSpace(article.Mid)
	}
	if idx == "" && article.Idx > 0 {
		idx = strconv.Itoa(article.Idx)
	}
	if idx == "" {
		idx = "1"
	}
	if mid == "" {
		return nil, ErrMetricArticleUnfetchable
	}
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/mp/getappmsgext")
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("__biz", account.Biz)
	query.Set("clientversion", metricClientVersion)
	query.Set("appmsg_token", account.AppmsgToken)
	query.Set("f", "json")
	query.Set("mock", "7")
	query.Set("x5", "0")
	u.RawQuery = query.Encode()

	if articleCreateTime == "" && article.PublishTime > 0 {
		articleCreateTime = strconv.FormatInt(article.PublishTime, 10)
	}
	form := url.Values{}
	form.Set("appmsg_type", "9")
	form.Set("mid", mid)
	form.Set("sn", sn)
	form.Set("idx", idx)
	form.Set("ct", articleCreateTime)
	form.Set("devicetype", "UnifiedPCWindows")
	form.Set("version", metricClientVersion)
	form.Set("msg_daily_idx", "1")
	form.Set("is_only_read", "1")
	form.Set("item_show_type", strconv.Itoa(article.ItemShowType))
	form.Set("appmsg_like_type", "2")
	form.Set("pass_ticket", account.PassTicket)
	form.Set("comment_id", "")
	form.Set("req_id", "")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Referer", normalizeArticleURL(firstNonEmpty(article.ContentURL, article.SourceURL)))
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if account.Cookie != "" {
		req.Header.Set("Cookie", account.Cookie)
	}
	return req, nil
}
