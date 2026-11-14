package websocket

import (
	"net/url"
	"strings"
)

func trustedChannelURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "channels.weixin.qq.com") || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return nil, false
	}
	return u, true
}

// IsChannelLoginURL identifies the official page that owns the login session.
func IsChannelLoginURL(raw string) bool {
	u, ok := trustedChannelURL(raw)
	if !ok {
		return false
	}
	switch strings.TrimRight(u.Path, "/") {
	case "", "/platform/login", "/login", "/login.html":
		return true
	}
	return false
}

func normalizeFinderUsername(state ClientStateBody) string {
	if _, ok := trustedChannelURL(state.Href); !ok || IsChannelLoginURL(state.Href) {
		return ""
	}
	username := strings.TrimSpace(state.FinderUsername)
	if len(username) > 512 {
		return ""
	}
	return username
}
