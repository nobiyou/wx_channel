package poc

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net"
	"strconv"
	"strings"
	"sync"

	SunnyNet "github.com/qtgolang/SunnyNet/SunnyNet"
	"github.com/qtgolang/SunnyNet/public"
)

const weChatProcessName = "WeChatAppEx.exe"

type sunnyCore interface {
	ConfigureLoopbackPort(int) error
	SetHTTPCallback(func(*SunnyNet.HttpConn))
	StartListener() error
	AddProcessName(string) error
	StartProcess() error
	DeleteProcessName(string) error
	CloseListener() error
	StopProcess(bool) error
}

type realSunnyCore struct {
	sunny *SunnyNet.Sunny
}

func newRealSunnyCore(sunny *SunnyNet.Sunny) sunnyCore {
	return &realSunnyCore{sunny: sunny}
}

func (r *realSunnyCore) ConfigureLoopbackPort(port int) error {
	if r == nil || r.sunny == nil || port < 1 || port > 65535 {
		return errors.New("invalid SunnyNet loopback configuration")
	}
	r.sunny.SetPort(port).SetLoopbackOnly()
	return nil
}

func (r *realSunnyCore) SetHTTPCallback(callback func(*SunnyNet.HttpConn)) {
	r.sunny.SetGoCallback(callback, nil, nil, nil)
}

func (r *realSunnyCore) StartListener() error {
	r.sunny.Start()
	if r.sunny.Error != nil {
		return errors.New("start SunnyNet loopback listener")
	}
	return nil
}

func (r *realSunnyCore) AddProcessName(name string) error {
	if name != weChatProcessName {
		return errors.New("process name is not allowed")
	}
	r.sunny.ProcessAddName(name)
	return nil
}

func (r *realSunnyCore) StartProcess() error {
	if !r.sunny.StartProcess() {
		return errors.New("start SunnyNet process rule")
	}
	return nil
}

func (r *realSunnyCore) DeleteProcessName(name string) error {
	if name != weChatProcessName {
		return errors.New("process name is not allowed")
	}
	r.sunny.ProcessDelName(name)
	return nil
}

func (r *realSunnyCore) CloseListener() error {
	r.sunny.Close()
	return nil
}

func (r *realSunnyCore) StopProcess(unregister bool) error {
	return r.sunny.StopProcess(unregister)
}

type Proxy struct {
	sunny   sunnyCore
	address string
	script  []byte
	config  BridgeConfig

	mu               sync.Mutex
	listenerStarted  bool
	processNameAdded bool
	processStarted   bool
	processNeedsStop bool
}

func NewProxy(sunny sunnyCore, address string) *Proxy {
	return &Proxy{sunny: sunny, address: address}
}

func (p *Proxy) ConfigureInjector(script []byte, config BridgeConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.script = append([]byte(nil), script...)
	p.config = config
}

func (p *Proxy) StartListener() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sunny == nil {
		return errors.New("SunnyNet adapter is missing")
	}
	if p.listenerStarted {
		return nil
	}
	host, portText, err := net.SplitHostPort(p.address)
	if err != nil || host != "127.0.0.1" {
		return errors.New("proxy listener must be IPv4 loopback")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("invalid proxy listener port")
	}
	if err := p.sunny.ConfigureLoopbackPort(port); err != nil {
		return err
	}
	if len(p.script) > 0 {
		p.sunny.SetHTTPCallback(p.httpCallback)
	}
	if err := p.sunny.StartListener(); err != nil {
		return err
	}
	p.listenerStarted = true
	return nil
}

func (p *Proxy) StartProcessRule() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.listenerStarted {
		return errors.New("proxy listener is not started")
	}
	if p.processStarted {
		return nil
	}
	if err := p.sunny.AddProcessName(weChatProcessName); err != nil {
		return err
	}
	p.processNameAdded = true
	p.processNeedsStop = true
	if err := p.sunny.StartProcess(); err != nil {
		if cleanupErr := p.sunny.DeleteProcessName(weChatProcessName); cleanupErr != nil {
			return errors.Join(err, cleanupErr)
		}
		p.processNameAdded = false
		return err
	}
	p.processStarted = true
	return nil
}

func (p *Proxy) Cleanup(unregisterDriver bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var cleanupErrors []error
	if p.processNameAdded {
		if err := p.sunny.DeleteProcessName(weChatProcessName); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			p.processNameAdded = false
			p.processStarted = false
		}
	}
	if p.listenerStarted {
		if err := p.sunny.CloseListener(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			p.listenerStarted = false
		}
	}
	if p.processNeedsStop && !p.processNameAdded {
		if err := p.sunny.StopProcess(unregisterDriver); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			p.processNeedsStop = false
		}
	}
	return errors.Join(cleanupErrors...)
}

func (p *Proxy) httpCallback(conn *SunnyNet.HttpConn) {
	if conn == nil || conn.Request == nil || conn.Request.URL == nil || conn.Request.URL.Hostname() != "channels.weixin.qq.com" {
		return
	}
	if _, approved := approvedPagePaths[conn.Request.URL.Path]; !approved {
		return
	}
	if conn.Type == public.HttpSendRequest {
		conn.Request.Header.Del("Accept-Encoding")
		return
	}
	if conn.Type != public.HttpResponseOK || conn.Response == nil || conn.Response.Body == nil {
		return
	}
	mediaType, _, err := mime.ParseMediaType(conn.Response.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "text/html") || conn.Response.Header.Get("Content-Encoding") != "" {
		return
	}
	const maxHTMLBytes = 8 << 20
	originalBody := conn.Response.Body
	body, err := io.ReadAll(io.LimitReader(originalBody, maxHTMLBytes+1))
	if err != nil || len(body) > maxHTMLBytes {
		conn.Response.Body = &combinedReadCloser{Reader: io.MultiReader(bytes.NewReader(body), originalBody), Closer: originalBody}
		return
	}
	_ = originalBody.Close()
	modified, changed := InjectHTML(conn.Request.URL.Hostname(), conn.Request.URL.Path, body, p.script, p.config)
	if !changed {
		conn.Response.Body = io.NopCloser(bytes.NewReader(body))
		return
	}
	conn.Response.Body = io.NopCloser(bytes.NewReader(modified))
	conn.Response.ContentLength = int64(len(modified))
	conn.Response.Header.Set("Content-Length", strconv.Itoa(len(modified)))
}

type combinedReadCloser struct {
	io.Reader
	io.Closer
}
