package api

import "wx_channel/internal/websocket"

const (
	channelPlatform       = "wxchannels"
	channelPageStatusKey  = "wxchannels:page"
	channelShareStatusKey = "wxchannels:sph"
)

type channelPlatformStatus struct {
	Platform     string `json:"platform"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Available    bool   `json:"available"`
	Reason       string `json:"reason,omitempty"`
	FreshClients int    `json:"fresh_clients"`
	ReadyClients int    `json:"ready_clients"`
}

func buildChannelPlatformStatuses(clientStatuses []websocket.ClientStatus) []channelPlatformStatus {
	freshClients := 0
	pageReadyClients := 0
	shareReadyClients := 0
	for _, client := range clientStatuses {
		if !client.Fresh {
			continue
		}
		freshClients++
		if client.APIReady {
			pageReadyClients++
		}
		if client.SupportsProfile {
			shareReadyClients++
		}
	}

	pageAvailable := pageReadyClients > 0
	shareAvailable := shareReadyClients > 0
	return []channelPlatformStatus{
		{
			Platform:     channelPlatform,
			Key:          channelPageStatusKey,
			Name:         "WeChat Channels page",
			Status:       channelStatusName(pageAvailable),
			Available:    pageAvailable,
			Reason:       channelPageStatusReason(freshClients, pageReadyClients),
			FreshClients: freshClients,
			ReadyClients: pageReadyClients,
		},
		{
			Platform:     channelPlatform,
			Key:          channelShareStatusKey,
			Name:         "WeChat Channels shared link",
			Status:       channelStatusName(shareAvailable),
			Available:    shareAvailable,
			Reason:       channelShareStatusReason(freshClients, shareReadyClients),
			FreshClients: freshClients,
			ReadyClients: shareReadyClients,
		},
	}
}

func channelStatusName(available bool) string {
	if available {
		return "available"
	}
	return "unavailable"
}

func channelPageStatusReason(freshClients, readyClients int) string {
	if freshClients == 0 {
		return "waiting for a fresh WeChat Channels page connection"
	}
	if readyClients == 0 {
		return "page API is not initialized"
	}
	return ""
}

func channelShareStatusReason(freshClients, readyClients int) string {
	if freshClients == 0 {
		return "waiting for a fresh WeChat Channels page connection"
	}
	if readyClients == 0 {
		return "shared-link API is not available on the connected page"
	}
	return ""
}
