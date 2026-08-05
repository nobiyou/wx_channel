//go:build windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartInsightDataPlaneEmitOnlyDisablesRadarAndCloud(t *testing.T) {
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell host is not available on PATH")
	}

	scriptPath := filepath.Join(scriptDir(t), "start-insight-data-plane.ps1")
	cmd := exec.Command(host, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-EmitOnly")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-only script failed: %v\n%s", err, string(output))
	}

	var payload struct {
		RepoRoot  string            `json:"repoRoot"`
		ExePath   string            `json:"exePath"`
		ProxyPort int               `json:"proxyPort"`
		APIPort   int               `json:"apiPort"`
		StatusURL string            `json:"statusURL"`
		Env       map[string]string `json:"env"`
		Mode      string            `json:"mode"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("emit-only script returned invalid JSON: %v\n%s", err, string(output))
	}

	if payload.Mode != "wx-channel-insight-data-plane" {
		t.Fatalf("unexpected mode: %+v", payload)
	}
	if payload.ProxyPort != 2025 || payload.APIPort != 2026 {
		t.Fatalf("expected proxy/api ports 2025/2026, got %+v", payload)
	}
	if payload.StatusURL != "http://127.0.0.1:2026/api/channels/status" {
		t.Fatalf("unexpected status URL: %q", payload.StatusURL)
	}
	if payload.Env["WX_CHANNEL_RADAR_ENABLED"] != "false" {
		t.Fatalf("data plane script must disable radar, got env=%+v", payload.Env)
	}
	if payload.Env["WX_CHANNEL_CLOUD_ENABLED"] != "false" {
		t.Fatalf("data plane script must disable cloud connector, got env=%+v", payload.Env)
	}
	if !strings.HasSuffix(strings.ToLower(payload.ExePath), "wx_channel.exe") {
		t.Fatalf("expected wx_channel.exe path, got %q", payload.ExePath)
	}
}

func TestSwitchInsightDataPlaneEmitOnlyPlansBackupAndApplyGate(t *testing.T) {
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell host is not available on PATH")
	}

	repoRoot := filepath.Dir(scriptDir(t))
	candidate := filepath.Join(t.TempDir(), "wx_channel_team_ops_test.exe")
	if err := os.WriteFile(candidate, []byte("test exe placeholder"), 0o644); err != nil {
		t.Fatalf("write candidate placeholder failed: %v", err)
	}
	target := filepath.Join(repoRoot, "wx_channel_cloud.exe")

	scriptPath := filepath.Join(scriptDir(t), "switch-insight-data-plane.ps1")
	cmd := exec.Command(host,
		"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath,
		"-CandidateExePath", candidate,
		"-TargetExePath", target,
		"-EmitOnly",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-only switch script failed: %v\n%s", err, string(output))
	}

	var payload struct {
		Mode                   string            `json:"mode"`
		ApplyRequired          bool              `json:"applyRequired"`
		CandidateExe           string            `json:"candidateExePath"`
		TargetExe              string            `json:"targetExePath"`
		BackupPath             string            `json:"backupPath"`
		ProxyPort              int               `json:"proxyPort"`
		APIPort                int               `json:"apiPort"`
		StatusURL              string            `json:"statusURL"`
		RequiredCapabilityURLs map[string]string `json:"requiredCapabilityURLs"`
		Env                    map[string]string `json:"env"`
		Actions                []string          `json:"actions"`
		AdminCommand           string            `json:"adminRerunCommand"`
		AdminHint              string            `json:"adminRerunInstruction"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("emit-only switch script returned invalid JSON: %v\n%s", err, string(output))
	}

	if payload.Mode != "wx-channel-insight-data-plane-switch" {
		t.Fatalf("unexpected switch mode: %+v", payload)
	}
	if !payload.ApplyRequired {
		t.Fatalf("switch script should require explicit -Apply by default: %+v", payload)
	}
	if payload.ProxyPort != 2025 || payload.APIPort != 2026 {
		t.Fatalf("expected proxy/api ports 2025/2026, got %+v", payload)
	}
	if payload.StatusURL != "http://127.0.0.1:2026/api/channels/status" {
		t.Fatalf("unexpected status URL: %q", payload.StatusURL)
	}
	for name, expectedURL := range map[string]string{
		"feedSearch":    "http://127.0.0.1:2026/api/channels/feed/search?keyword=__insight_capability_probe__",
		"feedList":      "http://127.0.0.1:2026/api/channels/contact/feed/list?username=__insight_capability_probe__",
		"commentExport": "http://127.0.0.1:2026/api/channels/feed/comment/export",
	} {
		if payload.RequiredCapabilityURLs[name] != expectedURL {
			t.Fatalf("expected capability %s URL %q, got %+v", name, expectedURL, payload.RequiredCapabilityURLs)
		}
	}
	if !strings.HasSuffix(strings.ToLower(payload.CandidateExe), "wx_channel_team_ops_test.exe") {
		t.Fatalf("unexpected candidate exe: %q", payload.CandidateExe)
	}
	if !strings.HasSuffix(strings.ToLower(payload.TargetExe), "wx_channel_cloud.exe") {
		t.Fatalf("unexpected target exe: %q", payload.TargetExe)
	}
	if !strings.Contains(strings.ToLower(payload.BackupPath), "release") || !strings.Contains(strings.ToLower(payload.BackupPath), "backup") {
		t.Fatalf("expected backup path under release/backup, got %q", payload.BackupPath)
	}
	if payload.Env["WX_CHANNEL_RADAR_ENABLED"] != "false" || payload.Env["WX_CHANNEL_CLOUD_ENABLED"] != "false" {
		t.Fatalf("switch script must disable radar/cloud for Insight data plane: %+v", payload.Env)
	}
	if len(payload.Actions) == 0 || !strings.Contains(strings.Join(payload.Actions, " "), "backup") {
		t.Fatalf("expected switch plan to include backup action, got %+v", payload.Actions)
	}
	if !strings.Contains(strings.Join(payload.Actions, " "), "required Insight API routes") {
		t.Fatalf("expected switch plan to include required route check, got %+v", payload.Actions)
	}
	if !strings.Contains(payload.AdminCommand, "-Apply") ||
		!strings.Contains(payload.AdminCommand, "-Port 2025") ||
		!strings.Contains(payload.AdminCommand, "wx_channel_team_ops_test.exe") ||
		!strings.Contains(payload.AdminCommand, "wx_channel_cloud.exe") {
		t.Fatalf("expected admin rerun command to include apply/port/candidate/target, got %q", payload.AdminCommand)
	}
	if !strings.Contains(strings.ToLower(payload.AdminHint), "elevated powershell") {
		t.Fatalf("expected admin rerun hint for access denied recovery, got %q", payload.AdminHint)
	}
}

func TestSwitchInsightDataPlaneWaitsForPortReleaseBeforeCopy(t *testing.T) {
	scriptPath := filepath.Join(scriptDir(t), "switch-insight-data-plane.ps1")
	contentBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read switch script failed: %v", err)
	}
	content := string(contentBytes)

	waitIndex := strings.Index(content, "Wait-ForListenerRelease -Ports @($Port, $apiPort)")
	backupIndex := strings.Index(content, "Copy-Item -LiteralPath $targetExe -Destination $backupPath")
	if waitIndex < 0 {
		t.Fatalf("switch script must wait for listener ports to be released before replacing the target exe")
	}
	if backupIndex < 0 {
		t.Fatalf("switch script must still back up the target exe before replacement")
	}
	if waitIndex > backupIndex {
		t.Fatalf("switch script backs up/replaces target before listener ports are released")
	}
	if strings.Contains(content, "Stop-Process -Id $listenerPid -Force -ErrorAction SilentlyContinue") {
		t.Fatalf("switch script should not silently continue after listener stop failures")
	}
	if !strings.Contains(content, "taskkill.exe") {
		t.Fatalf("switch script should retain a Windows fallback when Stop-Process cannot terminate the listener")
	}
	if !strings.Contains(content, "AdminRerunCommand") || !strings.Contains(content, "access is denied") {
		t.Fatalf("switch script should surface an elevated PowerShell recovery command when listener stop is denied")
	}
}

func TestSwitchInsightDataPlaneWaitsForRequiredAPIRoutesAfterStatus(t *testing.T) {
	scriptPath := filepath.Join(scriptDir(t), "switch-insight-data-plane.ps1")
	contentBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read switch script failed: %v", err)
	}
	content := string(contentBytes)

	statusIndex := strings.Index(content, "Wait-ForStatusURL -URL $statusURL")
	capabilityIndex := strings.Index(content, "Wait-ForInsightCapabilityRoutes -ResolvedBaseURL $apiBaseURL")
	if statusIndex < 0 {
		t.Fatalf("switch script must wait for /api/channels/status")
	}
	if capabilityIndex < 0 {
		t.Fatalf("switch script must wait for required Insight API routes")
	}
	if capabilityIndex < statusIndex {
		t.Fatalf("switch script should wait for required API routes after status is reachable")
	}
	for _, required := range []string{
		"/api/channels/feed/search?keyword=__insight_capability_probe__",
		"/api/channels/contact/feed/list?username=__insight_capability_probe__",
		"/api/channels/feed/comment/export",
		"statusCode -eq 404",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("switch script should check capability route %q", required)
		}
	}
}

func TestSwitchInsightDataPlaneAdminEmitOnlyBuildsElevatedCommand(t *testing.T) {
	host, ok := resolvePowerShellHost()
	if !ok {
		t.Skip("PowerShell host is not available on PATH")
	}

	scriptPath := filepath.Join(scriptDir(t), "switch-insight-data-plane-admin.ps1")
	cmd := exec.Command(host,
		"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath,
		"-EmitOnly",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-only admin switch script failed: %v\n%s", err, string(output))
	}

	var payload struct {
		Mode         string   `json:"mode"`
		Elevated     bool     `json:"elevated"`
		SwitchScript string   `json:"switchScript"`
		ProxyPort    int      `json:"proxyPort"`
		APIPort      int      `json:"apiPort"`
		ArgumentList []string `json:"argumentList"`
		ArgumentLine string   `json:"argumentLine"`
		Command      string   `json:"command"`
		Action       string   `json:"action"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("emit-only admin script returned invalid JSON: %v\n%s", err, string(output))
	}

	if payload.Mode != "wx-channel-insight-data-plane-admin-switch" || !payload.Elevated {
		t.Fatalf("unexpected admin switch mode/elevated flag: %+v", payload)
	}
	if payload.ProxyPort != 2025 || payload.APIPort != 2026 {
		t.Fatalf("expected proxy/api ports 2025/2026, got %+v", payload)
	}
	if !strings.HasSuffix(strings.ToLower(payload.SwitchScript), "switch-insight-data-plane.ps1") {
		t.Fatalf("expected admin launcher to call switch script, got %q", payload.SwitchScript)
	}
	joinedArgs := strings.Join(payload.ArgumentList, " ")
	for _, required := range []string{"-NoExit", "-File", "-RepoRoot", "-Port 2025", "-Apply"} {
		if !strings.Contains(joinedArgs, required) {
			t.Fatalf("expected admin argument list to contain %q, got %q", required, joinedArgs)
		}
	}
	if !strings.Contains(payload.Command, "switch-insight-data-plane.ps1") || !strings.Contains(payload.Command, "-Apply") {
		t.Fatalf("expected admin command to include switch script and -Apply, got %q", payload.Command)
	}
	if !strings.Contains(payload.ArgumentLine, "'") || !strings.Contains(payload.ArgumentLine, "-Apply") {
		t.Fatalf("expected admin argument line to preserve quoting and -Apply, got %q", payload.ArgumentLine)
	}
	if !strings.Contains(payload.Action, "Start-Process -Verb RunAs") {
		t.Fatalf("expected admin action to explain elevation path, got %q", payload.Action)
	}
}

func TestSwitchInsightDataPlaneAdminCmdDelegatesToPowerShellSwitch(t *testing.T) {
	cmdPath := filepath.Join(scriptDir(t), "switch-insight-data-plane-admin.cmd")
	contentBytes, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("read admin cmd failed: %v", err)
	}
	content := string(contentBytes)
	lower := strings.ToLower(content)

	for _, required := range []string{
		"switch-insight-data-plane-admin.ps1",
		"powershell.exe",
		"-executionpolicy bypass",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("expected admin cmd to contain %q, got:\n%s", required, content)
		}
	}
	for _, forbidden := range []string{"switch-insight-data-plane.ps1\" -apply", "-noexit"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("admin cmd should delegate to the UAC launcher instead of running switch directly; found %q", forbidden)
		}
	}
	for _, forbidden := range []string{"copy-item", "stop-process", "taskkill"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("admin cmd should delegate instead of duplicating switch logic; found %q", forbidden)
		}
	}
}

func resolvePowerShellHost() (string, bool) {
	for _, candidate := range []string{"pwsh.exe", "pwsh", "powershell.exe", "powershell"} {
		if path, err := exec.LookPath(candidate); err == nil && path != "" {
			return path, true
		}
	}
	return "", false
}

func scriptDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve script dir failed: %v", err)
	}
	return path
}
