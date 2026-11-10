package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"math"
	"os/exec"
	"strconv"
	"time"
)

const (
	maxFrameJPEGBytes = 48 << 10
	// These bound individual FFmpeg allocations and decoded source dimensions,
	// not the process's aggregate memory. 4K video fits within the pixel limit.
	maxFrameDecodeAlloc  = 64 << 20
	maxFrameDecodePixels = 4096 * 4096
)

var (
	errFrameDecodeFailed = errors.New("media_decode_failed: cannot decode a local video frame")
	errFrameJPEGTooLarge = errors.New("response_too_large: decoded frame exceeds the JPEG byte limit")
)

// decodeLocalFrame accepts only a trusted executable and a private MP4 snapshot
// copied from the caller's safely opened source handle. Path validation,
// snapshot lifecycle and the overall operation deadline belong to the caller.
func decodeLocalFrame(ctx context.Context, ffmpegPath, inputPath string, seconds float64, maxWidth int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxWidth == 0 {
		maxWidth = 480
	}
	if ffmpegPath == "" || inputPath == "" || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || maxWidth < 160 || maxWidth > 640 {
		return nil, errFrameDecodeFailed
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-xerror",
		"-max_alloc", strconv.Itoa(maxFrameDecodeAlloc),
		"-filter_threads", "1", "-filter_complex_threads", "1",
		"-threads", "1", "-max_pixels", strconv.Itoa(maxFrameDecodePixels),
		"-err_detect", "explode",
		"-protocol_whitelist", "file,pipe",
		"-f", "mov", "-enable_drefs", "0", "-use_absolute_path", "0",
		"-ss", strconv.FormatFloat(seconds, 'f', 6, 64), "-i", inputPath,
		"-map", "0:v:0", "-frames:v", "1", "-an", "-sn", "-dn",
		"-vf", fmt.Sprintf("scale=w='min(iw,%d)':h='min(ih,%d)':force_original_aspect_ratio=decrease", maxWidth, maxWidth),
		"-map_metadata", "-1", "-threads", "1",
		"-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "5", "pipe:1",
	}
	processCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &frameJPEGWriter{cancel: cancel}
	cmd := exec.CommandContext(processCtx, ffmpegPath, args...)
	cmd.Stdout = output
	// Decoder diagnostics can contain local paths and media metadata. Discard
	// them while streaming, without ever buffering, logging or wrapping them.
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if output.oversized {
		return nil, errFrameJPEGTooLarge
	}
	if err != nil {
		return nil, errFrameDecodeFailed
	}
	data := output.data[:output.n]
	if err := validateFrameJPEG(data, maxWidth); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func validateFrameJPEG(data []byte, maxWidth int) error {
	if len(data) < 4 || !bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) || !bytes.HasSuffix(data, []byte{0xff, 0xd9}) {
		return errFrameDecodeFailed
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > maxWidth || config.Height > maxWidth {
		return errFrameDecodeFailed
	}
	// DecodeConfig alone accepts a valid header with a corrupt/truncated scan.
	// Only decode the scan after dimensions have bounded Go's allocations.
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		return errFrameDecodeFailed
	}
	return nil
}

// os/exec writes serially to this fixed buffer and joins the copy goroutine
// before Run returns. Overflow cancels the child immediately, including a
// decoder that keeps writing indefinitely after the pipe's budget is spent.
type frameJPEGWriter struct {
	data      [maxFrameJPEGBytes]byte
	n         int
	oversized bool
	cancel    context.CancelFunc
}

func (w *frameJPEGWriter) Write(p []byte) (int, error) {
	n := copy(w.data[w.n:], p)
	w.n += n
	if n < len(p) {
		w.oversized = true
		w.cancel()
		return n, errFrameJPEGTooLarge
	}
	return n, nil
}
