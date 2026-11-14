package cloud

import (
	"testing"

	hubws "wx_channel/internal/websocket"
)

func TestCapabilitySummaryPrefersAuthenticatedPageOverOldLoginTab(t *testing.T) {
	loggedIn := hubws.ClientStatus{Href: "https://channels.weixin.qq.com/web/pages/home", FinderUsername: "own-account", ApplicationFresh: true, Fresh: true, APIReady: true}
	login := hubws.ClientStatus{Href: "https://channels.weixin.qq.com/platform/login", ApplicationFresh: true, Fresh: true}
	for _, statuses := range [][]hubws.ClientStatus{{loggedIn, login}, {login, loggedIn}} {
		summary := summarizeClientCapabilities(statuses)
		if summary.FinderUsername != "own-account" {
			t.Fatal("an old login tab must not override the authenticated page")
		}
	}
}

func TestCapabilitySummaryExcludesStaleLoginDataAndCapabilities(t *testing.T) {
	stale := hubws.ClientStatus{Href: "https://channels.weixin.qq.com/platform/login", APIReady: true, SupportsSearch: true, Methods: map[string]bool{"finderSearch": true}}
	fresh := hubws.ClientStatus{Href: "https://channels.weixin.qq.com/platform/login", ApplicationFresh: true}
	summary := summarizeClientCapabilities([]hubws.ClientStatus{stale, fresh})
	if summary.Href != fresh.Href || summary.APIReady || summary.SupportsSearch || len(summary.Methods) > 0 {
		t.Fatal("stale page data leaked into heartbeat capabilities")
	}
	if summary.WSClients != 2 {
		t.Fatal("connected client count must still include stale sockets")
	}
}

func TestCapabilitySummarySelectsMostRecentLoginPageDeterministically(t *testing.T) {
	old := hubws.ClientStatus{Href: "https://channels.weixin.qq.com/platform/login?tab=old", ApplicationFresh: true, LastSeenAt: "2026-11-14T12:00:00Z"}
	current := hubws.ClientStatus{Href: "https://channels.weixin.qq.com/platform/login?tab=new", ApplicationFresh: true, LastSeenAt: "2026-11-14T12:01:00Z"}
	for _, statuses := range [][]hubws.ClientStatus{{old, current}, {current, old}} {
		if got := summarizeClientCapabilities(statuses).Href; got != current.Href {
			t.Fatalf("selected page = %q, want current login page", got)
		}
	}
}

func TestCapabilitySummaryCannotUseAnotherOriginForLogin(t *testing.T) {
	other := hubws.ClientStatus{Href: "https://mp.weixin.qq.com/s/article", FinderUsername: "wrong-account", ApplicationFresh: true, Fresh: true, APIReady: true}
	summary := summarizeClientCapabilities([]hubws.ClientStatus{other})
	if summary.FinderUsername != "" || summary.Href != "" || summary.APIReady {
		t.Fatal("another origin must not participate in video-channel login or navigation")
	}
}

func TestCapabilitySummarySelectsTheSameSocketForEqualTimestamps(t *testing.T) {
	first := hubws.ClientStatus{Href: "https://channels.weixin.qq.com/platform/login", RemoteAddr: "127.0.0.1:1001", ApplicationFresh: true, LastSeenAt: "2026-11-14T12:00:00Z"}
	second := first
	second.RemoteAddr = "127.0.0.1:1002"
	for _, statuses := range [][]hubws.ClientStatus{{first, second}, {second, first}} {
		summary := summarizeClientCapabilities(statuses)
		if summary.ClientRemoteAddr != first.RemoteAddr {
			t.Fatal("equal page observations must keep the selected page and lifecycle target on the same socket")
		}
	}
}
