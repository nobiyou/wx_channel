# ltaoo Readiness Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the two-page ltaoo probe accept both the modern listener status schema and the verified legacy status schema without weakening request limits or redaction.

**Architecture:** Add one in-script normalizer that converts either `/api/status` shape into two fixed audit enums plus a `profile_allowed` decision. Keep shared-profile resolution as the mandatory page-WebSocket proof, then reuse the existing exact two-page flow unchanged.

**Tech Stack:** Windows PowerShell 5.1, Go `testing`, `httptest.Server`, JSON fixtures.

---

## File map

- Modify `scripts/ltaoo_probe_script_test.go`: add modern/legacy readiness regression cases, request-count assertions, and secret-leak assertions.
- Modify `scripts/probe-ltaoo-comments.ps1`: normalize status evidence, gate profile attempts, and persist only fixed evidence enums.
- Modify `docs/LTAOO_TWO_PAGE_PROBE.md`: document the legacy status defect and the profile-based readiness proof.

No preparation, cleanup, certificate, Clash, proxy, Go production service, or collector file changes are permitted.

### Task 1: Define status compatibility with failing tests

**Files:**

- Modify: `scripts/ltaoo_probe_script_test.go:95-227`
- Test: `scripts/ltaoo_probe_script_test.go`

- [ ] **Step 1: Extend the existing modern two-page assertion**

Add the two audit fields to the summary struct in `TestProbeFetchesExactlyTwoPagesAndRedacts`:

```go
StatusSchema   string `json:"status_schema"`
ReadinessProof string `json:"readiness_proof"`
```

Then add:

```go
if summary.StatusSchema != "modern" || summary.ReadinessProof != "listeners_and_profile" {
	t.Fatalf("unexpected readiness evidence: %+v", summary)
}
```

- [ ] **Step 2: Add the successful legacy two-page regression**

Insert after the modern two-page test:

```go
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
			fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":%q,"objectNonceId":%q}}}}`, oid, nid)
		case "/api/channels/feed/comment/list":
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
```

- [ ] **Step 3: Add early-rejection tests**

Insert:

```go
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
```

- [ ] **Step 4: Add the legacy profile-failure boundary**

Insert:

```go
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
```

- [ ] **Step 5: Run focused tests and confirm the compatibility failure**

```powershell
go test ./scripts -run 'TestProbe(FetchesExactlyTwoPagesAndRedacts|AcceptsLegacyStatusAndUsesProfileAsReadinessProof|RejectsUnusableStatusBeforeProfile|LegacyProfileFailureDoesNotRequestComments)$' -v -count=1
```

Expected: FAIL. Modern audit enums are empty, and legacy fixtures fail before profile because the current script assumes `data.api` and `data.proxy`.

### Task 2: Implement the in-script readiness normalizer

**Files:**

- Modify: `scripts/probe-ltaoo-comments.ps1:100-270`
- Test: `scripts/ltaoo_probe_script_test.go`

- [ ] **Step 1: Add safe property inspection and normalization**

Insert after `Assert-BusinessSuccess`:

```powershell
function Test-ObjectProperty {
    param([object]$Object, [string]$Name)
    return $null -ne $Object -and $Object.PSObject.Properties.Name -contains $Name
}

function Get-LtaooStatusEvidence {
    param([object]$Response)
    try {
        if (-not (Test-ObjectProperty $Response "body")) { throw "status_schema_error" }
        $body = $Response.body
        if (-not (Test-ObjectProperty $body "code")) { throw "status_schema_error" }
        if ([int]$body.code -ne 0) { throw "ltaoo_not_ready" }
        if (-not (Test-ObjectProperty $body "data") -or $null -eq $body.data) { throw "status_schema_error" }
        $data = $body.data

        $hasApi = Test-ObjectProperty $data "api"
        $hasProxy = Test-ObjectProperty $data "proxy"
        if ($hasApi -or $hasProxy) {
            if (-not $hasApi -or -not $hasProxy -or $null -eq $data.api -or $null -eq $data.proxy) { throw "status_schema_error" }
            if (-not (Test-ObjectProperty $data.api "listening") -or -not (Test-ObjectProperty $data.proxy "listening")) { throw "status_schema_error" }
            if ($data.api.listening -isnot [bool] -or $data.proxy.listening -isnot [bool]) { throw "status_schema_error" }
            return [pscustomobject]@{
                status_schema = "modern"
                readiness_proof = "listeners_and_profile"
                profile_allowed = [bool]($data.api.listening -and $data.proxy.listening)
            }
        }

        $hasChannels = Test-ObjectProperty $data "channels"
        $hasVersion = Test-ObjectProperty $data "version"
        if ($hasChannels -or $hasVersion) {
            if (-not $hasChannels -or -not $hasVersion -or $null -eq $data.channels) { throw "status_schema_error" }
            if (-not (Test-ObjectProperty $data.channels "available")) { throw "status_schema_error" }
            if ($data.version -isnot [string] -or [string]::IsNullOrWhiteSpace([string]$data.version)) { throw "status_schema_error" }
            return [pscustomobject]@{
                status_schema = "legacy"
                readiness_proof = "profile"
                profile_allowed = $true
            }
        }

        throw "status_schema_error"
    } catch {
        if ($_.Exception.Message -in @("ltaoo_not_ready", "status_schema_error")) { throw }
        throw "status_schema_error"
    }
}
```

- [ ] **Step 2: Initialize evidence before the main try block**

After `$commentRequestCount = 0`, add:

```powershell
$statusSchema = ""
$readinessProof = ""
```

- [ ] **Step 3: Replace the modern-only status gate**

Replace the current conditional after the status request with:

```powershell
$statusEvidence = Get-LtaooStatusEvidence $status
$statusSchema = [string]$statusEvidence.status_schema
$readinessProof = [string]$statusEvidence.readiness_proof
if (-not [bool]$statusEvidence.profile_allowed) { throw "ltaoo_not_ready" }
```

Do not change the profile or comment request sequence.

- [ ] **Step 4: Persist only fixed evidence enums**

Add after `status_http` in the successful summary:

```powershell
status_schema = $statusSchema
readiness_proof = $readinessProof
```

Immediately before writing the failure summary, add:

```powershell
if (-not [string]::IsNullOrEmpty($statusSchema)) { $failure["status_schema"] = $statusSchema }
if (-not [string]::IsNullOrEmpty($readinessProof)) { $failure["readiness_proof"] = $readinessProof }
```

Do not persist `$status.body`, `$profile.body`, the share URL, `oid/nid`, or a raw cursor.

- [ ] **Step 5: Run focused and full script tests**

```powershell
go test ./scripts -run 'TestProbe(FetchesExactlyTwoPagesAndRedacts|AcceptsLegacyStatusAndUsesProfileAsReadinessProof|RejectsUnusableStatusBeforeProfile|LegacyProfileFailureDoesNotRequestComments)$' -v -count=1
go test ./scripts -v -count=1
```

Expected: both commands PASS. Modern reports `modern/listeners_and_profile`; legacy reports `legacy/profile`; unusable status fixtures send one request; profile failure sends exactly two non-comment requests.

- [ ] **Step 6: Commit implementation and regression tests**

```powershell
git add scripts/probe-ltaoo-comments.ps1 scripts/ltaoo_probe_script_test.go
git diff --cached --check
git commit -m "fix: support legacy ltaoo readiness"
```

Expected: one commit containing only the probe script and its Go tests.

### Task 3: Document status evidence and complete verification

**Files:**

- Modify: `scripts/ltaoo_probe_script_test.go:366-380`
- Modify: `docs/LTAOO_TWO_PAGE_PROBE.md:41-48`
- Test: `scripts/ltaoo_probe_script_test.go`

- [ ] **Step 1: Strengthen the runbook static assertion**

In `TestLtaooProbeRunbookHasSafetySequence`, add:

```go
"status_schema", "readiness_proof", "channels.available",
```

- [ ] **Step 2: Run the runbook test and verify it fails**

```powershell
go test ./scripts -run TestLtaooProbeRunbookHasSafetySequence -v -count=1
```

Expected: FAIL because the runbook lacks the two schemas and evidence fields.

- [ ] **Step 3: Add the compatibility section to the runbook**

After the paragraph introducing `probe-summary.json`, add:

```markdown
### 状态协议兼容

探针识别两种 `/api/status` 结构：

- 新版必须同时报告 API 和代理监听成功，摘要记录 `status_schema=modern`、`readiness_proof=listeners_and_profile`。
- 已验证旧版的 `channels.available` 存在恒为 `false` 的实现缺陷。探针只用非空版本和该字段的存在确认旧版结构，再以共享页面 profile 成功和有效 `oid/nid` 证明页面 WebSocket 可用；摘要记录 `status_schema=legacy`、`readiness_proof=profile`。

未知或残缺状态结构立即以 `status_schema_error` 停止。新版监听失败以 `ltaoo_not_ready` 停止。两种模式都不会因为状态兼容而增加重试、回复请求或第三页请求。
```

- [ ] **Step 4: Run documentation and full script tests**

```powershell
go test ./scripts -run TestLtaooProbeRunbookHasSafetySequence -v -count=1
go test ./scripts -v -count=1
```

Expected: both commands PASS.

- [ ] **Step 5: Verify parser, diff hygiene, and scope**

```powershell
$tokens = $null
$errors = $null
[void][System.Management.Automation.Language.Parser]::ParseFile(
  (Resolve-Path scripts/probe-ltaoo-comments.ps1),
  [ref]$tokens,
  [ref]$errors
)
if ($errors.Count) { $errors | ForEach-Object { Write-Error $_.Message }; exit 1 }

git diff --check
git status --short
git diff --stat HEAD~1
```

Expected: no parser or whitespace errors; status lists only the runbook and its static test; the implementation commit contains only the probe script and its tests.

- [ ] **Step 6: Commit the runbook update**

```powershell
git add docs/LTAOO_TWO_PAGE_PROBE.md scripts/ltaoo_probe_script_test.go
git diff --cached --check
git commit -m "docs: explain ltaoo readiness evidence"
```

- [ ] **Step 7: Perform final clean-worktree verification**

```powershell
go test ./scripts -v -count=1
git status --short --branch
git log -3 --oneline
```

Expected: all script tests PASS and the worktree is clean. No ltaoo process is started, no CA is installed, and no Clash or proxy setting is changed by this plan.
