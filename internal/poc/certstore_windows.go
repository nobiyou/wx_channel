//go:build windows

package poc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows"
)

var fingerprintPattern = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)

type windowsCertificateStore struct {
	runner CommandRunner
}

func newCertificateStore(runner CommandRunner) CertificateStore {
	return &windowsCertificateStore{runner: runner}
}

func (s *windowsCertificateStore) Install(ctx context.Context, certPath, fingerprint string) error {
	fingerprint, err := normalizeFingerprint(fingerprint)
	if err != nil || !filepath.IsAbs(certPath) {
		return errors.New("invalid certificate install arguments")
	}
	info, err := os.Lstat(certPath)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("certificate path is not a regular file")
	}
	if filepath.Base(certPath) != "job-ca.cert" || filepath.Base(filepath.Dir(filepath.Dir(certPath))) != ".poc-secrets" {
		return errors.New("certificate path is outside the POC secrets tree")
	}
	parent, err := existingCanonicalDirectory(filepath.Dir(certPath))
	if err != nil || !samePath(parent, filepath.Dir(certPath)) {
		return errors.New("certificate directory failed link validation")
	}
	if err := rejectPlatformReparse(certPath); err != nil {
		return errors.New("certificate path is a reparse point")
	}
	if err := verifyCertificateFileFingerprint(certPath, fingerprint, info.Size()); err != nil {
		return err
	}
	present, err := s.ContainsSHA256(ctx, fingerprint)
	if err != nil {
		return err
	}
	if present {
		return errors.New("certificate fingerprint already exists")
	}
	installed, err := s.runBoolean(ctx, certificateInstallScript, certPath, fingerprint)
	if err != nil || !installed {
		return errors.New("install CurrentUser root certificate")
	}
	return nil
}

func verifyCertificateFileFingerprint(path, expected string, size int64) error {
	if size <= 0 || size > 1<<20 {
		return errors.New("certificate file size is invalid")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("read certificate file")
	}
	certificates, err := x509.ParseCertificates(raw)
	if err != nil || len(certificates) != 1 || !bytes.Equal(certificates[0].Raw, raw) {
		return errors.New("certificate file is not a single DER certificate")
	}
	certificate := certificates[0]
	if certificate == nil {
		return errors.New("parse certificate file")
	}
	if !certificate.IsCA || certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
		return errors.New("certificate file is not a CA")
	}
	digest := sha256.Sum256(certificate.Raw)
	if strings.ToUpper(hex.EncodeToString(digest[:])) != expected {
		return errors.New("certificate file fingerprint mismatch")
	}
	return nil
}

func (s *windowsCertificateStore) ContainsSHA256(ctx context.Context, fingerprint string) (bool, error) {
	fingerprint, err := normalizeFingerprint(fingerprint)
	if err != nil {
		return false, err
	}
	return s.runBoolean(ctx, certificateMatchScript, fingerprint)
}

func (s *windowsCertificateStore) RemoveBySHA256(ctx context.Context, fingerprint string) error {
	fingerprint, err := normalizeFingerprint(fingerprint)
	if err != nil {
		return err
	}
	matched, err := s.runBoolean(ctx, certificateMatchScript, fingerprint)
	if err != nil || !matched {
		return errors.New("certificate fingerprint did not match exactly one CurrentUser root")
	}
	removed, err := s.runBoolean(ctx, certificateRemoveScript, fingerprint)
	if err != nil || !removed {
		return errors.New("remove CurrentUser root certificate")
	}
	return nil
}

func (s *windowsCertificateStore) runBoolean(ctx context.Context, script string, args ...string) (bool, error) {
	if s == nil || s.runner == nil {
		return false, errors.New("certificate command runner is missing")
	}
	commandArgs := powerShellCommandArgs(script, args...)
	output, err := s.runner.Run(ctx, "powershell.exe", commandArgs...)
	if err != nil {
		return false, errors.New("certificate store command failed")
	}
	return parseBooleanOutput(output)
}

func normalizeFingerprint(value string) (string, error) {
	if !fingerprintPattern.MatchString(value) {
		return "", errors.New("invalid SHA-256 certificate fingerprint")
	}
	return strings.ToUpper(value), nil
}

func restrictSecretDirectory(ctx context.Context, runner CommandRunner, dir string) error {
	if runner == nil || !filepath.IsAbs(dir) {
		return errors.New("invalid secret ACL arguments")
	}
	canonical, err := existingCanonicalDirectory(dir)
	if err != nil || !samePath(canonical, dir) || filepath.Base(filepath.Dir(canonical)) != ".poc-secrets" {
		return errors.New("secret ACL directory failed validation")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return errors.New("read current user SID")
	}
	sid := user.User.Sid.String()
	_, err = runner.Run(ctx, "icacls.exe", canonical, "/inheritance:r", "/grant:r", "*"+sid+":(OI)(CI)F")
	if err != nil {
		return errors.New("restrict secret directory ACL")
	}
	return nil
}

const certificateHashPipeline = `$sha = [Security.Cryptography.SHA256]::Create(); try { $hash = ([BitConverter]::ToString($sha.ComputeHash($_.RawData))).Replace('-','') } finally { $sha.Dispose() }; `
const certificateMatchScript = `$expected = $args[0].ToUpperInvariant(); $matches = @(Get-ChildItem Cert:\CurrentUser\Root | Where-Object { ` + certificateHashPipeline + `$hash -eq $expected }); [Console]::Out.Write(($matches.Count -eq 1).ToString().ToLowerInvariant())`
const certificateInstallScript = `$path = $args[0]; $expected = $args[1].ToUpperInvariant(); $cert = @(Import-Certificate -FilePath $path -CertStoreLocation Cert:\CurrentUser\Root -ErrorAction Stop); if ($cert.Count -ne 1) { [Console]::Out.Write('false'); exit 0 }; $sha = [Security.Cryptography.SHA256]::Create(); try { $hash = ([BitConverter]::ToString($sha.ComputeHash($cert[0].RawData))).Replace('-','') } finally { $sha.Dispose() }; if ($hash -ne $expected) { Remove-Item -LiteralPath $cert[0].PSPath -Force -ErrorAction SilentlyContinue; [Console]::Out.Write('false'); exit 0 }; [Console]::Out.Write('true')`
const certificateRemoveScript = `$expected = $args[0].ToUpperInvariant(); $matches = @(Get-ChildItem Cert:\CurrentUser\Root | Where-Object { ` + certificateHashPipeline + `$hash -eq $expected }); if ($matches.Count -ne 1) { [Console]::Out.Write('false'); exit 0 }; Remove-Item -LiteralPath $matches[0].PSPath -Force -ErrorAction Stop; [Console]::Out.Write('true')`
