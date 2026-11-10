package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wx_channel/internal/config"
	"wx_channel/internal/websocket"
)

func TestSearchChannelAccountsOverMCP(t *testing.T) {
	hub := &fakeLiveHub{response: jsoniter.RawMessage(`{"data":{"infoList":[{"contact":{"nickname":"测试账号","username":"wx-test@finder","headUrl":"https://cdn.example.com/avatar.jpg"}}],"lastBuff":"cursor%2Bnext%3D","continueFlag":1},"token":"fixture-secret"}`)}
	session, ctx := accountSearchSession(t, hub)
	marker := ""
	for page := 0; page < 2; page++ {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "search_channel_accounts",
			Arguments: map[string]any{"keyword": " 测试账号 ", "next_marker": marker},
		})
		if err != nil || result.IsError {
			t.Fatalf("search page %d: result=%+v error=%v", page, result, err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "fixture-secret") {
			t.Fatal("account search leaked a sensitive field")
		}
		var output map[string]any
		if err := json.Unmarshal(encoded, &output); err != nil {
			t.Fatal(err)
		}
		if output["source"] != "live_wechat_page" || output["freshness"] != "live" {
			t.Fatalf("account search envelope = %s", encoded)
		}
		if _, err := time.Parse(time.RFC3339, output["observed_at"].(string)); err != nil {
			t.Fatalf("account observation time: %v", err)
		}
		data := output["data"].(map[string]any)
		contact := data["items"].([]any)[0].(map[string]any)
		if contact["username"] != "wx-test@finder" || contact["nickname"] != "测试账号" {
			t.Fatalf("account identity lost: %#v", contact)
		}
		if len(hub.calls) != page+1 {
			t.Fatalf("unexpected account search count: %d", len(hub.calls))
		}
		call := hub.calls[page]
		body, ok := call.body.(websocket.SearchContactBody)
		if call.key != "key:channels:contact_list" || !ok || body.Type != 1 || body.Keyword != "测试账号" || body.NextMarker != marker {
			t.Fatalf("account search must use account scene and preserve cursor: %#v", call)
		}
		marker = data["next_marker"].(string)
		if marker != url.PathEscape("cursor%2Bnext%3D") {
			t.Fatalf("continuation marker changed: %q", marker)
		}
	}

	for _, keyword := range []string{"", "   ", strings.Repeat("账", 101)} {
		before := len(hub.calls)
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "search_channel_accounts", Arguments: map[string]any{"keyword": keyword},
		})
		if err != nil || !result.IsError {
			t.Fatalf("invalid keyword must return a tool error: result=%+v error=%v", result, err)
		}
		if len(hub.calls) != before {
			t.Fatal("invalid keyword was sent to WeChat")
		}
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_channel_accounts", Arguments: map[string]any{"keyword": strings.Repeat("账", 100)},
	})
	if err != nil || result.IsError {
		t.Fatalf("100-character Chinese keyword must be accepted: result=%+v error=%v", result, err)
	}
}

func TestSearchChannelAccountsWithoutReadyPage(t *testing.T) {
	session, ctx := accountSearchSession(t, websocket.NewHub())
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_channel_accounts", Arguments: map[string]any{"keyword": "测试账号"},
	})
	if err != nil || !result.IsError {
		t.Fatalf("missing ready page must return a tool error: result=%+v error=%v", result, err)
	}
	encoded, _ := json.Marshal(result.Content)
	if !strings.Contains(string(encoded), "no_ready_client") {
		t.Fatalf("missing stable readiness error: %s", encoded)
	}
}

func accountSearchSession(t *testing.T, hub Hub) (*mcp.ClientSession, context.Context) {
	t.Helper()
	server, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: t.TempDir(), Hub: hub})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.handler)
	t.Cleanup(httpServer.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "account-search-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpServer.URL,
		HTTPClient: &http.Client{Transport: bearerRoundTripper{base: http.DefaultTransport, token: server.token}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}
