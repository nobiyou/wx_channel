package poc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxLtaooResponseBytes   = 8 << 20
	profileReadinessTimeout = 30 * time.Second
	profileRetryInterval    = 500 * time.Millisecond
)

type profileReadinessOptions struct {
	Clock         Clock
	Timeout       time.Duration
	RetryInterval time.Duration
}

type LtaooClient struct {
	baseURL *url.URL
	client  *http.Client
}

func NewLtaooClient(rawBaseURL string, timeout time.Duration) (*LtaooClient, error) {
	if timeout <= 0 {
		return nil, errors.New("ltaoo timeout must be positive")
	}
	parsed, err := url.Parse(rawBaseURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, errors.New("ltaoo API base is invalid")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, errors.New("ltaoo API base path is invalid")
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil || port == "" {
		return nil, errors.New("ltaoo API base requires an explicit port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("ltaoo API base must use a literal loopback address")
	}
	parsed.Path = ""
	return &LtaooClient{
		baseURL: parsed,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("ltaoo redirects are disabled")
			},
		},
	}, nil
}

func (c *LtaooClient) Status(ctx context.Context) error {
	raw, err := c.get(ctx, "/api/status", nil)
	if err != nil {
		return err
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := decodeSingleJSON(raw, &envelope); err != nil || envelope.Code != 0 || len(envelope.Data) == 0 {
		return CategorizedError{Category: ErrorStructure}
	}
	return nil
}

func (c *LtaooClient) ResolveWork(ctx context.Context, shareURL string, rank int) (Work, error) {
	normalized, err := NormalizeWeChatShareURL(shareURL)
	if err != nil {
		return Work{}, CategorizedError{Category: ErrorSafety}
	}
	raw, err := c.get(ctx, "/api/channels/feed/profile", url.Values{"url": []string{normalized}})
	if err != nil {
		return Work{}, err
	}
	data, err := decodeLtaooProfileData(raw)
	if err != nil {
		return Work{}, err
	}
	var profile struct {
		Object map[string]any `json:"object"`
	}
	if err := decodeSingleJSON(data, &profile); err != nil || profile.Object == nil {
		return Work{}, CategorizedError{Category: ErrorStructure}
	}
	id, ok := stringField(profile.Object, "id")
	if !ok || id == "" {
		return Work{}, CategorizedError{Category: ErrorStructure}
	}
	work := mapSearchWork(profile.Object, id, "", rank, 1, rank, SourceRef{Method: "finderGetCommentDetail"})
	if work.ObjectNonceID == nil || *work.ObjectNonceID == "" {
		return Work{}, CategorizedError{Category: ErrorStructure}
	}
	work.Locator.PublicURL = stringPointer(normalized)
	return work, nil
}

func (c *LtaooClient) Call(ctx context.Context, method string, body any) ([]byte, error) {
	if method != commentListMethod {
		return nil, CategorizedError{Category: ErrorMethodMissing}
	}
	values, ok := body.(map[string]any)
	if !ok {
		return nil, CategorizedError{Category: ErrorSafety}
	}
	allowed := map[string]string{
		"object_id": "oid", "nonce_id": "nid", "comment_id": "comment_id", "next_marker": "next_marker",
	}
	query := make(url.Values, len(values))
	for key, value := range values {
		queryKey, exists := allowed[key]
		if !exists {
			return nil, CategorizedError{Category: ErrorSafety}
		}
		text, valid := requestString(value)
		if !valid {
			return nil, CategorizedError{Category: ErrorSafety}
		}
		if text != "" || key == "object_id" || key == "nonce_id" {
			query.Set(queryKey, text)
		}
	}
	if query.Get("oid") == "" || query.Get("nid") == "" {
		return nil, CategorizedError{Category: ErrorSafety}
	}
	raw, err := c.get(ctx, "/api/channels/feed/comment/list", query)
	if err != nil {
		return nil, err
	}
	data, err := decodeLtaooBusinessData(raw)
	if err != nil {
		return nil, err
	}
	wrapped := struct {
		Data json.RawMessage `json:"data"`
	}{Data: data}
	return json.Marshal(wrapped)
}

func (c *LtaooClient) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	if c == nil || c.baseURL == nil || c.client == nil {
		return nil, errors.New("ltaoo client is missing")
	}
	target := *c.baseURL
	target.Path = path
	target.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, CategorizedError{Category: ErrorSafety}
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, CategorizedError{Category: ErrorTransient}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, CategorizedError{Category: ErrorAccessDenied}
		case http.StatusTooManyRequests:
			return nil, CategorizedError{Category: ErrorRateLimited}
		default:
			if response.StatusCode >= 500 {
				return nil, CategorizedError{Category: ErrorTransient}
			}
			return nil, CategorizedError{Category: ErrorUnknown}
		}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxLtaooResponseBytes+1))
	if err != nil {
		return nil, CategorizedError{Category: ErrorTransient}
	}
	if len(raw) > maxLtaooResponseBytes {
		return nil, CategorizedError{Category: ErrorStructure}
	}
	return raw, nil
}

func decodeLtaooBusinessData(raw []byte) (json.RawMessage, error) {
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ErrCode int             `json:"errCode"`
			Data    json.RawMessage `json:"data"`
		} `json:"data"`
	}
	if err := decodeSingleJSON(raw, &envelope); err != nil || envelope.Code != 0 || envelope.Data.ErrCode != 0 || len(envelope.Data.Data) == 0 {
		return nil, CategorizedError{Category: ErrorStructure}
	}
	return envelope.Data.Data, nil
}

func decodeLtaooProfileData(raw []byte) (json.RawMessage, error) {
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := decodeSingleJSON(raw, &envelope); err != nil {
		return nil, CategorizedError{Category: ErrorStructure}
	}
	if envelope.Code == 400 && (len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null"))) {
		return nil, CategorizedError{Category: ErrorTransient}
	}
	// The page bridge can report a temporary invocation failure in a valid
	// business envelope while WXU.API is still initializing. The collector's
	// shared readiness window will retry this category before the first profile
	// succeeds; after readiness it remains a closed, non-retried failure.
	var business struct {
		ErrCode int             `json:"errCode"`
		Data    json.RawMessage `json:"data"`
	}
	if envelope.Code == 0 && decodeSingleJSON(envelope.Data, &business) == nil &&
		business.ErrCode == 1011 && (len(business.Data) == 0 || bytes.Equal(bytes.TrimSpace(business.Data), []byte("null"))) {
		return nil, CategorizedError{Category: ErrorTransient}
	}
	return decodeLtaooBusinessData(raw)
}

func decodeSingleJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func requestString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case json.Number:
		return typed.String(), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	default:
		return "", false
	}
}

func NormalizeWeChatShareURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" ||
		!strings.EqualFold(parsed.Hostname(), "weixin.qq.com") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", errors.New("invalid WeChat share URL")
	}
	escapedPath := parsed.EscapedPath()
	if !strings.HasPrefix(escapedPath, "/sph/") || len(escapedPath) <= len("/sph/") {
		return "", errors.New("invalid WeChat share URL path")
	}
	escapedPath = strings.TrimSuffix(escapedPath, "/")
	if escapedPath == "/sph" || escapedPath == "/sph/" {
		return "", errors.New("invalid WeChat share URL path")
	}
	return "https://weixin.qq.com" + escapedPath, nil
}

func CollectWorksFromURLs(ctx context.Context, client *LtaooClient, rawURLs []string, limit int) ([]Work, []Issue) {
	return collectWorksFromURLs(ctx, client, rawURLs, limit, profileReadinessOptions{
		Clock:         batchClock{},
		Timeout:       profileReadinessTimeout,
		RetryInterval: profileRetryInterval,
	})
}

func collectWorksFromURLs(
	ctx context.Context,
	client *LtaooClient,
	rawURLs []string,
	limit int,
	options profileReadinessOptions,
) ([]Work, []Issue) {
	if client == nil || limit <= 0 || options.Clock == nil || options.Timeout <= 0 || options.RetryInterval <= 0 {
		return nil, []Issue{{Stage: "content", Code: "invalid_collection_request"}}
	}
	works := make([]Work, 0, min(limit, len(rawURLs)))
	issues := make([]Issue, 0)
	seenURLs := make(map[string]struct{})
	seenWorks := make(map[string]struct{})
	deadline := options.Clock.Now().Add(options.Timeout)
	readinessEstablished := false
	for index, rawURL := range rawURLs {
		if ctx.Err() != nil {
			issues = append(issues, Issue{Stage: "content", Code: "collection_cancelled", InputIndex: index + 1})
			break
		}
		normalized, err := NormalizeWeChatShareURL(rawURL)
		if err != nil {
			issues = append(issues, Issue{Stage: "content", Code: "invalid_share_url", InputIndex: index + 1})
			continue
		}
		if _, duplicate := seenURLs[normalized]; duplicate {
			continue
		}
		seenURLs[normalized] = struct{}{}
		if len(works) >= limit {
			issues = append(issues, Issue{Stage: "content", Code: "works_limit_reached", InputIndex: index + 1})
			break
		}
		var work Work
		for {
			if ctx.Err() != nil {
				issues = append(issues, Issue{Stage: "content", Code: "collection_cancelled", InputIndex: index + 1})
				return works, issues
			}
			if !readinessEstablished && !options.Clock.Now().Before(deadline) {
				issues = append(issues, Issue{Stage: "content", Code: "profile_not_ready", InputIndex: index + 1})
				return works, appendRemainingProfileNotReadyIssues(issues, rawURLs, index+1, seenURLs)
			}
			var err error
			work, err = client.ResolveWork(ctx, normalized, len(works)+1)
			if err == nil {
				readinessEstablished = true
				break
			}
			if ctx.Err() != nil {
				issues = append(issues, Issue{Stage: "content", Code: "collection_cancelled", InputIndex: index + 1})
				return works, issues
			}
			if readinessEstablished || ClassifyError(err) != ErrorTransient {
				issues = append(issues, Issue{Stage: "content", Code: profileIssueCode(err), InputIndex: index + 1})
				break
			}
			remaining := deadline.Sub(options.Clock.Now())
			if remaining <= 0 {
				issues = append(issues, Issue{Stage: "content", Code: "profile_not_ready", InputIndex: index + 1})
				return works, appendRemainingProfileNotReadyIssues(issues, rawURLs, index+1, seenURLs)
			}
			delay := min(options.RetryInterval, remaining)
			if err := options.Clock.Sleep(ctx, delay); err != nil {
				issues = append(issues, Issue{Stage: "content", Code: "collection_cancelled", InputIndex: index + 1})
				return works, issues
			}
		}
		if work.WorkID == nil {
			continue
		}
		key := dereferenceString(work.WorkID) + "\x00" + dereferenceString(work.ObjectNonceID)
		if _, duplicate := seenWorks[key]; duplicate {
			continue
		}
		seenWorks[key] = struct{}{}
		work.Locator.SearchRank = len(works) + 1
		work.Locator.IndexInPage = index + 1
		works = append(works, work)
	}
	return works, issues
}

func profileIssueCode(err error) string {
	switch ClassifyError(err) {
	case ErrorAccessDenied:
		return "profile_access_denied"
	case ErrorRateLimited:
		return "profile_rate_limited"
	case ErrorStructure:
		return "profile_schema_mismatch"
	default:
		return "profile_unavailable"
	}
}

func appendRemainingProfileNotReadyIssues(issues []Issue, rawURLs []string, start int, seenURLs map[string]struct{}) []Issue {
	for index := start; index < len(rawURLs); index++ {
		normalized, err := NormalizeWeChatShareURL(rawURLs[index])
		if err != nil {
			issues = append(issues, Issue{Stage: "content", Code: "invalid_share_url", InputIndex: index + 1})
			continue
		}
		if _, duplicate := seenURLs[normalized]; duplicate {
			continue
		}
		seenURLs[normalized] = struct{}{}
		issues = append(issues, Issue{Stage: "content", Code: "profile_not_ready", InputIndex: index + 1})
	}
	return issues
}
