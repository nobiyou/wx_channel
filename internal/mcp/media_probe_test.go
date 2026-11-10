package mcpserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mediaProbeBox(kind string, payload ...[]byte) []byte {
	b := make([]byte, 8)
	copy(b[4:], kind)
	for _, p := range payload {
		b = append(b, p...)
	}
	binary.BigEndian.PutUint32(b, uint32(len(b)))
	return b
}

func mediaProbeMovieHeader(version byte, scale uint32, duration uint64) []byte {
	b := make([]byte, 100)
	b[0] = version
	if version == 1 {
		b = make([]byte, 112)
		b[0] = version
		binary.BigEndian.PutUint32(b[20:], scale)
		binary.BigEndian.PutUint64(b[24:], duration)
	} else {
		binary.BigEndian.PutUint32(b[12:], scale)
		binary.BigEndian.PutUint32(b[16:], uint32(duration))
	}
	return mediaProbeBox("mvhd", b)
}

func mediaProbeTrackHeader(version byte, width, height uint16, rotation int) []byte {
	n, matrix, dimensions := 84, 40, 76
	if version == 1 {
		n, matrix, dimensions = 96, 52, 88
	}
	b := make([]byte, n)
	b[0], b[3] = version, 3
	transform := [9]int32{1 << 16, 0, 0, 0, 1 << 16, 0, 0, 0, 1 << 30}
	switch rotation {
	case 90:
		transform[0], transform[1], transform[3], transform[4] = 0, 1<<16, -1<<16, 0
	case 180:
		transform[0], transform[4] = -1<<16, -1<<16
	case 270:
		transform[0], transform[1], transform[3], transform[4] = 0, -1<<16, 1<<16, 0
	}
	for i, v := range transform {
		binary.BigEndian.PutUint32(b[matrix+i*4:], uint32(v))
	}
	binary.BigEndian.PutUint32(b[dimensions:], uint32(width)<<16)
	binary.BigEndian.PutUint32(b[dimensions+4:], uint32(height)<<16)
	return mediaProbeBox("tkhd", b)
}

func mediaProbeHandler(handler string) []byte {
	b := make([]byte, 24)
	copy(b[8:], handler)
	return mediaProbeBox("hdlr", b)
}

func mediaProbeTrack(handler string, version byte, width, height uint16, rotation int) []byte {
	// Put mdia before tkhd: handler and dimensions must be associated by track,
	// independently of child order.
	return mediaProbeBox("trak", mediaProbeBox("mdia", mediaProbeHandler(handler)), mediaProbeTrackHeader(version, width, height, rotation))
}

func mediaProbeFixture(t testing.TB, data []byte) *os.File {
	t.Helper()
	// Every format uses an mp4 filename so extension-based detection cannot pass.
	path := filepath.Join(t.TempDir(), "private-token-media.mp4")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func mediaProbeRun(t testing.TB, data []byte) (map[string]any, []string) {
	t.Helper()
	fields, warnings, err := probeLocalMedia(context.Background(), mediaProbeFixture(t, data), int64(len(data)))
	if err != nil {
		t.Fatalf("probeLocalMedia: %v", err)
	}
	mediaProbeCheckSchema(t, fields)
	return fields, warnings
}

func mediaProbeCheckSchema(t testing.TB, fields map[string]any) {
	t.Helper()
	sources, ok := fields["metadata_sources"].(map[string]string)
	if !ok {
		t.Fatalf("metadata_sources = %#v, want map[string]string", fields["metadata_sources"])
	}
	for _, key := range []string{"duration_seconds", "width", "height", "resolution"} {
		value, exists := fields[key]
		if !exists {
			t.Errorf("missing nullable field %q", key)
		}
		if value != nil && sources[key] == "" {
			t.Errorf("%q has a value without a metadata source", key)
		}
		if value == nil && sources[key] != "" {
			t.Errorf("%q has a source without a value", key)
		}
	}
}

func mediaProbeAssertNull(t testing.TB, fields map[string]any) {
	t.Helper()
	mediaProbeCheckSchema(t, fields)
	for _, key := range []string{"duration_seconds", "width", "height", "resolution"} {
		if fields[key] != nil {
			t.Errorf("%s = %#v, want null", key, fields[key])
		}
	}
}

func TestProbeLocalMediaMovieHeadersAndRotation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		version  byte
		duration uint64
		rotation int
		width    int
		height   int
	}{
		{"v0", 0, 12500, 0, 1920, 1080},
		{"v1-long-duration", 1, 1<<33 + 250, 0, 1920, 1080},
		{"v0-90", 0, 12500, 90, 1080, 1920},
		{"v1-270", 1, 12500, 270, 1080, 1920},
		{"v0-180", 0, 12500, 180, 1920, 1080},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := mediaProbeBox("ftyp", []byte("isom\x00\x00\x00\x00isom"))
			data = append(data, mediaProbeBox("moov", mediaProbeMovieHeader(tc.version, 1000, tc.duration), mediaProbeTrack("vide", tc.version, 1920, 1080, tc.rotation))...)
			got, warnings := mediaProbeRun(t, data)
			if got["duration_seconds"] != float64(tc.duration)/1000 || got["width"] != tc.width || got["height"] != tc.height {
				t.Errorf("metadata = %v", got)
			}
			wantResolution := "1920x1080"
			if tc.width == 1080 {
				wantResolution = "1080x1920"
			}
			if got["resolution"] != wantResolution || len(warnings) != 0 {
				t.Errorf("resolution = %v, warnings = %v", got["resolution"], warnings)
			}
			wantSources := map[string]string{"duration_seconds": "mp4_mvhd", "width": "mp4_tkhd", "height": "mp4_tkhd", "resolution": "mp4_tkhd"}
			if !reflect.DeepEqual(got["metadata_sources"], wantSources) {
				t.Errorf("metadata_sources = %v", got["metadata_sources"])
			}
		})
	}
}

func TestProbeLocalMediaTailMoovAfterLargeMdat(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "tail-*.mov")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	const tailOffset int64 = 64<<20 + 4096
	header := make([]byte, 16)
	binary.BigEndian.PutUint32(header, 1)
	copy(header[4:], "mdat")
	binary.BigEndian.PutUint64(header[8:], uint64(tailOffset))
	if _, err := f.WriteAt(header, 0); err != nil {
		t.Fatal(err)
	}
	tail := mediaProbeBox("moov", mediaProbeMovieHeader(1, 1000, 3750), mediaProbeTrack("vide", 1, 640, 480, 0))
	if _, err := f.WriteAt(tail, tailOffset); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(3, 0); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, warnings, err := probeLocalMedia(ctx, f, tailOffset+int64(len(tail)))
	if err != nil || got["duration_seconds"] != 3.75 || got["resolution"] != "640x480" || len(warnings) != 0 {
		t.Fatalf("tail probe = %v, %v, %v", got, warnings, err)
	}
	if pos, err := f.Seek(0, 1); err != nil || pos != 3 {
		t.Fatalf("caller file position changed: %d, %v", pos, err)
	}
}

func TestProbeLocalMediaAssociatesVideoTracks(t *testing.T) {
	data := mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 9000),
		mediaProbeTrack("soun", 0, 999, 999, 0),
		mediaProbeTrack("vide", 0, 0, 0, 0),
		mediaProbeTrack("vide", 1, 1280, 720, 90),
		mediaProbeTrack("vide", 0, 3840, 2160, 0),
		mediaProbeTrack("soun", 1, 888, 888, 0))
	got, warnings := mediaProbeRun(t, data)
	if got["resolution"] != "720x1280" || got["duration_seconds"] != float64(9) || len(warnings) != 0 {
		t.Fatalf("track selection = %v, warnings = %v", got, warnings)
	}
	audioOnly, warnings := mediaProbeRun(t, mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 9000), mediaProbeTrack("soun", 0, 999, 999, 0)))
	if audioOnly["width"] != nil || audioOnly["height"] != nil || audioOnly["resolution"] != nil || !reflect.DeepEqual(warnings, []string{"local_media_dimensions_unavailable"}) {
		t.Fatalf("audio track supplied video dimensions: %v, %v", audioOnly, warnings)
	}
	// A tkhd without a handler is never assumed to be video.
	got, _ = mediaProbeRun(t, mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 9000), mediaProbeBox("trak", mediaProbeTrackHeader(0, 1920, 1080, 0))))
	if got["resolution"] != nil {
		t.Fatal("untyped track supplied video dimensions")
	}
}

func TestProbeLocalMediaUnknownDuration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		version  byte
		scale    uint32
		duration uint64
	}{
		{"v0-unknown", 0, 1000, math.MaxUint32},
		{"v1-unknown", 1, 1000, math.MaxUint64},
		{"zero-timescale", 0, 0, 100},
		{"zero-duration", 0, 1000, 0},
		{"unknown-version", 2, 1000, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, warnings := mediaProbeRun(t, mediaProbeBox("moov", mediaProbeMovieHeader(tc.version, tc.scale, tc.duration), mediaProbeTrack("vide", 0, 16, 9, 0)))
			if got["duration_seconds"] != nil || got["resolution"] != "16x9" || !reflect.DeepEqual(warnings, []string{"local_media_duration_unavailable"}) {
				t.Fatalf("unknown duration = %v, %v", got, warnings)
			}
		})
	}
}

func TestProbeLocalMediaImageHeaders(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			var data bytes.Buffer
			img := image.NewRGBA(image.Rect(0, 0, 37, 19))
			var err error
			switch format {
			case "png":
				err = png.Encode(&data, img)
			case "jpeg":
				err = jpeg.Encode(&data, img, nil)
			case "gif":
				err = gif.Encode(&data, img, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, warnings := mediaProbeRun(t, data.Bytes())
			if got["duration_seconds"] != nil || got["width"] != 37 || got["height"] != 19 || got["resolution"] != "37x19" || len(warnings) != 0 {
				t.Fatalf("image header = %v, %v", got, warnings)
			}
			if got["metadata_sources"].(map[string]string)["width"] != "image_header" {
				t.Error("missing image header source")
			}
		})
	}
}

func TestProbeLocalMediaUnsupportedMagic(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("video"), []byte("ID3audio"), []byte("RIFFxxxxAVI "), {0x1a, 0x45, 0xdf, 0xa3}} {
		got, warnings := mediaProbeRun(t, data)
		mediaProbeAssertNull(t, got)
		if !reflect.DeepEqual(warnings, []string{"local_media_format_unsupported"}) {
			t.Errorf("unsupported warning = %v", warnings)
		}
	}
}

func TestProbeLocalMediaMalformed(t *testing.T) {
	extended := func(size uint64) []byte {
		b := make([]byte, 16)
		binary.BigEndian.PutUint32(b, 1)
		copy(b[4:], "moov")
		binary.BigEndian.PutUint64(b[8:], size)
		return b
	}
	valid := mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 5000), mediaProbeTrack("vide", 0, 640, 360, 0))
	for name, data := range map[string][]byte{
		"undersize":             {0, 0, 0, 4, 'm', 'o', 'o', 'v'},
		"overrun":               {0, 0, 1, 0, 'm', 'o', 'o', 'v'},
		"extended-truncated":    {0, 0, 0, 1, 'm', 'o', 'o', 'v'},
		"extended-undersize":    extended(15),
		"extended-overflow":     extended(math.MaxUint64),
		"child-escapes-parent":  mediaProbeBox("moov", []byte{0, 0, 1, 0, 'm', 'v', 'h', 'd'}),
		"short-mvhd":            mediaProbeBox("moov", mediaProbeBox("mvhd", make([]byte, 20))),
		"short-tkhd":            mediaProbeBox("moov", mediaProbeBox("trak", mediaProbeTrackHeader(0, 1, 1, 0)[:20])),
		"short-hdlr":            mediaProbeBox("moov", mediaProbeBox("trak", mediaProbeBox("mdia", mediaProbeBox("hdlr", make([]byte, 12))))),
		"trailing-fragment":     append(append([]byte{}, valid...), 1, 2, 3),
		"truncated-png":         []byte("\x89PNG\r\n\x1a\n"),
		"truncated-jpeg":        {0xff, 0xd8, 0xff, 0xe0},
		"truncated-gif":         []byte("GIF89a"),
		"conflicting-mvhd":      mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 1000), mediaProbeMovieHeader(0, 1000, 2000)),
		"conflicting-handlers":  mediaProbeBox("moov", mediaProbeBox("trak", mediaProbeBox("mdia", mediaProbeHandler("vide"), mediaProbeHandler("soun")), mediaProbeTrackHeader(0, 1, 1, 0))),
		"conflicting-track-hdr": mediaProbeBox("moov", mediaProbeBox("trak", mediaProbeTrackHeader(0, 1, 1, 0), mediaProbeTrackHeader(0, 2, 2, 0))),
	} {
		t.Run(name, func(t *testing.T) {
			got, warnings := mediaProbeRun(t, data)
			mediaProbeAssertNull(t, got)
			if !reflect.DeepEqual(warnings, []string{"local_media_metadata_invalid"}) {
				t.Fatalf("malformed warning = %v", warnings)
			}
		})
	}
}

func TestProbeLocalMediaBounds(t *testing.T) {
	t.Run("box-count", func(t *testing.T) {
		data := bytes.Repeat(mediaProbeBox("free"), 5000)
		data = append(data, mediaProbeBox("moov", mediaProbeMovieHeader(0, 1, 1))...)
		got, warnings := mediaProbeRun(t, data)
		mediaProbeAssertNull(t, got)
		if !reflect.DeepEqual(warnings, []string{"local_media_probe_limit_exceeded"}) {
			t.Fatalf("box count warning = %v", warnings)
		}
	})
	t.Run("image-byte-budget", func(t *testing.T) {
		// JPEG APP15 segments precede SOF. DecodeConfig must stop at the byte
		// budget instead of scanning arbitrary amounts of application metadata.
		segment := make([]byte, 65537)
		copy(segment, []byte{0xff, 0xef, 0xff, 0xff})
		data := append([]byte{0xff, 0xd8}, bytes.Repeat(segment, 20)...)
		got, warnings := mediaProbeRun(t, data)
		mediaProbeAssertNull(t, got)
		if !reflect.DeepEqual(warnings, []string{"local_media_probe_limit_exceeded"}) {
			t.Fatalf("image byte budget warning = %v", warnings)
		}
	})
	t.Run("nested-unrecognized-containers", func(t *testing.T) {
		data := mediaProbeMovieHeader(0, 1000, 9000)
		for range 5000 {
			data = mediaProbeBox("moov", data)
		}
		got, warnings := mediaProbeRun(t, data)
		mediaProbeAssertNull(t, got)
		if len(warnings) == 0 {
			t.Fatal("nested nonstandard movie structure must not yield metadata")
		}
	})
}

type mediaProbeCancelContext struct {
	context.Context
	cancel context.CancelFunc
	calls  int
}

func (c *mediaProbeCancelContext) Err() error {
	c.calls++
	if c.calls == 20 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestProbeLocalMediaCancellation(t *testing.T) {
	data := bytes.Repeat(mediaProbeBox("free"), 100)
	for _, during := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var probeCtx context.Context = ctx
		if during {
			probeCtx = &mediaProbeCancelContext{Context: ctx, cancel: cancel}
		} else {
			cancel()
		}
		got, _, err := probeLocalMedia(probeCtx, mediaProbeFixture(t, data), int64(len(data)))
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("during=%v: error = %v, want context.Canceled", during, err)
		}
		mediaProbeAssertNull(t, got)
	}
}

func TestProbeLocalMediaReadErrorsAreSanitized(t *testing.T) {
	f := mediaProbeFixture(t, []byte("private-media-contents"))
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	got, _, err := probeLocalMedia(context.Background(), f, 22)
	mediaProbeAssertNull(t, got)
	if err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), f.Name()) {
		t.Fatalf("expected stable read error without path or content, got %v", err)
	}
	for _, cut := range []int64{8, 16, 40} {
		data := mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 2000))
		got, warnings, err := probeLocalMedia(context.Background(), mediaProbeFixture(t, data[:cut]), int64(len(data)))
		if err != nil || !reflect.DeepEqual(warnings, []string{"local_media_metadata_invalid"}) {
			t.Errorf("concurrent truncation: warnings = %v, error = %v", warnings, err)
		}
		mediaProbeAssertNull(t, got)
	}
}

func FuzzProbeLocalMedia(f *testing.F) {
	f.Add([]byte("video"))
	f.Add(mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 1250), mediaProbeTrack("vide", 0, 640, 480, 0)))
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte("GIF89a"))
	f.Add([]byte{0xff, 0xd8, 0xff, 0xe0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		got, warnings := mediaProbeRun(t, data)
		for _, warning := range warnings {
			if !strings.HasPrefix(warning, "local_media_") || strings.Contains(warning, "private") {
				t.Fatalf("unstable or sensitive warning %q", warning)
			}
		}
		mediaProbeCheckSchema(t, got)
	})
}
