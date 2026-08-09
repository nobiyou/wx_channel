package poc

import (
	"errors"
	"time"
)

const CertificateSmokeSchemaVersion = "wx-channel-comment-poc/certificate-smoke-1"

type certificateSmokeErrorCode string

const (
	smokePreflightFailed           certificateSmokeErrorCode = "smoke_preflight_failed"
	smokeApprovalRejected          certificateSmokeErrorCode = "smoke_approval_rejected"
	smokeCertificatePreexisting    certificateSmokeErrorCode = "smoke_certificate_preexisting"
	smokeInstallFailed             certificateSmokeErrorCode = "smoke_install_failed"
	smokeInstallVerificationFailed certificateSmokeErrorCode = "smoke_install_verification_failed"
	smokeRemoveFailed              certificateSmokeErrorCode = "smoke_remove_failed"
	smokeRemoveVerificationFailed  certificateSmokeErrorCode = "smoke_remove_verification_failed"
	smokeSecretsCleanupFailed      certificateSmokeErrorCode = "smoke_secrets_cleanup_failed"
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
