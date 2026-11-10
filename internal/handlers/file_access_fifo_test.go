//go:build linux || darwin

package handlers

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestOpenFileInBaseRejectsFIFOWithoutBlocking(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "fifo.mp4")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		file, _, err := OpenFileInBase(base, path)
		if file != nil {
			_ = file.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(2 * time.Second):
		// Release a blocked reader before failing so test cleanup can finish.
		writer, _ := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if writer != nil {
			_ = writer.Close()
		}
		t.Fatal("opening a FIFO blocked")
	}
}
