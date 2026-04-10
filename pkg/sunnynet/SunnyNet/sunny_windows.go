//go:build windows

// On Windows, we need to provide the same API surface as the original SunnyNet.
// This stub preserves compilation but should be replaced by the real SunnyNet
// module once the replace directive is removed. For now, the replace directive
// in go.mod is still needed for macOS builds, so Windows builds use these stubs.
//
// Windows users should use the official pre-built .exe from GitHub Releases,
// which includes the full SunnyNet with DLL-based process injection.
package SunnyNet

import (
	"fmt"
	"sync"
)

// Sunny is the main MITM proxy engine stub for Windows.
// The real implementation uses SunnyNet's native DLL injection.
type Sunny struct {
	Error    error
	port     int
	callback func(*HttpConn)
	mu       sync.Mutex
}

func NewSunny() *Sunny           { return &Sunny{} }
func (s *Sunny) SetPort(port int) *Sunny { s.port = port; return s }
func (s *Sunny) GetPort() int    { return s.port }

func (s *Sunny) SetCACert(certPEM, keyPEM []byte) error {
	return fmt.Errorf("SetCACert: Windows builds should use the official pre-built binary with real SunnyNet")
}

func (s *Sunny) Start() *Sunny {
	s.Error = fmt.Errorf("this binary was built from source without SunnyNet; use the official Windows release")
	return s
}

func (s *Sunny) SetGoCallback(callback func(*HttpConn), _ interface{}, _ interface{}, _ interface{}) *Sunny {
	s.callback = callback
	return s
}

func (s *Sunny) GetCallback() func(*HttpConn) { return s.callback }
func (s *Sunny) ProcessAddName(name string)    {}
func (s *Sunny) StartProcess() bool            { return false }
func (s *Sunny) OpenDrive(flag bool) bool       { return true }
