package certificate

import (
	"testing"

	"wx_channel/pkg/certgen"
)

func TestCertFingerprint_ValidPEM(t *testing.T) {
	// Generate a real cert via certgen for testing
	dir := t.TempDir()
	certPEM, _, err := certgen.EnsureCA(dir)
	if err != nil {
		t.Fatalf("failed to generate test cert: %v", err)
	}

	fp := certFingerprint(certPEM)
	if fp == "" {
		t.Fatal("certFingerprint returned empty for valid PEM")
	}
	// Should be consistent across calls
	fp2 := certFingerprint(certPEM)
	if fp != fp2 {
		t.Error("certFingerprint is not deterministic")
	}
	t.Logf("fingerprint: %s", fp)
}

func TestCertFingerprint_InvalidPEM(t *testing.T) {
	fp := certFingerprint([]byte("not a pem"))
	if fp != "" {
		t.Errorf("expected empty fingerprint for invalid PEM, got %s", fp)
	}
}

func TestCertFingerprint_EmptyInput(t *testing.T) {
	fp := certFingerprint(nil)
	if fp != "" {
		t.Error("expected empty fingerprint for nil input")
	}
}

func TestRemoveCertificateInMacOS_BothKeychains(t *testing.T) {
	// This test verifies the function signature and structure, not actual keychain ops.
	// Calling with a non-existent cert name should not panic.
	err := removeCertificateInMacOS("__test_nonexistent_cert_12345__")
	// Error is expected (cert doesn't exist), but no panic
	_ = err
}
