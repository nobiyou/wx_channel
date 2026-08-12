package poc

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
)

func newCertificateSmokeApproval(input io.Reader, output io.Writer) func(context.Context, CertificateSmokePlan) error {
	return func(ctx context.Context, plan CertificateSmokePlan) error {
		if plan.CertificateScope != "CurrentUser\\Root" {
			return newCertificateSmokeError(smokeApprovalRejected)
		}
		_, _ = fmt.Fprintf(output, "Planned certificate smoke change: certificate=%s\nType CERT_APPLY to continue: ", plan.CertificateScope)
		scanner := bufio.NewScanner(input)
		line := make(chan string, 1)
		go func() {
			if scanner.Scan() {
				line <- scanner.Text()
			}
			close(line)
		}()
		select {
		case <-ctx.Done():
			return newCertificateSmokeError(smokeApprovalRejected)
		case value, ok := <-line:
			if !ok || value != "CERT_APPLY" || scanner.Err() != nil {
				return newCertificateSmokeError(smokeApprovalRejected)
			}
		}
		return nil
	}
}

func RunCertificateSmokeCLI(ctx context.Context, input io.Reader, output io.Writer, args []string, options Options) int {
	flags := flag.NewFlagSet("cert-smoke", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ack := flags.Bool("ack-isolated-vm", false, "acknowledge disposable isolated VM")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || !*ack {
		_, _ = fmt.Fprintln(output, "cert-smoke requires --ack-isolated-vm")
		return 2
	}
	options.AckIsolatedVM = true
	options.AllowEncryptedRaw = false
	receipt := runPlatformCertificateSmoke(ctx, input, output, options)
	_ = json.NewEncoder(output).Encode(receipt)
	if receipt.Success {
		return 0
	}
	return 1
}

func RunPreflightCLI(ctx context.Context, output io.Writer, options Options) int {
	options.AckIsolatedVM = true
	report, err := runPlatformPreflight(ctx, options)
	_ = json.NewEncoder(output).Encode(report)
	if err != nil || !report.Passed {
		return 1
	}
	return 0
}

func RunCLI(ctx context.Context, input io.Reader, output io.Writer, args []string, options Options) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ack := flags.Bool("ack-isolated-vm", false, "acknowledge disposable isolated VM")
	encryptedRaw := flags.Bool("allow-encrypted-raw", false, "temporarily retain encrypted raw evidence")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(output, "invalid run arguments")
		return 2
	}
	options.AckIsolatedVM = *ack
	options.AllowEncryptedRaw = *encryptedRaw
	if err := options.ValidateForRun(); err != nil {
		_, _ = fmt.Fprintln(output, "approved isolated VM acknowledgement is required")
		return 2
	}
	runtime, err := newPlatformRuntime(input, output)
	if err != nil {
		_, _ = fmt.Fprintln(output, err.Error())
		return 1
	}
	if err := runtime.Run(ctx, options); err != nil {
		writeSafeRunError(output, err)
		_, _ = fmt.Fprintln(output, "POC run did not complete; cleanup path executed")
		return 1
	}
	_, _ = fmt.Fprintln(output, "POC run completed and cleanup path executed")
	return 0
}

func writeSafeRunError(output io.Writer, err error) {
	if code, ok := safeCertificateErrorCode(err); ok {
		_, _ = fmt.Fprintf(output, "POC run error code: %s\n", code)
	}
}

func RunCleanupCLI(ctx context.Context, output io.Writer, args []string, options Options) int {
	flags := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jobID := flags.String("job-id", "", "POC job ID")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || !validJobID(*jobID) {
		_, _ = fmt.Fprintln(output, "cleanup requires a valid --job-id")
		return 2
	}
	if err := runPlatformCleanup(ctx, options, *jobID); err != nil {
		_, _ = fmt.Fprintln(output, err.Error())
		return 1
	}
	_, _ = fmt.Fprintln(output, "cleanup completed")
	return 0
}
