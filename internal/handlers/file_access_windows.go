//go:build windows

package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// EvalSymlinks does not reliably resolve Windows junctions under every Go
// compatibility setting. Query a handle opened with reparse-point traversal.
func resolveExistingPath(path string) (string, error) {
	extended := path
	if !strings.HasPrefix(path, `\\?\`) {
		if strings.HasPrefix(path, `\\`) {
			extended = `\\?\UNC\` + path[2:]
		} else {
			extended = `\\?\` + path
		}
	}
	name, err := windows.UTF16PtrFromString(extended)
	if err != nil {
		return "", &os.PathError{Op: "resolve", Path: path, Err: err}
	}
	handle, err := windows.CreateFile(name, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", &os.PathError{Op: "resolve", Path: path, Err: err}
	}
	defer windows.CloseHandle(handle)
	buf := make([]uint16, 256)
	for {
		n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", &os.PathError{Op: "resolve", Path: path, Err: err}
		}
		if n >= uint32(len(buf)) {
			buf = make([]uint16, n+1)
			continue
		}
		resolved, err := normalizeWindowsFinalPath(windows.UTF16ToString(buf[:n]))
		if err != nil {
			return "", &os.PathError{Op: "resolve", Path: path, Err: err}
		}
		return resolved, nil
	}
}

func normalizeWindowsFinalPath(path string) (string, error) {
	if len(path) >= 8 && strings.EqualFold(path[:8], `\\?\UNC\`) {
		path = `\\` + path[8:]
	} else if len(path) >= 7 && strings.HasPrefix(path, `\\?\`) && path[5] == ':' && path[6] == '\\' {
		path = path[4:]
	}
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return "", fmt.Errorf("unsupported final path")
	}
	return filepath.Clean(path), nil
}
