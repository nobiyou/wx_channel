package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPOCScriptsUseOnlyRestrictedBuildTarget(t *testing.T) {
	build := readScript(t, "build-poc.ps1")
	if !strings.Contains(build, "./cmd/wx_channel_poc") || !strings.Contains(build, ".poc-build") || !strings.Contains(build, "wx_channel_poc.exe") {
		t.Fatal("restricted build target or output is missing")
	}
	for name, source := range map[string]string{
		"build": build, "audit": readScript(t, "poc-security-audit.ps1"), "cleanup": readScript(t, "poc-cleanup.ps1"),
	} {
		lower := strings.ToLower(source)
		for _, forbidden := range []string{"start-insight-data-plane", "go run .", "wx_channel.exe", "remove-item -recurse"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s script contains forbidden text %q", name, forbidden)
			}
		}
	}
}

func TestPOCScriptsCleanupValidatesResolvedPrefixesAndDoesNotDeleteDirectly(t *testing.T) {
	cleanup := strings.ToLower(readScript(t, "poc-cleanup.ps1"))
	resolveIndex := strings.Index(cleanup, "resolve-path")
	prefixIndex := strings.Index(cleanup, "startswith")
	invokeIndex := strings.Index(cleanup, "cleanup --job-id")
	if resolveIndex < 0 || prefixIndex < 0 || invokeIndex < 0 || resolveIndex > invokeIndex || prefixIndex > invokeIndex {
		t.Fatalf("cleanup validation ordering is unsafe: resolve=%d prefix=%d invoke=%d", resolveIndex, prefixIndex, invokeIndex)
	}
	if strings.Contains(cleanup, "remove-item") || strings.Contains(cleanup, "cert:\\") {
		t.Fatal("cleanup wrapper performs a direct destructive action")
	}
}

func TestCertificateSmokeLauncherHasNoCollectionOrAutomaticApproval(t *testing.T) {
	launcher := strings.ToLower(readScript(t, "run-certificate-smoke.cmd"))
	if !strings.Contains(launcher, "cert-smoke --ack-isolated-vm") {
		t.Fatal("certificate smoke launcher is missing the restricted command")
	}
	for _, forbidden := range []string{
		" run ", "apply", "sunnynet", "wechatappex", "127.0.0.1:2025",
		"127.0.0.1:2026", "import-certificate", "certutil", "cert:\\",
	} {
		if strings.Contains(launcher, forbidden) {
			t.Fatalf("certificate smoke launcher contains forbidden text %q", forbidden)
		}
	}
}

func readScript(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate script test")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
