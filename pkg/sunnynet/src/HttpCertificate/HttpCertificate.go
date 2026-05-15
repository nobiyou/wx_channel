package HttpCertificate

import (
	crypto "crypto/tls"
	"crypto/x509"
	"github.com/qtgolang/SunnyNet/public"
	"github.com/qtgolang/SunnyNet/src/crypto/tls"
	"net/url"
	"strings"
	"sync"
)

var Lock sync.Mutex
var Map = make(map[string]*CertificateRequestManager)

type CertificateRequestManager struct {
	Rules  uint8
	Crypto *crypto.Config //没办法，得弄两份，因为 tls 复制了一份在程序内 两个都得用
	Sunny  *tls.Config
}

func (w *CertificateRequestManager) AddRootCAs(RootCAs *x509.CertPool) {
	if w.Sunny == nil {
		w.Sunny = &tls.Config{RootCAs: RootCAs}
	} else {
		w.Sunny.RootCAs = RootCAs
	}
	if w.Crypto == nil {
		w.Crypto = &crypto.Config{RootCAs: RootCAs}
	} else {
		w.Crypto.RootCAs = RootCAs
	}
}
func (w *CertificateRequestManager) Load(ca, key string) bool {
	s, e := tls.X509KeyPair([]byte(ca), []byte(key))
	if e != nil {
		return false
	}
	c, e1 := crypto.X509KeyPair([]byte(ca), []byte(key))
	if e1 != nil {
		return false
	}
	if w.Sunny == nil {
		w.Sunny = &tls.Config{}

	}
	if w.Crypto == nil {
		w.Crypto = &crypto.Config{}
	}
	w.Sunny.Certificates = []tls.Certificate{s}
	w.Crypto.Certificates = []crypto.Certificate{c}
	return true
}
func GetTlsConfigCrypto(host string, Rules uint8) *crypto.Config {
	RequestHost := ParsingHost(host)
	RequestHostLen := len(RequestHost)
	Lock.Lock()
	defer Lock.Unlock()
	for RulesHost, v := range Map {
		RulesHostLen := len(RulesHost)
		if RequestHostLen >= RulesHostLen && RulesHostLen > 3 {
			if RequestHost[RequestHostLen-RulesHostLen:] == RulesHost {
				if v.Rules == Rules || v.Rules == public.CertificateRequestManagerRulesSendAndReceive {
					return v.Crypto
				}
			}
		}
	}
	return nil
}
func GetTlsConfigSunny(host string, Rules uint8) *tls.Config {
	RequestHost := ParsingHost(host)
	RequestHostLen := len(RequestHost)
	Lock.Lock()
	defer Lock.Unlock()
	for RulesHost, v := range Map {
		RulesHostLen := len(RulesHost)
		if RequestHostLen >= RulesHostLen {
			if RequestHost[RequestHostLen-RulesHostLen:] == RulesHost {
				if v.Rules == Rules || v.Rules == public.CertificateRequestManagerRulesSendAndReceive {
					return v.Sunny
				}
			}
		}
	}
	return nil
}

func ParsingHost(host string) string {
	m := host
	if len(m) < 6 {
		return host
	}
	if !strings.HasPrefix(m, "http:") {
		if !strings.HasPrefix(m, "https:") {
			m = "https://" + m
		}
	}
	a, b := url.Parse(m)
	if b != nil {
		return host
	}
	return a.Hostname()
}
