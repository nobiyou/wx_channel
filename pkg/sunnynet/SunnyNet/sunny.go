package SunnyNet

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/elazarl/goproxy"
)

// HttpConn represents a proxied HTTP connection.
// Compatible with the original SunnyNet HttpConn type.
type HttpConn struct {
	Type     int
	Request  *http.Request
	Response *http.Response

	// Internal: whether StopRequest was called
	stopped    bool
	stopCode   int
	stopBody   string
	stopHeader http.Header
}

// StopRequest halts further processing and sends a synthetic response.
func (c *HttpConn) StopRequest(statusCode int, body string, headers http.Header) {
	c.stopped = true
	c.stopCode = statusCode
	c.stopBody = body
	c.stopHeader = headers
}

// IsStopped returns whether StopRequest was called.
func (c *HttpConn) IsStopped() bool {
	return c.stopped
}

// Sunny is the main MITM proxy engine.
// On macOS, it uses goproxy for HTTPS interception.
type Sunny struct {
	Error    error
	port     int
	callback func(*HttpConn)
	proxy    *goproxy.ProxyHttpServer
	mu       sync.Mutex
}

// NewSunny creates a new Sunny proxy instance.
func NewSunny() *Sunny {
	return &Sunny{}
}

// SetPort sets the proxy listening port.
func (s *Sunny) SetPort(port int) *Sunny {
	s.port = port
	return s
}

// GetPort returns the configured port.
func (s *Sunny) GetPort() int {
	return s.port
}

// SetCACert configures the CA certificate and key for MITM.
// Must be called before Start().
func (s *Sunny) SetCACert(certPEM, keyPEM []byte) error {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("failed to parse CA cert/key pair: %w", err)
	}
	if cert.Leaf == nil {
		cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("failed to parse CA leaf: %w", err)
		}
	}
	goproxy.GoproxyCa = cert
	return nil
}

// filteredWriter filters out noisy "Unsolicited response" log lines from Go's
// net/http transport. These occur when WeChat CDN pushes data on idle keep-alive
// connections — harmless but pollutes the console.
type filteredWriter struct {
	out io.Writer
}

func (w *filteredWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("Unsolicited response")) ||
		bytes.Contains(p, []byte("Cannot read response from mitm")) {
		return len(p), nil // swallow
	}
	return w.out.Write(p)
}

// Start starts the MITM proxy server.
func (s *Sunny) Start() *Sunny {
	log.SetOutput(&filteredWriter{out: log.Writer()})

	s.proxy = goproxy.NewProxyHttpServer()
	s.proxy.Verbose = false

	// Handle direct (non-proxy) requests so that http://127.0.0.1:<port>/console works
	s.proxy.NonproxyHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host == "" {
			r.URL.Host = r.Host
		}
		if r.URL.Scheme == "" {
			r.URL.Scheme = "http"
		}
		s.proxy.ServeHTTP(w, r)
	})

	// Enable MITM for all HTTPS connections
	s.proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)

	// Handle requests: call the callback with Type=HttpSendRequest
	s.proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if s.callback == nil {
			return req, nil
		}

		// Remove Accept-Encoding to get uncompressed response for JS injection
		req.Header.Del("Accept-Encoding")

		conn := &HttpConn{
			Type:    1, // public.HttpSendRequest
			Request: req,
		}
		s.callback(conn)

		if conn.IsStopped() {
			// Handler wants to send a synthetic response
			resp := &http.Response{
				StatusCode: conn.stopCode,
				Header:     conn.stopHeader,
				Body:       io.NopCloser(strings.NewReader(conn.stopBody)),
				Request:    req,
			}
			if resp.Header == nil {
				resp.Header = make(http.Header)
			}
			if resp.Header.Get("Content-Type") == "" {
				resp.Header.Set("Content-Type", "application/json; charset=utf-8")
			}
			return req, resp
		}

		return req, nil
	})

	// Handle responses: call the callback with Type=HttpResponseOK
	s.proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if s.callback == nil || resp == nil {
			return resp
		}

		conn := &HttpConn{
			Type:     2, // public.HttpResponseOK
			Request:  ctx.Req,
			Response: resp,
		}
		s.callback(conn)

		// The handlers may have modified resp.Body or resp.Header in place
		return conn.Response
	})

	// Start the proxy server in a goroutine
	addr := fmt.Sprintf(":%d", s.port)
	go func() {
		log.Printf("[MITM] Starting proxy on %s", addr)
		if err := http.ListenAndServe(addr, s.proxy); err != nil {
			log.Printf("[MITM] Proxy server error: %v", err)
			s.Error = err
		}
	}()

	return s
}

// SetGoCallback registers the HTTP interception callback.
func (s *Sunny) SetGoCallback(callback func(*HttpConn), _ interface{}, _ interface{}, _ interface{}) *Sunny {
	s.callback = callback
	return s
}

// GetCallback returns the registered callback.
func (s *Sunny) GetCallback() func(*HttpConn) {
	return s.callback
}

// ProcessAddName is a Windows-only feature (process injection). No-op on macOS.
func (s *Sunny) ProcessAddName(name string) {
	// No-op: process injection is Windows-only.
	// On macOS, system proxy is used instead.
}

// StartProcess is a Windows-only feature (process injection). No-op on macOS.
func (s *Sunny) StartProcess() bool {
	// No-op: process injection is Windows-only.
	return false
}

// OpenDrive is a Windows-only feature. No-op on macOS.
func (s *Sunny) OpenDrive(flag bool) bool {
	return true
}

// Ensure bytes import is used (for future use in response body handling)
var _ = bytes.NewBuffer
