package poc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const minimumRetention = 7 * 24 * time.Hour

var ErrCheckpointNotFound = errors.New("checkpoint not found")

var (
	jobIDPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	outputNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]*\.json$`)
	evidenceNamePattern = regexp.MustCompile(`^[0-9]{6}\.json$`)
)

func validJobID(value string) bool {
	return jobIDPattern.MatchString(value) && value != "." && value != ".."
}

type StoreOptions struct {
	RepoRoot    string
	DataRoot    string
	SecretsRoot string
	RuntimeRoot string
	BuildRoot   string
	VarRoot     string
	JobID       string
}

type Store struct {
	repoRoot      string
	dataRoot      string
	jobDir        string
	secretsDir    string
	runtimeDir    string
	buildDir      string
	varDir        string
	evidenceDir   string
	atomicReplace func(string, string) error
	mu            sync.Mutex
}

func NewStore(options StoreOptions) (*Store, error) {
	if !validJobID(options.JobID) {
		return nil, errors.New("invalid job ID")
	}
	repoRoot, err := existingCanonicalDirectory(options.RepoRoot)
	if err != nil {
		return nil, errors.New("repository root is not a safe existing directory")
	}

	options = withDefaultStoreRoots(options)
	roots := make([]string, 0, 5)
	for _, configured := range []string{options.DataRoot, options.SecretsRoot, options.RuntimeRoot, options.BuildRoot, options.VarRoot} {
		root, err := resolveStorePath(repoRoot, configured)
		if err != nil {
			return nil, err
		}
		if samePath(root, repoRoot) {
			return nil, errors.New("store root must be below repository root")
		}
		if err := ensurePathComponentsSafe(repoRoot, root, true); err != nil {
			return nil, errors.New("store root contains a link or reparse point")
		}
		if err := requireGitIgnored(repoRoot, root); err != nil {
			return nil, err
		}
		for _, existing := range roots {
			if samePath(existing, root) {
				return nil, errors.New("store roots must be distinct")
			}
		}
		roots = append(roots, root)
	}

	jobDirs := make([]string, len(roots))
	for i, root := range roots {
		jobDirs[i] = filepath.Join(root, options.JobID)
		if !pathWithin(root, jobDirs[i]) {
			return nil, errors.New("job directory escapes store root")
		}
		if err := ensurePathComponentsSafe(repoRoot, jobDirs[i], true); err != nil {
			return nil, errors.New("job directory contains a link or reparse point")
		}
	}
	evidenceDir := filepath.Join(jobDirs[0], "evidence")

	for _, directory := range append(append([]string{}, roots...), append(jobDirs, evidenceDir)...) {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, errors.New("create guarded store directory")
		}
		if err := validateExistingDirectory(repoRoot, directory); err != nil {
			return nil, errors.New("created store directory failed link validation")
		}
		if err := secureDirectory(directory); err != nil {
			return nil, errors.New("secure store directory permissions")
		}
	}

	return &Store{
		repoRoot:      repoRoot,
		dataRoot:      roots[0],
		jobDir:        jobDirs[0],
		secretsDir:    jobDirs[1],
		runtimeDir:    jobDirs[2],
		buildDir:      jobDirs[3],
		varDir:        jobDirs[4],
		evidenceDir:   evidenceDir,
		atomicReplace: atomicReplace,
	}, nil
}

func withDefaultStoreRoots(options StoreOptions) StoreOptions {
	if options.DataRoot == "" {
		options.DataRoot = ".poc-data"
	}
	if options.SecretsRoot == "" {
		options.SecretsRoot = ".poc-secrets"
	}
	if options.RuntimeRoot == "" {
		options.RuntimeRoot = ".poc-runtime"
	}
	if options.BuildRoot == "" {
		options.BuildRoot = ".poc-build"
	}
	if options.VarRoot == "" {
		options.VarRoot = "var"
	}
	return options
}

func existingCanonicalDirectory(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil || !samePath(absPath, resolved) {
		return "", errors.New("directory uses a link")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("not a directory")
	}
	if err := rejectPlatformReparse(resolved); err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func resolveStorePath(repoRoot, configured string) (string, error) {
	if configured == "" {
		return "", errors.New("store root is empty")
	}
	root := configured
	if !filepath.IsAbs(root) {
		root = filepath.Join(repoRoot, root)
	}
	root, err := filepath.Abs(root)
	if err != nil || !pathWithin(repoRoot, root) {
		return "", errors.New("store root must be inside repository")
	}
	return filepath.Clean(root), nil
}

func requireGitIgnored(repoRoot, root string) error {
	probe := filepath.Join(root, "probe")
	relative, err := filepath.Rel(repoRoot, probe)
	if err != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("git ignored store probe escapes repository")
	}
	command := exec.Command("git", "check-ignore", "--quiet", "--", filepath.ToSlash(relative))
	command.Dir = repoRoot
	if err := command.Run(); err != nil {
		return errors.New("store root is not git ignored")
	}
	return nil
}

func ensurePathComponentsSafe(repoRoot, target string, allowMissing bool) error {
	if !pathWithin(repoRoot, target) {
		return errors.New("path escapes repository")
	}
	relative, err := filepath.Rel(repoRoot, target)
	if err != nil {
		return err
	}
	current := repoRoot
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) && allowMissing {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("path contains a symbolic link")
		}
		if err := rejectPlatformReparse(current); err != nil {
			return err
		}
	}
	return nil
}

func validateExistingDirectory(repoRoot, directory string) error {
	if err := ensurePathComponentsSafe(repoRoot, directory, false); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || !samePath(directory, resolved) {
		return errors.New("directory resolves through a link")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return errors.New("path is not a directory")
	}
	return nil
}

func (s *Store) JobDir() string {
	return s.jobDir
}

func (s *Store) SecretsDir() string {
	return s.secretsDir
}

func (s *Store) RuntimeDir() string {
	return s.runtimeDir
}

func (s *Store) WriteJSON(name string, value any) error {
	if !outputNamePattern.MatchString(name) || filepath.Base(name) != name {
		return errors.New("invalid output filename")
	}
	return s.writeJSONAt(filepath.Join(s.jobDir, name), value)
}

func (s *Store) WriteDataset(value Dataset) error {
	return s.WriteJSON("dataset.json", value)
}

func (s *Store) WriteValidation(value Validation) error {
	return s.WriteJSON("validation.json", value)
}

func (s *Store) WriteManifest(value Manifest) error {
	return s.WriteJSON("manifest.json", value)
}

func (s *Store) WriteEvidence(value Evidence) (string, error) {
	if value.RequestSequence < 0 || !evidenceMethodPattern.MatchString(value.Method) {
		return "", errors.New("invalid evidence metadata")
	}
	name := fmt.Sprintf("%06d.json", value.RequestSequence)
	target := filepath.Join(s.evidenceDir, name)
	if _, err := os.Lstat(target); err == nil {
		return "", errors.New("evidence sequence already exists")
	} else if !os.IsNotExist(err) {
		return "", errors.New("inspect evidence target")
	}
	if err := s.writeJSONAt(target, value); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("evidence", name)), nil
}

func (s *Store) MaxEvidenceSequence() (int, error) {
	if err := validateExistingDirectory(s.repoRoot, s.evidenceDir); err != nil {
		return 0, errors.New("evidence directory failed link validation")
	}
	entries, err := os.ReadDir(s.evidenceDir)
	if err != nil {
		return 0, errors.New("read evidence directory")
	}
	maximum := 0
	for _, entry := range entries {
		if entry.IsDir() || !evidenceNamePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return 0, errors.New("inspect evidence file")
		}
		sequence, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return 0, errors.New("parse evidence sequence")
		}
		if sequence > maximum {
			maximum = sequence
		}
	}
	return maximum, nil
}

func (s *Store) SaveCheckpoint(value Checkpoint) error {
	return s.WriteJSON("checkpoint.json", value)
}

func (s *Store) LoadCheckpoint() (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateExistingDirectory(s.repoRoot, s.jobDir); err != nil {
		return Checkpoint{}, errors.New("checkpoint directory failed link validation")
	}
	target := filepath.Join(s.jobDir, "checkpoint.json")
	if err := validateOutputTarget(target); err != nil {
		return Checkpoint{}, err
	}
	raw, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return Checkpoint{}, ErrCheckpointNotFound
	}
	if err != nil {
		return Checkpoint{}, errors.New("read checkpoint")
	}
	if err := ScanOrdinaryOutput(raw); err != nil {
		return Checkpoint{}, err
	}
	var checkpoint Checkpoint
	if err := json.Unmarshal(raw, &checkpoint); err != nil {
		return Checkpoint{}, errors.New("decode checkpoint")
	}
	return checkpoint, nil
}

func (s *Store) WriteCleanupReceipt(value CleanupReceipt) error {
	return s.WriteJSON("cleanup_receipt.json", value)
}

func (s *Store) WriteRuntimeState(value PersistedRuntimeState) error {
	return s.writeJSONWithin(s.runtimeDir, filepath.Join(s.runtimeDir, "state.json"), value)
}

func (s *Store) LoadRuntimeState() (PersistedRuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateExistingDirectory(s.repoRoot, s.runtimeDir); err != nil {
		return PersistedRuntimeState{}, errors.New("runtime state directory failed validation")
	}
	target := filepath.Join(s.runtimeDir, "state.json")
	if err := validateOutputTarget(target); err != nil {
		return PersistedRuntimeState{}, err
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return PersistedRuntimeState{}, errors.New("read runtime state")
	}
	if err := ScanOrdinaryOutput(raw); err != nil {
		return PersistedRuntimeState{}, err
	}
	var state PersistedRuntimeState
	if err := json.Unmarshal(raw, &state); err != nil {
		return PersistedRuntimeState{}, errors.New("decode runtime state")
	}
	return state, nil
}

func (s *Store) RemoveRuntimeState() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateExistingDirectory(s.repoRoot, s.runtimeDir); err != nil {
		return errors.New("runtime state directory failed validation")
	}
	target := filepath.Join(s.runtimeDir, "state.json")
	if err := validateOutputTarget(target); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return errors.New("remove runtime state")
	}
	if err := os.Remove(s.runtimeDir); err != nil && !os.IsNotExist(err) {
		return errors.New("remove empty runtime directory")
	}
	return nil
}

func (s *Store) writeJSONAt(target string, value any) error {
	return s.writeJSONWithin(s.jobDir, target, value)
}

func (s *Store) writeJSONWithin(root, target string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !pathWithin(root, target) {
		return errors.New("output path escapes job directory")
	}
	parent := filepath.Dir(target)
	if err := validateExistingDirectory(s.repoRoot, parent); err != nil {
		return errors.New("output directory failed link validation")
	}
	if err := validateOutputTarget(target); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.New("encode output JSON")
	}
	if err := ScanOrdinaryOutput(raw); err != nil {
		return err
	}
	temp, err := os.CreateTemp(parent, ".poc-*.tmp")
	if err != nil {
		return errors.New("create atomic output temp file")
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
		return errors.New("restrict atomic output permissions")
	}
	if _, err := temp.Write(raw); err != nil {
		return errors.New("write atomic output")
	}
	if err := temp.Sync(); err != nil {
		return errors.New("sync atomic output")
	}
	if err := temp.Close(); err != nil {
		return errors.New("close atomic output")
	}
	closed = true
	if err := s.atomicReplace(tempPath, target); err != nil {
		return errors.New("replace atomic output")
	}
	return nil
}

func (s *Store) DeleteExpired(now time.Time, maxAge time.Duration) ([]string, error) {
	if maxAge < minimumRetention {
		return nil, errors.New("retention must be at least seven days")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateExistingDirectory(s.repoRoot, s.dataRoot); err != nil {
		return nil, errors.New("data root failed link validation")
	}
	entries, err := os.ReadDir(s.dataRoot)
	if err != nil {
		return nil, errors.New("list data root")
	}
	cutoff := now.Add(-maxAge)
	type retentionCandidate struct {
		name string
		path string
	}
	var candidates []retentionCandidate
	for _, entry := range entries {
		if !entry.IsDir() || !jobIDPattern.MatchString(entry.Name()) || entry.Name() == "." || entry.Name() == ".." {
			continue
		}
		candidate := filepath.Join(s.dataRoot, entry.Name())
		if !pathWithin(s.dataRoot, candidate) {
			return nil, errors.New("retention candidate escapes data root")
		}
		if err := ensureTreeNoLinks(s.repoRoot, candidate); err != nil {
			return nil, errors.New("retention candidate contains a link or reparse point")
		}
		manifest, err := readRetentionManifest(filepath.Join(candidate, "manifest.json"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if manifest.SchemaVersion != SchemaVersion || manifest.JobID != entry.Name() || manifest.CompletedAt == nil || !isTerminalStatus(manifest.Status) {
			continue
		}
		if !manifest.CompletedAt.Before(cutoff) {
			continue
		}
		candidates = append(candidates, retentionCandidate{name: entry.Name(), path: candidate})
	}
	var deleted []string
	for _, candidate := range candidates {
		if err := ensureTreeNoLinks(s.repoRoot, candidate.path); err != nil {
			return deleted, errors.New("retention candidate changed before deletion")
		}
		if err := os.RemoveAll(candidate.path); err != nil {
			return deleted, errors.New("remove expired job directory")
		}
		deleted = append(deleted, candidate.name)
	}
	sort.Strings(deleted)
	return deleted, nil
}

func validateOutputTarget(target string) error {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.New("inspect output target")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("output target is not a regular file")
	}
	if err := rejectPlatformReparse(target); err != nil {
		return errors.New("output target is a reparse point")
	}
	return nil
}

func readRetentionManifest(path string) (Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return Manifest{}, errors.New("invalid retention manifest")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, errors.New("read retention manifest")
	}
	if err := ScanOrdinaryOutput(raw); err != nil {
		return Manifest{}, errors.New("retention manifest failed safety scan")
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, errors.New("decode retention manifest")
	}
	return manifest, nil
}

func isTerminalStatus(status JobStatus) bool {
	switch status {
	case JobCompleted, JobRequiresHuman, JobPartial, JobFailed:
		return true
	default:
		return false
	}
}

func ensureTreeNoLinks(repoRoot, root string) error {
	if err := validateExistingDirectory(repoRoot, root); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !pathWithin(root, path) {
			return errors.New("tree entry escapes root")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("tree contains a symbolic link")
		}
		return rejectPlatformReparse(path)
	})
}
