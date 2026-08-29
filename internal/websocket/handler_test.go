package websocket

import "testing"

func TestIsOriginAllowedAcceptsBuiltInClientOrigins(t *testing.T) {
	allowedOrigins := []string{
		"http://127.0.0.1:2025",
		"http://localhost:2025",
		"https://channels.weixin.qq.com",
		"https://mp.weixin.qq.com",
	}

	for _, origin := range allowedOrigins {
		if !isOriginAllowed(origin, allowedOrigins) {
			t.Fatalf("origin %q was rejected", origin)
		}
	}
}

func TestIsOriginAllowedRejectsUnlistedOrigin(t *testing.T) {
	allowedOrigins := []string{
		"http://127.0.0.1:2025",
		"https://channels.weixin.qq.com",
		"https://mp.weixin.qq.com",
	}

	if isOriginAllowed("https://evil.example", allowedOrigins) {
		t.Fatal("unlisted origin was accepted")
	}
}
