package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type pathValidationError struct {
	status int
	msg    string
}

func (e *pathValidationError) Error() string {
	return e.msg
}

// isPathWithinBase compares fully resolved paths, including volume boundaries.
func isPathWithinBase(baseDir, targetPath string) bool {
	rel, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func pathAccessError(err error) error {
	if os.IsNotExist(err) {
		return &pathValidationError{status: http.StatusNotFound, msg: "file not found"}
	}
	if os.IsPermission(err) {
		return &pathValidationError{status: http.StatusForbidden, msg: "access to path is forbidden"}
	}
	return &pathValidationError{status: http.StatusInternalServerError, msg: "failed to resolve or access file"}
}

// resolvePathsInBase must resolve both sides successfully; lexical paths are
// never a safe fallback when a junction or symlink cannot be resolved.
func resolvePathsInBase(baseDir, targetPath string) (string, string, error) {
	absPath, err := filepath.Abs(targetPath)
	if err != nil || strings.IndexByte(targetPath, 0) >= 0 {
		return "", "", &pathValidationError{status: http.StatusBadRequest, msg: "invalid path"}
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", &pathValidationError{status: http.StatusInternalServerError, msg: "failed to resolve downloads directory"}
	}
	realBase, err := resolveExistingPath(absBase)
	if err != nil {
		return "", "", &pathValidationError{status: http.StatusInternalServerError, msg: "failed to resolve downloads directory"}
	}
	realPath, err := resolveExistingPath(absPath)
	if err != nil {
		return "", "", pathAccessError(err)
	}
	if !isPathWithinBase(realBase, realPath) {
		return "", "", &pathValidationError{status: http.StatusForbidden, msg: "access to path outside downloads directory is forbidden"}
	}
	return realBase, realPath, nil
}

func validatePathInBase(baseDir, targetPath string, allowDir bool) (string, error) {
	_, realPath, err := resolvePathsInBase(baseDir, targetPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return "", pathAccessError(err)
	}
	if !allowDir && info.IsDir() {
		return "", &pathValidationError{status: http.StatusBadRequest, msg: "path is a directory"}
	}
	return realPath, nil
}

// ValidateFilePathInBase validates and resolves a file inside the configured
// base directory. MCP and other local read-only integrations use the same
// junction/symlink-aware path policy as the file-serving handlers.
func ValidateFilePathInBase(baseDir, targetPath string) (string, error) {
	return validatePathInBase(baseDir, targetPath, false)
}

// OpenFileInBase returns the same confined handle used by file-serving
// handlers, so local integrations do not re-open a validated path by name.
func OpenFileInBase(baseDir, targetPath string) (*os.File, string, error) {
	return openFileInBase(baseDir, targetPath)
}

func openFileInBase(baseDir, targetPath string) (*os.File, string, error) {
	// Match the lexical normalization used by path validation before opening a
	// root: on Unix, a raw symlink/../ component can name a different directory.
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, "", &pathValidationError{status: http.StatusInternalServerError, msg: "failed to resolve downloads directory"}
	}
	// Pin the root before resolving paths. Root.Open also checks for replacements
	// with escaping links during the open, and callers read this same file handle.
	root, err := os.OpenRoot(absBase)
	if err != nil {
		return nil, "", &pathValidationError{status: http.StatusInternalServerError, msg: "failed to open downloads directory"}
	}
	defer root.Close()
	realBase, realPath, err := resolvePathsInBase(absBase, targetPath)
	if err != nil {
		return nil, "", err
	}
	rel, err := filepath.Rel(realBase, realPath)
	if err != nil {
		return nil, "", &pathValidationError{status: http.StatusForbidden, msg: "access to path outside downloads directory is forbidden"}
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return nil, "", pathAccessError(err)
	}
	if !info.Mode().IsRegular() {
		return nil, "", &pathValidationError{status: http.StatusBadRequest, msg: "path is not a regular file"}
	}
	// Nonblocking open avoids waiting on a FIFO if a regular file is replaced
	// between Stat and Open. Check the opened handle before returning it.
	file, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, "", pathAccessError(err)
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, "", &pathValidationError{status: http.StatusBadRequest, msg: "path is not a regular file"}
	}
	return file, realPath, nil
}
