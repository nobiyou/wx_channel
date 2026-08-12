package poc

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

type runnerCall struct {
	name string
	args []string
}

type recordingRunner struct {
	outputs      [][]byte
	calls        []runnerCall
	calledRemove bool
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	copyArgs := append([]string(nil), args...)
	r.calls = append(r.calls, runnerCall{name: name, args: copyArgs})
	for _, arg := range args {
		if strings.Contains(arg, "Remove-Item") {
			r.calledRemove = true
		}
	}
	if len(r.outputs) == 0 {
		return []byte("true"), nil
	}
	output := r.outputs[0]
	r.outputs = r.outputs[1:]
	return output, nil
}

type fakeVMDetector struct {
	isolated bool
	err      error
}

func (d fakeVMDetector) Detect(context.Context) (bool, string, error) {
	return d.isolated, "fixture-vm", d.err
}

func TestPreflightRejectsMissingIsolationAck(t *testing.T) {
	report, err := NewPreflight(&recordingRunner{}, fakeVMDetector{isolated: true}).Run(context.Background(), DefaultOptions())
	if err == nil || report.Passed {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestPreflightProducesHashesWithoutRawBaselines(t *testing.T) {
	runner := &recordingRunner{}
	preflight := NewPreflight(runner, fakeVMDetector{isolated: true})
	preflight.executableName = func() string { return "wx_channel_poc.exe" }
	options := DefaultOptions()
	options.AckIsolatedVM = true
	report, err := preflight.Run(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.DynamicPortBaselineHash == "" || report.CertificateBaselineHash == "" || report.ListenerBaselineHash == "" {
		t.Fatalf("report=%+v", report)
	}
	encoded := report.DynamicPortBaselineHash + report.CertificateBaselineHash + report.ListenerBaselineHash
	if strings.Contains(encoded, "true") {
		t.Fatalf("raw baseline leaked: %q", encoded)
	}
}

func TestPreflightRejectsNonVM(t *testing.T) {
	preflight := NewPreflight(&recordingRunner{}, fakeVMDetector{isolated: false})
	preflight.executableName = func() string { return "wx_channel_poc.exe" }
	options := DefaultOptions()
	options.AckIsolatedVM = true
	report, err := preflight.Run(context.Background(), options)
	if err == nil || report.Passed || !errors.Is(err, ErrPreflightFailed) {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestRunBooleanPassesArgumentsToPowerShellScript(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}

	preflight := &Preflight{runner: ExecCommandRunner{}}
	script := `[Console]::Out.Write((($args -join '|') -eq '127.0.0.1:2025|127.0.0.1:2026').ToString().ToLowerInvariant())`
	if !preflight.runBoolean(context.Background(), script, "127.0.0.1:2025", "127.0.0.1:2026") {
		t.Fatal("PowerShell script did not receive the supplied arguments")
	}
}
