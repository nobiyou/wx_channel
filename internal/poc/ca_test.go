package poc

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGenerateJobCAUsesUniqueCNAndSHA256(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	ca, err := GenerateJobCA("job-123", now)
	if err != nil {
		t.Fatal(err)
	}
	if ca.Certificate.Subject.CommonName != "wx-channel-poc-job-123" {
		t.Fatalf("CN=%s", ca.Certificate.Subject.CommonName)
	}
	if ca.SHA256Fingerprint == "" || bytes.Contains(ca.CertPEM, ca.KeyPEM) {
		t.Fatal("invalid CA material")
	}
	if ca.Certificate.PublicKey.(*rsa.PublicKey).N.BitLen() != 3072 {
		t.Fatalf("RSA bits=%d", ca.Certificate.PublicKey.(*rsa.PublicKey).N.BitLen())
	}
	if !ca.Certificate.IsCA || ca.Certificate.KeyUsage != x509.KeyUsageCertSign|x509.KeyUsageCRLSign|x509.KeyUsageDigitalSignature {
		t.Fatalf("invalid CA constraints: %+v", ca.Certificate)
	}
	if got := ca.Certificate.NotAfter.Sub(ca.Certificate.NotBefore); got != 24*time.Hour {
		t.Fatalf("validity=%s", got)
	}
}

func TestJobCAWritesOnlyToMatchingSecretDirectory(t *testing.T) {
	ca, err := GenerateJobCA("job-123", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), ".poc-secrets", "job-123")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	certPath, keyPath, err := ca.WriteSecrets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(certPath) != "job-ca.cert" {
		t.Fatalf("certificate file=%q; Windows Import-Certificate requires a supported certificate format", filepath.Base(certPath))
	}
	certRaw, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(certRaw)
	if err != nil || !bytes.Equal(parsed.Raw, ca.Certificate.Raw) {
		t.Fatalf("certificate file is not the generated DER certificate: err=%v", err)
	}
	for _, path := range []string{certPath, keyPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("permissions=%o", info.Mode().Perm())
		}
	}
	wrong := filepath.Join(t.TempDir(), "job-123")
	if err := os.MkdirAll(wrong, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ca.WriteSecrets(wrong); err == nil {
		t.Fatal("WriteSecrets accepted a directory outside .poc-secrets")
	}
}

func TestJobCANewSunnyIsLoopbackOnly(t *testing.T) {
	ca, err := GenerateJobCA("job-123", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	sunny, err := ca.NewSunny()
	if err != nil {
		t.Fatal(err)
	}
	if got := sunny.ListenHost(); got != "127.0.0.1" {
		t.Fatalf("ListenHost=%q", got)
	}
}

func TestCertificateRemovalVerifiesSHA256BeforeDelete(t *testing.T) {
	runner := &recordingRunner{outputs: [][]byte{[]byte("false")}}
	store := newCertificateStore(runner)
	if err := store.RemoveBySHA256(context.Background(), strings.Repeat("A", 64)); err == nil {
		t.Fatal("expected mismatch")
	}
	if runner.calledRemove {
		t.Fatal("must not remove mismatched certificate")
	}
}

func TestSecretACLUsesParameterizedSID(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows ACL test")
	}
	dir := filepath.Join(t.TempDir(), ".poc-secrets", "job-123")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	if err := restrictSecretDirectory(context.Background(), runner, dir); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || runner.calls[0].name != "icacls.exe" {
		t.Fatalf("calls=%+v", runner.calls)
	}
	call := runner.calls[0]
	if len(call.args) != 4 || call.args[0] != dir || call.args[1] != "/inheritance:r" || call.args[2] != "/grant:r" || !strings.HasPrefix(call.args[3], "*S-") || !strings.HasSuffix(call.args[3], ":(OI)(CI)F") {
		t.Fatalf("unsafe ACL arguments: %#v", call.args)
	}
}
