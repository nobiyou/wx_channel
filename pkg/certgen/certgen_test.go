package certgen

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCA_GeneratesNewKeyPair(t *testing.T) {
	dir := t.TempDir()

	cert, key, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("EnsureCA failed: %v", err)
	}
	if len(cert) == 0 || len(key) == 0 {
		t.Fatal("EnsureCA returned empty cert or key")
	}

	// Verify PEM-encoded cert is valid
	block, _ := pem.Decode(cert)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("cert is not a valid PEM CERTIFICATE block")
	}
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("cert is not valid x509: %v", err)
	}
	if !x509Cert.IsCA {
		t.Error("cert is not a CA certificate")
	}
	if x509Cert.Subject.CommonName != "WxChannel macOS MITM CA" {
		t.Errorf("unexpected CN: %s", x509Cert.Subject.CommonName)
	}

	// Verify PEM-encoded key is valid
	keyBlock, _ := pem.Decode(key)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" {
		t.Fatal("key is not a valid PEM EC PRIVATE KEY block")
	}

	// Verify files are saved to disk
	if _, err := os.Stat(filepath.Join(dir, "mitm_ca.pem")); os.IsNotExist(err) {
		t.Error("mitm_ca.pem not saved to disk")
	}
	if _, err := os.Stat(filepath.Join(dir, "mitm_ca.key")); os.IsNotExist(err) {
		t.Error("mitm_ca.key not saved to disk")
	}
}

func TestEnsureCA_LoadsExistingKeyPair(t *testing.T) {
	dir := t.TempDir()

	// First call generates
	cert1, key1, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("first EnsureCA failed: %v", err)
	}

	// Second call should load same keypair (not regenerate)
	cert2, key2, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("second EnsureCA failed: %v", err)
	}

	if string(cert1) != string(cert2) {
		t.Error("cert changed between calls — should be loaded from disk")
	}
	if string(key1) != string(key2) {
		t.Error("key changed between calls — should be loaded from disk")
	}
}

func TestEnsureCA_KeyFilePermissions(t *testing.T) {
	dir := t.TempDir()

	_, _, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("EnsureCA failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "mitm_ca.key"))
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		t.Errorf("key file permissions too open: %o (want 0600)", perm)
	}
}
