package poc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeLoggerRejectsUnknownEventAndFieldBeforeCreate(t *testing.T) {
	store := newTestStore(t, "safe-log-job")
	path := filepath.Join(store.JobDir(), "run.log")
	logger, err := NewFileSafeLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Event("unknown_event", nil); err == nil {
		t.Fatal("unknown event accepted")
	}
	if err := logger.Event("progress", map[string]any{"comment_id": "fixture-id"}); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("log was created: %v", err)
	}
}

func TestSafeLoggerRejectsContentURLAndCredentialShapedValues(t *testing.T) {
	store := newTestStore(t, "safe-log-values-job")
	path := filepath.Join(store.JobDir(), "run.log")
	logger, err := NewFileSafeLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"fixture 正文", "https://fixture.invalid/path?temporary=value", `{"token":"fixture"}`} {
		if err := logger.Event("progress", map[string]any{"phase": value}); err == nil {
			t.Fatalf("unsafe value accepted: %q", value)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unsafe log was created: %v", err)
	}
}

func TestSafeLoggerAppendsOnlyWhitelistedMetadata(t *testing.T) {
	store := newTestStore(t, "safe-log-append-job")
	path := filepath.Join(store.JobDir(), "run.log")
	logger, err := NewFileSafeLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Event("request_completed", map[string]any{"method": "finderSearch", "request_sequence": 1, "bytes": 42}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ScanOrdinaryOutput(raw); err != nil {
		t.Fatal(err)
	}
	size := len(raw)
	if err := logger.Event("progress", map[string]any{"phase": "https://fixture.invalid/path?temporary=value"}); err == nil {
		t.Fatal("unsafe append accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != size {
		t.Fatalf("unsafe event changed log size: before=%d after=%d", size, len(after))
	}
}
