# ltaoo Profile Readiness and Safe Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a batch-wide 30-second profile readiness window, preserve closed profile error categories through TrendRadar, and compare the current and historically successful ltaoo binaries in the same live environment.

**Architecture:** Keep ltaoo HTTP and business-envelope parsing in `internal/poc/ltaoo_client.go`. Add an internal collection path with an injected clock and readiness policy so deterministic tests can prove the shared deadline; the public batch path uses the fixed production policy. TrendRadar only expands its closed batch issue allowlist and maps the new codes to existing presentation identities.

**Tech Stack:** Go 1.24.3, Go `httptest`, Windows/TDM-GCC 10.3.0, Python 3.11+, pytest, Ruff, PowerShell 5.1, Mihomo, ltaoo.

---

## File Map

- Modify `internal/poc/ltaoo_client.go`: profile error-to-issue mapping and shared readiness collection loop.
- Modify `internal/poc/ltaoo_client_test.go`: deterministic readiness, deadline, cancellation, and category tests.
- Modify `internal/poc/ltaoo_batch_test.go`: assert failed batch reason codes and empty target serialization for the new categories.
- Modify `D:/Agent/services/trendradar-monitor/trendradar/social/wechat_channels_batch.py`: closed issue-code mappings.
- Modify `D:/Agent/services/trendradar-monitor/tests/test_social_wechat_channels_batch.py`: end-to-end verifier mappings and unknown-code rejection.
- Build `cmd/wx_channel_ltaoo_batch`: produce the reviewed delivery executable.
- Update ignored delivery file `D:/Agent/services/trendradar-monitor/tools/wechat-channels/wx_channel_ltaoo_batch.exe`: local A/B runtime only, never commit the binary.

### Task 1: Lock profile classifications with failing Go tests

**Files:**
- Modify: `internal/poc/ltaoo_client_test.go`

- [ ] **Step 1: Add a deterministic fake clock**

Add a test clock whose `Now` returns a controlled value and whose `Sleep` records durations, advances time, and respects cancelled contexts:

```go
type profileTestClock struct {
    now    time.Time
    sleeps []time.Duration
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
```

- [ ] **Step 2: Add the retry-then-success test**

Drive loopback servers that return HTTP 503 twice, HTTP 200 with outer `code=400` and no `data` twice, or HTTP 200 with outer `code=0` and inner `errCode=1011` without business `data` twice, then a valid profile on the third request. Call the internal readiness-aware collector with a 30-second timeout and assert one Work, no issues, exactly three requests, and no duplicate success request.

- [ ] **Step 3: Add the shared-deadline exhaustion test**

Use three unique URLs and a server that always returns HTTP 503. Assert the fake clock advances by no more than 30 seconds, only the active readiness URL is requested, and all three inputs receive `profile_not_ready` without three independent wait windows.

- [ ] **Step 4: Add category table tests**

Cover HTTP 401, 403, 429, malformed HTTP-200 JSON, invalid business envelope, startup `code=400/no data`, startup `errCode=1011/no data`, and HTTP 418. Expected codes are respectively `profile_access_denied`, `profile_access_denied`, `profile_rate_limited`, `profile_schema_mismatch`, `profile_schema_mismatch`, `profile_not_ready` after deadline, `profile_not_ready` after deadline, and `profile_unavailable`.

- [ ] **Step 5: Add non-retry-first-link and cancellation tests**

Assert a schema error on input 1 does not block a successful input 2 from establishing readiness. Assert cancellation during retry emits `collection_cancelled` and sends no later request.

- [ ] **Step 6: Run the new tests and verify RED**

Run with the approved toolchain when available:

```powershell
$go = '.\.poc-tools\go1.24.3\go\bin\go.exe'
& $go test ./internal/poc -run 'TestCollectWorksFromURLs(Profile|Shared|Cancel)' -count=1 -v
```

Expected: FAIL because the readiness-aware helper and new issue codes do not exist.

### Task 2: Implement the shared readiness policy

**Files:**
- Modify: `internal/poc/ltaoo_client.go`
- Test: `internal/poc/ltaoo_client_test.go`
- Test: `internal/poc/ltaoo_batch_test.go`

- [ ] **Step 1: Add the fixed policy and internal options**

Add unexported production constants and an options type:

```go
const (
    profileReadinessTimeout = 30 * time.Second
    profileRetryInterval    = 500 * time.Millisecond
)

type profileReadinessOptions struct {
    Clock         Clock
    Timeout       time.Duration
    RetryInterval time.Duration
}
```

- [ ] **Step 2: Add the closed error mapper**

```go
func profileIssueCode(err error) string {
    switch ClassifyError(err) {
    case ErrorAccessDenied:
        return "profile_access_denied"
    case ErrorRateLimited:
        return "profile_rate_limited"
    case ErrorStructure:
        return "profile_schema_mismatch"
    default:
        return "profile_unavailable"
    }
}
```

Do not inspect or persist response bodies to refine the category.

- [ ] **Step 3: Implement the readiness-aware collector**

Create an unexported `collectWorksFromURLs` that receives `profileReadinessOptions`. It must:

```go
deadline := options.Clock.Now().Add(options.Timeout)
readinessEstablished := false
readinessTimedOut := false
```

For each validated, unique URL, retry only `ErrorTransient` while readiness is not established. The profile decoder maps only a validated HTTP-200 outer `code=400` with absent/null `data`, or outer `code=0` with inner `errCode=1011` and absent/null business `data`, to `ErrorTransient`; other malformed or conflicting envelopes remain `ErrorStructure`. Clamp every sleep to `deadline.Sub(options.Clock.Now())`. On exhaustion, mark the current and remaining valid unique inputs `profile_not_ready` without additional HTTP calls. Reuse the first successful Work and process later URLs once each. Preserve URL deduplication, Work deduplication, input indexes, limits, and cancellation.

- [ ] **Step 4: Keep the public wrapper compatible**

Keep `CollectWorksFromURLs(ctx, client, urls, limit)` for existing callers. It delegates to the internal helper using `batchClock{}`, `profileReadinessTimeout`, and `profileRetryInterval`.

- [ ] **Step 5: Update existing expectations**

Change the existing broken-envelope assertion from `profile_unavailable` to `profile_schema_mismatch`. Add a failed-batch test proving `targets` remains `[]` and `reason_codes` contains the actual new profile code.

- [ ] **Step 6: Run focused tests and verify GREEN**

```powershell
& $go test ./internal/poc ./cmd/wx_channel_ltaoo_batch -count=1 -v
```

Expected: PASS.

- [ ] **Step 7: Commit the wx_channel implementation**

```powershell
git add internal/poc/ltaoo_client.go internal/poc/ltaoo_client_test.go internal/poc/ltaoo_batch.go internal/poc/ltaoo_batch_test.go
git commit -m "fix: wait for ltaoo profile readiness"
```

### Task 3: Lock TrendRadar safe mappings with failing tests

**Files:**
- Modify: `D:/Agent/services/trendradar-monitor/tests/test_social_wechat_channels_batch.py`

- [ ] **Step 1: Parameterize the five profile reason codes**

Rewrite a copied failed fixture for each source code and assert:

```python
(
    ("profile_not_ready", "wechat_page_unavailable"),
    ("profile_access_denied", "login_required"),
    ("profile_rate_limited", "rate_limited"),
    ("profile_schema_mismatch", "page_structure_changed"),
    ("profile_unavailable", "content_unavailable"),
)
```

For each case, update both `issues.jsonl` and `manifest.reason_codes`, refresh file hashes, and verify `targets == []` remains accepted.

- [ ] **Step 2: Add unknown-code rejection**

Use `profile_private_detail` as an unknown issue/reason code and assert `verify_wechat_channels_batch` raises the existing closed-schema `ValueError`.

- [ ] **Step 3: Run tests and verify RED**

```powershell
Set-Location 'D:\Agent\services\trendradar-monitor'
.\.venv\Scripts\python.exe -m pytest tests/test_social_wechat_channels_batch.py -q
```

Expected: FAIL for the four new source codes because `_ISSUE_MAP` does not allow them.

### Task 4: Implement TrendRadar mappings

**Files:**
- Modify: `D:/Agent/services/trendradar-monitor/trendradar/social/wechat_channels_batch.py`
- Test: `D:/Agent/services/trendradar-monitor/tests/test_social_wechat_channels_batch.py`

- [ ] **Step 1: Extend only the closed issue map**

Add:

```python
("content", "profile_not_ready"): ("detail", "wechat_page_unavailable"),
("content", "profile_access_denied"): ("detail", "login_required"),
("content", "profile_rate_limited"): ("search", "rate_limited"),
("content", "profile_schema_mismatch"): ("detail", "page_structure_changed"),
```

Keep the existing `profile_unavailable` mapping unchanged. Do not add wildcard acceptance.

- [ ] **Step 2: Run focused verifier tests and verify GREEN**

```powershell
.\.venv\Scripts\python.exe -m pytest tests/test_social_wechat_channels_batch.py tests/test_social_wechat_channels_collector.py -q
```

Expected: PASS.

- [ ] **Step 3: Run Ruff on touched Python files**

```powershell
.\.venv\Scripts\python.exe -m ruff check trendradar/social/wechat_channels_batch.py tests/test_social_wechat_channels_batch.py
```

Expected: PASS with no diagnostics.

- [ ] **Step 4: Do not commit unrelated TrendRadar changes**

Inspect `git diff -- trendradar/social/wechat_channels_batch.py tests/test_social_wechat_channels_batch.py`. If a commit is requested later, stage only those two paths.

### Task 5: Restore and verify the approved build toolchain

**Files:**
- Ignored directory: `.poc-tools/go1.24.3`
- Ignored directory: `.poc-tools/tdm-gcc-10.3.0-2`

- [ ] **Step 1: Check for an existing approved artifact before downloading**

Search only known local tool roots. Accept Go only when `go version` is exactly `go1.24.3 windows/amd64`; accept GCC only when `gcc -dumpfullversion` is exactly `10.3.0`.

- [ ] **Step 2: Restore missing tools from reviewed sources**

Use the official Go 1.24.3 Windows amd64 archive and the project runbook's TDM-GCC 10.3.0-2 artifact. Verify the TDM installer/archive SHA-256 against `819C7A1F74D45AD04E10662E1A2C3124D13D9A2BCA508847692251242CD455C3` before extraction. Do not modify the system PATH; place tools only under ignored `.poc-tools`.

- [ ] **Step 3: Verify exact versions**

```powershell
& '.\.poc-tools\go1.24.3\go\bin\go.exe' version
& '.\.poc-tools\tdm-gcc-10.3.0-2\bin\gcc.exe' -dumpfullversion
```

Expected: `go version go1.24.3 windows/amd64` and `10.3.0`.

### Task 6: Build and install the reviewed batch executable

**Files:**
- Build output: `.poc-build/wx_channel_ltaoo_batch.exe`
- Local delivery: `D:/Agent/services/trendradar-monitor/tools/wechat-channels/wx_channel_ltaoo_batch.exe`

- [ ] **Step 1: Run the full Go verification with approved environment variables**

Set `GOROOT`, `GOTOOLCHAIN=local`, `CGO_ENABLED=1`, `CC`, `CXX`, and a process-local PATH exactly as `scripts/build-poc.ps1` does. Run:

```powershell
& $go mod verify
& $go test ./internal/poc ./cmd/wx_channel_ltaoo_batch ./scripts -count=1
```

Expected: PASS.

- [ ] **Step 2: Build the batch command**

```powershell
New-Item -ItemType Directory -Force .poc-build | Out-Null
& $go build -trimpath -o .poc-build\wx_channel_ltaoo_batch.exe ./cmd/wx_channel_ltaoo_batch
```

Expected: exit 0.

- [ ] **Step 3: Probe capabilities and build metadata**

```powershell
.\.poc-build\wx_channel_ltaoo_batch.exe capabilities
& $go version -m .\.poc-build\wx_channel_ltaoo_batch.exe
Get-FileHash -Algorithm SHA256 .\.poc-build\wx_channel_ltaoo_batch.exe
```

Expected capabilities include `wechat-channels-local-runtime-v2` and `mihomo`; metadata reports Go 1.24.3.

- [ ] **Step 4: Back up and replace only the ignored delivery binary**

Record the existing SHA-256, copy it to a run-specific ignored backup, then copy the reviewed build to `D:/Agent/services/trendradar-monitor/tools/wechat-channels/wx_channel_ltaoo_batch.exe`. Recheck the destination hash. Do not touch the two PowerShell modules or `wx_video_download.exe`.

### Task 7: Execute live A/B with cleanup gates

**Files:**
- Current ltaoo: `D:/Agent/services/trendradar-monitor/tools/wechat-channels/wx_video_download.exe`
- Historical ltaoo: `D:/Agent/projects/wechat-channel-comment-poc/.worktrees/ltaoo-reply-two-page-probe/.tmp_runtime/ltaoo-build-c0c2b8cc36af52ab2c3eb50cb7dc08b7d963efb0/wx_video_download.exe`
- Runtime output: `D:/Agent/services/trendradar-monitor/output/admin/wechat-channels-runtime/<run-id>`

- [ ] **Step 1: Verify A/B candidates before mutation**

Confirm hashes are exactly `18F0E4...DAD9F40` and `F5D411...FCD898D`. Confirm PC 微信 has the first target video open and the visible window belongs to `WeChatAppEx.exe`.

- [ ] **Step 2: Run candidate A**

Keep the currently authorized ltaoo in place. Start the TrendRadar manual collection. When port 2022 is listening, refresh the visible PC 微信 video page once. Record only run ID, candidate hash, status, stable profile codes, counts, elapsed time, and cleanup receipt.

- [ ] **Step 3: Enforce the A cleanup gate**

Require `cleanup.safe=true`, no `TrendRadarWX-*` CA, no ltaoo process, no 2022/2023 listeners, no Mihomo runtime marker, and restored router bytes/receipt. Stop if any check fails.

- [ ] **Step 4: Switch to candidate B under explicit authorization**

Back up candidate A, place candidate B at the reviewed ltaoo delivery path, verify the exact hash, and renew the machine-local runtime authorization because it is hash-bound. Do not bypass the authorization service or edit its SQLite record directly.

- [ ] **Step 5: Run candidate B with identical inputs**

Repeat the same command and one page refresh. Record the same closed result fields.

- [ ] **Step 6: Enforce the B cleanup gate**

Run the same CA/router/process/port/secrets checks before any restoration.

- [ ] **Step 7: Restore candidate A and authorization state**

Restore the original current ltaoo bytes, verify hash `18F0E4...DAD9F40`, and renew authorization for the restored hash. Recheck no runtime residue remains.

### Task 8: Final regression verification

**Files:**
- All touched source and test files

- [ ] **Step 1: Run wx_channel focused and script suites**

```powershell
& $go test ./internal/poc ./cmd/wx_channel_ltaoo_batch ./scripts -count=1
```

- [ ] **Step 2: Run TrendRadar focused suite**

```powershell
Set-Location 'D:\Agent\services\trendradar-monitor'
.\.venv\Scripts\python.exe -m pytest tests/test_social_wechat_channels_batch.py tests/test_social_wechat_channels_collector.py tests/test_social_comment_config.py -q
.\.venv\Scripts\python.exe -m ruff check trendradar/social/wechat_channels_batch.py tests/test_social_wechat_channels_batch.py
```

- [ ] **Step 3: Inspect repository boundaries**

Confirm wx_channel contains only the planned source/test/docs changes. In TrendRadar, confirm existing user changes remain untouched and only the two planned files differ because of this task.

- [ ] **Step 4: Report evidence**

Report test counts, new batch SHA-256, candidate hashes, A/B run IDs and closed results, plus cleanup status. If the B authorization confirmation is not completed, report A/B as blocked rather than complete.
