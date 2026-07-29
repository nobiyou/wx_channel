# X509Store Certificate Smoke Test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a non-admin, VM-only `cert-smoke` command that proves a per-job CA can be installed in and removed from `CurrentUser\Root` through `.NET X509Store`, without starting any proxy, driver, WeChat integration, or collection code.

**Architecture:** Replace the Windows certificate store's `Import-Certificate` scripts with `.NET X509Store` scripts that decide success only by re-enumerating RawData SHA-256 matches. Add a small platform-independent certificate-smoke state machine with injected dependencies, a Windows-only non-elevation gate, a closed receipt/error schema, and a separate CLI command. Reuse the existing persisted runtime state only as a crash-cleanup ownership record; never assemble the normal runtime's bridge/proxy/driver dependencies.

**Tech Stack:** Go 1.24.3, `golang.org/x/sys/windows`, Windows PowerShell 5.1 hosting `.NET System.Security.Cryptography.X509Certificates.X509Store`, existing TDM-GCC 10.3.0 build, Go tests, existing POC security audit, VMware read-only ISO workflow.

---

### Task 1: Replace Import-Certificate with exact X509Store operations

**Files:**
- Modify: `internal/poc/certstore_windows.go`
- Modify: `internal/poc/certstore_windows_test.go`

- [ ] **Step 1: Replace the script boundary test first**

Replace `TestCertificateScriptsUseCurrentUserOnly` and the `Import-Certificate -WhatIf` test with:

```go
func TestCertificateScriptsUseCurrentUserX509StoreOnly(t *testing.T) {
	combined := certificateMatchScript + certificateInstallScript + certificateRemoveScript
	lower := strings.ToLower(combined)
	for _, required := range []string{
		"X509Store", "StoreName]::Root", "StoreLocation]::CurrentUser",
		"OpenFlags]::ReadWrite", ".Add(", ".Remove(", "SHA256",
		"$WarningPreference = 'SilentlyContinue'",
		"$InformationPreference = 'SilentlyContinue'",
		"$ProgressPreference = 'SilentlyContinue'",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("certificate script missing %q", required)
		}
	}
	for _, forbidden := range []string{"import-certificate", "certutil", "localmachine"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("certificate script contains forbidden fallback %q", forbidden)
		}
	}
}

func TestGeneratedCertificateCanBeLoadedByX509Certificate2(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}
	certPath, fingerprint := writeTestJobCertificate(t)
	store := &windowsCertificateStore{runner: ExecCommandRunner{}}
	script := `$cert = [Security.Cryptography.X509Certificates.X509Certificate2]::new($args[0]); try { $sha = [Security.Cryptography.SHA256]::Create(); try { $hash = ([BitConverter]::ToString($sha.ComputeHash($cert.RawData))).Replace('-','') } finally { $sha.Dispose() }; [Console]::Out.Write(($hash -eq $args[1]).ToString().ToLowerInvariant()) } finally { $cert.Dispose() }`
	ok, err := store.runBoolean(context.Background(), script, certPath, fingerprint)
	if err != nil || !ok {
		t.Fatalf("generated certificate was not accepted by X509Certificate2: ok=%v err=%v", ok, err)
	}
}
```

Add an idempotent removal protocol test:

```go
func TestCertificateRemoveRequiresSuccessfulZeroMatchPostcheck(t *testing.T) {
	runner := &certificateRunner{results: []certificateResult{
		{output: []byte("true")},
		{output: []byte("false")},
	}}
	store := &windowsCertificateStore{runner: runner}
	if err := store.RemoveBySHA256(context.Background(), strings.Repeat("A", 64)); err != nil {
		t.Fatal(err)
	}
	if len(runner.results) != 0 {
		t.Fatalf("unused command results=%d", len(runner.results))
	}
}
```

- [ ] **Step 2: Run the boundary tests and verify they fail**

Run:

```powershell
$toolBin=(Resolve-Path '.\.poc-tools\tdm-gcc-10.3.0-2\bin').Path
$env:CGO_ENABLED='1'
$env:PATH="$toolBin;$env:PATH"
$env:CC=Join-Path $toolBin 'gcc.exe'
$env:CXX=Join-Path $toolBin 'g++.exe'
go test ./internal/poc -run 'TestCertificateScriptsUseCurrentUserX509StoreOnly|TestGeneratedCertificateCanBeLoadedByX509Certificate2|TestCertificateRemoveRequiresSuccessfulZeroMatchPostcheck' -count=1
```

Expected: FAIL because the production constants still contain `Import-Certificate`, do not contain `X509Store.Add/Remove`, and `RemoveBySHA256` has no zero-match postcheck.

- [ ] **Step 3: Replace the three PowerShell scripts**

Keep each Go constant as one logical PowerShell command passed through `powerShellCommandArgs`. The scripts must be equivalent to the following blocks.

Match script:

```powershell
$WarningPreference = 'SilentlyContinue'
$InformationPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$expected = $args[0].ToUpperInvariant()
$store = [Security.Cryptography.X509Certificates.X509Store]::new(
    [Security.Cryptography.X509Certificates.StoreName]::Root,
    [Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser)
try {
    $store.Open([Security.Cryptography.X509Certificates.OpenFlags]::ReadOnly)
    $matches = @($store.Certificates | Where-Object {
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $hash = ([BitConverter]::ToString($sha.ComputeHash($_.RawData))).Replace('-','') }
        finally { $sha.Dispose() }
        $hash -eq $expected
    })
    if ($matches.Count -gt 1) { exit 3 }
    [Console]::Out.Write(($matches.Count -eq 1).ToString().ToLowerInvariant())
}
finally { $store.Close() }
```

Install script:

```powershell
$WarningPreference = 'SilentlyContinue'
$InformationPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$path = $args[0]
$expected = $args[1].ToUpperInvariant()
$certificate = [Security.Cryptography.X509Certificates.X509Certificate2]::new($path)
$store = [Security.Cryptography.X509Certificates.X509Store]::new(
    [Security.Cryptography.X509Certificates.StoreName]::Root,
    [Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser)
try {
    $store.Open([Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
    $store.Add($certificate)
    $matches = @($store.Certificates | Where-Object {
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $hash = ([BitConverter]::ToString($sha.ComputeHash($_.RawData))).Replace('-','') }
        finally { $sha.Dispose() }
        $hash -eq $expected
    })
    [Console]::Out.Write(($matches.Count -eq 1).ToString().ToLowerInvariant())
}
finally {
    $store.Close()
    $certificate.Dispose()
}
```

Remove script:

```powershell
$WarningPreference = 'SilentlyContinue'
$InformationPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$expected = $args[0].ToUpperInvariant()
$store = [Security.Cryptography.X509Certificates.X509Store]::new(
    [Security.Cryptography.X509Certificates.StoreName]::Root,
    [Security.Cryptography.X509Certificates.StoreLocation]::CurrentUser)
try {
    $store.Open([Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
    $matches = @($store.Certificates | Where-Object {
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $hash = ([BitConverter]::ToString($sha.ComputeHash($_.RawData))).Replace('-','') }
        finally { $sha.Dispose() }
        $hash -eq $expected
    })
    if ($matches.Count -eq 0) { [Console]::Out.Write('true'); exit 0 }
    if ($matches.Count -ne 1) { [Console]::Out.Write('false'); exit 0 }
    $store.Remove($matches[0])
    $remaining = @($store.Certificates | Where-Object {
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $hash = ([BitConverter]::ToString($sha.ComputeHash($_.RawData))).Replace('-','') }
        finally { $sha.Dispose() }
        $hash -eq $expected
    })
    [Console]::Out.Write(($remaining.Count -eq 0).ToString().ToLowerInvariant())
}
finally { $store.Close() }
```

- [ ] **Step 4: Require a separate zero-match postcheck in Go**

Replace `RemoveBySHA256` with:

```go
func (s *windowsCertificateStore) RemoveBySHA256(ctx context.Context, fingerprint string) error {
	fingerprint, err := normalizeFingerprint(fingerprint)
	if err != nil {
		return err
	}
	removed, err := s.runBoolean(ctx, certificateRemoveScript, fingerprint)
	if err != nil || !removed {
		return errors.New("remove CurrentUser root certificate")
	}
	present, err := s.ContainsSHA256(ctx, fingerprint)
	if err != nil || present {
		return errors.New("verify CurrentUser root certificate removal")
	}
	return nil
}
```

The match script's non-zero exit for multiple matches makes `present == false` mean exactly zero, not “zero or many.”

- [ ] **Step 5: Run all certificate tests**

Run:

```powershell
go test ./internal/poc -run 'TestCertificate|TestGeneratedCertificate' -count=1
```

Expected: PASS. The X509Certificate2 integration test is read-only and must not open a real certificate store.

- [ ] **Step 6: Commit the X509Store adapter**

```powershell
git add -- internal/poc/certstore_windows.go internal/poc/certstore_windows_test.go
git commit -m "fix: use CurrentUser X509Store for job CA"
```

### Task 2: Define the closed smoke receipt and error schema

**Files:**
- Create: `internal/poc/certificate_smoke_model.go`
- Create: `internal/poc/certificate_smoke_model_test.go`
- Modify: `internal/poc/store.go`
- Modify: `internal/poc/store_test.go`

- [ ] **Step 1: Write failing receipt and whitelist tests**

Create `internal/poc/certificate_smoke_model_test.go`:

```go
package poc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertificateSmokeErrorCodeIsClosed(t *testing.T) {
	for _, code := range []certificateSmokeErrorCode{
		smokePreflightFailed, smokeApprovalRejected, smokeCertificatePreexisting,
		smokeInstallFailed, smokeInstallVerificationFailed, smokeRemoveFailed,
		smokeRemoveVerificationFailed, smokeSecretsCleanupFailed,
	} {
		if got, ok := safeCertificateSmokeErrorCode(newCertificateSmokeError(code)); !ok || got != string(code) {
			t.Fatalf("code=%q got=%q ok=%v", code, got, ok)
		}
	}
	if _, ok := safeCertificateSmokeErrorCode(errors.New(`secret C:\fixture token XYZ`)); ok {
		t.Fatal("untrusted error was accepted")
	}
}

func TestCertificateSmokeReceiptContainsOnlySafeFields(t *testing.T) {
	code := string(smokeInstallFailed)
	receipt := CertificateSmokeReceipt{
		SchemaVersion: CertificateSmokeSchemaVersion,
		JobID: "poc-20260729T010203-a1b2c3d4e5f6",
		ErrorCode: &code,
		CompletedAt: time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC),
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := ScanOrdinaryOutput(raw); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"certificate_sha256", "certificate_path", "private_key", "raw_error"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("receipt contains forbidden field %q", forbidden)
		}
	}
}

func TestStoreWritesCertificateSmokeReceipt(t *testing.T) {
	store := newTestStore(t, "certificate-smoke-store")
	receipt := CertificateSmokeReceipt{SchemaVersion: CertificateSmokeSchemaVersion, JobID: "certificate-smoke-store"}
	if err := store.WriteCertificateSmokeReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(store.JobDir(), "certificate-smoke-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded CertificateSmokeReceipt
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.JobID != receipt.JobID {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
}
```

- [ ] **Step 2: Run the model tests and verify the missing types fail**

Run:

```powershell
go test ./internal/poc -run 'TestCertificateSmokeErrorCodeIsClosed|TestCertificateSmokeReceiptContainsOnlySafeFields|TestStoreWritesCertificateSmokeReceipt' -count=1
```

Expected: FAIL because the smoke types and store writer do not exist.

- [ ] **Step 3: Implement the closed model**

Create `internal/poc/certificate_smoke_model.go`:

```go
package poc

import (
	"errors"
	"time"
)

const CertificateSmokeSchemaVersion = "wx-channel-comment-poc/certificate-smoke-1"

type certificateSmokeErrorCode string

const (
	smokePreflightFailed            certificateSmokeErrorCode = "smoke_preflight_failed"
	smokeApprovalRejected           certificateSmokeErrorCode = "smoke_approval_rejected"
	smokeCertificatePreexisting     certificateSmokeErrorCode = "smoke_certificate_preexisting"
	smokeInstallFailed              certificateSmokeErrorCode = "smoke_install_failed"
	smokeInstallVerificationFailed  certificateSmokeErrorCode = "smoke_install_verification_failed"
	smokeRemoveFailed               certificateSmokeErrorCode = "smoke_remove_failed"
	smokeRemoveVerificationFailed   certificateSmokeErrorCode = "smoke_remove_verification_failed"
	smokeSecretsCleanupFailed       certificateSmokeErrorCode = "smoke_secrets_cleanup_failed"
)

type certificateSmokeError struct{ code certificateSmokeErrorCode }

func (e certificateSmokeError) Error() string { return string(e.code) }

func newCertificateSmokeError(code certificateSmokeErrorCode) error {
	if _, ok := safeCertificateSmokeCode(code); !ok {
		return errors.New("certificate smoke failure")
	}
	return certificateSmokeError{code: code}
}

func safeCertificateSmokeCode(code certificateSmokeErrorCode) (string, bool) {
	switch code {
	case smokePreflightFailed, smokeApprovalRejected, smokeCertificatePreexisting,
		smokeInstallFailed, smokeInstallVerificationFailed, smokeRemoveFailed,
		smokeRemoveVerificationFailed, smokeSecretsCleanupFailed:
		return string(code), true
	default:
		return "", false
	}
}

func safeCertificateSmokeErrorCode(err error) (string, bool) {
	var target certificateSmokeError
	if !errors.As(err, &target) {
		return "", false
	}
	return safeCertificateSmokeCode(target.code)
}

type CertificateSmokeReceipt struct {
	SchemaVersion         string    `json:"schema_version"`
	JobID                 string    `json:"job_id"`
	Success               bool      `json:"success"`
	PreflightPassed       bool      `json:"preflight_passed"`
	NotElevated           bool      `json:"not_elevated"`
	PreinstallAbsent      bool      `json:"preinstall_absent"`
	InstallVerified       bool      `json:"install_verified"`
	RemoveVerified        bool      `json:"remove_verified"`
	SecretsDestroyed      bool      `json:"secrets_destroyed"`
	RuntimeStateDestroyed bool      `json:"runtime_state_destroyed"`
	ErrorCode             *string   `json:"error_code"`
	CompletedAt           time.Time `json:"completed_at"`
}
```

- [ ] **Step 4: Add the exact store writer**

Add to `internal/poc/store.go`:

```go
func (s *Store) WriteCertificateSmokeReceipt(value CertificateSmokeReceipt) error {
	return s.WriteJSON("certificate-smoke-receipt.json", value)
}
```

- [ ] **Step 5: Run the model/store tests**

Run:

```powershell
go test ./internal/poc -run 'TestCertificateSmokeErrorCodeIsClosed|TestCertificateSmokeReceiptContainsOnlySafeFields|TestStoreWritesCertificateSmokeReceipt' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the smoke schema**

```powershell
git add -- internal/poc/certificate_smoke_model.go internal/poc/certificate_smoke_model_test.go internal/poc/store.go internal/poc/store_test.go
git commit -m "feat: add safe certificate smoke receipt"
```

### Task 3: Implement the certificate-only state machine with crash ownership

**Files:**
- Create: `internal/poc/certificate_smoke.go`
- Create: `internal/poc/certificate_smoke_test.go`

- [ ] **Step 1: Write the success-path test with an exact event order**

Create fakes local to `internal/poc/certificate_smoke_test.go`: an in-memory `CertificateStore` whose `present` flag changes on install/remove, a `smokeEvents` recorder, and dependency functions that use `newTestStore`. Then add:

```go
func TestCertificateSmokeInstallsVerifiesRemovesAndDestroysState(t *testing.T) {
	deps, events, store := fakeCertificateSmokeDeps(t)
	receipt := NewCertificateSmoke(deps).Run(context.Background(), approvedTestOptions())

	if !receipt.Success || !receipt.PreflightPassed || !receipt.NotElevated ||
		!receipt.PreinstallAbsent || !receipt.InstallVerified ||
		!receipt.RemoveVerified || !receipt.SecretsDestroyed ||
		!receipt.RuntimeStateDestroyed || receipt.ErrorCode != nil {
		t.Fatalf("receipt=%+v", receipt)
	}
	want := []string{
		"preflight", "not-elevated", "create-store", "create-ca", "write-ca",
		"write-runtime-unowned", "approve", "preinstall-check",
		"write-runtime-owned", "install", "install-check",
		"remove", "remove-check", "write-runtime-unowned",
		"destroy-secrets", "remove-runtime", "write-receipt",
	}
	if diff := compareStrings(events.snapshot(), want); diff != "" {
		t.Fatal(diff)
	}
	if _, err := os.Stat(filepath.Join(store.RuntimeDir(), "state.json")); !os.IsNotExist(err) {
		t.Fatalf("runtime state remains: %v", err)
	}
}
```

The fake `WriteReceipt` must retain a copy and then call `store.WriteCertificateSmokeReceipt`; it must never retain a raw error string.

- [ ] **Step 2: Add the ownership and cleanup failure tests**

Add these tests before implementation:

```go
func TestCertificateSmokeRejectsElevatedProcessBeforeCreatingCA(t *testing.T)
func TestCertificateSmokeNeverRemovesPreexistingCertificate(t *testing.T)
func TestCertificateSmokeRemovesCertificateAfterAmbiguousInstall(t *testing.T)
func TestCertificateSmokeMapsInstallPostcheckToVerificationFailure(t *testing.T)
func TestCertificateSmokeMapsPersistentRemovalToVerificationFailure(t *testing.T)
func TestCertificateSmokeCleanupFailureOverridesPrimaryFailure(t *testing.T)
func TestCertificateSmokeLeavesRuntimeStateWhenCleanupIsUncertain(t *testing.T)
func TestCertificateSmokeCleanupIsIdempotent(t *testing.T)
func TestCertificateSmokeReceiptNeverContainsDependencyErrorText(t *testing.T)
```

Required assertions:

| Scenario | Certificate removal allowed? | Final code | Runtime state |
|---|---:|---|---|
| elevated process | no | `smoke_preflight_failed` | never created |
| preflight failed | no | `smoke_preflight_failed` | never created |
| confirmation rejected | no certificate removal; secrets only | `smoke_approval_rejected` | removed after secrets cleanup |
| fingerprint existed before install | no | `smoke_certificate_preexisting` | removed after secrets cleanup |
| install returns an error and the exact fingerprint is now present | yes | `smoke_install_failed` unless cleanup fails | removed after successful cleanup |
| install returns success but exact postcheck is false/error | yes | `smoke_install_verification_failed` unless cleanup fails | removed only when exact-one/zero protocol succeeds |
| remove call fails | already attempted | `smoke_remove_failed` | retained for recovery |
| removal returns but exact fingerprint remains or cannot be counted | no second broad removal | `smoke_remove_verification_failed` | retained for recovery |
| secret deletion or runtime-state deletion fails | according to ownership | `smoke_secrets_cleanup_failed`, unless a certificate cleanup code is already more severe | retained for recovery |

Use an error fixture containing a fake absolute path, token, and URL; marshal the final receipt and pass it through `ScanOrdinaryOutput` to prove none of that text escapes.

- [ ] **Step 3: Run the state-machine tests and verify they fail**

Run:

```powershell
go test ./internal/poc -run '^TestCertificateSmoke' -count=1
```

Expected: FAIL because `CertificateSmokeDeps`, `NewCertificateSmoke`, and `Run` do not exist.

- [ ] **Step 4: Define the narrow dependency boundary**

Create `internal/poc/certificate_smoke.go` with these exact public/internal shapes:

```go
package poc

import (
	"context"
	"errors"
	"time"
)

type CertificateSmokePlan struct {
	CertificateScope string
}

type CertificateSmokeDeps struct {
	Preflight      func(context.Context, Options) (PreflightReport, error)
	IsElevated     func() (bool, error)
	Approve        func(context.Context, CertificateSmokePlan) error
	CertStore      CertificateStore
	StoreFactory   func(Options, string) (*Store, error)
	CreateCA       func(string) (*JobCA, error)
	WriteCA        func(*JobCA, *Store) (string, string, error)
	DestroySecrets func(*JobCA, *Store) error
	WriteReceipt   func(*Store, CertificateSmokeReceipt) error
	NewJobID       func() (string, error)
	Now            func() time.Time
}

type CertificateSmoke struct{ deps CertificateSmokeDeps }

func NewCertificateSmoke(deps CertificateSmokeDeps) *CertificateSmoke {
	if deps.NewJobID == nil { deps.NewJobID = newRuntimeJobID }
	if deps.Now == nil { deps.Now = func() time.Time { return time.Now().UTC() } }
	if deps.CreateCA == nil {
		deps.CreateCA = func(jobID string) (*JobCA, error) { return GenerateJobCA(jobID, deps.Now()) }
	}
	if deps.WriteCA == nil {
		deps.WriteCA = func(ca *JobCA, store *Store) (string, string, error) {
			return ca.WriteSecrets(store.SecretsDir())
		}
	}
	if deps.DestroySecrets == nil { deps.DestroySecrets = destroyRuntimeSecrets }
	if deps.WriteReceipt == nil {
		deps.WriteReceipt = func(store *Store, receipt CertificateSmokeReceipt) error {
			return store.WriteCertificateSmokeReceipt(receipt)
		}
	}
	return &CertificateSmoke{deps: deps}
}
```

Do not add `Bridge`, `RuntimeProxy`, `SunnyNet`, driver, WeChat process, collector, token, raw-evidence, or network-listener fields.

- [ ] **Step 5: Implement the ordered run and one deferred cleanup path**

Implement `func (s *CertificateSmoke) Run(ctx context.Context, options Options) CertificateSmokeReceipt` with this exact control flow:

1. Start a receipt with `SchemaVersion` and `CompletedAt`; reject nil/missing dependencies or a failed `options.ValidateForRun()` as `smoke_preflight_failed`.
2. Call `Preflight`; set `PreflightPassed` only for `err == nil && report.Passed`.
3. Call `IsElevated`; set `NotElevated` only for `err == nil && !elevated`; otherwise stop before job ID or CA creation.
4. Create and validate a job ID, create a `Store`, and generate the CA in memory.
5. Install one deferred finalizer immediately after the `Store` and CA exist. It is the only place allowed to remove the certificate, destroy secrets, remove runtime state, finalize `CompletedAt`, derive `Success`, and write the receipt. This must be active before any secret file is written.
6. Write the CA's DER/key into the restricted secrets directory, then immediately persist an unowned `PersistedRuntimeState{SchemaVersion: SchemaVersion, JobID: jobID, CertificateSHA256: ca.SHA256Fingerprint}`. This allows generic crash cleanup to destroy secret files without assuming a certificate exists.
7. Call `Approve` with only `CertificateSmokePlan{CertificateScope: "CurrentUser\\Root"}`.
8. Call `ContainsSHA256`. If it reports true, use `smoke_certificate_preexisting` and never claim ownership or remove it. If it errors, use `smoke_preflight_failed`.
9. Set `PreinstallAbsent = true`. Before calling `Install`, persist the same runtime state with `CertificateInstalled = true`. This is the ownership/crash-cleanup claim; it is safe only because the exact fingerprint was just proven absent.
10. Call `Install`. Map internal `certificateImportReportedFalse` and `certificatePostcheckFailed` outcomes to `smoke_install_verification_failed`; map every other install failure to `smoke_install_failed`. These legacy internal names are not emitted by the smoke receipt.
11. Independently call `ContainsSHA256` again. Only exact-one sets `InstallVerified = true`; false/error maps to `smoke_install_verification_failed`.

The deferred finalizer must perform these exact steps:

1. If and only if ownership was claimed, call `RemoveBySHA256` once. Map `certificateRemoveReportedFalse` (zero is already idempotent success, so false means an unsafe match count) and `certificateRemovePostcheckFailed` to `smoke_remove_verification_failed`; map `certificateRemoveCommandFailed` to `smoke_remove_failed`.
2. If removal returned nil, call `ContainsSHA256` independently. Only exact-zero sets `RemoveVerified = true`. A true value or error maps to `smoke_remove_verification_failed`.
3. After exact-zero, persist `CertificateInstalled = false`. A persistence failure is `smoke_secrets_cleanup_failed` and leaves the previous owned state available for recovery.
4. Always call `DestroySecrets` after CA creation. Set `SecretsDestroyed` only on success.
5. Call `RemoveRuntimeState` only when secrets are destroyed and either ownership was never claimed or the owned certificate is proven absent. Set `RuntimeStateDestroyed` only on success. A preexisting unowned certificate must never block deletion of this job's unowned recovery record.
6. Select one deterministic safe error code. Certificate cleanup codes take priority in this order: verification, remove. Then use secrets/runtime cleanup, then the primary operation code. This ensures uncertain system state is never hidden by an earlier install failure.
7. Set `Success` only when all seven prerequisite booleans are true and `ErrorCode == nil`.
8. Write the receipt last. If writing fails, change a previously successful/primary-only result to `smoke_secrets_cleanup_failed`, set `Success = false`, and make a best-effort second write. Never include the write error text.

Do not use `errors.Join(...).Error()` or any dependency error text to populate `ErrorCode` or another receipt field. Keep raw errors only in local control-flow variables.

- [ ] **Step 6: Add typed removal outcomes to the Windows adapter**

Modify `internal/poc/certificate_error.go` and `internal/poc/certstore_windows.go` so the state machine can distinguish command/removal failure from post-removal verification failure without exposing raw text:

```go
const (
	// existing codes remain for compatibility
	certificateRemoveCommandFailed certificateErrorCode = "certificate_remove_command_failed"
	certificateRemoveReportedFalse certificateErrorCode = "certificate_remove_reported_false"
	certificateRemovePostcheckFailed certificateErrorCode = "certificate_remove_postcheck_failed"
)
```

`RemoveBySHA256` must map script execution failure, a literal `false`, and the separate `ContainsSHA256` zero-match postcheck to those three codes respectively. Extend `newCertificateStoreError` and `safeCertificateErrorCode` with the same closed values. In `certificate_smoke.go`, use `errors.As` against `certificateStoreError` only for internal mapping; never serialize the underlying error.

- [ ] **Step 7: Run all state-machine and runtime cleanup tests**

Run:

```powershell
go test ./internal/poc -run 'TestCertificateSmoke|TestRuntimeCleans|TestCleanup' -count=1
```

Expected: PASS. Existing normal-runtime cleanup semantics remain intact.

- [ ] **Step 8: Commit the independent smoke state machine**

```powershell
git add -- internal/poc/certificate_smoke.go internal/poc/certificate_smoke_test.go internal/poc/certificate_error.go internal/poc/certstore_windows.go internal/poc/certstore_windows_test.go
git commit -m "feat: add certificate-only smoke lifecycle"
```

### Task 4: Add the non-admin Windows command and exact approval gate

**Files:**
- Create: `internal/poc/certificate_smoke_windows.go`
- Create: `internal/poc/certificate_smoke_windows_test.go`
- Modify: `internal/poc/runtime_other.go`
- Modify: `internal/poc/app.go`
- Modify: `internal/poc/app_test.go`
- Modify: `cmd/wx_channel_poc/main.go`

- [ ] **Step 1: Write failing CLI argument and approval tests**

Add to `internal/poc/app_test.go`:

```go
func TestCertificateSmokeCLIRequiresOnlyIsolatedVMAcknowledgement(t *testing.T) {
	for _, args := range [][]string{{}, {"--unknown"}, {"--allow-encrypted-raw"}, {"--ack-isolated-vm", "extra"}} {
		var output bytes.Buffer
		if code := RunCertificateSmokeCLI(context.Background(), strings.NewReader("CERT_APPLY\n"), &output, args, DefaultOptions()); code != 2 {
			t.Fatalf("args=%v code=%d output=%q", args, code, output.String())
		}
	}
}

func TestCertificateSmokeApprovalRequiresExactText(t *testing.T) {
	for _, input := range []string{"", "cert_apply\n", "CERT_APPLY \n", "APPLY\n"} {
		var output bytes.Buffer
		approve := newCertificateSmokeApproval(strings.NewReader(input), &output)
		if err := approve(context.Background(), CertificateSmokePlan{CertificateScope: "CurrentUser\\Root"}); err == nil {
			t.Fatalf("input=%q was accepted", input)
		}
	}
	approve := newCertificateSmokeApproval(strings.NewReader("CERT_APPLY\n"), io.Discard)
	if err := approve(context.Background(), CertificateSmokePlan{CertificateScope: "CurrentUser\\Root"}); err != nil {
		t.Fatal(err)
	}
}

func TestCertificateSmokeApprovalPrintsOnlyCurrentUserPlan(t *testing.T) {
	var output bytes.Buffer
	approve := newCertificateSmokeApproval(strings.NewReader("wrong\n"), &output)
	_ = approve(context.Background(), CertificateSmokePlan{CertificateScope: "CurrentUser\\Root"})
	if got := output.String(); got != "Planned certificate smoke change: certificate=CurrentUser\\Root\nType CERT_APPLY to continue: " {
		t.Fatalf("prompt=%q", got)
	}
}
```

- [ ] **Step 2: Run the app tests and verify they fail**

Run:

```powershell
go test ./internal/poc -run 'TestCertificateSmokeCLI|TestCertificateSmokeApproval' -count=1
```

Expected: FAIL because the CLI and approval constructor do not exist.

- [ ] **Step 3: Implement the exact common CLI contract**

Add `newCertificateSmokeApproval` and `RunCertificateSmokeCLI` to `internal/poc/app.go`:

```go
func newCertificateSmokeApproval(input io.Reader, output io.Writer) func(context.Context, CertificateSmokePlan) error {
	return func(ctx context.Context, plan CertificateSmokePlan) error {
		if plan.CertificateScope != "CurrentUser\\Root" { return newCertificateSmokeError(smokeApprovalRejected) }
		_, _ = fmt.Fprintf(output, "Planned certificate smoke change: certificate=%s\nType CERT_APPLY to continue: ", plan.CertificateScope)
		scanner := bufio.NewScanner(input)
		line := make(chan string, 1)
		go func() { if scanner.Scan() { line <- scanner.Text() }; close(line) }()
		select {
		case <-ctx.Done(): return newCertificateSmokeError(smokeApprovalRejected)
		case value, ok := <-line:
			if !ok || value != "CERT_APPLY" || scanner.Err() != nil { return newCertificateSmokeError(smokeApprovalRejected) }
		}
		return nil
	}
}

func RunCertificateSmokeCLI(ctx context.Context, input io.Reader, output io.Writer, args []string, options Options) int {
	flags := flag.NewFlagSet("cert-smoke", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ack := flags.Bool("ack-isolated-vm", false, "acknowledge disposable isolated VM")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || !*ack {
		_, _ = fmt.Fprintln(output, "cert-smoke requires --ack-isolated-vm")
		return 2
	}
	options.AckIsolatedVM = true
	options.AllowEncryptedRaw = false
	receipt := runPlatformCertificateSmoke(ctx, input, output, options)
	_ = json.NewEncoder(output).Encode(receipt)
	if receipt.Success { return 0 }
	return 1
}
```

The CLI must not print `err.Error()`. Its only failure detail is the closed `error_code` inside the receipt.

- [ ] **Step 4: Wire a Windows-only non-elevation gate**

Create `internal/poc/certificate_smoke_windows.go`:

```go
//go:build windows

package poc

import (
	"context"
	"errors"
	"io"
	"time"

	"golang.org/x/sys/windows"
)

func currentProcessIsElevated() (bool, error) {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated(), nil
}

func runPlatformCertificateSmoke(ctx context.Context, input io.Reader, output io.Writer, options Options) CertificateSmokeReceipt {
	runner := ExecCommandRunner{}
	deps := CertificateSmokeDeps{
		Preflight: func(ctx context.Context, options Options) (PreflightReport, error) {
			return NewPreflight(runner, NewWindowsVMDetector(runner)).Run(ctx, options)
		},
		IsElevated: currentProcessIsElevated,
		Approve: newCertificateSmokeApproval(input, output),
		CertStore: newCertificateStore(runner),
		StoreFactory: func(options Options, jobID string) (*Store, error) {
			repoRoot, err := existingCanonicalDirectory(".")
			if err != nil { return nil, errors.New("run POC from repository root") }
			return NewStore(StoreOptions{
				RepoRoot: repoRoot, DataRoot: options.DataRoot, SecretsRoot: options.SecretsRoot,
				RuntimeRoot: options.RuntimeRoot, BuildRoot: options.BuildRoot, VarRoot: "var", JobID: jobID,
			})
		},
		CreateCA: func(jobID string) (*JobCA, error) { return GenerateJobCA(jobID, time.Now().UTC()) },
	}
	return NewCertificateSmoke(deps).Run(ctx, options)
}
```

Do not call `OpenProcessToken` with requested elevation or use a privilege-changing API. `Token.IsElevated()` is read-only.

- [ ] **Step 5: Add the unsupported-platform stub and command dispatch**

Add to `internal/poc/runtime_other.go`:

```go
func runPlatformCertificateSmoke(context.Context, io.Reader, io.Writer, Options) CertificateSmokeReceipt {
	code := string(smokePreflightFailed)
	return CertificateSmokeReceipt{SchemaVersion: CertificateSmokeSchemaVersion, ErrorCode: &code, CompletedAt: time.Now().UTC()}
}
```

Add the `time` import there. Modify `cmd/wx_channel_poc/main.go` so usage is exactly:

```text
usage: wx_channel_poc <preflight|cert-smoke|run|cleanup>
```

and dispatch:

```go
case "cert-smoke":
	os.Exit(poc.RunCertificateSmokeCLI(ctx, os.Stdin, os.Stdout, os.Args[2:], options))
```

- [ ] **Step 6: Add Windows gate tests without touching a certificate store**

In `internal/poc/certificate_smoke_windows_test.go`, test that `currentProcessIsElevated()` returns without an error and agrees with `windows.GetCurrentProcessToken().IsElevated()`. This test is read-only. The state-machine elevated fake remains the authoritative refusal test; do not make a host test call `runPlatformCertificateSmoke`.

- [ ] **Step 7: Run CLI, Windows, and command-package tests**

Run:

```powershell
go test ./internal/poc ./cmd/wx_channel_poc -run 'TestCertificateSmoke|TestCurrentProcess' -count=1
```

Expected: PASS without creating a CA or modifying a certificate store.

- [ ] **Step 8: Commit the separate command**

```powershell
git add -- internal/poc/certificate_smoke_windows.go internal/poc/certificate_smoke_windows_test.go internal/poc/runtime_other.go internal/poc/app.go internal/poc/app_test.go cmd/wx_channel_poc/main.go
git commit -m "feat: add non-admin certificate smoke command"
```

### Task 5: Enforce the dependency boundary and document the one-shot procedure

**Files:**
- Modify: `internal/pocaudit/source_test.go`
- Create: `scripts/run-certificate-smoke.cmd`
- Modify: `scripts/poc_scripts_test.go`
- Modify: `docs/runbooks/wechat-channel-comment-poc.md`

- [ ] **Step 1: Add source-level boundary tests**

Add to `internal/pocaudit/source_test.go` a helper that reads a repository-relative source file, then add:

```go
func TestCertificateStoreUsesOnlyCurrentUserX509Store(t *testing.T) {
	source := strings.ToLower(readRepoFile(t, "internal/poc/certstore_windows.go"))
	for _, required := range []string{"x509store", "storename]::root", "storelocation]::currentuser", ".add(", ".remove(", "sha256"} {
		if !strings.Contains(source, required) { t.Errorf("missing %q", required) }
	}
	for _, forbidden := range []string{"import-certificate", "certutil", "localmachine"} {
		if strings.Contains(source, forbidden) { t.Errorf("forbidden certificate fallback %q", forbidden) }
	}
}

func TestCertificateSmokeSourceHasNoCollectionRuntimeDependencies(t *testing.T) {
	production := strings.ToLower(readRepoFile(t, "internal/poc/certificate_smoke.go") + readRepoFile(t, "internal/poc/certificate_smoke_windows.go"))
	for _, forbidden := range []string{"bridgestart", "runtimeproxy", "proxyfactory", "sunnynet", "nfapi", "wechatappex", "collectrestrictedpoc", "createtoken"} {
		if strings.Contains(production, forbidden) { t.Errorf("certificate smoke references %q", forbidden) }
	}
}
```

These tests inspect production files only, so their own forbidden fixture strings do not cause false positives.

- [ ] **Step 2: Add and test a smoke-only launcher**

Create `scripts/run-certificate-smoke.cmd` with exactly:

```bat
@echo off
setlocal
title WeChat Comment POC - Certificate Smoke Only
cd /d "%~dp0source"
".poc-build\wx_channel_poc.exe" cert-smoke --ack-isolated-vm
set "POC_EXIT=%ERRORLEVEL%"
echo Exit code: %POC_EXIT%
pause
exit /b %POC_EXIT%
```

Extend `scripts/poc_scripts_test.go` to require `cert-smoke --ack-isolated-vm` and forbid ` run `, `APPLY`, `SunnyNet`, `WeChatAppEx`, proxy addresses, bridge addresses, and direct certificate commands. The wrapper must not self-supply `CERT_APPLY`; the operator types it interactively.

- [ ] **Step 3: Update the runbook with a new gate before real collection**

Insert a new section before the existing real-run checkpoint with these statements:

- `cert-smoke` runs only from a fresh `wechat-login-baseline` VM and must be launched as the ordinary logged-in user, never with “Run as administrator.”
- It does not open WeChat and does not exercise search/comments.
- The operator must type exact `CERT_APPLY`.
- Success requires every receipt boolean to be true, `error_code` to be null, and pre/post preflight baseline hashes to match.
- After any outcome, shut down and restore `wechat-login-baseline`.
- A successful smoke test still does not authorize `run`; a new explicit user confirmation is required.

Move the existing real-run section after this new gate and label it “not authorized by certificate smoke.”

- [ ] **Step 4: Run the focused security checks**

Run:

```powershell
go test ./internal/pocaudit ./scripts -count=1
```

Expected: PASS, including no forbidden fallback or smoke dependency.

- [ ] **Step 5: Run the complete host-side verification**

Run:

```powershell
$toolBin=(Resolve-Path '.\.poc-tools\tdm-gcc-10.3.0-2\bin').Path
$env:CGO_ENABLED='1'
$env:PATH="$toolBin;$env:PATH"
$env:CC=Join-Path $toolBin 'gcc.exe'
$env:CXX=Join-Path $toolBin 'g++.exe'
go test ./... -count=1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\build-poc.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\poc-security-audit.ps1
```

Expected: every Go package passes, the restricted binary is rebuilt from source, the audit prints `POC security audit passed`, and the binary receipt contains a SHA-256. Do not execute `cert-smoke` on the host.

- [ ] **Step 6: Commit the boundary and operator procedure**

```powershell
git add -- internal/pocaudit/source_test.go scripts/poc_scripts_test.go scripts/run-certificate-smoke.cmd docs/runbooks/wechat-channel-comment-poc.md
git commit -m "docs: add certificate smoke safety gate"
```

### Task 6: Produce exact-commit read-only media and run the VM smoke once

**Files:**
- Create outside Git: `D:\Agent\vm-poc-downloads\poc-runtime-media-$shortCommit\1_PREPARE_BUILD_AUDIT.cmd`
- Create outside Git: `D:\Agent\vm-poc-downloads\poc-runtime-media-$shortCommit\2_RUN_PREFLIGHT.cmd`
- Create outside Git: `D:\Agent\vm-poc-downloads\poc-runtime-media-$shortCommit\prepare-build-audit.ps1`
- Create outside Git: `D:\Agent\vm-poc-downloads\poc-runtime-media-$shortCommit\README.txt`
- Create outside Git: `D:\Agent\vm-poc-downloads\poc-runtime-media-$shortCommit\5_RUN_CERT_SMOKE.cmd`
- Create outside Git: `D:\Agent\vm-poc-downloads\poc-runtime-media-$shortCommit\media-manifest.json`
- Create outside Git: `D:\Agent\vm-poc-downloads\wechat-channel-poc-source-$shortCommit.iso`
- Read in VM only: the latest job directory's `certificate-smoke-receipt.json` below `C:\Users\liuhan\AppData\Local\wechat-channel-comment-poc\source\.poc-data`

- [ ] **Step 1: Record the exact clean source commit**

From the implementation worktree, run:

```powershell
git status --short
$sourceCommit=(git rev-parse HEAD).Trim()
$shortCommit=(git rev-parse --short=7 HEAD).Trim()
git show --stat --oneline --decorate $sourceCommit
```

Expected: clean status and the smoke implementation/docs commits visible. Stop if there are uncommitted files.

- [ ] **Step 2: Build reproducible VM media without a repository executable**

Copy the non-secret tool archives from `poc-runtime-media-25a585a`, create a full exact-commit Git bundle, and copy the reviewed smoke wrapper:

```powershell
$mediaRoot="D:\Agent\vm-poc-downloads\poc-runtime-media-$shortCommit"
New-Item -ItemType Directory -Path $mediaRoot | Out-Null
Copy-Item -LiteralPath 'D:\Agent\vm-poc-downloads\poc-runtime-media-25a585a\tools' -Destination $mediaRoot -Recurse
git bundle create (Join-Path $mediaRoot 'wx_channel.bundle') refs/heads/codex/wechat-channel-comment-poc
git bundle verify (Join-Path $mediaRoot 'wx_channel.bundle')
if ((git rev-parse refs/heads/codex/wechat-channel-comment-poc).Trim() -ne $sourceCommit) { throw 'Branch moved while creating media.' }
Copy-Item -LiteralPath 'scripts\run-certificate-smoke.cmd' -Destination (Join-Path $mediaRoot '5_RUN_CERT_SMOKE.cmd')
```

Reuse the reviewed `1_PREPARE_BUILD_AUDIT.cmd`, `2_RUN_PREFLIGHT.cmd`, and preparation script only after changing their expected source commit to `$sourceCommit` and their expected bundle hash to the newly computed SHA-256. Create `README.txt` with the only allowed order `1_PREPARE_BUILD_AUDIT.cmd`, `2_RUN_PREFLIGHT.cmd`, `5_RUN_CERT_SMOKE.cmd`, `2_RUN_PREFLIGHT.cmd`, shutdown, rollback. State explicitly that smoke success does not authorize `run`. Do not include `3_OPEN_POC_SHELL.cmd`, `4_UPDATE_PREFLIGHT_FIX.cmd`, updater scripts, or any launcher for `run`. Apply those text edits with `apply_patch`, not shell redirection.

- [ ] **Step 3: Generate and verify the media manifest**

After all helper files are patched and the bundle hash in `prepare-build-audit.ps1` is current, generate the manifest mechanically:

```powershell
$entries = Get-ChildItem -LiteralPath $mediaRoot -File -Recurse |
  Where-Object Name -ne 'media-manifest.json' |
  Sort-Object FullName |
  ForEach-Object {
    $relative=$_.FullName.Substring($mediaRoot.Length).TrimStart('\').Replace('\','/')
    [ordered]@{path=$relative; size=$_.Length; sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash}
  }
[ordered]@{
  schema_version='wx-channel-comment-poc/media-2'
  source_commit=$sourceCommit
  branch='codex/wechat-channel-comment-poc'
  files=@($entries)
} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $mediaRoot 'media-manifest.json') -Encoding UTF8
```

The manifest must record normalized relative path, byte length, and SHA-256 and have this decoded shape:

```json
{
  "schema_version": "wx-channel-comment-poc/media-2",
  "source_commit": "$sourceCommit",
  "branch": "codex/wechat-channel-comment-poc",
  "files": []
}
```

Independently re-read it and fail unless the set of files, each size, and each hash exactly match. Confirm there are no files named `wx_channel*.exe`, no `.key`/`.pem` files, and no captured-result or login-state files. The repository bundle is allowed to contain audited source identifiers such as `Token` but not secret values; `TestTrackedSourceContainsNoPrivateKey` remains mandatory.

- [ ] **Step 4: Build, mount, and independently verify the ISO**

Create the ISO with the audited local IMAPI2 helper:

```powershell
$isoPath="D:\Agent\vm-poc-downloads\wechat-channel-poc-source-$shortCommit.iso"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File 'D:\Agent\vm-poc-downloads\tmp\new-poc-iso.ps1' `
  -Source $mediaRoot -Path $isoPath -Title 'POC_SOURCE'
$isoHash=(Get-FileHash -LiteralPath $isoPath -Algorithm SHA256).Hash
$mounted=Mount-DiskImage -ImagePath $isoPath -PassThru
try {
  $volume=$mounted | Get-Volume
  if ($volume.FileSystemLabel -ne 'POC_SOURCE') { throw 'Unexpected ISO label.' }
  $mountedRoot=$volume.DriveLetter + ':\'
  $mountedManifest=Get-Content -LiteralPath (Join-Path $mountedRoot 'media-manifest.json') -Raw | ConvertFrom-Json
  if ($mountedManifest.source_commit -ne $sourceCommit -or $mountedManifest.branch -ne 'codex/wechat-channel-comment-poc') { throw 'ISO source identity mismatch.' }
  $expectedPaths=@($mountedManifest.files.path | Sort-Object)
  $actualPaths=@(Get-ChildItem -LiteralPath $mountedRoot -File -Recurse |
    Where-Object Name -ne 'media-manifest.json' |
    ForEach-Object { $_.FullName.Substring($mountedRoot.Length).Replace('\','/') } |
    Sort-Object)
  if (Compare-Object $expectedPaths $actualPaths) { throw 'ISO file set mismatch.' }
  foreach ($entry in $mountedManifest.files) {
    $path=Join-Path $mountedRoot ($entry.path.Replace('/','\'))
    $item=Get-Item -LiteralPath $path
    if ($item.Length -ne $entry.size) { throw "ISO size mismatch: $($entry.path)" }
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $entry.sha256) { throw "ISO hash mismatch: $($entry.path)" }
  }
}
finally { Dismount-DiskImage -ImagePath $isoPath }
```

Record only:

- ISO path and SHA-256;
- exact source commit;
- bundle SHA-256;
- manifest file count and all-pass result.

Do not paste source data, certificate material, or VM login information into chat.

- [ ] **Step 5: Start only the reverted baseline and rebuild inside the VM**

With the VM powered off, attach the verified ISO. Start `wechat-login-baseline`, sign in as the ordinary local user, and do not open WeChat. Run the preparation/build/audit launcher from the DVD. Require:

- exact source commit equals the manifest;
- source bundle hash and every media hash match;
- `go mod verify` succeeds;
- all POC tests pass;
- `scripts/build-poc.ps1` succeeds;
- `scripts/poc-security-audit.ps1` prints `POC security audit passed`;
- the freshly built binary SHA-256 is recorded without copying the binary out of the VM.

Stop immediately on any mismatch. Never run a tracked or upstream `wx_channel*.exe`.

- [ ] **Step 6: Capture the pre-smoke baseline**

Double-click `2_RUN_PREFLIGHT.cmd` normally. Save only the returned booleans and three baseline hashes. Require:

- `passed`, `isolated_vm`, `git_ignored`, `executable_name_ok`, `loopback_ports_free`, and `sunny_driver_absent` are true;
- `reason_codes` is null;
- dynamic-port, certificate, and listener hashes are non-empty.

If preflight fails, shut down and restore the snapshot. Do not run the smoke command.

- [ ] **Step 7: Execute exactly one certificate-only smoke attempt**

Double-click `5_RUN_CERT_SMOKE.cmd` normally; do not use “Run as administrator.” Verify the displayed plan contains only `certificate=CurrentUser\Root`, then type exact `CERT_APPLY`.

Success requires exit code 0 and a receipt with:

```json
{
  "success": true,
  "preflight_passed": true,
  "not_elevated": true,
  "preinstall_absent": true,
  "install_verified": true,
  "remove_verified": true,
  "secrets_destroyed": true,
  "runtime_state_destroyed": true,
  "error_code": null
}
```

Do not retry a failure in the same VM state. Record only the safe receipt fields and classify the result as `inconclusive` on any failure.

- [ ] **Step 8: Prove restoration, shut down, and force snapshot rollback**

Run `2_RUN_PREFLIGHT.cmd` once more. Every boolean must pass, `reason_codes` must be null, and all three baseline hashes must exactly equal Step 6. Then:

1. shut down Windows normally;
2. in VMware restore `wechat-login-baseline` even if the smoke succeeded;
3. confirm the VM is powered off after restore;
4. retain no CA/key/runtime artifact outside the reverted VM.

If the smoke or postflight failed, restore the snapshot immediately without trying the host.

- [ ] **Step 9: Stop at the real-collection authorization gate**

Report the certificate-smoke outcome only. Do not run `wx_channel_poc.exe run`, do not open WeChat for collection, and do not prepare a real-collection attempt until the user gives a new explicit confirmation after reviewing the smoke evidence.
