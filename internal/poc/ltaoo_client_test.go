package poc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type profileTestClock struct {
	now    time.Time
	sleeps []time.Duration
}

func TestCollectWorksFromURLsUsesOneMinuteProfileReadinessWindow(t *testing.T) {
	if profileReadinessTimeout != time.Minute {
		t.Fatalf("profile readiness timeout=%s, want %s", profileReadinessTimeout, time.Minute)
	}
}

func (c *profileTestClock) Now() time.Time { return c.now }

func (c *profileTestClock) Sleep(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.sleeps = append(c.sleeps, duration)
	c.now = c.now.Add(duration)
	return nil
}

func TestLtaooClientMapsStatusProfileAndContinuousCommentCursor(t *testing.T) {
	const shareURL = "https://weixin.qq.com/sph/fixture-share"
	var mu sync.Mutex
	requests := make([]string, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.RequestURI())
		mu.Unlock()
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write(readFixture(t, "ltaoo_status_ok.json"))
		case "/api/channels/feed/profile":
			if got := r.URL.Query().Get("url"); got != shareURL {
				t.Errorf("share url=%q", got)
			}
			_, _ = w.Write(readFixture(t, "ltaoo_profile_ok.json"))
		case "/api/channels/feed/comment/list":
			if r.URL.Query().Get("oid") != "fixture-work-id" || r.URL.Query().Get("nid") != "fixture-nonce-id" {
				t.Errorf("work query=%s", r.URL.RawQuery)
			}
			switch r.URL.Query().Get("next_marker") {
			case "":
				_, _ = w.Write(readFixture(t, "ltaoo_comments_page_1.json"))
			case "fixture-next-marker":
				if r.URL.Query().Get("comment_id") != "fixture-root-id" {
					t.Errorf("root=%q", r.URL.Query().Get("comment_id"))
				}
				_, _ = w.Write(readFixture(t, "ltaoo_comments_page_2.json"))
			default:
				t.Errorf("unexpected cursor=%q", r.URL.Query().Get("next_marker"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	work, err := client.ResolveWork(context.Background(), shareURL, 1)
	if err != nil {
		t.Fatal(err)
	}
	if dereference(work.WorkID) != "fixture-work-id" || dereference(work.ObjectNonceID) != "fixture-nonce-id" ||
		dereference(work.Title) != "fixture title" || dereference(work.Author.AccountID) != "fixture-account-id" ||
		dereference(work.Locator.PublicURL) != shareURL {
		t.Fatalf("work=%+v", work)
	}

	first, err := client.Call(context.Background(), commentListMethod, map[string]any{
		"object_id": "fixture-work-id", "nonce_id": "fixture-nonce-id", "next_marker": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, marker, present, _, err := parseCommentPage(first)
	if err != nil || !present || marker != "fixture-next-marker" {
		t.Fatalf("marker=%q present=%t err=%v", marker, present, err)
	}
	second, err := client.Call(context.Background(), commentListMethod, map[string]any{
		"object_id": "fixture-work-id", "nonce_id": "fixture-nonce-id", "comment_id": "fixture-root-id", "next_marker": marker,
	})
	if err != nil {
		t.Fatal(err)
	}
	items, finalMarker, present, _, err := parseCommentPage(second)
	if err != nil || !present || finalMarker != "" || len(items) != 1 {
		t.Fatalf("items=%d marker=%q present=%t err=%v", len(items), finalMarker, present, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("requests=%v", requests)
	}
}

func TestNewLtaooClientRejectsUnsafeAPIBase(t *testing.T) {
	for _, base := range []string{
		"https://127.0.0.1:2023", "http://localhost:2023", "http://0.0.0.0:2023",
		"http://127.0.0.1", "http://user@127.0.0.1:2023", "http://127.0.0.1:2023/other",
		"http://127.0.0.1:2023?query=1", "http://127.0.0.1:2023/#fragment",
	} {
		t.Run(base, func(t *testing.T) {
			if _, err := NewLtaooClient(base, time.Second); err == nil {
				t.Fatal("unsafe API base accepted")
			}
		})
	}
}

func TestLtaooClientRejectsRedirectOversizeAndBusinessErrorsWithoutLeak(t *testing.T) {
	const secret = "NEVER_LEAK_PROFILE_OR_RESPONSE_SECRET"
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://example.com/"+secret, http.StatusFound)
		}},
		{"oversize", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, strings.Repeat("x", maxLtaooResponseBytes+1))
		}},
		{"business", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprintf(w, `{"code":0,"data":{"errCode":7,"errMsg":%q}}`, secret)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			client, err := NewLtaooClient(server.URL, 2*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ResolveWork(context.Background(), "https://weixin.qq.com/sph/"+secret, 1)
			if err == nil {
				t.Fatal("unsafe response accepted")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("secret leaked in error: %v", err)
			}
		})
	}
}

func TestCollectWorksFromURLsPreservesOrderAndDeduplicatesRequestsAndWorks(t *testing.T) {
	var mu sync.Mutex
	profileCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		profileCalls++
		mu.Unlock()
		shareURL := r.URL.Query().Get("url")
		switch {
		case strings.HasSuffix(shareURL, "/first"):
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"work-1","objectNonceId":"nonce-1"}}}}`)
		case strings.HasSuffix(shareURL, "/same-work"):
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"work-1","objectNonceId":"nonce-1"}}}}`)
		case strings.HasSuffix(shareURL, "/broken"):
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":9}}`)
		default:
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"work-2","objectNonceId":"nonce-2"}}}}`)
		}
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	urls := []string{
		"https://weixin.qq.com/sph/first",
		"https://weixin.qq.com/sph/first",
		"https://weixin.qq.com/sph/same-work",
		"https://weixin.qq.com/sph/broken",
		"https://weixin.qq.com/sph/second",
	}
	works, issues := CollectWorksFromURLs(context.Background(), client, urls, 10)
	if len(works) != 2 || dereference(works[0].WorkID) != "work-1" || dereference(works[1].WorkID) != "work-2" {
		t.Fatalf("works=%+v", works)
	}
	if len(issues) != 1 || issues[0].Code != "profile_schema_mismatch" || issues[0].InputIndex != 4 {
		t.Fatalf("issues=%+v", issues)
	}
	mu.Lock()
	defer mu.Unlock()
	if profileCalls != 4 {
		t.Fatalf("profile calls=%d", profileCalls)
	}
}

func TestCollectWorksFromURLsProfileRetriesTransientUntilReady(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			http.NotFound(w, r)
			return
		}
		requests++
		if requests < 3 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"ready-work","objectNonceId":"ready-nonce"}}}}`)
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clock := &profileTestClock{now: time.Unix(100, 0)}

	works, issues := collectWorksFromURLs(context.Background(), client, []string{"https://weixin.qq.com/sph/ready"}, 1, profileReadinessOptions{
		Clock: clock, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
	})

	if len(works) != 1 || dereference(works[0].WorkID) != "ready-work" || len(issues) != 0 {
		t.Fatalf("works=%+v issues=%+v", works, issues)
	}
	if requests != 3 || len(clock.sleeps) != 2 {
		t.Fatalf("requests=%d sleeps=%v", requests, clock.sleeps)
	}
}

func TestCollectWorksFromURLsProfileCode400WithoutDataRetriesUntilReady(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			http.NotFound(w, r)
			return
		}
		requests++
		if requests < 3 {
			_, _ = fmt.Fprint(w, `{"code":400}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"ready-after-code400","objectNonceId":"ready-after-code400-nonce"}}}}`)
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clock := &profileTestClock{now: time.Unix(150, 0)}

	works, issues := collectWorksFromURLs(context.Background(), client, []string{"https://weixin.qq.com/sph/code400-ready"}, 1, profileReadinessOptions{
		Clock: clock, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
	})

	if len(works) != 1 || dereference(works[0].WorkID) != "ready-after-code400" || len(issues) != 0 {
		t.Fatalf("works=%+v issues=%+v", works, issues)
	}
	if requests != 3 || len(clock.sleeps) != 2 {
		t.Fatalf("requests=%d sleeps=%v", requests, clock.sleeps)
	}
}

func TestCollectWorksFromURLsProfileCode400AfterReadinessDoesNotRetry(t *testing.T) {
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			http.NotFound(w, r)
			return
		}
		shareURL := r.URL.Query().Get("url")
		requests[shareURL]++
		if strings.HasSuffix(shareURL, "/ready") {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"ready-work","objectNonceId":"ready-nonce"}}}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":400}`)
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clock := &profileTestClock{now: time.Unix(175, 0)}
	urls := []string{"https://weixin.qq.com/sph/ready", "https://weixin.qq.com/sph/code400-after-ready"}

	works, issues := collectWorksFromURLs(context.Background(), client, urls, 2, profileReadinessOptions{
		Clock: clock, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
	})

	if len(works) != 1 || len(issues) != 1 || issues[0].Code != "profile_unavailable" || issues[0].InputIndex != 2 {
		t.Fatalf("works=%+v issues=%+v", works, issues)
	}
	if requests[urls[0]] != 1 || requests[urls[1]] != 1 || len(clock.sleeps) != 0 {
		t.Fatalf("requests=%v sleeps=%v", requests, clock.sleeps)
	}
}

func TestCollectWorksFromURLsProfileBridgeNotReadyRetriesUntilReady(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			http.NotFound(w, r)
			return
		}
		requests++
		if requests < 3 {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":1011,"errMsg":"page_api_failed"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"ready-after-bridge","objectNonceId":"ready-after-bridge-nonce"}}}}`)
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clock := &profileTestClock{now: time.Unix(185, 0)}

	works, issues := collectWorksFromURLs(context.Background(), client, []string{"https://weixin.qq.com/sph/bridge-ready"}, 1, profileReadinessOptions{
		Clock: clock, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
	})

	if len(works) != 1 || dereference(works[0].WorkID) != "ready-after-bridge" || len(issues) != 0 {
		t.Fatalf("works=%+v issues=%+v", works, issues)
	}
	if requests != 3 || len(clock.sleeps) != 2 {
		t.Fatalf("requests=%d sleeps=%v", requests, clock.sleeps)
	}
}

func TestCollectWorksFromURLsProfileBridgeErrorAfterReadinessDoesNotRetry(t *testing.T) {
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" {
			http.NotFound(w, r)
			return
		}
		shareURL := r.URL.Query().Get("url")
		requests[shareURL]++
		if strings.HasSuffix(shareURL, "/ready") {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"ready-work","objectNonceId":"ready-nonce"}}}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":1011,"errMsg":"page_api_failed"}}`)
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clock := &profileTestClock{now: time.Unix(190, 0)}
	urls := []string{"https://weixin.qq.com/sph/ready", "https://weixin.qq.com/sph/bridge-after-ready"}

	works, issues := collectWorksFromURLs(context.Background(), client, urls, 2, profileReadinessOptions{
		Clock: clock, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
	})

	if len(works) != 1 || len(issues) != 1 || issues[0].Code != "profile_unavailable" || issues[0].InputIndex != 2 {
		t.Fatalf("works=%+v issues=%+v", works, issues)
	}
	if requests[urls[0]] != 1 || requests[urls[1]] != 1 || len(clock.sleeps) != 0 {
		t.Fatalf("requests=%v sleeps=%v", requests, clock.sleeps)
	}
}

func TestCollectWorksFromURLsSharedReadinessDeadlineAppliesToWholeBatch(t *testing.T) {
	var mu sync.Mutex
	requestedURLs := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedURLs = append(requestedURLs, r.URL.Query().Get("url"))
		mu.Unlock()
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(200, 0)
	clock := &profileTestClock{now: start}
	urls := []string{
		"https://weixin.qq.com/sph/first",
		"https://weixin.qq.com/sph/second",
		"https://weixin.qq.com/sph/third",
	}

	works, issues := collectWorksFromURLs(context.Background(), client, urls, 3, profileReadinessOptions{
		Clock: clock, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
	})

	if len(works) != 0 || len(issues) != 3 {
		t.Fatalf("works=%+v issues=%+v", works, issues)
	}
	for index, issue := range issues {
		if issue.Code != "profile_not_ready" || issue.InputIndex != index+1 {
			t.Fatalf("issues=%+v", issues)
		}
	}
	if elapsed := clock.Now().Sub(start); elapsed != 30*time.Second {
		t.Fatalf("shared readiness elapsed=%s", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requestedURLs) < 2 {
		t.Fatalf("expected retries, requests=%v", requestedURLs)
	}
	for _, requestedURL := range requestedURLs {
		if requestedURL != urls[0] {
			t.Fatalf("later URL requested after shared timeout: %v", requestedURLs)
		}
	}
}

func TestCollectWorksFromURLsProfileClassifiesClosedErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   string
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantCode: "profile_access_denied"},
		{name: "forbidden", statusCode: http.StatusForbidden, wantCode: "profile_access_denied"},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, wantCode: "profile_rate_limited"},
		{name: "malformed JSON", statusCode: http.StatusOK, body: `{`, wantCode: "profile_schema_mismatch"},
		{name: "invalid business envelope", statusCode: http.StatusOK, body: `{"code":0,"data":{"errCode":9}}`, wantCode: "profile_schema_mismatch"},
		{name: "unknown status", statusCode: http.StatusTeapot, wantCode: "profile_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			client, err := NewLtaooClient(server.URL, 2*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			clock := &profileTestClock{now: time.Unix(300, 0)}

			works, issues := collectWorksFromURLs(context.Background(), client, []string{"https://weixin.qq.com/sph/classify"}, 1, profileReadinessOptions{
				Clock: clock, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
			})

			if len(works) != 0 || len(issues) != 1 || issues[0].Code != test.wantCode {
				t.Fatalf("works=%+v issues=%+v", works, issues)
			}
			if len(clock.sleeps) != 0 {
				t.Fatalf("non-transient profile error retried: %v", clock.sleeps)
			}
		})
	}
}

func TestCollectWorksFromURLsProfileSchemaErrorDoesNotBlockLaterReadiness(t *testing.T) {
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shareURL := r.URL.Query().Get("url")
		requests[shareURL]++
		if strings.HasSuffix(shareURL, "/broken") {
			_, _ = fmt.Fprint(w, `{`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"later-work","objectNonceId":"later-nonce"}}}}`)
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	urls := []string{"https://weixin.qq.com/sph/broken", "https://weixin.qq.com/sph/later"}

	works, issues := collectWorksFromURLs(context.Background(), client, urls, 2, profileReadinessOptions{
		Clock: &profileTestClock{now: time.Unix(400, 0)}, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
	})

	if len(works) != 1 || dereference(works[0].WorkID) != "later-work" {
		t.Fatalf("works=%+v", works)
	}
	if len(issues) != 1 || issues[0].Code != "profile_schema_mismatch" || issues[0].InputIndex != 1 {
		t.Fatalf("issues=%+v", issues)
	}
	if requests[urls[0]] != 1 || requests[urls[1]] != 1 {
		t.Fatalf("requests=%v", requests)
	}
}

func TestCollectWorksFromURLsCancellationStopsReadinessRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		cancel()
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	works, issues := collectWorksFromURLs(ctx, client, []string{
		"https://weixin.qq.com/sph/first", "https://weixin.qq.com/sph/second",
	}, 2, profileReadinessOptions{
		Clock: &profileTestClock{now: time.Unix(500, 0)}, Timeout: 30 * time.Second, RetryInterval: 500 * time.Millisecond,
	})

	if len(works) != 0 || len(issues) != 1 || issues[0].Code != "collection_cancelled" || issues[0].InputIndex != 1 {
		t.Fatalf("works=%+v issues=%+v", works, issues)
	}
	if requests != 1 {
		t.Fatalf("requests=%d", requests)
	}
}

func TestCollectCommentsThroughLtaooClientKeepsReplyCursorAndRelations(t *testing.T) {
	const cursor = "fixture-reply-cursor"
	var mu sync.Mutex
	requests := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/comment/list" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		requests = append(requests, r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Query().Get("oid") != "fixture-live-work" || r.URL.Query().Get("nid") != "fixture-live-nonce" {
			t.Errorf("work query=%s", r.URL.RawQuery)
		}
		switch {
		case r.URL.Query().Get("comment_id") == "":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"fixture-live-root","expandCommentCount":3}],"lastBuffer":""}}}`)
		case r.URL.Query().Get("next_marker") == "":
			_, _ = fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"fixture-live-reply-1","replyCommentId":"fixture-live-root","rootCommentId":"fixture-live-root"},{"commentId":"fixture-live-reply-2","replyCommentId":"fixture-live-root","rootCommentId":"fixture-live-root"}],"lastBuffer":%q}}}`, cursor)
		case r.URL.Query().Get("next_marker") == cursor:
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"fixture-live-reply-3","replyCommentId":"fixture-live-reply-2","rootCommentId":"fixture-live-root"}],"lastBuffer":""}}}`)
		default:
			t.Errorf("unexpected reply query=%s", r.URL.RawQuery)
		}
	}))
	defer server.Close()
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	collector := NewCollector(client, NewEvidenceRecorder(nil), newTestStore(t, "ltaoo-comments-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-live-work", "fixture-live-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if summary.TopLevel != 1 || summary.Replies != 3 || len(comments) != 4 {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
	}
	lastReply := findComment(t, comments, "fixture-live-reply-3")
	if dereference(lastReply.ParentCommentID) != "fixture-live-reply-2" || dereference(lastReply.RootCommentID) != "fixture-live-root" ||
		dereference(lastReply.RetrievalRootCommentID) != "fixture-live-root" {
		t.Fatalf("last reply=%+v", lastReply)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 || !strings.Contains(requests[2], "next_marker="+url.QueryEscape(cursor)) {
		t.Fatalf("requests=%v", requests)
	}
}

func TestNormalizeWeChatShareURLRejectsNonPublicShareLinks(t *testing.T) {
	for _, raw := range []string{
		"http://weixin.qq.com/sph/a", "https://example.com/sph/a", "https://weixin.qq.com/not-sph/a",
		"https://user@weixin.qq.com/sph/a", "https://weixin.qq.com/sph/a?q=1", "https://weixin.qq.com/sph/a#x",
	} {
		if _, err := NormalizeWeChatShareURL(raw); err == nil {
			t.Fatalf("unsafe share URL accepted: %s", raw)
		}
	}
	got, err := NormalizeWeChatShareURL("  https://weixin.qq.com/sph/AbC/  ")
	if err != nil || got != "https://weixin.qq.com/sph/AbC" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
