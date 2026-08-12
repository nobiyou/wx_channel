package poc

import (
	"context"
	"errors"
	"time"
)

type CertificateSmokePlan struct {
	CertificateScope string
}

type CertificateSmokeDeps struct {
	Preflight          func(context.Context, Options) (PreflightReport, error)
	IsElevated         func() (bool, error)
	Approve            func(context.Context, CertificateSmokePlan) error
	CertStore          CertificateStore
	StoreFactory       func(Options, string) (*Store, error)
	CreateCA           func(string) (*JobCA, error)
	WriteCA            func(*JobCA, *Store) (string, string, error)
	DestroySecrets     func(*JobCA, *Store) error
	WriteRuntimeState  func(*Store, PersistedRuntimeState) error
	RemoveRuntimeState func(*Store) error
	WriteReceipt       func(*Store, CertificateSmokeReceipt) error
	NewJobID           func() (string, error)
	Now                func() time.Time
}

type CertificateSmoke struct {
	deps CertificateSmokeDeps
}

func NewCertificateSmoke(deps CertificateSmokeDeps) *CertificateSmoke {
	if deps.NewJobID == nil {
		deps.NewJobID = newRuntimeJobID
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	if deps.CreateCA == nil {
		deps.CreateCA = func(jobID string) (*JobCA, error) {
			return GenerateJobCA(jobID, deps.Now())
		}
	}
	if deps.WriteCA == nil {
		deps.WriteCA = func(ca *JobCA, store *Store) (string, string, error) {
			return ca.WriteSecrets(store.SecretsDir())
		}
	}
	if deps.DestroySecrets == nil {
		deps.DestroySecrets = destroyRuntimeSecrets
	}
	if deps.WriteRuntimeState == nil {
		deps.WriteRuntimeState = func(store *Store, state PersistedRuntimeState) error {
			return store.WriteRuntimeState(state)
		}
	}
	if deps.RemoveRuntimeState == nil {
		deps.RemoveRuntimeState = func(store *Store) error { return store.RemoveRuntimeState() }
	}
	if deps.WriteReceipt == nil {
		deps.WriteReceipt = func(store *Store, receipt CertificateSmokeReceipt) error {
			return store.WriteCertificateSmokeReceipt(receipt)
		}
	}
	return &CertificateSmoke{deps: deps}
}

func (s *CertificateSmoke) Run(ctx context.Context, options Options) (receipt CertificateSmokeReceipt) {
	receipt.SchemaVersion = CertificateSmokeSchemaVersion
	if s == nil {
		setCertificateSmokeReceiptCode(&receipt, smokePreflightFailed)
		receipt.CompletedAt = time.Now().UTC()
		return receipt
	}
	finishEarly := func(code certificateSmokeErrorCode) CertificateSmokeReceipt {
		setCertificateSmokeReceiptCode(&receipt, code)
		receipt.CompletedAt = s.deps.Now().UTC()
		return receipt
	}
	if err := options.ValidateForRun(); err != nil || s.missingRequiredDeps() {
		return finishEarly(smokePreflightFailed)
	}
	report, err := s.deps.Preflight(ctx, options)
	if err != nil || !report.Passed {
		return finishEarly(smokePreflightFailed)
	}
	receipt.PreflightPassed = true
	elevated, err := s.deps.IsElevated()
	if err != nil || elevated {
		return finishEarly(smokePreflightFailed)
	}
	receipt.NotElevated = true

	jobID, err := s.deps.NewJobID()
	if err != nil || !validJobID(jobID) {
		return finishEarly(smokePreflightFailed)
	}
	receipt.JobID = jobID
	store, err := s.deps.StoreFactory(options, jobID)
	if err != nil || store == nil {
		return finishEarly(smokePreflightFailed)
	}
	ca, err := s.deps.CreateCA(jobID)
	if err != nil || ca == nil || !runtimeFingerprintPattern.MatchString(ca.SHA256Fingerprint) {
		return finishEarly(smokePreflightFailed)
	}

	state := PersistedRuntimeState{
		SchemaVersion:     SchemaVersion,
		JobID:             jobID,
		CertificateSHA256: ca.SHA256Fingerprint,
	}
	owned := false
	primaryCode := certificateSmokeErrorCode("")
	defer func() {
		s.finalize(context.Background(), store, ca, state, owned, primaryCode, &receipt)
	}()

	certPath, _, err := s.deps.WriteCA(ca, store)
	if err != nil {
		primaryCode = smokeSecretsCleanupFailed
		return receipt
	}
	if err := s.deps.WriteRuntimeState(store, state); err != nil {
		primaryCode = smokeSecretsCleanupFailed
		return receipt
	}
	if err := s.deps.Approve(ctx, CertificateSmokePlan{CertificateScope: "CurrentUser\\Root"}); err != nil {
		primaryCode = smokeApprovalRejected
		return receipt
	}
	present, err := s.deps.CertStore.ContainsSHA256(ctx, ca.SHA256Fingerprint)
	if err != nil {
		primaryCode = smokePreflightFailed
		return receipt
	}
	if present {
		primaryCode = smokeCertificatePreexisting
		return receipt
	}
	receipt.PreinstallAbsent = true
	state.CertificateInstalled = true
	if err := s.deps.WriteRuntimeState(store, state); err != nil {
		primaryCode = smokeSecretsCleanupFailed
		return receipt
	}
	owned = true
	if err := s.deps.CertStore.Install(ctx, certPath, ca.SHA256Fingerprint); err != nil {
		primaryCode = certificateSmokeInstallErrorCode(err)
		return receipt
	}
	present, err = s.deps.CertStore.ContainsSHA256(ctx, ca.SHA256Fingerprint)
	if err != nil || !present {
		primaryCode = smokeInstallVerificationFailed
		return receipt
	}
	receipt.InstallVerified = true
	return receipt
}

func (s *CertificateSmoke) missingRequiredDeps() bool {
	return s.deps.Preflight == nil || s.deps.IsElevated == nil || s.deps.Approve == nil ||
		s.deps.CertStore == nil || s.deps.StoreFactory == nil || s.deps.CreateCA == nil ||
		s.deps.WriteCA == nil || s.deps.DestroySecrets == nil || s.deps.WriteRuntimeState == nil ||
		s.deps.RemoveRuntimeState == nil || s.deps.WriteReceipt == nil || s.deps.NewJobID == nil || s.deps.Now == nil
}

func (s *CertificateSmoke) finalize(
	ctx context.Context,
	store *Store,
	ca *JobCA,
	state PersistedRuntimeState,
	owned bool,
	primaryCode certificateSmokeErrorCode,
	receipt *CertificateSmokeReceipt,
) {
	removeCode := certificateSmokeErrorCode("")
	cleanupCode := certificateSmokeErrorCode("")
	certificateAbsent := !owned
	if owned {
		if err := s.deps.CertStore.RemoveBySHA256(ctx, ca.SHA256Fingerprint); err != nil {
			removeCode = certificateSmokeRemoveErrorCode(err)
		} else {
			present, err := s.deps.CertStore.ContainsSHA256(ctx, ca.SHA256Fingerprint)
			if err != nil || present {
				removeCode = smokeRemoveVerificationFailed
			} else {
				certificateAbsent = true
				receipt.RemoveVerified = true
				state.CertificateInstalled = false
				if err := s.deps.WriteRuntimeState(store, state); err != nil {
					cleanupCode = smokeSecretsCleanupFailed
				}
			}
		}
	}
	if err := s.deps.DestroySecrets(ca, store); err != nil {
		cleanupCode = smokeSecretsCleanupFailed
	} else {
		receipt.SecretsDestroyed = true
	}
	if receipt.SecretsDestroyed && (!owned || certificateAbsent) {
		if err := s.deps.RemoveRuntimeState(store); err != nil {
			cleanupCode = smokeSecretsCleanupFailed
		} else {
			receipt.RuntimeStateDestroyed = true
		}
	}
	finalCode := selectCertificateSmokeCode(primaryCode, removeCode, cleanupCode)
	setCertificateSmokeReceiptCode(receipt, finalCode)
	receipt.CompletedAt = s.deps.Now().UTC()
	receipt.Success = receipt.PreflightPassed && receipt.NotElevated && receipt.PreinstallAbsent &&
		receipt.InstallVerified && receipt.RemoveVerified && receipt.SecretsDestroyed &&
		receipt.RuntimeStateDestroyed && receipt.ErrorCode == nil
	if err := s.deps.WriteReceipt(store, *receipt); err != nil {
		receipt.Success = false
		if removeCode == "" {
			setCertificateSmokeReceiptCode(receipt, smokeSecretsCleanupFailed)
		}
		_ = s.deps.WriteReceipt(store, *receipt)
	}
}

func selectCertificateSmokeCode(primary, removal, cleanup certificateSmokeErrorCode) certificateSmokeErrorCode {
	if removal == smokeRemoveVerificationFailed {
		return removal
	}
	if removal == smokeRemoveFailed {
		return removal
	}
	if cleanup != "" {
		return cleanup
	}
	return primary
}

func certificateSmokeInstallErrorCode(err error) certificateSmokeErrorCode {
	var target certificateStoreError
	if errors.As(err, &target) {
		switch target.code {
		case certificateImportReportedFalse, certificatePostcheckFailed:
			return smokeInstallVerificationFailed
		}
	}
	return smokeInstallFailed
}

func certificateSmokeRemoveErrorCode(err error) certificateSmokeErrorCode {
	var target certificateStoreError
	if errors.As(err, &target) {
		switch target.code {
		case certificateRemoveReportedFalse, certificateRemovePostcheckFailed:
			return smokeRemoveVerificationFailed
		}
	}
	return smokeRemoveFailed
}

func setCertificateSmokeReceiptCode(receipt *CertificateSmokeReceipt, code certificateSmokeErrorCode) {
	if receipt == nil {
		return
	}
	value, ok := safeCertificateSmokeCode(code)
	if !ok {
		receipt.ErrorCode = nil
		return
	}
	receipt.ErrorCode = &value
}
