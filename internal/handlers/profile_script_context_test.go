package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"wx_channel/internal/assets"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

const profileAPIFixture = `import "./child.js";class API{async finderUserPage(args){return await this.load(args)}async next(){}}var api=new API();export{api};`

func rewriteProfileScript(t *testing.T, handler *ScriptHandler, scriptURL *url.URL, referer string) string {
	t.Helper()
	requestURL := *scriptURL
	requestURL.Path = "/t/wx_fed/finder/web/web-finder/res/js/virtual_svg-icons-register.js"
	conn := &SunnyNet.HttpConn{
		Request: httptest.NewRequest(http.MethodGet, requestURL.String(), nil),
		Response: &http.Response{Header: http.Header{
			"Content-Type":   {"application/javascript"},
			"Content-Length": {"999"},
		}},
	}
	conn.Request.Header.Set("Referer", referer)
	if !handler.HandleJavaScriptResponse(conn, requestURL.Hostname(), requestURL.Path, []byte(profileAPIFixture)) {
		t.Fatal("script response was not handled")
	}
	body, err := io.ReadAll(conn.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != profileAPIFixture && conn.Response.Header.Get("Content-Length") != "" {
		t.Fatal("modified script retained its old response length")
	}
	return string(body)
}

func TestChannelPageScriptsKeepProfileEventsWithoutFullReferer(t *testing.T) {
	livePageContextUntil.Store(0)
	t.Cleanup(func() { livePageContextUntil.Store(0) })
	for _, route := range []string{"/web/pages/profile", "/web/pages/home", "/web/pages/feed", "/web/pages/account/like"} {
		for _, referer := range []string{"https://channels.weixin.qq.com/", "https://channels.weixin.qq.com", ""} {
			t.Run(route+"/"+referer, func(t *testing.T) {
				handler := liveCacheTestHandler()
				scriptURL := pageScriptURL(t, handler, route)
				body := rewriteProfileScript(t, handler, scriptURL, referer)
				if !strings.Contains(body, "WXU.Events.UserFeedsLoaded") || !strings.Contains(body, "WXU.Events.APILoaded") {
					t.Fatal("injected channel page lost its profile list/API events when Referer did not identify the page")
				}
				marker := scriptURL.Query().Get("wx_channels")
				if marker == "" || !strings.Contains(body, "child.js?t=5.7.16&wx_channels="+marker) {
					t.Fatal("page runtime identity did not propagate to imported dependencies")
				}
			})
		}
	}
}

func TestProfileScriptsKeepTheirRuntimeAfterLiveNavigation(t *testing.T) {
	livePageContextUntil.Store(0)
	t.Cleanup(func() { livePageContextUntil.Store(0) })
	handler := liveCacheTestHandler()
	liveURL := pageScriptURL(t, handler, "/web/pages/live")
	profileURL := pageScriptURL(t, handler, "/web/pages/profile")
	for _, referer := range []string{"https://channels.weixin.qq.com/", "https://channels.weixin.qq.com/web/pages/live", ""} {
		// Callback instances can differ. The URL must identify the owning
		// runtime independently of handler state and stale navigation hints.
		body := rewriteProfileScript(t, liveCacheTestHandler(), profileURL, referer)
		if !strings.Contains(body, "WXU.Events.UserFeedsLoaded") || strings.Contains(body, "wx_live=") {
			t.Fatal("recent live navigation overrode the profile page's script runtime")
		}
	}
	if profileURL.Query().Get("wx_channels") == "" || profileURL.Query().Get("wx_live") != "" || liveURL.Query().Get("wx_channels") != "" {
		t.Fatal("profile and live scripts must have distinct runtime/cache identities")
	}
}

func TestProfileBatchButtonReceivesCapturedVideos(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is needed to execute the injected page scripts")
	}
	livePageContextUntil.Store(0)
	t.Cleanup(func() { livePageContextUntil.Store(0) })
	handler := liveCacheTestHandler()
	scriptURL := pageScriptURL(t, handler, "/web/pages/profile")
	module := rewriteProfileScript(t, handler, scriptURL, "https://channels.weixin.qq.com/")
	// Only the fixture's module boundary is adapted for VM execution. The API
	// method body and its emitted events come from the real response rewriter.
	module = strings.Replace(module, `import"./child.js`+handler.channelsScriptVersion+`";`, "", 1)
	module = strings.Replace(module, "export{api};", "", 1)
	input, err := json.Marshal([]string{string(assets.MittJS), string(assets.EventbusJS), string(assets.UtilsJS), string(assets.BatchDownloadJS), string(assets.ProfileJS), module})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "-e", profileButtonHarness)
	command.Stdin = bytes.NewReader(input)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("profile API response -> collection -> batch button failed: %v\n%.4000s", err, output)
	}
}

const profileButtonHarness = `
const assert = require('node:assert/strict');
const vm = require('node:vm');
const scripts = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
const buttons = [];
const messages = [];
const container = { querySelector() { return null; }, appendChild(button) { buttons.push(button); } };
const sandbox = {
  console: { log() {}, warn() {}, error() {} },
  location: { pathname: '/web/pages/profile' },
  navigator: { userAgent: 'profile-regression-test' },
  addEventListener() {},
  setTimeout() {}, setInterval() {}, clearInterval() {},
  __wx_log(message) { messages.push(message.msg); },
  document: {
    readyState: 'loading', addEventListener() {}, getElementById() { return null; },
    querySelector(selector) { return selector === '.profile-info .opr-area' ? container : null; },
    createElement() { return { style: {}, set innerHTML(value) { this.textContent = value.replace(/<[^>]*>/g, ''); } }; },
  },
};
sandbox.window = sandbox;
vm.createContext(sandbox);
for (const script of scripts) vm.runInContext(script, sandbox);
const response = { data: { object: [
  { id: 'short-video', objectDesc: { mediaType: 4, description: 'Profile video', media: [{ url: 'https://example.test/video.mp4', decodeKey: '123' }] } },
  { id: 'picture', objectDesc: { mediaType: 2, description: 'Picture', media: [{ coverUrl: 'https://example.test/cover.jpg' }] } },
] } };
// The existing batch renderer has separate tests. Observe this producer /
// collector / real click seam at the actual manager it feeds.
sandbox.__show_batch_download_ui__ = (videos, title) => {
  sandbox.__wx_batch_download_manager__.setVideos(videos, title);
  sandbox.__wx_batch_download_manager__.isVisible = true;
};
sandbox.api.load = async () => response;
(async () => {
  assert.equal(await sandbox.api.finderUserPage({ username: 'author' }), response, 'page API result must be preserved');
  sandbox.__wx_channels_profile_collector.injectToolbarDownloadIcon();
  assert.equal(buttons.length, 1, 'profile batch button is injected');
  await buttons[0].onclick();
  const manager = sandbox.__wx_batch_download_manager__;
  assert.equal(manager.isVisible, true, messages.join('\n'));
  assert.equal(manager.activeTab, 'video');
  assert.deepEqual(Array.from(manager.getCurrentPageVideos(), item => item.id), ['short-video']);
  assert.equal(messages.some(message => message.includes('暂无视频数据')), false);
})().catch(error => { console.error(error); process.exitCode = 1; });
`
