//go:build windows

package check

// A simple implementation for Windows
func Check() bool {
	return true
}

func CheckFile(data []byte) bool {
	return true
}

func CheckFileWithKey(data []byte, key string) bool {
	return true
}
