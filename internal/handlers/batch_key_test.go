package handlers

import "testing"

func TestBatchTaskKeyAliasesAndDecryptionDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		task        BatchTask
		wantKey     string
		wantDecrypt bool
	}{
		{
			name:        "key alias",
			task:        BatchTask{Key: "123"},
			wantKey:     "123",
			wantDecrypt: true,
		},
		{
			name:        "database decrypt key alias",
			task:        BatchTask{DecryptKey: "456"},
			wantKey:     "456",
			wantDecrypt: true,
		},
		{
			name:        "key takes precedence",
			task:        BatchTask{Key: "123", DecryptKey: "456"},
			wantKey:     "123",
			wantDecrypt: true,
		},
		{
			name:        "blank key falls back",
			task:        BatchTask{Key: "  ", DecryptKey: "789"},
			wantKey:     "789",
			wantDecrypt: true,
		},
		{
			name:        "legacy decryptor prefix",
			task:        BatchTask{DecryptorPrefix: "prefix", PrefixLen: 4},
			wantKey:     "",
			wantDecrypt: true,
		},
		{
			name:        "no decryption input",
			task:        BatchTask{},
			wantKey:     "",
			wantDecrypt: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.task.GetKey(); got != tt.wantKey {
				t.Fatalf("GetKey() = %q, want %q", got, tt.wantKey)
			}
			if got := tt.task.NeedsDecryption(); got != tt.wantDecrypt {
				t.Fatalf("NeedsDecryption() = %v, want %v", got, tt.wantDecrypt)
			}
		})
	}
}
