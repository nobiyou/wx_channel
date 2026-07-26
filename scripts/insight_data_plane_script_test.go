//go:build windows

package main

import (
	"encoding/json"
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
