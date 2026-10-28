//go:build windows

package handlers

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wx_channel/internal/config"
)

// Junctions need no symlink privilege and are not reported as ModeSymlink by Go.
func createTestJunction(t *testing.T, link, target string) {
	t.Helper()
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"New-Item -ItemType Junction -Path $env:WX_TEST_JUNCTION -Target $env:WX_TEST_TARGET -ErrorAction Stop | Out-Null")
	cmd.Env = append(os.Environ(), "WX_TEST_JUNCTION="+link, "WX_TEST_TARGET="+target)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create junction %q: %v: %s", link, err, output)
	}
	t.Cleanup(func() {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove junction %q: %v", link, err)
		}
	})
}

func TestDownloadPaths_Junctions(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "downloads")
	inside := filepath.Join(base, "作者")
	outside := filepath.Join(root, "downloads-other")
	for _, dir := range []string{inside, outside} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{inside, outside} {
		if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("0123456789"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	internalLink := filepath.Join(base, "internal")
	externalLink := filepath.Join(base, "external")
	rootLink := filepath.Join(root, "downloads-link")
	createTestJunction(t, internalLink, inside)
	createTestJunction(t, externalLink, outside)
	createTestJunction(t, rootLink, base)

	tests := []struct {
		name       string
		base       string
		target     string
		allowDir   bool
		resolved   string
		wantStatus int
	}{
		{"ordinary file", base, filepath.Join(inside, "video.mp4"), false, filepath.Join(inside, "video.mp4"), 0},
		{"internal junction file", base, filepath.Join(internalLink, "video.mp4"), false, filepath.Join(inside, "video.mp4"), 0},
		{"internal junction directory", base, internalLink, true, inside, 0},
		{"external junction file", base, filepath.Join(externalLink, "video.mp4"), false, "", http.StatusForbidden},
		{"external junction directory", base, externalLink, true, "", http.StatusForbidden},
		{"root junction alias", rootLink, filepath.Join(rootLink, "作者", "video.mp4"), false, filepath.Join(inside, "video.mp4"), 0},
		{"root junction actual path", rootLink, filepath.Join(inside, "video.mp4"), false, filepath.Join(inside, "video.mp4"), 0},
		{"root junction directory", rootLink, rootLink, true, base, 0},
		{"sibling prefix", base, filepath.Join(outside, "video.mp4"), false, "", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := validatePathInBase(tt.base, tt.target, tt.allowDir)
			if tt.wantStatus != 0 {
				var pe *pathValidationError
				if !errors.As(err, &pe) || pe.status != tt.wantStatus {
					t.Fatalf("validation error = %v, want HTTP %d", err, tt.wantStatus)
				}
				return
			}
			if err != nil || !strings.EqualFold(resolved, tt.resolved) {
				t.Fatalf("resolved = %q, error = %v; want %q", resolved, err, tt.resolved)
			}
		})
	}

	for _, tt := range []struct {
		name, base, target, byteRange string
		status                        int
		body                          string
	}{
		{"internal full", base, filepath.Join(internalLink, "video.mp4"), "", http.StatusOK, "0123456789"},
		{"root alias range", rootLink, filepath.Join(rootLink, "作者", "video.mp4"), "bytes=2-5", http.StatusPartialContent, "2345"},
		{"external rejected", base, filepath.Join(externalLink, "video.mp4"), "", http.StatusForbidden, ""},
	} {
		t.Run("stream "+tt.name, func(t *testing.T) {
			handler := &ConsoleAPIHandler{cfg: &config.Config{DownloadsDir: tt.base}}
			req := httptest.NewRequest(http.MethodGet, "/api/video/stream?path="+url.QueryEscape(tt.target), nil)
			req.Header.Set("Range", tt.byteRange)
			rr := httptest.NewRecorder()
			handler.HandleVideoStream(rr, req)
			if rr.Code != tt.status {
				t.Fatalf("status = %d, want %d; body = %s", rr.Code, tt.status, rr.Body.String())
			}
			if tt.body != "" && rr.Body.String() != tt.body {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tt.body)
			}
			if tt.status == http.StatusForbidden && strings.Contains(rr.Body.String(), "0123456789") {
				t.Fatal("outside file content leaked")
			}
			if tt.byteRange != "" && rr.Header().Get("Content-Range") != "bytes 2-5/10" {
				t.Fatalf("unexpected Content-Range: %q", rr.Header().Get("Content-Range"))
			}
		})
	}
}

func TestOpenFileInBase_JunctionRetargeted(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "downloads")
	inside := filepath.Join(base, "inside")
	outside := filepath.Join(root, "outside")
	for dir, body := range map[string]string{inside: "original video", outside: "outside secret"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(base, "link")
	createTestJunction(t, link, inside)
	file, _, err := openFileInBase(base, filepath.Join(link, "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	createTestJunction(t, link, outside)
	content, err := io.ReadAll(file)
	if err != nil || string(content) != "original video" {
		t.Fatalf("opened file changed after retargeting: %q, %v", content, err)
	}
	escaped, _, err := openFileInBase(base, filepath.Join(link, "video.mp4"))
	if escaped != nil {
		escaped.Close()
		t.Fatal("retargeted junction escaped the root")
	}
	var pe *pathValidationError
	if !errors.As(err, &pe) || pe.status != http.StatusForbidden {
		t.Fatalf("retargeted junction error = %v, want HTTP 403", err)
	}
}

func TestDownloadPaths_LongWindowsPath(t *testing.T) {
	base := t.TempDir()
	dir := base
	for i := 0; i < 5; i++ {
		dir = filepath.Join(dir, strings.Repeat("a", 50))
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(target, []byte("long path video"), 0644); err != nil {
		t.Fatal(err)
	}
	resolved, err := validatePathInBase(base, target, false)
	if err != nil || !strings.EqualFold(resolved, target) {
		t.Fatalf("long path = %q, error = %v", resolved, err)
	}
	file, _, err := openFileInBase(base, target)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || string(content) != "long path video" {
		t.Fatalf("read long path: %q, %v", content, err)
	}
}

func TestDownloadPaths_BrokenJunction(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "broken")
	createTestJunction(t, link, target)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, base, target string
		status             int
	}{
		{"broken target", base, link, http.StatusNotFound},
		{"broken root", link, filepath.Join(link, "video.mp4"), http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := validatePathInBase(tt.base, tt.target, true)
			var pe *pathValidationError
			if resolved != "" || !errors.As(err, &pe) || pe.status != tt.status {
				t.Fatalf("unresolved junction accepted: path = %q, error = %v; want HTTP %d", resolved, err, tt.status)
			}
			file, _, err := openFileInBase(tt.base, tt.target)
			if file != nil {
				file.Close()
				t.Fatal("unresolved junction opened")
			}
			if !errors.As(err, &pe) || pe.status != tt.status {
				t.Fatalf("open error = %v, want HTTP %d", err, tt.status)
			}
		})
	}
}

func TestNormalizeWindowsFinalPath(t *testing.T) {
	for _, tt := range []struct{ path, want string }{
		{`\\?\C:\downloads\video.mp4`, `C:\downloads\video.mp4`},
		{`\\?\UNC\server\share\video.mp4`, `\\server\share\video.mp4`},
		{`\\?\unc\server\share\video.mp4`, `\\server\share\video.mp4`},
		{`C:\downloads\video.mp4`, `C:\downloads\video.mp4`},
		{`\\?\GLOBALROOT\Device\HarddiskVolume1\video.mp4`, ""},
		{`\\.\C:\downloads\video.mp4`, ""},
		{`relative\video.mp4`, ""},
	} {
		t.Run(tt.path, func(t *testing.T) {
			got, err := normalizeWindowsFinalPath(tt.path)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("unsupported path accepted: %q", got)
				}
			} else if err != nil || got != tt.want {
				t.Fatalf("normalized path = %q, error = %v; want %q", got, err, tt.want)
			}
		})
	}
}
