package poc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadBatchRequestAcceptsStrictConfiguredLinksAndLimits(t *testing.T) {
	runRoot := t.TempDir()
	requestPath := filepath.Join(runRoot, "request.json")
	writeBatchRequestFixture(t, requestPath, runRoot, nil)
	request, err := LoadBatchRequest(requestPath, runRoot)
	if err != nil {
		t.Fatal(err)
	}
	if request.SchemaVersion != 1 || request.RunID != "fixture-run-1" || len(request.ContentURLs) != 1 ||
		request.Limits.RepliesPerComment != 2 || request.Limits.RepliesPerWork != 3 {
		t.Fatalf("request=%+v", request)
	}
}

func TestLoadBatchRequestRejectsUnknownFieldsUnsafeOutputAndInvalidLimits(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any, string)
	}{
		{"unknown", func(value map[string]any, _ string) { value["unexpected"] = true }},
		{"wrong schema", func(value map[string]any, _ string) { value["schema_version"] = 2 }},
		{"unsafe run id", func(value map[string]any, _ string) { value["run_id"] = "../escape" }},
		{"outside output", func(value map[string]any, root string) {
			value["output_root"] = filepath.Join(filepath.Dir(root), "outside")
		}},
		{"wrong output name", func(value map[string]any, root string) { value["output_root"] = filepath.Join(root, "other") }},
		{"bad link", func(value map[string]any, _ string) { value["content_urls"] = []string{"https://example.com/sph/no"} }},
		{"duplicate link", func(value map[string]any, _ string) {
			value["content_urls"] = []string{"https://weixin.qq.com/sph/a", "https://weixin.qq.com/sph/a/"}
		}},
		{"too many works", func(value map[string]any, _ string) { value["limits"].(map[string]any)["works"] = 11 }},
		{"mismatched reply limits", func(value map[string]any, _ string) { value["limits"].(map[string]any)["replies_per_comment"] = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runRoot := t.TempDir()
			requestPath := filepath.Join(runRoot, "request.json")
			writeBatchRequestFixture(t, requestPath, runRoot, test.mutate)
			if _, err := LoadBatchRequest(requestPath, runRoot); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}

func TestRunAndFinalizeLtaooBatchPublishesOnlyClosedVerifiedFiles(t *testing.T) {
	const forbidden = "NEVER_PERSIST_COOKIE_HEADER_OR_CURSOR"
	server := newBatchFixtureServer(t, forbidden)
	defer server.Close()
	runRoot := t.TempDir()
	requestPath := filepath.Join(runRoot, "request.json")
	writeBatchRequestFixture(t, requestPath, runRoot, nil)
	request, err := LoadBatchRequest(requestPath, runRoot)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := RunLtaooBatch(context.Background(), request, client, runRoot)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Status != BatchSucceeded || draft.Counts.Works != 1 || draft.Counts.TopLevelComments != 1 || draft.Counts.Replies != 2 {
		t.Fatalf("draft=%+v", draft)
	}
	if _, err := os.Stat(filepath.Join(runRoot, "batch")); !os.IsNotExist(err) {
		t.Fatal("batch published before cleanup receipt")
	}
	receiptPath := filepath.Join(runRoot, "cleanup-receipt.input.json")
	writeCleanupReceiptFixture(t, receiptPath, request.RunID, true)
	manifest, err := FinalizeLtaooBatch(request, runRoot, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != BatchSucceeded || !manifest.Cleanup.Safe {
		t.Fatalf("manifest=%+v", manifest)
	}
	batchRoot := filepath.Join(runRoot, "batch")
	entries, err := os.ReadDir(batchRoot)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"cleanup-receipt.json", "comments.jsonl", "contents.jsonl", "issues.jsonl", "manifest.json"}
	if len(entries) != len(wantNames) {
		t.Fatalf("entries=%v", entries)
	}
	for index, entry := range entries {
		if entry.Name() != wantNames[index] || entry.IsDir() {
			t.Fatalf("entry[%d]=%s", index, entry.Name())
		}
	}
	for name, record := range manifest.Files {
		raw, err := os.ReadFile(filepath.Join(batchRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != record.SHA256 || int64(len(raw)) != record.Bytes {
			t.Fatalf("file record mismatch for %s", name)
		}
	}
	var all strings.Builder
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(batchRoot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		all.Write(raw)
	}
	for _, secret := range []string{forbidden, runRoot, "BEGIN PRIVATE KEY", "Cookie:", "Authorization:"} {
		if strings.Contains(all.String(), secret) {
			t.Fatalf("forbidden batch content: %q", secret)
		}
	}
	if _, err := FinalizeLtaooBatch(request, runRoot, receiptPath); err == nil {
		t.Fatal("existing final batch was overwritten")
	}
}

func TestFinalizeLtaooBatchPublishesValidDataWhenCleanupNeedsVerification(t *testing.T) {
	server := newBatchFixtureServer(t, "fixture-redacted-cursor")
	defer server.Close()
	runRoot := t.TempDir()
	requestPath := filepath.Join(runRoot, "request.json")
	writeBatchRequestFixture(t, requestPath, runRoot, nil)
	request, err := LoadBatchRequest(requestPath, runRoot)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunLtaooBatch(context.Background(), request, client, runRoot); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(runRoot, "cleanup-receipt.input.json")
	writeCleanupReceiptFixture(t, receiptPath, request.RunID, false)
	manifest, err := FinalizeLtaooBatch(request, runRoot, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != BatchNeedsVerification || manifest.Cleanup.Safe || manifest.Counts.TopLevelComments != 1 {
		t.Fatalf("manifest=%+v", manifest)
	}
	if _, err := os.Stat(filepath.Join(runRoot, "batch", "comments.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestRunLtaooBatchRetainsFirstPageWhenLaterPageIsInvalid(t *testing.T) {
	commentCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"partial-work","objectNonceId":"partial-nonce"}}}}`)
		case "/api/channels/feed/comment/list":
			commentCalls++
			if commentCalls == 1 {
				_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"partial-root","content":"retained public comment"}],"lastBuffer":"next-page"}}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"unexpected":true}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	runRoot := t.TempDir()
	requestPath := filepath.Join(runRoot, "request.json")
	writeBatchRequestFixture(t, requestPath, runRoot, nil)
	request, err := LoadBatchRequest(requestPath, runRoot)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewLtaooClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunLtaooBatch(context.Background(), request, client, runRoot)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != BatchPartial || result.Counts.TopLevelComments != 1 || commentCalls != 2 {
		t.Fatalf("result=%+v calls=%d", result, commentCalls)
	}
	receiptPath := filepath.Join(runRoot, "cleanup-receipt.input.json")
	writeCleanupReceiptFixture(t, receiptPath, request.RunID, true)
	manifest, err := FinalizeLtaooBatch(request, runRoot, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != BatchPartial || manifest.Counts.TopLevelComments != 1 {
		t.Fatalf("manifest=%+v", manifest)
	}
	raw, err := os.ReadFile(filepath.Join(runRoot, "batch", "comments.jsonl"))
	if err != nil || !strings.Contains(string(raw), "retained public comment") {
		t.Fatalf("comments=%s err=%v", raw, err)
	}
}

func writeBatchRequestFixture(t *testing.T, path, runRoot string, mutate func(map[string]any, string)) {
	t.Helper()
	value := map[string]any{
		"schema_version": 1,
		"run_id":         "fixture-run-1",
		"keyword":        "fixture topic",
		"content_urls":   []string{"https://weixin.qq.com/sph/fixture-one"},
		"limits": map[string]any{
			"works": 1, "top_level_comments_per_work": 3, "replies_per_comment": 2, "replies_per_work": 3,
		},
		"output_root": filepath.Join(runRoot, "batch"),
	}
	if mutate != nil {
		mutate(value, runRoot)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeCleanupReceiptFixture(t *testing.T, path, runID string, safe bool) {
	t.Helper()
	value := BatchCleanupReceipt{
		SchemaVersion: 1, RunID: runID, Safe: safe, CAAbsent: safe, ClashRestored: safe,
		ProcessStopped: safe, PortsReleased: safe, SecretsDeleted: safe, CompletedAt: time.Now().UTC(),
	}
	if !safe {
		value.ReasonCodes = []string{"cleanup_attention_required"}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newBatchFixtureServer(t *testing.T, forbidden string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/status":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"api":{"listening":true},"proxy":{"listening":true}}}`)
		case "/api/channels/feed/profile":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":"batch-work-1","objectNonceId":"batch-nonce-1","objectDesc":{"description":"batch title","mediaType":2}}}}}`)
		case "/api/channels/feed/comment/list":
			if r.URL.Query().Get("comment_id") == "" {
				_, _ = fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"batch-root-1","content":"public root","expandCommentCount":2,"privateHeader":%q}],"lastBuffer":""}}}`, forbidden)
				return
			}
			_, _ = fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"commentInfo":[{"commentId":"batch-reply-1","replyCommentId":"batch-root-1","rootCommentId":"batch-root-1","content":"public reply one"},{"commentId":"batch-reply-2","replyCommentId":"batch-reply-1","rootCommentId":"batch-root-1","content":"public reply two"}],"debugCursor":%q,"lastBuffer":""}}}`, forbidden)
		default:
			http.NotFound(w, r)
		}
	}))
}
