package poc

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRefusesUnknownFlagsAndMissingAcknowledgement(t *testing.T) {
	for _, args := range [][]string{{}, {"--unknown"}, {"--allow-encrypted-raw"}} {
		var output bytes.Buffer
		if code := RunCLI(context.Background(), strings.NewReader("APPLY\n"), &output, args, DefaultOptions()); code != 2 {
			t.Fatalf("args=%v code=%d output=%q", args, code, output.String())
		}
	}
}

func TestCleanupRequiresStrictJobID(t *testing.T) {
	for _, args := range [][]string{{}, {"--job-id", "../escape"}, {"--job-id", ".."}, {"--job-id", "."}, {"--job-id", ""}, {"--job-id", "fixture", "extra"}} {
		var output bytes.Buffer
		if code := RunCleanupCLI(context.Background(), &output, args, DefaultOptions()); code != 2 {
			t.Fatalf("args=%v code=%d output=%q", args, code, output.String())
		}
	}
}
