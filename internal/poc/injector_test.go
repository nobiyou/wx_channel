package poc

import (
	"bytes"
	"testing"
)

func TestInjectHTMLOnlyOnApprovedChannelsPages(t *testing.T) {
	in := []byte("<html><head></head><body></body></html>")
	got, changed := InjectHTML("channels.weixin.qq.com", "/web/pages/home", in, []byte("window.poc=true"), BridgeConfig{Port: 2026, Token: "secret"})
	if !changed || !bytes.Contains(got, []byte("window.__WX_CHANNEL_POC_CONFIG__")) {
		t.Fatalf("not injected: %s", got)
	}
	if _, changed := InjectHTML("evil.example", "/web/pages/home", in, nil, BridgeConfig{}); changed {
		t.Fatal("foreign host injected")
	}
	if _, changed := InjectHTML("channels.weixin.qq.com", "/unapproved", in, nil, BridgeConfig{}); changed {
		t.Fatal("unapproved path injected")
	}
}

func TestInjectHTMLEscapesConfig(t *testing.T) {
	in := []byte("<html><head></head></html>")
	got, changed := InjectHTML("channels.weixin.qq.com", "/web/pages/feed", in, []byte("window.poc=true"), BridgeConfig{Port: 2026, Token: `</script><script>bad()`})
	if !changed {
		t.Fatal("approved page was not injected")
	}
	if bytes.Contains(got, []byte(`</script><script>bad()`)) {
		t.Fatalf("config escaped script context: %s", got)
	}
}
