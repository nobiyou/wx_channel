package poc

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestObserveRawStoresStructureNotValues(t *testing.T) {
	recorder := NewEvidenceRecorder(nil)
	ev, err := recorder.Observe(1, "finderGetCommentList", []byte(`{"data":{"commentInfo":[{"commentId":"c-secret","content":"正文"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("c-secret")) || bytes.Contains(encoded, []byte("正文")) {
		t.Fatalf("raw value leaked: %s", encoded)
	}
	if ev.SourceResponseSHA256 == "" || ev.RecordCount != 1 {
		t.Fatalf("bad evidence: %+v", ev)
	}
	if len(ev.Fields) == 0 {
		t.Fatal("schema fields are empty")
	}
}

func TestObserveRejectsUnsafeMethodWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	store, err := NewEncryptedRawStore(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	recorder := NewEvidenceRecorder(store)
	if _, err := recorder.Observe(1, "method?token=secret", []byte(`{"ok":true}`)); err == nil {
		t.Fatal("Observe() accepted an unsafe evidence method")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unsafe observation wrote files: %v", entries)
	}
}

func TestEncryptedRawStoreContainsNoPlaintextAndDestroysKey(t *testing.T) {
	dir := t.TempDir()
	store, err := NewEncryptedRawStore(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"commentId":"c-secret","content":"正文"}`)
	if err := store.Write(7, raw); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "000007.enc")
	ciphertext, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("c-secret")) || bytes.Contains(ciphertext, []byte("正文")) {
		t.Fatalf("plaintext leaked into ciphertext: %q", ciphertext)
	}
	if len(ciphertext) <= len(raw) {
		t.Fatalf("ciphertext too short: %d", len(ciphertext))
	}

	key := store.key
	if err := store.Destroy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("encrypted evidence remains: %v", err)
	}
	for i, b := range key {
		if b != 0 {
			t.Fatalf("key byte %d was not zeroed", i)
		}
	}
}

func TestEncryptedRawStoreDisabled(t *testing.T) {
	store, err := NewEncryptedRawStore(t.TempDir(), false)
	if err != nil || store != nil {
		t.Fatalf("store=%v err=%v", store, err)
	}
}
