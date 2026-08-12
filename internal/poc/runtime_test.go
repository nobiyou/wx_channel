package poc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type runtimeEvents struct {
	mu    sync.Mutex
	items []string
}

func (e *runtimeEvents) add(item string) {
	e.mu.Lock()
	e.items = append(e.items, item)
	e.mu.Unlock()
}

func (e *runtimeEvents) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.items...)
}

type fakeRuntimeCertStore struct{ events *runtimeEvents }

func (s fakeRuntimeCertStore) Install(context.Context, string, string) error {
	s.events.add("install-ca")
	return nil
}
func (s fakeRuntimeCertStore) RemoveBySHA256(context.Context, string) error {
	s.events.add("remove-ca")
	return nil
}
func (s fakeRuntimeCertStore) ContainsSHA256(context.Context, string) (bool, error) {
	return false, nil
}

type flakyRuntimeCertStore struct {
	events   *runtimeEvents
	mu       sync.Mutex
	removals int
}

type ambiguousInstallCertStore struct{ events *runtimeEvents }

func (s ambiguousInstallCertStore) Install(context.Context, string, string) error {
	s.events.add("install-ca")
	return newCertificateStoreError(certificateImportCommandFailed)
}
func (s ambiguousInstallCertStore) ContainsSHA256(context.Context, string) (bool, error) {
	return true, nil
}
func (s ambiguousInstallCertStore) RemoveBySHA256(context.Context, string) error {
	s.events.add("remove-ca")
	return nil
}

func (s *flakyRuntimeCertStore) Install(context.Context, string, string) error {
	s.events.add("install-ca")
	return nil
}
func (s *flakyRuntimeCertStore) RemoveBySHA256(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removals++
	s.events.add("remove-ca")
	if s.removals == 1 {
		return errors.New("fixture transient cleanup failure")
	}
	return nil
}
func (s *flakyRuntimeCertStore) ContainsSHA256(context.Context, string) (bool, error) {
	return false, nil
}

type fakeRuntimeBridge struct{}

func (fakeRuntimeBridge) WaitReady(context.Context, []string) error         { return nil }
func (fakeRuntimeBridge) Call(context.Context, string, any) ([]byte, error) { return nil, nil }
func (fakeRuntimeBridge) State() BridgeState                                { return BridgeState{} }
func (fakeRuntimeBridge) Close() error                                      { return nil }

type eventCloser struct{ events *runtimeEvents }

func (c eventCloser) Close() error { c.events.add("stop-bridge"); return nil }

type fakeRuntimeProxy struct{ events *runtimeEvents }

func (p fakeRuntimeProxy) StartListener() error    { p.events.add("start-proxy"); return nil }
func (p fakeRuntimeProxy) StartProcessRule() error { p.events.add("start-driver"); return nil }
func (p fakeRuntimeProxy) Cleanup(bool) error {
	p.events.add("remove-process-rule")
	p.events.add("stop-proxy")
	return nil
}

func fakeRuntimeDeps(t *testing.T, collect func(context.Context) error) (RuntimeDeps, *runtimeEvents) {
	t.Helper()
	events := &runtimeEvents{}
	deps := RuntimeDeps{
		Preflight: func(context.Context, Options) (PreflightReport, error) {
			events.add("preflight")
			return PreflightReport{Passed: true, SunnyDriverAbsent: true}, nil
		},
		NewJobID: func() (string, error) { return "runtime-test-job", nil },
		StoreFactory: func(options Options, jobID string) (*Store, error) {
			return newTestStore(t, jobID), nil
		},
		CreateCA: func(string) (*JobCA, error) {
			events.add("create-ca")
			return &JobCA{SHA256Fingerprint: "fixture-fingerprint", KeyPEM: []byte("fixture-key")}, nil
		},
		WriteCA: func(*JobCA, *Store) (string, string, error) {
			return "fixture-cert-path", "fixture-key-path", nil
		},
		CreateToken: func() (string, error) { return "fixture-runtime-token-value", nil },
		Approve:     func(context.Context, ChangeSummary) error { return nil },
		CertStore:   fakeRuntimeCertStore{events: events},
		BridgeStart: func(context.Context, string, string) (Bridge, io.Closer, error) {
			events.add("start-bridge")
			return fakeRuntimeBridge{}, eventCloser{events: events}, nil
		},
		ProxyFactory: func(context.Context, *JobCA, string, string) (RuntimeProxy, error) {
			return fakeRuntimeProxy{events: events}, nil
		},
		Collect: func(ctx context.Context, _ Bridge, _ *Store, _ Options) (Dataset, Validation, error) {
			events.add("collect")
			if err := collect(ctx); err != nil {
				return Dataset{}, Validation{}, err
			}
			return Dataset{Job: Job{Status: JobCompleted}}, Validation{CapabilityStatus: CapabilityVerified, CoverageStatus: CoverageTargetMet}, nil
		},
		StopRequests: func() { events.add("stop-requests") },
		DestroySecrets: func(*JobCA, *Store) error {
			events.add("destroy-secrets")
			return nil
		},
		CleanupRecorded: func(CleanupReceipt) { events.add("write-cleanup-receipt") },
		Now:             func() time.Time { return time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC) },
	}
	return deps, events
}

func TestRuntimeCleansUpInReverseOrderOnCollectorFailure(t *testing.T) {
	deps, events := fakeRuntimeDeps(t, func(context.Context) error { return errors.New("fixture collector failure") })
	var store *Store
	deps.StoreFactory = func(_ Options, jobID string) (*Store, error) {
		store = newTestStore(t, jobID)
		return store, nil
	}
	err := NewRuntime(deps).Run(context.Background(), approvedTestOptions())
	if err == nil {
		t.Fatal("expected error")
	}
	want := []string{"preflight", "create-ca", "install-ca", "start-bridge", "start-proxy", "start-driver", "collect", "stop-requests", "remove-process-rule", "stop-proxy", "stop-bridge", "remove-ca", "destroy-secrets", "write-cleanup-receipt"}
	assertRuntimeEvents(t, events.snapshot(), want)
	assertSuccessfulCleanupReceipt(t, store)
}

func TestRuntimeCleansCertificateWhenImportOutcomeIsAmbiguous(t *testing.T) {
	deps, events := fakeRuntimeDeps(t, func(context.Context) error { return nil })
	var store *Store
	deps.StoreFactory = func(_ Options, jobID string) (*Store, error) {
		store = newTestStore(t, jobID)
		return store, nil
	}
	deps.CertStore = ambiguousInstallCertStore{events: events}

	err := NewRuntime(deps).Run(context.Background(), approvedTestOptions())
	assertCertificateErrorCode(t, err, certificateImportCommandFailed)
	assertRuntimeEvents(t, events.snapshot(), []string{
		"preflight", "create-ca", "install-ca", "stop-requests", "remove-ca",
		"destroy-secrets", "write-cleanup-receipt",
	})
	assertSuccessfulCleanupReceipt(t, store)
}

func assertSuccessfulCleanupReceipt(t *testing.T, store *Store) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(store.JobDir(), "cleanup-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt CleanupReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Success {
		t.Fatalf("cleanup receipt was not successful: %+v", receipt)
	}
	for _, category := range []string{
		"requests_stopped", "process_rule_removed", "proxy_stopped", "bridge_stopped",
		"driver_removed", "certificate_removed", "secrets_destroyed", "encrypted_raw_destroyed",
	} {
		if !receipt.Categories[category] {
			t.Errorf("cleanup category %q was not successful", category)
		}
	}
	if _, err := os.Stat(filepath.Join(store.JobDir(), "cleanup_receipt.json")); !os.IsNotExist(err) {
		t.Errorf("legacy cleanup receipt filename exists or could not be inspected: %v", err)
	}
}

func TestRunRefusesWithoutAckOrInteractiveApply(t *testing.T) {
	deps, events := fakeRuntimeDeps(t, func(context.Context) error { return nil })
	runtime := NewRuntime(deps)
	options := DefaultOptions()
	if err := runtime.Run(context.Background(), options); err == nil {
		t.Fatal("missing isolated VM acknowledgement accepted")
	}
	if len(events.snapshot()) != 0 {
		t.Fatalf("resources touched before acknowledgement: %v", events.snapshot())
	}

	deps, events = fakeRuntimeDeps(t, func(context.Context) error { return nil })
	deps.Approve = func(context.Context, ChangeSummary) error { return errors.New("APPLY not entered") }
	if err := NewRuntime(deps).Run(context.Background(), approvedTestOptions()); err == nil {
		t.Fatal("missing APPLY accepted")
	}
	for _, event := range events.snapshot() {
		if event == "install-ca" || event == "start-bridge" || event == "start-proxy" || event == "start-driver" {
			t.Fatalf("resource changed before APPLY: %v", events.snapshot())
		}
	}
}

func TestCleanupTwiceIsIdempotent(t *testing.T) {
	deps, events := fakeRuntimeDeps(t, func(context.Context) error { return errors.New("fixture failure") })
	runtime := NewRuntime(deps)
	if err := runtime.Run(context.Background(), approvedTestOptions()); err == nil {
		t.Fatal("expected error")
	}
	before := events.snapshot()
	if err := runtime.Cleanup(context.Background(), "runtime-test-job"); err != nil {
		t.Fatal(err)
	}
	assertRuntimeEvents(t, events.snapshot(), before)
}

func TestCleanupFailureCanBeRetriedInSameProcess(t *testing.T) {
	deps, events := fakeRuntimeDeps(t, func(context.Context) error { return errors.New("fixture collection failure") })
	certStore := &flakyRuntimeCertStore{events: events}
	deps.CertStore = certStore
	runtime := NewRuntime(deps)
	if err := runtime.Run(context.Background(), approvedTestOptions()); err == nil {
		t.Fatal("expected initial cleanup failure")
	}
	if err := runtime.Cleanup(context.Background(), "runtime-test-job"); err != nil {
		t.Fatal(err)
	}
	if certStore.removals != 2 {
		t.Fatalf("certificate cleanup attempts=%d", certStore.removals)
	}
}

func TestSignalUsesSameCleanupPath(t *testing.T) {
	started := make(chan struct{})
	deps, events := fakeRuntimeDeps(t, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	runtime := NewRuntime(deps)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx, approvedTestOptions()) }()
	<-started
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected cancellation")
	}
	got := events.snapshot()
	wantSuffix := []string{"stop-requests", "remove-process-rule", "stop-proxy", "stop-bridge", "remove-ca", "destroy-secrets", "write-cleanup-receipt"}
	if len(got) < len(wantSuffix) {
		t.Fatalf("events=%v", got)
	}
	assertRuntimeEvents(t, got[len(got)-len(wantSuffix):], wantSuffix)
}

func TestPersistedCleanupUsesExactCertificateAndDriverOwnership(t *testing.T) {
	store := newTestStore(t, "persisted-cleanup-job")
	state := PersistedRuntimeState{
		SchemaVersion: SchemaVersion, JobID: "persisted-cleanup-job", CertificateSHA256: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		CertificateInstalled: true, DriverStarted: true, DriverInstalledByJob: true,
	}
	if err := store.WriteRuntimeState(state); err != nil {
		t.Fatal(err)
	}
	events := &runtimeEvents{}
	certStore := fakeRuntimeCertStore{events: events}
	err := cleanupPersistedRuntime(context.Background(), store, state, certStore, func(unregister bool) error {
		if !unregister {
			t.Fatal("job-owned driver was not marked for unregister")
		}
		events.add("stop-driver")
		return nil
	}, func(*JobCA, *Store) error {
		events.add("destroy-secrets")
		return nil
	}, time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeEvents(t, events.snapshot(), []string{"stop-driver", "remove-ca", "destroy-secrets"})
	if _, err := store.LoadRuntimeState(); err == nil {
		t.Fatal("runtime state remained after successful cleanup")
	}
	assertSuccessfulCleanupReceipt(t, store)
}

func assertRuntimeEvents(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("events=%v want=%v", got, want)
		}
	}
}
