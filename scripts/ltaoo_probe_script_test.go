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
	if len(summary.Pages) != 2 || summary.Pages[0].CommentCount != 1 || summary.Pages[1].CommentCount != 1 {
		t.Fatalf("unexpected pages: %+v", summary.Pages)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("request count = %d, want status + profile + two comment pages", len(requests))
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
