//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func probeRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func probePowerShell(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"powershell.exe", "powershell"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("Windows PowerShell is not available")
	return ""
}

func runProbeScript(t *testing.T, script string, args ...string) ([]byte, error) {
	t.Helper()
	commandArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(probeRepoRoot(t), "scripts", script)}
	commandArgs = append(commandArgs, args...)
	return exec.Command(probePowerShell(t), commandArgs...).CombinedOutput()
}

func readJSONFile(t *testing.T, path string, out any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("invalid JSON in %s: %v", path, err)
	}
}

func cleanupProbeTestRun(t *testing.T, runRoot string) {
	t.Helper()
	base := filepath.Clean(filepath.Join(probeRepoRoot(t), ".tmp_runtime", "ltaoo-probe"))
	target := filepath.Clean(runRoot)
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatalf("refusing unsafe test cleanup target %q", target)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Errorf("test cleanup failed: %v", err)
	}
}

func writeProbeManifest(t *testing.T, runID, apiBase string) string {
	t.Helper()
	runRoot := filepath.Join(probeRepoRoot(t), ".tmp_runtime", "ltaoo-probe", runID)
	t.Cleanup(func() { cleanupProbeTestRun(t, runRoot) })
	if err := os.MkdirAll(runRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"schema_version":1,"run_id":%q,"runtime_root":%q,"ltaoo":{"api_base":%q}}`, runID, runRoot, apiBase)
	if err := os.WriteFile(filepath.Join(runRoot, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return runRoot
}

func TestLtaooProbeScriptsExist(t *testing.T) {
	for _, name := range []string{
		"prepare-ltaoo-probe.ps1",
		"probe-ltaoo-comments.ps1",
		"probe-ltaoo-replies.ps1",
		"cleanup-ltaoo-probe.ps1",
	} {
		if _, err := os.Stat(filepath.Join(probeRepoRoot(t), "scripts", name)); err != nil {
			t.Errorf("%s must exist: %v", name, err)
		}
	}
}

func TestProbeFetchesExactlyTwoPagesAndRedacts(t *testing.T) {
	const shareURL = "https://weixin.qq.com/sph/FixtureShareSecret"
	const oid = "fixture-oid-secret"
	const nid = "fixture-nid-secret"
	const marker = "cursor+/=fixture-secret"
	const bait = "NEVER_PERSIST_COMMENT_BODY_OR_NICKNAME"

	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			if got := r.URL.Query().Get("url"); got != shareURL {
				t.Errorf("share URL = %q", got)
			}
			fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":%q,"objectNonceId":%q}}}}`, oid, nid)
		case "/api/channels/feed/comment/list":
			if r.URL.Query().Get("oid") != oid || r.URL.Query().Get("nid") != nid {
				t.Errorf("missing oid/nid")
			}
			switch r.URL.Query().Get("next_marker") {
			case "":
				fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"comment-one-secret","content":%q,"nickname":%q}],"lastBuffer":%q}}}`, bait, bait, marker)
			case marker:
				fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"comment-two-secret","content":%q,"nickname":%q}],"lastBuffer":""}}}`, bait, bait)
			default:
				t.Errorf("unexpected marker %q", r.URL.Query().Get("next_marker"))
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-two-pages"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-comments.ps1", "-RunId", runID, "-ShareUrl", shareURL, "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err != nil {
		t.Fatalf("probe failed: %v\n%s", err, output)
	}

	raw, err := os.ReadFile(filepath.Join(runRoot, "probe-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range []string{shareURL, oid, nid, marker, bait, "comment-one-secret", "comment-two-secret"} {
		if strings.Contains(text, secret) || strings.Contains(string(output), secret) {
			t.Errorf("secret leaked: %q", secret)
		}
	}
	var summary struct {
		Status              string `json:"status"`
		StatusSchema        string `json:"status_schema"`
		ReadinessProof      string `json:"readiness_proof"`
		CommentRequestCount int    `json:"comment_request_count"`
		CursorContinuity    bool   `json:"cursor_continuity"`
		Pages               []struct {
			CommentCount int `json:"comment_count"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Status != "verified_two_pages" || summary.CommentRequestCount != 2 || !summary.CursorContinuity {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.StatusSchema != "modern" || summary.ReadinessProof != "listeners_and_profile" {
		t.Fatalf("unexpected readiness evidence: %+v", summary)
	}
	if len(summary.Pages) != 2 || summary.Pages[0].CommentCount != 1 || summary.Pages[1].CommentCount != 1 {
		t.Fatalf("unexpected pages: %+v", summary.Pages)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("request count = %d, want status + profile + two comment pages", len(requests))
	}
}

type replyProbeSummary struct {
	Status                       string `json:"status"`
	ReasonCode                   string `json:"reason_code"`
	StatusSchema                 string `json:"status_schema"`
	ReadinessProof               string `json:"readiness_proof"`
	TopLevelRequestCount         int    `json:"top_level_request_count"`
	ReplyRequestCount            int    `json:"reply_request_count"`
	CommentRequestCount          int    `json:"comment_request_count"`
	SecondReplyRequestCursorHash string `json:"second_reply_request_cursor_hash"`
	CursorContinuity             bool   `json:"cursor_continuity"`
	TopPage                      struct {
		CommentCount      int  `json:"comment_count"`
		EligibleRootFound bool `json:"eligible_root_found"`
	} `json:"top_page"`
	SelectedRoot *struct {
		CommentIDHash          string   `json:"comment_id_hash"`
		ExpandCommentCount     int64    `json:"expand_comment_count"`
		EmbeddedReplyCount     int      `json:"embedded_reply_count"`
		EmbeddedMissingIDCount int      `json:"embedded_missing_id_count"`
		EmbeddedReplyIDHashes  []string `json:"embedded_reply_id_hashes"`
	} `json:"selected_root"`
	ReplyPages []struct {
		PageNumber                   int      `json:"page_number"`
		ReplyCount                   int      `json:"reply_count"`
		MissingIDCount               int      `json:"missing_id_count"`
		ReplyIDHashes                []string `json:"reply_id_hashes"`
		PageDuplicateCount           int      `json:"page_duplicate_count"`
		CrossReplyPageDuplicateCount int      `json:"cross_reply_page_duplicate_count"`
		EmbeddedDuplicateCount       int      `json:"embedded_duplicate_count"`
		RelationMatchCount           int      `json:"relation_match_count"`
		RelationGapCount             int      `json:"relation_gap_count"`
		RelationMismatchCount        int      `json:"relation_mismatch_count"`
		LastBufferPresent            bool     `json:"last_buffer_present"`
		LastBufferHash               string   `json:"last_buffer_hash"`
	} `json:"reply_pages"`
	Totals struct {
		ReplyCount                   int `json:"reply_count"`
		MissingIDCount               int `json:"missing_id_count"`
		PageDuplicateCount           int `json:"page_duplicate_count"`
		CrossReplyPageDuplicateCount int `json:"cross_reply_page_duplicate_count"`
		EmbeddedDuplicateCount       int `json:"embedded_duplicate_count"`
		RelationMatchCount           int `json:"relation_match_count"`
		RelationGapCount             int `json:"relation_gap_count"`
		RelationMismatchCount        int `json:"relation_mismatch_count"`
	} `json:"totals"`
}

func TestReplyProbeSelectsFirstEligibleRootAndFetchesExactlyTwoReplyPages(t *testing.T) {
	const shareURL = "https://weixin.qq.com/sph/ReplyFixtureShareSecret"
	const oid = "reply-fixture-oid-secret"
	const nid = "reply-fixture-nid-secret"
	const rootID = "selected-root-secret"
	const embeddedOne = "embedded-reply-one-secret"
	const embeddedTwo = "embedded-reply-two-secret"
	const marker = "reply+cursor/with=reserved"
	const bait = "NEVER_PERSIST_REPLY_BODY_OR_NICKNAME"

	var mu sync.Mutex
	commentRequests := 0
	replyRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			if got := r.URL.Query().Get("url"); got != shareURL {
				t.Errorf("share URL = %q", got)
			}
			fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":%q,"objectNonceId":%q}}}}`, oid, nid)
		case "/api/channels/feed/comment/list":
			mu.Lock()
			commentRequests++
			requestNumber := commentRequests
			mu.Unlock()
			if requestNumber > 3 {
				t.Errorf("unexpected fourth comment request: %s", r.URL.RequestURI())
				http.Error(w, "request budget exceeded", http.StatusTooManyRequests)
				return
			}
			if r.URL.Query().Get("oid") != oid || r.URL.Query().Get("nid") != nid {
				t.Errorf("missing oid/nid")
			}
			commentID := r.URL.Query().Get("comment_id")
			nextMarker := r.URL.Query().Get("next_marker")
			if commentID == "" {
				if nextMarker != "" {
					t.Errorf("top-level marker must be empty, got %q", nextMarker)
				}
				fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"","expandCommentCount":9,"levelTwoComment":[]},{"commentId":"fully-embedded-root","expandCommentCount":1,"levelTwoComment":[{"commentId":"embedded-full"}]},{"commentId":"invalid-count-root","expandCommentCount":"NaN","levelTwoComment":[]},{"commentId":%q,"expandCommentCount":6,"levelTwoComment":[{"commentId":%q,"content":%q},{"commentId":%q,"nickname":%q}]},{"commentId":"later-eligible-root","expandCommentCount":99,"levelTwoComment":[]}],"lastBuffer":"TOP_LEVEL_MARKER_MUST_NOT_BE_USED"}}}`, rootID, embeddedOne, bait, embeddedTwo, bait)
				return
			}
			mu.Lock()
			replyRequests++
			mu.Unlock()
			if commentID != rootID {
				t.Errorf("wrong root selected: %q", commentID)
			}
			switch nextMarker {
			case "":
				fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":%q,"replyCommentId":%q,"rootCommentId":%q,"content":%q},{"commentId":"page-one-unique","replyCommentId":"0","rootCommentId":%q},{"commentId":"page-one-unique","replyCommentId":%q},{"commentId":"","replyCommentId":%q,"rootCommentId":%q}],"lastBuffer":%q}}}`, embeddedOne, rootID, rootID, bait, rootID, rootID, rootID, rootID, marker)
			case marker:
				fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"page-one-unique","replyCommentId":%q,"rootCommentId":%q},{"commentId":"page-two-unique","replyCommentId":%q,"rootCommentId":%q}],"lastBuffer":"DO_NOT_REQUEST_REPLY_PAGE_THREE"}}}`, rootID, rootID, rootID, rootID)
			default:
				t.Errorf("unexpected reply marker %q", nextMarker)
				http.Error(w, "bad marker", http.StatusBadRequest)
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-reply-two-pages"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", shareURL, "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err != nil {
		t.Fatalf("reply probe failed: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(runRoot, "reply-probe-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{shareURL, oid, nid, rootID, embeddedOne, embeddedTwo, marker, bait,
		"fully-embedded-root", "invalid-count-root", "later-eligible-root", "page-one-unique", "page-two-unique",
		"TOP_LEVEL_MARKER_MUST_NOT_BE_USED", "DO_NOT_REQUEST_REPLY_PAGE_THREE"} {
		if strings.Contains(string(raw), secret) || strings.Contains(string(output), secret) {
			t.Errorf("reply secret leaked: %q", secret)
		}
	}
	var summary replyProbeSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Status != "verified_reply_two_pages" || summary.ReasonCode != "reply_two_pages_verified" {
		t.Fatalf("unexpected result: %+v", summary)
	}
	if summary.StatusSchema != "modern" || summary.ReadinessProof != "listeners_and_profile" {
		t.Fatalf("unexpected readiness: %+v", summary)
	}
	if summary.TopLevelRequestCount != 1 || summary.ReplyRequestCount != 2 || summary.CommentRequestCount != 3 {
		t.Fatalf("unexpected budgets: %+v", summary)
	}
	if !summary.CursorContinuity || summary.SecondReplyRequestCursorHash == "" || summary.SecondReplyRequestCursorHash != summary.ReplyPages[0].LastBufferHash {
		t.Fatalf("cursor continuity not proven: %+v", summary)
	}
	if summary.SelectedRoot == nil || summary.SelectedRoot.ExpandCommentCount != 6 || summary.SelectedRoot.EmbeddedReplyCount != 2 {
		t.Fatalf("wrong root selected: %+v", summary.SelectedRoot)
	}
	if len(summary.ReplyPages) != 2 || summary.ReplyPages[0].PageDuplicateCount != 1 || summary.ReplyPages[0].EmbeddedDuplicateCount != 1 || summary.ReplyPages[1].CrossReplyPageDuplicateCount != 1 {
		t.Fatalf("wrong duplicate evidence: %+v", summary.ReplyPages)
	}
	if summary.Totals.RelationMatchCount != 10 || summary.Totals.RelationGapCount != 2 || summary.Totals.RelationMismatchCount != 0 {
		t.Fatalf("wrong relation evidence: %+v", summary.Totals)
	}
	mu.Lock()
	defer mu.Unlock()
	if commentRequests != 3 || replyRequests != 2 {
		t.Fatalf("server observed comment=%d reply=%d", commentRequests, replyRequests)
	}
}

func TestReplyProbeStopsOnExplicitRelationMismatch(t *testing.T) {
	const rootID = "relation-root-secret"
	commentRequests := 0
	replyPageTwoRequested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"relation-oid","objectNonceId":"relation-nid"}}}}`)
		case "/api/channels/feed/comment/list":
			commentRequests++
			if r.URL.Query().Get("comment_id") == "" {
				fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":%q,"expandCommentCount":2,"levelTwoComment":[]}],"lastBuffer":""}}}`, rootID)
				return
			}
			if r.URL.Query().Get("next_marker") != "" {
				replyPageTwoRequested = true
			}
			fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"relation-reply-secret","replyCommentId":"wrong-root-secret","rootCommentId":%q}],"lastBuffer":"must-not-be-used"}}}`, rootID)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-reply-relation-mismatch"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/relation-secret", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err == nil {
		t.Fatalf("relation mismatch accepted: %s", output)
	}
	var summary replyProbeSummary
	summaryPath := filepath.Join(runRoot, "reply-probe-summary.json")
	readJSONFile(t, summaryPath, &summary)
	if summary.Status != "failed" || summary.ReasonCode != "reply_relation_mismatch" || summary.CommentRequestCount != 2 || summary.ReplyRequestCount != 1 {
		t.Fatalf("unexpected mismatch result: %+v output=%s", summary, output)
	}
	if len(summary.ReplyPages) != 1 || summary.ReplyPages[0].RelationMismatchCount != 1 || summary.Totals.RelationMismatchCount != 1 {
		t.Fatalf("mismatch evidence missing: %+v", summary)
	}
	if commentRequests != 2 || replyPageTwoRequested {
		t.Fatalf("probe continued after mismatch: requests=%d pageTwo=%v", commentRequests, replyPageTwoRequested)
	}
	failureRaw, readErr := os.ReadFile(summaryPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, secret := range []string{rootID, "relation-reply-secret", "wrong-root-secret", "must-not-be-used", "relation-secret"} {
		if strings.Contains(string(output), secret) || strings.Contains(string(failureRaw), secret) {
			t.Errorf("mismatch secret leaked: %q", secret)
		}
	}
}

func TestReplyProbeTreatsEmptyEmbeddedListAsZero(t *testing.T) {
	commentRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"empty-embedded-oid","objectNonceId":"empty-embedded-nid"}}}}`)
		case "/api/channels/feed/comment/list":
			commentRequests++
			if r.URL.Query().Get("comment_id") == "" {
				fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"empty-embedded-root","expandCommentCount":1,"levelTwoComment":[]}],"lastBuffer":""}}}`)
				return
			}
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[],"lastBuffer":""}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-reply-empty-embedded"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/empty-embedded", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err != nil {
		t.Fatalf("empty embedded list rejected: %v\n%s", err, output)
	}
	var summary replyProbeSummary
	readJSONFile(t, filepath.Join(runRoot, "reply-probe-summary.json"), &summary)
	if summary.Status != "inconclusive_no_second_reply_page" || summary.SelectedRoot == nil || summary.SelectedRoot.EmbeddedReplyCount != 0 || commentRequests != 2 {
		t.Fatalf("empty list did not count as zero: %+v requests=%d", summary, commentRequests)
	}
}

func TestReplyProbeStopsWhenNoEligibleRoot(t *testing.T) {
	commentRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"no-root-oid","objectNonceId":"no-root-nid"}}}}`)
		case "/api/channels/feed/comment/list":
			commentRequests++
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"","expandCommentCount":9,"levelTwoComment":[]},{"commentId":"fully-embedded","expandCommentCount":1,"levelTwoComment":[{"commentId":"embedded"}]},{"commentId":"invalid-count","expandCommentCount":"1","levelTwoComment":[]},{"commentId":"negative-count","expandCommentCount":-1,"levelTwoComment":[]}],"lastBuffer":"unused-top-marker"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-reply-no-root"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/no-root-secret", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err != nil {
		t.Fatalf("inconclusive probe failed: %v\n%s", err, output)
	}
	var summary replyProbeSummary
	readJSONFile(t, filepath.Join(runRoot, "reply-probe-summary.json"), &summary)
	if summary.Status != "inconclusive_no_eligible_root" || summary.ReasonCode != "top_page_has_no_eligible_root" ||
		summary.TopLevelRequestCount != 1 || summary.ReplyRequestCount != 0 || summary.CommentRequestCount != 1 ||
		summary.SelectedRoot != nil || len(summary.ReplyPages) != 0 || commentRequests != 1 {
		t.Fatalf("unexpected no-root result: %+v requests=%d", summary, commentRequests)
	}
}

func TestReplyProbeStopsWhenReplyPageOneHasNoMarker(t *testing.T) {
	commentRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"single-page-oid","objectNonceId":"single-page-nid"}}}}`)
		case "/api/channels/feed/comment/list":
			commentRequests++
			if r.URL.Query().Get("comment_id") == "" {
				fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"single-page-root","expandCommentCount":2,"levelTwoComment":[]}],"lastBuffer":""}}}`)
				return
			}
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"single-reply","replyCommentId":"single-page-root","rootCommentId":"0"}],"lastBuffer":""}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-reply-no-second-page"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/single-page-secret", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err != nil {
		t.Fatalf("single reply page probe failed: %v\n%s", err, output)
	}
	var summary replyProbeSummary
	readJSONFile(t, filepath.Join(runRoot, "reply-probe-summary.json"), &summary)
	if summary.Status != "inconclusive_no_second_reply_page" || summary.ReasonCode != "reply_page_one_has_no_marker" ||
		summary.TopLevelRequestCount != 1 || summary.ReplyRequestCount != 1 || summary.CommentRequestCount != 2 ||
		summary.CursorContinuity || summary.SecondReplyRequestCursorHash != "" || len(summary.ReplyPages) != 1 || commentRequests != 2 {
		t.Fatalf("unexpected single-page result: %+v requests=%d", summary, commentRequests)
	}
}

func TestReplyProbeProfileFailureDoesNotRequestComments(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"channels":{"available":false},"version":"legacy-fixture"}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":7,"data":{}}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-reply-profile-failure"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/profile-failure", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err == nil {
		t.Fatalf("profile failure accepted: %s", output)
	}
	var summary replyProbeSummary
	readJSONFile(t, filepath.Join(runRoot, "reply-probe-summary.json"), &summary)
	if summary.StatusSchema != "legacy" || summary.ReadinessProof != "profile" || summary.ReasonCode != "profile_business_error" ||
		summary.CommentRequestCount != 0 || summary.TopLevelRequestCount != 0 || summary.ReplyRequestCount != 0 || requestCount != 2 {
		t.Fatalf("unexpected profile failure: %+v requests=%d", summary, requestCount)
	}
}

func TestReplyProbeRejectsUnknownStatusBeforeProfile(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/status" {
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"ready":true}}`)
	}))
	defer server.Close()

	runID := "test-reply-unknown-status"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/unknown-status", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err == nil {
		t.Fatalf("unknown status accepted: %s", output)
	}
	var summary replyProbeSummary
	readJSONFile(t, filepath.Join(runRoot, "reply-probe-summary.json"), &summary)
	if summary.ReasonCode != "status_schema_error" || summary.CommentRequestCount != 0 || requestCount != 1 {
		t.Fatalf("unexpected status rejection: %+v requests=%d", summary, requestCount)
	}
}

func TestReplyProbeTopPageFailureDoesNotRequestReplies(t *testing.T) {
	commentRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"top-fail-oid","objectNonceId":"top-fail-nid"}}}}`)
		case "/api/channels/feed/comment/list":
			commentRequests++
			fmt.Fprint(w, `{"code":0,"data":{"errCode":12,"data":{}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-reply-top-failure"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/top-failure", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err == nil {
		t.Fatalf("top-page failure accepted: %s", output)
	}
	var summary replyProbeSummary
	readJSONFile(t, filepath.Join(runRoot, "reply-probe-summary.json"), &summary)
	if summary.ReasonCode != "top_page_business_error" || summary.TopLevelRequestCount != 1 || summary.ReplyRequestCount != 0 || summary.CommentRequestCount != 1 || commentRequests != 1 {
		t.Fatalf("unexpected top-page failure: %+v requests=%d", summary, commentRequests)
	}
}

func TestReplyProbeRejectsRemoteAPIBase(t *testing.T) {
	const shareURL = "https://weixin.qq.com/sph/reply-remote-secret"
	runID := "test-reply-remote"
	writeProbeManifest(t, runID, "http://127.0.0.1:2022")
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", shareURL, "-RepoRoot", probeRepoRoot(t), "-ApiBase", "http://192.0.2.10:2022")
	if err == nil {
		t.Fatalf("remote API base accepted: %s", output)
	}
	if !strings.Contains(string(output), "api_base_not_loopback") || strings.Contains(string(output), shareURL) {
		t.Fatalf("wrong or leaking failure: %s", output)
	}
}

func TestReplyProbeCountsTransportAttemptBeforeFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"transport-oid","objectNonceId":"transport-nid"}}}}`)
		case "/api/channels/feed/comment/list":
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("test server cannot hijack connection")
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack failed: %v", err)
				return
			}
			_ = connection.Close()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-reply-transport-failure"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-replies.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/transport-failure", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err == nil {
		t.Fatalf("transport failure accepted: %s", output)
	}
	var summary replyProbeSummary
	readJSONFile(t, filepath.Join(runRoot, "reply-probe-summary.json"), &summary)
	if summary.ReasonCode != "top_page_request_failed" || summary.TopLevelRequestCount != 1 || summary.ReplyRequestCount != 0 || summary.CommentRequestCount != 1 {
		t.Fatalf("attempt was not counted: %+v", summary)
	}
}

func TestReplyProbeHasHardRequestCeiling(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(probeRepoRoot(t), "scripts", "probe-ltaoo-replies.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"Assert-CommentRequestBudget", "$CommentCount -ge 3", "$TopCount -ge 1", "$ReplyCount -ge 2",
		"comment_request_limit_exceeded", "reply-probe-summary.json",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("reply probe ceiling missing %q", required)
		}
	}
	lower := strings.ToLower(text)
	for _, forbidden := range []string{"while (", "do {"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("reply probe contains unbounded construct %q", forbidden)
		}
	}
}

func TestProbeAcceptsLegacyStatusAndUsesProfileAsReadinessProof(t *testing.T) {
	const shareURL = "https://weixin.qq.com/sph/LegacyFixtureShareSecret"
	const oid = "legacy-oid-secret"
	const nid = "legacy-nid-secret"
	const marker = "legacy-cursor+/=secret"
	const bait = "NEVER_PERSIST_LEGACY_COMMENT_BODY"

	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"channels":{"available":false},"version":"legacy-fixture"}}`)
		case "/api/channels/feed/profile":
			if got := r.URL.Query().Get("url"); got != shareURL {
				t.Errorf("share URL = %q", got)
			}
			fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":%q,"objectNonceId":%q}}}}`, oid, nid)
		case "/api/channels/feed/comment/list":
			if r.URL.Query().Get("oid") != oid || r.URL.Query().Get("nid") != nid {
				t.Errorf("missing oid/nid")
			}
			switch r.URL.Query().Get("next_marker") {
			case "":
				fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"legacy-comment-one","content":%q}],"lastBuffer":%q}}}`, bait, marker)
			case marker:
				fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"legacy-comment-two","content":%q}],"lastBuffer":""}}}`, bait)
			default:
				t.Errorf("unexpected marker %q", r.URL.Query().Get("next_marker"))
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runID := "test-legacy-two-pages"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-comments.ps1", "-RunId", runID, "-ShareUrl", shareURL, "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err != nil {
		t.Fatalf("legacy probe failed: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(filepath.Join(runRoot, "probe-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{shareURL, oid, nid, marker, bait, "legacy-comment-one", "legacy-comment-two"} {
		if strings.Contains(string(raw), secret) || strings.Contains(string(output), secret) {
			t.Errorf("legacy secret leaked: %q", secret)
		}
	}
	var summary struct {
		Status              string `json:"status"`
		StatusSchema        string `json:"status_schema"`
		ReadinessProof      string `json:"readiness_proof"`
		CommentRequestCount int    `json:"comment_request_count"`
		CursorContinuity    bool   `json:"cursor_continuity"`
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Status != "verified_two_pages" || summary.StatusSchema != "legacy" || summary.ReadinessProof != "profile" || summary.CommentRequestCount != 2 || !summary.CursorContinuity {
		t.Fatalf("unexpected legacy summary: %+v", summary)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("request count = %d, want status + profile + two comment pages", len(requests))
	}
}

func TestProbeRejectsUnusableStatusBeforeProfile(t *testing.T) {
	tests := []struct {
		name, statusBody, reasonCode string
	}{
		{"modern listener is false", `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":false}}}`, "ltaoo_not_ready"},
		{"legacy version is empty", `{"code":0,"data":{"channels":{"available":false},"version":""}}`, "status_schema_error"},
		{"unknown schema", `{"code":0,"data":{"ready":true}}`, "status_schema_error"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/api/status" {
					t.Errorf("unexpected request %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				fmt.Fprint(w, test.statusBody)
			}))
			defer server.Close()
			runID := fmt.Sprintf("test-unusable-status-%d", index)
			runRoot := writeProbeManifest(t, runID, server.URL)
			output, err := runProbeScript(t, "probe-ltaoo-comments.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/test", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
			if err == nil {
				t.Fatalf("unusable status accepted: %s", output)
			}
			var summary struct {
				ReasonCode          string `json:"reason_code"`
				CommentRequestCount int    `json:"comment_request_count"`
			}
			readJSONFile(t, filepath.Join(runRoot, "probe-summary.json"), &summary)
			if summary.ReasonCode != test.reasonCode || summary.CommentRequestCount != 0 || requestCount != 1 {
				t.Fatalf("unexpected rejection: summary=%+v requests=%d output=%s", summary, requestCount, output)
			}
		})
	}
}

func TestProbeLegacyProfileFailureDoesNotRequestComments(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"channels":{"available":false},"version":"legacy-fixture"}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":7,"data":{}}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	runID := "test-legacy-profile-failure"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-comments.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/test", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err == nil {
		t.Fatalf("profile failure accepted: %s", output)
	}
	var summary struct {
		StatusSchema        string `json:"status_schema"`
		ReadinessProof      string `json:"readiness_proof"`
		ReasonCode          string `json:"reason_code"`
		CommentRequestCount int    `json:"comment_request_count"`
	}
	readJSONFile(t, filepath.Join(runRoot, "probe-summary.json"), &summary)
	if summary.StatusSchema != "legacy" || summary.ReadinessProof != "profile" || summary.ReasonCode != "profile_business_error" || summary.CommentRequestCount != 0 || requestCount != 2 {
		t.Fatalf("unexpected profile failure: summary=%+v requests=%d output=%s", summary, requestCount, output)
	}
}

func TestProbeStopsWhenFirstPageHasNoMarker(t *testing.T) {
	commentRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"oid","objectNonceId":"nid"}}}}`)
		case "/api/channels/feed/comment/list":
			commentRequests++
			fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"one"}],"lastBuffer":""}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	runID := "test-no-marker"
	runRoot := writeProbeManifest(t, runID, server.URL)
	output, err := runProbeScript(t, "probe-ltaoo-comments.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/test", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
	if err != nil {
		t.Fatalf("probe failed: %v\n%s", err, output)
	}
	var summary struct {
		Status              string `json:"status"`
		CommentRequestCount int    `json:"comment_request_count"`
	}
	readJSONFile(t, filepath.Join(runRoot, "probe-summary.json"), &summary)
	if summary.Status != "inconclusive_no_second_page" || summary.CommentRequestCount != 1 || commentRequests != 1 {
		t.Fatalf("unexpected result: summary=%+v requests=%d", summary, commentRequests)
	}
}

func TestProbeRejectsRemoteAPIBase(t *testing.T) {
	runID := "test-remote"
	writeProbeManifest(t, runID, "http://127.0.0.1:2022")
	output, err := runProbeScript(t, "probe-ltaoo-comments.ps1",
		"-RunId", runID,
		"-ShareUrl", "https://weixin.qq.com/sph/test",
		"-RepoRoot", probeRepoRoot(t),
		"-ApiBase", "http://192.0.2.10:2022")
	if err == nil {
		t.Fatalf("remote API base accepted: %s", output)
	}
	if !strings.Contains(string(output), "api_base_not_loopback") {
		t.Fatalf("wrong failure: %s", output)
	}
	if strings.Contains(string(output), "https://weixin.qq.com/sph/test") {
		t.Fatal("share URL leaked in error output")
	}
}

func TestPrepareScriptHasConstrainedConfiguration(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(probeRepoRoot(t), "scripts", "prepare-ltaoo-probe.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"CurrentUser\\Root", "skipInstallRootCert: true", "system: false", "tun: false",
		"127.0.0.1", "Get-NetRoute", "Get-NetTCPConnection", "CertificateRequest",
		"Pkcs8PrivateBlob", "certutil.exe", "-user", "-addstore", "Root", "Cert:\\LocalMachine\\Root",
		"wx_channel", ".tmp_runtime\\ltaoo-probe", "cleanup_not_implemented", "rollback_failed", "  key: \"$keyYaml\"",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("missing safety element %q", required)
		}
	}
	for _, forbidden := range []string{
		"Cert:\\LocalMachine\\Root\\", "system: true", "tun: true", "keyFile:", "Set-ItemProperty", "Set-NetRoute", "New-NetRoute", "Remove-Item -Recurse",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("forbidden preparation behavior %q", forbidden)
		}
	}
}

func TestPrepareRejectsNobiyouExecutableBeforePreparation(t *testing.T) {
	exePath := filepath.Join(probeRepoRoot(t), "wx_channel.exe")
	if _, err := os.Stat(exePath); err != nil {
		t.Skip("tracked nobiyou executable is not present")
	}
	output, err := runProbeScript(t, "prepare-ltaoo-probe.ps1", "-LtaooExePath", exePath, "-RepoRoot", probeRepoRoot(t))
	if err == nil {
		t.Fatalf("nobiyou executable accepted: %s", output)
	}
	if !strings.Contains(string(output), "nobiyou_executable_rejected") {
		t.Fatalf("wrong rejection: %s", output)
	}
	if strings.Contains(string(output), exePath) {
		t.Fatal("executable path leaked in output")
	}
}

func writeCleanupFixture(t *testing.T, runID string, privateKeyOverride string) string {
	t.Helper()
	runRoot := filepath.Join(probeRepoRoot(t), ".tmp_runtime", "ltaoo-probe", runID)
	secrets := filepath.Join(runRoot, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(secrets, "ca-cert.pem")
	keyPath := filepath.Join(secrets, "ca-key.pem")
	configPath := filepath.Join(runRoot, "ltaoo-probe.yaml")
	for path, body := range map[string]string{certPath: "cert", keyPath: "key", configPath: "config"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if privateKeyOverride != "" {
		keyPath = privateKeyOverride
	}
	baseline := `{"schema_version":1,"user_proxy_sha256":"x","winhttp_proxy_sha256":"x","route_table_sha256":"x","current_user_roots_sha256":"x","local_machine_roots_sha256":"x","probe_listeners_sha256":"x","related_processes_sha256":"x"}`
	if err := os.WriteFile(filepath.Join(runRoot, "baseline.json"), []byte(baseline), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"schema_version":1,"run_id":%q,"runtime_root":%q,"ca":{"store":"CurrentUser\\Root","thumbprint":"","certificate_file":%q,"private_key_file":%q},"ltaoo":{"pid":0,"process_start_time":"","executable_sha256":"","config_file":%q,"api_base":"http://127.0.0.1:2022","proxy_endpoint":"127.0.0.1:2023"}}`, runID, runRoot, certPath, keyPath, configPath)
	if err := os.WriteFile(filepath.Join(runRoot, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return runRoot
}

func TestCleanupIsIdempotentForRunOwnedFiles(t *testing.T) {
	runID := "test-cleanup-idempotent"
	runRoot := writeCleanupFixture(t, runID, "")
	t.Cleanup(func() { cleanupProbeTestRun(t, runRoot) })
	for attempt := 0; attempt < 2; attempt++ {
		output, err := runProbeScript(t, "cleanup-ltaoo-probe.ps1", "-RunId", runID, "-RepoRoot", probeRepoRoot(t))
		if err != nil {
			t.Fatalf("cleanup %d failed: %v\n%s", attempt+1, err, output)
		}
	}
	if _, err := os.Stat(filepath.Join(runRoot, "secrets", "ca-key.pem")); !os.IsNotExist(err) {
		t.Fatal("private key remains")
	}
	if _, err := os.Stat(filepath.Join(runRoot, "ltaoo-probe.yaml")); !os.IsNotExist(err) {
		t.Fatal("config remains")
	}
	var receipt struct {
		CleanupSuccess bool `json:"cleanup_success"`
	}
	readJSONFile(t, filepath.Join(runRoot, "cleanup-receipt.json"), &receipt)
	if !receipt.CleanupSuccess {
		t.Fatal("cleanup receipt reports failure")
	}
}

func TestCleanupRejectsExternalManifestTarget(t *testing.T) {
	external := filepath.Join(t.TempDir(), "must-survive.key")
	if err := os.WriteFile(external, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	runID := "test-cleanup-external"
	runRoot := writeCleanupFixture(t, runID, external)
	t.Cleanup(func() { cleanupProbeTestRun(t, runRoot) })
	if output, err := runProbeScript(t, "cleanup-ltaoo-probe.ps1", "-RunId", runID, "-RepoRoot", probeRepoRoot(t)); err == nil {
		t.Fatalf("external target accepted: %s", output)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatalf("external target was changed: %v", err)
	}
}

func TestCleanupScriptSecuritySurface(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(probeRepoRoot(t), "scripts", "cleanup-ltaoo-probe.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"# LTAOO_PROBE_CLEANUP_READY=1",
		"CurrentUser\\Root", "process_start_time", "executable_sha256", "Get-FileHash",
		"-user", "-delstore", "Remove-Item -LiteralPath", "cleanup_success",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("cleanup missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"Cert:\\LocalMachine\\Root\\", "uninstall_by_name", "Remove-Item -Recurse",
		"Set-ItemProperty", "Set-NetRoute", "New-NetRoute",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("cleanup contains forbidden %q", forbidden)
		}
	}
}

func TestLtaooProbeRunbookHasSafetySequence(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(probeRepoRoot(t), "docs", "LTAOO_TWO_PAGE_PROBE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"prepare-ltaoo-probe.ps1", "probe-ltaoo-comments.ps1", "cleanup-ltaoo-probe.ps1",
		"INSTALL", "CurrentUser\\Root", "verified_two_pages", "inconclusive_no_second_page",
		"cleanup_success", "Clash", "system: false", "tun: false",
		"status_schema", "readiness_proof", "channels.available",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("runbook missing %q", required)
		}
	}
}

func TestLtaooReplyProbeRunbookHasSafetySequence(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(probeRepoRoot(t), "docs", "LTAOO_REPLY_TWO_PAGE_PROBE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"prepare-ltaoo-probe.ps1", "probe-ltaoo-replies.ps1", "cleanup-ltaoo-probe.ps1",
		"INSTALL", "CurrentUser\\Root", "reply-probe-summary.json",
		"verified_reply_two_pages", "inconclusive_no_eligible_root",
		"inconclusive_no_second_reply_page", "reply_relation_mismatch",
		"comment_request_count", "reply_request_count", "cleanup_success",
		"Clash", "system: false", "tun: false", "status_schema", "readiness_proof",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("reply runbook missing %q", required)
		}
	}
}

func TestLtaooProbeScriptsParseInWindowsPowerShell(t *testing.T) {
	for _, name := range []string{"prepare-ltaoo-probe.ps1", "probe-ltaoo-comments.ps1", "probe-ltaoo-replies.ps1", "cleanup-ltaoo-probe.ps1"} {
		path := filepath.Join(probeRepoRoot(t), "scripts", name)
		quotedPath := strings.ReplaceAll(path, "'", "''")
		command := fmt.Sprintf(`$tokens=$null; $errors=$null; [void][System.Management.Automation.Language.Parser]::ParseFile('%s',[ref]$tokens,[ref]$errors); if($errors.Count){ $errors | ForEach-Object { Write-Error $_.Message }; exit 1 }`, quotedPath)
		output, err := exec.Command(probePowerShell(t), "-NoProfile", "-Command", command).CombinedOutput()
		if err != nil {
			t.Errorf("%s does not parse: %v\n%s", name, err, output)
		}
	}
}
