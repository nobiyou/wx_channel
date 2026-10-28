package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	file, err := root.Open(rel)
	if err != nil {
		return nil, "", pathAccessError(err)
	}
	return file, realPath, nil
}
