package officialaccount

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"wx_channel/internal/utils"
)

const maxObservedCommentRequestBody = 256 << 10

// IsOfficialAccountPath identifies article and public-account endpoints whose
// requests may carry the short-lived credentials needed by the collection API.
func IsOfficialAccountPath(path string) bool {
	return path == "/s" || strings.HasPrefix(path, "/s/") ||
		path == "/mp/profile_ext" || path == "/mp/author" || path == "/mp/getappmsgext" || path == "/mp/appmsg_comment"
}

// CaptureRequest captures credentials from the request boundary. WeChat's
// article HTML can keep these values in a local script scope or leave them
// empty, while the credential-bearing profile request still carries them in
// its query string and Cookie header.
func (s *Service) CaptureRequest(req *http.Request) bool {
	if s == nil || req == nil || req.URL == nil {
		return false
	}
	if strings.ToLower(req.URL.Hostname()) != "mp.weixin.qq.com" || !IsOfficialAccountPath(req.URL.Path) {
		return false
	}
	if req.URL.Path == "/mp/appmsg_comment" {
		observeCommentRequest(req)
	}

	account := accountFromURL(req.URL)
	if referer, err := url.Parse(req.Referer()); err == nil && referer != nil &&
		strings.ToLower(referer.Hostname()) == "mp.weixin.qq.com" &&
		IsOfficialAccountPath(referer.Path) {
		refererAccount := accountFromURL(referer)
		if account.Biz == "" {
			account.Biz = refererAccount.Biz
		}
		account.MergeFrom(refererAccount)
	}
	if req.Header != nil {
		account.Cookie = strings.TrimSpace(req.Header.Get("Cookie"))
		if account.Cookie != "" {
			account.CookieExpiration = s.now().Add(24 * time.Hour).Unix()
		}
	}
	if account.Biz == "" || account.Key == "" {
		return false
	}
	return s.Upsert(account) == nil
}

// observeCommentRequest records only the shape and non-credential identity
// fields of the browser's comment request. It is useful when WeChat changes
// the endpoint contract, while preserving the request body for SunnyNet.
func observeCommentRequest(req *http.Request) {
	if req == nil || req.URL == nil {
		return
	}

	form := make(url.Values)
	if req.Body != nil && req.ContentLength >= 0 && req.ContentLength <= maxObservedCommentRequestBody {
		body, err := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
		if err == nil {
			form, _ = url.ParseQuery(string(body))
		}
	}

	query := req.URL.Query()
	queryKeys := sortedValueKeys(query)
	formKeys := sortedValueKeys(form)
	identityKeys := []string{
		"action", "__biz", "appmsgid", "mid", "idx", "comment_id", "offset", "limit",
		"begin_comment_id", "f", "clientversion", "devicetype", "is_need_reward",
	}
	identity := make([]string, 0, len(identityKeys))
	for _, key := range identityKeys {
		value := firstNonEmpty(query.Get(key), form.Get(key))
		if value == "" {
			continue
		}
		identity = append(identity, key+"="+sanitizeObservedCommentValue(value))
	}
	utils.LogFileInfo("[公众号评论请求观测] method=%s | query_keys=%s | form_keys=%s | identity=%s | content_length=%d",
		req.Method, strings.Join(queryKeys, ","), strings.Join(formKeys, ","), strings.Join(identity, ","), req.ContentLength)
}

func sortedValueKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sanitizeObservedCommentValue(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			continue
		}
		builder.WriteRune(char)
		if builder.Len() >= 128 {
			break
		}
	}
	return builder.String()
}

func accountFromURL(u *url.URL) Account {
	if u == nil {
		return Account{}
	}
	query := u.Query()
	return Account{
		Biz:         firstNonEmpty(query.Get("__biz"), query.Get("biz"), query.Get("bizuin")),
		Nickname:    firstNonEmpty(query.Get("nickname"), query.Get("nick_name")),
		AvatarURL:   firstNonEmpty(query.Get("avatar_url"), query.Get("headimg")),
		AuthorID:    firstNonEmpty(query.Get("author_id"), query.Get("authorid"), query.Get("user_name")),
		Uin:         firstNonEmpty(query.Get("uin"), query.Get("user_uin")),
		Key:         query.Get("key"),
		PassTicket:  query.Get("pass_ticket"),
		AppmsgToken: query.Get("appmsg_token"),
		RefreshURI:  u.String(),
	}
}
