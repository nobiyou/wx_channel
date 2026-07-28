//go:build windows

package poc

import (
	"strings"
	"testing"
)

func TestCertificateScriptsUseCurrentUserOnly(t *testing.T) {
	combined := certificateMatchScript + certificateInstallScript + certificateRemoveScript
	if !strings.Contains(combined, `Cert:\CurrentUser\Root`) || strings.Contains(strings.ToLower(combined), "localmachine") {
		t.Fatal("certificate scripts are not restricted to CurrentUser Root")
	}
}
