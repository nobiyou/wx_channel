package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"wx_channel/internal/websocket"
)

type fakeProbe struct {
	running bool
	err     error
}

func (p *fakeProbe) Running(context.Context) (bool, error) {
	return p.running, p.err
}

type fakeOpener struct {
	calls int
	err   error
}

func (o *fakeOpener) Open(context.Context) error {
	o.calls++
	return o.err
}

type sentCommand struct {
	action  string
	payload interface{}
}

type fakeTransport struct {
	statuses []websocket.ClientStatus
	commands []sentCommand
	err      error
}

func (t *fakeTransport) ClientStatuses() []websocket.ClientStatus {
	return append([]websocket.ClientStatus(nil), t.statuses...)
}

func (t *fakeTransport) SendCommandToMatchingClient(predicate func(websocket.ClientStatus) bool, action string, payload interface{}) error {
	if predicate == nil {
		return errors.New("nil predicate")
	}
	for _, status := range t.statuses {
		if predicate(status) {
			t.commands = append(t.commands, sentCommand{action: action, payload: payload})
			return t.err
		}
	}
	return errors.New("no matching websocket client")
}

func testManager(probe *fakeProbe, opener *fakeOpener, transport *fakeTransport, now *time.Time) *Manager {
	cfg := Config{
		Enabled:      true,
		PollInterval: time.Second,
		StaleAfter:   90 * time.Second,
		InitialDelay: 0,
		RetryDelays:  []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second},
		MaxAttempts:  3,
		Cooldown:     5 * time.Minute,
	}
	return NewManager(probe, opener, transport, cfg, func() time.Time { return *now })
}

func readyStatus(now time.Time) websocket.ClientStatus {
	return websocket.ClientStatus{
		PagePath:         "/web/pages/home",
		Href:             "https://channels.weixin.qq.com/web/pages/home",
		APIReady:         true,
		Fresh:            true,
		ApplicationFresh: true,
		LastPingAt:       now.Format(time.RFC3339),
		LastSeenAt:       now.Format(time.RFC3339),
	}
}

func candidateStatus(now time.Time) websocket.ClientStatus {
	status := readyStatus(now)
	status.APIReady = false
	return status
}

func TestManagerOpensReadyClientWithoutStartupRefresh(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	probe := &fakeProbe{running: true}
	opener := &fakeOpener{}
	transport := &fakeTransport{statuses: []websocket.ClientStatus{readyStatus(now)}}
	manager := testManager(probe, opener, transport, &now)

	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	if opener.calls != 1 {
		t.Fatalf("opener calls = %d, want 1", opener.calls)
	}
	if len(transport.commands) != 0 {
		t.Fatalf("startup commands = %#v, want no refresh", transport.commands)
	}
	if got := manager.Snapshot().State; got != stateHealthy {
		t.Fatalf("state = %s, want %s", got, stateHealthy)
	}

	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}
	if opener.calls != 1 || len(transport.commands) != 0 {
		t.Fatalf("steady healthy state repeated action: opener=%d commands=%d", opener.calls, len(transport.commands))
	}
	if got := manager.Snapshot().LastAction; got != "open" {
		t.Fatalf("last action = %s, want open", got)
	}
}

func TestManagerWaitsWithoutStartingWeChat(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	probe := &fakeProbe{running: false}
	opener := &fakeOpener{}
	transport := &fakeTransport{}
	manager := testManager(probe, opener, transport, &now)

	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if opener.calls != 0 {
		t.Fatalf("opener calls = %d, want 0", opener.calls)
	}
	status := manager.Snapshot()
	if status.State != stateWaitingForWeChat || status.WeChatRunning {
		t.Fatalf("status = %#v, want waiting_for_wechat and false", status)
	}
}

func TestManagerDebouncesUnhealthyPageBeforeReload(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	probe := &fakeProbe{running: true}
	opener := &fakeOpener{}
	transport := &fakeTransport{statuses: []websocket.ClientStatus{candidateStatus(now)}}
	manager := testManager(probe, opener, transport, &now)

	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("startup Tick() error = %v", err)
	}
	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("debounce Tick() error = %v", err)
	}
	if len(transport.commands) != 1 || transport.commands[0].action != commandReload {
		t.Fatalf("commands = %#v, want one recovery reload", transport.commands)
	}
	if got := manager.Snapshot().Attempts; got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}

	now = now.Add(2 * time.Second)
	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("retry Tick() error = %v", err)
	}
	if len(transport.commands) != 2 {
		t.Fatalf("commands after retry = %d, want 2", len(transport.commands))
	}
}

func TestManagerOpensAgainWhenNoPageCandidateExists(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	probe := &fakeProbe{running: true}
	opener := &fakeOpener{}
	transport := &fakeTransport{}
	manager := testManager(probe, opener, transport, &now)

	_ = manager.Tick(context.Background())
	if opener.calls != 1 {
		t.Fatalf("startup opener calls = %d, want 1", opener.calls)
	}
	_ = manager.Tick(context.Background())
	if opener.calls != 2 {
		t.Fatalf("recovery opener calls = %d, want 2", opener.calls)
	}
	if manager.Snapshot().LastAction != "open" {
		t.Fatalf("last action = %s, want open", manager.Snapshot().LastAction)
	}
}

func TestManagerEntersCooldownAfterMaxAttempts(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	probe := &fakeProbe{running: true}
	opener := &fakeOpener{}
	transport := &fakeTransport{}
	manager := testManager(probe, opener, transport, &now)
	manager.cfg.MaxAttempts = 1
	manager.cfg.RetryDelays = []time.Duration{time.Second}

	_ = manager.Tick(context.Background())
	_ = manager.Tick(context.Background())
	if opener.calls != 2 {
		t.Fatalf("opener calls = %d, want startup plus one recovery", opener.calls)
	}
	now = now.Add(time.Second)
	_ = manager.Tick(context.Background())
	if got := manager.Snapshot().State; got != stateManualRequired {
		t.Fatalf("state = %s, want %s", got, stateManualRequired)
	}
	now = now.Add(time.Minute)
	_ = manager.Tick(context.Background())
	if opener.calls != 2 {
		t.Fatalf("cooldown caused opener calls = %d, want 2", opener.calls)
	}
}

func TestManagerRejectsStaleReadyPage(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	stale := readyStatus(now.Add(-91 * time.Second))
	probe := &fakeProbe{running: true}
	opener := &fakeOpener{}
	transport := &fakeTransport{statuses: []websocket.ClientStatus{stale}}
	manager := testManager(probe, opener, transport, &now)

	_ = manager.Tick(context.Background())
	_ = manager.Tick(context.Background())
	if len(transport.commands) != 1 {
		t.Fatalf("stale recovery commands = %d, want 1", len(transport.commands))
	}
	if manager.Snapshot().State != stateRecovering {
		t.Fatalf("state = %s, want %s", manager.Snapshot().State, stateRecovering)
	}
}

func TestManagerDoesNotTreatProtocolPongAsApplicationFresh(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	stale := readyStatus(now.Add(-91 * time.Second))
	stale.Fresh = false
	stale.ApplicationFresh = false
	stale.ProtocolFresh = true
	stale.LastPongAt = now.Format(time.RFC3339)
	probe := &fakeProbe{running: true}
	opener := &fakeOpener{}
	transport := &fakeTransport{statuses: []websocket.ClientStatus{stale}}
	manager := testManager(probe, opener, transport, &now)

	_ = manager.Tick(context.Background())
	_ = manager.Tick(context.Background())
	if len(transport.commands) != 0 || opener.calls != 2 {
		t.Fatalf("protocol-fresh recovery = commands=%#v opener=%d, want external opener fallback", transport.commands, opener.calls)
	}
}

func TestManagerReloadsAfterFunctionalProbeFailure(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	failed := readyStatus(now)
	failed.Fresh = false
	failed.APIFunctional = false
	failed.APIProbeStatus = "failed"
	failed.APIFunctionalFresh = false
	failed.APIProbeError = "功能探针超时"
	probe := &fakeProbe{running: true}
	opener := &fakeOpener{}
	transport := &fakeTransport{statuses: []websocket.ClientStatus{failed}}
	manager := testManager(probe, opener, transport, &now)

	_ = manager.Tick(context.Background())
	_ = manager.Tick(context.Background())
	if len(transport.commands) != 1 || transport.commands[0].action != commandReload {
		t.Fatalf("functional failure recovery commands = %#v, want one reload", transport.commands)
	}
}

func TestManagerSnapshotIncludesProbeError(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.Local)
	probe := &fakeProbe{err: errors.New("access denied")}
	manager := testManager(probe, &fakeOpener{}, &fakeTransport{}, &now)

	if err := manager.Tick(context.Background()); err == nil {
		t.Fatal("Tick() error = nil, want probe error")
	}
	status := manager.Snapshot()
	if status.State != stateWaitingForWeChat || status.LastError == "" {
		t.Fatalf("status = %#v, want waiting state with error", status)
	}
}

func TestNewDefaultManagerWithAutoOpenDisabled(t *testing.T) {
	manager := NewDefaultManagerWithAutoOpen(&fakeTransport{}, false)
	if manager == nil {
		t.Fatal("manager is nil")
	}
	if manager.cfg.Enabled {
		t.Fatal("auto-open disabled manager must not be enabled")
	}
	if got := manager.Snapshot().State; got != stateDisabled {
		t.Fatalf("state = %s, want %s", got, stateDisabled)
	}
}
