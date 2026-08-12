# Certificate Install Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the restricted Windows POC install its per-job CA non-interactively in `CurrentUser\Root` and report only allowlisted certificate-stage error codes when installation fails.

**Architecture:** Keep the existing `Import-Certificate` boundary and exact SHA-256 cleanup. Add a cross-platform typed error-code unit that the Windows certificate store can return and the CLI can safely render; harden the PowerShell script so non-error streams cannot contaminate its boolean protocol. Do not start bridge, proxy, driver, or collection until certificate import and post-import fingerprint verification both succeed.

**Tech Stack:** Go 1.24.3, Windows PowerShell 5.1 PKI module, TDM-GCC 10.3.0, Go tests, existing POC build/security-audit scripts, VMware read-only ISO workflow.

---

### Task 1: Add closed certificate error codes and safe CLI rendering

**Files:**
- Create: `internal/poc/certificate_error.go`
- Create: `internal/poc/certificate_error_test.go`
- Modify: `internal/poc/app.go`

- [ ] **Step 1: Write the failing whitelist test**

Create `internal/poc/certificate_error_test.go`:

```go
package poc

import (
    "errors"
    "strings"
    "testing"
)

func TestWriteSafeRunErrorOnlyPrintsAllowlistedCertificateCode(t *testing.T) {
    var output strings.Builder
    writeSafeRunError(&output, newCertificateStoreError(certificateImportCommandFailed))
    if got := output.String(); got != "POC run error code: certificate_import_command_failed\n" {
        t.Fatalf("output=%q", got)
    }

    output.Reset()
    writeSafeRunError(&output, errors.New(`secret C:\path fingerprint ABC token XYZ`))
    if output.Len() != 0 {
        t.Fatalf("untrusted error leaked: %q", output.String())
    }
}
```

- [ ] **Step 2: Run the test and verify the missing API failure**

Run in the implementation worktree with the approved compiler environment:

```powershell
$toolBin=(Resolve-Path '.\.poc-tools\tdm-gcc-10.3.0-2\bin').Path
$env:CGO_ENABLED='1'
$env:PATH="$toolBin;$env:PATH"
$env:CC=Join-Path $toolBin 'gcc.exe'
$env:CXX=Join-Path $toolBin 'g++.exe'
go test ./internal/poc -run TestWriteSafeRunErrorOnlyPrintsAllowlistedCertificateCode -count=1
```

Expected: FAIL because `writeSafeRunError`, `newCertificateStoreError`, and `certificateImportCommandFailed` do not exist.

- [ ] **Step 3: Implement the closed error-code type**

Create `internal/poc/certificate_error.go`:

```go
package poc

import "errors"

type certificateErrorCode string

const (
    certificatePrecheckCommandFailed certificateErrorCode = "certificate_precheck_command_failed"
    certificateAlreadyPresent        certificateErrorCode = "certificate_already_present"
    certificateImportCommandFailed   certificateErrorCode = "certificate_import_command_failed"
    certificateImportReportedFalse   certificateErrorCode = "certificate_import_reported_false"
    certificatePostcheckFailed       certificateErrorCode = "certificate_postcheck_failed"
)

type certificateStoreError struct{ code certificateErrorCode }

func (e certificateStoreError) Error() string { return string(e.code) }

func newCertificateStoreError(code certificateErrorCode) error {
    switch code {
    case certificatePrecheckCommandFailed, certificateAlreadyPresent,
        certificateImportCommandFailed, certificateImportReportedFalse,
        certificatePostcheckFailed:
        return certificateStoreError{code: code}
    default:
        return errors.New("certificate store failure")
    }
}

func safeCertificateErrorCode(err error) (string, bool) {
    var target certificateStoreError
    if !errors.As(err, &target) {
        return "", false
    }
    switch target.code {
    case certificatePrecheckCommandFailed, certificateAlreadyPresent,
        certificateImportCommandFailed, certificateImportReportedFalse,
        certificatePostcheckFailed:
        return string(target.code), true
    default:
        return "", false
    }
}
```

- [ ] **Step 4: Add the safe CLI helper and call it on run failure**

Add to `internal/poc/app.go`:

```go
func writeSafeRunError(output io.Writer, err error) {
    if code, ok := safeCertificateErrorCode(err); ok {
        _, _ = fmt.Fprintf(output, "POC run error code: %s\n", code)
    }
}
```

Change the run failure branch to:

```go
if err := runtime.Run(ctx, options); err != nil {
    writeSafeRunError(output, err)
    _, _ = fmt.Fprintln(output, "POC run did not complete; cleanup path executed")
    return 1
}
```

- [ ] **Step 5: Run the targeted test and secret-leak audit test**

Run:

```powershell
go test ./internal/poc -run 'TestWriteSafeRunErrorOnlyPrintsAllowlistedCertificateCode|TestSafeLoggerRejectsContentURLAndCredentialShapedValues' -count=1
```

Expected: PASS; arbitrary error text produces no CLI output.

- [ ] **Step 6: Commit the closed error-code unit**

```powershell
git add -- internal/poc/certificate_error.go internal/poc/certificate_error_test.go internal/poc/app.go
git commit -m "feat: report allowlisted certificate errors"
```

### Task 2: Harden the CurrentUser certificate import protocol

**Files:**
- Modify: `internal/poc/certstore_windows.go`
- Modify: `internal/poc/certstore_windows_test.go`
- Modify: `internal/poc/runtime_test.go`

- [ ] **Step 1: Extend the script-boundary test so it fails first**

Extend `TestCertificateScriptsUseCurrentUserOnly` in `internal/poc/certstore_windows_test.go`:

```go
func TestCertificateScriptsUseCurrentUserOnly(t *testing.T) {
    combined := certificateMatchScript + certificateInstallScript + certificateRemoveScript
    lower := strings.ToLower(combined)
    for _, required := range []string{
        `Cert:\CurrentUser\Root`, `-Confirm:$false`,
        `$WarningPreference = 'SilentlyContinue'`,
        `$InformationPreference = 'SilentlyContinue'`,
        `$ProgressPreference = 'SilentlyContinue'`,
    } {
        if !strings.Contains(combined, required) {
            t.Fatalf("certificate script missing %q", required)
        }
    }
    if strings.Contains(lower, "localmachine") {
        t.Fatal("certificate scripts are not restricted to CurrentUser Root")
    }
}
```

- [ ] **Step 2: Add command-result-to-error-code tests**

Add a deterministic runner and tests in `internal/poc/certstore_windows_test.go`:

```go
type certificateResult struct {
    output []byte
    err    error
}

type certificateRunner struct{ results []certificateResult }

func (r *certificateRunner) Run(context.Context, string, ...string) ([]byte, error) {
    result := r.results[0]
    r.results = r.results[1:]
    return result.output, result.err
}

func writeTestJobCertificate(t *testing.T) (string, string) {
    t.Helper()
    ca, err := GenerateJobCA("job-cert-errors", time.Now().UTC())
    if err != nil {
        t.Fatal(err)
    }
    dir := filepath.Join(t.TempDir(), ".poc-secrets", "job-cert-errors")
    if err := os.MkdirAll(dir, 0o700); err != nil {
        t.Fatal(err)
    }
    certPath, _, err := ca.WriteSecrets(dir)
    if err != nil {
        t.Fatal(err)
    }
    return certPath, ca.SHA256Fingerprint
}

func assertCertificateErrorCode(t *testing.T, err error, want certificateErrorCode) {
    t.Helper()
    code, ok := safeCertificateErrorCode(err)
    if !ok || code != string(want) {
        t.Fatalf("code=%q ok=%v err=%v", code, ok, err)
    }
}

func TestCertificateInstallMapsPrecheckCommandFailure(t *testing.T) {
    certPath, fingerprint := writeTestJobCertificate(t)
    runner := &certificateRunner{results: []certificateResult{{err: errors.New("fixture")}}}
    err := (&windowsCertificateStore{runner: runner}).Install(context.Background(), certPath, fingerprint)
    assertCertificateErrorCode(t, err, certificatePrecheckCommandFailed)
}

func TestCertificateInstallRejectsAlreadyPresentFingerprint(t *testing.T) {
    certPath, fingerprint := writeTestJobCertificate(t)
    runner := &certificateRunner{results: []certificateResult{{output: []byte("true")}}}
    err := (&windowsCertificateStore{runner: runner}).Install(context.Background(), certPath, fingerprint)
    assertCertificateErrorCode(t, err, certificateAlreadyPresent)
}

func TestCertificateInstallMapsImportCommandFailure(t *testing.T) {
    certPath, fingerprint := writeTestJobCertificate(t)
    runner := &certificateRunner{results: []certificateResult{{output: []byte("false")}, {err: errors.New("fixture")}}}
    err := (&windowsCertificateStore{runner: runner}).Install(context.Background(), certPath, fingerprint)
    assertCertificateErrorCode(t, err, certificateImportCommandFailed)
}

func TestCertificateInstallMapsImportReportedFalse(t *testing.T) {
    certPath, fingerprint := writeTestJobCertificate(t)
    runner := &certificateRunner{results: []certificateResult{
        {output: []byte("false")}, {output: []byte("false")},
    }}
    err := (&windowsCertificateStore{runner: runner}).Install(context.Background(), certPath, fingerprint)
    assertCertificateErrorCode(t, err, certificateImportReportedFalse)
}

func TestCertificateInstallRequiresSuccessfulPostcheck(t *testing.T) {
    certPath, fingerprint := writeTestJobCertificate(t)
    runner := &certificateRunner{results: []certificateResult{
        {output: []byte("false")}, {output: []byte("true")}, {output: []byte("false")},
    }}
    err := (&windowsCertificateStore{runner: runner}).Install(context.Background(), certPath, fingerprint)
    assertCertificateErrorCode(t, err, certificatePostcheckFailed)
}
```

- [ ] **Step 3: Run the new tests and verify they fail**

Run:

```powershell
go test ./internal/poc -run 'TestCertificateScriptsUseCurrentUserOnly|TestCertificateInstallMaps|TestCertificateInstallRejectsAlreadyPresentFingerprint|TestCertificateInstallRequiresSuccessfulPostcheck' -count=1
```

Expected: FAIL because the install script lacks explicit non-interactive controls and `Install` returns a generic error without postcheck.

- [ ] **Step 4: Harden the PowerShell import script**

Replace `certificateInstallScript` in `internal/poc/certstore_windows.go` with a concatenated raw string equivalent to:

```powershell
$WarningPreference = 'SilentlyContinue'
$InformationPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$path = $args[0]
$expected = $args[1].ToUpperInvariant()
$cert = @(Import-Certificate -FilePath $path -CertStoreLocation Cert:\CurrentUser\Root -Confirm:$false -WarningAction SilentlyContinue -InformationAction SilentlyContinue -ErrorAction Stop)
if ($cert.Count -ne 1) { [Console]::Out.Write('false'); exit 0 }
$sha = [Security.Cryptography.SHA256]::Create()
try { $hash = ([BitConverter]::ToString($sha.ComputeHash($cert[0].RawData))).Replace('-','') } finally { $sha.Dispose() }
if ($hash -ne $expected) {
    Remove-Item -LiteralPath $cert[0].PSPath -Force -ErrorAction SilentlyContinue
    [Console]::Out.Write('false')
    exit 0
}
[Console]::Out.Write('true')
```

Keep the Go constant on one logical command line so `powershell.exe -Command` receives one script block.

- [ ] **Step 5: Map each certificate stage and add a postcheck**

Change the command portion of `Install` to:

```go
present, err := s.ContainsSHA256(ctx, fingerprint)
if err != nil {
    return newCertificateStoreError(certificatePrecheckCommandFailed)
}
if present {
    return newCertificateStoreError(certificateAlreadyPresent)
}
installed, err := s.runBoolean(ctx, certificateInstallScript, certPath, fingerprint)
if err != nil {
    return newCertificateStoreError(certificateImportCommandFailed)
}
if !installed {
    return newCertificateStoreError(certificateImportReportedFalse)
}
present, err = s.ContainsSHA256(ctx, fingerprint)
if err != nil || !present {
    return newCertificateStoreError(certificatePostcheckFailed)
}
return nil
```

- [ ] **Step 6: Add a runtime regression test for ambiguous import success**

Add to `internal/poc/runtime_test.go` a store that simulates `Import-Certificate` succeeding while its command channel reports failure:

```go
type ambiguousInstallCertStore struct{ events *runtimeEvents }

func (s ambiguousInstallCertStore) Install(context.Context, string, string) error {
    s.events.add("install-ca")
    return newCertificateStoreError(certificateImportCommandFailed)
}
func (s ambiguousInstallCertStore) ContainsSHA256(context.Context, string) (bool, error) {
    return true, nil
}
func (s ambiguousInstallCertStore) RemoveBySHA256(context.Context, string) error {
    s.events.add("remove-ca")
    return nil
}

func TestRuntimeCleansCertificateWhenImportOutcomeIsAmbiguous(t *testing.T) {
    deps, events := fakeRuntimeDeps(t, func(context.Context) error { return nil })
    var store *Store
    deps.StoreFactory = func(_ Options, jobID string) (*Store, error) {
        store = newTestStore(t, jobID)
        return store, nil
    }
    deps.CertStore = ambiguousInstallCertStore{events: events}

    err := NewRuntime(deps).Run(context.Background(), approvedTestOptions())
    assertCertificateErrorCode(t, err, certificateImportCommandFailed)
    assertRuntimeEvents(t, events.snapshot(), []string{
        "preflight", "create-ca", "install-ca", "remove-ca",
        "destroy-secrets", "write-cleanup-receipt",
    })
    assertSuccessfulCleanupReceipt(t, store)
}
```

This proves that the existing runtime containment check records ownership and removes the job CA before bridge, proxy, driver, or collection can start.

- [ ] **Step 7: Run certificate and cleanup tests**

Run:

```powershell
go test ./internal/poc -run 'TestCertificate|TestGeneratedCertificate|TestRuntimeCleansCertificateWhenImportOutcomeIsAmbiguous|TestRuntimeCleansUpInReverseOrderOnCollectorFailure' -count=1
```

Expected: PASS, including the existing `Import-Certificate -WhatIf` test and cleanup receipt assertions.

- [ ] **Step 8: Commit the non-interactive import fix**

```powershell
git add -- internal/poc/certstore_windows.go internal/poc/certstore_windows_test.go internal/poc/runtime_test.go
git commit -m "fix: harden CurrentUser certificate import"
```

### Task 3: Verify, package, and perform the single remaining VM attempt

**Files:**
- Modify mechanically: `D:\Agent\vm-poc-downloads\poc-runtime-media-<commit>\media-manifest.json`
- Create mechanically: `D:\Agent\vm-poc-downloads\wechat-channel-poc-source-<commit>.iso`
- No tracked source changes unless verification finds a defect.

- [ ] **Step 1: Run the full scoped test suite**

```powershell
$toolBin=(Resolve-Path '.\.poc-tools\tdm-gcc-10.3.0-2\bin').Path
$env:CGO_ENABLED='1'
$env:PATH="$toolBin;$env:PATH"
$env:CC=Join-Path $toolBin 'gcc.exe'
$env:CXX=Join-Path $toolBin 'g++.exe'
go test ./internal/poc ./internal/pocaudit ./scripts -count=1
```

Expected: all three packages PASS.

- [ ] **Step 2: Rebuild and run the security audit**

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-poc.ps1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/poc-security-audit.ps1
```

Expected: `all modules verified` and `POC security audit passed`.

- [ ] **Step 3: Create the exact-commit bundle and update media constants**

Create a new staging directory rather than overwriting prior evidence. Copy the approved tools and helper scripts from the most recent staging directory, excluding its bundle and manifest. Then run:

```powershell
$commit=(git rev-parse HEAD).Trim()
$short=$commit.Substring(0,7)
$media="D:\Agent\vm-poc-downloads\poc-runtime-media-$short"
git bundle create (Join-Path $media 'wx_channel.bundle') refs/heads/codex/wechat-channel-comment-poc
git bundle verify (Join-Path $media 'wx_channel.bundle')
Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $media 'wx_channel.bundle')
```

Update `prepare-build-audit.ps1`, `update-preflight-fix.ps1`, and `README.txt` in the staging directory with the exact old commit, new commit, and new bundle hash. The updater must accept only the currently installed VM commit and fast-forward only to `$commit`.

- [ ] **Step 4: Generate and verify the read-only ISO**

Use the already-audited local helper:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File 'D:\Agent\vm-poc-downloads\tmp\new-poc-iso.ps1' `
  -Source $media `
  -Path "D:\Agent\vm-poc-downloads\wechat-channel-poc-source-$short.iso" `
  -Title 'POC_SOURCE'
```

Mount the ISO, parse `media-manifest.json`, and require every listed file's size and SHA-256 to match before dismounting. Expected: volume label `POC_SOURCE`, exact source commit, zero missing/size/hash failures.

- [ ] **Step 5: Update and preflight the VM**

In VMware, attach the new ISO read-only. In the guest run `4_UPDATE_PREFLIGHT_FIX.cmd`; verify `vm-build-receipt.json` contains the exact source commit and `audit_passed: true`. Run `2_RUN_PREFLIGHT.cmd`; require `passed:true`, all baseline booleans true, `reason_codes:null`, and exit code 0.

- [ ] **Step 6: Perform the single remaining real validation attempt**

Close the Channels window. Start `3_OPEN_POC_SHELL.cmd` as Administrator because the audited NFAPI driver registration writes to the VM system driver directory. Run:

```cmd
.\.poc-build\wx_channel_poc.exe run --ack-isolated-vm
```

Verify the displayed plan remains `CurrentUser\Root`, `127.0.0.1:2025`, `127.0.0.1:2026`, and `WeChatAppEx.exe`; then the operator types exact `APPLY`. Reopen Channels and enter its search interface. Handle login or human verification manually; never bypass CAPTCHA.

If the run fails, record only the allowlisted error code and safe phase log, classify the result as `inconclusive`, and do not retry. If it proceeds, wait for completion and inspect only manifest status/counts and validation field statuses, not comment content in chat.

- [ ] **Step 7: Verify cleanup and terminate the disposable environment**

Require `cleanup-receipt.json` success for requests, process rule, proxy, bridge, job-owned driver, CurrentUser CA, secrets, and encrypted raw evidence. Run preflight again to confirm CA/driver/listener baseline is clean. Copy only approved redacted outputs to a Git-ignored controlled location, disable the host proxy bridge, then revert to `wechat-login-baseline` or destroy the VM. If any cleanup category is uncertain, destroy the VM instead of trusting partial cleanup.
