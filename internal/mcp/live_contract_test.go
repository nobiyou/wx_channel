package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"wx_channel/internal/websocket"
)

func TestLiveProjectionPreservesIDsMetricsAndPagination(t *testing.T) {
	hub := &fakeLiveHub{response: jsoniter.RawMessage(`{"data":{"objectList":[{"id":18446744073709551615,"objectNonceId":"nonce-1","contact":{"username":"author@finder","nickname":"作者"},"objectDesc":{"description":"视频标题","media":[{"videoPlayLen":12,"url":"https://finder.video.qq.com/stodownload?token=secret","decodeKey":"secret","thumbUrl":"https://cdn.example.com/a.jpg"}]},"likeCount":0,"commentCount":"5"}],"lastBuff":"raw%+/= buffer","continueFlag":1},"payload":{"cookie":"secret"}}`)}
	session, ctx := accountSearchSession(t, hub)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_videos", Arguments: map[string]any{"keyword": "科技"}})
	if err != nil || result.IsError {
		t.Fatalf("query: %v, %v", result, err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var out map[string]any
	_ = json.Unmarshal(encoded, &out)
	data := out["data"].(map[string]any)
	items, ok := data["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("normalized page missing: %s", encoded)
	}
	item := items[0].(map[string]any)
	if item["object_id"] != "18446744073709551615" || item["nonce_id"] != "nonce-1" {
		t.Fatalf("identity corrupted: %s", encoded)
	}
	if item["like_count"] != float64(0) || item["comment_count"] != float64(5) {
		t.Fatalf("metric presence lost: %s", encoded)
	}
	if _, exists := item["play_count"]; exists {
		t.Fatalf("retired play_count leaked: %s", encoded)
	}
	if strings.Contains(string(encoded), "secret") || data["result"] != nil {
		t.Fatalf("raw data leaked: %s", encoded)
	}
	marker, err := url.PathUnescape(data["next_marker"].(string))
	if err != nil || marker != "raw%+/= buffer" || data["has_more"] != true {
		t.Fatalf("marker corrupted: %s", encoded)
	}
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_videos", Arguments: map[string]any{"keyword": "科技", "next_marker": data["next_marker"]}})
	if err != nil || hub.calls[1].body.(websocket.SearchContactBody).NextMarker != data["next_marker"] {
		t.Fatal("continuation was not forwarded unchanged")
	}
}

func TestLiveEmptyAndUnknownSchemasAreDifferent(t *testing.T) {
	for _, tc := range []struct{ raw, code string }{
		{`{"data":{"objectList":[],"lastBuff":"","continueFlag":0}}`, ""},
		{`{"data":{"unexpected":[]}}`, "unsupported_live_schema"},
		{`{"data":{"BaseResponse":{"Ret":-1},"objectList":[]}}`, "live_query_failed"},
		{`{"errCode":-70003,"data":null}`, "page_context_mismatch"},
		{`{"data":{"objectList":[]}} {}`, "invalid_live_response"},
		{``, "empty_live_response"},
		{`{"data":{"objectList":"broken"}}`, "unsupported_live_schema"},
		{`{"data":{"objectList":[{"newSchema":true}]}}`, "unsupported_live_schema"},
		{`null`, "empty_live_response"},
		{`{"data":null}`, "empty_live_response"},
		{`{"data":{"objectList":[],"continueFlag":1}}`, "pagination_unavailable"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			session, ctx := accountSearchSession(t, &fakeLiveHub{response: jsoniter.RawMessage(tc.raw)})
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_videos", Arguments: map[string]any{"keyword": "test"}})
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(result)
			if tc.code == "" {
				if result.IsError {
					t.Fatalf("empty page failed: %s", encoded)
				}
				return
			}
			if !result.IsError || !strings.Contains(string(encoded), tc.code) {
				t.Fatalf("expected %s: %s", tc.code, encoded)
			}
			if result.StructuredContent == nil {
				t.Fatal("error has no structured contract")
			}
		})
	}
}

type errorContractHub struct{ err error }

func (*errorContractHub) ClientStatuses() []websocket.ClientStatus { return nil }
func (*errorContractHub) GetClientForKey(string) (*websocket.Client, error) {
	return &websocket.Client{}, nil
}
func (h *errorContractHub) CallAPIContext(context.Context, string, interface{}, time.Duration) (jsoniter.RawMessage, error) {
	return nil, h.err
}

func TestLiveErrorsAreStableAndDoNotLeak(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{context.DeadlineExceeded, "timeout"}, {websocket.ErrRequestTimeout, "timeout"},
		{context.Canceled, "canceled"}, {websocket.ErrNoReadyClient, "no_ready_client"},
		{errors.New("API error (code=-70003): JSAPI_JSONPARSE_FAILED token=secret"), "page_context_mismatch"},
		{errors.New("https://host/path?pass_ticket=secret"), "live_query_failed"},
	} {
		session, ctx := accountSearchSession(t, &errorContractHub{err: tc.err})
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_accounts", Arguments: map[string]any{"keyword": "test"}})
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(result)
		if !result.IsError || !strings.Contains(string(encoded), tc.code) || strings.Contains(string(encoded), "secret") || !strings.Contains(string(encoded), "recovery") {
			t.Fatalf("error contract: %s", encoded)
		}
	}
}

func TestLiveProfileAndCommentsProjection(t *testing.T) {
	hub := &fakeLiveHub{response: jsoniter.RawMessage(`{"data":{"object":{"id":"video","objectDesc":{"description":"title"},"playCount":0}}}`)}
	session, ctx := accountSearchSession(t, hub)
	for _, name := range []string{"get_channel_video_profile", "resolve_channel_share_link"} {
		args := map[string]any{"url": "https://weixin.qq.com/sph/abc"}
		if name == "get_channel_video_profile" {
			args["object_id"] = "video"
			args["nonce_id"] = "nonce"
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("profile: %v %v", result, err)
		}
		encoded, _ := json.Marshal(result.StructuredContent)
		if !strings.Contains(string(encoded), `"object_id":"video"`) || strings.Contains(string(encoded), `"play_count"`) {
			t.Fatalf("profile projection: %s", encoded)
		}
	}
	hub.response = jsoniter.RawMessage(`{"data":{"commentInfo":[{"commentId":9007199254740993,"content":"hello","nickname":"reader","likeCount":0,"expandCommentCount":2,"levelTwoComment":[{"content":"nested"}]}],"lastBuffer":""}}`)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_channel_comments", Arguments: map[string]any{"object_id": "video", "nonce_id": "nonce"}})
	encoded, _ := json.Marshal(result.StructuredContent)
	if err != nil || result.IsError || !strings.Contains(string(encoded), `"comment_id":"9007199254740993"`) || strings.Contains(string(encoded), "nested") {
		t.Fatalf("comments: %s %v", encoded, err)
	}
}

func TestLiveOversizedPageFailsWithoutSkippingAndURLsValidated(t *testing.T) {
	items := strings.TrimSuffix(strings.Repeat(`{"id":"video"},`, 51), ",")
	hub := &fakeLiveHub{response: jsoniter.RawMessage(`{"data":{"objectList":[` + items + `],"lastBuffer":"next"}}`)}
	session, ctx := accountSearchSession(t, hub)
	result, _ := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_videos", Arguments: map[string]any{"keyword": "test"}})
	encoded, _ := json.Marshal(result)
	if !result.IsError || !strings.Contains(string(encoded), "response_too_large") {
		t.Fatalf("oversized page silently accepted: %s", encoded)
	}
	before := len(hub.calls)
	for _, badURL := range []string{"http://channels.weixin.qq.com/web/pages/feed", "https://example.com/video", "https://channels.weixin.qq.com.evil.test/web/pages/feed"} {
		result, _ := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_channel_video_profile", Arguments: map[string]any{"url": badURL}})
		if !result.IsError {
			t.Fatalf("invalid URL accepted: %s", badURL)
		}
	}
	if len(hub.calls) != before {
		t.Fatal("invalid URLs reached page bridge")
	}
}

func TestSharedMetadataWithoutStableVideoIdentity(t *testing.T) {
	for _, id := range []string{"", `"id":"shared_feed",`, `"id":"export-1",`} {
		hub := &fakeLiveHub{response: jsoniter.RawMessage(`{"data":{"object":{` + id + `"objectDesc":{"description":"share title","media":[{"durationMs":12500,"videoPlayLen":0}]}},"sceneInfo":{"dynamicExportId":"export-1"},"feedInfo":{"durationMs":12500}}}`)}
		session, ctx := accountSearchSession(t, hub)
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "resolve_channel_share_link", Arguments: map[string]any{"url": "https://weixin.qq.com/sph/abc"}})
		if err != nil || r.IsError {
			t.Fatalf("valid share metadata rejected: %+v %v", r, err)
		}
		encoded, _ := json.Marshal(r.StructuredContent)
		var out map[string]any
		_ = json.Unmarshal(encoded, &out)
		data := out["data"].(map[string]any)
		if data["object_id"] != nil || data["duration_seconds"] != 12.5 || data["share_reference"] != "export-1" {
			t.Fatalf("share reference confused with video id or duration lost: %s", encoded)
		}
	}
}

func TestProfileWrongTypesAreSchemaErrors(t *testing.T) {
	for _, objectValue := range []string{`[]`, `"broken"`} {
		session, ctx := accountSearchSession(t, &fakeLiveHub{response: jsoniter.RawMessage(`{"data":{"object":` + objectValue + `}}`)})
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_channel_video_profile", Arguments: map[string]any{"object_id": "video", "nonce_id": "nonce"}})
		encoded, _ := json.Marshal(r)
		if err != nil || !r.IsError || !strings.Contains(string(encoded), "unsupported_live_schema") {
			t.Fatalf("wrong-type object mistaken for empty: %s %v", encoded, err)
		}
	}
}

func TestResultWireSizeIncludesTextAndStructuredContent(t *testing.T) {
	item := `{"id":"video","objectDesc":{"description":"` + strings.Repeat("x", 4096) + `"}}`
	raw := `{"data":{"objectList":[` + strings.TrimSuffix(strings.Repeat(item+",", 40), ",") + `],"lastBuff":""}}`
	session, ctx := accountSearchSession(t, &fakeLiveHub{response: jsoniter.RawMessage(raw)})
	r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_channel_videos", Arguments: map[string]any{"keyword": "test"}})
	encoded, _ := json.Marshal(r)
	if err != nil || len(encoded) > maxToolResponseBytes {
		t.Fatalf("wire limit exceeded: %d bytes, error=%v", len(encoded), err)
	}
}
