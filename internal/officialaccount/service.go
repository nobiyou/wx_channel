package officialaccount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"wx_channel/internal/response"
)

var (
	ErrMissingBiz         = errors.New("biz is required")
	ErrMissingKey         = errors.New("key is required")
	ErrMissingMetadata    = errors.New("official account metadata is empty")
	ErrAccountNotFound    = errors.New("official account not found")
	ErrAccountExpired     = errors.New("official account credential expired")
	ErrMissingAuthorID    = errors.New("official account is missing author_id")
	ErrUpstream           = errors.New("official account upstream request failed")
	ErrInvalidProxyURL    = errors.New("proxy url is not allowed")
	ErrContentNotFound    = errors.New("article content not found")
	ErrArticleTooLarge    = errors.New("article content too large")
	ErrSyncAlreadyRunning = errors.New("official account sync is already running")
	ErrInvalidSyncMode    = errors.New("invalid official account sync mode")
	ErrSyncNotFound       = errors.New("official account sync run not found")
	ErrSyncStalled        = errors.New("official account sync cursor did not advance")
)

const (
	accountStaleAfter = 30 * time.Minute
	maxRequestBody    = 1 << 20
	maxJSONBody       = 16 << 20
	defaultUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Service owns public-account credentials and the HTTP-facing collection APIs.
// It deliberately does not own a second WebSocket or proxy runtime: page
// injection and request interception remain owned by wx_channel.
type Service struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	catalog  CatalogRepository

	syncMu            sync.Mutex
	activeSyncs       map[string]string
	syncCancels       map[string]context.CancelFunc
	activeMetricSyncs map[string]string
	metricSyncCancels map[string]context.CancelFunc

	storePath       string
	apiOrigin       string
	upstreamBaseURL string
	httpClient      *http.Client
	now             func() time.Time
}

func NewService(storePath string) (*Service, error) {
	s := &Service{
		accounts:          make(map[string]*Account),
		activeSyncs:       make(map[string]string),
		syncCancels:       make(map[string]context.CancelFunc),
		activeMetricSyncs: make(map[string]string),
		metricSyncCancels: make(map[string]context.CancelFunc),
		storePath:         storePath,
		upstreamBaseURL:   "https://mp.weixin.qq.com",
		httpClient:        &http.Client{Timeout: 15 * time.Second},
		now:               time.Now,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func NewMemoryService() *Service {
	s, _ := NewService("")
	return s
}

func (s *Service) SetAPIOrigin(origin string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.apiOrigin = strings.TrimRight(strings.TrimSpace(origin), "/")
	s.mu.Unlock()
}

func (s *Service) SetUpstreamBaseURL(baseURL string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.upstreamBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	s.mu.Unlock()
}

func (s *Service) SetHTTPClient(client *http.Client) {
	if s == nil || client == nil {
		return
	}
	s.mu.Lock()
	s.httpClient = client
	s.mu.Unlock()
}

// SetCatalogRepository makes SQLite the durable catalog owner while keeping
// the existing mp.json file as a credential compatibility store.
func (s *Service) SetCatalogRepository(repository CatalogRepository) error {
	if s == nil {
		return ErrCatalogUnavailable
	}
	s.mu.Lock()
	s.catalog = repository
	accounts := make([]Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		if account != nil {
			accounts = append(accounts, *account)
		}
	}
	s.mu.Unlock()
	if repository == nil {
		return nil
	}
	for _, account := range accounts {
		if err := repository.UpsertAccount(account); err != nil {
			return fmt.Errorf("migrate captured account %q into catalog: %w", account.Biz, err)
		}
	}
	for _, account := range accounts {
		if err := s.resumePendingCatalogSync(account.Biz); err != nil {
			return err
		}
		if err := s.resumePendingMetricSync(account.Biz); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) catalogRepository() CatalogRepository {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.catalog
}

func (s *Service) persistCatalogAccount(account Account) error {
	repository := s.catalogRepository()
	if repository == nil {
		return nil
	}
	if err := repository.UpsertAccount(account); err != nil {
		return fmt.Errorf("persist official account %q in catalog: %w", account.Biz, err)
	}
	return nil
}

func (s *Service) load() error {
	if s.storePath == "" {
		return nil
	}
	data, err := os.ReadFile(s.storePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read official account store: %w", err)
	}

	var stored map[string]Account
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("decode official account store: %w", err)
	}
	for biz, account := range stored {
		if account.Biz == "" {
			account.Biz = biz
		}
		if account.Cookie != "" && account.CookieExpiration == 0 {
			cookieBaseTime := account.UpdateTime
			if cookieBaseTime == 0 {
				cookieBaseTime = time.Now().Unix()
			}
			account.CookieExpiration = cookieBaseTime + int64((24 * time.Hour).Seconds())
		}
		if account.Biz != "" {
			copy := account
			s.accounts[account.Biz] = &copy
		}
	}
	return nil
}

func (s *Service) saveLocked() error {
	if s.storePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.storePath), 0755); err != nil {
		return fmt.Errorf("create official account store directory: %w", err)
	}

	stored := make(map[string]Account, len(s.accounts))
	for biz, account := range s.accounts {
		if account != nil {
			stored[biz] = *account
		}
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("encode official account store: %w", err)
	}
	if err := os.WriteFile(s.storePath, data, 0600); err != nil {
		return fmt.Errorf("write official account store: %w", err)
	}
	return nil
}

func (s *Service) Upsert(account Account) error {
	if s == nil {
		return errors.New("official account service is unavailable")
	}
	account.Biz = strings.TrimSpace(account.Biz)
	account.Key = strings.TrimSpace(account.Key)
	if account.Biz == "" {
		return ErrMissingBiz
	}
	if account.Key == "" {
		return ErrMissingKey
	}

	now := s.now().Unix()
	s.mu.Lock()
	previous, existed := s.accounts[account.Biz]
	var merged Account
	if existed && previous != nil {
		merged = *previous
		merged.MergeFrom(account)
	} else {
		merged = account
	}
	merged.Biz = account.Biz
	merged.IsEffective = true
	merged.UpdateTime = now
	merged.Error = ""
	if merged.CreatedAt == 0 {
		merged.CreatedAt = now
	}
	s.accounts[merged.Biz] = &merged
	if err := s.saveLocked(); err != nil {
		if existed {
			s.accounts[account.Biz] = previous
		} else {
			delete(s.accounts, account.Biz)
		}
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if err := s.persistCatalogAccount(merged); err != nil {
		return err
	}
	if err := s.resumePendingCatalogSync(merged.Biz); err != nil {
		return err
	}
	if err := s.resumePendingMetricSync(merged.Biz); err != nil {
		return err
	}
	return nil
}

// UpdateMetadata enriches an already captured account without changing its
// credential freshness. Article pages commonly expose biz and profile data,
// while the short-lived key only appears on a separate request.
func (s *Service) UpdateMetadata(account Account) error {
	if s == nil {
		return errors.New("official account service is unavailable")
	}
	account.Biz = strings.TrimSpace(account.Biz)
	account.Nickname = strings.TrimSpace(account.Nickname)
	account.AvatarURL = strings.TrimSpace(account.AvatarURL)
	account.AuthorID = strings.TrimSpace(account.AuthorID)
	if account.Biz == "" {
		return ErrMissingBiz
	}
	if account.Nickname == "" && account.AvatarURL == "" && account.AuthorID == "" {
		return ErrMissingMetadata
	}

	s.mu.Lock()
	previous, existed := s.accounts[account.Biz]
	if !existed || previous == nil {
		s.mu.Unlock()
		return ErrAccountNotFound
	}
	merged := *previous
	if account.Nickname != "" {
		merged.Nickname = account.Nickname
	}
	if account.AvatarURL != "" {
		merged.AvatarURL = account.AvatarURL
	}
	if account.AuthorID != "" {
		merged.AuthorID = account.AuthorID
	}
	s.accounts[account.Biz] = &merged
	if err := s.saveLocked(); err != nil {
		s.accounts[account.Biz] = previous
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if err := s.persistCatalogAccount(merged); err != nil {
		return err
	}
	return nil
}

func (s *Service) accountSnapshot(biz string) (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	account, ok := s.accounts[biz]
	if !ok || account == nil {
		return Account{}, false
	}
	return *account, true
}

// ArchiveRequestHeaders returns the minimum upstream headers needed by the
// article archive downloader. Credentials stay inside this service instead of
// crossing the HTTP API boundary.
func (s *Service) ArchiveRequestHeaders(biz, referer string) (map[string]string, error) {
	biz = strings.TrimSpace(biz)
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

	headers := map[string]string{
		"Accept":     "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
		"User-Agent": defaultUserAgent,
	}
	if account.Cookie != "" {
		headers["Cookie"] = account.Cookie
	}
	if referer = normalizeArticleURL(referer); referer != "" {
		headers["Referer"] = referer
	}
	return headers, nil
}

func (s *Service) ListAccounts() []AccountSummary {
	return s.ListAccountsByKeyword("")
}

// ListAccountsByKeyword returns safe summaries for captured accounts whose
// nickname or biz contains keyword. It never performs an upstream search.
func (s *Service) ListAccountsByKeyword(keyword string) []AccountSummary {
	if s == nil {
		return nil
	}
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	now := s.now()
	s.mu.RLock()
	list := make([]AccountSummary, 0, len(s.accounts))
	for _, account := range s.accounts {
		if account == nil {
			continue
		}
		effective := account.IsEffective
		if account.UpdateTime > 0 && now.Sub(time.Unix(account.UpdateTime, 0)) > accountStaleAfter {
			effective = false
		}
		summary := s.summaryLocked(*account, effective)
		if keyword != "" &&
			!strings.Contains(strings.ToLower(summary.Nickname), keyword) &&
			!strings.Contains(strings.ToLower(summary.Biz), keyword) {
			continue
		}
		list = append(list, summary)
	}
	apiOrigin := s.apiOrigin
	s.mu.RUnlock()

	for i := range list {
		list[i].Links = buildAccountLinks(apiOrigin, list[i].Biz)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt != list[j].CreatedAt {
			return list[i].CreatedAt > list[j].CreatedAt
		}
		return list[i].Biz < list[j].Biz
	})
	return list
}

func (s *Service) summaryLocked(account Account, effective bool) AccountSummary {
	return AccountSummary{
		Biz:         account.Biz,
		Nickname:    account.Nickname,
		AvatarURL:   account.AvatarURL,
		IsEffective: effective,
		CreatedAt:   account.CreatedAt,
		UpdateTime:  account.UpdateTime,
		Error:       account.Error,
		RefreshURI:  sanitizeRefreshURI(account.RefreshURI),
	}
}

func sanitizeRefreshURI(raw string) string {
	raw = html.UnescapeString(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	query := u.Query()
	for _, name := range []string{"uin", "key", "pass_ticket", "appmsg_token", "wxtoken", "token"} {
		query.Del(name)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func buildAccountLinks(origin, biz string) []AccountLink {
	base := strings.TrimRight(origin, "/")
	if base == "" {
		base = ""
	}
	return []AccountLink{
		{Name: "msg_api", URI: base + "/api/mp/msg/list?biz=" + url.QueryEscape(biz)},
		{Name: "article_api", URI: base + "/api/mp/article/list?biz=" + url.QueryEscape(biz)},
		{Name: "catalog", URI: base + "/api/mp/articles?biz=" + url.QueryEscape(biz)},
		{Name: "sync", URI: base + "/api/mp/sync?biz=" + url.QueryEscape(biz)},
		{Name: "metrics_sync", URI: base + "/api/mp/metrics/sync?biz=" + url.QueryEscape(biz)},
		{Name: "rss", URI: base + "/rss/mp?biz=" + url.QueryEscape(biz)},
	}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	if mux == nil || s == nil {
		return
	}
	mux.HandleFunc("/api/mp/list", s.HandleList)
	mux.HandleFunc("/api/mp/refresh", s.HandleRefresh)
	mux.HandleFunc("/api/mp/metadata", s.HandleMetadata)
	mux.HandleFunc("/api/mp/msg/list", s.HandleMsgList)
	mux.HandleFunc("/api/mp/article/list", s.HandleArticleList)
	mux.HandleFunc("/api/mp/articles", s.HandleCatalogArticles)
	mux.HandleFunc("/api/mp/metrics", s.HandleArticleMetrics)
	mux.HandleFunc("/api/mp/metrics/sync", s.HandleStartMetricSync)
	mux.HandleFunc("/api/mp/metrics/sync/", s.HandleMetricSyncStatus)
	mux.HandleFunc("/api/mp/sync", s.HandleStartSync)
	mux.HandleFunc("/api/mp/sync/", s.HandleSyncStatus)
	mux.HandleFunc("/api/mp/catalog/export", s.HandleCatalogExport)
	mux.HandleFunc("/api/mp/catalog/import", s.HandleCatalogImport)
	mux.HandleFunc("/api/mp/archive/plan", s.HandleArchivePlan)
}

func (s *Service) RegisterPublicRoutes(mux *http.ServeMux) {
	if mux == nil || s == nil {
		return
	}
	mux.HandleFunc("/rss/mp", s.HandleRSS)
	mux.HandleFunc("/mp/proxy", s.HandleProxy)
}

func (s *Service) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	page, err := s.ListAccountsPage(keyword, parsePage(r.URL.Query().Get("page")), parsePageSize(r.URL.Query().Get("page_size")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, map[string]interface{}{
		"list":        page.Items,
		"items":       page.Items,
		"total":       page.Total,
		"page":        page.Page,
		"page_size":   page.PageSize,
		"total_pages": page.TotalPages,
		"keyword":     keyword,
	})
}

func (s *Service) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer r.Body.Close()
	var account Account
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&account); err != nil {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid account payload")
		return
	}
	if err := s.Upsert(account); err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, map[string]string{"biz": account.Biz})
}

func (s *Service) HandleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer r.Body.Close()
	var account Account
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&account); err != nil {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid account metadata payload")
		return
	}
	if err := s.UpdateMetadata(account); err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, map[string]string{"biz": account.Biz})
}

func (s *Service) HandleMsgList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	biz := strings.TrimSpace(r.URL.Query().Get("biz"))
	offset := parseOffset(r.URL.Query().Get("offset"))
	data, err := s.FetchMsgList(biz, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, data)
}

func (s *Service) HandleArticleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	biz := strings.TrimSpace(r.URL.Query().Get("biz"))
	data, err := s.FetchArticleList(biz)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, data)
}

func (s *Service) HandleArchivePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	defer r.Body.Close()

	var request ArchiveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response.ErrorWithStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid archive request payload")
		return
	}
	plan, err := request.BuildPlan()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.Success(w, SanitizeArchivePlanForResponse(plan))
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	code := http.StatusBadGateway
	message := err.Error()
	switch {
	case errors.Is(err, ErrMissingBiz), errors.Is(err, ErrMissingKey), errors.Is(err, ErrMissingMetadata), errors.Is(err, ErrMissingAuthorID), errors.Is(err, ErrMetricArticleUnfetchable):
		status = http.StatusBadRequest
		code = http.StatusBadRequest
	case errors.Is(err, ErrInvalidSyncMode):
		status = http.StatusBadRequest
		code = http.StatusBadRequest
	case errors.Is(err, ErrSyncAlreadyRunning):
		status = http.StatusConflict
		code = http.StatusConflict
	case errors.Is(err, ErrMetricSyncAlreadyRunning):
		status = http.StatusConflict
		code = http.StatusConflict
	case errors.Is(err, ErrSyncNotFound):
		status = http.StatusNotFound
		code = http.StatusNotFound
	case errors.Is(err, ErrMetricSyncNotFound):
		status = http.StatusNotFound
		code = http.StatusNotFound
	case errors.Is(err, ErrContentNotFound), errors.Is(err, ErrArticleIdentity):
		status = http.StatusBadRequest
		code = http.StatusBadRequest
	case errors.Is(err, ErrArticleTooLarge):
		status = http.StatusRequestEntityTooLarge
		code = http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrAccountNotFound):
		status = http.StatusNotFound
		code = http.StatusNotFound
	case errors.Is(err, ErrAccountExpired):
		status = http.StatusUnauthorized
		code = http.StatusUnauthorized
	case errors.Is(err, ErrInvalidProxyURL):
		status = http.StatusBadRequest
		code = http.StatusBadRequest
	}
	response.ErrorWithStatus(w, status, code, message)
}

func parseOffset(raw string) int {
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 {
		return 0
	}
	if offset > 1000000 {
		return 1000000
	}
	return offset
}

func (s *Service) FetchMsgList(biz string, offset int) (*MessageListResponse, error) {
	return s.fetchMsgList(context.Background(), biz, offset)
}

func (s *Service) fetchMsgList(ctx context.Context, biz string, offset int) (*MessageListResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	biz = strings.TrimSpace(biz)
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

	target := s.buildMsgListURL(account, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build message list request: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", s.buildProfileURL(account))
	req.Header.Set("User-Agent", defaultUserAgent)
	if account.Cookie != "" {
		req.Header.Set("Cookie", account.Cookie)
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}
	var data MessageListResponse
	if err := decodeJSONBody(resp.Body, &data); err != nil {
		return nil, fmt.Errorf("%w: decode message list: %v", ErrUpstream, err)
	}
	if data.Ret != 0 {
		if data.Ret == -3 || data.Ret == -6 {
			_ = s.markAccountInvalid(biz, data.ErrMsg)
			return nil, fmt.Errorf("%w: %s", ErrAccountExpired, firstNonEmpty(data.ErrMsg, "credential rejected"))
		}
		return nil, fmt.Errorf("%w: %s", ErrUpstream, firstNonEmpty(data.ErrMsg, "message list request rejected"))
	}
	data.List, data.Articles, err = parseMessageList(data.GeneralMsgList)
	if err != nil {
		return nil, fmt.Errorf("%w: decode general_msg_list: %v", ErrUpstream, err)
	}
	return &data, nil
}

func (s *Service) FetchArticleList(biz string) (*ArticleListResponse, error) {
	biz = strings.TrimSpace(biz)
	if biz == "" {
		return nil, ErrMissingBiz
	}
	account, ok := s.accountSnapshot(biz)
	if !ok {
		return nil, ErrAccountNotFound
	}
	if account.AuthorID == "" {
		return nil, ErrMissingAuthorID
	}
	if account.Cookie == "" || s.now().Unix() >= account.CookieExpiration {
		if err := s.fetchCookie(account); err != nil {
			return nil, err
		}
		account, _ = s.accountSnapshot(biz)
	}

	base := s.upstream()
	u, _ := url.Parse(base + "/mp/author")
	q := u.Query()
	q.Set("action", "get_articles")
	q.Set("author_id", account.AuthorID)
	q.Set("scene", "142")
	q.Set("limit", "30")
	q.Set("version", "undefined")
	q.Set("appmsg_token", account.AppmsgToken)
	q.Set("x5", "0")
	q.Set("f", "json")
	q.Set("user_article_role", "0")
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build article list request: %w", err)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", s.buildAuthorURL(account))
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if account.Cookie != "" {
		req.Header.Set("Cookie", account.Cookie)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}
	var data ArticleListResponse
	if err := decodeJSONBody(resp.Body, &data); err != nil {
		return nil, fmt.Errorf("%w: decode article list: %v", ErrUpstream, err)
	}
	if data.Ret != 0 || data.BaseResp.Ret != 0 {
		return nil, fmt.Errorf("%w: %s", ErrUpstream, firstNonEmpty(data.ErrMsg, "article list request rejected"))
	}
	return &data, nil
}

func (s *Service) fetchCookie(account Account) error {
	u := s.buildAuthorURL(account)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build cookie request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := s.client().Do(req)
	if err != nil {
		return fmt.Errorf("%w: fetch account cookie: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: cookie status %d", ErrUpstream, resp.StatusCode)
	}
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return fmt.Errorf("%w: no account cookie returned", ErrUpstream)
	}
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}

	s.mu.Lock()
	current, ok := s.accounts[account.Biz]
	if ok && current != nil {
		current.Cookie = strings.Join(parts, "; ")
		current.CookieExpiration = s.now().Add(24 * time.Hour).Unix()
		_ = s.saveLocked()
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) markAccountInvalid(biz, message string) error {
	s.mu.Lock()
	account, ok := s.accounts[biz]
	if !ok || account == nil {
		s.mu.Unlock()
		return ErrAccountNotFound
	}
	account.IsEffective = false
	account.Error = message
	saved := s.saveLocked()
	snapshot := *account
	s.mu.Unlock()
	if saved != nil {
		return saved
	}
	return s.persistCatalogAccount(snapshot)
}

func (s *Service) buildMsgListURL(account Account, offset int) string {
	u, _ := url.Parse(s.upstream() + "/mp/profile_ext")
	q := u.Query()
	q.Set("action", "getmsg")
	q.Set("__biz", account.Biz)
	q.Set("uin", account.Uin)
	q.Set("key", account.Key)
	q.Set("pass_ticket", account.PassTicket)
	q.Set("wxtoken", "")
	q.Set("x5", "0")
	q.Set("count", "10")
	q.Set("offset", strconv.Itoa(offset))
	q.Set("f", "json")
	if account.AppmsgToken != "" {
		q.Set("appmsg_token", account.AppmsgToken)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Service) buildProfileURL(account Account) string {
	u, _ := url.Parse(s.upstream() + "/mp/profile_ext")
	q := u.Query()
	q.Set("action", "home")
	q.Set("__biz", account.Biz)
	q.Set("scene", "124")
	q.Set("uin", account.Uin)
	q.Set("key", account.Key)
	q.Set("pass_ticket", account.PassTicket)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Service) buildAuthorURL(account Account) string {
	u, _ := url.Parse(s.upstream() + "/mp/author")
	q := u.Query()
	q.Set("action", "show")
	q.Set("__biz", account.Biz)
	q.Set("idx", "1")
	q.Set("author_id", account.AuthorID)
	q.Set("scene", "142")
	q.Set("rscene", "128")
	q.Set("uin", account.Uin)
	q.Set("key", account.Key)
	q.Set("devicetype", "UnifiedPCWindows")
	q.Set("version", "f2640619")
	q.Set("lang", "zh_CN")
	q.Set("ascene", "1")
	q.Set("acctmode", "0")
	q.Set("pass_ticket", account.PassTicket)
	u.RawQuery = q.Encode()
	return u.String()
}

func parseMessageList(raw string) ([]MessageItem, []ArticleItem, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil, nil
	}
	var envelope struct {
		List []MessageItem `json:"list"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, nil, err
	}
	articles := make([]ArticleItem, 0, len(envelope.List))
	for _, item := range envelope.List {
		msg := item.MsgExtInfo
		published := int64(item.CommonMsgInfo.Datetime)
		parent := ArticleItem{
			Title:                  msg.Title,
			Digest:                 msg.Digest,
			Content:                msg.Content,
			FileID:                 msg.FileID,
			VideoID:                strings.TrimSpace(msg.VideoID),
			ContentURL:             msg.ContentURL,
			SourceURL:              msg.SourceURL,
			Cover:                  msg.Cover,
			Author:                 msg.Author,
			Subtype:                msg.Subtype,
			IsMulti:                msg.IsMulti,
			IsOriginal:             msg.IsOriginal,
			IsPaid:                 msg.IsPaid,
			IsPaySubscribe:         msg.IsPaySubscribe,
			ItemShowType:           msg.ItemShowType,
			CopyrightStat:          msg.CopyrightStat,
			Duration:               msg.Duration,
			AudioFileID:            msg.AudioFileID,
			PlayURL:                msg.PlayURL,
			MaliciousTitleReasonID: msg.MaliciousTitleReasonID,
			MaliciousContentType:   msg.MaliciousContentType,
			DelFlag:                msg.DelFlag,
			PublishTime:            published,
		}
		articles = append(articles, parent)
		for _, child := range msg.MultiAppMsgItemList {
			child.PublishTime = published
			articles = append(articles, child)
		}
	}
	return envelope.List, articles, nil
}

func decodeJSONBody(reader io.Reader, target interface{}) error {
	decoder := json.NewDecoder(io.LimitReader(reader, maxJSONBody))
	return decoder.Decode(target)
}

func (s *Service) client() *http.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.httpClient != nil {
		return s.httpClient
	}
	return http.DefaultClient
}

func (s *Service) upstream() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.upstreamBaseURL == "" {
		return "https://mp.weixin.qq.com"
	}
	return s.upstreamBaseURL
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func normalizeArticleURL(raw string) string {
	return html.UnescapeString(strings.TrimSpace(raw))
}
