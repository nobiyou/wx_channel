package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

func TestBuildInjectedScriptsOmitsRemovedLogPanel(t *testing.T) {
	script := (&ScriptHandler{}).buildInjectedScripts("/web/pages/home")
	for _, marker := range []string{
		"__wx_channels_log_panel",
		"__wx_channels_log_toggle_btn",
		"WX_CHANNEL_SHOW_LOG_BUTTON",
	} {
		if strings.Contains(script, marker) {
			t.Fatalf("injected page script still contains removed log-panel marker %q", marker)
		}
	}
}

func TestScriptHandlerInjectsLoginPage(t *testing.T) {
	handler := &ScriptHandler{apiClientJS: []byte("__wx_api_client"), coreJS: []byte("business_core"), homeJS: []byte("business_home")}
	script := handler.buildInjectedScripts("/platform/login")
	if !strings.Contains(script, "__wx_api_client") {
		t.Fatal("login page injection must include the websocket client")
	}
	if !isChannelLoginPagePath("/platform/login") || !isChannelLoginPagePath("/login.html") {
		t.Fatal("known video-channel login paths must be recognized")
	}
	if isChannelLoginPagePath("/web/pages/home") {
		t.Fatal("business page must not be recognized as a login path")
	}
	for _, marker := range []string{"business_core", "business_home", "insert_download_btn", "__wx_start_page_injection_lifecycle__"} {
		if strings.Contains(script, marker) {
			t.Fatalf("login page must not load business-page code: %s", marker)
		}
	}
}

func TestOfficialOAuthHTMLIsForwardedUnchanged(t *testing.T) {
	handler := &ScriptHandler{version: "5.7.16"}
	conn := &SunnyNet.HttpConn{Response: &http.Response{Header: http.Header{"Content-Type": {"text/html"}}}}
	original := `<html><head><script src="/official.js"></script></head><body><img class="qrcode" src="/connect/qrcode/session"></body></html>`
	handler.HandleHTMLResponse(conn, "open.weixin.qq.com", "/connect/qrconnect", []byte(original))
	body, err := io.ReadAll(conn.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != original {
		t.Fatal("OAuth HTML must not be rewritten or receive QR capture scripts")
	}
}

func TestLoginHTMLKeepsOfficialScriptNamesAndNonce(t *testing.T) {
	handler := &ScriptHandler{apiClientJS: []byte("window.__wx_login_client__=true"), version: "5.7.16"}
	conn := &SunnyNet.HttpConn{Response: &http.Response{Header: http.Header{"Content-Type": {"text/html"}, "Content-Security-Policy": {"script-src 'nonce-login'"}}}}
	handler.HandleHTMLResponse(conn, "channels.weixin.qq.com", "/platform/login", []byte(`<html><head><script src="/official.js"></script></head></html>`))
	body, err := io.ReadAll(conn.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `src="/official.js"`) || !strings.Contains(string(body), `<script nonce="login">window.__wx_login_client__`) {
		t.Fatal("login injection changed official script names or lost the CSP nonce")
	}
}

func TestLoginJavascriptIsForwardedWithoutRewritingDependencies(t *testing.T) {
	handler := &ScriptHandler{version: "5.7.16"}
	conn := &SunnyNet.HttpConn{
		Request:  httptest.NewRequest(http.MethodGet, "https://res.wx.qq.com/shared.js", nil),
		Response: &http.Response{Header: http.Header{"Content-Type": {"application/javascript"}}},
	}
	conn.Request.Header.Set("Referer", "https://channels.weixin.qq.com/platform/login")
	original := `import "./dependency.js";`
	handler.HandleJavaScriptResponse(conn, "res.wx.qq.com", "/shared.js", []byte(original))
	body, err := io.ReadAll(conn.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != original {
		t.Fatal("login JavaScript dependencies must remain unchanged")
	}
}
