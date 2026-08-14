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
	if len(issues) != 1 || issues[0].Code != "profile_unavailable" || issues[0].InputIndex != 4 {
		t.Fatalf("issues=%+v", issues)
	}
	mu.Lock()
	defer mu.Unlock()
	if profileCalls != 4 {
		t.Fatalf("profile calls=%d", profileCalls)
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
