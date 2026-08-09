package poc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type smokeEvents struct {
	mu    sync.Mutex
	items []string
}

func (e *smokeEvents) add(item string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.items = append(e.items, item)
}

func (e *smokeEvents) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.items...)
}

type smokeContainsResult struct {
	present bool
	err     error
}

type fakeCertificateSmokeStore struct {
	events     *smokeEvents
	contains   []smokeContainsResult
	installErr error
	removeErr  error
}

func (s *fakeCertificateSmokeStore) Install(context.Context, string, string) error {
	s.events.add("install")
	return s.installErr
}

func (s *fakeCertificateSmokeStore) RemoveBySHA256(context.Context, string) error {
	s.events.add("remove")
	if s.removeErr != nil {
		return s.removeErr
	}
	return nil
}

func (s *fakeCertificateSmokeStore) ContainsSHA256(context.Context, string) (bool, error) {
	s.events.add("contains")
	if len(s.contains) == 0 {
		return false, errors.New("fixture contains result missing")
	}
	result := s.contains[0]
	s.contains = s.contains[1:]
	return result.present, result.err
}

type fakeCertificateSmokeFixture struct {
	deps       CertificateSmokeDeps
	events     *smokeEvents
	store      *Store
	certStore  *fakeCertificateSmokeStore
	approval   error
	destroyErr error
}

func newFakeCertificateSmokeFixture(t *testing.T) *fakeCertificateSmokeFixture {
	t.Helper()
	events := &smokeEvents{}
	store := newTestStore(t, "certificate-smoke-test")
	certStore := &fakeCertificateSmokeStore{events: events}
	fixture := &fakeCertificateSmokeFixture{events: events, store: store, certStore: certStore}
	fixture.deps = CertificateSmokeDeps{
		Preflight: func(context.Context, Options) (PreflightReport, error) {
			events.add("preflight")
			return PreflightReport{Passed: true}, nil
		},
		IsElevated: func() (bool, error) {
			events.add("not-elevated")
			return false, nil
		},
		Approve: func(context.Context, CertificateSmokePlan) error {
			events.add("approve")
			return fixture.approval
		},
		CertStore: certStore,
		StoreFactory: func(Options, string) (*Store, error) {
			events.add("create-store")
			return store, nil
		},
		CreateCA: func(string) (*JobCA, error) {
			events.add("create-ca")
			return &JobCA{SHA256Fingerprint: strings.Repeat("A", 64), KeyPEM: []byte("fixture-key")}, nil
		},
		WriteCA: func(*JobCA, *Store) (string, string, error) {
			events.add("write-ca")
			return filepath.Join(store.SecretsDir(), "job-ca.cert"), filepath.Join(store.SecretsDir(), "job-ca.key"), nil
		},
		DestroySecrets: func(ca *JobCA, store *Store) error {
			events.add("destroy-secrets")
			if fixture.destroyErr != nil {
				return fixture.destroyErr
			}
			for i := range ca.KeyPEM {
				ca.KeyPEM[i] = 0
			}
			ca.KeyPEM = nil
			return os.Remove(store.SecretsDir())
		},
		WriteRuntimeState: func(store *Store, state PersistedRuntimeState) error {
			if state.CertificateInstalled {
				events.add("write-runtime-owned")
			} else {
				events.add("write-runtime-unowned")
			}
			return store.WriteRuntimeState(state)
		},
		RemoveRuntimeState: func(store *Store) error {
			events.add("remove-runtime")
			return store.RemoveRuntimeState()
		},
		WriteReceipt: func(store *Store, receipt CertificateSmokeReceipt) error {
			events.add("write-receipt")
			return store.WriteCertificateSmokeReceipt(receipt)
		},
		NewJobID: func() (string, error) { return "certificate-smoke-test", nil },
		Now:      func() time.Time { return time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC) },
	}
	return fixture
}

func TestCertificateSmokeInstallsVerifiesRemovesAndDestroysState(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.certStore.contains = []smokeContainsResult{{present: false}, {present: true}, {present: false}}

	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	if !receipt.Success || !receipt.PreflightPassed || !receipt.NotElevated ||
		!receipt.PreinstallAbsent || !receipt.InstallVerified || !receipt.RemoveVerified ||
		!receipt.SecretsDestroyed || !receipt.RuntimeStateDestroyed || receipt.ErrorCode != nil {
		t.Fatalf("receipt=%+v", receipt)
	}
	want := []string{
		"preflight", "not-elevated", "create-store", "create-ca", "write-ca",
		"write-runtime-unowned", "approve", "contains", "write-runtime-owned",
		"install", "contains", "remove", "contains", "write-runtime-unowned",
		"destroy-secrets", "remove-runtime", "write-receipt",
	}
	if got := fixture.events.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
	if _, err := os.Stat(filepath.Join(fixture.store.RuntimeDir(), "state.json")); !os.IsNotExist(err) {
		t.Fatalf("runtime state remains: %v", err)
	}
}

func TestCertificateSmokeRejectsElevatedProcessBeforeCreatingCA(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.deps.IsElevated = func() (bool, error) {
		fixture.events.add("elevated")
		return true, nil
	}
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	assertSmokeCode(t, receipt, smokePreflightFailed)
	if got := fixture.events.snapshot(); !reflect.DeepEqual(got, []string{"preflight", "elevated"}) {
		t.Fatalf("events=%v", got)
	}
}

func TestCertificateSmokeNeverRemovesPreexistingCertificate(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.certStore.contains = []smokeContainsResult{{present: true}}
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	assertSmokeCode(t, receipt, smokeCertificatePreexisting)
	for _, event := range fixture.events.snapshot() {
		if event == "install" || event == "remove" {
			t.Fatalf("preexisting certificate was mutated: %v", fixture.events.snapshot())
		}
	}
	if !receipt.SecretsDestroyed || !receipt.RuntimeStateDestroyed {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestCertificateSmokeRemovesCertificateAfterAmbiguousInstall(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.certStore.contains = []smokeContainsResult{{present: false}, {present: false}}
	fixture.certStore.installErr = errors.New("fixture ambiguous install")
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	assertSmokeCode(t, receipt, smokeInstallFailed)
	if !receipt.RemoveVerified || !containsEvent(fixture.events.snapshot(), "remove") {
		t.Fatalf("ambiguous install was not cleaned: receipt=%+v events=%v", receipt, fixture.events.snapshot())
	}
}

func TestCertificateSmokeMapsInstallPostcheckToVerificationFailure(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.certStore.contains = []smokeContainsResult{{present: false}, {present: false}}
	fixture.certStore.installErr = newCertificateStoreError(certificatePostcheckFailed)
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	assertSmokeCode(t, receipt, smokeInstallVerificationFailed)
}

func TestCertificateSmokeMapsPersistentRemovalToVerificationFailure(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.certStore.contains = []smokeContainsResult{{present: false}, {present: true}, {present: true}}
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	assertSmokeCode(t, receipt, smokeRemoveVerificationFailed)
	if receipt.RuntimeStateDestroyed {
		t.Fatalf("uncertain certificate cleanup removed recovery state: %+v", receipt)
	}
}

func TestCertificateSmokeCleanupFailureOverridesPrimaryFailure(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.approval = errors.New("fixture approval")
	fixture.destroyErr = errors.New("fixture secret cleanup")
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	assertSmokeCode(t, receipt, smokeSecretsCleanupFailed)
}

func TestCertificateSmokeLeavesRuntimeStateWhenCleanupIsUncertain(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.certStore.contains = []smokeContainsResult{{present: false}, {present: true}}
	fixture.certStore.removeErr = newCertificateStoreError(certificateRemoveCommandFailed)
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	assertSmokeCode(t, receipt, smokeRemoveFailed)
	if _, err := os.Stat(filepath.Join(fixture.store.RuntimeDir(), "state.json")); err != nil {
		t.Fatalf("recovery state missing: %v", err)
	}
}

func TestCertificateSmokeCleanupIsIdempotent(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.certStore.contains = []smokeContainsResult{{present: false}, {present: false}}
	fixture.certStore.installErr = errors.New("fixture install failed before add")
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	assertSmokeCode(t, receipt, smokeInstallFailed)
	if !receipt.RemoveVerified || !receipt.SecretsDestroyed || !receipt.RuntimeStateDestroyed {
		t.Fatalf("idempotent zero-match cleanup failed: %+v", receipt)
	}
}

func TestCertificateSmokeReceiptNeverContainsDependencyErrorText(t *testing.T) {
	fixture := newFakeCertificateSmokeFixture(t)
	fixture.deps.Preflight = func(context.Context, Options) (PreflightReport, error) {
		return PreflightReport{}, errors.New(`C:\private\job-ca.key token=fixture https://secret.invalid`)
	}
	receipt := NewCertificateSmoke(fixture.deps).Run(context.Background(), approvedTestOptions())
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := ScanOrdinaryOutput(raw); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private", "fixture", "secret.invalid"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("receipt leaked %q: %s", forbidden, raw)
		}
	}
}

func assertSmokeCode(t *testing.T, receipt CertificateSmokeReceipt, want certificateSmokeErrorCode) {
	t.Helper()
	if receipt.ErrorCode == nil || *receipt.ErrorCode != string(want) || receipt.Success {
		t.Fatalf("receipt=%+v want=%s", receipt, want)
	}
}

func containsEvent(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}
