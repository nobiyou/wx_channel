package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/response"
	"wx_channel/internal/utils"
	"wx_channel/internal/websocket"
)

// SearchService 搜索服务
type SearchService struct {
	hub                 *websocket.Hub
	callAPI             func(key string, body interface{}, timeout time.Duration) ([]byte, error)
	callAPIContext      func(context.Context, string, interface{}, time.Duration) ([]byte, error)
	resolveDownloadsDir func() (string, error)
	runtimeDiagnostics  *RuntimeDiagnostics
	commentJobsMu       sync.RWMutex
	commentJobs         *CommentExportJobManager
}

// NewSearchService 创建搜索服务
func NewSearchService(hub *websocket.Hub) *SearchService {
	service := &SearchService{hub: hub}
	service.callAPI = service.defaultCallAPI
	service.callAPIContext = service.defaultCallAPIContext
	service.resolveDownloadsDir = func() (string, error) {
		return config.Get().GetResolvedDownloadsDir()
	}
	return service
}

func (s *SearchService) SetRuntimeDiagnostics(diagnostics *RuntimeDiagnostics) {
	s.runtimeDiagnostics = diagnostics
}

func (s *SearchService) defaultCallAPI(key string, body interface{}, timeout time.Duration) ([]byte, error) {
	return s.hub.CallAPI(key, body, timeout)
}

func (s *SearchService) defaultCallAPIContext(ctx context.Context, key string, body interface{}, timeout time.Duration) ([]byte, error) {
	return s.hub.CallAPIContext(ctx, key, body, timeout)
}

func (s *SearchService) callAPIWithContext(ctx context.Context, key string, body interface{}, timeout time.Duration) ([]byte, error) {
	if s.callAPIContext != nil {
		return s.callAPIContext(ctx, key, body, timeout)
	}
	if s.callAPI != nil {
		return s.callAPI(key, body, timeout)
	}
	return nil, fmt.Errorf("search API caller is not configured")
}

func (s *SearchService) ensureCommentExportJobs() *CommentExportJobManager {
	s.commentJobsMu.Lock()
	defer s.commentJobsMu.Unlock()
	if s.commentJobs == nil {
		s.commentJobs = NewCommentExportJobManager(s)
	}
	return s.commentJobs
}

// SearchContactRequest 搜索账号请求参数
type SearchContactRequest struct {
	Keyword    string `json:"keyword"`
	Type       int    `json:"type"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	NextMarker string `json:"next_marker"`
}

// SearchContact 搜索账号
func (s *SearchService) SearchContact(w http.ResponseWriter, r *http.Request) {
	var req SearchContactRequest

	// 支持 GET 和 POST
	if r.Method == http.MethodGet {
		req.Keyword = r.URL.Query().Get("keyword")
		req.Type, _ = strconv.Atoi(r.URL.Query().Get("type"))
		req.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		req.PageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
		req.NextMarker = r.URL.Query().Get("next_marker")
	} else if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, 400, "Invalid request body")
			return
		}
	}

	// 参数校验
	if req.Keyword == "" {
		response.Error(w, 400, "keyword is required")
		return
	}
	if len(req.Keyword) > 100 {
		response.Error(w, 400, "keyword too long (max 100 characters)")
		return
	}

	// 默认分页
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 50 {
		req.PageSize = 20
	}

	// 账号搜索入口固定使用 finderSearch 的账号场景，视频搜索请走 /api/channels/feed/search。
	req.Type = 1

	// 调用前端 API
	body := websocket.SearchContactBody{
		Keyword:    req.Keyword,
		Type:       req.Type,
		NextMarker: req.NextMarker,
	}

	data, err := s.callAPI("key:channels:contact_list", body, 60*time.Second)
	if err != nil {
		if strings.Contains(err.Error(), "no available client") || strings.Contains(err.Error(), "no ready client") {
			response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "No ready WeChat page is available for search. Please open a supported page and wait for API initialization.")
			return
		}
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 解析返回数据以支持分页
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		response.Success(w, json.RawMessage(data))
		return
	}

	response.Success(w, result)
}

// SearchFeed 搜索视频，复用视频号客户端 finderSearch 的 Type=3 场景。
func (s *SearchService) SearchFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var req SearchContactRequest
		req.Keyword = r.URL.Query().Get("keyword")
		req.Type = 3
		req.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		req.PageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
		req.NextMarker = r.URL.Query().Get("next_marker")
		data, err := s.searchWithRequest(req)
		if err != nil {
			writeSearchError(w, err)
			return
		}
		response.Success(w, data)
		return
	}
	if r.Method == http.MethodPost {
		var req SearchContactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, 400, "Invalid request body")
			return
		}
		req.Type = 3
		data, err := s.searchWithRequest(req)
		if err != nil {
			writeSearchError(w, err)
			return
		}
		response.Success(w, data)
		return
	}
	s.SearchContact(w, r)
}

func (s *SearchService) searchWithRequest(req SearchContactRequest) (interface{}, error) {
	if req.Keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}
	if len(req.Keyword) > 100 {
		return nil, fmt.Errorf("keyword too long (max 100 characters)")
	}
	body := websocket.SearchContactBody{
		Keyword:    req.Keyword,
		Type:       req.Type,
		NextMarker: req.NextMarker,
	}
	data, err := s.callAPI("key:channels:contact_list", body, 60*time.Second)
	if err != nil {
		return nil, err
	}
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return json.RawMessage(data), nil
	}
	return result, nil
}

func writeSearchError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "keyword is required") || strings.Contains(err.Error(), "keyword too long") {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.Contains(err.Error(), "no available client") || strings.Contains(err.Error(), "no ready client") {
		response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "No ready WeChat page is available for search. Please open a supported page and wait for API initialization.")
		return
	}
	response.Error(w, http.StatusInternalServerError, err.Error())
}

// GetFeedListRequest 获取视频列表请求参数
type GetFeedListRequest struct {
	Username   string `json:"username"`
	NextMarker string `json:"next_marker"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

// GetFeedList 获取账号的视频列表
func (s *SearchService) GetFeedList(w http.ResponseWriter, r *http.Request) {
	var req GetFeedListRequest

	if r.Method == http.MethodGet {
		req.Username = r.URL.Query().Get("username")
		req.NextMarker = r.URL.Query().Get("next_marker")
		req.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		req.PageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))
	} else if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, 400, "Invalid request body")
			return
		}
	}

	if req.Username == "" {
		response.Error(w, 400, "username is required")
		return
	}

	// 默认分页
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 50 {
		req.PageSize = 20
	}

	// 调用前端 API
	body := websocket.FeedListBody{
		Username:   req.Username,
		NextMarker: req.NextMarker,
	}

	data, err := s.callAPI("key:channels:feed_list", body, 60*time.Second)
	if err != nil {
		if strings.Contains(err.Error(), "no available client") || strings.Contains(err.Error(), "no ready client") {
			response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "No ready WeChat page is available for feed list.")
			return
		}
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		response.Success(w, json.RawMessage(data))
		return
	}

	response.Success(w, result)
}

// GetFeedProfileRequest 获取视频详情请求参数
type GetFeedProfileRequest struct {
	ObjectID string `json:"object_id"`
	NonceID  string `json:"nonce_id"`
	URL      string `json:"url"`
}

func decodeGetFeedProfileRequest(r *http.Request) (GetFeedProfileRequest, error) {
	var req GetFeedProfileRequest

	if r.Method == http.MethodGet {
		req.ObjectID = r.URL.Query().Get("object_id")
		req.NonceID = r.URL.Query().Get("nonce_id")
		req.URL = r.URL.Query().Get("url")
		return req, nil
	}

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return req, err
		}
		return req, nil
	}

	return req, nil
}

func normalizeFeedProfileURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	decoded, err := url.QueryUnescape(raw)
	if err == nil {
		return decoded
	}
	return raw
}

func isSharedFeedURL(raw string) bool {
	normalized := strings.ToLower(normalizeFeedProfileURL(raw))
	if normalized == "" {
		return false
	}

	return strings.Contains(normalized, "weixin.qq.com/sph/") ||
		strings.Contains(normalized, "channels.weixin.qq.com/finder-preview/pages/sph")
}

func feedProfileAPIKey(req GetFeedProfileRequest) string {
	if isSharedFeedURL(req.URL) {
		return "key:channels:shared_feed_profile"
	}
	return "key:channels:feed_profile"
}

func (s *SearchService) fetchFeedProfile(req GetFeedProfileRequest, forceShared bool) ([]byte, error) {
	body := websocket.FeedProfileBody{
		ObjectID: req.ObjectID,
		NonceID:  req.NonceID,
		URL:      req.URL,
	}

	key := feedProfileAPIKey(req)
	if forceShared {
		key = "key:channels:shared_feed_profile"
	}

	return s.callAPI(key, body, 60*time.Second)
}

func normalizePageContextAPIError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	if strings.Contains(lower, "jsapi_jsonparse_failed") {
		return fmt.Errorf("当前页面上下文与这组视频 ID / nonce_id 不匹配。请先切到目标视频页面，等待当前视频信息完全加载后重试；如果这是旧评论导出绑定，请重新导出或重新绑定当前视频")
	}
	return err
}

func (s *SearchService) fetchSharedFeedResolveProfile(req GetFeedProfileRequest) ([]byte, error) {
	body := websocket.FeedProfileBody{
		ObjectID: req.ObjectID,
		NonceID:  req.NonceID,
		URL:      req.URL,
	}

	return s.callAPI("key:channels:shared_feed_resolve", body, 60*time.Second)
}

// GetFeedCommentListRequest 获取视频评论列表请求参数
type GetFeedCommentListRequest struct {
	ObjectID   string `json:"object_id"`
	NonceID    string `json:"nonce_id"`
	CommentID  string `json:"comment_id"`
	NextMarker string `json:"next_marker"`
}

// ExportFeedCommentsRequest 获取并保存完整评论列表请求参数
type ExportFeedCommentsRequest struct {
	ObjectID string `json:"object_id"`
	NonceID  string `json:"nonce_id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
}

// ExportFeedCommentsResult 评论导出结果
type ExportFeedCommentsResult struct {
	ObjectID      string `json:"object_id"`
	TopLevelCount int    `json:"top_level_count"`
	ReplyCount    int    `json:"reply_count"`
	TotalCount    int    `json:"total_count"`
	ReportedCount int    `json:"reported_count"`
	SavedPath     string `json:"saved_path"`
	RelativePath  string `json:"relative_path"`
	Title         string `json:"title"`
	Author        string `json:"author"`
	Source        string `json:"source"`
}

type feedCommentAPIResponse struct {
	ErrCode int                `json:"errCode"`
	ErrMsg  string             `json:"errMsg"`
	Data    feedCommentAPIData `json:"data"`
}

type feedCommentAPIData struct {
	CommentInfo []map[string]interface{} `json:"commentInfo"`
	CountInfo   struct {
		CommentCount int `json:"commentCount"`
	} `json:"countInfo"`
	LastBuffer string `json:"lastBuffer"`
}

type commentExportFile struct {
	ObjectID             string                     `json:"objectId"`
	ObjectNonceID        string                     `json:"objectNonceId"`
	Title                string                     `json:"title"`
	Author               string                     `json:"author"`
	CommentInfo          []formattedCommentEntry    `json:"commentInfo"`
	CountInfo            feedCommentExportCountInfo `json:"countInfo"`
	LastBuffer           string                     `json:"lastBuffer"`
	UpContinueFlag       int                        `json:"upContinueFlag"`
	DownContinueFlag     int                        `json:"downContinueFlag"`
	OriginalCommentCount int                        `json:"originalCommentCount"`
	SavedAt              string                     `json:"savedAt"`
	Source               string                     `json:"source"`
}

type formattedCommentEntry struct {
	Username           string                  `json:"username"`
	Nickname           string                  `json:"nickname"`
	Content            string                  `json:"content"`
	CommentID          string                  `json:"commentId"`
	ReplyCommentID     string                  `json:"replyCommentId"`
	HeadURL            string                  `json:"headUrl,omitempty"`
	LevelTwoComment    []formattedCommentEntry `json:"levelTwoComment"`
	Createtime         string                  `json:"createtime,omitempty"`
	LikeFlag           int                     `json:"likeFlag"`
	LikeCount          int                     `json:"likeCount"`
	ExpandCommentCount int                     `json:"expandCommentCount"`
	LastBuffer         string                  `json:"lastBuffer,omitempty"`
	ContinueFlag       int                     `json:"continueFlag"`
	DisplayFlag        int                     `json:"displayFlag"`
	ReplyContent       string                  `json:"replyContent,omitempty"`
	UpContinueFlag     int                     `json:"upContinueFlag"`
	ExtFlag            int                     `json:"extFlag"`
	ContentType        int                     `json:"contentType"`
	ReportJSON         string                  `json:"reportJson,omitempty"`
	DislikeCount       int                     `json:"dislikeCount"`
	IPRegionInfo       formattedIPRegionInfo   `json:"ipRegionInfo"`
}

type formattedIPRegionInfo struct {
	RegionText string `json:"regionText"`
}

type feedCommentExportCountInfo struct {
	CommentCount int `json:"commentCount"`
}

// GetFeedProfile 获取视频详情
func (s *SearchService) GetFeedProfile(w http.ResponseWriter, r *http.Request) {
	req, err := decodeGetFeedProfileRequest(r)
	if err != nil {
		response.Error(w, 400, "Invalid request body")
		return
	}

	if req.ObjectID == "" && req.URL == "" {
		response.Error(w, 400, "object_id or url is required")
		return
	}

	data, err := s.fetchFeedProfile(req, false)
	if err != nil {
		err = normalizePageContextAPIError(err)
		if strings.Contains(err.Error(), "no available client") || strings.Contains(err.Error(), "no ready client") {
			response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "No ready WeChat page is available for feed profile.")
			return
		}
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		response.Success(w, json.RawMessage(data))
		return
	}

	response.Success(w, result)
}

// GetSharedFeedProfile 获取分享链接视频详情
func (s *SearchService) GetSharedFeedProfile(w http.ResponseWriter, r *http.Request) {
	req, err := decodeGetFeedProfileRequest(r)
	if err != nil {
		response.Error(w, 400, "Invalid request body")
		return
	}

	if req.URL == "" {
		response.Error(w, 400, "url is required")
		return
	}

	data, err := s.fetchFeedProfile(req, true)
	if err != nil {
		err = normalizePageContextAPIError(err)
		if strings.Contains(err.Error(), "no available client") || strings.Contains(err.Error(), "no ready client") {
			response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "No ready WeChat page is available for shared feed profile.")
			return
		}
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		response.Success(w, json.RawMessage(data))
		return
	}

	response.Success(w, result)
}

// GetFeedCommentList 获取视频评论列表
func (s *SearchService) GetFeedCommentList(w http.ResponseWriter, r *http.Request) {
	var req GetFeedCommentListRequest

	if r.Method == http.MethodGet {
		req.ObjectID = r.URL.Query().Get("object_id")
		if req.ObjectID == "" {
			req.ObjectID = r.URL.Query().Get("oid")
		}
		req.NonceID = r.URL.Query().Get("nonce_id")
		if req.NonceID == "" {
			req.NonceID = r.URL.Query().Get("nid")
		}
		req.CommentID = r.URL.Query().Get("comment_id")
		req.NextMarker = r.URL.Query().Get("next_marker")
	} else if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, 400, "Invalid request body")
			return
		}
	}

	if req.ObjectID == "" {
		response.Error(w, 400, "object_id is required")
		return
	}
	if req.NonceID == "" && req.CommentID == "" {
		response.Error(w, 400, "nonce_id or comment_id is required")
		return
	}

	body := websocket.FeedCommentListBody{
		ObjectID:   req.ObjectID,
		NonceID:    req.NonceID,
		CommentID:  req.CommentID,
		NextMarker: req.NextMarker,
	}

	data, err := s.callAPI("key:channels:fetch_feed_comment_list", body, 60*time.Second)
	if err != nil {
		err = normalizePageContextAPIError(err)
		if strings.Contains(err.Error(), "no available client") || strings.Contains(err.Error(), "no ready client") {
			response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "No ready WeChat page is available for feed comment list.")
			return
		}
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		response.Success(w, json.RawMessage(data))
		return
	}

	response.Success(w, result)
}

// ExportFeedComments 获取完整评论列表并保存到本地
func (s *SearchService) ExportFeedComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req ExportFeedCommentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ObjectID == "" {
		response.Error(w, http.StatusBadRequest, "object_id is required")
		return
	}
	if req.NonceID == "" {
		response.Error(w, http.StatusBadRequest, "nonce_id is required")
		return
	}

	// Production services use a bounded background task so the browser does not
	// have to keep a single HTTP request open for every comment/reply page.
	if s.hub != nil {
		job, err := s.ensureCommentExportJobs().Submit(req)
		if err != nil {
			response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, err.Error())
			return
		}
		response.SuccessWithStatus(w, http.StatusAccepted, job)
		return
	}

	// Keep direct service fixtures and focused unit tests synchronous.
	result, err := s.exportFeedComments(req)
	if err != nil {
		err = normalizePageContextAPIError(err)
		if strings.Contains(err.Error(), "no available client") || strings.Contains(err.Error(), "no ready client") {
			response.ErrorWithStatus(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "No ready WeChat page is available for feed comment export.")
			return
		}
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(w, result)
}

// CommentExportStatus returns the state of a background comment export.
func (s *SearchService) CommentExportStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID == "" {
		response.Error(w, http.StatusBadRequest, "job_id is required")
		return
	}
	s.commentJobsMu.RLock()
	manager := s.commentJobs
	s.commentJobsMu.RUnlock()
	if manager == nil {
		response.ErrorWithStatus(w, http.StatusNotFound, http.StatusNotFound, "comment export job not found")
		return
	}
	job, ok := manager.Get(jobID)
	if !ok {
		response.ErrorWithStatus(w, http.StatusNotFound, http.StatusNotFound, "comment export job not found")
		return
	}
	response.Success(w, job)
}

func (s *SearchService) exportFeedComments(req ExportFeedCommentsRequest) (*ExportFeedCommentsResult, error) {
	return s.exportFeedCommentsContext(context.Background(), req, nil)
}

func (s *SearchService) exportFeedCommentsContext(ctx context.Context, req ExportFeedCommentsRequest, onProgress func(CommentExportProgress)) (*ExportFeedCommentsResult, error) {
	downloadsDir, err := s.resolveDownloadsDir()
	if err != nil {
		return nil, err
	}

	persistence, err := newCommentExportPersistence(downloadsDir, req)
	if err != nil {
		return nil, err
	}

	topLevelComments, reportedCount, err := s.fetchCommentPagesContext(ctx, req.ObjectID, req.NonceID, "")
	if err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress(CommentExportProgress{
			Stage:         "top_level_comments",
			TopLevelCount: len(topLevelComments),
			ReportedCount: reportedCount,
		})
	}
	if err := persistence.SaveCheckpoint(req, topLevelComments, reportedCount, 0, "finderGetCommentList.partial"); err != nil {
		return nil, err
	}

	replyCount := 0
	for _, comment := range topLevelComments {
		commentID := stringValue(comment["commentId"])
		if commentID == "" || !commentHasReplies(comment) {
			continue
		}

		replies, _, err := s.fetchCommentPagesContext(ctx, req.ObjectID, "", commentID)
		if err != nil {
			return nil, err
		}
		comment["levelTwoComment"] = replies
		replyCount += len(replies)
		if onProgress != nil {
			onProgress(CommentExportProgress{
				Stage:            "replies",
				TopLevelCount:    len(topLevelComments),
				ReplyCount:       replyCount,
				ReportedCount:    reportedCount,
				CompletedReplies: replyCount,
				TotalReplies:     countReplyTargets(topLevelComments),
			})
		}

		if err := persistence.SaveCheckpoint(req, topLevelComments, reportedCount, replyCount, "finderGetCommentList.partial"); err != nil {
			return nil, err
		}
	}

	savedPath, relativePath, err := persistence.Finalize(req, topLevelComments, reportedCount, replyCount, "finderGetCommentList")
	if err != nil {
		return nil, err
	}

	result := &ExportFeedCommentsResult{
		ObjectID:      req.ObjectID,
		TopLevelCount: len(topLevelComments),
		ReplyCount:    replyCount,
		TotalCount:    len(topLevelComments) + replyCount,
		ReportedCount: reportedCount,
		SavedPath:     savedPath,
		RelativePath:  relativePath,
		Title:         req.Title,
		Author:        req.Author,
		Source:        "finderGetCommentList",
	}

	utils.LogComment(req.ObjectID, req.Title, result.TotalCount, true)
	return result, nil
}

func (s *SearchService) fetchCommentPages(objectID, nonceID, commentID string) ([]map[string]interface{}, int, error) {
	return s.fetchCommentPagesContext(context.Background(), objectID, nonceID, commentID)
}

func (s *SearchService) fetchCommentPagesContext(ctx context.Context, objectID, nonceID, commentID string) ([]map[string]interface{}, int, error) {
	items := make([]map[string]interface{}, 0, 32)
	seen := make(map[string]struct{})
	nextMarker := ""
	reportedCount := 0
	const maxCommentPages = 1000
	const maxCommentsPerBranch = 100000
	pageCount := 0

	for {
		pageCount++
		if pageCount > maxCommentPages {
			return nil, 0, fmt.Errorf("comment pagination exceeded %d pages", maxCommentPages)
		}
		body := websocket.FeedCommentListBody{
			ObjectID:   objectID,
			NonceID:    nonceID,
			CommentID:  commentID,
			NextMarker: nextMarker,
		}

		raw, err := s.callAPIWithContext(ctx, "key:channels:fetch_feed_comment_list", body, 60*time.Second)
		if err != nil {
			return nil, 0, err
		}

		var resp feedCommentAPIResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, 0, fmt.Errorf("parse feed comment list failed: %w", err)
		}
		if resp.ErrCode != 0 {
			if resp.ErrMsg == "" {
				resp.ErrMsg = "unknown error"
			}
			return nil, 0, fmt.Errorf("feed comment list failed: %s", resp.ErrMsg)
		}

		if resp.Data.CountInfo.CommentCount > 0 {
			reportedCount = resp.Data.CountInfo.CommentCount
		}

		pageNewCount := 0
		for _, item := range resp.Data.CommentInfo {
			commentKey := stringValue(item["commentId"])
			if commentKey == "" {
				commentKey = fmt.Sprintf("idx-%d", len(items))
			}
			if _, exists := seen[commentKey]; exists {
				continue
			}
			seen[commentKey] = struct{}{}
			if _, ok := item["levelTwoComment"]; !ok {
				item["levelTwoComment"] = []map[string]interface{}{}
			}
			items = append(items, item)
			if len(items) > maxCommentsPerBranch {
				return nil, 0, fmt.Errorf("comment export exceeded %d comments in one branch", maxCommentsPerBranch)
			}
			pageNewCount++
		}

		if resp.Data.LastBuffer == "" || pageNewCount == 0 {
			break
		}
		nextMarker = resp.Data.LastBuffer
	}

	return items, reportedCount, nil
}

func countReplyTargets(comments []map[string]interface{}) int {
	count := 0
	for _, comment := range comments {
		if commentHasReplies(comment) {
			count++
		}
	}
	return count
}

func commentHasReplies(comment map[string]interface{}) bool {
	if intValue(comment["expandCommentCount"]) > 0 {
		return true
	}
	if intValue(comment["continueFlag"]) > 0 {
		return true
	}
	existingReplies, ok := comment["levelTwoComment"].([]interface{})
	return ok && len(existingReplies) > 0
}

func stringValue(v interface{}) string {
	switch value := v.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case float64:
		return strconv.FormatInt(int64(value), 10)
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprintf("%v", value)
	}
}

func intValue(v interface{}) int {
	switch value := v.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	default:
		return 0
	}
}

func formatCommentsForExport(comments []map[string]interface{}) []formattedCommentEntry {
	formatted := make([]formattedCommentEntry, 0, len(comments))
	for _, comment := range comments {
		formatted = append(formatted, formatCommentEntry(comment))
	}
	return formatted
}

func formatCommentEntry(comment map[string]interface{}) formattedCommentEntry {
	entry := formattedCommentEntry{
		Username:           stringValue(comment["username"]),
		Nickname:           stringValue(comment["nickname"]),
		Content:            stringValue(comment["content"]),
		CommentID:          stringValue(comment["commentId"]),
		ReplyCommentID:     stringValue(comment["replyCommentId"]),
		HeadURL:            stringValue(comment["headUrl"]),
		LevelTwoComment:    []formattedCommentEntry{},
		Createtime:         stringValue(comment["createtime"]),
		LikeFlag:           intValue(comment["likeFlag"]),
		LikeCount:          intValue(comment["likeCount"]),
		ExpandCommentCount: intValue(comment["expandCommentCount"]),
		LastBuffer:         stringValue(comment["lastBuffer"]),
		ContinueFlag:       intValue(comment["continueFlag"]),
		DisplayFlag:        intValue(comment["displayFlag"]),
		ReplyContent:       stringValue(comment["replyContent"]),
		UpContinueFlag:     intValue(comment["upContinueFlag"]),
		ExtFlag:            intValue(comment["extFlag"]),
		ContentType:        intValue(comment["contentType"]),
		ReportJSON:         stringValue(comment["reportJson"]),
		DislikeCount:       intValue(comment["dislikeCount"]),
		IPRegionInfo: formattedIPRegionInfo{
			RegionText: extractRegionText(comment["ipRegionInfo"], comment["ipRegion"]),
		},
	}

	replyMaps := extractReplyMaps(comment["levelTwoComment"])
	if len(replyMaps) > 0 {
		entry.LevelTwoComment = formatCommentsForExport(replyMaps)
		entry.ExpandCommentCount = len(entry.LevelTwoComment)
	}

	return entry
}

func extractReplyMaps(v interface{}) []map[string]interface{} {
	switch replies := v.(type) {
	case []map[string]interface{}:
		return replies
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(replies))
		for _, item := range replies {
			replyMap, ok := item.(map[string]interface{})
			if ok {
				result = append(result, replyMap)
			}
		}
		return result
	default:
		return nil
	}
}

func formatCommentTime(v interface{}) string {
	raw := stringValue(v)
	if raw == "" {
		return ""
	}

	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return raw
	}

	return time.Unix(seconds, 0).In(time.Local).Format("2006-01-02 15:04:05")
}

func extractRegionText(ipRegionInfo interface{}, fallback interface{}) string {
	if info, ok := ipRegionInfo.(map[string]interface{}); ok {
		if region := stringValue(info["regionText"]); region != "" {
			return region
		}
	}
	return stringValue(fallback)
}

// GetStatus 获取 WebSocket 连接状态
func (s *SearchService) GetStatus(w http.ResponseWriter, r *http.Request) {
	clientStatuses := s.hub.ClientStatuses()
	readyCount := 0
	searchReadyCount := 0
	feedReadyCount := 0
	profileReadyCount := 0
	commentReadyCount := 0
	for _, client := range clientStatuses {
		if client.APIReady {
			readyCount++
		}
		if client.SupportsSearch {
			searchReadyCount++
		}
		if client.SupportsFeed {
			feedReadyCount++
		}
		if client.SupportsProfile {
			profileReadyCount++
		}
		if client.SupportsComment {
			commentReadyCount++
		}
	}

	status := map[string]interface{}{
		"connected":             s.hub.ClientCount() > 0,
		"clients":               s.hub.ClientCount(),
		"ready_clients":         readyCount,
		"search_ready_clients":  searchReadyCount,
		"feed_ready_clients":    feedReadyCount,
		"profile_ready_clients": profileReadyCount,
		"comment_ready_clients": commentReadyCount,
		"client_list":           clientStatuses,
	}
	if s.runtimeDiagnostics != nil {
		status["runtime"] = s.runtimeDiagnostics.Snapshot()
	}
	response.Success(w, status)
}

// RegisterRoutes 注册搜索相关路由
func (s *SearchService) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/search/contact", s.SearchContact)
	mux.HandleFunc("/api/v1/search/feed/search", s.SearchFeed)
	mux.HandleFunc("/api/v1/search/feed", s.GetFeedList)
	mux.HandleFunc("/api/v1/search/feed/profile", s.GetFeedProfile)
	mux.HandleFunc("/api/v1/search/shared_feed/profile", s.GetSharedFeedProfile)
	mux.HandleFunc("/api/v1/search/share/resolve", s.ResolveSharedFeedLinks)
	mux.HandleFunc("/api/v1/search/feed/comments", s.GetFeedCommentList)
	mux.HandleFunc("/api/v1/search/feed/comments/export", s.ExportFeedComments)
	mux.HandleFunc("/api/v1/search/feed/comments/export/status", s.CommentExportStatus)
	mux.HandleFunc("/api/v1/status", s.GetStatus)

	// 兼容旧路由
	mux.HandleFunc("/api/search/contact", s.SearchContact)
	mux.HandleFunc("/api/search/feed/search", s.SearchFeed)
	mux.HandleFunc("/api/search/feed", s.GetFeedList)
	mux.HandleFunc("/api/search/feed/profile", s.GetFeedProfile)
	mux.HandleFunc("/api/search/shared_feed/profile", s.GetSharedFeedProfile)
	mux.HandleFunc("/api/search/share/resolve", s.ResolveSharedFeedLinks)
	mux.HandleFunc("/api/search/feed/comments", s.GetFeedCommentList)
	mux.HandleFunc("/api/search/feed/comments/export", s.ExportFeedComments)
	mux.HandleFunc("/api/search/feed/comments/export/status", s.CommentExportStatus)
	mux.HandleFunc("/api/status", s.GetStatus)

	// 兼容 /api/channels 路由 (WebSocket服务器原有的路由)
	mux.HandleFunc("/api/channels/contact/search", s.SearchContact)
	mux.HandleFunc("/api/channels/feed/search", s.SearchFeed)
	mux.HandleFunc("/api/channels/contact/feed/list", s.GetFeedList)
	mux.HandleFunc("/api/channels/feed/profile", s.GetFeedProfile)
	mux.HandleFunc("/api/channels/shared_feed/profile", s.GetSharedFeedProfile)
	mux.HandleFunc("/api/channels/share/resolve", s.ResolveSharedFeedLinks)
	mux.HandleFunc("/api/channels/feed/comment/list", s.GetFeedCommentList)
	mux.HandleFunc("/api/channels/feed/comment/export", s.ExportFeedComments)
	mux.HandleFunc("/api/channels/feed/comment/export/status", s.CommentExportStatus)
	mux.HandleFunc("/api/channels/status", s.GetStatus)
}
