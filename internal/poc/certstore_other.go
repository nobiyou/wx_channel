//go:build !windows

package poc

import (
	"context"
	"errors"
)

var errWindowsPOCRuntime = errors.New("POC runtime requires Windows")

type unsupportedCertificateStore struct{}

func newCertificateStore(CommandRunner) CertificateStore {
	return unsupportedCertificateStore{}
}

func (unsupportedCertificateStore) Install(context.Context, string, string) error {
	return errWindowsPOCRuntime
}

func (unsupportedCertificateStore) RemoveBySHA256(context.Context, string) error {
	return errWindowsPOCRuntime
}

func (unsupportedCertificateStore) ContainsSHA256(context.Context, string) (bool, error) {
	return false, errWindowsPOCRuntime
}

func restrictSecretDirectory(context.Context, CommandRunner, string) error {
	return errWindowsPOCRuntime
}
