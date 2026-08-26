package lifecycle

import (
	"context"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/websocket"
)

const (
	stateDisabled         = "disabled"
	stateWaitingForWeChat = "waiting_for_wechat"
	stateOpening          = "opening"
	stateWaitingForClient = "waiting_for_client"
	stateHealthy          = "healthy"
	stateRecovering       = "recovering"
	stateManualRequired   = "manual_required"

	commandReload   = "channel_reload"
	commandNavigate = "channel_navigate"

	channelHost = "channels.weixin.qq.com"
)

var allowedChannelPaths = map[string]struct{}{
	"/web/pages/feed":         {},
	"/web/pages/home":         {},
	"/web/pages/profile":      {},
	"/web/pages/account/like": {},
}

// ProcessProbe reports whether the WeChat desktop process is present.
type ProcessProbe interface {
	Running(context.Context) (bool, error)
}

// PageOpener asks the installed WeChat client to open its video-channel page.
type PageOpener interface {
	Open(context.Context) error
}

// ChannelTransport exposes the generic WebSocket operations needed by the
// lifecycle policy without making the policy responsible for client storage.
type ChannelTransport interface {
	ClientStatuses() []websocket.ClientStatus
	SendCommandToMatchingClient(func(websocket.ClientStatus) bool, string, interface{}) error
}

// Config controls lifecycle polling and recovery behavior.
type Config struct {
	Enabled      bool
	PollInterval time.Duration
	StaleAfter   time.Duration
	InitialDelay time.Duration
	RetryDelays  []time.Duration
	MaxAttempts  int
	Cooldown     time.Duration
}

// Status is exposed through runtime diagnostics.
type Status struct {
	State         string `json:"state"`
	WeChatRunning bool   `json:"wechat_running"`
	LastAction    string `json:"last_action,omitempty"`
	Attempts      int    `json:"attempts"`
	LastError     string `json:"last_error,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

// Manager owns the process-aware page lifecycle state machine.
type Manager struct {
	probe     ProcessProbe
	opener    PageOpener
	transport ChannelTransport
	cfg       Config
	now       func() time.Time

	tickMu sync.Mutex
	mu     sync.RWMutex

	state              string
	wechatRunning      bool
	processKnown       bool
	lastProcessRunning bool
	unhealthySamples   int
	attempts           int
	nextRetryAt        time.Time
	cooldownUntil      time.Time
	lastAction         string
	lastError          string
	updatedAt          time.Time
}

// DefaultConfig returns the approved lifecycle timing defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:      runtime.GOOS == "windows",
		PollInterval: 10 * time.Second,
		StaleAfter:   90 * time.Second,
		InitialDelay: 2 * time.Second,
		RetryDelays:  []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second},
		MaxAttempts:  3,
		Cooldown:     5 * time.Minute,
	}
}

// NewManager creates a manager with injectable dependencies for deterministic tests.
func NewManager(probe ProcessProbe, opener PageOpener, transport ChannelTransport, cfg Config, now func() time.Time) *Manager {
	defaults := DefaultConfig()
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaults.PollInterval
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = defaults.StaleAfter
	}
	if cfg.InitialDelay < 0 {
		cfg.InitialDelay = defaults.InitialDelay
	}
	if len(cfg.RetryDelays) == 0 {
		cfg.RetryDelays = append([]time.Duration(nil), defaults.RetryDelays...)
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaults.MaxAttempts
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = defaults.Cooldown
	}
	if now == nil {
		now = time.Now
	}

	initialState := stateWaitingForWeChat
	if !cfg.Enabled {
		initialState = stateDisabled
	}
	return &Manager{
		probe:     probe,
		opener:    opener,
		transport: transport,
		cfg:       cfg,
		now:       now,
		state:     initialState,
		updatedAt: now(),
	}
}

// NewDefaultManager creates the platform-backed manager used by App.
func NewDefaultManager(transport ChannelTransport) *Manager {
	cfg := DefaultConfig()
	return NewManager(newProcessProbe(), newPageOpener(), transport, cfg, time.Now)
}

// Run starts the lifecycle ticker and returns when ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	if m == nil || !m.enabled() {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if m.cfg.InitialDelay > 0 {
		timer := time.NewTimer(m.cfg.InitialDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}

	_ = m.Tick(ctx)
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = m.Tick(ctx)
		}
	}
}

// Tick evaluates one process/page observation and performs at most one recovery action.
func (m *Manager) Tick(ctx context.Context) error {
	if m == nil || !m.enabled() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	m.tickMu.Lock()
	defer m.tickMu.Unlock()

	now := m.now()
	running, err := m.probe.Running(ctx)
	if err != nil {
		m.mu.Lock()
		m.wechatRunning = false
		m.lastError = fmt.Sprintf("process probe failed: %v", err)
		m.setStateLocked(stateWaitingForWeChat, now)
		m.mu.Unlock()
		return err
	}

	m.mu.Lock()
	wasRunning := m.processKnown && m.lastProcessRunning
	m.processKnown = true
	m.lastProcessRunning = running
	m.wechatRunning = running
	if !running {
		m.unhealthySamples = 0
		m.attempts = 0
		m.nextRetryAt = time.Time{}
		m.cooldownUntil = time.Time{}
		m.lastAction = ""
		m.lastError = ""
		m.setStateLocked(stateWaitingForWeChat, now)
		m.mu.Unlock()
		return nil
	}
	newlyRunning := !wasRunning
	if newlyRunning {
		m.unhealthySamples = 0
		m.attempts = 0
		m.nextRetryAt = time.Time{}
		m.cooldownUntil = time.Time{}
		m.lastAction = "open"
		m.lastError = ""
		m.setStateLocked(stateOpening, now)
	}
	m.mu.Unlock()

	if newlyRunning {
		if err := m.open(ctx, now); err != nil {
			return err
		}
	}

	statuses := m.transport.ClientStatuses()
	candidate, healthy := selectChannelStatus(statuses, now, m.cfg.StaleAfter)
	if healthy {
		m.mu.Lock()
		m.unhealthySamples = 0
		m.attempts = 0
		m.nextRetryAt = time.Time{}
		m.cooldownUntil = time.Time{}
		m.lastError = ""
		m.setStateLocked(stateHealthy, now)
		m.mu.Unlock()
		return nil
	}

	if candidate {
		// A page candidate exists but is not ready; recovery can still ask it to reload.
		m.mu.Lock()
		m.unhealthySamples++
		samples := m.unhealthySamples
		m.setStateLocked(stateWaitingForClient, now)
		m.mu.Unlock()
		if samples < 2 {
			return nil
		}
	} else {
		m.mu.Lock()
		m.unhealthySamples++
		samples := m.unhealthySamples
		m.setStateLocked(stateWaitingForClient, now)
		m.mu.Unlock()
		if samples < 2 {
			return nil
		}
	}

	m.mu.Lock()
	if !m.cooldownUntil.IsZero() && now.Before(m.cooldownUntil) {
		m.setStateLocked(stateManualRequired, now)
		m.mu.Unlock()
		return nil
	}
	if !m.nextRetryAt.IsZero() && now.Before(m.nextRetryAt) {
		m.setStateLocked(stateRecovering, now)
		m.mu.Unlock()
		return nil
	}
	if m.attempts >= m.cfg.MaxAttempts {
		m.cooldownUntil = now.Add(m.cfg.Cooldown)
		m.setStateLocked(stateManualRequired, now)
		m.mu.Unlock()
		return nil
	}
	m.attempts++
	attempt := m.attempts
	m.setStateLocked(stateRecovering, now)
	m.mu.Unlock()

	var actionErr error
	if candidate {
		actionErr = m.transport.SendCommandToMatchingClient(isChannelPage, commandReload, map[string]interface{}{
			"reason":  "wx_channel unhealthy",
			"attempt": attempt,
		})
		m.mu.Lock()
		m.lastAction = commandReload
	} else {
		actionErr = m.opener.Open(ctx)
		m.mu.Lock()
		m.lastAction = "open"
	}
	if actionErr != nil {
		m.lastError = actionErr.Error()
	} else {
		m.lastError = ""
	}
	delay := m.cfg.RetryDelays[len(m.cfg.RetryDelays)-1]
	if attempt-1 < len(m.cfg.RetryDelays) {
		delay = m.cfg.RetryDelays[attempt-1]
	}
	m.nextRetryAt = now.Add(delay)
	m.updatedAt = now
	m.mu.Unlock()
	return actionErr
}

func (m *Manager) open(ctx context.Context, now time.Time) error {
	err := m.opener.Open(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAction = "open"
	m.updatedAt = now
	if err != nil {
		m.lastError = err.Error()
		m.setStateLocked(stateWaitingForClient, now)
	} else {
		m.lastError = ""
		m.setStateLocked(stateWaitingForClient, now)
	}
	return err
}

func (m *Manager) enabled() bool {
	return m != nil && m.cfg.Enabled && m.probe != nil && m.opener != nil && m.transport != nil
}

func (m *Manager) setStateLocked(state string, now time.Time) {
	m.state = state
	m.updatedAt = now
}

// Snapshot returns a race-safe diagnostic copy.
func (m *Manager) Snapshot() Status {
	if m == nil {
		return Status{State: stateDisabled}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := Status{
		State:         m.state,
		WeChatRunning: m.wechatRunning,
		LastAction:    m.lastAction,
		Attempts:      m.attempts,
		LastError:     m.lastError,
	}
	if !m.updatedAt.IsZero() {
		status.UpdatedAt = m.updatedAt.Format(time.RFC3339)
	}
	return status
}

func selectChannelStatus(statuses []websocket.ClientStatus, now time.Time, staleAfter time.Duration) (candidate, healthy bool) {
	for _, status := range statuses {
		if !isChannelPage(status) {
			continue
		}
		candidate = true
		if status.APIReady && statusFresh(status, now, staleAfter) {
			return true, true
		}
	}
	return candidate, false
}

func statusFresh(status websocket.ClientStatus, now time.Time, staleAfter time.Duration) bool {
	for _, raw := range []string{status.LastPongAt, status.LastPingAt, status.LastSeenAt} {
		if raw == "" {
			continue
		}
		seen, err := time.Parse(time.RFC3339, raw)
		if err == nil && !seen.After(now) && now.Sub(seen) <= staleAfter {
			return true
		}
	}
	return false
}

func isChannelPage(status websocket.ClientStatus) bool {
	parsed, err := url.Parse(strings.TrimSpace(status.Href))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Hostname(), channelHost) {
		return false
	}
	_, ok := allowedChannelPaths[parsed.Path]
	return ok
}
