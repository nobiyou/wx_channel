package poc

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestRunRefusesUnknownFlagsAndMissingAcknowledgement(t *testing.T) {
	for _, args := range [][]string{{}, {"--unknown"}, {"--allow-encrypted-raw"}} {
		var output bytes.Buffer
		if code := RunCLI(context.Background(), strings.NewReader("APPLY\n"), &output, args, DefaultOptions()); code != 2 {
			t.Fatalf("args=%v code=%d output=%q", args, code, output.String())
		}
	}
}

func TestCleanupRequiresStrictJobID(t *testing.T) {
	for _, args := range [][]string{{}, {"--job-id", "../escape"}, {"--job-id", ".."}, {"--job-id", "."}, {"--job-id", ""}, {"--job-id", "fixture", "extra"}} {
		var output bytes.Buffer
		if code := RunCleanupCLI(context.Background(), &output, args, DefaultOptions()); code != 2 {
			t.Fatalf("args=%v code=%d output=%q", args, code, output.String())
		}
	}
}

func TestCertificateSmokeCLIRequiresOnlyIsolatedVMAcknowledgement(t *testing.T) {
	for _, args := range [][]string{{}, {"--unknown"}, {"--allow-encrypted-raw"}, {"--ack-isolated-vm", "extra"}} {
		var output bytes.Buffer
		if code := RunCertificateSmokeCLI(context.Background(), strings.NewReader("CERT_APPLY\n"), &output, args, DefaultOptions()); code != 2 {
			t.Fatalf("args=%v code=%d output=%q", args, code, output.String())
		}
	}
}

func TestCertificateSmokeApprovalRequiresExactText(t *testing.T) {
	for _, input := range []string{"", "cert_apply\n", "CERT_APPLY \n", "APPLY\n"} {
		var output bytes.Buffer
		approve := newCertificateSmokeApproval(strings.NewReader(input), &output)
		if err := approve(context.Background(), CertificateSmokePlan{CertificateScope: "CurrentUser\\Root"}); err == nil {
			t.Fatalf("input=%q was accepted", input)
		}
	}
	approve := newCertificateSmokeApproval(strings.NewReader("CERT_APPLY\n"), io.Discard)
	if err := approve(context.Background(), CertificateSmokePlan{CertificateScope: "CurrentUser\\Root"}); err != nil {
		t.Fatal(err)
	}
}

func TestCertificateSmokeApprovalPrintsOnlyCurrentUserPlan(t *testing.T) {
	var output bytes.Buffer
	approve := newCertificateSmokeApproval(strings.NewReader("wrong\n"), &output)
	_ = approve(context.Background(), CertificateSmokePlan{CertificateScope: "CurrentUser\\Root"})
	if got := output.String(); got != "Planned certificate smoke change: certificate=CurrentUser\\Root\nType CERT_APPLY to continue: " {
		t.Fatalf("prompt=%q", got)
	}
}
