package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

func liveCacheTestHandler() *ScriptHandler {
	return NewScriptHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "?t=5.7.16")
}

func pageScriptURL(t *testing.T, handler *ScriptHandler, route string) *url.URL {
	t.Helper()
	source := `<html><head><script src="https://res.wx.qq.com/shared.js"></script></head><body></body></html>`
	conn := &SunnyNet.HttpConn{
		Request:  httptest.NewRequest(http.MethodGet, "https://channels.weixin.qq.com"+route, nil),
		Response: &http.Response{Header: http.Header{"Content-Type": {"text/html"}}},
	}
	if !handler.HandleHTMLResponse(conn, "channels.weixin.qq.com", route, []byte(source)) {
		t.Fatal("page was not handled")
	}
	body, err := io.ReadAll(conn.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`src="(https://res\.wx\.qq\.com/[^\"]+)"`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatal("missing shared script URL")
	}
	result, err := url.Parse(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestLiveScriptCacheIsSeparateFromHomeAndPreviousRuntime(t *testing.T) {
	t.Cleanup(func() { livePageContextUntil.Store(0) })
	handler := liveCacheTestHandler()
	home := pageScriptURL(t, handler, "/web/pages/home")
	live := pageScriptURL(t, handler, "/web/pages/live")
	if home.String() == live.String() || live.Query().Get("wx_live") == "" {
		t.Fatalf("live page reuses the Home script cache: home=%s live=%s", home, live)
	}
	if home.Query().Get("t") != "5.7.16" || live.Query().Get("t") != "5.7.16" {
		t.Fatal("application version was lost")
	}
	restarted := pageScriptURL(t, liveCacheTestHandler(), "/web/pages/live")
	if live.String() == restarted.String() {
		t.Fatal("restarting a test build must not reuse an older live capture bundle")
	}
}

func TestMarkedLiveScriptKeepsCaptureAcrossStaleReferers(t *testing.T) {
	livePageContextUntil.Store(0)
	t.Cleanup(func() { livePageContextUntil.Store(0) })
	handler := liveCacheTestHandler()
	source := `import "chunk.js";class API{async finderGetLiveInfo(args){return this.post(args)}async next(){}}export{API};`
	conn := &SunnyNet.HttpConn{
		Request:  httptest.NewRequest(http.MethodGet, "https://res.wx.qq.com/virtual_svg-icons-register.js?t=5.7.16&wx_live=previous-navigation", nil),
		Response: &http.Response{Header: http.Header{"Content-Type": {"application/javascript"}}},
	}
	conn.Request.Header.Set("Referer", "https://channels.weixin.qq.com/web/pages/home")
	if !handler.HandleJavaScriptResponse(conn, "res.wx.qq.com", conn.Request.URL.Path, []byte(source)) {
		t.Fatal("live bundle was not handled")
	}
	body, err := io.ReadAll(conn.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "__wx_channels_capture_live_data__") || strings.Contains(string(body), "WXU.emit") {
		t.Fatalf("marked live bundle received the wrong page runtime: %s", body)
	}
	if !strings.Contains(string(body), "chunk.js?t=5.7.16&wx_live=") {
		t.Fatalf("live context was lost from dependency imports: %s", body)
	}
}
