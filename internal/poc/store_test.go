package poc

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreRefusesUnignoredRoot(t *testing.T) {
	repo := initTestRepository(t, "")
	_, err := NewStore(StoreOptions{RepoRoot: repo, DataRoot: "results", JobID: "job-1"})
	if err == nil || !strings.Contains(err.Error(), "git ignored") {
		t.Fatalf("err=%v", err)
	}
}

func TestStoreRejectsSecretBeforeAtomicReplace(t *testing.T) {
	store := newTestStore(t, "job-1")
	err := store.WriteJSON("dataset.json", map[string]any{"token": "secret"})
	if err == nil {
		t.Fatal("expected secret rejection")
	}
	if _, statErr := os.Stat(filepath.Join(store.JobDir(), "dataset.json")); !os.IsNotExist(statErr) {
		t.Fatalf("unsafe file exists: %v", statErr)
	}
}

func TestStoreRejectsOverlappingRoots(t *testing.T) {
	repo := initTestRepository(t, ignoredPOCRoots)
	_, err := NewStore(StoreOptions{
		RepoRoot:    repo,
		DataRoot:    ".poc-data",
		SecretsRoot: ".poc-data",
		JobID:       "job-1",
	})
	if err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("err=%v", err)
	}
}

func TestCheckpointAtomicReplaceFailurePreservesPreviousFile(t *testing.T) {
	store := newTestStore(t, "job-1")
	first := Checkpoint{SchemaVersion: SchemaVersion, JobID: "job-1", Phase: "search", SavedAt: time.Now().UTC()}
	if err := store.SaveCheckpoint(first); err != nil {
		t.Fatal(err)
	}

	replace := store.atomicReplace
	store.atomicReplace = func(string, string) error { return errors.New("injected replace failure") }
	second := first
	second.Phase = "comments"
	if err := store.SaveCheckpoint(second); err == nil {
		t.Fatal("expected atomic replace failure")
	}
	assertCheckpointPhase(t, store, "search")

	store.atomicReplace = replace
	if err := store.SaveCheckpoint(second); err != nil {
		t.Fatal(err)
	}
	assertCheckpointPhase(t, store, "comments")
}

func TestStoreWritesEvidenceReference(t *testing.T) {
	store := newTestStore(t, "job-1")
	reference, err := store.WriteEvidence(Evidence{RequestSequence: 3, Method: "finderSearch", RedactionRuleVersion: RedactionRuleVersion})
	if err != nil {
		t.Fatal(err)
	}
	if reference != "evidence/000003.json" {
		t.Fatalf("reference=%q", reference)
	}
	if _, err := os.Stat(filepath.Join(store.JobDir(), filepath.FromSlash(reference))); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteExpiredOnlyRemovesCompletedOldJobs(t *testing.T) {
	repo := initTestRepository(t, ignoredPOCRoots)
	now := time.Now().UTC()
	oldStore := newTestStoreAt(t, repo, "old-job")
	newStore := newTestStoreAt(t, repo, "new-job")
	incompleteStore := newTestStoreAt(t, repo, "incomplete-job")
	oldCompleted := now.Add(-8 * 24 * time.Hour)
	newCompleted := now.Add(-6 * 24 * time.Hour)
	if err := oldStore.WriteManifest(Manifest{SchemaVersion: SchemaVersion, JobID: "old-job", Status: JobCompleted, CompletedAt: &oldCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := newStore.WriteManifest(Manifest{SchemaVersion: SchemaVersion, JobID: "new-job", Status: JobCompleted, CompletedAt: &newCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := incompleteStore.WriteManifest(Manifest{SchemaVersion: SchemaVersion, JobID: "incomplete-job", Status: JobPartial}); err != nil {
		t.Fatal(err)
	}

	deleted, err := newStore.DeleteExpired(now, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "old-job" {
		t.Fatalf("deleted=%v", deleted)
	}
	if _, err := os.Stat(oldStore.JobDir()); !os.IsNotExist(err) {
		t.Fatalf("old job remains: %v", err)
	}
	for _, path := range []string{newStore.JobDir(), incompleteStore.JobDir()} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retained job missing: %v", err)
		}
	}
}

func TestDeleteExpiredRejectsShortRetention(t *testing.T) {
	store := newTestStore(t, "job-1")
	if _, err := store.DeleteExpired(time.Now(), 6*24*time.Hour); err == nil {
		t.Fatal("accepted retention shorter than seven days")
	}
}

func assertCheckpointPhase(t *testing.T, store *Store, want string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(store.JobDir(), "checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint Checkpoint
	if err := json.Unmarshal(raw, &checkpoint); err != nil {
		t.Fatal(err)
	}
	if checkpoint.Phase != want {
		t.Fatalf("phase=%q want=%q", checkpoint.Phase, want)
	}
}

const ignoredPOCRoots = "/.poc-data/\n/.poc-secrets/\n/.poc-runtime/\n/.poc-build/\n/var/\n"

func newTestStore(t *testing.T, jobID string) *Store {
	t.Helper()
	return newTestStoreAt(t, initTestRepository(t, ignoredPOCRoots), jobID)
}

func newTestStoreAt(t *testing.T, repo, jobID string) *Store {
	t.Helper()
	store, err := NewStore(StoreOptions{RepoRoot: repo, JobID: jobID})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func initTestRepository(t *testing.T, gitignore string) string {
	t.Helper()
	repo := t.TempDir()
	command := exec.Command("git", "init", "--quiet")
	command.Dir = repo
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(gitignore), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo
}
