package poc

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
)

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
