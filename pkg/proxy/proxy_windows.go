//go:build windows

// Stub for Windows — macOS proxy functions are no-ops here.
package proxy

type ProxySettings struct {
	Device   string
	Hostname string
	Port     string
}

func EnablePACProxyInMacOS(_ ProxySettings, _ string) error { return nil }
func DisableProxyInMacOS(_ ProxySettings) error             { return nil }
