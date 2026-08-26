package handlers

import (
	"net/url"
	"testing"
)

func TestIsOriginalVideoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "legacy original marker",
			raw:  "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def&X-snsvideoflag=original",
			want: true,
		},
		{
			name: "specific spec marker",
			raw:  "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def&X-snsvideoflag=WT111",
			want: false,
		},
		{
			name: "no video flag",
			raw:  "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def",
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isOriginalVideoURL(tt.raw)
			if got != tt.want {
				t.Fatalf("isOriginalVideoURL(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeOriginalVideoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "removes legacy original query param",
			raw:  "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def&X-snsvideoflag=original",
			want: "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def",
		},
		{
			name: "keeps specific spec query param",
			raw:  "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def&X-snsvideoflag=WT111",
			want: "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def&X-snsvideoflag=WT111",
		},
		{
			name: "keeps url without spec flag",
			raw:  "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def",
			want: "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeOriginalVideoURL(tt.raw)
			if got != tt.want {
				t.Fatalf("normalizeOriginalVideoURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeDownloadURLPreservesOriginalSignedParameters(t *testing.T) {
	t.Parallel()

	raw := "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&hy=SH&idx=1&m=compressed&uzid=7a1ac&token=def&basedata=base&sign=sig&web=1&extg=10f0000&svrbypass=bypass&svrnonce=123"
	got, mode := NormalizeDownloadURL(raw, "")
	if mode != downloadVideoModeOriginal {
		t.Fatalf("NormalizeDownloadURL mode = %q, want %q", mode, downloadVideoModeOriginal)
	}
	if got != raw {
		t.Fatalf("NormalizeDownloadURL changed signed URL without legacy marker: got %q, want %q", got, raw)
	}

	marked := raw + "&X-snsvideoflag=original"
	got, mode = NormalizeDownloadURL(marked, "")
	if mode != downloadVideoModeOriginal {
		t.Fatalf("NormalizeDownloadURL marked mode = %q, want %q", mode, downloadVideoModeOriginal)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse normalized URL: %v", err)
	}
	query := parsed.Query()
	for _, key := range []string{"encfilekey", "hy", "idx", "m", "uzid", "token", "basedata", "sign", "web", "extg", "svrbypass", "svrnonce"} {
		if query.Get(key) == "" {
			t.Fatalf("normalized URL lost signed query parameter %q: %q", key, got)
		}
	}
	if marker := query.Get("X-snsvideoflag"); marker != "" {
		t.Fatalf("normalized URL retained legacy marker: %q", marker)
	}
}

func TestDownloadVideoModeFromRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  DownloadVideoRequest
		want downloadVideoMode
	}{
		{
			name: "legacy original marker uses original mode",
			req: DownloadVideoRequest{
				VideoURL: "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def&X-snsvideoflag=original",
			},
			want: downloadVideoModeOriginal,
		},
		{
			name: "specific file format uses specific mode",
			req: DownloadVideoRequest{
				VideoURL:   "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def&X-snsvideoflag=WT111",
				FileFormat: "WT111",
			},
			want: downloadVideoModeSpecific,
		},
		{
			name: "file format alone keeps specific mode",
			req: DownloadVideoRequest{
				VideoURL:   "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def",
				FileFormat: "WT111",
			},
			want: downloadVideoModeSpecific,
		},
		{
			name: "missing spec uses original mode",
			req: DownloadVideoRequest{
				VideoURL: "https://finder.video.qq.com/251/20302/stodownload?encfilekey=abc&token=def",
			},
			want: downloadVideoModeOriginal,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := downloadModeFromRequest(tt.req)
			if got != tt.want {
				t.Fatalf("downloadModeFromRequest(%+v) = %q, want %q", tt.req, got, tt.want)
			}
		})
	}
}

func TestDownloadConnectionCountFromMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		base        int
		mode        downloadVideoMode
		wantConnect int
	}{
		{
			name:        "original mode forces single connection",
			base:        8,
			mode:        downloadVideoModeOriginal,
			wantConnect: 1,
		},
		{
			name:        "specific mode preserves configured connections",
			base:        8,
			mode:        downloadVideoModeSpecific,
			wantConnect: 8,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := downloadConnectionCountFromMode(tt.base, tt.mode)
			if got != tt.wantConnect {
				t.Fatalf("downloadConnectionCountFromMode(%d, %q) = %d, want %d", tt.base, tt.mode, got, tt.wantConnect)
			}
		})
	}
}

func TestValidateOriginalDownloadSize(t *testing.T) {
	t.Parallel()

	const megabyte = int64(1024 * 1024)
	tests := []struct {
		name         string
		mode         downloadVideoMode
		expectedSize int64
		actualSize   int64
		wantErr      bool
	}{
		{
			name:         "rejects a lower-quality rendition",
			mode:         downloadVideoModeOriginal,
			expectedSize: 362 * megabyte,
			actualSize:   39 * megabyte,
			wantErr:      true,
		},
		{
			name:         "rejects the reported 4.26 MB to 0.25 MB shrink",
			mode:         downloadVideoModeOriginal,
			expectedSize: 4*megabyte + 26*megabyte/100,
			actualSize:   25 * megabyte / 100,
			wantErr:      true,
		},
		{
			name:         "allows an approximate source-size hint",
			mode:         downloadVideoModeOriginal,
			expectedSize: 362 * megabyte,
			actualSize:   300 * megabyte,
		},
		{
			name:         "does not apply to a specific rendition",
			mode:         downloadVideoModeSpecific,
			expectedSize: 362 * megabyte,
			actualSize:   39 * megabyte,
		},
		{
			name:         "does not apply without a source-size hint",
			mode:         downloadVideoModeOriginal,
			expectedSize: 0,
			actualSize:   39 * megabyte,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateOriginalDownloadSize(tt.mode, tt.expectedSize, tt.actualSize)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateOriginalDownloadSize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
