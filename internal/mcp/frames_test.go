package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"image/jpeg"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"wx_channel/internal/config"
)

func TestFrameSamplingLimitsAndUnknownDuration(t *testing.T) {
	points, mode, err := planVideoFrames(localVideoFramesInput{}, 12)
	if err != nil || mode != "uniform" || len(points) != 3 || points[0] != 2 || points[1] != 6 || points[2] != 10 {
		t.Fatalf("uniform plan: %v, %s, %v", points, mode, err)
	}
	points, mode, err = planVideoFrames(localVideoFramesInput{TimestampsSeconds: []float64{0, 2.5}}, 0)
	if err != nil || mode != "timestamps" || len(points) != 2 || points[1] != 2.5 {
		t.Fatalf("explicit unknown-duration plan: %v, %s, %v", points, mode, err)
	}
	for _, input := range []localVideoFramesInput{
		{FrameCount: 4}, {FrameCount: -1}, {FrameCount: 2, TimestampsSeconds: []float64{1}},
		{TimestampsSeconds: []float64{-1}}, {TimestampsSeconds: []float64{math.NaN()}}, {TimestampsSeconds: []float64{math.Inf(1)}},
		{TimestampsSeconds: []float64{12}}, {TimestampsSeconds: []float64{0, 1, 2, 3}}, {TimestampsSeconds: []float64{1, 1}},
		{TimestampsSeconds: []float64{21601}},
	} {
		if _, _, err := planVideoFrames(input, 12); err == nil {
			t.Errorf("invalid plan accepted: %+v", input)
		}
	}
	if _, _, err := planVideoFrames(localVideoFramesInput{}, 0); err == nil || !strings.HasPrefix(err.Error(), "media_duration_unavailable:") {
		t.Fatalf("unknown duration: %v", err)
	}
}

func TestFrameSourceIsConfinedAndReadOnly(t *testing.T) {
	s := localContractServer(t)
	for _, path := range []string{"../outside.mp4", "../config.yaml"} {
		_, _, err := s.getLocalVideoFrames(context.Background(), nil, localVideoFramesInput{RelativePath: path})
		if err == nil || !strings.Contains(err.Error(), "path_forbidden") {
			t.Errorf("unsafe path %q: %v", path, err)
		}
	}
	path := filepath.Join(s.deps.DownloadsDir, "video.mp4")
	data := mediaProbeBox("moov", mediaProbeMovieHeader(0, 1000, 12000), mediaProbeTrack("vide", 0, 320, 240, 0))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s.deps.FFmpegPath = filepath.Join(t.TempDir(), "missing-ffmpeg.exe")
	_, _, err := s.getLocalVideoFrames(context.Background(), nil, localVideoFramesInput{RelativePath: "video.mp4"})
	if err == nil || !strings.HasPrefix(err.Error(), "media_decoder_unavailable:") {
		t.Fatalf("missing decoder: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(data) {
		t.Fatal("source video was modified")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.getLocalVideoFrames(ctx, nil, localVideoFramesInput{RelativePath: "video.mp4"}); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestMCPImageBudgetIncludesActualImagePayload(t *testing.T) {
	out := envelope("local_file", "stored", map[string]any{"frames": []any{}}, nil)
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/jpeg", Data: make([]byte, 200<<10)}}}
	size, err := serializedCallToolResultSize(result, out)
	if err != nil || size <= maxToolResponseBytes {
		t.Fatalf("image payload not counted: %d, %v", size, err)
	}
	result.StructuredContent = out
	wire, err := json.Marshal(result)
	if err != nil || size != len(wire)+256 {
		t.Fatalf("wire budget mismatch: estimated %d, actual %d, %v", size, len(wire), err)
	}
}

func TestFrameSnapshotUsesOpenHandleAndChecksChanges(t *testing.T) {
	s := localContractServer(t)
	path := filepath.Join(s.deps.DownloadsDir, "video.mp4")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	media, err := s.openLocalMedia(context.Background(), localMediaMetadataInput{RelativePath: "video.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	defer media.file.Close()
	target := filepath.Join(t.TempDir(), "snapshot.mp4")
	if err := snapshotFrameSource(context.Background(), media, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "original" {
		t.Fatalf("snapshot: %q, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("changed video"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _ = media.file.Seek(0, 0)
	if err := snapshotFrameSource(context.Background(), media, filepath.Join(t.TempDir(), "changed.mp4")); err == nil {
		t.Fatal("source changed during snapshot was accepted")
	}
}

func TestFrameImagesAndBudgetThroughMCP(t *testing.T) {
	ffmpeg := os.Getenv("WX_CHANNEL_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("set WX_CHANNEL_TEST_FFMPEG to an existing FFmpeg for real frame extraction")
	}
	snapshotDir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", snapshotDir)
	} else {
		t.Setenv("TMPDIR", snapshotDir)
	}
	assertSnapshotsRemoved := func() {
		t.Helper()
		entries, err := os.ReadDir(snapshotDir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("temporary media snapshots retained: %v, %v", entries, err)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "real.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10:duration=3", "-threads", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", path)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create synthetic video: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: dir, FFmpegPath: ffmpeg})
	if err != nil {
		t.Fatal(err)
	}
	registerReadTool(s, &mcp.Tool{Name: "test_oversize_image"}, func(context.Context, *mcp.CallToolRequest, emptyInput) (*mcp.CallToolResult, map[string]any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/jpeg", Data: make([]byte, 200<<10)}}}, envelope("local_file", "stored", map[string]any{}, nil), nil
	})
	server := httptest.NewServer(s.handler)
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "frame-transport-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearerRoundTripper{base: http.DefaultTransport, token: s.token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_local_video_frames", Arguments: map[string]any{"relative_path": "real.mp4"}})
	if err != nil || result.IsError {
		t.Fatalf("frame tool: %v, %v", result, err)
	}
	wire, err := json.Marshal(result)
	if err != nil || len(wire)+256 > maxToolResponseBytes {
		t.Fatalf("frame wire limit: %d, %v", len(wire), err)
	}
	if strings.Contains(string(wire), filepath.ToSlash(dir)) || strings.Contains(string(wire), "wx-channel-frames-") {
		t.Fatal("absolute or temporary media path leaked")
	}
	var frames [][]byte
	for _, content := range result.Content {
		if frame, ok := content.(*mcp.ImageContent); ok {
			config, err := jpeg.DecodeConfig(bytes.NewReader(frame.Data))
			if err != nil || config.Width != 160 || config.Height != 120 || frame.MIMEType != "image/jpeg" {
				t.Fatalf("invalid delivered frame: %+v, %v", config, err)
			}
			frames = append(frames, frame.Data)
		}
	}
	if len(frames) != 3 || bytes.Equal(frames[0], frames[1]) || bytes.Equal(frames[1], frames[2]) {
		t.Fatalf("expected three distinct source frames, got %d", len(frames))
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("frame tool modified the source")
	}
	assertSnapshotsRemoved()
	if err := os.WriteFile(filepath.Join(dir, "damaged.mp4"), []byte("not a video"), 0600); err != nil {
		t.Fatal(err)
	}
	damaged, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_local_video_frames", Arguments: map[string]any{"relative_path": "damaged.mp4", "timestamps_seconds": []float64{0}}})
	if err != nil || !damaged.IsError {
		t.Fatalf("damaged video accepted: %v, %v", damaged, err)
	}
	encodedFailure, _ := json.Marshal(damaged.StructuredContent)
	if !strings.Contains(string(encodedFailure), "media_decode_failed") {
		t.Fatalf("missing safe decode error: %s", encodedFailure)
	}
	assertSnapshotsRemoved()
	oversize, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "test_oversize_image"})
	if err != nil || !oversize.IsError {
		t.Fatalf("oversized image accepted: %v, %v", oversize, err)
	}
	encoded, _ := json.Marshal(oversize.StructuredContent)
	if !strings.Contains(string(encoded), "response_too_large") {
		t.Fatalf("missing safe image limit error: %s", encoded)
	}
}

func TestFrameTimeoutFailureContract(t *testing.T) {
	err := failure("media_decode_timeout")
	if err.Code != "media_decode_timeout" || !strings.Contains(err.Recovery, "max_width") {
		t.Fatalf("local timeout contract = %#v", err)
	}
}
