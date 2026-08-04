package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/response"
	"wx_channel/internal/websocket"
)

func TestGetStatusIncludesRuntimeDiagnostics(t *testing.T) {
	hub := websocket.NewHub()
	diagnostics := NewRuntimeDiagnostics(&config.Config{
		Port:           2025,
		RadarEnabled:   false,
		CloudEnabled:   false,
		MetricsEnabled: true,
	})
	diagnostics.RecordInjectionResult(false, "administrator permission may be required")

	service := NewSearchService(hub)
	service.SetRuntimeDiagnostics(diagnostics)

	req := httptest.NewRequest(http.MethodGet, "/api/channels/status", nil)
	w := httptest.NewRecorder()
	service.GetStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map data, got %T", resp.Data)
	}
	runtimeInfo, ok := data["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime diagnostics, got %#v", data["runtime"])
	}
	proxyInfo, ok := runtimeInfo["proxy"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime.proxy, got %#v", runtimeInfo["proxy"])
	}
	if proxyInfo["address"] != "127.0.0.1:2025" {
		t.Fatalf("expected proxy address 127.0.0.1:2025, got %#v", proxyInfo["address"])
	}
	injectionInfo, ok := runtimeInfo["injection"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime.injection, got %#v", runtimeInfo["injection"])
	}
	if injectionInfo["checked"] != true || injectionInfo["started"] != false {
		t.Fatalf("expected checked failed injection status, got %#v", injectionInfo)
	}
}

func TestGetStatusDoesNotBlockOnColdCertificateCheck(t *testing.T) {
	hub := websocket.NewHub()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCheck := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	defer releaseCheck()
	diagnostics := NewRuntimeDiagnostics(&config.Config{Port: 2025})
	diagnostics.checkCertificate = func(string) (bool, error) {
		close(started)
		<-release
		return true, nil
	}

	service := NewSearchService(hub)
	service.SetRuntimeDiagnostics(diagnostics)

	req := httptest.NewRequest(http.MethodGet, "/api/channels/status", nil)
	w := httptest.NewRecorder()
	done := make(chan time.Duration, 1)
	go func() {
		startedAt := time.Now()
		service.GetStatus(w, req)
		done <- time.Since(startedAt)
	}()

	select {
	case elapsed := <-done:
		if elapsed > 200*time.Millisecond {
			t.Fatalf("status endpoint blocked on certificate check for %s", elapsed)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("status endpoint blocked on certificate check")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map data, got %T", resp.Data)
	}
	runtimeInfo, ok := data["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime diagnostics, got %#v", data["runtime"])
	}
	certificateInfo, ok := runtimeInfo["certificate"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime.certificate, got %#v", runtimeInfo["certificate"])
	}
	if certificateInfo["checked"] == true {
		t.Fatalf("expected cold certificate snapshot to be pending, got %#v", certificateInfo)
	}
	if certificateInfo["error"] != "certificate check pending" {
		t.Fatalf("expected pending certificate error, got %#v", certificateInfo)
	}

	select {
	case <-started:
		releaseCheck()
	case <-time.After(time.Second):
		t.Fatal("expected async certificate check to start")
	}
}
