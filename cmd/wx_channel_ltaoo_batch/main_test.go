package main

import (
	"context"
	"testing"

	"wx_channel/internal/poc"
)

func TestRunRejectsIncompleteOrUnknownCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"collect"}, {"finalize", "--request", "missing"}} {
		if got := run(context.Background(), args); got != exitRequestInvalid {
			t.Fatalf("args=%v exit=%d", args, got)
		}
	}
}

func TestExitCodeForStatusIsClosed(t *testing.T) {
	tests := map[poc.BatchStatus]int{
		poc.BatchSucceeded: 0, poc.BatchPartial: 2, poc.BatchNeedsVerification: 3, poc.BatchFailed: 4,
	}
	for status, want := range tests {
		if got := exitCodeForStatus(status); got != want {
			t.Fatalf("status=%s exit=%d", status, got)
		}
	}
	if got := exitCodeForStatus(poc.BatchStatus("unexpected")); got != exitFailed {
		t.Fatalf("unexpected status exit=%d", got)
	}
}
