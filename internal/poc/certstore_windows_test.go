//go:build windows

package poc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertificateScriptsUseCurrentUserOnly(t *testing.T) {
	combined := certificateMatchScript + certificateInstallScript + certificateRemoveScript
	if !strings.Contains(combined, `Cert:\CurrentUser\Root`) || strings.Contains(strings.ToLower(combined), "localmachine") {
		t.Fatal("certificate scripts are not restricted to CurrentUser Root")
	}
}

func TestCertificateBooleanPassesArgumentsToPowerShellScript(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}

	store := &windowsCertificateStore{runner: ExecCommandRunner{}}
	script := `[Console]::Out.Write((($args -join '|') -eq 'certificate|fingerprint').ToString().ToLowerInvariant())`
	ok, err := store.runBoolean(context.Background(), script, "certificate", "fingerprint")
	if err != nil || !ok {
		t.Fatalf("PowerShell script did not receive the supplied arguments: ok=%v err=%v", ok, err)
	}
}

func TestGeneratedCertificateIsAcceptedByImportCertificateWhatIf(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}

	ca, err := GenerateJobCA("job-import-whatif", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), ".poc-secrets", "job-import-whatif")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	certPath, _, err := ca.WriteSecrets(dir)
	if err != nil {
		t.Fatal(err)
	}

	store := &windowsCertificateStore{runner: ExecCommandRunner{}}
	script := `$null = Import-Certificate -FilePath $args[0] -CertStoreLocation Cert:\CurrentUser\Root -WhatIf -Confirm:$false -InformationAction SilentlyContinue -ErrorAction Stop; [Console]::Out.Write('true')`
	ok, err := store.runBoolean(context.Background(), script, certPath)
	if err != nil || !ok {
		t.Fatalf("generated certificate was not accepted by Import-Certificate -WhatIf: ok=%v err=%v", ok, err)
	}
}
