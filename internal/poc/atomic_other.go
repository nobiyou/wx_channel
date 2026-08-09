//go:build !windows

package poc

import "os"

func atomicReplace(source, target string) error {
	return os.Rename(source, target)
}

func rejectPlatformReparse(string) error {
	return nil
}

func secureDirectory(path string) error {
	return os.Chmod(path, 0o700)
}
