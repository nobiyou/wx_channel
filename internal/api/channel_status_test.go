package api

import (
	"testing"

	"wx_channel/internal/websocket"
)

func TestBuildChannelPlatformStatusesSeparatesPageAndShareReadiness(t *testing.T) {
	statuses := buildChannelPlatformStatuses([]websocket.ClientStatus{
		{Fresh: true, APIReady: true, SupportsProfile: true},
		{Fresh: true, APIReady: true},
		{Fresh: false, APIReady: true, SupportsProfile: true},
	})

	if len(statuses) != 2 {
		t.Fatalf("status count = %d, want 2", len(statuses))
	}
	if statuses[0].Key != channelPageStatusKey || !statuses[0].Available {
		t.Fatalf("page status = %#v, want available", statuses[0])
	}
	if statuses[0].FreshClients != 2 || statuses[0].ReadyClients != 2 {
		t.Fatalf("page counters = %#v, want fresh=2 ready=2", statuses[0])
	}
	if statuses[1].Key != channelShareStatusKey || !statuses[1].Available {
		t.Fatalf("share status = %#v, want available", statuses[1])
	}
	if statuses[1].FreshClients != 2 || statuses[1].ReadyClients != 1 {
		t.Fatalf("share counters = %#v, want fresh=2 ready=1", statuses[1])
	}
}

func TestBuildChannelPlatformStatusesReportsStaleReason(t *testing.T) {
	statuses := buildChannelPlatformStatuses([]websocket.ClientStatus{
		{Fresh: false, APIReady: true, SupportsProfile: true},
	})

	for _, status := range statuses {
		if status.Available {
			t.Fatalf("status %s unexpectedly available: %#v", status.Key, status)
		}
		if status.Reason != "waiting for a fresh WeChat Channels page connection" {
			t.Fatalf("status %s reason = %q", status.Key, status.Reason)
		}
	}
}
