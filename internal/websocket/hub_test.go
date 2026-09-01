package websocket

import (
	"context"
	"testing"
	"time"

	json "github.com/json-iterator/go"
)

const testAsyncTimeout = 5 * time.Second

func newTestAPIClient(id string) (*Client, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		ID:       id,
		send:     make(chan []byte, 4),
		hub:      nil,
		ctx:      ctx,
		cancel:   cancel,
		apiReady: true,
		methods:  map[string]bool{"finderGetCommentList": true},
		lastSeen: time.Now(),
	}, cancel
}

func TestGetClientForKeySkipsStaleClient(t *testing.T) {
	hub := NewHub()
	stale, cancelStale := newTestAPIClient("stale")
	defer cancelStale()
	stale.lastSeen = time.Now().Add(-2 * defaultLivenessTimeout)
	fresh, cancelFresh := newTestAPIClient("fresh")
	defer cancelFresh()
	hub.clients[stale] = true
	hub.clients[fresh] = true

	client, err := hub.GetClientForKey("key:channels:fetch_feed_comment_list")
	if err != nil {
		t.Fatalf("GetClientForKey() error = %v", err)
	}
	if client != fresh {
		t.Fatalf("GetClientForKey() selected %q, want fresh client", client.ID)
	}
}

func TestGetClientForKeySkipsProtocolFreshButApplicationStaleClient(t *testing.T) {
	hub := NewHub()
	client, cancel := newTestAPIClient("protocol-only")
	defer cancel()
	client.lastSeen = time.Now().Add(-2 * defaultLivenessTimeout)
	client.lastPong = time.Now()
	hub.clients[client] = true

	if _, err := hub.GetClientForKey("key:channels:fetch_feed_comment_list"); err == nil {
		t.Fatal("GetClientForKey() error = nil, want stale application client to be rejected")
	}
}

func TestClientStatusSeparatesApplicationAndProtocolFreshness(t *testing.T) {
	client, cancel := newTestAPIClient("status")
	defer cancel()
	client.lastSeen = time.Now().Add(-2 * defaultLivenessTimeout)
	client.lastPong = time.Now()

	status := client.statusAt(time.Now(), defaultLivenessTimeout)
	if status.ApplicationFresh {
		t.Fatal("ApplicationFresh = true, want false")
	}
	if !status.ProtocolFresh {
		t.Fatal("ProtocolFresh = false, want true")
	}
	if status.Fresh {
		t.Fatal("Fresh = true, want false")
	}
}

func TestClientStatusRejectsFailedFunctionalProbe(t *testing.T) {
	client, cancel := newTestAPIClient("probe-failed")
	defer cancel()
	client.apiProbeStatus = "failed"
	client.apiProbeAt = time.Now()

	status := client.statusAt(time.Now(), defaultLivenessTimeout)
	if status.Fresh {
		t.Fatal("Fresh = true, want false after failed functional probe")
	}
	if status.APIFunctionalFresh {
		t.Fatal("APIFunctionalFresh = true, want false after failed functional probe")
	}
}

func TestCallAPIRetriesOnDisconnectedClient(t *testing.T) {
	hub := NewHub()
	hub.SetSelector(NewRoundRobinSelector())

	first, cancelFirst := newTestAPIClient("client-a")
	defer cancelFirst()
	second, cancelSecond := newTestAPIClient("client-b")
	defer cancelSecond()
	hub.clients[first] = true
	hub.clients[second] = true

	type result struct {
		data []byte
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		data, err := hub.CallAPI("key:channels:fetch_feed_comment_list", FeedCommentListBody{
			ObjectID: "object-1",
			NonceID:  "nonce-1",
		}, 2*testAsyncTimeout)
		resultCh <- result{data: data, err: err}
	}()

	select {
	case <-first.send:
		cancelFirst()
	case <-time.After(testAsyncTimeout):
		t.Fatal("first client did not receive API request")
	}

	var request APICallRequest
	select {
	case raw := <-second.send:
		message, ok := decodeAPIRequest(raw)
		if !ok {
			t.Fatal("second client received malformed API request")
		}
		request = message
	case <-time.After(testAsyncTimeout):
		t.Fatal("API request was not retried on second client")
	}

	hub.handleAPIResponse(APICallResponse{
		ID:   request.ID,
		Data: []byte(`{"errCode":0,"data":"ok"}`),
	})

	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatalf("CallAPI() error = %v", got.err)
		}
		if string(got.data) != `{"errCode":0,"data":"ok"}` {
			t.Fatalf("CallAPI() data = %s", got.data)
		}
	case <-time.After(testAsyncTimeout):
		t.Fatal("CallAPI() did not complete after retry response")
	}
}

func TestCallAPIDoesNotRetryNonIdempotentCall(t *testing.T) {
	hub := NewHub()
	hub.SetSelector(NewRoundRobinSelector())

	first, cancelFirst := newTestAPIClient("client-a")
	defer cancelFirst()
	second, cancelSecond := newTestAPIClient("client-b")
	defer cancelSecond()
	hub.clients[first] = true
	hub.clients[second] = true

	resultCh := make(chan error, 1)
	go func() {
		_, err := hub.CallAPI("key:channels:download_video", map[string]string{"videoUrl": "https://example.test/video"}, testAsyncTimeout)
		resultCh <- err
	}()

	select {
	case <-first.send:
		cancelFirst()
	case <-time.After(testAsyncTimeout):
		t.Fatal("first client did not receive API request")
	}

	select {
	case err := <-resultCh:
		if err == nil {
			t.Fatal("CallAPI() error = nil, want disconnected client error")
		}
	case <-time.After(testAsyncTimeout):
		t.Fatal("non-idempotent CallAPI() did not stop after disconnect")
	}

	select {
	case <-second.send:
		t.Fatal("non-idempotent API call was retried on second client")
	default:
	}
}

func TestSendCommandToMatchingClientTargetsOnlyOneClient(t *testing.T) {
	hub := NewHub()
	first, cancelFirst := newTestAPIClient("client-a")
	defer cancelFirst()
	first.pagePath = "/web/pages/home"
	first.href = "https://channels.weixin.qq.com/web/pages/home"
	second, cancelSecond := newTestAPIClient("client-b")
	defer cancelSecond()
	second.pagePath = "/web/pages/profile"
	second.href = "https://channels.weixin.qq.com/web/pages/profile"
	hub.clients[first] = true
	hub.clients[second] = true

	err := hub.SendCommandToMatchingClient(func(status ClientStatus) bool {
		return status.PagePath == "/web/pages/profile"
	}, "channel_reload", map[string]interface{}{"reason": "test"})
	if err != nil {
		t.Fatalf("SendCommandToMatchingClient() error = %v", err)
	}

	select {
	case raw := <-second.send:
		var message WSMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode command: %v", err)
		}
		if message.Type != WSMessageTypeCommand {
			t.Fatalf("message type = %s, want %s", message.Type, WSMessageTypeCommand)
		}
	case <-time.After(time.Second):
		t.Fatal("matching client did not receive command")
	}
	select {
	case <-first.send:
		t.Fatal("unmatched client received command")
	default:
	}
}

func TestSendCommandToMatchingClientRejectsInvalidSelection(t *testing.T) {
	hub := NewHub()
	if err := hub.SendCommandToMatchingClient(nil, "channel_reload", nil); err == nil {
		t.Fatal("nil predicate error = nil")
	}
	if err := hub.SendCommandToMatchingClient(func(ClientStatus) bool { return true }, "channel_reload", nil); err == nil {
		t.Fatal("empty hub error = nil")
	}
}

func TestBroadcastCommandStillReachesAllClients(t *testing.T) {
	hub := NewHub()
	first, cancelFirst := newTestAPIClient("client-a")
	defer cancelFirst()
	second, cancelSecond := newTestAPIClient("client-b")
	defer cancelSecond()
	hub.clients[first] = true
	hub.clients[second] = true

	if err := hub.BroadcastCommand("download_progress", map[string]interface{}{"done": true}); err != nil {
		t.Fatalf("BroadcastCommand() error = %v", err)
	}
	for name, client := range map[string]*Client{"first": first, "second": second} {
		select {
		case <-client.send:
		case <-time.After(time.Second):
			t.Fatalf("%s client did not receive broadcast", name)
		}
	}
}

func decodeAPIRequest(raw []byte) (APICallRequest, bool) {
	var message WSMessage
	if err := json.Unmarshal(raw, &message); err != nil || message.Type != WSMessageTypeAPICall {
		return APICallRequest{}, false
	}

	var request APICallRequest
	if err := json.Unmarshal(message.Data, &request); err != nil {
		return APICallRequest{}, false
	}
	return request, true
}
