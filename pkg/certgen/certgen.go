// Package certgen generates and caches a self-signed MITM CA certificate + key
// on first run. The keypair is stored in ~/.wx_channel/ so it persists across
// restarts but is never committed to version control.
package certgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	caCertFile = "mitm_ca.pem"
	caKeyFile  = "mitm_ca.key"
)

// EnsureCA returns a PEM-encoded CA cert and key, generating them if they
// don't already exist in dataDir.
func EnsureCA(dataDir string) (certPEM, keyPEM []byte, err error) {
	certPath := filepath.Join(dataDir, caCertFile)
	keyPath := filepath.Join(dataDir, caKeyFile)

	// Try loading existing keypair
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil && len(certPEM) > 0 && len(keyPEM) > 0 {
		return certPEM, keyPEM, nil
	}

	// Generate new CA
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("create data dir: %w", err)
	}

	certPEM, keyPEM, err = generateCA()
	if err != nil {
		return nil, nil, err
	}

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return nil, nil, fmt.Errorf("write CA cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, nil, fmt.Errorf("write CA key: %w", err)
	}

	return certPEM, keyPEM, nil
}

func generateCA() (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "WxChannel macOS MITM CA",
			Organization: []string{"WxChannel"},
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}
