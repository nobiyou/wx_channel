//go:build !windows

package poc

import (
	"context"
	"errors"
	"io"
	"time"
)

var errWindowsPOCRequired = errors.New("POC runtime requires isolated Windows VM")

func newPlatformRuntime(io.Reader, io.Writer) (*Runtime, error) { return nil, errWindowsPOCRequired }
func runPlatformPreflight(context.Context, Options) (PreflightReport, error) {
	return PreflightReport{}, errWindowsPOCRequired
}
func runPlatformCleanup(context.Context, Options, string) error { return errWindowsPOCRequired }
func runPlatformCertificateSmoke(context.Context, io.Reader, io.Writer, Options) CertificateSmokeReceipt {
	receipt := CertificateSmokeReceipt{SchemaVersion: CertificateSmokeSchemaVersion, CompletedAt: time.Now().UTC()}
	setCertificateSmokeReceiptCode(&receipt, smokePreflightFailed)
	return receipt
}
