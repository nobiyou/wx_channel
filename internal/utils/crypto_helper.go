package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strconv"

	"wx_channel/pkg/util"
)

// RandomString generates a random string of length n
func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = letters[b[i]%byte(len(letters))]
	}
	return string(b)
}

// DecryptFileInPlace performs in-place XOR decryption on a file
func DecryptFileInPlace(filePath string, key string, decryptorPrefixStr string, prefixLenInput int) error {
	// Open file for read/write
	f, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var decryptorPrefix []byte
	var prefixLen int

	// Priority 1: Use Key to generate decryptor array
	if key != "" {
		seed, err := ParseKey(key)
		if err != nil {
			return fmt.Errorf("failed to parse key: %v", err)
		}
		prefixLen = 131072 // 128KB default for generated arrays
		decryptorPrefix = util.GenerateDecryptorArray(seed, prefixLen)
	} else if decryptorPrefixStr != "" && prefixLenInput > 0 {
		// Priority 2: Use provided decryptor prefix string (Base64)
		var err error
		decryptorPrefix, err = base64.StdEncoding.DecodeString(decryptorPrefixStr)
		if err != nil {
			return fmt.Errorf("failed to decode decryptor prefix: %v", err)
		}
		prefixLen = prefixLenInput
	} else {
		return fmt.Errorf("missing decryption key or prefix")
	}

	// Double check prefix length consistency
	if len(decryptorPrefix) < prefixLen {
		// If generated/decoded is shorter, adjust len
		prefixLen = len(decryptorPrefix)
	}

	// Read file header
	chunk := make([]byte, prefixLen)
	n, err := f.ReadAt(chunk, 0)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read file header: %v", err)
	}

	if n == 0 {
		return nil // Empty file?
	}

	backup := append([]byte(nil), chunk[:n]...)

	// XOR Decrypt
	for i := 0; i < n; i++ {
		chunk[i] ^= decryptorPrefix[i]
	}

	// Write back
	_, err = f.WriteAt(chunk[:n], 0)
	if err != nil {
		return fmt.Errorf("failed to write decrypted data: %v", err)
	}

	if err := validateDecryptedVideoHeader(chunk[:n]); err != nil {
		if _, restoreErr := f.WriteAt(backup, 0); restoreErr != nil {
			return fmt.Errorf("decrypted header validation failed: %v (restore failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("decrypted header validation failed: %v", err)
	}

	return nil
}

// ParseKey parses a key string into uint64 seed
func ParseKey(key string) (uint64, error) {
	if seed, err := strconv.ParseUint(key, 10, 64); err == nil {
		return seed, nil
	}
	return 0, fmt.Errorf("invalid key format: %s", key)
}

func validateDecryptedVideoHeader(header []byte) error {
	if len(header) < 12 {
		return nil
	}
	if looksLikeMediaHeader(header) {
		return nil
	}
	return fmt.Errorf("header does not look like a valid media stream")
}

func looksLikeMediaHeader(header []byte) bool {
	limit := len(header)
	if limit > 32 {
		limit = 32
	}
	for i := 4; i+4 <= limit; i++ {
		boxType := header[i : i+4]
		if bytes.Equal(boxType, []byte("ftyp")) ||
			bytes.Equal(boxType, []byte("styp")) ||
			bytes.Equal(boxType, []byte("moov")) ||
			bytes.Equal(boxType, []byte("mdat")) {
			return true
		}
	}
	return false
}
