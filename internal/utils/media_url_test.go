package utils

import "testing"

func TestJoinURLToken(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		token string
		want  string
	}{
		{
			name:  "ampersand token",
			base:  "https://video.example.test/file?encfilekey=abc",
			token: "&token=def&sign=sig",
			want:  "https://video.example.test/file?encfilekey=abc&sign=sig&token=def",
		},
		{
			name:  "question token",
			base:  "https://video.example.test/file",
			token: "?token=def",
			want:  "https://video.example.test/file?token=def",
		},
		{
			name:  "encoded query token",
			base:  "https://video.example.test/file?encfilekey=abc",
			token: "%26token%3Ddef%26sign%3Ds%252B1",
			want:  "https://video.example.test/file?encfilekey=abc&sign=s%2B1&token=def",
		},
		{
			name:  "complete token URL",
			base:  "https://video.example.test/base",
			token: "https://cdn.example.test/file?token=def",
			want:  "https://cdn.example.test/file?token=def",
		},
		{
			name:  "literal plus in signed query",
			base:  "https://video.example.test/file?encfilekey=a+b",
			token: "&token=tok+plus",
			want:  "https://video.example.test/file?encfilekey=a%2Bb&token=tok%2Bplus",
		},
		{
			name:  "empty base",
			base:  "",
			token: "https://cdn.example.test/file.mp4",
			want:  "https://cdn.example.test/file.mp4",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := JoinURLToken(test.base, test.token); got != test.want {
				t.Fatalf("JoinURLToken(%q, %q) = %q, want %q", test.base, test.token, got, test.want)
			}
		})
	}
}
