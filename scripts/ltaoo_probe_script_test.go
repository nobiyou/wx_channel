//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func probeRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func probePowerShell(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"powershell.exe", "powershell"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("Windows PowerShell is not available")
	return ""
}

func runProbeScript(t *testing.T, script string, args ...string) ([]byte, error) {
	t.Helper()
	commandArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(probeRepoRoot(t), "scripts", script)}
	commandArgs = append(commandArgs, args...)
	return exec.Command(probePowerShell(t), commandArgs...).CombinedOutput()
}

func TestLtaooProbeScriptsExist(t *testing.T) {
	for _, name := range []string{
		"prepare-ltaoo-probe.ps1",
		"probe-ltaoo-comments.ps1",
		"cleanup-ltaoo-probe.ps1",
	} {
		if _, err := os.Stat(filepath.Join(probeRepoRoot(t), "scripts", name)); err != nil {
			t.Errorf("%s must exist: %v", name, err)
		}
	}
}
