//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrendRadarRuntimeRejectsIncompleteOrMixedRouterArguments(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	base := []string{
		"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(root, "scripts", "Invoke-LtaooTrendRadarBatch.ps1"),
		"-RequestPath", "missing-request", "-GrantPath", "missing-grant", "-RunRoot", "missing-root",
		"-RuntimeJournalPath", "missing-journal", "-LtaooExePath", "missing-ltaoo", "-BatchExePath", "missing-batch",
	}
	cases := map[string][]string{
		"incomplete generic": {"-RouterKind", "mihomo"},
		"incomplete legacy":  {"-ClashExePath", "clash.exe"},
		"mixed groups": {
			"-RouterKind", "mihomo", "-RouterExePath", "mihomo.exe", "-RouterConfigPath", "config.yaml",
			"-RouterCapabilityFingerprint", strings.Repeat("a", 64), "-ClashExePath", "clash.exe", "-ClashConfigPath", "clash.yaml",
		},
	}
	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			command := exec.Command(probePowerShell(t), append(base, arguments...)...)
			output, runErr := command.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 64 {
				t.Fatalf("exit=%v output=%s", runErr, output)
			}
		})
	}
}

func TestTrendRadarRuntimeAcceptsClosedLegacyAndGenericArgumentGroups(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	base := []string{
		"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(root, "scripts", "Invoke-LtaooTrendRadarBatch.ps1"),
		"-RequestPath", "missing-request", "-GrantPath", "missing-grant", "-RunRoot", "missing-root",
		"-RuntimeJournalPath", "missing-journal", "-LtaooExePath", "missing-ltaoo", "-BatchExePath", "missing-batch",
	}
	cases := map[string][]string{
		"legacy": {"-ClashExePath", "clash.exe", "-ClashConfigPath", "clash.yaml"},
		"generic": {
			"-RouterKind", "mihomo", "-RouterExePath", "mihomo.exe", "-RouterConfigPath", "config.yaml",
			"-RouterCapabilityFingerprint", strings.Repeat("a", 64),
		},
	}
	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			command := exec.Command(probePowerShell(t), append(base, arguments...)...)
			output, runErr := command.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 4 {
				t.Fatalf("exit=%v output=%s", runErr, output)
			}
		})
	}
}

func TestTrendRadarRuntimeImportsKeepRuntimeCommandsVisible(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimeModule := filepath.Join(root, "scripts", "LtaooRuntime.psm1")
	routerModule := filepath.Join(root, "scripts", "LtaooRouter.psm1")
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	command := strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"Import-Module " + quote(runtimeModule) + " -Force",
		"Import-Module " + quote(routerModule) + " -Force",
		"Get-Command Assert-LtaooNoReparsePoint -ErrorAction Stop | Out-Null",
		"Get-Command New-LtaooRouterBackend -ErrorAction Stop | Out-Null",
	}, "; ")

	output, runErr := exec.Command(
		probePowerShell(t),
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		command,
	).CombinedOutput()
	if runErr != nil {
		t.Fatalf("runtime commands disappeared after router import: %v\n%s", runErr, output)
	}
}

func TestTrendRadarRuntimeTreatsAlreadyRestoredRouterAsClean(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	entry := readRuntimeScript(t, filepath.Join(root, "scripts", "Invoke-LtaooTrendRadarBatch.ps1"))
	if !strings.Contains(entry, "if ($action -eq 'already_restored')") || !strings.Contains(entry, "return $true") {
		t.Fatal("already-restored router config must not require another reload")
	}
	if !strings.Contains(strings.ToLower(entry), "test-ltaooprocessidentityorabsent") {
		t.Fatal("cleanup must treat a PID that vanished during identity validation as already stopped")
	}
	if strings.Contains(entry, "if (-not [IO.File]::Exists($configPath) -or -not [IO.File]::Exists($backupPath)) { return $false }") {
		t.Fatal("already-restored router config must not require the deleted backup")
	}
}

func TestLtaooRuntimeReadsUTF8JSONWithoutBOM(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(
		requestPath,
		[]byte(`{"schema_version":1,"keyword":"启东音乐节"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runtimeModule := filepath.Join(root, "scripts", "LtaooRuntime.psm1")
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	command := strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"Import-Module " + quote(runtimeModule) + " -Force",
		"$value = Read-LtaooStrictJson -LiteralPath " + quote(requestPath) + " -AllowedProperties @('schema_version','keyword')",
		"if ($value.keyword -cne '启东音乐节') { throw 'keyword_mismatch' }",
	}, "; ")

	output, runErr := exec.Command(
		probePowerShell(t),
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		command,
	).CombinedOutput()
	if runErr != nil {
		t.Fatalf("UTF-8 JSON without BOM was rejected: %v\n%s", runErr, output)
	}
}

func TestTrendRadarRuntimeScriptSafety(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	entry := readRuntimeScript(t, filepath.Join(root, "scripts", "Invoke-LtaooTrendRadarBatch.ps1"))
	module := readRuntimeScript(t, filepath.Join(root, "scripts", "LtaooRuntime.psm1"))
	routerModule := readRuntimeScript(t, filepath.Join(root, "scripts", "LtaooRouter.psm1"))
	combined := strings.ToLower(entry + "\n" + module + "\n" + routerModule)

	for _, required := range []string{
		"-literalpath", "convertfrom-json", "allowedproperties", "currentuser\\root", "certutil.exe -user",
		"start-process", "-windowstyle hidden", "try", "finally", "grant", "request_sha256",
		"runtime_paths_sha256", "ltaoo_executable_sha256", "threading.mutex", "runtime-journal.json",
		"batch_executable_sha256", "test-ltaooprocessidentity", "convertfrom-ltaooutf8bytes",
		"wx_channel_ltaoo_batch", "cleanup-receipt", "process-name,wx_video_download.exe,direct",
		"process-name,wechatappex.exe", "process-name,weixin.exe", "process-name,wechat.exe", "external-controller", "/configs?force=true",
		"-datadirectory (split-path -parent $resolvedrouterconfig)",
		"safefailurecode", "runtime_failed",
	} {
		if !strings.Contains(combined, required) {
			t.Errorf("runtime scripts missing %q", required)
		}
	}
	for _, forbidden := range []string{"read-host", "install $runid", "invoke-expression", "cmd /c", "localmachine\\root"} {
		if strings.Contains(combined, forbidden) {
			t.Errorf("runtime scripts contain forbidden %q", forbidden)
		}
	}
	for _, required := range []string{
		"new-ltaoorouterbackend", "router_kind_unsupported", "trendradar.mihomobackend",
		"add-ltaoorouterblock", "remove-ltaoorouterblock", "test-ltaoorouterconfig",
	} {
		if !strings.Contains(strings.ToLower(routerModule), required) {
			t.Errorf("router module missing %q", required)
		}
	}
	for _, required := range []string{
		"$routerkind", "$routerexepath", "$routerconfigpath", "$routercapabilityfingerprint",
		"router_restored", "restore-journalrouterconfig",
		"router_baseline_sha256", "router_temporary_sha256", "router_backup_path",
	} {
		if !strings.Contains(strings.ToLower(entry), required) {
			t.Errorf("runtime entry missing generic contract %q", required)
		}
	}
	if !strings.Contains(combined, "modify_proxy_router") {
		t.Error("runtime modules missing generic router grant action")
	}
}

func readRuntimeScript(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
