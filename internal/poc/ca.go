package poc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	SunnyNet "github.com/qtgolang/SunnyNet/SunnyNet"
	"github.com/qtgolang/SunnyNet/src/Certificate"
)

type JobCA struct {
	CertPEM           []byte
	KeyPEM            []byte
	Certificate       *x509.Certificate
	SHA256Fingerprint string
}

func GenerateJobCA(jobID string, now time.Time) (*JobCA, error) {
	if !jobIDPattern.MatchString(jobID) || jobID == "." || jobID == ".." {
		return nil, errors.New("invalid CA job ID")
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, errors.New("generate job CA key")
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, errors.New("generate job CA serial")
	}
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	now = now.UTC()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "wx-channel-poc-" + jobID},
		NotBefore:             now,
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		SignatureAlgorithm:    x509.SHA256WithRSA,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, errors.New("create job CA certificate")
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, errors.New("parse generated job CA certificate")
	}
	digest := sha256.Sum256(der)
	return &JobCA{
		CertPEM:           pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		KeyPEM:            pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}),
		Certificate:       certificate,
		SHA256Fingerprint: strings.ToUpper(hex.EncodeToString(digest[:])),
	}, nil
}

func (c *JobCA) WriteSecrets(secretsDir string) (certPath string, keyPath string, err error) {
	if c == nil || c.Certificate == nil || len(c.CertPEM) == 0 || len(c.KeyPEM) == 0 || !filepath.IsAbs(secretsDir) {
		return "", "", errors.New("invalid CA or secrets directory")
	}
	canonical, err := existingCanonicalDirectory(secretsDir)
	if err != nil || !samePath(canonical, secretsDir) {
		return "", "", errors.New("secrets directory failed link validation")
	}
	jobID := strings.TrimPrefix(c.Certificate.Subject.CommonName, "wx-channel-poc-")
	if jobID == c.Certificate.Subject.CommonName || filepath.Base(canonical) != jobID || filepath.Base(filepath.Dir(canonical)) != ".poc-secrets" {
		return "", "", errors.New("secrets directory does not match job CA")
	}
	certPath = filepath.Join(canonical, "job-ca.pem")
	keyPath = filepath.Join(canonical, "job-ca.key")
	if err := writeSecretExclusive(certPath, c.CertPEM); err != nil {
		return "", "", err
	}
	if err := writeSecretExclusive(keyPath, c.KeyPEM); err != nil {
		_ = os.Remove(certPath)
		return "", "", err
	}
	return certPath, keyPath, nil
}

func writeSecretExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create secret file")
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return errors.New("restrict secret file permissions")
	}
	if _, err := file.Write(data); err != nil {
		return errors.New("write secret file")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync secret file")
	}
	if err := file.Close(); err != nil {
		return errors.New("close secret file")
	}
	complete = true
	return nil
}

func (c *JobCA) NewSunny() (*SunnyNet.Sunny, error) {
	if c == nil || c.Certificate == nil || len(c.CertPEM) == 0 || len(c.KeyPEM) == 0 {
		return nil, errors.New("invalid job CA")
	}
	managerID := Certificate.CreateCertificate()
	defer Certificate.RemoveCertificate(managerID)
	manager := Certificate.LoadCertificateContext(managerID)
	if manager == nil || !manager.LoadX509Certificate(c.Certificate.Subject.CommonName, string(c.CertPEM), string(c.KeyPEM)) {
		return nil, errors.New("load job CA into SunnyNet")
	}
	sunny := SunnyNet.NewSunny()
	sunny.SetCert(managerID).SetLoopbackOnly()
	if sunny.Error != nil {
		return nil, errors.New("configure SunnyNet job CA")
	}
	return sunny, nil
}
