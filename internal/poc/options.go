package poc

import (
	"errors"
	"net"
	"time"
)

type Options struct {
	Keyword           string
	Limits            Limits
	HumanWait         HumanWaitPolicy
	RequestInterval   time.Duration
	ProxyAddress      string
	BridgeAddress     string
	DataRoot          string
	SecretsRoot       string
	RuntimeRoot       string
	BuildRoot         string
	AckIsolatedVM     bool
	AllowEncryptedRaw bool
}

func DefaultOptions() Options {
	return Options{
		Keyword: "青云装饰",
		Limits: Limits{
			Works:                   10,
			TopLevelCommentsPerWork: 100,
			RepliesPerWork:          200,
		},
		HumanWait: HumanWaitPolicy{
			Timeout:       300 * time.Second,
			Extension:     300 * time.Second,
			MaxExtensions: 1,
		},
		RequestInterval: time.Second,
		ProxyAddress:    "127.0.0.1:2025",
		BridgeAddress:   "127.0.0.1:2026",
		DataRoot:        ".poc-data",
		SecretsRoot:     ".poc-secrets",
		RuntimeRoot:     ".poc-runtime",
		BuildRoot:       ".poc-build",
	}
}

func (o Options) ValidateForRun() error {
	if !o.AckIsolatedVM {
		return errors.New("isolated VM acknowledgement is required")
	}
	if o.Keyword != "青云装饰" {
		return errors.New("keyword must be 青云装饰")
	}
	if o.Limits != (Limits{Works: 10, TopLevelCommentsPerWork: 100, RepliesPerWork: 200}) {
		return errors.New("limits differ from approved spec")
	}
	if o.HumanWait.Timeout != 300*time.Second || o.HumanWait.Extension != 300*time.Second || o.HumanWait.MaxExtensions != 1 {
		return errors.New("human wait policy differs from approved spec")
	}
	if o.RequestInterval != time.Second {
		return errors.New("request interval differs from approved spec")
	}
	for _, address := range []string{o.ProxyAddress, o.BridgeAddress} {
		host, _, err := net.SplitHostPort(address)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			return errors.New("all listeners must be loopback")
		}
	}
	if o.ProxyAddress == o.BridgeAddress {
		return errors.New("proxy and bridge listeners must be distinct")
	}
	return nil
}
