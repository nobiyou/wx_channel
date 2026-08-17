//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrendRadarRuntimeScriptSafety(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	entry := readRuntimeScript(t, filepath.Join(root, "scripts", "Invoke-LtaooTrendRadarBatch.ps1"))
	module := readRuntimeScript(t, filepath.Join(root, "scripts", "LtaooRuntime.psm1"))
	combined := strings.ToLower(entry + "\n" + module)

	for _, required := range []string{
		"-literalpath", "convertfrom-json", "allowedproperties", "currentuser\\root", "certutil.exe -user",
		"start-process", "-windowstyle hidden", "try", "finally", "grant", "request_sha256",
		"runtime_paths_sha256", "ltaoo_executable_sha256", "threading.mutex", "runtime-journal.json",
		"batch_executable_sha256", "test-ltaooprocessidentity", "convertfrom-ltaooutf8bytes",
		"wx_channel_ltaoo_batch", "cleanup-receipt", "process-name,wx_video_download.exe,direct",
		"process-name,wechatappex.exe", "process-name,weixin.exe", "process-name,wechat.exe", "external-controller", "/configs?force=true",
		"-datadirectory (split-path -parent $resolvedclashconfig)",
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
}

func readRuntimeScript(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
