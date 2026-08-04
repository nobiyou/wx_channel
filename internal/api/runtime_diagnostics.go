package api

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/config"
	"wx_channel/pkg/certificate"
)

const defaultWXClientProcessName = "WeChatAppEx.exe"
const runtimeCertificateStatusTTL = 15 * time.Second

type RuntimeDiagnostics struct {
	mu                    sync.RWMutex
	cfg                   *config.Config
	injection             RuntimeInjectionStatus
	certificate           RuntimeCertificateStatus
	certificateTTL        time.Time
	certificateRefreshing bool
	checkCertificate      func(string) (bool, error)
}

type RuntimeDiagnosticsSnapshot struct {
	Proxy       RuntimeProxyStatus       `json:"proxy"`
	Certificate RuntimeCertificateStatus `json:"certificate"`
	Injection   RuntimeInjectionStatus   `json:"injection"`
	Features    RuntimeFeatureStatus     `json:"features"`
}

type RuntimeProxyStatus struct {
	Mode    string `json:"mode"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Address string `json:"address"`
	APIPort int    `json:"api_port"`
}

type RuntimeCertificateStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Checked   bool   `json:"checked"`
	Error     string `json:"error,omitempty"`
}

type RuntimeInjectionStatus struct {
	Enabled       bool   `json:"enabled"`
	TargetProcess string `json:"target_process,omitempty"`
	Checked       bool   `json:"checked"`
	Started       bool   `json:"started"`
	LastError     string `json:"last_error,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type RuntimeFeatureStatus struct {
	RadarEnabled   bool `json:"radar_enabled"`
	CloudEnabled   bool `json:"cloud_enabled"`
	MetricsEnabled bool `json:"metrics_enabled"`
}

func NewRuntimeDiagnostics(cfg *config.Config) *RuntimeDiagnostics {
	target := defaultWXClientProcessName
	enabled := runtime.GOOS == "windows"
	return &RuntimeDiagnostics{
		cfg: cfg,
		injection: RuntimeInjectionStatus{
			Enabled:       enabled,
			TargetProcess: target,
		},
		checkCertificate: certificate.CheckCertificate,
	}
}

func (d *RuntimeDiagnostics) RecordInjectionResult(started bool, lastError string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.injection.Enabled = runtime.GOOS == "windows"
	if d.injection.TargetProcess == "" {
		d.injection.TargetProcess = defaultWXClientProcessName
	}
	d.injection.Checked = true
	d.injection.Started = started
	d.injection.LastError = strings.TrimSpace(lastError)
	d.injection.UpdatedAt = time.Now().Format(time.RFC3339)
}

func (d *RuntimeDiagnostics) Snapshot() RuntimeDiagnosticsSnapshot {
	var cfg *config.Config
	if d != nil {
		cfg = d.cfg
	}
	if cfg == nil {
		cfg = config.Get()
	}

	port := 2025
	features := RuntimeFeatureStatus{}
	if cfg != nil {
		if cfg.Port > 0 {
			port = cfg.Port
		}
		features = RuntimeFeatureStatus{
			RadarEnabled:   cfg.RadarEnabled,
			CloudEnabled:   cfg.CloudEnabled,
			MetricsEnabled: cfg.MetricsEnabled,
		}
	}

	proxyMode := "system_proxy"
	if runtime.GOOS == "windows" {
		proxyMode = "process_proxy"
	}

	injection := RuntimeInjectionStatus{
		Enabled:       runtime.GOOS == "windows",
		TargetProcess: defaultWXClientProcessName,
	}
	if d != nil {
		d.mu.RLock()
		injection = d.injection
		d.mu.RUnlock()
		if injection.TargetProcess == "" {
			injection.TargetProcess = defaultWXClientProcessName
		}
	}

	certStatus := d.certificateStatus()

	return RuntimeDiagnosticsSnapshot{
		Proxy: RuntimeProxyStatus{
			Mode:    proxyMode,
			Host:    "127.0.0.1",
			Port:    port,
			Address: fmt.Sprintf("127.0.0.1:%d", port),
			APIPort: port + 1,
		},
		Certificate: certStatus,
		Injection:   injection,
		Features:    features,
	}
}

func (d *RuntimeDiagnostics) certificateStatus() RuntimeCertificateStatus {
	if d == nil {
		return checkRuntimeCertificateStatus(certificate.CheckCertificate)
	}

	now := time.Now()
	d.mu.RLock()
	if !d.certificateTTL.IsZero() && now.Before(d.certificateTTL) {
		status := d.certificate
		d.mu.RUnlock()
		return status
	}
	cached := d.certificate
	hasCached := !d.certificateTTL.IsZero()
	refreshing := d.certificateRefreshing
	d.mu.RUnlock()

	if !refreshing {
		d.refreshCertificateStatusAsync()
	}
	if hasCached {
		return cached
	}
	return RuntimeCertificateStatus{
		Name:  "SunnyNet",
		Error: "certificate check pending",
	}
}

func (d *RuntimeDiagnostics) refreshCertificateStatusAsync() {
	if d == nil {
		return
	}

	d.mu.Lock()
	if d.certificateRefreshing {
		d.mu.Unlock()
		return
	}
	d.certificateRefreshing = true
	checkCertificate := d.checkCertificate
	d.mu.Unlock()

	go func() {
		status := checkRuntimeCertificateStatus(checkCertificate)
		now := time.Now()
		d.mu.Lock()
		d.certificate = status
		d.certificateTTL = now.Add(runtimeCertificateStatusTTL)
		d.certificateRefreshing = false
		d.mu.Unlock()
	}()
}

func checkRuntimeCertificateStatus(checkCertificate func(string) (bool, error)) RuntimeCertificateStatus {
	if checkCertificate == nil {
		checkCertificate = certificate.CheckCertificate
	}

	installed, certErr := checkCertificate("SunnyNet")
	status := RuntimeCertificateStatus{Name: "SunnyNet", Installed: installed, Checked: true}
	if certErr != nil {
		status.Error = certErr.Error()
	}
	return status
}
