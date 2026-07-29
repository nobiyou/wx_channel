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

func TestCertificateScriptsUseCurrentUserX509StoreOnly(t *testing.T) {
	combined := certificateMatchScript + certificateInstallScript + certificateRemoveScript
	lower := strings.ToLower(combined)
	for _, required := range []string{
		"X509Store", "StoreName]::Root", "StoreLocation]::CurrentUser",
		"OpenFlags]::ReadWrite", ".Add(", ".Remove(", "SHA256",
		`$WarningPreference = 'SilentlyContinue'`,
		`$InformationPreference = 'SilentlyContinue'`,
		`$ProgressPreference = 'SilentlyContinue'`,
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

func TestCertificateRemoveMapsCommandFailure(t *testing.T) {
	runner := &certificateRunner{results: []certificateResult{{err: errors.New("fixture")}}}
	err := (&windowsCertificateStore{runner: runner}).RemoveBySHA256(
		context.Background(), strings.Repeat("A", 64),
	)
	assertCertificateErrorCode(t, err, certificateRemoveCommandFailed)
}

func TestCertificateRemoveMapsReportedFalse(t *testing.T) {
	runner := &certificateRunner{results: []certificateResult{{output: []byte("false")}}}
	err := (&windowsCertificateStore{runner: runner}).RemoveBySHA256(
		context.Background(), strings.Repeat("A", 64),
	)
	assertCertificateErrorCode(t, err, certificateRemoveReportedFalse)
}

func TestCertificateRemoveMapsPostcheckFailure(t *testing.T) {
	runner := &certificateRunner{results: []certificateResult{
		{output: []byte("true")},
		{output: []byte("true")},
	}}
	err := (&windowsCertificateStore{runner: runner}).RemoveBySHA256(
		context.Background(), strings.Repeat("A", 64),
	)
	assertCertificateErrorCode(t, err, certificateRemovePostcheckFailed)
}
