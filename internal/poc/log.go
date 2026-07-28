package poc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

type SafeLogger interface {
	Event(name string, fields map[string]any) error
}

type DiscardLogger struct{}

func (DiscardLogger) Event(string, map[string]any) error {
	return nil
}

var safeLogIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

var safeLogEvents = map[string]struct{}{
	"preflight_completed": {}, "approval_received": {}, "ca_installed": {},
	"bridge_started": {}, "bridge_connected": {}, "bridge_disconnected": {},
	"proxy_started": {}, "driver_started": {}, "collection_started": {},
	"request_completed": {}, "retry_scheduled": {}, "progress": {},
	"cleanup_started": {}, "cleanup_completed": {}, "run_completed": {}, "run_failed": {},
}

var safeLogFields = map[string]struct{}{
	"phase": {}, "request_sequence": {}, "method": {}, "duration_ms": {}, "bytes": {},
	"error_category": {}, "work_rank": {}, "works": {}, "top_level_comments": {},
	"replies": {}, "truncated": {},
}

type FileSafeLogger struct {
	path string
	mu   sync.Mutex
}

func NewFileSafeLogger(path string) (*FileSafeLogger, error) {
	if !filepath.IsAbs(path) || filepath.Base(path) != "run.log" {
		return nil, errors.New("safe log path must be an absolute run.log path")
	}
	parent, err := existingCanonicalDirectory(filepath.Dir(path))
	dataRoot := filepath.Dir(parent)
	repoRoot := filepath.Dir(dataRoot)
	canonicalDataRoot, dataErr := existingCanonicalDirectory(dataRoot)
	canonicalRepoRoot, repoErr := existingCanonicalDirectory(repoRoot)
	if err != nil || dataErr != nil || repoErr != nil || !samePath(parent, filepath.Dir(path)) || !samePath(dataRoot, canonicalDataRoot) ||
		!samePath(repoRoot, canonicalRepoRoot) || filepath.Base(dataRoot) != ".poc-data" || requireGitIgnored(repoRoot, dataRoot) != nil {
		return nil, errors.New("safe log path is outside a guarded POC job directory")
	}
	return &FileSafeLogger{path: path}, nil
}

func (l *FileSafeLogger) Event(name string, fields map[string]any) error {
	if l == nil {
		return errors.New("safe logger is nil")
	}
	if _, ok := safeLogEvents[name]; !ok {
		return errors.New("safe log event is not allowed")
	}
	entry := make(map[string]any, len(fields)+1)
	entry["event"] = name
	for key, value := range fields {
		if _, ok := safeLogFields[key]; !ok || !safeLogValue(key, value) {
			return errors.New("safe log field is not allowed")
		}
		entry[key] = value
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return errors.New("encode safe log event")
	}
	raw = append(raw, '\n')
	if err := ScanOrdinaryOutput(raw); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := validateExistingDirectory(filepath.Dir(filepath.Dir(l.path)), filepath.Dir(l.path)); err != nil {
		return errors.New("safe log directory failed validation")
	}
	if err := validateOutputTarget(l.path); err != nil {
		return err
	}
	file, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return errors.New("open safe log")
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return errors.New("restrict safe log")
	}
	if _, err := file.Write(raw); err != nil {
		return errors.New("append safe log")
	}
	return file.Sync()
}

func safeLogValue(key string, value any) bool {
	switch key {
	case "phase":
		text, ok := value.(string)
		return ok && safeLogIdentifier.MatchString(text)
	case "method":
		text, ok := value.(string)
		_, allowed := allowedMethods[text]
		return ok && allowed
	case "error_category":
		text, ok := value.(string)
		if !ok {
			return false
		}
		switch ErrorCategory(text) {
		case ErrorTransient, ErrorRateLimited, ErrorAccessDenied, ErrorTargetContext, ErrorMethodMissing, ErrorSafety, ErrorStructure, ErrorUnknown:
			return true
		default:
			return false
		}
	case "truncated":
		_, ok := value.(bool)
		return ok
	default:
		integer, ok := value.(int)
		return ok && integer >= 0
	}
}
