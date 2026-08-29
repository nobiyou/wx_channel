package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

func TestScriptHandlerInjectsOfficialAccountPageOnly(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	handler.SetOfficialAccountScript([]byte("window.__official_account_test__=true;"), "http://127.0.0.1:2026", "secret")

	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://mp.weixin.qq.com/s/article-id?__biz=biz", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type":            []string{"text/html; charset=utf-8"},
				"Content-Security-Policy": []string{"script-src 'nonce-test-nonce'"},
			},
			Body: io.NopCloser(strings.NewReader("<html><head></head><body>article</body></html>")),
		},
	}
	if !handler.HandleHTMLResponse(page, "mp.weixin.qq.com", "/s/article-id", []byte("<html><head></head><body>article</body></html>")) {
		t.Fatal("official-account page was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read injected page: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "window.__official_account_test__=true") || !strings.Contains(bodyText, "secret") {
		t.Fatalf("official-account script/config missing: %s", body)
	}
	if !strings.Contains(bodyText, `nonce="test-nonce"`) || !strings.Contains(bodyText, `"biz":"biz"`) {
		t.Fatalf("CSP nonce or request biz missing from official-account injection: %s", body)
	}
	if strings.Index(bodyText, "window.__official_account_test__=true") < strings.Index(bodyText, "</head>") {
		t.Fatalf("official-account script should be injected after the document head: %s", body)
	}

	nonOfficial := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/home", nil),
		Response: &http.Response{
			Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body:   io.NopCloser(strings.NewReader("<html><head></head><body>home</body></html>")),
		},
	}
	if !handler.HandleHTMLResponse(nonOfficial, "channels.weixin.qq.com", "/web/pages/unknown", []byte("<html><head></head><body>home</body></html>")) {
		t.Fatal("non-official HTML response was not handled")
	}
	body, err = io.ReadAll(nonOfficial.Response.Body)
	if err != nil {
		t.Fatalf("read non-official page: %v", err)
	}
	if strings.Contains(string(body), "window.__official_account_test__") {
		t.Fatalf("official-account script leaked into non-official page: %s", body)
	}
}
