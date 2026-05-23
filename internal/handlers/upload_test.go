package handlers

import "testing"

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
