package util

import "strings"

func Includes(str, substr string) bool {
	return strings.Contains(str, substr)
}

// XorDecrypt 使用XOR解密数据
func XorDecrypt(data []byte, key []byte) []byte {
	result := make([]byte, len(data))
	keyLen := len(key)
	
	for i := 0; i < len(data); i++ {
		if i < keyLen {
			result[i] = data[i] ^ key[i]
		} else {
			result[i] = data[i]
		}
	}
	
	return result
}
