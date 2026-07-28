//go:build windows

package poc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestCertificateBooleanPassesArgumentsToPowerShellScript(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}

	store := &windowsCertificateStore{runner: ExecCommandRunner{}}
	script := `[Console]::Out.Write((($args -join '|') -eq 'certificate|fingerprint').ToString().ToLowerInvariant())`
	ok, err := store.runBoolean(context.Background(), script, "certificate", "fingerprint")
	if err != nil || !ok {
		t.Fatalf("PowerShell script did not receive the supplied arguments: ok=%v err=%v", ok, err)
	}
}

func TestGeneratedCertificateIsAcceptedByImportCertificateWhatIf(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}

	ca, err := GenerateJobCA("job-import-whatif", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), ".poc-secrets", "job-import-whatif")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	certPath, _, err := ca.WriteSecrets(dir)
	if err != nil {
		t.Fatal(err)
	}

	store := &windowsCertificateStore{runner: ExecCommandRunner{}}
	script := `$null = Import-Certificate -FilePath $args[0] -CertStoreLocation Cert:\CurrentUser\Root -WhatIf -Confirm:$false -InformationAction SilentlyContinue -ErrorAction Stop; [Console]::Out.Write('true')`
	ok, err := store.runBoolean(context.Background(), script, certPath)
	if err != nil || !ok {
		t.Fatalf("generated certificate was not accepted by Import-Certificate -WhatIf: ok=%v err=%v", ok, err)
	}
}
