package poc

import (
	"errors"
	"strings"
	"testing"
)

func TestWriteSafeRunErrorOnlyPrintsAllowlistedCertificateCode(t *testing.T) {
	var output strings.Builder
	writeSafeRunError(&output, newCertificateStoreError(certificateImportCommandFailed))
	if got := output.String(); got != "POC run error code: certificate_import_command_failed\n" {
		t.Fatalf("output=%q", got)
	}

	output.Reset()
	writeSafeRunError(&output, errors.New(`secret C:\path fingerprint ABC token XYZ`))
	if output.Len() != 0 {
		t.Fatalf("untrusted error leaked: %q", output.String())
	}
}
