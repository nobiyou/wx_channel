package officialaccount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const maxCommentPageBody = 8 << 20

type articleCommentIdentity struct {
	AppmsgID  string
	Idx       string
	CommentID string
}

var articleCommentFieldPattern = regexp.MustCompile(`(?is)(?:^|[^[:alnum:]_-])["']?(comment_id|commentId|segment_comment_id|segmentCommentId|extra_comment_id|extraCommentId|appmsgid|appmsg_id|appmsgId|mid|idx)["']?\s*(?::|=)\s*(?:"([^"]*)"|'([^']*)'|([^,;}\s]+))`)

// extractArticleCommentIdentity reads the short-lived identifiers embedded in
// the article page. They are deliberately kept in memory and never persisted.
func extractArticleCommentIdentity(pageHTML string, article ArticleRecord) (articleCommentIdentity, bool) {
	var identity articleCommentIdentity
	commentPriority := 0
	appmsgPriority := 0
	for _, match := range articleCommentFieldPattern.FindAllStringSubmatch(pageHTML, -1) {
		if len(match) < 5 {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(match[1]), "_", ""))
		value := firstNonEmpty(match[2], match[3], match[4])
		if value == "" {
			continue
		}
		switch key {
		case "commentid":
			if commentPriority < 3 {
				identity.CommentID = value
				commentPriority = 3
			}
		case "segmentcommentid":
			if commentPriority < 2 {
				identity.CommentID = value
				commentPriority = 2
			}
		case "extracommentid":
			if commentPriority < 1 {
				identity.CommentID = value
				commentPriority = 1
			}
		case "appmsgid":
			if appmsgPriority < 2 {
				identity.AppmsgID = value
				appmsgPriority = 2
			}
		case "mid":
			if appmsgPriority < 1 {
				identity.AppmsgID = value
				appmsgPriority = 1
			}
		case "idx":
			if identity.Idx == "" {
				identity.Idx = value
			}
		}
	}

	for _, raw := range []string{article.ContentURL, article.SourceURL} {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		query := parsed.Query()
		if identity.AppmsgID == "" {
			identity.AppmsgID = firstNonEmpty(query.Get("appmsgid"), query.Get("appmsg_id"), query.Get("mid"))
		}
		if identity.Idx == "" {
			identity.Idx = query.Get("idx")
		}
		if identity.CommentID == "" {
			identity.CommentID = firstNonEmpty(query.Get("comment_id"), query.Get("commentid"))
		}
	}
	if identity.AppmsgID == "" {
		identity.AppmsgID = strings.TrimSpace(article.Mid)
	}
	if identity.Idx == "" && article.Idx > 0 {
		identity.Idx = strconv.Itoa(article.Idx)
	}
	if identity.Idx == "" {
		identity.Idx = "1"
	}
	return identity, identity.AppmsgID != "" && identity.CommentID != ""
}

func (s *Service) fetchArticleCommentCount(ctx context.Context, account Account, article ArticleRecord) (ArticleMetricPayload, string, error) {
	pageRequest, err := buildArticlePageRequest(ctx, s.upstream(), account, article)
	if err != nil {
		return ArticleMetricPayload{}, "", err
	}
	pageResponse, err := s.client().Do(pageRequest)
	if err != nil {
		return ArticleMetricPayload{}, "", fmt.Errorf("fetch article page for comments: %w", err)
	}
	defer pageResponse.Body.Close()
	if pageResponse.StatusCode < http.StatusOK || pageResponse.StatusCode >= http.StatusMultipleChoices {
		return ArticleMetricPayload{}, "", fmt.Errorf("article page status %d", pageResponse.StatusCode)
	}
	pageBody, err := io.ReadAll(io.LimitReader(pageResponse.Body, maxCommentPageBody+1))
	if err != nil {
		return ArticleMetricPayload{}, "", fmt.Errorf("read article page for comments: %w", err)
	}
	if len(pageBody) > maxCommentPageBody {
		return ArticleMetricPayload{}, "", fmt.Errorf("article page for comments is too large")
	}
	identity, ok := extractArticleCommentIdentity(string(pageBody), article)
	if !ok {
		return ArticleMetricPayload{}, "", errors.New("article page did not expose comment identity")
	}

	commentRequest, err := buildArticleCommentRequest(ctx, s.upstream(), account, article, identity)
	if err != nil {
		return ArticleMetricPayload{}, "", err
	}
	commentResponse, err := s.client().Do(commentRequest)
	if err != nil {
		return ArticleMetricPayload{}, "", fmt.Errorf("fetch article comments: %w", err)
	}
	defer commentResponse.Body.Close()
	if commentResponse.StatusCode < http.StatusOK || commentResponse.StatusCode >= http.StatusMultipleChoices {
		return ArticleMetricPayload{}, "", fmt.Errorf("article comments status %d", commentResponse.StatusCode)
	}
	commentBody, err := io.ReadAll(io.LimitReader(commentResponse.Body, maxMetricResponseBody+1))
	if err != nil {
		return ArticleMetricPayload{}, "", fmt.Errorf("read article comments: %w", err)
	}
	if len(commentBody) > maxMetricResponseBody {
		return ArticleMetricPayload{}, "", errors.New("article comments response is too large")
	}
	var envelope struct {
		Ret      int    `json:"ret"`
		ErrMsg   string `json:"errmsg"`
		BaseResp struct {
			Ret int `json:"ret"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(commentBody, &envelope); err != nil {
		return ArticleMetricPayload{}, "", fmt.Errorf("decode article comments: %w", err)
	}
	ret := envelope.Ret
	if ret == 0 {
		ret = envelope.BaseResp.Ret
	}
	if ret != 0 {
		if ret == -3 || ret == -6 {
			_ = s.markAccountInvalid(account.Biz, envelope.ErrMsg)
			return ArticleMetricPayload{}, "", fmt.Errorf("%w: %s", ErrAccountExpired, firstNonEmpty(envelope.ErrMsg, "credential rejected"))
		}
		return ArticleMetricPayload{}, "", fmt.Errorf("%w: %s", ErrUpstream, firstNonEmpty(envelope.ErrMsg, "comment request rejected"))
	}
	payload, raw := ExtractArticleMetricsJSON(string(commentBody))
	if payload.CommentCount == nil {
		return ArticleMetricPayload{}, raw, errors.New("article comments response did not expose total")
	}
	return payload, raw, nil
}

func buildArticlePageRequest(ctx context.Context, baseURL string, account Account, article ArticleRecord) (*http.Request, error) {
	target, err := buildArticlePageURL(baseURL, account, article)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", normalizeArticleURL(firstNonEmpty(article.ContentURL, article.SourceURL)))
	req.Header.Set("User-Agent", defaultUserAgent)
	if account.Cookie != "" {
		req.Header.Set("Cookie", account.Cookie)
	}
	return req, nil
}

func buildArticlePageURL(baseURL string, account Account, article ArticleRecord) (string, error) {
	rawArticleURL := firstNonEmpty(article.ContentURL, article.SourceURL)
	if rawArticleURL == "" {
		return "", ErrMetricArticleUnfetchable
	}
	target, err := url.Parse(normalizeArticleURL(rawArticleURL))
	if err != nil {
		return "", fmt.Errorf("parse article URL: %w", err)
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Host == "" {
		return "", fmt.Errorf("parse article upstream URL: %w", err)
	}
	if target.Scheme == "" {
		target.Scheme = base.Scheme
	}
	if target.Host == "" || !strings.EqualFold(target.Host, base.Host) {
		target.Scheme = base.Scheme
		target.Host = base.Host
	}
	query := target.Query()
	query.Set("__biz", account.Biz)
	query.Set("uin", account.Uin)
	query.Set("key", account.Key)
	query.Set("pass_ticket", account.PassTicket)
	query.Set("wxtoken", "")
	query.Set("x5", "0")
	if account.AppmsgToken != "" {
		query.Set("appmsg_token", account.AppmsgToken)
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func buildArticleCommentRequest(ctx context.Context, baseURL string, account Account, article ArticleRecord, identity articleCommentIdentity) (*http.Request, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/mp/appmsg_comment")
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("action", "getcomment")
	query.Set("__biz", account.Biz)
	query.Set("scene", "0")
	query.Set("uin", account.Uin)
	query.Set("key", account.Key)
	query.Set("pass_ticket", account.PassTicket)
	query.Set("wxtoken", "")
	query.Set("x5", "0")
	query.Set("f", "json")
	query.Set("devicetype", "UnifiedPCWindows")
	query.Set("clientversion", metricClientVersion)
	query.Set("appmsgid", identity.AppmsgID)
	query.Set("idx", identity.Idx)
	query.Set("comment_id", identity.CommentID)
	query.Set("offset", "0")
	query.Set("limit", "100")
	if account.AppmsgToken != "" {
		query.Set("appmsg_token", account.AppmsgToken)
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", normalizeArticleURL(firstNonEmpty(article.ContentURL, article.SourceURL)))
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if account.Cookie != "" {
		req.Header.Set("Cookie", account.Cookie)
	}
	return req, nil
}

func mergeMetricEvidenceRaw(primary, secondary string) string {
	merged := make(map[string]json.RawMessage)
	for _, raw := range []string{primary, secondary} {
		var values map[string]json.RawMessage
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
			continue
		}
		for key, value := range values {
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		return firstNonEmpty(primary, secondary)
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return firstNonEmpty(primary, secondary)
	}
	return string(data)
}
