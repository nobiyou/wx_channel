package poc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertificateSmokeErrorCodeIsClosed(t *testing.T) {
	for _, code := range []certificateSmokeErrorCode{
		smokePreflightFailed, smokeApprovalRejected, smokeCertificatePreexisting,
		smokeInstallFailed, smokeInstallVerificationFailed, smokeRemoveFailed,
		smokeRemoveVerificationFailed, smokeSecretsCleanupFailed,
	} {
		if got, ok := safeCertificateSmokeErrorCode(newCertificateSmokeError(code)); !ok || got != string(code) {
			t.Fatalf("code=%q got=%q ok=%v", code, got, ok)
		}
	}
	if _, ok := safeCertificateSmokeErrorCode(errors.New(`secret C:\fixture token XYZ`)); ok {
		t.Fatal("untrusted error was accepted")
	}
}

func TestCertificateSmokeReceiptContainsOnlySafeFields(t *testing.T) {
	code := string(smokeInstallFailed)
	receipt := CertificateSmokeReceipt{
		SchemaVersion: CertificateSmokeSchemaVersion,
		JobID:         "poc-20260729T010203-a1b2c3d4e5f6",
		ErrorCode:     &code,
		CompletedAt:   time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC),
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := ScanOrdinaryOutput(raw); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"certificate_sha256", "certificate_path", "private_key", "raw_error"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("receipt contains forbidden field %q", forbidden)
		}
	}
}

func TestStoreWritesCertificateSmokeReceipt(t *testing.T) {
	store := newTestStore(t, "certificate-smoke-store")
	receipt := CertificateSmokeReceipt{SchemaVersion: CertificateSmokeSchemaVersion, JobID: "certificate-smoke-store"}
	if err := store.WriteCertificateSmokeReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(store.JobDir(), "certificate-smoke-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded CertificateSmokeReceipt
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.JobID != receipt.JobID {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
}
