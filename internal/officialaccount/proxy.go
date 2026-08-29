package officialaccount

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"wx_channel/internal/response"
)

const maxProxyBody = 16 << 20

var proxyAssetURLPattern = regexp.MustCompile(`(?i)(?:https?:)?//(?:mmbiz\.qpic\.cn|mmbiz\.qpic\.com|mmbiz\.qpic\.qq\.com|mmbiz\.qpic\.net|mmbiz\.qpic\.org)/[^\s"'<>]+`)

func (s *Service) HandleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		responseError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	target, err := parseAllowedProxyURL(html.UnescapeString(strings.TrimSpace(r.URL.Query().Get("url"))))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		writeServiceError(w, ErrInvalidProxyURL)
		return
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := s.client().Do(req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer resp.Body.Close()
	copyProxyHeaders(w, resp.Header)

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxProxyBody+1))
		if readErr != nil || len(body) > maxProxyBody {
			if readErr == nil {
				readErr = ErrUpstream
			}
			writeServiceError(w, readErr)
			return
		}
		body = s.rewriteProxyDocument(r, body)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Del("Content-Encoding")
		w.Header().Del("Content-Length")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
		return
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxProxyBody))
}

func parseAllowedProxyURL(raw string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || target == nil || target.Hostname() == "" || target.User != nil {
		return nil, ErrInvalidProxyURL
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, ErrInvalidProxyURL
	}
	if !isAllowedProxyHost(strings.ToLower(target.Hostname())) {
		return nil, ErrInvalidProxyURL
	}
	return target, nil
}

func isAllowedProxyHost(host string) bool {
	if host == "mp.weixin.qq.com" || host == "res.wx.qq.com" {
		return true
	}
	return host == "mmbiz.qpic.cn" || strings.HasSuffix(host, ".qpic.cn")
}

func (s *Service) rewriteProxyDocument(r *http.Request, body []byte) []byte {
	origin := s.requestOrigin(r)
	rewritten := proxyAssetURLPattern.ReplaceAllStringFunc(string(body), func(raw string) string {
		target := html.UnescapeString(raw)
		if strings.HasPrefix(target, "//") {
			target = "https:" + target
		}
		if _, err := parseAllowedProxyURL(target); err != nil {
			return raw
		}
		return s.proxyURLWithOrigin(origin, target)
	})
	return []byte(rewritten)
}

func (s *Service) proxyURL(r *http.Request, target string) string {
	return s.proxyURLWithOrigin(s.requestOrigin(r), target)
}

func (s *Service) proxyURLWithOrigin(origin, target string) string {
	return strings.TrimRight(origin, "/") + "/mp/proxy?url=" + url.QueryEscape(target)
}

func (s *Service) requestOrigin(r *http.Request) string {
	s.mu.RLock()
	origin := s.apiOrigin
	s.mu.RUnlock()
	if origin != "" {
		return origin
	}
	if r == nil {
		return "http://127.0.0.1"
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func copyProxyHeaders(w http.ResponseWriter, headers http.Header) {
	for key, values := range headers {
		switch strings.ToLower(key) {
		case "content-length", "content-encoding", "transfer-encoding", "set-cookie", "content-security-policy", "content-security-policy-report-only":
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}

func responseError(w http.ResponseWriter, status int, message string) {
	response.ErrorWithStatus(w, status, status, message)
}
