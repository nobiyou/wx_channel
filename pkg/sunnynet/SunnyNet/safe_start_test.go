package SunnyNet

import "testing"

func TestSetLoopbackOnly(t *testing.T) {
	sunny := NewSunny()
	if got := sunny.ListenHost(); got != "0.0.0.0" {
		t.Fatalf("default ListenHost() = %q, want %q", got, "0.0.0.0")
	}

	sunny.SetLoopbackOnly()
	if got := sunny.ListenHost(); got != "127.0.0.1" {
		t.Fatalf("loopback ListenHost() = %q, want %q", got, "127.0.0.1")
	}
}

func TestSafeLifecycleSurface(t *testing.T) {
	var _ func(*Sunny, bool) error = (*Sunny).StopProcess
}
