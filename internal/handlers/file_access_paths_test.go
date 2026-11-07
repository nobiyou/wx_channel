package handlers

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFileInBase_Symlinks(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "downloads")
	inside := filepath.Join(base, "inside")
	outside := filepath.Join(root, "outside")
	for dir, body := range map[string]string{inside: "inside video", outside: "outside secret"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "downloads-link")
	internalLink := filepath.Join(base, "internal")
	externalLink := filepath.Join(base, "external")
	for link, target := range map[string]string{alias: base, internalLink: inside, externalLink: outside} {
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	for _, tt := range []struct {
		name, base, target string
		wantForbidden      bool
	}{
		{"internal link", base, filepath.Join(internalLink, "video.mp4"), false},
		{"root alias", alias, filepath.Join(alias, "inside", "video.mp4"), false},
		{"root actual path", alias, filepath.Join(inside, "video.mp4"), false},
		{"external link", base, filepath.Join(externalLink, "video.mp4"), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file, _, err := openFileInBase(tt.base, tt.target)
			if file != nil {
				defer file.Close()
			}
			if tt.wantForbidden {
				var pe *pathValidationError
				if file != nil || !errors.As(err, &pe) || pe.status != http.StatusForbidden {
					t.Fatalf("escape error = %v, file = %v; want HTTP 403", err, file)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(file)
			if err != nil || string(content) != "inside video" {
				t.Fatalf("unexpected file content = %q, error = %v", content, err)
			}
		})
	}
}

func TestOpenFileInBase_RootWithSymlinkAndParent(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	base := filepath.Join(work, "downloads")
	outside := filepath.Join(root, "outside")
	for _, dir := range []string{base, filepath.Join(outside, "child"), filepath.Join(outside, "downloads")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for dir, body := range map[string]string{base: "inside video", filepath.Join(outside, "downloads"): "outside secret"} {
		if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(work, "link")
	if err := os.Symlink(filepath.Join(outside, "child"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Preserve .. in the configuration instead of cleaning it with filepath.Join.
	configuredBase := link + string(filepath.Separator) + ".." + string(filepath.Separator) + "downloads"
	file, _, err := openFileInBase(configuredBase, filepath.Join(base, "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || string(content) != "inside video" {
		t.Fatalf("opened a different root from the validated root: content = %q, error = %v", content, err)
	}
}
