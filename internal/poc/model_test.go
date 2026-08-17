package poc

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestDefaultOptionsAreFixedToApprovedSpec(t *testing.T) {
	got := DefaultOptions()
	if got.Keyword != "青云装饰" || got.Limits.Works != 10 ||
		got.Limits.TopLevelCommentsPerWork != 100 || got.Limits.RepliesPerComment != 20 ||
		got.Limits.RepliesPerWork != 200 {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if got.HumanWait.Timeout != 300*time.Second || got.HumanWait.Extension != 300*time.Second || got.HumanWait.MaxExtensions != 1 {
		t.Fatalf("unexpected human wait: %+v", got.HumanWait)
	}
	if got.RequestInterval != time.Second {
		t.Fatalf("interval=%s", got.RequestInterval)
	}
}

func TestDatasetSerializesMissingSourceFieldsAsNull(t *testing.T) {
	dataset := Dataset{
		SchemaVersion: SchemaVersion,
		Job:           Job{Status: JobCompleted},
		Comments:      []Comment{{Level: 1}},
	}
	raw, err := json.Marshal(dataset)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"comment_id":null`, `"parent_comment_id":null`, `"text":null`, `"account_id":null`} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
}

func TestValidateForRunRequiresApprovedSafetyEnvelope(t *testing.T) {
	options := DefaultOptions()
	if err := options.ValidateForRun(); err == nil {
		t.Fatal("ValidateForRun() accepted missing isolated VM acknowledgement")
	}

	options.AckIsolatedVM = true
	if err := options.ValidateForRun(); err != nil {
		t.Fatalf("ValidateForRun() rejected approved defaults: %v", err)
	}

	options.ProxyAddress = "0.0.0.0:2025"
	if err := options.ValidateForRun(); err == nil {
		t.Fatal("ValidateForRun() accepted a non-loopback listener")
	}

	options = DefaultOptions()
	options.AckIsolatedVM = true
	options.Limits.RepliesPerComment++
	if err := options.ValidateForRun(); err == nil {
		t.Fatal("ValidateForRun() accepted unapproved per-comment reply limit")
	}

	options = DefaultOptions()
	options.AckIsolatedVM = true
	options.Limits.RepliesPerWork++
	if err := options.ValidateForRun(); err == nil {
		t.Fatal("ValidateForRun() accepted unapproved limits")
	}
}
