package mcpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// sanitizeJSONPayload keeps live responses useful while removing credentials,
// keys, signed download URLs, and request metadata before they reach MCP.
func sanitizeJSONPayload(raw []byte) (any, int, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, 0, fmt.Errorf("decode live response: %w", err)
	}
	redacted := 0
	value = sanitizeValue(value, "", &redacted)
	return value, redacted, nil
}

func sanitizeValue(value any, key string, redacted *int) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			if isSensitiveKey(childKey) {
				*redacted++
				continue
			}
			result[childKey] = sanitizeValue(childValue, childKey, redacted)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			result[i] = sanitizeValue(child, key, redacted)
		}
		return result
	case string:
		if isSensitiveURLKey(key) {
			if sanitized, ok := sanitizeURL(typed); ok {
				*redacted++
				return sanitized
			}
		}
		return typed
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
	if normalized == "" {
		return false
	}
	for _, token := range []string{
		"decryptkey", "decodekey", "encfilekey", "exportkey", "passticket", "authorization", "cookie", "referer", "signature",
	} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	// Token-like values are sensitive even when the upstream field uses a
	// nested or abbreviated name. Avoid matching ordinary titles such as
	// "tokenization" by requiring the field to be short and key-like.
	return strings.HasSuffix(normalized, "token") || strings.HasPrefix(normalized, "token")
}

func isSensitiveURLKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "url") || lower == "link" || lower == "href"
}

func sanitizeURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw, false
	}
	if parsed.User != nil || strings.EqualFold(parsed.Hostname(), "finder.video.qq.com") || strings.Contains(strings.ToLower(parsed.Path), "stodownload") {
		return "[redacted_signed_url]", true
	}
	query := parsed.Query()
	for key := range query {
		if isSensitiveKey(key) || strings.EqualFold(key, "auth_key") || strings.EqualFold(key, "key") {
			return "[redacted_signed_url]", true
		}
	}
	return raw, false
}
