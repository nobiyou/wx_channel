package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/jpeg"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxVideoFrames            = 3
	maxSourceVideoBytes int64 = 256 << 20
	maxFrameTimeSeconds       = 21600
)

type localVideoFramesInput struct {
	RelativePath      string    `json:"relative_path,omitempty" jsonschema:"video path relative to the downloads directory; choose either this or record_id"`
	RecordID          string    `json:"record_id,omitempty" jsonschema:"stored download record id; choose either this or relative_path"`
	Source            string    `json:"source,omitempty" jsonschema:"browse or download when using record_id"`
	TimestampsSeconds []float64 `json:"timestamps_seconds,omitempty" jsonschema:"one to three requested offsets in seconds; omit for uniform sampling"`
	FrameCount        int       `json:"frame_count,omitempty" jsonschema:"number of uniform samples, default 3, maximum 3; omit with timestamps_seconds"`
	MaxWidth          int       `json:"max_width,omitempty" jsonschema:"maximum image side in pixels, default 480, between 160 and 640"`
}

func validateFrameOptions(input localVideoFramesInput) error {
	if input.FrameCount < 0 || input.FrameCount > maxVideoFrames || len(input.TimestampsSeconds) > maxVideoFrames || (input.FrameCount != 0 && len(input.TimestampsSeconds) > 0) {
		return errors.New("invalid_argument: use at most three timestamps or a frame_count between one and three")
	}
	if input.MaxWidth != 0 && (input.MaxWidth < 160 || input.MaxWidth > 640) {
		return errors.New("invalid_argument: max_width must be between 160 and 640")
	}
	seen := map[float64]bool{}
	for _, stamp := range input.TimestampsSeconds {
		if math.IsNaN(stamp) || math.IsInf(stamp, 0) || stamp < 0 || stamp > maxFrameTimeSeconds || seen[stamp] {
			return errors.New("invalid_argument: timestamps must be distinct finite offsets between zero and 21600 seconds")
		}
		seen[stamp] = true
	}
	return nil
}

func planVideoFrames(input localVideoFramesInput, duration float64) ([]float64, string, error) {
	if err := validateFrameOptions(input); err != nil {
		return nil, "", err
	}
	if len(input.TimestampsSeconds) > 0 {
		for _, stamp := range input.TimestampsSeconds {
			if duration > 0 && stamp >= duration {
				return nil, "", errors.New("invalid_argument: requested timestamp is outside the video duration")
			}
		}
		return append([]float64(nil), input.TimestampsSeconds...), "timestamps", nil
	}
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return nil, "", errors.New("media_duration_unavailable: choose explicit timestamps when file duration is unavailable")
	}
	count := input.FrameCount
	if count == 0 {
		count = maxVideoFrames
	}
	points := make([]float64, count)
	for i := range points {
		points[i] = duration * (float64(i) + 0.5) / float64(count)
		if points[i] > maxFrameTimeSeconds {
			return nil, "", errors.New("invalid_argument: use explicit timestamps within the supported sampling range")
		}
	}
	return points, "uniform", nil
}

func findFFmpeg(configured string) (string, error) {
	check := func(candidate string) (string, bool) {
		if !filepath.IsAbs(candidate) {
			return "", false
		}
		info, err := os.Stat(candidate)
		return candidate, err == nil && info.Mode().IsRegular()
	}
	if configured = strings.TrimSpace(configured); configured != "" {
		if path, ok := check(configured); ok {
			return path, nil
		}
		return "", errors.New("media_decoder_unavailable: the configured FFmpeg executable is unavailable")
	}
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if executable, err := os.Executable(); err == nil {
		if path, ok := check(filepath.Join(filepath.Dir(executable), "tools", "ffmpeg", "bin", name)); ok {
			return path, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		if path, ok := check(path); ok {
			return path, nil
		}
	}
	return "", errors.New("media_decoder_unavailable: FFmpeg is not available")
}

func (s *Server) getLocalVideoFrames(ctx context.Context, _ *mcp.CallToolRequest, input localVideoFramesInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := validateFrameOptions(input); err != nil {
		return nil, nil, err
	}
	if (strings.TrimSpace(input.RelativePath) == "") == (strings.TrimSpace(input.RecordID) == "") {
		return nil, nil, errors.New("invalid_argument: choose exactly one of relative_path and record_id")
	}
	if input.Source != "" && input.Source != "browse" && input.Source != "download" {
		return nil, nil, errors.New("invalid_argument: source must be browse or download")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	media, err := s.openLocalMedia(ctx, localMediaMetadataInput{RelativePath: input.RelativePath, RecordID: input.RecordID, Source: input.Source})
	if err != nil {
		return nil, nil, err
	}
	defer media.file.Close()
	switch strings.ToLower(filepath.Ext(media.relativePath)) {
	case ".mp4", ".mov", ".m4v":
	default:
		return nil, nil, errors.New("media_format_unsupported: frame sampling currently supports MP4 and MOV videos")
	}
	if media.info.Size() <= 0 || media.info.Size() > maxSourceVideoBytes {
		return nil, nil, errors.New("media_file_too_large: frame sampling accepts nonempty videos up to 256 MiB")
	}
	ffmpegPath, err := findFFmpeg(s.deps.FFmpegPath)
	if err != nil {
		return nil, nil, err
	}
	metadata, probeWarnings, err := probeLocalMedia(ctx, media.file, media.info.Size())
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, errors.New("local_file_unavailable: cannot read local video headers")
	}
	duration, _ := metadata["duration_seconds"].(float64)
	points, mode, err := planVideoFrames(input, duration)
	if err != nil {
		return nil, nil, err
	}
	// A bounded private snapshot lets FFmpeg seek a tail-moov file without
	// reopening an untrusted source path or granting it access to other files.
	dir, err := os.MkdirTemp("", "wx-channel-frames-")
	if err != nil {
		return nil, nil, errors.New("local_file_unavailable: cannot prepare frame extraction")
	}
	defer os.RemoveAll(dir)
	inputPath := filepath.Join(dir, "input.mp4")
	if err := snapshotFrameSource(ctx, media, inputPath); err != nil {
		return nil, nil, err
	}
	width := input.MaxWidth
	if width == 0 {
		width = 480
	}
	frames := make([]map[string]any, 0, len(points))
	images := make([][]byte, 0, len(points))
	for i, stamp := range points {
		data, err := decodeLocalFrame(ctx, ffmpegPath, inputPath, stamp, width)
		if err != nil {
			return nil, nil, err
		}
		config, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, nil, errors.New("media_decode_failed: the decoder did not return a valid image")
		}
		frames = append(frames, map[string]any{"index": i + 1, "requested_timestamp_seconds": stamp, "width": config.Width, "height": config.Height, "mime_type": "image/jpeg"})
		images = append(images, data)
	}
	warnings := appendLocalWarnings(probeWarnings, "sampled_frames_not_full_video", "audio_not_analyzed")
	data := map[string]any{"relative_path": media.relativePath, "file_modified_at": media.info.ModTime().UTC().Format(time.RFC3339), "duration_seconds": metadata["duration_seconds"], "sampling": mode, "frames": frames, "frame_count": len(frames), "content_scope": "sampled_frames_only"}
	if media.record != nil {
		data["record_id"] = s.recordMap(*media.record)["id"]
		data["record_source"] = media.record.Source
	}
	out := envelope("local_file", "stored", data, warnings)
	manifest, err := json.Marshal(out)
	if err != nil {
		return nil, nil, errors.New("media_decode_failed: cannot encode frame manifest")
	}
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(manifest)}}}
	for i, image := range images {
		label, _ := json.Marshal(frames[i])
		result.Content = append(result.Content, &mcp.TextContent{Text: string(label)}, &mcp.ImageContent{MIMEType: "image/jpeg", Data: image})
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return result, out, nil
}

type frameCopyReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *frameCopyReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func snapshotFrameSource(ctx context.Context, media *openedLocalMedia, target string) error {
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("local_file_unavailable: cannot prepare frame extraction")
	}
	count, copyErr := io.CopyBuffer(file, io.LimitReader(&frameCopyReader{ctx, media.file}, media.info.Size()+1), make([]byte, 128<<10))
	closeErr := file.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	if copyErr != nil || closeErr != nil {
		return errors.New("local_file_unavailable: cannot read the source video")
	}
	info, err := media.file.Stat()
	if err != nil || count != media.info.Size() || info.Size() != media.info.Size() || !info.ModTime().Equal(media.info.ModTime()) {
		return errors.New("local_file_unavailable: source video changed during frame extraction")
	}
	return nil
}
