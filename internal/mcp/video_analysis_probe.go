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
	"regexp"
	"strconv"
	"strings"
	"time"
)

type analysisSegment struct {
	Start, End float64
	Transition string
}

var sceneTimePattern = regexp.MustCompile(`pts_time:([0-9]+(?:\.[0-9]+)?)`)

// A single pass processes the whole video at source frame timestamps. Uniform
// thumbnail sampling misses short cuts and must not invent shot boundaries.
func detectLocalAnalysisShots(ctx context.Context, ffmpeg, input string, duration float64) ([]analysisSegment, error) {
	args := analysisFFmpegInput(input)
	args = append(args, "-map", "0:v:0", "-an", "-sn", "-dn", "-vf", fmt.Sprintf("scale=w='min(iw,160)':h='min(ih,160)':force_original_aspect_ratio=decrease,select='gt(scene,%.2f)',showinfo", analysisBoundaryThreshold), "-fps_mode", "vfr", "-f", "null", "-")
	output, err := runAnalysisProcess(ctx, ffmpeg, args, 256<<10)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if strings.HasPrefix(err.Error(), "analysis_limit_exceeded:") {
			return nil, err
		}
		return nil, errors.New("media_decode_failed: shot detection failed")
	}
	return parseAnalysisSceneTimes(string(output), duration)
}

func parseAnalysisSceneTimes(output string, duration float64) ([]analysisSegment, error) {
	segments := []analysisSegment{}
	start := 0.0
	transition := "start"
	for _, match := range sceneTimePattern.FindAllStringSubmatch(output, -1) {
		point, err := strconv.ParseFloat(match[1], 64)
		if err != nil || point <= start || point >= duration {
			continue
		}
		if point-start < analysisMinShotSeconds || duration-point < 0.04 {
			continue
		}
		segments = append(segments, analysisSegment{Start: start, End: point, Transition: transition})
		if len(segments) >= maxAnalysisShots {
			return nil, errors.New("analysis_limit_exceeded: too many candidate shots")
		}
		start = point
		transition = "candidate_cut"
	}
	return append(segments, analysisSegment{Start: start, End: duration, Transition: transition}), nil
}

func analysisFFmpegInput(input string) []string {
	return []string{"-nostdin", "-nostats", "-hide_banner", "-loglevel", "info", "-max_alloc", strconv.Itoa(maxFrameDecodeAlloc), "-filter_threads", "1", "-filter_complex_threads", "1", "-threads", "1", "-max_pixels", strconv.Itoa(maxFrameDecodePixels), "-protocol_whitelist", "file,pipe", "-f", "mov", "-enable_drefs", "0", "-use_absolute_path", "0", "-i", input}
}

type analysisOutput struct {
	bytes.Buffer
	limit    int
	cancel   context.CancelFunc
	overflow bool
}

func (w *analysisOutput) Write(p []byte) (int, error) {
	if w.Len()+len(p) > w.limit {
		w.overflow = true
		w.cancel()
		return 0, errors.New("analysis_output_limit")
	}
	return w.Buffer.Write(p)
}

func runAnalysisProcess(ctx context.Context, ffmpeg string, args []string, limit int) ([]byte, error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &analysisOutput{limit: limit, cancel: cancel}
	cmd := exec.CommandContext(child, ffmpeg, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = output
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if output.overflow {
		return nil, errors.New("analysis_limit_exceeded: decoder output exceeds limit")
	}
	return output.Bytes(), err
}

func analysisImageSize(data []byte) (int, int) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

var (
	meanVolumePattern   = regexp.MustCompile(`mean_volume:\s*(-?[0-9]+(?:\.[0-9]+)?)\s*dB`)
	maxVolumePattern    = regexp.MustCompile(`max_volume:\s*(-?[0-9]+(?:\.[0-9]+)?)\s*dB`)
	silenceStartPattern = regexp.MustCompile(`silence_start:\s*([0-9]+(?:\.[0-9]+)?)`)
	silenceEndPattern   = regexp.MustCompile(`silence_end:\s*([0-9]+(?:\.[0-9]+)?)`)
	audioStreamPattern  = regexp.MustCompile(`(?i)Stream #\d+:\d+.*:\s*Audio:`)
)

func probeAnalysisAudio(ctx context.Context, ffmpeg, input string) map[string]any {
	result := map[string]any{"status": "unavailable", "source": "ffmpeg", "stream_present": false, "speech_transcribed": false, "music_identified": false}
	child, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	args := append(analysisFFmpegInput(input), "-map", "0:a:0?", "-vn", "-sn", "-dn", "-af", "volumedetect,silencedetect=n=-45dB:d=0.25", "-f", "null", "-")
	data, err := runAnalysisProcess(child, ffmpeg, args, 256<<10)
	if err != nil {
		text := string(data)
		if audioStreamPattern.MatchString(text) {
			result["stream_present"] = true
		}
		if !audioStreamPattern.MatchString(text) || strings.Contains(text, "does not contain any stream") {
			result["status"] = "no_audio"
		}
		return result
	}
	text := string(data)
	streamPresent := audioStreamPattern.MatchString(text)
	result["stream_present"] = streamPresent
	if !streamPresent {
		result["status"] = "no_audio"
		return result
	}
	mean := firstAudioNumber(meanVolumePattern, text)
	max := firstAudioNumber(maxVolumePattern, text)
	if mean == nil && max == nil {
		result["status"] = "available"
		return result
	}
	result["status"] = "available"
	if mean != nil {
		result["mean_volume_db"] = *mean
	}
	if max != nil {
		result["max_volume_db"] = *max
	}
	silences := []map[string]float64{}
	var start *float64
	for _, line := range strings.Split(text, "\n") {
		if v := firstAudioNumber(silenceStartPattern, line); v != nil {
			start = v
		}
		if v := firstAudioNumber(silenceEndPattern, line); v != nil && start != nil {
			silences = append(silences, map[string]float64{"start_seconds": *start, "end_seconds": *v})
			start = nil
		}
		if len(silences) >= 100 {
			result["truncated"] = true
			break
		}
	}
	result["silence_segments"] = silences
	return result
}

func firstAudioNumber(pattern *regexp.Regexp, text string) *float64 {
	match := pattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return nil
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil
	}
	return &value
}
