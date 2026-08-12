package poc

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	pemPrivateKeyPattern = regexp.MustCompile(`(?i)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	credentialPattern    = regexp.MustCompile(`(?i)(authorization|bearer[[:space:]]+|set-cookie|"(?:cookie|token|session_token|private_key)"[[:space:]]*:)`)
	highConfidenceSecret = regexp.MustCompile(`(?i)(-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----|bearer[[:space:]]+[^[:space:]]+|set-cookie[[:space:]]*:|"(?:authorization|cookie|token|session_token|private_key)"[[:space:]]*:)`)
	queryURLPattern      = regexp.MustCompile(`(?i)https?://[^[:space:]"'<>]*\?[^[:space:]"'<>]+`)
)

func SafeURL(raw string) (*string, FieldStatus) {
	if raw == "" {
		return nil, FieldMissingInSource
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, FieldInvalidFormat
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, FieldRedactedForSafety
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	safe := parsed.String()
	return &safe, FieldPresent
}

func ScanOrdinaryOutput(raw []byte) error {
	switch {
	case pemPrivateKeyPattern.Match(raw):
		return errors.New("ordinary output contains a private-key marker")
	case credentialPattern.Match(raw):
		return errors.New("ordinary output contains a credential marker")
	case queryURLPattern.Match(raw):
		return errors.New("ordinary output contains a URL query")
	default:
		return nil
	}
}

func RedactString(raw string) (*string, FieldStatus) {
	if raw == "" {
		return nil, FieldMissingInSource
	}
	if highConfidenceSecret.MatchString(raw) || queryURLPattern.MatchString(raw) {
		return nil, FieldRedactedForSafety
	}
	value := strings.Clone(raw)
	return &value, FieldPresent
}

func countRedactions(raw []byte) RedactionCounts {
	return RedactionCounts{
		CredentialKeys: len(credentialPattern.FindAll(raw, -1)),
		QueryURLs:      len(queryURLPattern.FindAll(raw, -1)),
		PEMMarkers:     len(pemPrivateKeyPattern.FindAll(raw, -1)),
	}
}
