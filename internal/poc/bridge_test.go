package poc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestBridgeRejectsQueryToken(t *testing.T) {
	server, httpServer, token := newTestBridge(t)
	defer server.Close()
	defer httpServer.Close()

	conn, response, err := dialBridge(t, httpServer.URL+"/ws/api?token=secret", token, bridgeOrigin)
	if conn != nil {
		_ = conn.CloseNow()
	}
	if response != nil {
		defer response.Body.Close()
	}
	if err == nil {
		t.Fatal("bridge accepted a query-string token")
	}
}

func TestBridgeRejectsWrongOrigin(t *testing.T) {
	server, httpServer, token := newTestBridge(t)
	defer server.Close()
	defer httpServer.Close()

	conn, response, err := dialBridge(t, httpServer.URL+"/ws/api", token, "https://example.test")
	if conn != nil {
		_ = conn.CloseNow()
	}
	if response != nil {
		defer response.Body.Close()
	}
	if err == nil {
		t.Fatal("bridge accepted the wrong origin")
	}
}

func TestBridgeAcceptsAuthSubprotocolWithoutEchoingSecret(t *testing.T) {
	server, httpServer, token := newTestBridge(t)
	defer server.Close()
	defer httpServer.Close()

	conn, response, err := dialBridge(t, httpServer.URL+"/ws/api", token, bridgeOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if got := conn.Subprotocol(); got != bridgeProtocol {
		t.Fatalf("subprotocol=%q", got)
	}
	if strings.Contains(response.Header.Get("Sec-WebSocket-Protocol"), token) {
		t.Fatal("authentication token was echoed in the handshake")
	}
}

func TestBridgeRejectsNonReadOnlyMethod(t *testing.T) {
	server, httpServer, token := newTestBridge(t)
	defer server.Close()
	defer httpServer.Close()
	conn := connectReadyBridge(t, server, httpServer.URL, token, map[string]bool{"finderSearch": true})
	defer conn.CloseNow()

	if _, err := server.Call(context.Background(), "commentLike", map[string]any{}); err == nil {
		t.Fatal("bridge accepted a write method")
	}
}

func TestBridgeStateDropsHrefAndUserAgent(t *testing.T) {
	server, httpServer, token := newTestBridge(t)
	defer server.Close()
	defer httpServer.Close()
	conn, _, err := dialBridge(t, httpServer.URL+"/ws/api", token, bridgeOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	writeBridgeJSON(t, conn, map[string]any{
		"type": "client_state",
		"data": map[string]any{
			"pagePath":  "/platform/post/list",
			"visible":   true,
			"methods":   map[string]bool{"finderSearch": true},
			"href":      "https://channels.weixin.qq.com/platform/post/list?token=secret",
			"userAgent": "secret-agent",
		},
	})
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.WaitReady(waitCtx, []string{"finderSearch"}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(server.State())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "href") || strings.Contains(string(encoded), "userAgent") {
		t.Fatalf("unsafe state retained: %s", encoded)
	}
}

func TestBridgeCallsDeclaredReadOnlyMethod(t *testing.T) {
	server, httpServer, token := newTestBridge(t)
	defer server.Close()
	defer httpServer.Close()
	conn := connectReadyBridge(t, server, httpServer.URL, token, map[string]bool{"finderSearch": true})
	defer conn.CloseNow()

	done := make(chan error, 1)
	go func() {
		_, raw, err := conn.Read(context.Background())
		if err != nil {
			done <- err
			return
		}
		var call struct {
			Type string `json:"type"`
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &call); err != nil {
			done <- err
			return
		}
		response := map[string]any{"type": "api_response", "data": map[string]any{
			"id": call.Data.ID, "data": map[string]any{"items": []any{}}, "errCode": 0,
		}}
		encoded, err := json.Marshal(response)
		if err == nil {
			err = conn.Write(context.Background(), websocket.MessageText, encoded)
		}
		done <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	response, err := server.Call(ctx, "finderSearch", map[string]any{"keyword": "青云装饰"})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if string(response) != `{"items":[]}` {
		t.Fatalf("response=%s", response)
	}
}

func TestBridgeRejectsConcurrentCalls(t *testing.T) {
	server, httpServer, token := newTestBridge(t)
	defer server.Close()
	defer httpServer.Close()
	conn := connectReadyBridge(t, server, httpServer.URL, token, map[string]bool{"finderSearch": true})
	defer conn.CloseNow()

	callReceived := make(chan struct{})
	go func() {
		_, _, err := conn.Read(context.Background())
		if err == nil {
			close(callReceived)
		}
	}()
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := server.Call(firstCtx, "finderSearch", map[string]any{"keyword": "青云装饰"})
		firstDone <- err
	}()
	select {
	case <-callReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("first bridge call was not sent")
	}
	if _, err := server.Call(context.Background(), "finderSearch", map[string]any{"keyword": "青云装饰"}); err == nil || !strings.Contains(err.Error(), "concurrent") {
		t.Fatalf("concurrent call err=%v", err)
	}
	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first call err=%v", err)
	}
}

func newTestBridge(t *testing.T) (*BridgeServer, *httptest.Server, string) {
	t.Helper()
	token := base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	server := NewBridgeServer(token, DiscardLogger{})
	httpServer := httptest.NewServer(server.Handler())
	return server, httpServer, token
}

func dialBridge(t *testing.T, rawURL, token, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(rawURL, "http")
	return websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{
		HTTPHeader:   http.Header{"Origin": []string{origin}},
		Subprotocols: []string{bridgeProtocol, "auth." + token},
	})
}

func connectReadyBridge(t *testing.T, server *BridgeServer, rawURL, token string, methods map[string]bool) *websocket.Conn {
	t.Helper()
	conn, _, err := dialBridge(t, rawURL+"/ws/api", token, bridgeOrigin)
	if err != nil {
		t.Fatal(err)
	}
	writeBridgeJSON(t, conn, map[string]any{"type": "client_state", "data": map[string]any{
		"pagePath": "/platform/post/list", "visible": true, "methods": methods,
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	required := make([]string, 0, len(methods))
	for method, available := range methods {
		if available {
			required = append(required, method)
		}
	}
	if err := server.WaitReady(ctx, required); err != nil {
		_ = conn.CloseNow()
		t.Fatal(err)
	}
	return conn
}

func writeBridgeJSON(t *testing.T, conn *websocket.Conn, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, raw); err != nil {
		t.Fatal(err)
	}
}
