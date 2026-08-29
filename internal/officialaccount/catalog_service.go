package officialaccount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"wx_channel/internal/response"
)

const (
	defaultCatalogPageSize = 50
	maxCatalogPageSize     = 100
	defaultSyncPageSize    = 10
	syncStaleAfter         = 15 * time.Minute
)

func (s *Service) ListAccountsPage(keyword string, page, pageSize int) (AccountPage, error) {
	if s == nil {
		return AccountPage{}, ErrCatalogUnavailable
	}
	repository := s.catalogRepository()
	if repository != nil {
		result, err := repository.ListAccounts(keyword, page, pageSize)
		if err != nil {
			return AccountPage{}, err
		}
		s.enrichAccountPage(&result)
		return result, nil
	}

	page, pageSize = normalizeCatalogPage(page, pageSize, defaultCatalogPageSize, maxCatalogPageSize)
	list := s.ListAccountsByKeyword(keyword)
	total := int64(len(list))
	start := (page - 1) * pageSize
	if start >= len(list) {
		list = []AccountSummary{}
	} else {
		end := start + pageSize
		if end > len(list) {
			end = len(list)
		}
		list = list[start:end]
	}
	return AccountPage{
		Items:      list,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: catalogTotalPages(total, pageSize),
	}, nil
}

func (s *Service) enrichAccountPage(page *AccountPage) {
	if s == nil || page == nil {
		return
	}
	now := s.now()
	s.mu.RLock()
	origin := s.apiOrigin
	for i := range page.Items {
		item := &page.Items[i]
		if account, ok := s.accounts[item.Biz]; ok && account != nil {
			effective := account.IsEffective
			if account.UpdateTime > 0 && now.Sub(time.Unix(account.UpdateTime, 0)) > accountStaleAfter {
				effective = false
			}
			item.IsEffective = effective
			item.Error = account.Error
			item.RefreshURI = sanitizeRefreshURI(account.RefreshURI)
		}
		item.Links = buildAccountLinks(origin, item.Biz)
	}
	s.mu.RUnlock()
}

func (s *Service) HandleCatalogArticles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	repository := s.catalogRepository()
	if repository == nil {
		writeServiceError(w, ErrCatalogUnavailable)
		return
	}
	page, err := repository.ListArticles(ArticleQuery{
		Biz:           strings.TrimSpace(r.URL.Query().Get("biz")),
		Keyword:       strings.TrimSpace(r.URL.Query().Get("keyword")),
		ArchiveStatus: strings.TrimSpace(r.URL.Query().Get("archive_status")),
		Page:          parsePage(r.URL.Query().Get("page")),
		PageSize:      parsePageSize(r.URL.Query().Get("page_size")),
		Sort:          strings.TrimSpace(r.URL.Query().Get("sort")),
		Descending:    parseDescending(r.URL.Query().Get("descending")),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, page)
}

func (s *Service) StartSync(request SyncRequest) (*SyncRun, error) {
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
	mode := strings.TrimSpace(request.Mode)
	if mode == "" {
		mode = SyncModeHistory
	}
	if mode != SyncModeHistory && mode != SyncModeRecent {
		return nil, ErrInvalidSyncMode
	}
	account, ok := s.accountSnapshot(biz)
	if !ok {
		return nil, ErrAccountNotFound
	}
	if account.Key == "" {
		return nil, ErrAccountExpired
	}

	now := s.now()
	var (
		previous *SyncRun
		reuse    bool
	)
	if request.Resume {
		var err error
		previous, err = repository.GetLatestSyncRun(biz, mode)
		if err != nil {
			return nil, err
		}
		if previous != nil && catalogSyncRunCanResume(previous.Status) {
			reuse = true
		}
	}

	run := SyncRun{}
	if reuse {
		run = *previous
		if run.NextOffset < 0 {
			run.NextOffset = 0
		}
		run.Offset = run.NextOffset
		run.Status = SyncStatusQueued
		run.FinishedAt = 0
		run.Error = ""
		run.CanContinue = true
		run.StartedAt = now.Unix()
		if run.PageSize <= 0 {
			run.PageSize = defaultSyncPageSize
		}
	} else {
		run = SyncRun{
			ID:          uuid.NewString(),
			Biz:         biz,
			Mode:        mode,
			Status:      SyncStatusQueued,
			PageSize:    defaultSyncPageSize,
			CanContinue: true,
			StartedAt:   now.Unix(),
		}
	}
	run.Biz = biz
	run.Mode = mode

	return s.startCatalogSyncRun(repository, run, reuse)
}

func catalogSyncRunCanResume(status string) bool {
	switch status {
	case SyncStatusQueued, SyncStatusRunning, SyncStatusPartial, SyncStatusFailed, SyncStatusCancelled:
		return true
	default:
		return false
	}
}

func catalogSyncRunNeedsAutoResume(status string) bool {
	return status == SyncStatusQueued || status == SyncStatusRunning
}

// startCatalogSyncRun is shared by explicit resume and process-start recovery.
// The caller supplies the existing run ID when durable progress should be
// continued instead of creating a second history record.
func (s *Service) startCatalogSyncRun(repository CatalogRepository, run SyncRun, reuse bool) (*SyncRun, error) {
	if repository == nil {
		return nil, ErrCatalogUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.syncMu.Lock()
	if s.activeSyncs == nil {
		s.activeSyncs = make(map[string]string)
	}
	if s.syncCancels == nil {
		s.syncCancels = make(map[string]context.CancelFunc)
	}
	if active := s.activeSyncs[run.Biz]; active != "" {
		s.syncMu.Unlock()
		cancel()
		return nil, ErrSyncAlreadyRunning
	}
	var err error
	if reuse {
		err = repository.UpdateSyncRun(run)
	} else {
		err = repository.CreateSyncRun(run)
	}
	if err != nil {
		s.syncMu.Unlock()
		cancel()
		return nil, err
	}
	s.activeSyncs[run.Biz] = run.ID
	s.syncCancels[run.ID] = cancel
	s.syncMu.Unlock()

	go s.runSync(ctx, run)
	return &run, nil
}

// resumePendingCatalogSync starts the newest queued/interrupted catalog run
// for an account after its credentials become available. Catalog imports may
// contain metadata without credentials, so only the in-memory account owner
// can authorize this recovery.
func (s *Service) resumePendingCatalogSync(biz string) error {
	repository := s.catalogRepository()
	if repository == nil {
		return nil
	}
	biz = strings.TrimSpace(biz)
	if biz == "" {
		return nil
	}
	var pending *SyncRun
	for _, mode := range []string{SyncModeHistory, SyncModeRecent} {
		run, err := repository.GetLatestSyncRun(biz, mode)
		if err != nil {
			return fmt.Errorf("load pending official account sync for %q: %w", biz, err)
		}
		if run == nil || !catalogSyncRunNeedsAutoResume(run.Status) {
			continue
		}
		if pending == nil || run.StartedAt > pending.StartedAt ||
			(run.StartedAt == pending.StartedAt && run.ID > pending.ID) {
			pending = run
		}
	}
	if pending == nil {
		return nil
	}
	pending.Status = SyncStatusQueued
	pending.FinishedAt = 0
	pending.Error = ""
	pending.CanContinue = true
	pending.StartedAt = s.now().Unix()
	if pending.NextOffset < 0 {
		pending.NextOffset = 0
	}
	pending.Offset = pending.NextOffset
	if pending.PageSize <= 0 {
		pending.PageSize = defaultSyncPageSize
	}
	if _, err := s.startCatalogSyncRun(repository, *pending, true); err != nil {
		if errors.Is(err, ErrSyncAlreadyRunning) {
			return nil
		}
		return fmt.Errorf("resume official account sync %q: %w", biz, err)
	}
	return nil
}

func (s *Service) runSync(ctx context.Context, run SyncRun) {
	defer func() {
		s.syncMu.Lock()
		if s.activeSyncs[run.Biz] == run.ID {
			delete(s.activeSyncs, run.Biz)
		}
		if cancel := s.syncCancels[run.ID]; cancel != nil {
			delete(s.syncCancels, run.ID)
		}
		s.syncMu.Unlock()
	}()

	repository := s.catalogRepository()
	if repository == nil {
		return
	}
	run.Status = SyncStatusRunning
	if err := repository.UpdateSyncRun(run); err != nil {
		s.finishSync(repository, &run, SyncStatusFailed, fmt.Errorf("mark sync run running: %w", err))
		return
	}
	offset := run.NextOffset
	for {
		if err := ctx.Err(); err != nil {
			s.finishSync(repository, &run, SyncStatusCancelled, err)
			return
		}
		data, err := s.fetchMsgList(ctx, run.Biz, offset)
		if err != nil {
			if ctx.Err() != nil {
				s.finishSync(repository, &run, SyncStatusCancelled, ctx.Err())
				return
			}
			status := SyncStatusFailed
			if run.PageCount > 0 {
				status = SyncStatusPartial
			}
			run.Offset = offset
			run.NextOffset = offset
			run.CanContinue = true
			s.finishSync(repository, &run, status, err)
			return
		}

		seenAt := s.now()
		records := make([]ArticleRecord, 0, len(data.Articles))
		seenKeys := make(map[string]struct{}, len(data.Articles))
		for _, item := range data.Articles {
			record, ok := ArticleRecordFromItem(run.Biz, item, seenAt)
			if !ok {
				continue
			}
			if _, exists := seenKeys[record.Key]; exists {
				continue
			}
			seenKeys[record.Key] = struct{}{}
			records = append(records, record)
		}
		stats, err := repository.UpsertArticles(run.Biz, records, seenAt)
		if err != nil {
			status := SyncStatusFailed
			if run.PageCount > 0 {
				status = SyncStatusPartial
			}
			run.Offset = offset
			run.NextOffset = offset
			run.CanContinue = true
			s.finishSync(repository, &run, status, err)
			return
		}

		run.PageCount++
		run.Fetched += len(data.Articles)
		run.Inserted += stats.Inserted
		run.Updated += stats.Updated
		run.Offset = offset
		nextOffset := data.NextOffset
		batchSize := len(data.List)
		if batchSize == 0 {
			batchSize = len(data.Articles)
		}
		if nextOffset <= offset && data.CanMsgContinue != 0 {
			nextOffset = offset + batchSize
		}
		run.NextOffset = nextOffset
		run.CanContinue = data.CanMsgContinue != 0 && run.Mode == SyncModeHistory
		if err := repository.UpdateSyncRun(run); err != nil {
			log.Printf("official account sync %s: persist page state: %v", run.ID, err)
			run.CanContinue = true
			s.finishSync(repository, &run, SyncStatusPartial, fmt.Errorf("persist page state: %w", err))
			return
		}

		if run.Mode == SyncModeRecent || data.CanMsgContinue == 0 {
			run.CanContinue = false
			s.finishSync(repository, &run, SyncStatusCompleted, nil)
			return
		}
		if nextOffset <= offset {
			s.finishSync(repository, &run, SyncStatusPartial, ErrSyncStalled)
			return
		}
		offset = nextOffset
	}
}

func (s *Service) finishSync(repository CatalogRepository, run *SyncRun, status string, syncErr error) {
	run.Status = status
	run.FinishedAt = s.now().Unix()
	run.Error = ""
	if syncErr != nil {
		run.Error = syncErr.Error()
	}
	if status == SyncStatusCompleted {
		run.CanContinue = false
	}
	s.persistSyncRun(repository, *run)
}

func (s *Service) persistSyncRun(repository CatalogRepository, run SyncRun) {
	if err := repository.UpdateSyncRun(run); err != nil {
		log.Printf("official account sync %s: persist state: %v", run.ID, err)
	}
}

func (s *Service) HandleStartSync(w http.ResponseWriter, r *http.Request) {
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
		mode := strings.TrimSpace(r.URL.Query().Get("mode"))
		if mode == "" {
			mode = SyncModeHistory
		}
		if mode != SyncModeHistory && mode != SyncModeRecent {
			writeServiceError(w, ErrInvalidSyncMode)
			return
		}
		run, err := repository.GetLatestSyncRun(biz, mode)
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
	var request SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid sync request payload")
		return
	}
	if request.Biz == "" {
		request.Biz = strings.TrimSpace(r.URL.Query().Get("biz"))
	}
	run, err := s.StartSync(request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.SuccessWithStatus(w, http.StatusAccepted, run)
}

func (s *Service) HandleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	repository := s.catalogRepository()
	if repository == nil {
		writeServiceError(w, ErrCatalogUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/mp/sync/")
	id, _ = url.PathUnescape(strings.TrimSpace(id))
	if id == "" || strings.Contains(id, "/") {
		writeServiceError(w, ErrSyncNotFound)
		return
	}
	run, err := repository.GetSyncRun(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if run == nil {
		writeServiceError(w, ErrSyncNotFound)
		return
	}
	if r.Method == http.MethodDelete {
		s.syncMu.Lock()
		cancel := s.syncCancels[id]
		s.syncMu.Unlock()
		if cancel == nil {
			if run.Status == SyncStatusCompleted || run.Status == SyncStatusPartial || run.Status == SyncStatusFailed || run.Status == SyncStatusCancelled {
				response.Success(w, run)
				return
			}
			response.ErrorWithStatus(w, http.StatusConflict, http.StatusConflict, "sync run is not cancellable in this process")
			return
		}
		cancel()
		response.SuccessWithStatus(w, http.StatusAccepted, map[string]interface{}{"id": id, "status": "cancelling"})
		return
	}
	response.Success(w, run)
}

func parsePage(raw string) int {
	page, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || page < 1 {
		return 1
	}
	if page > 1000000 {
		return 1000000
	}
	return page
}

func parsePageSize(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	pageSize, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || pageSize < 1 {
		return 0
	}
	if pageSize > maxCatalogPageSize {
		return maxCatalogPageSize
	}
	return pageSize
}

func parseDescending(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return value
}

func normalizeCatalogPage(page, pageSize, defaultSize, maxSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultSize
	}
	if pageSize > maxSize {
		pageSize = maxSize
	}
	return page, pageSize
}

func catalogTotalPages(total int64, pageSize int) int {
	if total == 0 || pageSize <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}
