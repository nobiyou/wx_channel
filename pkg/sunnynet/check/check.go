//go:build !windows

package check

// Non-Windows stub implementations.
func Check() bool {
	return true
}

func CheckFile(data []byte) bool {
	return true
}

func CheckFileWithKey(data []byte, key string) bool {
	return true
}
