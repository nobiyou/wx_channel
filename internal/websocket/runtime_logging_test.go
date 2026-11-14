package websocket

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"wx_channel/internal/utils"
)

func TestClientStateLoggingDoesNotRepeatHeartbeats(t *testing.T) {
	if os.Getenv("WX_CHANNEL_STATE_LOG_TEST") == "1" {
		if err := utils.InitLoggerWithRotation(utils.INFO, "state.log", 5); err != nil {
			t.Fatal(err)
		}
		defer utils.GetLogger().Close()
		client, cancel := newTestAPIClient("quiet-state")
		defer cancel()
		state := ClientStateBody{
			Href: "https://channels.weixin.qq.com/web/pages/home", PagePath: "/web/pages/home",
			APIReady: true, Methods: map[string]bool{"finderSearch": true},
		}
		client.UpdateState(state)
		firstSeen := client.lastSeen
		state.Timestamp = time.Now().UnixMilli()
		client.UpdateState(state)
		if client.lastSeen.Before(firstSeen) || client.lastPing.UnixMilli() != state.Timestamp {
			t.Fatal("quiet heartbeat did not refresh liveness")
		}
		state.APIFunctional = true
		client.UpdateState(state)
		state.Methods["finderUserPage"] = true
		client.UpdateState(state)
		client.UpdateState(state)
		if !client.SupportsKey("key:channels:feed_list") {
			t.Fatal("quiet heartbeat did not update supported methods")
		}
		content, err := os.ReadFile("state.log")
		if err != nil || strings.Count(string(content), "WebSocket 客户端状态更新") != 3 {
			t.Fatalf("expected only three meaningful changes: %s, %v", content, err)
		}
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestClientStateLoggingDoesNotRepeatHeartbeats$")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "WX_CHANNEL_STATE_LOG_TEST=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("client logging regression: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "WebSocket 客户端状态更新") {
		t.Fatalf("client diagnostics leaked to console: %s", output)
	}
}
