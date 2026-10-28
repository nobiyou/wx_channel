package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"wx_channel/internal/config"
)

func TestValidatePathInBase_ResolutionErrors(t *testing.T) {
	base := t.TempDir()
	for _, tt := range []struct {
		name, base, target string
		status             int
	}{
		{"missing root", filepath.Join(base, "missing"), filepath.Join(base, "missing", "video.mp4"), http.StatusInternalServerError},
		{"missing file", base, filepath.Join(base, "missing.mp4"), http.StatusNotFound},
		{"invalid file path", base, filepath.Join(base, "invalid\x00.mp4"), http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePathInBase(tt.base, tt.target, false)
			var pe *pathValidationError
			if !errors.As(err, &pe) || pe.status != tt.status {
				t.Fatalf("validation error = %v, want HTTP %d", err, tt.status)
			}
		})
	}
}

func TestValidatePathInBase_RootSymlink(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "downloads")
	if err := os.Mkdir(base, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(base, "video.mp4")
	if err := os.WriteFile(file, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "downloads-link")
	if err := os.Symlink(base, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, target := range []string{file, filepath.Join(link, "video.mp4")} {
		resolved, err := validatePathInBase(link, target, false)
		if err != nil {
			t.Fatalf("validate %q: %v", target, err)
		}
		info, err := os.Stat(resolved)
		if err != nil || info.Size() != 5 {
			t.Fatalf("resolved path = %q, stat error = %v", resolved, err)
		}
	}
}

func TestHandleVideoStream_LocalFile(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "video.mp4")
	if err := os.WriteFile(file, []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := &ConsoleAPIHandler{cfg: &config.Config{DownloadsDir: base}}
	for _, tt := range []struct {
		name, target, byteRange string
		status                  int
		body                    string
	}{
		{"complete", file, "", http.StatusOK, "0123456789"},
		{"range", file, "bytes=2-5", http.StatusPartialContent, "2345"},
		{"open ended range", file, "bytes=7-", http.StatusPartialContent, "789"},
		{"invalid range", file, "bytes=10-20", http.StatusRequestedRangeNotSatisfiable, ""},
		{"missing file", filepath.Join(base, "missing.mp4"), "", http.StatusNotFound, ""},
		{"directory", base, "", http.StatusBadRequest, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/video/stream?path="+url.QueryEscape(tt.target), nil)
			req.Header.Set("Range", tt.byteRange)
			rr := httptest.NewRecorder()
			handler.HandleVideoStream(rr, req)
			if rr.Code != tt.status {
				t.Fatalf("status = %d, want %d; body = %s", rr.Code, tt.status, rr.Body.String())
			}
			if tt.body != "" {
				if rr.Body.String() != tt.body {
					t.Fatalf("body = %q, want %q", rr.Body.String(), tt.body)
				}
				if rr.Header().Get("Content-Type") != "video/mp4" || rr.Header().Get("Accept-Ranges") != "bytes" {
					t.Fatalf("unexpected video headers: %v", rr.Header())
				}
			}
		})
	}
}
