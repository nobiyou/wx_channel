# ltaoo Two-Page Comment Probe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build three auditable Windows PowerShell scripts that record a network/certificate baseline, run exactly two ltaoo comment-list requests from one share URL, emit only redacted evidence, and precisely clean up this run's CA, secrets, configuration, and ltaoo process.

**Architecture:** `prepare-ltaoo-probe.ps1` owns run creation, baseline capture, ephemeral CA creation, constrained ltaoo configuration, CurrentUser certificate installation, and ltaoo launch. `probe-ltaoo-comments.ps1` talks only to the loopback ltaoo HTTP API and never persists raw responses. `cleanup-ltaoo-probe.ps1` trusts only the run manifest, validates every target, removes exact run-owned resources, and reports but never repairs external drift.

**Tech Stack:** Windows PowerShell 5.1, .NET Framework cryptography and HTTP APIs, Windows certificate store and `certutil.exe`, Go 1.25 test harness with `httptest`.

---

## File map

- Modify: `.gitignore` — explicitly version the three probe scripts and their Go test while retaining `.tmp_runtime/` exclusion.
- Create: `scripts/prepare-ltaoo-probe.ps1` — baseline, CA, configuration, confirmation, certificate installation, process launch, rollback-on-error.
- Create: `scripts/probe-ltaoo-comments.ps1` — loopback API validation, profile request, exactly two comment requests, redacted summary.
- Create: `scripts/cleanup-ltaoo-probe.ps1` — run/path validation, process identity check, exact certificate removal, exact secret removal, drift receipt.
- Create: `scripts/ltaoo_probe_script_test.go` — safe Windows integration and static security tests.
- Create: `docs/LTAOO_TWO_PAGE_PROBE.md` — operator runbook and decision table.

The runtime tree is fixed at `.tmp_runtime/ltaoo-probe/<run_id>/`. No implementation task may add a database, Go production command, shared collection service, reply request, third comment request, Clash mutation, system-proxy mutation, TUN setup, or route mutation.

## Public script interfaces

The implementation must keep these parameter names stable because tests and the runbook call them directly.

```powershell
# prepare-ltaoo-probe.ps1
param(
    [Parameter(Mandatory = $true)][string]$LtaooExePath,
    [string]$RepoRoot = "",
    [int]$ApiPort = 2022,
    [int]$ProxyPort = 2023
)

# probe-ltaoo-comments.ps1
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9-]{1,80}$')][string]$RunId,
    [Parameter(Mandatory = $true)][string]$ShareUrl,
    [string]$RepoRoot = "",
    [string]$ApiBase = ""
)

# cleanup-ltaoo-probe.ps1
param(
    [Parameter(Mandatory = $true)][ValidatePattern('^[A-Za-z0-9-]{1,80}$')][string]$RunId,
    [string]$RepoRoot = ""
)
```

`ApiBase` exists only so the Go test can use a loopback `httptest` port. When omitted, the probe reads `ltaoo.api_base` from the manifest. It must still reject any host other than literal `127.0.0.1` and any scheme other than `http`.

All persisted JSON keys use lower snake case. The two manifest ownership objects are named `ca` and `ltaoo`; PowerShell may access them case-insensitively, but tests and serialized files use these exact lowercase names.

### Task 1: Version-control boundary and failing security tests

**Files:**
- Modify: `.gitignore`
- Create: `scripts/ltaoo_probe_script_test.go`

- [ ] **Step 1: Add exact `.gitignore` exceptions**

Append these lines immediately after the existing allowed `scripts/` entries:

```gitignore
!scripts/prepare-ltaoo-probe.ps1
!scripts/probe-ltaoo-comments.ps1
!scripts/cleanup-ltaoo-probe.ps1
!scripts/ltaoo_probe_script_test.go
```

Do not alter `.tmp_runtime/`; probe output must remain ignored.

- [ ] **Step 2: Add the Windows test harness and initial missing-file tests**

Create `scripts/ltaoo_probe_script_test.go` with the Windows build tag, package `main`, and these helpers:

```go
//go:build windows

package main

import (
    "crypto/sha256"
    "encoding/hex"
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
    if err != nil { t.Fatal(err) }
    return root
}

func probePowerShell(t *testing.T) string {
    t.Helper()
    for _, name := range []string{"powershell.exe", "powershell"} {
        if path, err := exec.LookPath(name); err == nil { return path }
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
    if err != nil { t.Fatal(err) }
    if err := json.Unmarshal(raw, out); err != nil { t.Fatalf("invalid JSON in %s: %v", path, err) }
}

func sha256Hex(value string) string {
    sum := sha256.Sum256([]byte(value))
    return hex.EncodeToString(sum[:])
}

func cleanupProbeTestRun(t *testing.T, runRoot string) {
    t.Helper()
    base := filepath.Clean(filepath.Join(probeRepoRoot(t), ".tmp_runtime", "ltaoo-probe"))
    target := filepath.Clean(runRoot)
    rel, err := filepath.Rel(base, target)
    if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
        t.Fatalf("refusing unsafe test cleanup target %q", target)
    }
    if err := os.RemoveAll(target); err != nil { t.Errorf("test cleanup failed: %v", err) }
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
```

Leave all listed imports in place; subsequent tasks add tests that use them.

- [ ] **Step 3: Run the focused test and confirm RED**

Run:

```powershell
go test ./scripts -run TestLtaooProbeScriptsExist -v
```

Expected: FAIL naming all three missing `.ps1` files.

- [ ] **Step 4: Create empty script files only to establish tracked paths**

Create each script with only the strict-mode header and its approved parameter block. Each file must end with an explicit failure so an empty scaffold cannot be mistaken for a usable probe:

```powershell
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
throw "ltaoo probe script is not implemented"
```

- [ ] **Step 5: Run the existence test and confirm GREEN**

Run:

```powershell
go test ./scripts -run TestLtaooProbeScriptsExist -v
```

Expected: PASS.

- [ ] **Step 6: Commit the boundary and scaffolds**

```powershell
git add .gitignore scripts/prepare-ltaoo-probe.ps1 scripts/probe-ltaoo-comments.ps1 scripts/cleanup-ltaoo-probe.ps1 scripts/ltaoo_probe_script_test.go
git commit -m "test: define ltaoo probe safety boundary"
```

### Task 2: Two-page redacted comment probe

**Files:**
- Modify: `scripts/ltaoo_probe_script_test.go`
- Modify: `scripts/probe-ltaoo-comments.ps1`

- [ ] **Step 1: Add an end-to-end two-page test**

Add a test server that records every request. Its concrete responses are:

```go
const shareURL = "https://weixin.qq.com/sph/FixtureShareSecret"
const oid = "fixture-oid-secret"
const nid = "fixture-nid-secret"
const marker = "cursor+/=fixture-secret"
const bait = "NEVER_PERSIST_COMMENT_BODY_OR_NICKNAME"

func TestProbeFetchesExactlyTwoPagesAndRedacts(t *testing.T) {
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
            if got := r.URL.Query().Get("url"); got != shareURL { t.Errorf("share URL = %q", got) }
            fmt.Fprintf(w, `{"code":0,"data":{"errCode":0,"data":{"object":{"id":%q,"objectNonceId":%q}}}}`, oid, nid)
        case "/api/channels/feed/comment/list":
            if r.URL.Query().Get("oid") != oid || r.URL.Query().Get("nid") != nid { t.Errorf("missing oid/nid") }
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
    runRoot := filepath.Join(probeRepoRoot(t), ".tmp_runtime", "ltaoo-probe", runID)
    t.Cleanup(func() { cleanupProbeTestRun(t, runRoot) })
    if err := os.MkdirAll(runRoot, 0o700); err != nil { t.Fatal(err) }
    manifest := fmt.Sprintf(`{"schema_version":1,"run_id":%q,"runtime_root":%q,"ltaoo":{"api_base":%q}}`, runID, runRoot, server.URL)
    if err := os.WriteFile(filepath.Join(runRoot, "manifest.json"), []byte(manifest), 0o600); err != nil { t.Fatal(err) }

    output, err := runProbeScript(t, "probe-ltaoo-comments.ps1", "-RunId", runID, "-ShareUrl", shareURL, "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
    if err != nil { t.Fatalf("probe failed: %v\n%s", err, output) }

    raw, err := os.ReadFile(filepath.Join(runRoot, "probe-summary.json"))
    if err != nil { t.Fatal(err) }
    text := string(raw)
    for _, secret := range []string{shareURL, oid, nid, marker, bait, "comment-one-secret", "comment-two-secret"} {
        if strings.Contains(text, secret) || strings.Contains(string(output), secret) { t.Errorf("secret leaked: %q", secret) }
    }
    var summary struct {
        Status string `json:"status"`
        CommentRequestCount int `json:"comment_request_count"`
        CursorContinuity bool `json:"cursor_continuity"`
        Pages []struct{ CommentCount int `json:"comment_count"` } `json:"pages"`
    }
    if err := json.Unmarshal(raw, &summary); err != nil { t.Fatal(err) }
    if summary.Status != "verified_two_pages" || summary.CommentRequestCount != 2 || !summary.CursorContinuity {
        t.Fatalf("unexpected summary: %+v", summary)
    }
    if len(summary.Pages) != 2 || summary.Pages[0].CommentCount != 1 || summary.Pages[1].CommentCount != 1 {
        t.Fatalf("unexpected pages: %+v", summary.Pages)
    }
    mu.Lock()
    defer mu.Unlock()
    if len(requests) != 4 { t.Fatalf("request count = %d, want status + profile + two comment pages", len(requests)) }
}
```

- [ ] **Step 2: Add no-second-page and remote-base rejection tests**

Add this concrete no-marker test; it accepts only status, profile, and one comment request:

```go
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
    runRoot := filepath.Join(probeRepoRoot(t), ".tmp_runtime", "ltaoo-probe", runID)
    t.Cleanup(func() { cleanupProbeTestRun(t, runRoot) })
    if err := os.MkdirAll(runRoot, 0o700); err != nil { t.Fatal(err) }
    manifest := fmt.Sprintf(`{"schema_version":1,"run_id":%q,"runtime_root":%q,"ltaoo":{"api_base":%q}}`, runID, runRoot, server.URL)
    if err := os.WriteFile(filepath.Join(runRoot, "manifest.json"), []byte(manifest), 0o600); err != nil { t.Fatal(err) }
    output, err := runProbeScript(t, "probe-ltaoo-comments.ps1", "-RunId", runID, "-ShareUrl", "https://weixin.qq.com/sph/test", "-RepoRoot", probeRepoRoot(t), "-ApiBase", server.URL)
    if err != nil { t.Fatalf("probe failed: %v\n%s", err, output) }
    var summary struct{ Status string `json:"status"`; CommentRequestCount int `json:"comment_request_count"` }
    readJSONFile(t, filepath.Join(runRoot, "probe-summary.json"), &summary)
    if summary.Status != "inconclusive_no_second_page" || summary.CommentRequestCount != 1 || commentRequests != 1 {
        t.Fatalf("unexpected result: summary=%+v requests=%d", summary, commentRequests)
    }
}
```

Add this direct remote-base invocation:

```go
func TestProbeRejectsRemoteAPIBase(t *testing.T) {
    runID := "test-remote"
    runRoot := filepath.Join(probeRepoRoot(t), ".tmp_runtime", "ltaoo-probe", runID)
    t.Cleanup(func() { cleanupProbeTestRun(t, runRoot) })
    if err := os.MkdirAll(runRoot, 0o700); err != nil { t.Fatal(err) }
    manifest := fmt.Sprintf(`{"schema_version":1,"run_id":%q,"runtime_root":%q,"ltaoo":{"api_base":"http://127.0.0.1:2022"}}`, runID, runRoot)
    if err := os.WriteFile(filepath.Join(runRoot, "manifest.json"), []byte(manifest), 0o600); err != nil { t.Fatal(err) }
    output, err := runProbeScript(t, "probe-ltaoo-comments.ps1",
        "-RunId", runID,
        "-ShareUrl", "https://weixin.qq.com/sph/test",
        "-RepoRoot", probeRepoRoot(t),
        "-ApiBase", "http://192.0.2.10:2022")
    if err == nil { t.Fatalf("remote API base accepted: %s", output) }
    if !strings.Contains(string(output), "api_base_not_loopback") { t.Fatalf("wrong failure: %s", output) }
    if strings.Contains(string(output), "https://weixin.qq.com/sph/test") { t.Fatal("share URL leaked in error output") }
}
```

- [ ] **Step 3: Run probe tests and confirm RED**

Run:

```powershell
go test ./scripts -run 'TestProbe' -v
```

Expected: FAIL because `probe-ltaoo-comments.ps1` still throws the scaffold error.

- [ ] **Step 4: Implement strict run and loopback validation**

Replace the scaffold with the approved parameter block and these concrete guards:

```powershell
function Resolve-ProbeRepoRoot {
    param([string]$Value)
    if ([string]::IsNullOrWhiteSpace($Value)) { return (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path }
    return (Resolve-Path -LiteralPath $Value).Path
}

function Resolve-RunRoot {
    param([string]$ResolvedRepoRoot, [string]$Value)
    $base = [IO.Path]::GetFullPath((Join-Path $ResolvedRepoRoot ".tmp_runtime\ltaoo-probe"))
    $candidate = [IO.Path]::GetFullPath((Join-Path $base $Value))
    if (-not $candidate.StartsWith($base + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "invalid_run_root"
    }
    return $candidate
}

function Assert-LoopbackApiBase {
    param([string]$Value)
    $uri = [Uri]$Value
    if ($uri.Scheme -ne "http" -or $uri.Host -ne "127.0.0.1" -or $uri.IsDefaultPort) { throw "api_base_not_loopback" }
    return $uri.GetLeftPart([UriPartial]::Authority).TrimEnd('/')
}
```

Load `.tmp_runtime/ltaoo-probe/<run_id>/manifest.json`, require `schema_version=1` and matching `run_id`, and use `ApiBase` only after `Assert-LoopbackApiBase`.

- [ ] **Step 5: Implement in-memory HTTP and hashing**

Use `System.Net.Http.HttpClient` with a 30-second timeout. Build every query with `[Uri]::EscapeDataString()`. The HTTP helper must return parsed JSON and may only throw stable reason codes:

```powershell
Add-Type -AssemblyName System.Net.Http
$client = [Net.Http.HttpClient]::new()
$client.Timeout = [TimeSpan]::FromSeconds(30)

function Invoke-LocalJsonGet {
    param([Net.Http.HttpClient]$Client, [string]$Uri, [string]$Stage)
    try {
        $response = $Client.GetAsync($Uri).GetAwaiter().GetResult()
        if (-not $response.IsSuccessStatusCode) { throw ($Stage + "_http_error") }
        $raw = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        return ($raw | ConvertFrom-Json)
    } catch {
        if ($_.Exception.Message -eq ($Stage + "_http_error")) { throw }
        throw ($Stage + "_request_failed")
    }
}

function Get-Sha256Hex {
    param([byte[]]$Bytes)
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return (($sha.ComputeHash($Bytes) | ForEach-Object { $_.ToString("x2") }) -join "") }
    finally { $sha.Dispose() }
}

function Get-TextHash { param([string]$Value); return Get-Sha256Hex ([Text.Encoding]::UTF8.GetBytes($Value)) }
function Get-SaltedTextHash {
    param([byte[]]$Salt, [string]$Value)
    $valueBytes = [Text.Encoding]::UTF8.GetBytes($Value)
    $combined = New-Object byte[] ($Salt.Length + $valueBytes.Length)
    [Array]::Copy($Salt, 0, $combined, 0, $Salt.Length)
    [Array]::Copy($valueBytes, 0, $combined, $Salt.Length, $valueBytes.Length)
    return Get-Sha256Hex $combined
}
```

Never include `$raw`, a requested URI, or an exception's original message in terminal output or JSON.

- [ ] **Step 6: Implement profile and exactly two comment requests**

Validate outer `code` and inner `data.errCode`. Read profile values only from `data.data.object.id` and `data.data.objectNonceId`. Read comments only from `data.data.commentInfo` and `data.data.lastBuffer`.

Use this fixed request sequence:

```powershell
$status = Invoke-LocalJsonGet $client ($apiBaseValue + "/api/status") "status"
$profileUrl = $apiBaseValue + "/api/channels/feed/profile?url=" + [Uri]::EscapeDataString($ShareUrl)
$profile = Invoke-LocalJsonGet $client $profileUrl "profile"
$oid = [string]$profile.data.data.object.id
$nid = [string]$profile.data.data.objectNonceId
$pageOneUrl = $apiBaseValue + "/api/channels/feed/comment/list?oid=" + [Uri]::EscapeDataString($oid) + "&nid=" + [Uri]::EscapeDataString($nid)
$pageOne = Invoke-LocalJsonGet $client $pageOneUrl "page_one"
$marker = [string]$pageOne.data.data.lastBuffer
$commentRequestCount = 1

if ([string]::IsNullOrEmpty($marker)) {
    $finalStatus = "inconclusive_no_second_page"
} else {
    $pageTwoUrl = $pageOneUrl + "&next_marker=" + [Uri]::EscapeDataString($marker)
    $pageTwo = Invoke-LocalJsonGet $client $pageTwoUrl "page_two"
    $commentRequestCount = 2
    $finalStatus = "verified_two_pages"
}
```

Immediately after the status call, require outer `code=0`, `data.api.listening=true`, and `data.proxy.listening=true`; otherwise throw `ltaoo_not_ready`. The implementation must not contain a loop around comment requests.

- [ ] **Step 7: Build and atomically write the redacted summary**

Generate 32 random salt bytes with `RandomNumberGenerator.Create().GetBytes()`. Hash comment IDs and cursors with the salt; hash share URL and `oid/nid` without salt as approved. Store only counts, hashes, lengths, status codes, business codes, `comment_request_count`, `cursor_continuity`, `status`, and `reason_code`.

Write with a same-directory temporary file followed by `[IO.File]::Replace()` when the target exists or `[IO.File]::Move()` when it does not. Terminal output is limited to:

```powershell
[pscustomobject]@{
    run_id = $RunId
    status = $finalStatus
    summary_file = $summaryPath
} | ConvertTo-Json -Compress
```

On a failure, write a redacted summary with `status="failed"` and a stable `reason_code`, dispose the client, and exit non-zero.

- [ ] **Step 8: Run probe tests and confirm GREEN**

Run:

```powershell
go test ./scripts -run 'TestProbe' -v
```

Expected: PASS, including exact marker continuity and no secret substrings.

- [ ] **Step 9: Commit the two-page probe**

```powershell
git add scripts/probe-ltaoo-comments.ps1 scripts/ltaoo_probe_script_test.go
git commit -m "feat: add redacted ltaoo two-page probe"
```

### Task 3: Baseline, ephemeral CA, and constrained ltaoo preparation

**Files:**
- Modify: `scripts/ltaoo_probe_script_test.go`
- Modify: `scripts/prepare-ltaoo-probe.ps1`

- [ ] **Step 1: Add static preparation safety tests**

Add a test that reads the script and checks required and forbidden strings:

```go
func TestPrepareScriptHasConstrainedConfiguration(t *testing.T) {
    raw, err := os.ReadFile(filepath.Join(probeRepoRoot(t), "scripts", "prepare-ltaoo-probe.ps1"))
    if err != nil { t.Fatal(err) }
    text := string(raw)
    for _, required := range []string{
        "CurrentUser\\Root", "skipInstallRootCert: true", "system: false", "tun: false",
        "127.0.0.1", "Get-NetRoute", "Get-NetTCPConnection", "CertificateRequest",
        "Pkcs8PrivateBlob", "certutil.exe", "-user", "-addstore", "Root", "Cert:\\LocalMachine\\Root",
        "wx_channel", ".tmp_runtime\\ltaoo-probe", "cleanup_not_implemented",
    } {
        if !strings.Contains(text, required) { t.Errorf("missing safety element %q", required) }
    }
    for _, forbidden := range []string{
        "Cert:\\LocalMachine\\Root\\", "system: true", "tun: true", "Set-ItemProperty", "Set-NetRoute", "New-NetRoute",
    } {
        if strings.Contains(text, forbidden) { t.Errorf("forbidden preparation behavior %q", forbidden) }
    }
}
```

- [ ] **Step 2: Run preparation test and confirm RED**

Run:

```powershell
go test ./scripts -run TestPrepareScript -v
```

Expected: FAIL because the scaffold lacks every required safety element.

- [ ] **Step 3: Implement canonical hashing and baseline capture**

Implement `Get-TextHash` as in Task 2 and `Get-Baseline`. It must build sensitive values in memory, serialize them with `ConvertTo-Json -Compress -Depth 8`, hash the serialization, and discard the serialization. Return only these fields:

```powershell
[ordered]@{
    schema_version = 1
    captured_at = [DateTimeOffset]::UtcNow.ToString("o")
    user_proxy_sha256 = Get-CanonicalHash $userProxy
    winhttp_proxy_sha256 = Get-CanonicalHash $winHttp
    route_table_sha256 = Get-CanonicalHash $routes
    current_user_roots_sha256 = Get-CanonicalHash $currentRoots
    local_machine_roots_sha256 = Get-CanonicalHash $machineRoots
    probe_listeners_sha256 = Get-CanonicalHash $listeners
    related_processes_sha256 = Get-CanonicalHash $processes
    api_port_in_use = [bool]$apiListener
    proxy_port_in_use = [bool]$proxyListener
}
```

Capture inputs with:

```powershell
Get-ItemProperty -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
& netsh.exe winhttp show proxy
Get-NetRoute | Sort-Object AddressFamily, DestinationPrefix, NextHop, RouteMetric, InterfaceIndex
Get-ChildItem Cert:\CurrentUser\Root | Select-Object -ExpandProperty Thumbprint | Sort-Object
Get-ChildItem Cert:\LocalMachine\Root | Select-Object -ExpandProperty Thumbprint | Sort-Object
Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -in @($ApiPort, $ProxyPort) }
Get-Process | Where-Object { $_.ProcessName -match 'clash|wechat|wx_video_download' }
```

Do not write or print any raw baseline input.

- [ ] **Step 4: Implement path, executable, port, and reparse-point guards**

Resolve `RepoRoot`, create `.tmp_runtime/ltaoo-probe`, generate `run_id` as `yyyyMMdd-HHmmss-` plus 12 lowercase hexadecimal random characters, and calculate the exact run root.

Reject the EXE when:

```powershell
$leaf = [IO.Path]::GetFileName($resolvedExe)
if ($leaf -match '^(?i)wx_channel.*\.exe$') { throw "nobiyou_executable_rejected" }
if ([IO.Path]::GetExtension($resolvedExe) -ne ".exe") { throw "ltaoo_executable_required" }
```

Reject any existing reparse point between the probe base and run root. Capture the baseline before creating the CA. If either `api_port_in_use` or `proxy_port_in_use` is true, write the baseline, print only `ports_already_in_use`, and stop.

- [ ] **Step 5: Implement .NET CA generation and private-key ACL**

Create an exportable `RSACng` key and a CA certificate:

```powershell
$rsa = [Security.Cryptography.RSA]::Create(2048)
$subject = "CN=WXChannelsPOC-$runId"
$request = [Security.Cryptography.X509Certificates.CertificateRequest]::new(
    $subject, $rsa, [Security.Cryptography.HashAlgorithmName]::SHA256,
    [Security.Cryptography.RSASignaturePadding]::Pkcs1
)
$request.CertificateExtensions.Add([Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]::new($true, $false, 0, $true))
$keyUsage = [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::KeyCertSign -bor [Security.Cryptography.X509Certificates.X509KeyUsageFlags]::CrlSign
$request.CertificateExtensions.Add([Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new($keyUsage, $true))
$request.CertificateExtensions.Add([Security.Cryptography.X509Certificates.X509SubjectKeyIdentifierExtension]::new($request.PublicKey, $false))
$cert = $request.CreateSelfSigned([DateTimeOffset]::UtcNow.AddMinutes(-5), [DateTimeOffset]::UtcNow.AddDays(2))
$certDer = $cert.Export([Security.Cryptography.X509Certificates.X509ContentType]::Cert)
$keyDer = $rsa.Key.Export([Security.Cryptography.CngKeyBlobFormat]::Pkcs8PrivateBlob)
```

Write PEM using a helper that emits 64-character Base64 lines and exact labels `CERTIFICATE` and `PRIVATE KEY`. For `ca-key.pem`, create a new `FileSecurity`, disable inheritance, set the owner to `[WindowsIdentity]::GetCurrent().User`, and grant that SID FullControl. Verify the resulting access rules contain no other non-inherited identity.

- [ ] **Step 6: Write constrained YAML and initial manifest**

Normalize certificate paths to forward slashes and double any literal double quote. Write exactly:

```yaml
api:
  protocol: http
  hostname: 127.0.0.1
  port: 2022
proxy:
  enabled: true
  system: false
  hostname: 127.0.0.1
  port: 2023
  tun: false
  skipInstallRootCert: true
cert:
  file: "D:/resolved/run/secrets/ca-cert.pem"
  key: "D:/resolved/run/secrets/ca-key.pem"
  name: "WXChannelsPOC-run-id"
```

Build that YAML with a PowerShell format string fed only the validated paths and selected integer ports. Write manifest schema version 1 with CA store `CurrentUser\Root`, exact thumbprint, subject and paths; ltaoo hash, null PID/start time, config path, `("http://127.0.0.1:{0}" -f $ApiPort)`, and `("127.0.0.1:{0}" -f $ProxyPort)`. Use atomic JSON writes.

- [ ] **Step 7: Add exact confirmation, install CurrentUser CA, and launch ltaoo**

Before displaying the confirmation, read `cleanup-ltaoo-probe.ps1` and require the exact marker `# LTAOO_PROBE_CLEANUP_READY=1`; if it is absent, throw `cleanup_not_implemented`. This prevents the preparation script from installing a CA while Task 4 cleanup is still a scaffold.

Print only `run_id`, CA thumbprint, API endpoint, proxy endpoint, and the statement that Clash/system proxy/routes will not be modified. Require:

```powershell
$expected = "INSTALL $runId"
$actual = Read-Host "Type '$expected' to install the CurrentUser test CA and start ltaoo"
if ($actual -cne $expected) { throw "confirmation_rejected" }
```

Install with `certutil.exe -user -addstore Root <certificate path>`, then verify exactly one `Cert:\CurrentUser\Root\<thumbprint>` exists. Launch with the generated config:

```powershell
$argumentLine = '-c "' + $configPath + '"'
$process = Start-Process -FilePath $resolvedExe -ArgumentList $argumentLine -WorkingDirectory $runRoot -PassThru -WindowStyle Hidden
$processStart = $process.StartTime.ToUniversalTime().ToString("o")
```

Update only manifest PID and process start time. Do not persist `$resolvedExe` or `$argumentLine`. Pre-confirmation information must use `Write-Host`; the only success object written to the pipeline is the final compressed JSON containing `run_id`, `manifest_file`, `api_base`, and `proxy_endpoint` so the runbook's `ConvertFrom-Json` is deterministic.

- [ ] **Step 8: Add rollback-on-error**

Wrap all actions after run-directory creation in `try/catch`. If a manifest exists, invoke:

```powershell
& (Join-Path $PSScriptRoot "cleanup-ltaoo-probe.ps1") -RunId $runId -RepoRoot $repoRootValue | Out-Null
```

Then output only `run_id` and a stable `reason_code`, and exit 1. Never include the original exception message.

- [ ] **Step 9: Run preparation safety tests and confirm GREEN**

Run:

```powershell
go test ./scripts -run TestPrepareScript -v
```

Expected: PASS. This task does not execute the real preparation script because certificate installation remains an explicit manual verification step.

- [ ] **Step 10: Commit preparation**

```powershell
git add scripts/prepare-ltaoo-probe.ps1 scripts/ltaoo_probe_script_test.go
git commit -m "feat: add constrained ltaoo probe preparation"
```

### Task 4: Exact idempotent cleanup

**Files:**
- Modify: `scripts/ltaoo_probe_script_test.go`
- Modify: `scripts/cleanup-ltaoo-probe.ps1`

- [ ] **Step 1: Add safe cleanup integration tests**

Add these fixture and cleanup tests. The fixture has an empty CA thumbprint and PID zero, so it never touches the certificate store or a process:

```go
func writeCleanupFixture(t *testing.T, runID string, privateKeyOverride string) string {
    t.Helper()
    runRoot := filepath.Join(probeRepoRoot(t), ".tmp_runtime", "ltaoo-probe", runID)
    secrets := filepath.Join(runRoot, "secrets")
    if err := os.MkdirAll(secrets, 0o700); err != nil { t.Fatal(err) }
    certPath := filepath.Join(secrets, "ca-cert.pem")
    keyPath := filepath.Join(secrets, "ca-key.pem")
    configPath := filepath.Join(runRoot, "ltaoo-probe.yaml")
    for path, body := range map[string]string{certPath: "cert", keyPath: "key", configPath: "config"} {
        if err := os.WriteFile(path, []byte(body), 0o600); err != nil { t.Fatal(err) }
    }
    if privateKeyOverride != "" { keyPath = privateKeyOverride }
    baseline := `{"schema_version":1,"user_proxy_sha256":"x","winhttp_proxy_sha256":"x","route_table_sha256":"x","current_user_roots_sha256":"x","local_machine_roots_sha256":"x","probe_listeners_sha256":"x","related_processes_sha256":"x"}`
    if err := os.WriteFile(filepath.Join(runRoot, "baseline.json"), []byte(baseline), 0o600); err != nil { t.Fatal(err) }
    manifest := fmt.Sprintf(`{"schema_version":1,"run_id":%q,"runtime_root":%q,"ca":{"store":"CurrentUser\\Root","thumbprint":"","certificate_file":%q,"private_key_file":%q},"ltaoo":{"pid":0,"process_start_time":"","executable_sha256":"","config_file":%q}}`, runID, runRoot, certPath, keyPath, configPath)
    if err := os.WriteFile(filepath.Join(runRoot, "manifest.json"), []byte(manifest), 0o600); err != nil { t.Fatal(err) }
    return runRoot
}

func TestCleanupIsIdempotentForRunOwnedFiles(t *testing.T) {
    runID := "test-cleanup-idempotent"
    runRoot := writeCleanupFixture(t, runID, "")
    t.Cleanup(func() { cleanupProbeTestRun(t, runRoot) })
    for attempt := 0; attempt < 2; attempt++ {
        output, err := runProbeScript(t, "cleanup-ltaoo-probe.ps1", "-RunId", runID, "-RepoRoot", probeRepoRoot(t))
        if err != nil { t.Fatalf("cleanup %d failed: %v\n%s", attempt+1, err, output) }
    }
    if _, err := os.Stat(filepath.Join(runRoot, "secrets", "ca-key.pem")); !os.IsNotExist(err) { t.Fatal("private key remains") }
    if _, err := os.Stat(filepath.Join(runRoot, "ltaoo-probe.yaml")); !os.IsNotExist(err) { t.Fatal("config remains") }
    if _, err := os.Stat(filepath.Join(runRoot, "cleanup-receipt.json")); err != nil { t.Fatal(err) }
}

func TestCleanupRejectsExternalManifestTarget(t *testing.T) {
    external := filepath.Join(t.TempDir(), "must-survive.key")
    if err := os.WriteFile(external, []byte("keep"), 0o600); err != nil { t.Fatal(err) }
    runID := "test-cleanup-external"
    runRoot := writeCleanupFixture(t, runID, external)
    t.Cleanup(func() { cleanupProbeTestRun(t, runRoot) })
    if output, err := runProbeScript(t, "cleanup-ltaoo-probe.ps1", "-RunId", runID, "-RepoRoot", probeRepoRoot(t)); err == nil {
        t.Fatalf("external target accepted: %s", output)
    }
    if _, err := os.Stat(external); err != nil { t.Fatalf("external target was changed: %v", err) }
}
```

The second invocation must exit zero. Both invocations must leave a valid receipt.

- [ ] **Step 2: Add static cleanup safety tests**

Add this complete static safety test:

```go
func TestCleanupScriptSecuritySurface(t *testing.T) {
    raw, err := os.ReadFile(filepath.Join(probeRepoRoot(t), "scripts", "cleanup-ltaoo-probe.ps1"))
    if err != nil { t.Fatal(err) }
    text := string(raw)
    for _, required := range []string{
        "# LTAOO_PROBE_CLEANUP_READY=1",
        "CurrentUser\\Root", "process_start_time", "executable_sha256", "Get-FileHash",
        "-user", "-delstore", "Remove-Item -LiteralPath", "cleanup_success",
    } {
        if !strings.Contains(text, required) { t.Errorf("cleanup missing %q", required) }
    }
    for _, forbidden := range []string{
        "Cert:\\LocalMachine\\Root\\", "uninstall_by_name", "Remove-Item -Recurse",
        "Set-ItemProperty", "Set-NetRoute", "New-NetRoute",
    } {
        if strings.Contains(text, forbidden) { t.Errorf("cleanup contains forbidden %q", forbidden) }
    }
}
```

- [ ] **Step 3: Run cleanup tests and confirm RED**

Run:

```powershell
go test ./scripts -run 'TestCleanup' -v
```

Expected: FAIL because cleanup still throws the scaffold error.

- [ ] **Step 4: Implement run-root and target-path validation**

Replace the scaffold header with `# LTAOO_PROBE_CLEANUP_READY=1`, strict mode, the approved parameter block, and the cleanup implementation. Use the same `Resolve-ProbeRepoRoot` and `Resolve-RunRoot` semantics as Task 2. Validate all existing path segments from probe base through run root with:

```powershell
if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "reparse_point_rejected" }
```

For every manifest target, compare `[IO.Path]::GetFullPath()` to the validated run-root prefix using `OrdinalIgnoreCase`. Reject a target equal to the run root or outside it.

- [ ] **Step 5: Implement exact process identity cleanup**

When PID is greater than zero:

```powershell
$process = Get-Process -Id ([int]$manifest.ltaoo.pid) -ErrorAction SilentlyContinue
if ($null -ne $process) {
    $startMatches = $process.StartTime.ToUniversalTime().ToString("o") -eq [string]$manifest.ltaoo.process_start_time
    $imageHash = (Get-FileHash -LiteralPath $process.Path -Algorithm SHA256).Hash.ToLowerInvariant()
    $hashMatches = $imageHash -eq ([string]$manifest.ltaoo.executable_sha256).ToLowerInvariant()
    if ($startMatches -and $hashMatches) {
        Stop-Process -Id $process.Id -Force
        Wait-Process -Id $process.Id -Timeout 10 -ErrorAction SilentlyContinue
    } else {
        $warnings.Add("ltaoo_process_identity_mismatch")
    }
}
```

Do not stop a process when either identity comparison fails.

- [ ] **Step 6: Implement exact CurrentUser certificate cleanup**

If thumbprint is non-empty, require manifest store to equal `CurrentUser\Root`, normalize thumbprint to uppercase hexadecimal, and reject any other characters. Check only `Cert:\CurrentUser\Root\<thumbprint>`. Delete with:

```powershell
& certutil.exe -user -delstore Root $thumbprint | Out-Null
if (Test-Path -LiteralPath ("Cert:\CurrentUser\Root\" + $thumbprint)) { throw "ca_removal_failed" }
```

An already absent exact thumbprint is a successful no-op. Never call deletion with a subject or CN.

- [ ] **Step 7: Delete only exact run-owned files and write receipt**

Validate and remove, in order: private key, public certificate, generated configuration. Use only `Remove-Item -LiteralPath <validated path> -Force`. Delete the `secrets` directory only with `Remove-Item -LiteralPath $secretsRoot` after verifying it has no children; never use `-Recurse`.

Capture a new baseline with the same algorithm and field names as preparation. Compare each hash to `baseline.json`; add category names to `external_drift_warnings` but do not modify those categories. Atomically write:

```json
{
  "schema_version": 1,
  "run_id": "validated-run-id",
  "process_stopped": true,
  "ca_absent": true,
  "private_key_absent": true,
  "certificate_file_absent": true,
  "config_absent": true,
  "external_drift_warnings": [],
  "cleanup_success": true,
  "completed_at": "UTC ISO-8601"
}
```

`cleanup_success` is false if CA absence or secret absence cannot be proven. Drift warnings alone do not make it false.

- [ ] **Step 8: Run cleanup tests and confirm GREEN**

Run:

```powershell
go test ./scripts -run 'TestCleanup' -v
```

Expected: PASS twice for the same run and PASS for refusing the external file target.

- [ ] **Step 9: Commit cleanup**

```powershell
git add scripts/cleanup-ltaoo-probe.ps1 scripts/ltaoo_probe_script_test.go
git commit -m "feat: add exact ltaoo probe cleanup"
```

### Task 5: Operator runbook

**Files:**
- Create: `docs/LTAOO_TWO_PAGE_PROBE.md`
- Modify: `scripts/ltaoo_probe_script_test.go`

- [ ] **Step 1: Add a documentation completeness test**

```go
func TestLtaooProbeRunbookHasSafetySequence(t *testing.T) {
    raw, err := os.ReadFile(filepath.Join(probeRepoRoot(t), "docs", "LTAOO_TWO_PAGE_PROBE.md"))
    if err != nil { t.Fatal(err) }
    text := string(raw)
    for _, required := range []string{
        "prepare-ltaoo-probe.ps1", "probe-ltaoo-comments.ps1", "cleanup-ltaoo-probe.ps1",
        "INSTALL", "CurrentUser\\Root", "verified_two_pages", "inconclusive_no_second_page",
        "cleanup_success", "Clash", "system: false", "tun: false",
    } {
        if !strings.Contains(text, required) { t.Errorf("runbook missing %q", required) }
    }
}
```

- [ ] **Step 2: Run the documentation test and confirm RED**

Run:

```powershell
go test ./scripts -run TestLtaooProbeRunbook -v
```

Expected: FAIL because the runbook does not exist.

- [ ] **Step 3: Write the fixed operator sequence**

Create `docs/LTAOO_TWO_PAGE_PROBE.md` with these sections and concrete commands:

```powershell
$prepare = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/prepare-ltaoo-probe.ps1 -LtaooExePath 'D:\Tools\ltaoo-src\wx_video_download.exe'
$run = $prepare | ConvertFrom-Json
$shareUrl = Read-Host 'Paste the selected public WeChat Channels share URL'

& powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/probe-ltaoo-comments.ps1 `
  -RunId $run.run_id `
  -ShareUrl $shareUrl

& powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/cleanup-ltaoo-probe.ps1 -RunId $run.run_id
```

Explain before the commands:

- build ltaoo from reviewed source;
- configure Clash's official ltaoo forwarding manually;
- keep ltaoo `system:false`, `tun:false`, `skipInstallRootCert:true`;
- open/refresh a WeChat Channels page after preparation;
- choose a work with more than one page of public comments;
- never paste the share URL into an issue, commit, or retained log.

Explain after the commands:

- `verified_two_pages` opens the adapter-design gate;
- `inconclusive_no_second_page` requires a new run with a larger sample;
- `failed` stops integration until its stage-specific reason is diagnosed;
- cleanup is mandatory on success, inconclusive, failure, or manual interruption;
- inspect `cleanup-receipt.json` and require CA/key/config absence; drift warnings are reviewed manually and never auto-restored.

- [ ] **Step 4: Run the documentation test and confirm GREEN**

Run:

```powershell
go test ./scripts -run TestLtaooProbeRunbook -v
```

Expected: PASS.

- [ ] **Step 5: Commit the runbook**

```powershell
git add docs/LTAOO_TWO_PAGE_PROBE.md scripts/ltaoo_probe_script_test.go
git commit -m "docs: add ltaoo two-page probe runbook"
```

### Task 6: Full verification and manual-test gate

**Files:**
- Modify only if a verification failure requires a scoped correction: `.gitignore`, `scripts/prepare-ltaoo-probe.ps1`, `scripts/probe-ltaoo-comments.ps1`, `scripts/cleanup-ltaoo-probe.ps1`, `scripts/ltaoo_probe_script_test.go`, `docs/LTAOO_TWO_PAGE_PROBE.md`

- [ ] **Step 1: Format the Go test**

Run:

```powershell
gofmt -w scripts/ltaoo_probe_script_test.go
```

Expected: no output.

- [ ] **Step 2: Run all script tests**

Run:

```powershell
go test ./scripts -v
```

Expected: PASS. Tests must not prompt for confirmation, install certificates, alter network settings, or launch ltaoo.

- [ ] **Step 3: Run the repository test suite**

Run:

```powershell
go test ./...
```

Expected: PASS. If a pre-existing unrelated test fails, record the exact package and error and do not weaken probe tests.

- [ ] **Step 4: Run Git and secret-surface checks**

Run:

```powershell
git check-ignore .tmp_runtime/ltaoo-probe/example/ca-key.pem
git check-ignore -v scripts/prepare-ltaoo-probe.ps1
git diff --check
rg -n "Cert:\\\\LocalMachine\\\\Root\\\\|system:\s*true|tun:\s*true|Remove-Item\s+-Recurse|uninstall_by_name" scripts/prepare-ltaoo-probe.ps1 scripts/probe-ltaoo-comments.ps1 scripts/cleanup-ltaoo-probe.ps1
```

Expected:

- runtime key path is ignored;
- prepare script is explicitly unignored by a negation rule;
- `git diff --check` has no output;
- forbidden-pattern search has no output.

- [ ] **Step 5: Verify tracked files contain no runtime secrets**

Run:

```powershell
git status --short
git ls-files | rg "ca-key|ca-cert|probe-summary|baseline.json|cleanup-receipt|ltaoo-probe.yaml"
```

Expected: the status contains only intended source/document changes; the tracked-file search returns no runtime artifact.

- [ ] **Step 6: Commit any verification-only correction**

If Step 1 formatting or a scoped correction changed files:

```powershell
git add .gitignore scripts/prepare-ltaoo-probe.ps1 scripts/probe-ltaoo-comments.ps1 scripts/cleanup-ltaoo-probe.ps1 scripts/ltaoo_probe_script_test.go docs/LTAOO_TWO_PAGE_PROBE.md
git commit -m "test: verify ltaoo two-page probe"
```

If no files changed, do not create an empty commit.

- [ ] **Step 7: Stop before real certificate installation**

Report automated verification results and the exact manual command from the runbook. Do not run `prepare-ltaoo-probe.ps1` against the primary machine until the user supplies or approves the reviewed source-built ltaoo EXE path and explicitly authorizes the CurrentUser CA installation shown by the script.

## Completion criteria

Implementation is ready for the real compatibility gate only when:

- all automated tests pass;
- the tracked scripts cannot modify Clash, system proxy, WinHTTP proxy, route table, or LocalMachine certificate store;
- the probe test proves exactly two comment requests and exact cursor continuity;
- bait secrets do not appear in terminal output or `probe-summary.json`;
- cleanup tests prove idempotence and refusal of out-of-run targets;
- the runbook makes real CA installation an explicit, visible human action;
- no real CA has been installed during automated implementation or verification.
