package poc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const RedactionRuleVersion = "wx-channel-comment-poc/redaction-1"

var evidenceMethodPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

type SchemaField struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Count int    `json:"count,omitempty"`
}

type RedactionCounts struct {
	CredentialKeys int `json:"credential_keys"`
	QueryURLs      int `json:"query_urls"`
	PEMMarkers     int `json:"pem_markers"`
}

type Evidence struct {
	RequestSequence      int             `json:"request_sequence"`
	Method               string          `json:"method"`
	SourceResponseSHA256 string          `json:"source_response_sha256"`
	ResponseBytes        int             `json:"response_bytes"`
	RecordCount          int             `json:"record_count"`
	Fields               []SchemaField   `json:"fields"`
	RedactionRuleVersion string          `json:"redaction_rule_version"`
	Redactions           RedactionCounts `json:"redactions"`
}

type EvidenceRecorder struct {
	encrypted *EncryptedRawStore
}

func NewEvidenceRecorder(encrypted *EncryptedRawStore) *EvidenceRecorder {
	return &EvidenceRecorder{encrypted: encrypted}
}

func (r *EvidenceRecorder) Observe(sequence int, method string, raw []byte) (Evidence, error) {
	if sequence < 0 {
		return Evidence{}, errors.New("request sequence must be non-negative")
	}
	if !evidenceMethodPattern.MatchString(method) {
		return Evidence{}, errors.New("evidence method is not an approved identifier")
	}
	digest := sha256.Sum256(raw)
	redactions := countRedactions(raw)

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return Evidence{}, fmt.Errorf("decode response structure: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Evidence{}, errors.New("decode response structure: multiple JSON values")
		}
		return Evidence{}, fmt.Errorf("decode response structure: %w", err)
	}

	fields, recordCount := schemaOf(document)
	if r != nil && r.encrypted != nil {
		if err := r.encrypted.Write(sequence, raw); err != nil {
			return Evidence{}, fmt.Errorf("encrypt raw evidence: %w", err)
		}
	}

	return Evidence{
		RequestSequence:      sequence,
		Method:               method,
		SourceResponseSHA256: hex.EncodeToString(digest[:]),
		ResponseBytes:        len(raw),
		RecordCount:          recordCount,
		Fields:               fields,
		RedactionRuleVersion: RedactionRuleVersion,
		Redactions:           redactions,
	}, nil
}

func schemaOf(document any) ([]SchemaField, int) {
	fields := make(map[string]SchemaField)
	recordCount := 0
	var visit func(string, any)
	visit = func(path string, value any) {
		field := SchemaField{Path: path, Type: jsonType(value)}
		switch typed := value.(type) {
		case []any:
			field.Count = len(typed)
			if len(typed) > recordCount {
				recordCount = len(typed)
			}
			fields[path+"\x00"+field.Type] = field
			for _, item := range typed {
				visit(path+"[]", item)
			}
		case map[string]any:
			fields[path+"\x00"+field.Type] = field
			for key, item := range typed {
				visit(path+"."+key, item)
			}
		default:
			fields[path+"\x00"+field.Type] = field
		}
	}
	visit("$", document)

	result := make([]SchemaField, 0, len(fields))
	for _, field := range fields {
		result = append(result, field)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Type < result[j].Type
		}
		return result[i].Path < result[j].Path
	})
	return result, recordCount
}

func jsonType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

type EncryptedRawStore struct {
	dir       string
	key       []byte
	mu        sync.Mutex
	destroyed bool
}

func NewEncryptedRawStore(dir string, enabled bool) (*EncryptedRawStore, error) {
	if !enabled {
		return nil, nil
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.New("resolve encrypted evidence directory")
	}
	resolvedDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return nil, errors.New("encrypted evidence directory must already exist")
	}
	if !samePath(absDir, resolvedDir) {
		return nil, errors.New("encrypted evidence directory must not be a symlink")
	}
	info, err := os.Stat(resolvedDir)
	if err != nil || !info.IsDir() {
		return nil, errors.New("encrypted evidence path is not a directory")
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, errors.New("generate encrypted evidence key")
	}
	return &EncryptedRawStore{dir: resolvedDir, key: key}, nil
}

func (s *EncryptedRawStore) Write(sequence int, raw []byte) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyed {
		return errors.New("encrypted evidence store is destroyed")
	}
	if sequence < 0 {
		return errors.New("request sequence must be non-negative")
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return errors.New("initialize encrypted evidence cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return errors.New("initialize encrypted evidence GCM")
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return errors.New("generate encrypted evidence nonce")
	}
	sealed := gcm.Seal(nil, nonce, raw, nil)
	payload := append(nonce, sealed...)
	path := filepath.Join(s.dir, fmt.Sprintf("%06d.enc", sequence))
	if !pathWithin(s.dir, path) {
		return errors.New("encrypted evidence path escapes directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create encrypted evidence file")
	}
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(payload); err != nil {
		return errors.New("write encrypted evidence file")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync encrypted evidence file")
	}
	if err := file.Close(); err != nil {
		return errors.New("close encrypted evidence file")
	}
	written = true
	return nil
}

func (s *EncryptedRawStore) Destroy() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.key {
		s.key[i] = 0
	}
	s.destroyed = true

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.New("list encrypted evidence directory")
	}
	var removeErrors []error
	for _, entry := range entries {
		if entry.IsDir() || !isEncryptedEvidenceName(entry.Name()) {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		if !pathWithin(s.dir, path) {
			removeErrors = append(removeErrors, errors.New("encrypted evidence path escapes directory"))
			continue
		}
		if err := os.Remove(path); err != nil {
			removeErrors = append(removeErrors, errors.New("remove encrypted evidence file"))
		}
	}
	return errors.Join(removeErrors...)
}

func isEncryptedEvidenceName(name string) bool {
	if filepath.Ext(name) != ".enc" {
		return false
	}
	base := strings.TrimSuffix(name, ".enc")
	if base == "" {
		return false
	}
	_, err := strconv.ParseUint(base, 10, 64)
	return err == nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
