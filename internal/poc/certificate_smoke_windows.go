//go:build windows

package poc

import (
	"context"
	"errors"
	"io"
	"time"

	"golang.org/x/sys/windows"
)

func currentProcessIsElevated() (bool, error) {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated(), nil
}

func runPlatformCertificateSmoke(ctx context.Context, input io.Reader, output io.Writer, options Options) CertificateSmokeReceipt {
	runner := ExecCommandRunner{}
	deps := CertificateSmokeDeps{
		Preflight: func(ctx context.Context, options Options) (PreflightReport, error) {
			return NewPreflight(runner, NewWindowsVMDetector(runner)).Run(ctx, options)
		},
		IsElevated: currentProcessIsElevated,
		Approve:    newCertificateSmokeApproval(input, output),
		CertStore:  newCertificateStore(runner),
		StoreFactory: func(options Options, jobID string) (*Store, error) {
			repoRoot, err := existingCanonicalDirectory(".")
			if err != nil {
				return nil, errors.New("run POC from repository root")
			}
			return NewStore(StoreOptions{
				RepoRoot: repoRoot, DataRoot: options.DataRoot, SecretsRoot: options.SecretsRoot,
				RuntimeRoot: options.RuntimeRoot, BuildRoot: options.BuildRoot, VarRoot: "var", JobID: jobID,
			})
		},
		CreateCA: func(jobID string) (*JobCA, error) {
			return GenerateJobCA(jobID, time.Now().UTC())
		},
	}
	return NewCertificateSmoke(deps).Run(ctx, options)
}
