package mcpserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFrameJPEGWriterBudget(t *testing.T) {
	if maxFrameJPEGBytes != 48<<10 {
		t.Fatalf("JPEG budget = %d, want 48 KiB", maxFrameJPEGBytes)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &frameJPEGWriter{cancel: cancel}
	payload := bytes.Repeat([]byte{0x55}, maxFrameJPEGBytes)
	if n, err := w.Write(payload); n != len(payload) || err != nil {
		t.Fatalf("exact budget write = %d, %v", n, err)
	}
	if ctx.Err() != nil || w.oversized || w.n != maxFrameJPEGBytes {
		t.Fatal("exact budget was rejected")
	}
	if n, err := w.Write([]byte{1, 2, 3}); n != 0 || err == nil {
		t.Fatalf("overflow write = %d, %v", n, err)
	}
	if !w.oversized || w.n != maxFrameJPEGBytes || ctx.Err() != context.Canceled {
		t.Fatal("overflow must keep the buffer bounded and cancel the process")
	}
	if !bytes.Equal(w.data[:w.n], payload) {
		t.Fatal("overflow changed retained bytes")
	}

	w = &frameJPEGWriter{cancel: cancel}
	if n, err := w.Write(append(payload, 1)); n != maxFrameJPEGBytes || err == nil || !w.oversized {
		t.Fatalf("single oversized write = %d, %v, oversized=%v", n, err, w.oversized)
	}
}

// Build a tiny executable in a test-owned directory. It exercises os/exec,
// streaming limits and process cancellation without a package-global runner,
// TestMain, a shell script, or changing the trusted FFmpeg binary.
func frameDecoderTestProcess(t *testing.T) string {
	t.Helper()
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skip("process tests require the Go toolchain")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	const program = `package main
import ("io"; "os"; "time")
func main() {
    os.Stderr.WriteString("private-path-token https://private.invalid/?token=secret\n")
    switch os.Getenv("WX_CHANNEL_FRAME_HELPER_MODE") {
    case "wait":
        if err := os.WriteFile(os.Getenv("WX_CHANNEL_FRAME_HELPER_READY"), []byte("ready"), 0600); err != nil { os.Exit(3) }
        time.Sleep(time.Minute)
        return
    case "overflow":
        b := make([]byte, 32 << 10)
        for { if _, err := os.Stdout.Write(b); err != nil { return } }
    }
    f, err := os.Open(os.Getenv("WX_CHANNEL_FRAME_HELPER_OUTPUT"))
    if err != nil { os.Exit(4) }
    defer f.Close()
    if _, err := io.Copy(os.Stdout, f); err != nil { os.Exit(5) }
    if os.Getenv("WX_CHANNEL_FRAME_HELPER_MODE") == "exit" { os.Exit(23) }
}
`
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	name := "frame-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, goPath, "build", "-o", path, source)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatalf("build private process fixture: %v", err)
	}
	return path
}

func frameDecoderTestJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func frameDecoderTestFailure(t *testing.T, data []byte, err error, prefix string) {
	t.Helper()
	if len(data) != 0 || err == nil || !strings.HasPrefix(err.Error(), prefix) {
		t.Fatalf("decode = %d bytes, %v; want no bytes and %s", len(data), err, prefix)
	}
	for _, private := range []string{"private-path-token", "private.invalid", "secret", "must-not-exist", "exit status"} {
		if strings.Contains(err.Error(), private) {
			t.Fatalf("decoder exposed private diagnostic %q", private)
		}
	}
}

func TestDecodeLocalFrameProcess(t *testing.T) {
	helper := frameDecoderTestProcess(t)
	dir := t.TempDir()
	output := filepath.Join(dir, "stdout")
	t.Setenv("WX_CHANNEL_FRAME_HELPER_OUTPUT", output)
	valid := frameDecoderTestJPEG(t, 160, 80)
	writeOutput := func(data []byte) {
		t.Helper()
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeOutput(valid)

	t.Run("valid", func(t *testing.T) {
		got, err := decodeLocalFrame(context.Background(), helper, "trusted snapshot.mp4", 0, 160)
		if err != nil || !bytes.Equal(got, valid) {
			t.Fatalf("valid child JPEG = %d bytes, %v", len(got), err)
		}
	})
	t.Run("invalid-arguments", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			seconds float64
			width   int
		}{
			{"negative-seek", -0.01, 160},
			{"nan", math.NaN(), 160},
			{"positive-infinity", math.Inf(1), 160},
			{"negative-infinity", math.Inf(-1), 160},
			{"negative-width", 0, -1},
			{"small-width", 0, 159},
			{"large-width", 0, 641},
			{"max-int-width", 0, math.MaxInt},
		} {
			t.Run(tc.name, func(t *testing.T) {
				data, err := decodeLocalFrame(context.Background(), helper, "trusted snapshot.mp4", tc.seconds, tc.width)
				frameDecoderTestFailure(t, data, err, "media_decode_failed")
			})
		}
		for _, tc := range []struct{ binary, input string }{{"", "snapshot.mp4"}, {helper, ""}} {
			data, err := decodeLocalFrame(context.Background(), tc.binary, tc.input, 0, 160)
			frameDecoderTestFailure(t, data, err, "media_decode_failed")
		}
	})
	t.Run("invalid-output", func(t *testing.T) {
		var pngData bytes.Buffer
		if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 160, 80))); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name string
			data []byte
		}{
			{"no-frame", nil},
			{"garbage", []byte("not a JPEG")},
			{"png", pngData.Bytes()},
			{"truncated", valid[:len(valid)-32]},
			{"width-over-request", frameDecoderTestJPEG(t, 161, 80)},
			{"height-over-request", frameDecoderTestJPEG(t, 80, 161)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				writeOutput(tc.data)
				data, err := decodeLocalFrame(context.Background(), helper, "trusted snapshot.mp4", 0, 160)
				frameDecoderTestFailure(t, data, err, "media_decode_failed")
			})
		}
		writeOutput(valid)
	})
	t.Run("child-failure-discards-valid-output", func(t *testing.T) {
		t.Setenv("WX_CHANNEL_FRAME_HELPER_MODE", "exit")
		data, err := decodeLocalFrame(context.Background(), helper, "trusted snapshot.mp4", 0, 160)
		frameDecoderTestFailure(t, data, err, "media_decode_failed")
	})
	t.Run("missing-binary", func(t *testing.T) {
		data, err := decodeLocalFrame(context.Background(), filepath.Join(dir, "must-not-exist"), "trusted snapshot.mp4", 0, 160)
		frameDecoderTestFailure(t, data, err, "media_decode_failed")
	})
	t.Run("unlimited-stdout-is-stopped", func(t *testing.T) {
		t.Setenv("WX_CHANNEL_FRAME_HELPER_MODE", "overflow")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		data, err := decodeLocalFrame(ctx, helper, "trusted snapshot.mp4", 0, 160)
		frameDecoderTestFailure(t, data, err, "response_too_large")
		if ctx.Err() != nil {
			t.Fatal("overflow did not stop the child before the caller's deadline")
		}
	})
	t.Run("cancel-running-process", func(t *testing.T) {
		t.Setenv("WX_CHANNEL_FRAME_HELPER_MODE", "wait")
		ready := filepath.Join(dir, "cancel-ready")
		t.Setenv("WX_CHANNEL_FRAME_HELPER_READY", ready)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := decodeLocalFrame(ctx, helper, "trusted snapshot.mp4", 0, 160)
			done <- err
		}()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.NewTimer(5 * time.Second)
		defer timeout.Stop()
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			}
			select {
			case err := <-done:
				t.Fatalf("process exited before cancellation: %v", err)
			case <-timeout.C:
				t.Fatal("process fixture did not start")
			case <-ticker.C:
			}
		}
		cancel()
		select {
		case err := <-done:
			if err != context.Canceled {
				t.Fatalf("cancellation = %v, want original context.Canceled", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("cancellation failed to reap the child")
		}
	})
	t.Run("deadline-running-process", func(t *testing.T) {
		t.Setenv("WX_CHANNEL_FRAME_HELPER_MODE", "wait")
		ready := filepath.Join(dir, "deadline-ready")
		t.Setenv("WX_CHANNEL_FRAME_HELPER_READY", ready)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		data, err := decodeLocalFrame(ctx, helper, "trusted snapshot.mp4", 0, 160)
		if err != context.DeadlineExceeded || len(data) != 0 {
			t.Fatalf("deadline = %d bytes, %v", len(data), err)
		}
		if _, err := os.Stat(ready); err != nil {
			t.Fatal("deadline did not exercise a running child")
		}
	})
}

func TestDecodeLocalFrameAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, err := decodeLocalFrame(ctx, "", "", math.NaN(), -1)
	if len(data) != 0 || err != context.Canceled {
		t.Fatalf("pre-canceled = %d bytes, %v", len(data), err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	data, err = decodeLocalFrame(ctx, "", "", 0, 0)
	if len(data) != 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pre-expired = %d bytes, %v", len(data), err)
	}
}

func frameDecoderFFmpegFixture(t *testing.T, ffmpeg string, args ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private snapshot with spaces.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	base := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-filter_threads", "1", "-filter_complex_threads", "1"}
	base = append(base, args...)
	base = append(base, "-threads", "1", "-y", path)
	cmd := exec.CommandContext(ctx, ffmpeg, base...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatalf("generate synthetic MP4: %v", err)
	}
	return path
}

func frameDecoderFFmpegImage(t *testing.T, ffmpeg, input string, seconds float64, width int) image.Image {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data, err := decodeLocalFrame(ctx, ffmpeg, input, seconds, width)
	if err != nil {
		t.Fatalf("decode synthetic MP4 at %.3f: %v", seconds, err)
	}
	if len(data) > 48<<10 {
		t.Fatalf("JPEG exceeds raw budget: %d", len(data))
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("invalid JPEG: %v", err)
	}
	return img
}

func TestDecodeLocalFrameFFmpeg(t *testing.T) {
	ffmpeg := os.Getenv("WX_CHANNEL_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("set WX_CHANNEL_TEST_FFMPEG to an existing trusted FFmpeg 7.1 executable")
	}
	t.Run("scale-default-boundaries-and-no-upscale", func(t *testing.T) {
		for _, tc := range []struct {
			name, size string
			width      int
			want       image.Point
		}{
			{"default-landscape", "960x540", 0, image.Pt(480, 270)},
			{"portrait-both-sides", "360x720", 480, image.Pt(240, 480)},
			{"minimum", "640x320", 160, image.Pt(160, 80)},
			{"maximum", "1280x640", 640, image.Pt(640, 320)},
			{"small-no-upscale", "64x32", 480, image.Pt(64, 32)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				input := frameDecoderFFmpegFixture(t, ffmpeg, "-f", "lavfi", "-i", "color=c=red:s="+tc.size+":r=5:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p")
				img := frameDecoderFFmpegImage(t, ffmpeg, input, 0, tc.width)
				if got := img.Bounds().Size(); got != tc.want {
					t.Fatalf("dimensions = %v, want %v", got, tc.want)
				}
			})
		}
	})
	t.Run("rotation", func(t *testing.T) {
		input := frameDecoderFFmpegFixture(t, ffmpeg, "-f", "lavfi", "-i", "color=c=black:s=320x160:r=5:d=1,drawbox=x=0:y=0:w=160:h=160:color=red:t=fill,drawbox=x=160:y=0:w=160:h=160:color=blue:t=fill", "-c:v", "libx264", "-pix_fmt", "yuv420p")
		// FFmpeg 7.1 writes display matrices via the input rotation override;
		// the legacy rotate metadata tag is not retained by this build.
		rotated := frameDecoderFFmpegFixture(t, ffmpeg, "-display_rotation:v:0", "90", "-i", input, "-c", "copy")
		f, err := os.Open(rotated)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		metadata, _, err := probeLocalMedia(context.Background(), f, info.Size())
		if err != nil || metadata["width"] != 160 || metadata["height"] != 320 {
			t.Fatalf("fixture does not contain a 90-degree rotation matrix: %v, %v", metadata, err)
		}
		img := frameDecoderFFmpegImage(t, ffmpeg, rotated, 0, 160)
		if img.Bounds().Size() != image.Pt(80, 160) {
			t.Fatalf("rotation dimensions = %v, want (80,160)", img.Bounds().Size())
		}
		top := color.NRGBAModel.Convert(img.At(40, 20)).(color.NRGBA)
		bottom := color.NRGBAModel.Convert(img.At(40, 140)).(color.NRGBA)
		if top.B < 150 || top.R > 80 || bottom.R < 150 || bottom.B > 80 {
			t.Fatalf("90-degree rotation pixels: top=%v bottom=%v", top, bottom)
		}
	})
	t.Run("random-seek-uses-requested-content", func(t *testing.T) {
		input := frameDecoderFFmpegFixture(t, ffmpeg, "-f", "lavfi", "-i", "color=c=red:s=320x160:r=10:d=1", "-f", "lavfi", "-i", "color=c=blue:s=320x160:r=10:d=1", "-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v]", "-map", "[v]", "-c:v", "libx264", "-g", "100", "-pix_fmt", "yuv420p")
		for _, tc := range []struct {
			seconds float64
			blue    bool
		}{{1.25, true}, {0.25, false}, {1.75, true}} {
			img := frameDecoderFFmpegImage(t, ffmpeg, input, tc.seconds, 160)
			pixel := color.NRGBAModel.Convert(img.At(80, 40)).(color.NRGBA)
			if tc.blue && (pixel.B < 150 || pixel.R > 80) || !tc.blue && (pixel.R < 150 || pixel.B > 80) {
				t.Fatalf("seek %.2f returned wrong content: %v", tc.seconds, pixel)
			}
		}
		data, err := decodeLocalFrame(context.Background(), ffmpeg, input, 10, 160)
		frameDecoderTestFailure(t, data, err, "media_decode_failed")
	})
	t.Run("corrupt-missing-and-non-mov", func(t *testing.T) {
		dir := t.TempDir()
		for _, tc := range []struct {
			name string
			data []byte
		}{
			{"empty.mp4", nil},
			{"private-path-token.mp4", []byte("corrupt private-path-token media")},
			{"jpeg-disguised-as.mp4", frameDecoderTestJPEG(t, 160, 80)},
		} {
			path := filepath.Join(dir, tc.name)
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			data, err := decodeLocalFrame(context.Background(), ffmpeg, path, 0, 160)
			frameDecoderTestFailure(t, data, err, "media_decode_failed")
		}
		data, err := decodeLocalFrame(context.Background(), ffmpeg, filepath.Join(dir, "must-not-exist.mp4"), 0, 160)
		frameDecoderTestFailure(t, data, err, "media_decode_failed")
	})
	t.Run("audio-only-has-no-frame", func(t *testing.T) {
		input := frameDecoderFFmpegFixture(t, ffmpeg, "-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono", "-t", "0.2", "-c:a", "aac")
		data, err := decodeLocalFrame(context.Background(), ffmpeg, input, 0, 160)
		frameDecoderTestFailure(t, data, err, "media_decode_failed")
	})
	t.Run("corrupt-video-packets", func(t *testing.T) {
		input := frameDecoderFFmpegFixture(t, ffmpeg, "-f", "lavfi", "-i", "color=c=red:s=320x160:r=5:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p")
		movie, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		// This synthetic MP4 has one ordinary mdat, before moov. Keep its
		// metadata intact but invalidate every NAL unit in the media payload.
		mdat := bytes.Index(movie, []byte("mdat"))
		if mdat < 4 {
			t.Fatal("fixture has no mdat")
		}
		size := int(binary.BigEndian.Uint32(movie[mdat-4 : mdat]))
		if size < 8 || mdat-4+size > len(movie) {
			t.Fatal("fixture has an unexpected mdat size")
		}
		clear(movie[mdat+4 : mdat-4+size])
		if err := os.WriteFile(input, movie, 0600); err != nil {
			t.Fatal(err)
		}
		data, err := decodeLocalFrame(context.Background(), ffmpeg, input, 0, 160)
		frameDecoderTestFailure(t, data, err, "media_decode_failed")
	})
	t.Run("high-entropy-jpeg-exceeds-budget", func(t *testing.T) {
		input := frameDecoderFFmpegFixture(t, ffmpeg, "-f", "lavfi", "-i", "nullsrc=s=640x640:r=1:d=1,geq=lum='random(1)*255':cb='random(2)*255':cr='random(3)*255'", "-c:v", "libx264", "-qp", "0", "-pix_fmt", "yuv444p")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		data, err := decodeLocalFrame(ctx, ffmpeg, input, 0, 640)
		frameDecoderTestFailure(t, data, err, "response_too_large")
	})
}
