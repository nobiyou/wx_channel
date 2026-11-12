package handlers

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/qtgolang/SunnyNet/SunnyNet"
	sunnyPublic "github.com/qtgolang/SunnyNet/public"
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
				"Content-Encoding":        []string{"gzip"},
				"Content-Length":          []string{"7"},
				"Cache-Control":           []string{"public, max-age=3600"},
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
	if page.Response.Header.Get("Content-Encoding") != "" || page.Response.Header.Get("Content-Length") != "" {
		t.Fatalf("modified official-account response kept stale body headers: %v", page.Response.Header)
	}
	if page.Response.Header.Get("Cache-Control") != "no-cache, no-store, must-revalidate" ||
		page.Response.Header.Get("Pragma") != "no-cache" || page.Response.Header.Get("Expires") != "0" {
		t.Fatalf("modified official-account response was not marked uncacheable: %v", page.Response.Header)
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

func TestScriptHandlerRewritesLiveHTMLWithoutStaleBodyHeaders(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "?t=test")
	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/live", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type":     []string{"text/html; charset=utf-8"},
				"Content-Encoding": []string{"gzip"},
				"Content-Length":   []string{"12"},
				"Cache-Control":    []string{"public, max-age=3600"},
			},
			Body: io.NopCloser(strings.NewReader(`<html><head><script src="/web/app.js"></script></head><body>live</body></html>`)),
		},
	}

	if !handler.HandleHTMLResponse(page, "channels.weixin.qq.com", "/web/pages/live", []byte(`<html><head><script src="/web/app.js"></script></head><body>live</body></html>`)) {
		t.Fatal("live HTML response was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read live page: %v", err)
	}
	if !strings.Contains(string(body), `src="/web/app.js?t=test"`) {
		t.Fatalf("live page script version was not applied: %s", body)
	}
	if page.Response.Header.Get("Content-Encoding") != "" || page.Response.Header.Get("Content-Length") != "" {
		t.Fatalf("live page kept stale body headers: %v", page.Response.Header)
	}
	if page.Response.Header.Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
		t.Fatalf("live page cache headers were not reset: %v", page.Response.Header)
	}
}

func TestScriptHandlerRewritesUnchangedLiveHTMLWithoutStaleBodyHeaders(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/live", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type":     []string{"text/html; charset=utf-8"},
				"Content-Encoding": []string{"gzip"},
				"Content-Length":   []string{"12"},
				"Cache-Control":    []string{"public, max-age=3600"},
			},
			Body: io.NopCloser(strings.NewReader(`<html><head></head><body>live</body></html>`)),
		},
	}

	bodyText := `<html><head></head><body>live</body></html>`
	if !handler.HandleHTMLResponse(page, "channels.weixin.qq.com", "/web/pages/live", []byte(bodyText)) {
		t.Fatal("unchanged live HTML response was not handled")
	}
	if page.Response.Header.Get("Content-Encoding") != "" || page.Response.Header.Get("Content-Length") != "" {
		t.Fatalf("unchanged live page kept stale body headers: %v", page.Response.Header)
	}
	if page.Response.Header.Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
		t.Fatalf("unchanged live page cache headers were not reset: %v", page.Response.Header)
	}
}

func TestScriptHandlerInjectsOnlyIsolatedLiveScript(t *testing.T) {
	handler := NewScriptHandler(nil, []byte("core"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	handler.SetLiveScript([]byte("window.__wx_live_test__=true;"))
	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/live", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type":     []string{"text/html; charset=utf-8"},
				"Content-Encoding": []string{"gzip"},
				"Content-Length":   []string{"12"},
			},
			Body: io.NopCloser(strings.NewReader(`<html><head></head><body>live</body></html>`)),
		},
	}

	if !handler.HandleHTMLResponse(page, "channels.weixin.qq.com", "/web/pages/live", []byte(`<html><head></head><body>live</body></html>`)) {
		t.Fatal("live HTML response was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read live page: %v", err)
	}
	bodyText := string(body)
	if !strings.Contains(bodyText, "window.__wx_live_test__=true") {
		t.Fatalf("isolated live script was not injected: %s", bodyText)
	}
	if !strings.Contains(bodyText, "__wx_channels_save_page_content") {
		t.Fatalf("live page did not receive the page-save script: %s", bodyText)
	}
	if strings.Contains(bodyText, "core") {
		t.Fatalf("live page received the Home/Profile base injection: %s", bodyText)
	}
}

func TestScriptHandlerInjectsLiveScriptWhenHeadHasAttributes(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	handler.SetLiveScript([]byte("window.__wx_live_test__=true;"))
	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/live", nil),
		Response: &http.Response{
			Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body:   io.NopCloser(strings.NewReader(`<HTML><HEAD data-page="live"></HEAD><BODY>live</BODY></HTML>`)),
		},
	}

	if !handler.HandleHTMLResponse(page, "channels.weixin.qq.com", "/web/pages/live", []byte(`<HTML><HEAD data-page="live"></HEAD><BODY>live</BODY></HTML>`)) {
		t.Fatal("live HTML response was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read live page: %v", err)
	}
	bodyText := string(body)
	openHead := strings.Index(bodyText, `<HEAD data-page="live">`)
	headEnd := strings.Index(bodyText, `</HEAD>`)
	liveScript := strings.Index(bodyText, "__wx_live_test__")
	if openHead < 0 || headEnd < 0 || liveScript < openHead || liveScript > headEnd {
		t.Fatalf("live script was not inserted after an attributed head tag: %s", bodyText)
	}
	if !strings.Contains(bodyText, "__wx_channels_save_page_content") {
		t.Fatalf("live page-save script was not inserted after an attributed head tag: %s", bodyText)
	}
}

func TestScriptHandlerRewritesLiveJavascriptWithoutStaleBodyHeaders(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "?t=test")
	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/live", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type":     []string{"application/javascript; charset=utf-8"},
				"Content-Encoding": []string{"gzip"},
				"Content-Length":   []string{"12"},
			},
			Body: io.NopCloser(strings.NewReader(`import "chunk.js";`)),
		},
	}

	if !handler.HandleJavaScriptResponse(page, "channels.weixin.qq.com", "/web/app.js", []byte(`import "chunk.js";`)) {
		t.Fatal("live JavaScript response was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read live JavaScript: %v", err)
	}
	if !strings.Contains(string(body), `import"chunk.js?t=test"`) {
		t.Fatalf("live JavaScript dependency version was not applied: %s", body)
	}
	if page.Response.Header.Get("Content-Encoding") != "" || page.Response.Header.Get("Content-Length") != "" {
		t.Fatalf("live JavaScript kept stale body headers: %v", page.Response.Header)
	}
}

func TestScriptHandlerSkipsPageSpecificRewritesForLivePageJavascript(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://res.wx.qq.com/web/finder/virtual_svg-icons-register.publish.js", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/javascript; charset=utf-8"},
			},
			Body: io.NopCloser(strings.NewReader("")),
		},
	}
	page.Request.Header.Set("Referer", "https://channels.weixin.qq.com/web/pages/live?scene=detail")

	source := `async finderUserPage(args){return await fetchUser(args)}async
async finderLiveUserPage(args){return await fetchLive(args)}async
async finderGetLiveInfo(args){return await fetchLiveInfo(args)}async
async joinLive(args){return await joinLiveRequest(args)}async
export{finderUserPage as userPage,finderLiveUserPage as livePage}`
	if !handler.HandleJavaScriptResponse(page, "res.wx.qq.com", "/web/finder/virtual_svg-icons-register.publish.js", []byte(source)) {
		t.Fatal("live JavaScript response was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read live virtual SVG bundle: %v", err)
	}
	bodyText := string(body)
	if strings.Contains(bodyText, "WXU.emit") || strings.Contains(bodyText, "[Profile API]") {
		t.Fatalf("live page received page-specific API injection: %s", bodyText)
	}
	if !strings.Contains(bodyText, "__wx_channels_capture_live_data__") || !strings.Contains(bodyText, "finderGetLiveInfo") || !strings.Contains(bodyText, "joinLive") {
		t.Fatalf("live page API capture wrappers were not installed: %s", bodyText)
	}
}

func TestScriptHandlerKeepsPageSpecificRewritesOutsideLivePage(t *testing.T) {
	// The live-route marker is process-wide to model SunnyNet callback
	// boundaries; clear it so this Home-route test is order-independent.
	livePageContextUntil.Store(0)
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/javascript; charset=utf-8"},
			},
			Body: io.NopCloser(strings.NewReader("")),
		},
	}
	page.Request.Header.Set("Referer", "https://channels.weixin.qq.com/web/pages/home")

	source := `async finderUserPage(args){return await fetchUser(args)}async
export{finderUserPage as userPage}`
	if !handler.HandleJavaScriptResponse(page, "channels.weixin.qq.com", "/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js", []byte(source)) {
		t.Fatal("home JavaScript response was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read home virtual SVG bundle: %v", err)
	}
	if !strings.Contains(string(body), "WXU.emit") || !strings.Contains(string(body), "[Profile API]") {
		t.Fatalf("home page API injection was unexpectedly skipped: %s", body)
	}
}

func TestScriptHandlerSkipsPageSpecificRewritesWithoutPageReferer(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	page := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type": []string{"application/javascript; charset=utf-8"},
			},
			Body: io.NopCloser(strings.NewReader("")),
		},
	}

	source := `async finderUserPage(args){return await fetchUser(args)}async
export{finderUserPage as userPage}`
	if !handler.HandleJavaScriptResponse(page, "channels.weixin.qq.com", "/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js", []byte(source)) {
		t.Fatal("unknown-source JavaScript response was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read unknown-source virtual SVG bundle: %v", err)
	}
	bodyText := string(body)
	if strings.Contains(bodyText, "WXU.emit") || strings.Contains(bodyText, "[Profile API]") {
		t.Fatalf("page-specific source rewrite ran without a page referer: %s", bodyText)
	}
	if !strings.Contains(bodyText, "finderUserPage") {
		t.Fatalf("original shared bundle was not preserved: %s", bodyText)
	}
}

func TestScriptHandlerSkipsHomeRewritesAfterLiveNavigationWithHomeReferer(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	livePage := &SunnyNet.HttpConn{
		PID:     4242,
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/live", nil),
		Response: &http.Response{
			Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body:   io.NopCloser(strings.NewReader(`<html><head></head><body>live</body></html>`)),
		},
	}
	if !handler.HandleHTMLResponse(livePage, "channels.weixin.qq.com", "/web/pages/live", []byte(`<html><head></head><body>live</body></html>`)) {
		t.Fatal("live HTML response was not handled")
	}

	shared := &SunnyNet.HttpConn{
		PID:     4242,
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js", nil),
		Response: &http.Response{
			Header: http.Header{"Content-Type": []string{"application/javascript; charset=utf-8"}},
			Body:   io.NopCloser(strings.NewReader("")),
		},
	}
	shared.Request.Header.Set("Referer", "https://channels.weixin.qq.com/web/pages/home")
	source := `async finderUserPage(args){return await fetchUser(args)}async
export{finderUserPage as userPage}`
	// Use a different handler instance to model callbacks that do not preserve
	// the same connection-local state.
	sharedHandler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	if !sharedHandler.HandleJavaScriptResponse(shared, "channels.weixin.qq.com", "/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js", []byte(source)) {
		t.Fatal("shared live JavaScript response was not handled")
	}
	body, err := io.ReadAll(shared.Response.Body)
	if err != nil {
		t.Fatalf("read shared live JavaScript: %v", err)
	}
	if strings.Contains(string(body), "WXU.emit") || strings.Contains(string(body), "[Profile API]") {
		t.Fatalf("home-specific rewrite ran for a live navigation with a stale Home referer: %s", body)
	}
}

func TestScriptHandlerSkipsHomeRewritesAcrossLiveResourceConnections(t *testing.T) {
	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	livePage := &SunnyNet.HttpConn{
		PID:      1001,
		ClientIP: "127.0.0.1:41001",
		Request:  httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/live", nil),
		Response: &http.Response{
			Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body:   io.NopCloser(strings.NewReader(`<html><head></head><body>live</body></html>`)),
		},
	}
	if !handler.HandleHTMLResponse(livePage, "channels.weixin.qq.com", "/web/pages/live", []byte(`<html><head></head><body>live</body></html>`)) {
		t.Fatal("live HTML response was not handled")
	}

	shared := &SunnyNet.HttpConn{
		PID:      2002,
		ClientIP: "127.0.0.1:41002",
		Request:  httptest.NewRequest(http.MethodGet, "https://res.wx.qq.com/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js", nil),
		Response: &http.Response{
			Header: http.Header{"Content-Type": []string{"application/javascript; charset=utf-8"}},
			Body:   io.NopCloser(strings.NewReader("")),
		},
	}
	shared.Request.Header.Set("Referer", "https://channels.weixin.qq.com/web/pages/home")
	source := `async finderUserPage(args){return await fetchUser(args)}async
export{finderUserPage as userPage}`
	sharedHandler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	if !sharedHandler.HandleJavaScriptResponse(shared, "res.wx.qq.com", "/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js", []byte(source)) {
		t.Fatal("shared live JavaScript response was not handled")
	}
	body, err := io.ReadAll(shared.Response.Body)
	if err != nil {
		t.Fatalf("read shared live JavaScript: %v", err)
	}
	if strings.Contains(string(body), "WXU.emit") || strings.Contains(string(body), "[Profile API]") {
		t.Fatalf("home-specific rewrite ran across live resource connections: %s", body)
	}
}

func TestDecodeHTTPResponseBodyDecodesGzipBeforeRewrite(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("<html><body>live</body></html>")); err != nil {
		t.Fatalf("write gzip body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip body: %v", err)
	}

	decoded, ok, err := decodeHTTPResponseBody(http.Header{"Content-Encoding": []string{"gzip"}}, compressed.Bytes())
	if err != nil {
		t.Fatalf("decode gzip body: %v", err)
	}
	if !ok || string(decoded) != "<html><body>live</body></html>" {
		t.Fatalf("decoded gzip body = %q, decoded=%v", decoded, ok)
	}
}

func TestScriptHandlerRewritesGzipLiveHTMLAfterDecoding(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("<html><head></head><body>live</body></html>")); err != nil {
		t.Fatalf("write gzip live page: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip live page: %v", err)
	}

	handler := NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	handler.SetLiveScript([]byte("window.__wx_live_test__=true;"))
	page := &SunnyNet.HttpConn{
		Type:    sunnyPublic.HttpResponseOK,
		Request: httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com/web/pages/live", nil),
		Response: &http.Response{
			Header: http.Header{
				"Content-Type":     []string{"text/html; charset=utf-8"},
				"Content-Encoding": []string{"gzip"},
				"Content-Length":   []string{strconv.Itoa(compressed.Len())},
			},
			Body: io.NopCloser(bytes.NewReader(compressed.Bytes())),
		},
	}

	if !handler.Handle(page) {
		t.Fatal("gzip live HTML response was not handled")
	}
	body, err := io.ReadAll(page.Response.Body)
	if err != nil {
		t.Fatalf("read decoded live page: %v", err)
	}
	if !strings.Contains(string(body), "window.__wx_live_test__=true") {
		t.Fatalf("live script was not injected into decoded page: %s", body)
	}
	if page.Response.Header.Get("Content-Encoding") != "" || page.Response.Header.Get("Content-Length") != "" {
		t.Fatalf("decoded live page kept compressed body headers: %v", page.Response.Header)
	}
}
