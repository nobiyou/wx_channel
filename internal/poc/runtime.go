package poc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var runtimeFingerprintPattern = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)

type RuntimeProxy interface {
	StartListener() error
	StartProcessRule() error
	Cleanup(unregisterDriver bool) error
}

type ChangeSummary struct {
	CertificateScope string
	ProxyAddress     string
	BridgeAddress    string
	ProcessName      string
}

type PersistedRuntimeState struct {
	SchemaVersion        string `json:"schema_version"`
	JobID                string `json:"job_id"`
	CertificateSHA256    string `json:"certificate_sha256"`
	CertificateInstalled bool   `json:"certificate_installed"`
	BridgeStarted        bool   `json:"bridge_started"`
	ProxyStarted         bool   `json:"proxy_started"`
	DriverStarted        bool   `json:"driver_started"`
	DriverInstalledByJob bool   `json:"driver_installed_by_job"`
}

type RuntimeDeps struct {
	Preflight       func(context.Context, Options) (PreflightReport, error)
	CreateCA        func(string) (*JobCA, error)
	WriteCA         func(*JobCA, *Store) (string, string, error)
	CreateToken     func() (string, error)
	Approve         func(context.Context, ChangeSummary) error
	CertStore       CertificateStore
	BridgeStart     func(context.Context, string, string) (Bridge, io.Closer, error)
	ProxyFactory    func(context.Context, *JobCA, string, string) (RuntimeProxy, error)
	Collect         func(context.Context, Bridge, *Store, Options) (Dataset, Validation, error)
	StoreFactory    func(Options, string) (*Store, error)
	NewJobID        func() (string, error)
	Now             func() time.Time
	StopRequests    func()
	DestroySecrets  func(*JobCA, *Store) error
	CleanupRecorded func(CleanupReceipt)
	Logger          SafeLogger
	LoggerFactory   func(*Store) (SafeLogger, error)
}

type runtimeState struct {
	jobID                string
	store                *Store
	ca                   *JobCA
	certInstalled        bool
	bridgeCloser         io.Closer
	proxy                RuntimeProxy
	driverInstalledByJob bool
	cancelRequests       context.CancelFunc
	manifest             *Manifest
	persisted            PersistedRuntimeState
	cleaned              bool
	mu                   sync.Mutex
}

type Runtime struct {
	deps RuntimeDeps
	mu   sync.Mutex
	jobs map[string]*runtimeState
	done map[string]struct{}
}

func NewRuntime(deps RuntimeDeps) *Runtime {
	if deps.Logger == nil {
		deps.Logger = DiscardLogger{}
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	if deps.NewJobID == nil {
		deps.NewJobID = newRuntimeJobID
	}
	if deps.CreateToken == nil {
		deps.CreateToken = newRuntimeToken
	}
	if deps.WriteCA == nil {
		deps.WriteCA = func(ca *JobCA, store *Store) (string, string, error) {
			return ca.WriteSecrets(store.SecretsDir())
		}
	}
	if deps.DestroySecrets == nil {
		deps.DestroySecrets = destroyRuntimeSecrets
	}
	return &Runtime{deps: deps, jobs: make(map[string]*runtimeState), done: make(map[string]struct{})}
}

func (r *Runtime) Run(ctx context.Context, options Options) (runErr error) {
	if r == nil {
		return errors.New("POC runtime is nil")
	}
	if err := options.ValidateForRun(); err != nil {
		return err
	}
	if err := r.validateDeps(); err != nil {
		return err
	}
	report, err := r.deps.Preflight(ctx, options)
	if err != nil || !report.Passed {
		return ErrPreflightFailed
	}
	_ = r.deps.Logger.Event("preflight_completed", map[string]any{"phase": "preflight"})

	jobID, err := r.deps.NewJobID()
	if err != nil || !validJobID(jobID) {
		return errors.New("create valid POC job ID")
	}
	store, err := r.deps.StoreFactory(options, jobID)
	if err != nil {
		return err
	}
	state := &runtimeState{jobID: jobID, store: store}
	state.persisted = PersistedRuntimeState{SchemaVersion: SchemaVersion, JobID: jobID}
	if r.deps.LoggerFactory != nil {
		logger, loggerErr := r.deps.LoggerFactory(store)
		if loggerErr != nil {
			return loggerErr
		}
		r.deps.Logger = logger
	}
	r.mu.Lock()
	r.jobs[jobID] = state
	r.mu.Unlock()
	defer func() {
		cleanupErr := r.cleanupState(context.Background(), state)
		if cleanupErr != nil {
			runErr = errors.Join(runErr, cleanupErr)
		}
	}()

	ca, err := r.deps.CreateCA(jobID)
	if err != nil {
		return err
	}
	state.ca = ca
	state.persisted.CertificateSHA256 = ca.SHA256Fingerprint
	certPath, _, err := r.deps.WriteCA(ca, store)
	if err != nil {
		return err
	}
	if err := state.persist(); err != nil {
		return err
	}
	token, err := r.deps.CreateToken()
	if err != nil {
		return err
	}
	if err := r.deps.Approve(ctx, ChangeSummary{
		CertificateScope: "CurrentUser\\Root", ProxyAddress: options.ProxyAddress,
		BridgeAddress: options.BridgeAddress, ProcessName: weChatProcessName,
	}); err != nil {
		return err
	}
	_ = r.deps.Logger.Event("approval_received", map[string]any{"phase": "approval"})

	if err := r.deps.CertStore.Install(ctx, certPath, ca.SHA256Fingerprint); err != nil {
		if present, checkErr := r.deps.CertStore.ContainsSHA256(ctx, ca.SHA256Fingerprint); checkErr == nil && present {
			state.certInstalled = true
			state.persisted.CertificateInstalled = true
			_ = state.persist()
		}
		return err
	}
	state.certInstalled = true
	state.persisted.CertificateInstalled = true
	if err := state.persist(); err != nil {
		return err
	}
	_ = r.deps.Logger.Event("ca_installed", map[string]any{"phase": "certificate"})

	bridge, closer, err := r.deps.BridgeStart(ctx, options.BridgeAddress, token)
	if err != nil {
		return err
	}
	state.bridgeCloser = closer
	state.persisted.BridgeStarted = true
	if err := state.persist(); err != nil {
		return err
	}
	_ = r.deps.Logger.Event("bridge_started", map[string]any{"phase": "bridge"})

	proxy, err := r.deps.ProxyFactory(ctx, ca, options.ProxyAddress, token)
	if err != nil {
		return err
	}
	state.proxy = proxy
	if err := proxy.StartListener(); err != nil {
		return err
	}
	state.persisted.ProxyStarted = true
	if err := state.persist(); err != nil {
		return err
	}
	_ = r.deps.Logger.Event("proxy_started", map[string]any{"phase": "proxy"})
	state.driverInstalledByJob = report.SunnyDriverAbsent
	state.persisted.DriverInstalledByJob = report.SunnyDriverAbsent
	if err := proxy.StartProcessRule(); err != nil {
		state.persisted.DriverStarted = true
		_ = state.persist()
		return err
	}
	state.persisted.DriverStarted = true
	if err := state.persist(); err != nil {
		return err
	}
	_ = r.deps.Logger.Event("driver_started", map[string]any{"phase": "driver"})

	collectionCtx, cancel := context.WithCancel(ctx)
	state.cancelRequests = cancel
	_ = r.deps.Logger.Event("collection_started", map[string]any{"phase": "collection"})
	dataset, validation, err := r.deps.Collect(collectionCtx, bridge, store, options)
	startedAt := r.deps.Now().UTC()
	dataset.SchemaVersion = SchemaVersion
	dataset.Job.JobID = jobID
	dataset.Job.Keyword = options.Keyword
	dataset.Job.Limits = options.Limits
	if dataset.Job.StartedAt.IsZero() {
		dataset.Job.StartedAt = startedAt
	}
	validation.JobID = jobID
	if err != nil && dataset.Job.Status == "" {
		dataset.Job.Status = JobPartial
	}
	if err != nil && validation.CapabilityStatus == "" {
		validation.CapabilityStatus = CapabilityInconclusive
	}
	if validation.CoverageStatus == "" {
		validation.CoverageStatus = CoverageIncomplete
	}
	if err := store.WriteDataset(dataset); err != nil {
		return err
	}
	if err := store.WriteValidation(validation); err != nil {
		return err
	}
	manifest := manifestFor(dataset, validation, false, r.deps.Now().UTC())
	if err := store.WriteManifest(manifest); err != nil {
		return err
	}
	state.manifest = &manifest
	return err
}

func (r *Runtime) Cleanup(ctx context.Context, jobID string) error {
	if r == nil || !validJobID(jobID) {
		return errors.New("invalid cleanup job ID")
	}
	r.mu.Lock()
	state := r.jobs[jobID]
	_, alreadyDone := r.done[jobID]
	r.mu.Unlock()
	if alreadyDone {
		return nil
	}
	if state == nil {
		return errors.New("cleanup job is not active in this process")
	}
	return r.cleanupState(ctx, state)
}

func (r *Runtime) cleanupState(ctx context.Context, state *runtimeState) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.cleaned {
		return nil
	}
	_ = r.deps.Logger.Event("cleanup_started", map[string]any{"phase": "cleanup"})
	var cleanupErrors []error
	proxyClean := !state.persisted.ProxyStarted
	bridgeClean := !state.persisted.BridgeStarted
	driverClean := !state.persisted.DriverStarted
	certificateClean := !state.persisted.CertificateInstalled
	secretsClean := state.ca == nil
	encryptedRawClean := state.ca == nil
	if state.cancelRequests != nil {
		state.cancelRequests()
	}
	if r.deps.StopRequests != nil {
		r.deps.StopRequests()
	}
	if state.proxy != nil {
		if err := state.proxy.Cleanup(state.driverInstalledByJob); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			state.persisted.ProxyStarted = false
			state.persisted.DriverStarted = false
			proxyClean = true
			driverClean = true
		}
	}
	if state.bridgeCloser != nil {
		if err := state.bridgeCloser.Close(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			state.persisted.BridgeStarted = false
			bridgeClean = true
		}
	}
	if state.certInstalled && state.ca != nil {
		if err := r.deps.CertStore.RemoveBySHA256(ctx, state.ca.SHA256Fingerprint); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			state.persisted.CertificateInstalled = false
			state.certInstalled = false
			certificateClean = true
		}
	}
	if state.ca != nil && state.store != nil {
		if err := r.deps.DestroySecrets(state.ca, state.store); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			secretsClean = true
			encryptedRawClean = true
		}
	}
	success := len(cleanupErrors) == 0
	receipt := CleanupReceipt{
		JobID: state.jobID, Success: success,
		Categories: map[string]bool{
			"requests_stopped":        true,
			"process_rule_removed":    proxyClean,
			"proxy_stopped":           proxyClean,
			"bridge_stopped":          bridgeClean,
			"driver_removed":          driverClean,
			"certificate_removed":     certificateClean,
			"secrets_destroyed":       secretsClean,
			"encrypted_raw_destroyed": encryptedRawClean,
		},
		CompletedAt: r.deps.Now().UTC(),
	}
	if !success {
		receipt.ReasonCodes = []string{"cleanup_failed"}
	}
	if state.store != nil {
		if err := state.store.WriteCleanupReceipt(receipt); err != nil {
			cleanupErrors = append(cleanupErrors, err)
			receipt.Success = false
		}
	}
	finalSuccess := len(cleanupErrors) == 0
	if state.store != nil && state.manifest != nil {
		finalManifest := *state.manifest
		finalManifest.CleanupSuccess = finalSuccess
		if !finalSuccess {
			finalManifest.Status = JobFailed
			finalManifest.CapabilityStatus = CapabilityFailed
			finalManifest.ReasonCodes = append(finalManifest.ReasonCodes, "cleanup_failed")
		}
		completed := r.deps.Now().UTC()
		finalManifest.CompletedAt = &completed
		if err := state.store.WriteManifest(finalManifest); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if r.deps.CleanupRecorded != nil {
		r.deps.CleanupRecorded(receipt)
	}
	if len(cleanupErrors) == 0 && state.store != nil {
		if err := state.store.RemoveRuntimeState(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	} else if state.store != nil {
		_ = state.persist()
	}
	state.cleaned = len(cleanupErrors) == 0
	if state.cleaned {
		r.mu.Lock()
		delete(r.jobs, state.jobID)
		r.done[state.jobID] = struct{}{}
		r.mu.Unlock()
	}
	_ = r.deps.Logger.Event("cleanup_completed", map[string]any{"phase": "cleanup"})
	return errors.Join(cleanupErrors...)
}

func (s *runtimeState) persist() error {
	if s == nil || s.store == nil {
		return errors.New("runtime state store is missing")
	}
	return s.store.WriteRuntimeState(s.persisted)
}

func (r *Runtime) validateDeps() error {
	if r.deps.Preflight == nil || r.deps.CreateCA == nil || r.deps.Approve == nil || r.deps.CertStore == nil ||
		r.deps.BridgeStart == nil || r.deps.ProxyFactory == nil || r.deps.Collect == nil || r.deps.StoreFactory == nil {
		return errors.New("POC runtime dependency is missing")
	}
	return nil
}

func manifestFor(dataset Dataset, validation Validation, cleanup bool, completed time.Time) Manifest {
	counts := Counts{Works: len(dataset.Works)}
	for _, comment := range dataset.Comments {
		if comment.Level == 1 {
			counts.TopLevelComments++
		} else if comment.Level == 2 {
			counts.Replies++
		}
	}
	return Manifest{
		SchemaVersion: SchemaVersion, JobID: dataset.Job.JobID, Status: dataset.Job.Status,
		CapabilityStatus: validation.CapabilityStatus, CoverageStatus: validation.CoverageStatus,
		Counts: counts, CleanupSuccess: cleanup, CompletedAt: &completed, ReasonCodes: append([]string(nil), validation.ReasonCodes...),
	}
}

func newRuntimeJobID() (string, error) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", errors.New("generate job ID")
	}
	return fmt.Sprintf("poc-%s-%x", time.Now().UTC().Format("20060102T150405"), random), nil
}

func newRuntimeToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", errors.New("generate bridge token")
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func destroyRuntimeSecrets(ca *JobCA, store *Store) error {
	if ca == nil || store == nil {
		return errors.New("secret cleanup dependency is missing")
	}
	directory, err := existingCanonicalDirectory(store.SecretsDir())
	if os.IsNotExist(err) {
		for index := range ca.KeyPEM {
			ca.KeyPEM[index] = 0
		}
		ca.KeyPEM = nil
		return nil
	}
	if err != nil || !samePath(directory, store.SecretsDir()) || filepath.Base(filepath.Dir(directory)) != ".poc-secrets" {
		return errors.New("secret cleanup directory failed validation")
	}
	rawDirectory := filepath.Join(directory, "raw-evidence")
	if info, statErr := os.Lstat(rawDirectory); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("encrypted evidence cleanup target is not a directory")
		}
		entries, readErr := os.ReadDir(rawDirectory)
		if readErr != nil {
			return errors.New("list encrypted evidence cleanup directory")
		}
		for _, entry := range entries {
			if entry.IsDir() || !isEncryptedEvidenceName(entry.Name()) {
				return errors.New("unexpected encrypted evidence cleanup entry")
			}
			if err := os.Remove(filepath.Join(rawDirectory, entry.Name())); err != nil {
				return errors.New("remove encrypted evidence cleanup entry")
			}
		}
		if err := os.Remove(rawDirectory); err != nil {
			return errors.New("remove encrypted evidence cleanup directory")
		}
	} else if !os.IsNotExist(statErr) {
		return errors.New("inspect encrypted evidence cleanup directory")
	}
	for _, name := range []string{"job-ca.pem", "job-ca.key"} {
		path := filepath.Join(directory, name)
		if info, statErr := os.Lstat(path); statErr == nil {
			if !info.Mode().IsRegular() {
				return errors.New("secret cleanup target is not a regular file")
			}
			if err := os.Remove(path); err != nil {
				return errors.New("remove job secret")
			}
		} else if !os.IsNotExist(statErr) {
			return errors.New("inspect job secret")
		}
	}
	for index := range ca.KeyPEM {
		ca.KeyPEM[index] = 0
	}
	ca.KeyPEM = nil
	if err := os.Remove(directory); err != nil && !os.IsNotExist(err) {
		return errors.New("remove empty secret directory")
	}
	return nil
}

func cleanupPersistedRuntime(ctx context.Context, store *Store, state PersistedRuntimeState, certStore CertificateStore, stopDriver func(bool) error, destroy func(*JobCA, *Store) error, now time.Time) error {
	if store == nil || certStore == nil || state.SchemaVersion != SchemaVersion || state.JobID != filepath.Base(store.RuntimeDir()) || !validJobID(state.JobID) {
		return errors.New("persisted runtime state identity mismatch")
	}
	if state.CertificateInstalled && !runtimeFingerprintPattern.MatchString(state.CertificateSHA256) {
		return errors.New("persisted certificate fingerprint is invalid")
	}
	var cleanupErrors []error
	driverClean := !state.DriverStarted
	certificateClean := !state.CertificateInstalled
	secretsClean := false
	if state.DriverStarted {
		if stopDriver == nil {
			cleanupErrors = append(cleanupErrors, errors.New("persisted driver cleanup is unavailable"))
		} else if err := stopDriver(state.DriverInstalledByJob); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			state.DriverStarted = false
			state.ProxyStarted = false
			driverClean = true
		}
	}
	if state.CertificateInstalled {
		if err := certStore.RemoveBySHA256(ctx, state.CertificateSHA256); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			state.CertificateInstalled = false
			certificateClean = true
		}
	}
	ca := &JobCA{SHA256Fingerprint: state.CertificateSHA256}
	if destroy == nil {
		destroy = destroyRuntimeSecrets
	}
	if err := destroy(ca, store); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	} else {
		secretsClean = true
	}
	state.ProxyStarted = false
	state.BridgeStarted = false
	if len(cleanupErrors) != 0 {
		_ = store.WriteRuntimeState(state)
	}
	receipt := CleanupReceipt{
		JobID: state.JobID, Success: len(cleanupErrors) == 0,
		Categories: map[string]bool{
			"requests_stopped":        true,
			"process_rule_removed":    driverClean,
			"proxy_stopped":           !state.ProxyStarted,
			"bridge_stopped":          !state.BridgeStarted,
			"driver_removed":          driverClean,
			"certificate_removed":     certificateClean,
			"secrets_destroyed":       secretsClean,
			"encrypted_raw_destroyed": secretsClean,
		},
		CompletedAt: now.UTC(),
	}
	if !receipt.Success {
		receipt.ReasonCodes = []string{"cleanup_failed"}
	}
	if err := store.WriteCleanupReceipt(receipt); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	}
	if len(cleanupErrors) == 0 {
		if err := store.RemoveRuntimeState(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}
