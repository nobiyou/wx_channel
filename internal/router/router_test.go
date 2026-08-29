package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wx_channel/internal/config"
	"wx_channel/internal/officialaccount"
	"wx_channel/internal/websocket"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

func newTestRouter() *APIRouter {
	cfg := &config.Config{
		Port:           2025,
		AllowedOrigins: []string{"*"},
	}
	hub := websocket.NewHub()
	sunny := SunnyNet.NewSunny()

	// Create router with nil dependencies where possible or mocked ones
	r := NewAPIRouter(cfg, hub, sunny)
	return r
}

func TestSystemAPI(t *testing.T) {
	router := newTestRouter()

	// Test Info
	req, _ := http.NewRequest("GET", "/api/v1/system/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"].(float64) != 0 {
		t.Errorf("Expected code 0, got %v", resp["code"])
	}

	// Test Health
	req, _ = http.NewRequest("GET", "/api/v1/system/health", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestOfficialAccountRoutesCanBeMounted(t *testing.T) {
	router := newTestRouter()
	service := officialaccount.NewMemoryService()
	if err := service.Upsert(officialaccount.Account{Biz: "biz-1", Nickname: "公众号", Key: "key-1"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	router.SetOfficialAccountService(service)

	req := httptest.NewRequest(http.MethodGet, "/api/mp/list", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected mounted account route to return 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "biz-1") || strings.Contains(recorder.Body.String(), "key-1") {
		t.Fatalf("unexpected account response: %s", recorder.Body.String())
	}
}

func TestOfficialAccountListFiltersByKeyword(t *testing.T) {
	router := newTestRouter()
	service := officialaccount.NewMemoryService()
	if err := service.Upsert(officialaccount.Account{Biz: "biz-target", Nickname: "目标公众号", Key: "secret-target"}); err != nil {
		t.Fatalf("upsert target account: %v", err)
	}
	if err := service.Upsert(officialaccount.Account{Biz: "biz-other", Nickname: "其他公众号", Key: "secret-other"}); err != nil {
		t.Fatalf("upsert other account: %v", err)
	}
	router.SetOfficialAccountService(service)

	request := httptest.NewRequest(http.MethodGet, "/api/mp/list?keyword=%E7%9B%AE%E6%A0%87", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected filtered list 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "biz-target") || strings.Contains(recorder.Body.String(), "biz-other") {
		t.Fatalf("unexpected filtered account response: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "secret-target") || strings.Contains(recorder.Body.String(), "secret-other") {
		t.Fatalf("filtered account response leaked credentials: %s", recorder.Body.String())
	}
}

func TestOfficialAccountRefreshAllowsWeChatPageOrigin(t *testing.T) {
	cfg := &config.Config{
		Port:           2025,
		AllowedOrigins: []string{"https://mp.weixin.qq.com"},
	}
	router := NewAPIRouter(cfg, websocket.NewHub(), SunnyNet.NewSunny())
	service := officialaccount.NewMemoryService()
	router.SetOfficialAccountService(service)

	preflight := httptest.NewRequest(http.MethodOptions, "/api/mp/refresh", nil)
	preflight.Header.Set("Origin", "https://mp.weixin.qq.com")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "content-type")
	preflightRecorder := httptest.NewRecorder()
	router.ServeHTTP(preflightRecorder, preflight)
	if preflightRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected preflight 204, got %d: %s", preflightRecorder.Code, preflightRecorder.Body.String())
	}
	if got := preflightRecorder.Header().Get("Access-Control-Allow-Origin"); got != "https://mp.weixin.qq.com" {
		t.Fatalf("expected WeChat origin to be allowed, got %q", got)
	}

	payload := `{"biz":"biz-1","nickname":"测试公众号","avatar_url":"https://mmbiz.qpic.cn/avatar/1","author_id":"author-1","uin":"uin-1","key":"key-1","pass_ticket":"ticket-1"}`
	request := httptest.NewRequest(http.MethodPost, "/api/mp/refresh", strings.NewReader(payload))
	request.Header.Set("Origin", "https://mp.weixin.qq.com")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected metadata refresh 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	accounts := service.ListAccounts()
	if len(accounts) != 1 || accounts[0].Nickname != "测试公众号" {
		t.Fatalf("metadata refresh did not update account: %+v", accounts)
	}

	metadataRequest := httptest.NewRequest(http.MethodPost, "/api/mp/metadata", strings.NewReader(`{"biz":"biz-1","nickname":"页面名称","avatar_url":"https://mmbiz.qpic.cn/avatar/2","author_id":"author-2"}`))
	metadataRequest.Header.Set("Origin", "https://mp.weixin.qq.com")
	metadataRequest.Header.Set("Content-Type", "application/json")
	metadataRecorder := httptest.NewRecorder()
	router.ServeHTTP(metadataRecorder, metadataRequest)
	if metadataRecorder.Code != http.StatusOK {
		t.Fatalf("expected metadata enrichment 200, got %d: %s", metadataRecorder.Code, metadataRecorder.Body.String())
	}
	accounts = service.ListAccounts()
	if len(accounts) != 1 || accounts[0].Nickname != "页面名称" || accounts[0].AvatarURL == "" {
		t.Fatalf("metadata enrichment did not update account: %+v", accounts)
	}
}

func TestLogsAPI(t *testing.T) {
	router := newTestRouter()

	req, _ := http.NewRequest("GET", "/api/v1/logs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestProxyAPI(t *testing.T) {
	router := newTestRouter()

	// Test Status
	req, _ := http.NewRequest("GET", "/api/v1/proxy/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestSearchAPI(t *testing.T) {
	router := newTestRouter()

	// Test Contact Search (expecting 500 because no WS client connected)
	params := map[string]interface{}{
		"keyword":   "test",
		"page":      1,
		"page_size": 10,
	}
	body, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", "/api/v1/search/contact", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Expecting 503 Service Unavailable (no available client)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 (no client), got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if msg, ok := resp["message"].(string); !ok || !strings.Contains(msg, "WeChat client not connected") {
		t.Logf("Got message: %v", msg)
	}

	// Test Feed Search
	req, _ = http.NewRequest("GET", "/api/v1/search/feed?username=test", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

func TestCertificateAPI(t *testing.T) {
	router := newTestRouter()

	// Test Status
	req, _ := http.NewRequest("GET", "/api/v1/certificate/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Note: Verify response format. CheckCertificate might fail or satisfy depending on environment
	// We just check if it returns a valid JSON response structure
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("Failed to parse response JSON: %v", err)
	}
}

func TestAuthMiddleware_PublicPathsBypass(t *testing.T) {
	handler := AuthMiddleware("secret-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	publicPaths := []string{
		"/api/health",
		"/api/console/verify-token",
		"/api/system/health",
		"/api/v1/system/health",
	}

	for _, p := range publicPaths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected public path %s to bypass auth, got %d", p, w.Code)
		}
	}
}

func TestAuthMiddleware_ProtectedPathRequiresToken(t *testing.T) {
	handler := AuthMiddleware("secret-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 无 token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}

	// 正确 token
	req = httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	req.Header.Set("X-Local-Auth", "secret-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d", w.Code)
	}
}

func TestVideoPlayRoute_UsesPlayHandler(t *testing.T) {
	router := newTestRouter()

	req, _ := http.NewRequest(http.MethodGet, "/api/video/play", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	msg, _ := resp["message"].(string)
	if msg == "" {
		msg, _ = resp["error"].(string)
	}
	if !strings.Contains(msg, "url parameter is required") {
		t.Fatalf("expected play handler error message, got: %v", msg)
	}
}
