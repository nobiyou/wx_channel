package websocket

import (
	json "github.com/json-iterator/go"
	"strings"
	"testing"
	"time"
)

func TestClientLoginStateClearsIdentityAfterLogout(t *testing.T) {
	client, cancel := newTestAPIClient("login")
	defer cancel()
	client.UpdateState(ClientStateBody{Href: "https://channels.weixin.qq.com/web/pages/home", FinderUsername: "own-account"})
	if client.Status().FinderUsername != "own-account" {
		t.Fatal("authenticated identity was lost")
	}
	client.UpdateState(ClientStateBody{Href: "https://channels.weixin.qq.com/platform/login", FinderUsername: "old-account"})
	if client.Status().FinderUsername != "" {
		t.Fatal("login route must clear the previous account")
	}
}

func TestLegacyQRCodePayloadCannotReintroduceRemoteQRRendering(t *testing.T) {
	var state ClientStateBody
	if err := json.Unmarshal([]byte(`{"href":"https://channels.weixin.qq.com/platform/login","loginQRCode":"data:image/png;base64,aGVsbG8=","loginQRCodeURL":"https://open.weixin.qq.com/connect/qrconnect?appid=test"}`), &state); err != nil {
		t.Fatal(err)
	}
	client, cancel := newTestAPIClient("login")
	defer cancel()
	client.UpdateState(state)
	data, err := json.Marshal(client.Status())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "login_qr") || strings.Contains(string(data), "qrconnect") {
		t.Fatal("retired QR data leaked into client status")
	}
}

func TestClientOnlyAcceptsIdentityFromTrustedChannelPages(t *testing.T) {
	for _, href := range []string{"https://mp.weixin.qq.com/s/article", "https://channels.weixin.qq.com.evil.example/web/pages/home", "https://user:pass@channels.weixin.qq.com/web/pages/home", "https://channels.weixin.qq.com:444/web/pages/home", "http://channels.weixin.qq.com/web/pages/home"} {
		client, cancel := newTestAPIClient("login")
		client.UpdateState(ClientStateBody{Href: href, FinderUsername: "wrong-account"})
		if client.Status().FinderUsername != "" {
			t.Errorf("untrusted identity accepted from %s", href)
		}
		cancel()
	}
}

func TestStaleClientStatusDoesNotExposeOldIdentity(t *testing.T) {
	client, cancel := newTestAPIClient("login")
	defer cancel()
	client.UpdateState(ClientStateBody{Href: "https://channels.weixin.qq.com/web/pages/home", FinderUsername: "old-account"})
	if client.statusAt(time.Now().Add(2*defaultLivenessTimeout), defaultLivenessTimeout).FinderUsername != "" {
		t.Fatal("stale identity must not be offered as authenticated")
	}
}
