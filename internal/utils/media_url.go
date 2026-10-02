package utils

import (
	"net/url"
	"strings"
)

// JoinURLToken combines a base media URL with the query fragment returned by
// WeChat's media APIs. The token is commonly returned as "&token=...", but
// some clients return a leading "?", an encoded fragment, or a complete URL.
func JoinURLToken(base, token string) string {
	base = strings.TrimSpace(base)
	token = strings.TrimSpace(token)
	if base == "" {
		return token
	}
	if token == "" {
		return base
	}

	token = decodeURLTokenQuery(token)
	if isAbsoluteHTTPURL(token) {
		return token
	}

	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return joinURLTokenFallback(base, token)
	}

	queryText := strings.TrimLeft(token, "?&")
	if queryText == "" {
		return base
	}
	query, err := parseURLQueryPreservingPlus(queryText)
	if err != nil || len(query) == 0 {
		return joinURLTokenFallback(base, token)
	}
	baseQuery, err := parseURLQueryPreservingPlus(parsed.RawQuery)
	if err != nil {
		return joinURLTokenFallback(base, token)
	}
	for key, values := range query {
		baseQuery.Del(key)
		for _, value := range values {
			baseQuery.Add(key, value)
		}
	}
	parsed.RawQuery = baseQuery.Encode()
	return parsed.String()
}

func decodeURLTokenQuery(token string) string {
	if !strings.Contains(token, "%") {
		return token
	}
	decoded, err := url.PathUnescape(token)
	if err != nil {
		return token
	}
	if strings.HasPrefix(decoded, "?") || strings.HasPrefix(decoded, "&") || strings.Contains(decoded, "=") {
		return decoded
	}
	return token
}

func isAbsoluteHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
}

func joinURLTokenFallback(base, token string) string {
	token = strings.TrimLeft(token, "?&")
	if token == "" {
		return base
	}
	trimmedBase := strings.TrimRight(base, "?&")
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	if len(trimmedBase) != len(base) {
		separator = ""
	}
	return trimmedBase + separator + token
}

// parseURLQueryPreservingPlus keeps literal '+' characters in signed media
// tokens. url.ParseQuery follows HTML form semantics and turns '+' into a
// space, while WeChat's opaque tokens commonly use '+' as a base64 character.
func parseURLQueryPreservingPlus(raw string) (url.Values, error) {
	if raw == "" {
		return url.Values{}, nil
	}
	return url.ParseQuery(strings.ReplaceAll(raw, "+", "%2B"))
}
