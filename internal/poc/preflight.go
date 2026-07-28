package poc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var ErrPreflightFailed = errors.New("POC preflight failed")

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

type CertificateStore interface {
	Install(context.Context, string, string) error
	RemoveBySHA256(context.Context, string) error
	ContainsSHA256(context.Context, string) (bool, error)
}

type PreflightReport struct {
	Passed                  bool     `json:"passed"`
	IsolatedVM              bool     `json:"isolated_vm"`
	GitIgnored              bool     `json:"git_ignored"`
	ExecutableNameOK        bool     `json:"executable_name_ok"`
	LoopbackPortsFree       bool     `json:"loopback_ports_free"`
	SunnyDriverAbsent       bool     `json:"sunny_driver_absent"`
	DynamicPortBaselineHash string   `json:"dynamic_port_baseline_hash"`
	CertificateBaselineHash string   `json:"certificate_baseline_hash"`
	ListenerBaselineHash    string   `json:"listener_baseline_hash"`
	ReasonCodes             []string `json:"reason_codes"`
}

type VMDetector interface {
	Detect(context.Context) (bool, string, error)
}

type Preflight struct {
	runner         CommandRunner
	detector       VMDetector
	executableName func() string
}

func NewPreflight(runner CommandRunner, detector VMDetector) *Preflight {
	return &Preflight{
		runner:   runner,
		detector: detector,
		executableName: func() string {
			path, err := os.Executable()
			if err != nil {
				return ""
			}
			return filepath.Base(path)
		},
	}
}

func (p *Preflight) Run(ctx context.Context, options Options) (PreflightReport, error) {
	report := PreflightReport{}
	if err := options.ValidateForRun(); err != nil {
		report.ReasonCodes = []string{"options_not_approved"}
		return report, ErrPreflightFailed
	}
	if p == nil || p.runner == nil || p.detector == nil || p.executableName == nil {
		report.ReasonCodes = []string{"preflight_dependency_missing"}
		return report, ErrPreflightFailed
	}

	isolated, _, err := p.detector.Detect(ctx)
	report.IsolatedVM = err == nil && isolated
	if !report.IsolatedVM {
		report.ReasonCodes = append(report.ReasonCodes, "isolated_vm_not_detected")
	}
	report.ExecutableNameOK = strings.EqualFold(p.executableName(), "wx_channel_poc.exe")
	if !report.ExecutableNameOK {
		report.ReasonCodes = append(report.ReasonCodes, "unexpected_executable_name")
	}

	report.GitIgnored = p.checkGit(ctx, options)
	if !report.GitIgnored {
		report.ReasonCodes = append(report.ReasonCodes, "git_guard_failed")
	}
	report.LoopbackPortsFree = p.runBoolean(ctx, preflightPortsScript, options.ProxyAddress, options.BridgeAddress)
	if !report.LoopbackPortsFree {
		report.ReasonCodes = append(report.ReasonCodes, "loopback_port_in_use")
	}
	report.SunnyDriverAbsent = p.runBoolean(ctx, preflightDriverScript)
	if !report.SunnyDriverAbsent {
		report.ReasonCodes = append(report.ReasonCodes, "sunny_driver_present")
	}

	if output4, err4 := p.runner.Run(ctx, "netsh", "int", "ipv4", "show", "dynamicport", "tcp"); err4 == nil {
		if output6, err6 := p.runner.Run(ctx, "netsh", "int", "ipv6", "show", "dynamicport", "tcp"); err6 == nil {
			report.DynamicPortBaselineHash = hashNormalized(append(append([]byte("ipv4\n"), output4...), append([]byte("\nipv6\n"), output6...)...))
		}
	}
	if report.DynamicPortBaselineHash == "" {
		report.ReasonCodes = append(report.ReasonCodes, "dynamic_port_baseline_failed")
	}
	if output, err := p.runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", preflightCertificateBaselineScript); err == nil {
		report.CertificateBaselineHash = hashNormalized(output)
	}
	if report.CertificateBaselineHash == "" {
		report.ReasonCodes = append(report.ReasonCodes, "certificate_baseline_failed")
	}
	if output, err := p.runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", preflightListenerBaselineScript); err == nil {
		report.ListenerBaselineHash = hashNormalized(output)
	}
	if report.ListenerBaselineHash == "" {
		report.ReasonCodes = append(report.ReasonCodes, "listener_baseline_failed")
	}

	report.Passed = report.IsolatedVM && report.GitIgnored && report.ExecutableNameOK && report.LoopbackPortsFree && report.SunnyDriverAbsent && report.DynamicPortBaselineHash != "" && report.CertificateBaselineHash != "" && report.ListenerBaselineHash != ""
	if !report.Passed {
		return report, ErrPreflightFailed
	}
	return report, nil
}

func (p *Preflight) checkGit(ctx context.Context, options Options) bool {
	if _, err := p.runner.Run(ctx, "git", "status", "--porcelain"); err != nil {
		return false
	}
	for _, root := range []string{options.DataRoot, options.SecretsRoot, options.RuntimeRoot, options.BuildRoot, "var"} {
		probe := filepath.ToSlash(filepath.Join(root, "probe"))
		if _, err := p.runner.Run(ctx, "git", "check-ignore", "--quiet", "--", probe); err != nil {
			return false
		}
	}
	return true
}

func (p *Preflight) runBoolean(ctx context.Context, script string, args ...string) bool {
	if len(args) > 0 {
		quoted := make([]string, len(args))
		for i, arg := range args {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", "''") + "'"
		}
		script = "& { " + script + " } " + strings.Join(quoted, " ")
	}
	commandArgs := []string{"-NoProfile", "-NonInteractive", "-Command", script}
	output, err := p.runner.Run(ctx, "powershell.exe", commandArgs...)
	if err != nil {
		return false
	}
	value, err := parseBooleanOutput(output)
	return err == nil && value
}

func hashNormalized(raw []byte) string {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			normalized = append(normalized, line)
		}
	}
	sort.Strings(normalized)
	digest := sha256.Sum256([]byte(strings.Join(normalized, "\n")))
	return strings.ToUpper(hex.EncodeToString(digest[:]))
}

func parseBooleanOutput(raw []byte) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.New("command returned a non-boolean result")
	}
}

type WindowsVMDetector struct {
	runner CommandRunner
}

func NewWindowsVMDetector(runner CommandRunner) WindowsVMDetector {
	return WindowsVMDetector{runner: runner}
}

func (d WindowsVMDetector) Detect(ctx context.Context) (bool, string, error) {
	if d.runner == nil {
		return false, "detector_unavailable", errors.New("VM detector runner is missing")
	}
	output, err := d.runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", preflightVMScript)
	if err != nil {
		return false, "detector_failed", errors.New("query VM identity")
	}
	identity := strings.ToLower(strings.Join(strings.Fields(string(output)), " "))
	switch {
	case strings.Contains(identity, "vmware"):
		return true, "vmware", nil
	case strings.Contains(identity, "virtualbox"), strings.Contains(identity, "innotek"):
		return true, "virtualbox", nil
	case strings.Contains(identity, "kvm"), strings.Contains(identity, "qemu"):
		return true, "kvm-qemu", nil
	case strings.Contains(identity, "microsoft corporation") && strings.Contains(identity, "virtual machine"):
		return true, "hyper-v", nil
	default:
		return false, "no_known_vm_signal", nil
	}
}

const preflightVMScript = `$system = Get-CimInstance -ClassName Win32_ComputerSystem; [Console]::Out.Write(($system.Manufacturer + '|' + $system.Model))`
const preflightPortsScript = `$ports = @($args | ForEach-Object { [int](($_ -split ':')[-1]) }); $used = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $ports -contains $_.LocalPort }); [Console]::Out.Write(($used.Count -eq 0).ToString().ToLowerInvariant())`
const preflightDriverScript = `$service = Get-Service -Name SunnyFilter2 -ErrorAction SilentlyContinue; [Console]::Out.Write(($null -eq $service).ToString().ToLowerInvariant())`
const preflightCertificateBaselineScript = `Get-ChildItem Cert:\CurrentUser\Root | ForEach-Object { $sha = [Security.Cryptography.SHA256]::Create(); try { ([BitConverter]::ToString($sha.ComputeHash($_.RawData))).Replace('-','') } finally { $sha.Dispose() } } | Sort-Object`
const preflightListenerBaselineScript = `Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | ForEach-Object { "$($_.LocalAddress)|$($_.LocalPort)|$($_.OwningProcess)" } | Sort-Object`
