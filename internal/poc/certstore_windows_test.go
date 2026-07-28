//go:build windows

package poc

import (
	"context"
	"os/exec"
	"strings"
	"testing"
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
