package poc

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const BatchSchemaVersion = "wechat-channels-batch/1"

type BatchStatus string

const (
	BatchSucceeded         BatchStatus = "succeeded"
	BatchPartial           BatchStatus = "partial"
	BatchNeedsVerification BatchStatus = "needs_verification"
	BatchFailed            BatchStatus = "failed"
)

type BatchRequest struct {
	SchemaVersion int      `json:"schema_version"`
	RunID         string   `json:"run_id"`
	Keyword       string   `json:"keyword"`
	ContentURLs   []string `json:"content_urls"`
	Limits        Limits   `json:"limits"`
	OutputRoot    string   `json:"output_root"`
}

type BatchTargetSummary struct {
	InputIndex       int      `json:"input_index"`
	WorkID           string   `json:"work_id,omitempty"`
	Status           string   `json:"status"`
	TopLevelComments int      `json:"top_level_comments"`
	Replies          int      `json:"replies"`
	Truncated        bool     `json:"truncated"`
	ReasonCodes      []string `json:"reason_codes,omitempty"`
}

type BatchDraftResult struct {
	SchemaVersion string               `json:"schema_version"`
	RunID         string               `json:"run_id"`
	Status        BatchStatus          `json:"status"`
	Counts        Counts               `json:"counts"`
	Targets       []BatchTargetSummary `json:"targets"`
	ReasonCodes   []string             `json:"reason_codes,omitempty"`
	CompletedAt   time.Time            `json:"completed_at"`
}

type BatchCleanupReceipt struct {
	SchemaVersion  int       `json:"schema_version"`
	RunID          string    `json:"run_id"`
	Safe           bool      `json:"safe"`
	CAAbsent       bool      `json:"ca_absent"`
	ClashRestored  bool      `json:"clash_restored"`
	ProcessStopped bool      `json:"process_stopped"`
	PortsReleased  bool      `json:"ports_released"`
	SecretsDeleted bool      `json:"secrets_deleted"`
	CompletedAt    time.Time `json:"completed_at"`
	ReasonCodes    []string  `json:"reason_codes,omitempty"`
}

type BatchFileRecord struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Lines  int    `json:"lines"`
}

type BatchCleanupSummary struct {
	Safe        bool     `json:"safe"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}

type BatchManifest struct {
	SchemaVersion string                     `json:"schema_version"`
	RunID         string                     `json:"run_id"`
	Status        BatchStatus                `json:"status"`
	Limits        Limits                     `json:"limits"`
	Counts        Counts                     `json:"counts"`
	Targets       []BatchTargetSummary       `json:"targets"`
	ReasonCodes   []string                   `json:"reason_codes,omitempty"`
	Cleanup       BatchCleanupSummary        `json:"cleanup"`
	Files         map[string]BatchFileRecord `json:"files"`
	CompletedAt   time.Time                  `json:"completed_at"`
}

func LoadBatchRequest(requestPath, runRoot string) (BatchRequest, error) {
	root, err := existingCanonicalDirectory(runRoot)
	if err != nil {
		return BatchRequest{}, errors.New("batch run root is unsafe")
	}
	requestAbs, err := filepath.Abs(requestPath)
	if err != nil || !pathWithin(root, requestAbs) || samePath(root, requestAbs) {
		return BatchRequest{}, errors.New("batch request path is unsafe")
	}
	if err := ensurePathComponentsSafe(root, requestAbs, false); err != nil {
		return BatchRequest{}, errors.New("batch request path contains a link")
	}
	if err := requireRegularFileWithoutReparse(requestAbs, 1<<20); err != nil {
		return BatchRequest{}, errors.New("batch request file is unsafe")
	}
	raw, err := os.ReadFile(requestAbs)
	if err != nil {
		return BatchRequest{}, errors.New("read batch request")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var request BatchRequest
	if err := decoder.Decode(&request); err != nil {
		return BatchRequest{}, errors.New("decode batch request")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return BatchRequest{}, errors.New("batch request contains trailing JSON")
	}
	if err := validateBatchRequest(&request, root); err != nil {
		return BatchRequest{}, err
	}
	return request, nil
}

func validateBatchRequest(request *BatchRequest, runRoot string) error {
	if request == nil || request.SchemaVersion != 1 || !validJobID(request.RunID) {
		return errors.New("batch request identity is invalid")
	}
	request.Keyword = strings.TrimSpace(request.Keyword)
	if request.Keyword == "" || len([]rune(request.Keyword)) > 200 || strings.ContainsAny(request.Keyword, "\r\n\x00") {
		return errors.New("batch keyword is invalid")
	}
	if len(request.ContentURLs) == 0 || len(request.ContentURLs) > 10 {
		return errors.New("batch content URL count is invalid")
	}
	seenURLs := make(map[string]struct{}, len(request.ContentURLs))
	for index, rawURL := range request.ContentURLs {
		normalized, err := NormalizeWeChatShareURL(rawURL)
		if err != nil {
			return errors.New("batch content URL is invalid")
		}
		if _, duplicate := seenURLs[normalized]; duplicate {
			return errors.New("batch content URL is duplicated")
		}
		seenURLs[normalized] = struct{}{}
		request.ContentURLs[index] = normalized
	}
	if request.Limits.Works < 1 || request.Limits.Works > 10 ||
		request.Limits.TopLevelCommentsPerWork < 1 || request.Limits.TopLevelCommentsPerWork > 100 ||
		request.Limits.RepliesPerComment < 0 || request.Limits.RepliesPerComment > 20 ||
		request.Limits.RepliesPerWork < 0 || request.Limits.RepliesPerWork > 200 {
		return errors.New("batch limits are invalid")
	}
	repliesDisabled := request.Limits.RepliesPerComment == 0 && request.Limits.RepliesPerWork == 0
	repliesEnabled := request.Limits.RepliesPerComment > 0 && request.Limits.RepliesPerWork > 0
	if !repliesDisabled && !repliesEnabled {
		return errors.New("batch reply limits are inconsistent")
	}
	if !filepath.IsAbs(request.OutputRoot) {
		return errors.New("batch output root must be absolute")
	}
	outputAbs, err := filepath.Abs(request.OutputRoot)
	if err != nil || !samePath(outputAbs, filepath.Join(runRoot, "batch")) {
		return errors.New("batch output root is outside the run boundary")
	}
	if _, err := os.Lstat(outputAbs); err == nil {
		return errors.New("batch output root already exists")
	} else if !os.IsNotExist(err) {
		return errors.New("inspect batch output root")
	}
	request.OutputRoot = filepath.Clean(outputAbs)
	return nil
}

func RunLtaooBatch(ctx context.Context, request BatchRequest, client *LtaooClient, runRoot string) (BatchDraftResult, error) {
	root, err := existingCanonicalDirectory(runRoot)
	if err != nil {
		return BatchDraftResult{}, errors.New("batch run root is unsafe")
	}
	if err := validateBatchRequest(&request, root); err != nil {
		return BatchDraftResult{}, err
	}
	if client == nil {
		return BatchDraftResult{}, errors.New("ltaoo client is missing")
	}
	draftRoot := filepath.Join(root, "batch.partial-"+request.RunID)
	if err := makeNewPrivateDirectory(draftRoot); err != nil {
		return BatchDraftResult{}, err
	}
	stateRoot := filepath.Join(root, "collector-state", request.RunID)
	stateStore, err := newBatchStateStore(stateRoot, request.RunID)
	if err != nil {
		return BatchDraftResult{}, err
	}
	if err := ensurePathComponentsSafe(root, stateRoot, false); err != nil {
		return BatchDraftResult{}, errors.New("batch state path contains a link")
	}
	result := BatchDraftResult{SchemaVersion: BatchSchemaVersion, RunID: request.RunID, Status: BatchSucceeded}
	issues := make([]Issue, 0)
	works := make([]Work, 0)
	comments := make([]Comment, 0)

	if err := client.Status(ctx); err != nil {
		issues = append(issues, Issue{Stage: "startup", Code: "ltaoo_unavailable"})
		result.Status = BatchFailed
	} else {
		works, issues = CollectWorksFromURLs(ctx, client, request.ContentURLs, request.Limits.Works)
		if len(works) == 0 {
			result.Status = BatchFailed
		}
	}
	collector := NewCollector(client, NewEvidenceRecorder(nil), stateStore, batchClock{})
	for index := range works {
		works[index].Locator.Keyword = request.Keyword
		collected, summary, collectErr := collector.CollectComments(ctx, Options{Limits: request.Limits}, works[index])
		comments = append(comments, collected...)
		works[index].TopLevelCommentCount = summary.TopLevel
		works[index].ReplyCount = summary.Replies
		works[index].Truncation.Truncated = summary.Truncated
		works[index].Truncation.Reasons = append([]string(nil), summary.Reasons...)
		target := BatchTargetSummary{
			InputIndex: works[index].Locator.IndexInPage, WorkID: dereferenceString(works[index].WorkID),
			TopLevelComments: summary.TopLevel, Replies: summary.Replies, Truncated: summary.Truncated,
			ReasonCodes: append([]string(nil), summary.Reasons...),
		}
		if collectErr != nil {
			works[index].CollectionStatus = "partial"
			target.Status = "partial"
			issue := issueForCollectionError(collectErr, dereferenceString(works[index].WorkID))
			issues = append(issues, issue)
			result.Status = BatchPartial
		} else if summary.Partial {
			works[index].CollectionStatus = "partial"
			target.Status = "partial"
			issues = append(issues, Issue{Stage: "comments", Code: "comments_incomplete", InputIndex: works[index].Locator.IndexInPage})
			result.Status = BatchPartial
		} else {
			works[index].CollectionStatus = "completed"
			target.Status = "completed"
		}
		result.Targets = append(result.Targets, target)
	}
	for _, issue := range issues {
		if issue.Code != "works_limit_reached" && result.Status == BatchSucceeded {
			result.Status = BatchPartial
		}
		appendUniqueString(&result.ReasonCodes, issue.Code)
	}
	result.Counts = countBatchRecords(works, comments)
	result.CompletedAt = time.Now().UTC()
	clearEvidenceReferences(works, comments)
	if err := writeJSONLines(filepath.Join(draftRoot, "contents.jsonl"), works); err != nil {
		return BatchDraftResult{}, err
	}
	if err := writeJSONLines(filepath.Join(draftRoot, "comments.jsonl"), comments); err != nil {
		return BatchDraftResult{}, err
	}
	if err := writeJSONLines(filepath.Join(draftRoot, "issues.jsonl"), issues); err != nil {
		return BatchDraftResult{}, err
	}
	if err := writeAtomicJSON(filepath.Join(root, "collection-result.json"), result); err != nil {
		return BatchDraftResult{}, err
	}
	return result, nil
}

func FinalizeLtaooBatch(request BatchRequest, runRoot, cleanupReceiptPath string) (BatchManifest, error) {
	root, err := existingCanonicalDirectory(runRoot)
	if err != nil {
		return BatchManifest{}, errors.New("batch run root is unsafe")
	}
	if err := validateBatchRequest(&request, root); err != nil {
		return BatchManifest{}, err
	}
	draftRoot := filepath.Join(root, "batch.partial-"+request.RunID)
	if err := validateExistingDirectory(root, draftRoot); err != nil {
		return BatchManifest{}, errors.New("batch draft is unsafe")
	}
	resultPath := filepath.Join(root, "collection-result.json")
	var result BatchDraftResult
	if err := readStrictJSONFile(resultPath, 1<<20, &result); err != nil || result.SchemaVersion != BatchSchemaVersion || result.RunID != request.RunID {
		return BatchManifest{}, errors.New("batch draft result is invalid")
	}
	var receipt BatchCleanupReceipt
	receiptAbs, err := filepath.Abs(cleanupReceiptPath)
	if err != nil || !pathWithin(root, receiptAbs) || samePath(root, receiptAbs) {
		return BatchManifest{}, errors.New("cleanup receipt path is unsafe")
	}
	if err := ensurePathComponentsSafe(root, receiptAbs, false); err != nil {
		return BatchManifest{}, errors.New("cleanup receipt path contains a link")
	}
	if err := readStrictJSONFile(receiptAbs, 1<<20, &receipt); err != nil {
		return BatchManifest{}, errors.New("cleanup receipt is invalid")
	}
	if err := validateBatchCleanupReceipt(receipt, request.RunID); err != nil {
		return BatchManifest{}, err
	}
	receiptTarget := filepath.Join(draftRoot, "cleanup-receipt.json")
	if err := writeAtomicJSON(receiptTarget, receipt); err != nil {
		return BatchManifest{}, err
	}
	fileNames := []string{"cleanup-receipt.json", "comments.jsonl", "contents.jsonl", "issues.jsonl"}
	files := make(map[string]BatchFileRecord, len(fileNames))
	for _, name := range fileNames {
		record, err := inspectBatchFile(filepath.Join(draftRoot, name))
		if err != nil {
			return BatchManifest{}, err
		}
		files[name] = record
	}
	status := result.Status
	reasons := append([]string(nil), result.ReasonCodes...)
	if !receipt.Safe {
		status = BatchNeedsVerification
		for _, code := range receipt.ReasonCodes {
			appendUniqueString(&reasons, code)
		}
	}
	manifest := BatchManifest{
		SchemaVersion: BatchSchemaVersion, RunID: request.RunID, Status: status, Limits: request.Limits,
		Counts: result.Counts, Targets: result.Targets, ReasonCodes: reasons,
		Cleanup: BatchCleanupSummary{Safe: receipt.Safe, ReasonCodes: append([]string(nil), receipt.ReasonCodes...)},
		Files:   files, CompletedAt: time.Now().UTC(),
	}
	if err := writeAtomicJSON(filepath.Join(draftRoot, "manifest.json"), manifest); err != nil {
		return BatchManifest{}, err
	}
	stateRoot := filepath.Join(root, "collector-state")
	if err := removeContainedTree(root, stateRoot); err != nil {
		return BatchManifest{}, errors.New("remove batch collector state")
	}
	if err := os.Rename(draftRoot, request.OutputRoot); err != nil {
		return BatchManifest{}, errors.New("publish final batch")
	}
	_ = os.Remove(resultPath)
	return manifest, nil
}

func validateBatchCleanupReceipt(receipt BatchCleanupReceipt, runID string) error {
	if receipt.SchemaVersion != 1 || receipt.RunID != runID || receipt.CompletedAt.IsZero() {
		return errors.New("cleanup receipt identity is invalid")
	}
	allSafe := receipt.CAAbsent && receipt.ClashRestored && receipt.ProcessStopped && receipt.PortsReleased && receipt.SecretsDeleted
	if receipt.Safe != allSafe || (receipt.Safe && len(receipt.ReasonCodes) != 0) || (!receipt.Safe && len(receipt.ReasonCodes) == 0) {
		return errors.New("cleanup receipt state is inconsistent")
	}
	for _, code := range receipt.ReasonCodes {
		if code != "cleanup_attention_required" && code != "cleanup_failed" {
			return errors.New("cleanup receipt reason is invalid")
		}
	}
	return nil
}

type batchClock struct{}

func (batchClock) Now() time.Time { return time.Now().UTC() }
func (batchClock) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type batchStateStore struct {
	root        string
	jobDir      string
	evidenceDir string
	mu          sync.Mutex
}

func newBatchStateStore(root, runID string) (*batchStateStore, error) {
	jobDir := filepath.Join(root, runID)
	evidenceDir := filepath.Join(jobDir, "evidence")
	for _, directory := range []string{root, jobDir, evidenceDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, errors.New("create batch state directory")
		}
		if err := rejectPlatformReparse(directory); err != nil {
			return nil, errors.New("batch state directory is a reparse point")
		}
	}
	return &batchStateStore{root: root, jobDir: jobDir, evidenceDir: evidenceDir}, nil
}

func (s *batchStateStore) JobDir() string { return s.jobDir }

func (s *batchStateStore) WriteEvidence(value Evidence) (string, error) {
	name := fmt.Sprintf("%06d.json", value.RequestSequence)
	if err := writeAtomicJSON(filepath.Join(s.evidenceDir, name), value); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("evidence", name)), nil
}

func (s *batchStateStore) MaxEvidenceSequence() (int, error) {
	entries, err := os.ReadDir(s.evidenceDir)
	if err != nil {
		return 0, errors.New("read batch evidence directory")
	}
	maximum := 0
	for _, entry := range entries {
		var sequence int
		if entry.IsDir() {
			continue
		}
		if _, err := fmt.Sscanf(entry.Name(), "%06d.json", &sequence); err != nil {
			continue
		}
		if sequence > maximum {
			maximum = sequence
		}
	}
	return maximum, nil
}

func (s *batchStateStore) SaveCheckpoint(value Checkpoint) error {
	return writeAtomicJSON(filepath.Join(s.jobDir, "checkpoint.json"), value)
}

func (s *batchStateStore) LoadCheckpoint() (Checkpoint, error) {
	path := filepath.Join(s.jobDir, "checkpoint.json")
	var checkpoint Checkpoint
	if err := readStrictJSONFile(path, 32<<20, &checkpoint); err != nil {
		if os.IsNotExist(err) {
			return Checkpoint{}, ErrCheckpointNotFound
		}
		return Checkpoint{}, err
	}
	return checkpoint, nil
}

func makeNewPrivateDirectory(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return errors.New("batch draft already exists")
	} else if !os.IsNotExist(err) {
		return errors.New("inspect batch draft")
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return errors.New("create batch draft")
	}
	return rejectPlatformReparse(path)
}

func writeJSONLines[T any](path string, values []T) error {
	parent := filepath.Dir(path)
	temp, err := os.CreateTemp(parent, ".batch-*.tmp")
	if err != nil {
		return errors.New("create batch JSONL temp file")
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return errors.New("restrict batch JSONL temp file")
	}
	writer := bufio.NewWriter(temp)
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil || ScanOrdinaryOutput(raw) != nil {
			return errors.New("encode safe batch JSONL record")
		}
		if _, err := writer.Write(append(raw, '\n')); err != nil {
			return errors.New("write batch JSONL record")
		}
	}
	if err := writer.Flush(); err != nil || temp.Sync() != nil || temp.Close() != nil {
		return errors.New("close batch JSONL file")
	}
	closed = true
	if err := os.Rename(tempPath, path); err != nil {
		return errors.New("publish batch JSONL file")
	}
	return nil
}

func writeAtomicJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil || ScanOrdinaryOutput(raw) != nil {
		return errors.New("encode safe batch JSON")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return errors.New("create batch JSON parent")
	}
	temp, err := os.CreateTemp(parent, ".batch-*.tmp")
	if err != nil {
		return errors.New("create batch JSON temp file")
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return errors.New("restrict batch JSON temp file")
	}
	if _, err := temp.Write(raw); err != nil || temp.Sync() != nil || temp.Close() != nil {
		return errors.New("write batch JSON temp file")
	}
	closed = true
	if err := atomicReplace(tempPath, path); err != nil {
		return errors.New("publish batch JSON file")
	}
	return nil
}

func readStrictJSONFile(path string, maxBytes int64, target any) error {
	if err := requireRegularFileWithoutReparse(path, maxBytes); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("JSON file contains trailing content")
	}
	return nil
}

func requireRegularFileWithoutReparse(path string, maxBytes int64) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("file is not a bounded regular file")
	}
	return rejectPlatformReparse(path)
}

func inspectBatchFile(path string) (BatchFileRecord, error) {
	if err := requireRegularFileWithoutReparse(path, 64<<20); err != nil {
		return BatchFileRecord{}, errors.New("batch file is unsafe")
	}
	raw, err := os.ReadFile(path)
	if err != nil || ScanOrdinaryOutput(raw) != nil {
		return BatchFileRecord{}, errors.New("batch file failed safety validation")
	}
	digest := sha256.Sum256(raw)
	lines := 0
	if len(raw) > 0 {
		lines = strings.Count(string(raw), "\n")
	}
	return BatchFileRecord{SHA256: hex.EncodeToString(digest[:]), Bytes: int64(len(raw)), Lines: lines}, nil
}

func countBatchRecords(works []Work, comments []Comment) Counts {
	counts := Counts{Works: len(works)}
	for _, comment := range comments {
		if comment.Level == 1 {
			counts.TopLevelComments++
		} else if comment.Level == 2 {
			counts.Replies++
		}
	}
	return counts
}

func issueForCollectionError(err error, workID string) Issue {
	stage := "comments"
	code := "comment_api_failed"
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = "collection_cancelled"
	} else if ClassifyError(err) == ErrorStructure {
		code = "comment_response_invalid"
	}
	return Issue{Stage: stage, Code: code, WorkID: workID}
}

func clearEvidenceReferences(works []Work, comments []Comment) {
	for index := range works {
		works[index].Source.EvidenceRef = ""
	}
	for index := range comments {
		comments[index].Source.EvidenceRef = ""
	}
}

func appendUniqueString(values *[]string, value string) {
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
	sort.Strings(*values)
}

func removeContainedTree(root, target string) error {
	if !pathWithin(root, target) || samePath(root, target) {
		return errors.New("refuse broad tree removal")
	}
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := ensureTreeNoLinks(root, target); err != nil {
		return err
	}
	return os.RemoveAll(target)
}
