package handlers

import "testing"

func TestExtractDecryptKeyAcceptsFeedAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{name: "legacy key", data: map[string]interface{}{"key": "123"}, want: "123"},
		{name: "camel decode key", data: map[string]interface{}{"decodeKey": "456"}, want: "456"},
		{name: "nested decrypt key", data: map[string]interface{}{"media": map[string]interface{}{"decryptKey": "789"}}, want: "789"},
		{name: "snake case fallback", data: map[string]interface{}{"key": "", "decode_key": "012"}, want: "012"},
		{name: "numeric key", data: map[string]interface{}{"decodeKey": float64(2136343393)}, want: "2136343393"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractDecryptKey(tt.data); got != tt.want {
				t.Fatalf("extractDecryptKey(%v) = %q, want %q", tt.data, got, tt.want)
			}
		})
	}
}

func TestMergeDecryptKeyDoesNotMixSignedURLPairs(t *testing.T) {
	t.Parallel()

	if got := mergeDecryptKey("url-a", "old-key", "url-b", ""); got != "" {
		t.Fatalf("stale key was retained for refreshed URL: %q", got)
	}
	if got := mergeDecryptKey("url-a", "old-key", "url-a", ""); got != "old-key" {
		t.Fatalf("same URL key was not retained: %q", got)
	}
	if got := mergeDecryptKey("url-a", "old-key", "url-b", "new-key"); got != "new-key" {
		t.Fatalf("new key was not preferred: %q", got)
	}
}
