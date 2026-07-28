package poc

import "errors"

type certificateErrorCode string

const (
	certificatePrecheckCommandFailed certificateErrorCode = "certificate_precheck_command_failed"
	certificateAlreadyPresent        certificateErrorCode = "certificate_already_present"
	certificateImportCommandFailed   certificateErrorCode = "certificate_import_command_failed"
	certificateImportReportedFalse   certificateErrorCode = "certificate_import_reported_false"
	certificatePostcheckFailed       certificateErrorCode = "certificate_postcheck_failed"
)

type certificateStoreError struct{ code certificateErrorCode }

func (e certificateStoreError) Error() string { return string(e.code) }

func newCertificateStoreError(code certificateErrorCode) error {
	switch code {
	case certificatePrecheckCommandFailed, certificateAlreadyPresent,
		certificateImportCommandFailed, certificateImportReportedFalse,
		certificatePostcheckFailed:
		return certificateStoreError{code: code}
	default:
		return errors.New("certificate store failure")
	}
}

func safeCertificateErrorCode(err error) (string, bool) {
	var target certificateStoreError
	if !errors.As(err, &target) {
		return "", false
	}
	switch target.code {
	case certificatePrecheckCommandFailed, certificateAlreadyPresent,
		certificateImportCommandFailed, certificateImportReportedFalse,
		certificatePostcheckFailed:
		return string(target.code), true
	default:
		return "", false
	}
}
