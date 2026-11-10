package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"wx_channel/internal/config"
	"wx_channel/internal/websocket"
)

type blockingContractHub struct {
	started chan struct{}
	release chan struct{}
}

func (*blockingContractHub) ClientStatuses() []websocket.ClientStatus { return nil }
func (*blockingContractHub) GetClientForKey(string) (*websocket.Client, error) {
	return &websocket.Client{}, nil
}
func (h *blockingContractHub) CallAPIContext(ctx context.Context, _ string, _ interface{}, _ time.Duration) (jsoniter.RawMessage, error) {
	h.started <- struct{}{}
	select {
	case <-h.release:
		return jsoniter.RawMessage(`{"data":{"infoList":[]}}`), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestMCPConcurrencyBoundAndSlotRelease(t *testing.T) {
	hub := &blockingContractHub{started: make(chan struct{}, 3), release: make(chan struct{})}
	session, ctx := accountSearchSession(t, hub)
	finished := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			r, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_accounts", Arguments: map[string]any{"keyword": "test"}})
			if e == nil && r.IsError {
				e = context.Canceled
			}
			finished <- e
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-hub.started:
		case <-ctx.Done():
			t.Fatal("concurrent queries not started")
		}
	}
	r, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_accounts", Arguments: map[string]any{"keyword": "test"}})
	encoded, _ := json.Marshal(r)
	if e != nil || !r.IsError || !strings.Contains(string(encoded), "busy") {
		t.Fatalf("third query not bounded: %s %v", encoded, e)
	}
	close(hub.release)
	for i := 0; i < 2; i++ {
		if e := <-finished; e != nil {
			t.Fatal(e)
		}
	}
	r, e = session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_accounts", Arguments: map[string]any{"keyword": "test"}})
	if e != nil || r.IsError {
		t.Fatalf("slot not released: %+v %v", r, e)
	}
}

func TestMCPRequestBodyLimit(t *testing.T) {
	s, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(strings.Repeat("x", maxRequestBytes+1)))
	req.Header.Set("Authorization", "Bearer "+s.token)
	w := httptest.NewRecorder()
	s.handler.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status=%d", w.Code)
	}
}

func TestRuntimeClientDiagnosticsRedacted(t *testing.T) {
	hub := &fakeLiveHub{status: websocket.ClientStatus{PagePath: "/web/pages/feed?pass_ticket=secret#secret", APIProbeError: `failed C:\private\config.yaml token=secret`, Href: "https://channels.weixin.qq.com/web/pages/feed?pass_ticket=secret", Methods: map[string]bool{"finderSearch": true, "token=secret": true}}}
	session, ctx := accountSearchSession(t, hub)
	r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_runtime_status"})
	encoded, _ := json.Marshal(r)
	if err != nil || r.IsError || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "private") {
		t.Fatalf("runtime diagnostics leaked: %s %v", encoded, err)
	}
}
