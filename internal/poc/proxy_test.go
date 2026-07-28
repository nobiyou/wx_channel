package poc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	SunnyNet "github.com/qtgolang/SunnyNet/SunnyNet"
	"github.com/qtgolang/SunnyNet/public"
)

type fakeSunny struct {
	calls []string
}

func (f *fakeSunny) ConfigureLoopbackPort(port int) error {
	f.calls = append(f.calls, fmt.Sprintf("configure-loopback:%d", port))
	return nil
}
func (f *fakeSunny) SetHTTPCallback(func(*SunnyNet.HttpConn)) {}
func (f *fakeSunny) StartListener() error {
	f.calls = append(f.calls, "start-listener")
	return nil
}
func (f *fakeSunny) AddProcessName(name string) error {
	f.calls = append(f.calls, "add-name:"+name)
	return nil
}
func (f *fakeSunny) StartProcess() error {
	f.calls = append(f.calls, "start-process")
	return nil
}
func (f *fakeSunny) DeleteProcessName(name string) error {
	f.calls = append(f.calls, "del-name:"+name)
	return nil
}
func (f *fakeSunny) CloseListener() error {
	f.calls = append(f.calls, "close")
	return nil
}
func (f *fakeSunny) StopProcess(unregister bool) error {
	f.calls = append(f.calls, fmt.Sprintf("stop-process:%t", unregister))
	return nil
}

func TestProxyStartsLoopbackThenExactProcessRule(t *testing.T) {
	sunny := &fakeSunny{}
	proxy := NewProxy(sunny, "127.0.0.1:2025")
	if err := proxy.StartListener(); err != nil {
		t.Fatal(err)
	}
	if err := proxy.StartProcessRule(); err != nil {
		t.Fatal(err)
	}
	want := []string{"configure-loopback:2025", "start-listener", "add-name:WeChatAppEx.exe", "start-process"}
	if !reflect.DeepEqual(sunny.calls, want) {
		t.Fatalf("calls=%v want=%v", sunny.calls, want)
	}
}

func TestProxyRejectsNonLoopback(t *testing.T) {
	proxy := NewProxy(&fakeSunny{}, "0.0.0.0:2025")
	if err := proxy.StartListener(); err == nil {
		t.Fatal("accepted non-loopback listener")
	}
}

func TestProxyCleanupIsReverseAndIdempotent(t *testing.T) {
	sunny := &fakeSunny{}
	proxy := NewProxy(sunny, "127.0.0.1:2025")
	if err := proxy.StartListener(); err != nil {
		t.Fatal(err)
	}
	if err := proxy.StartProcessRule(); err != nil {
		t.Fatal(err)
	}
	sunny.calls = nil
	if err := proxy.Cleanup(true); err != nil {
		t.Fatal(err)
	}
	if err := proxy.Cleanup(true); err != nil {
		t.Fatal(err)
	}
	want := []string{"del-name:WeChatAppEx.exe", "close", "stop-process:true"}
	if !reflect.DeepEqual(sunny.calls, want) {
		t.Fatalf("calls=%v want=%v", sunny.calls, want)
	}
}

func TestProxyHTTPCallbackOnlyMutatesApprovedHTML(t *testing.T) {
	proxy := NewProxy(&fakeSunny{}, "127.0.0.1:2025")
	proxy.ConfigureInjector([]byte("window.poc=true"), BridgeConfig{Port: 2026, Token: "secret"})
	request := &http.Request{
		URL:    &url.URL{Scheme: "https", Host: "channels.weixin.qq.com", Path: "/web/pages/home"},
		Header: http.Header{"Accept-Encoding": []string{"gzip"}},
	}
	proxy.httpCallback(&SunnyNet.HttpConn{Type: public.HttpSendRequest, Request: request})
	if request.Header.Get("Accept-Encoding") != "" {
		t.Fatal("approved request retained Accept-Encoding")
	}

	response := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:   io.NopCloser(bytes.NewBufferString("<html><head></head></html>")),
	}
	proxy.httpCallback(&SunnyNet.HttpConn{Type: public.HttpResponseOK, Request: request, Response: response})
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("window.__WX_CHANNEL_POC_CONFIG__")) || response.ContentLength != int64(len(body)) {
		t.Fatalf("response was not safely injected: %s", body)
	}
}
