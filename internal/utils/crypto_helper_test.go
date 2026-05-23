package utils

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestDecryptFileInPlaceRestoresValidHeader(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "video.mp4")
	original := []byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'i', 's', 'o', 'm',
		0x00, 0x00, 0x00, 0x01,
		'i', 's', 'o', 'm', 'a', 'v', 'c', '1',
	}
	decryptor := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}
	encrypted := make([]byte, len(original))
	copy(encrypted, original)
	for i := range encrypted {
		encrypted[i] ^= decryptor[i]
	}

	if err := os.WriteFile(filePath, encrypted, 0644); err != nil {
		t.Fatalf("write encrypted file: %v", err)
	}

	err := DecryptFileInPlace(filePath, "", base64.StdEncoding.EncodeToString(decryptor), len(decryptor))
	if err != nil {
		t.Fatalf("DecryptFileInPlace returned error: %v", err)
	}

	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read decrypted file: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("decrypted file mismatch: got %v want %v", got, original)
	}
}

func TestDecryptFileInPlaceRestoresBackupOnInvalidHeader(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "video.mp4")
	encrypted := []byte{
		0x10, 0x20, 0x30, 0x40,
		0x50, 0x60, 0x70, 0x80,
		0x90, 0xA0, 0xB0, 0xC0,
	}
	wrongDecryptor := []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}

	if err := os.WriteFile(filePath, encrypted, 0644); err != nil {
		t.Fatalf("write encrypted file: %v", err)
	}

	err := DecryptFileInPlace(filePath, "", base64.StdEncoding.EncodeToString(wrongDecryptor), len(wrongDecryptor))
	if err == nil {
		t.Fatal("DecryptFileInPlace() expected error, got nil")
	}

	got, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatalf("read restored file: %v", readErr)
	}
	if string(got) != string(encrypted) {
		t.Fatalf("backup not restored: got %v want %v", got, encrypted)
	}
}

func TestLooksLikeMediaHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header []byte
		want   bool
	}{
		{
			name:   "ftyp",
			header: []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'},
			want:   true,
		},
		{
			name:   "mdat",
			header: []byte{0x00, 0x00, 0x00, 0x18, 'm', 'd', 'a', 't', 0x00, 0x00, 0x00, 0x00},
			want:   true,
		},
		{
			name:   "invalid",
			header: []byte{0x00, 0x00, 0x00, 0x18, 'b', 'a', 'd', '!', 0x00, 0x00, 0x00, 0x00},
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := looksLikeMediaHeader(tt.header)
			if got != tt.want {
				t.Fatalf("looksLikeMediaHeader(%v) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}
