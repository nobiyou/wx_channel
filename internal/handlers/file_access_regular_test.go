package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFileInBaseRejectsNonRegular(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "directory.mp4")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file, _, err := openFileInBase(base, dir)
	if file != nil {
		_ = file.Close()
	}
	if err == nil || file != nil {
		t.Fatal("nonregular media handle was accepted")
	}
}
