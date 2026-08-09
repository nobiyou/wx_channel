//go:build windows

package poc

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestCurrentProcessElevationCheckIsReadOnlyAndAccurate(t *testing.T) {
	got, err := currentProcessIsElevated()
	if err != nil {
		t.Fatal(err)
	}
	want := windows.GetCurrentProcessToken().IsElevated()
	if got != want {
		t.Fatalf("elevated=%v want=%v", got, want)
	}
}
