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
	}, cancel
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
