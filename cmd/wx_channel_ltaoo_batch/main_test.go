package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"testing"

	"wx_channel/internal/poc"
)

func TestRunRejectsIncompleteOrUnknownCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"collect"}, {"finalize", "--request", "missing"}} {
		if got := run(context.Background(), args, io.Discard); got != exitRequestInvalid {
			t.Fatalf("args=%v exit=%d", args, got)
		}
	}
}

func TestCapabilitiesAdvertiseOnlyTheGenericRouterProtocol(t *testing.T) {
	var output bytes.Buffer
	if got := run(context.Background(), []string{"capabilities"}, &output); got != exitSucceeded {
		t.Fatalf("capabilities exit=%d", got)
	}
	var value struct {
		SchemaVersion    int      `json:"schema_version"`
		RuntimeProtocols []string `json:"runtime_protocols"`
		RouterKinds      []string `json:"router_kinds"`
	}
	if err := json.Unmarshal(output.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != 1 ||
		!reflect.DeepEqual(value.RuntimeProtocols, []string{"wechat-channels-local-runtime-v2"}) ||
		!reflect.DeepEqual(value.RouterKinds, []string{"mihomo"}) {
		t.Fatalf("unexpected capabilities: %#v", value)
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
