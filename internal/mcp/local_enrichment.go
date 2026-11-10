package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxLocalTranscriptSegments = 400
	maxLocalTranscriptRunes    = 1000
	maxLocalOCRFrames          = 120
	maxLocalOCRTextItems       = 20
	maxLocalOCRTextRunes       = 500
	maxLocalPythonOutputBytes  = 8 << 20
	maxLocalPythonErrorBytes   = 64 << 10
	defaultLocalASRModel       = "small"
	defaultLocalASRDevice      = "cpu"
	defaultLocalASRComputeType = "int8"
)

type localOCRInput struct {
	ShotIndex    int
	Timestamp    float64
	RelativePath string
	Path         string
}

// Subtitle changes do not imply a scene cut. Sample independently of the
// small MCP keyframe gallery, retaining readable local evidence for review.
func subtitleOCRSamples(shots []analysisShot) ([]localOCRInput, bool) {
	if len(shots) == 0 {
		return nil, false
	}
	duration := shots[len(shots)-1].End
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return nil, false
	}
	interval := math.Max(0.75, duration/float64(maxLocalOCRFrames))
	count := min(int(math.Ceil(duration/interval)), maxLocalOCRFrames)
	frames := make([]localOCRInput, 0, count)
	shotIndex := 0
	for i := 0; i < count; i++ {
		stamp := math.Min((float64(i)+0.5)*interval, math.Max(0, duration-0.1))
		for shotIndex+1 < len(shots) && stamp >= shots[shotIndex].End {
			shotIndex++
		}
		frames = append(frames, localOCRInput{ShotIndex: shots[shotIndex].Index, Timestamp: stamp})
	}
	return frames, interval > 0.75
}

func prepareSubtitleOCRInputs(ctx context.Context, root *os.Root, id, ffmpeg, input string, shots []analysisShot, tempDir string) ([]localOCRInput, []string) {
	samples, limited := subtitleOCRSamples(shots)
	warnings := []string{}
	if limited {
		warnings = append(warnings, "video_ocr_frame_limit")
	}
	if root == nil {
		return nil, append(warnings, "ocr_keyframes_unavailable")
	}
	if err := root.Mkdir("ocr", 0700); err != nil && !os.IsExist(err) {
		return nil, append(warnings, "ocr_keyframes_unavailable")
	}
	ocrDir := filepath.Join(tempDir, "subtitle-frames")
	if err := os.MkdirAll(ocrDir, 0700); err != nil {
		return nil, append(warnings, "ocr_keyframes_unavailable")
	}
	frames := make([]localOCRInput, 0, len(samples))
	for i, frame := range samples {
		if ctx.Err() != nil {
			break
		}
		data, err := decodeLocalOCRFrame(ctx, ffmpeg, input, frame.Timestamp)
		if err != nil {
			warnings = appendUniqueLocalString(warnings, "ocr_subtitle_sampling_failed")
			continue
		}
		name := fmt.Sprintf("ocr/frame_%03d.jpg", i+1)
		if err := writeAnalysisFile(root, name, data); err != nil {
			warnings = appendUniqueLocalString(warnings, "ocr_keyframes_unavailable")
			continue
		}
		frame.Path = filepath.Join(ocrDir, fmt.Sprintf("frame_%03d.jpg", i+1))
		if err := os.WriteFile(frame.Path, data, 0600); err != nil {
			warnings = appendUniqueLocalString(warnings, "ocr_keyframes_unavailable")
			continue
		}
		frame.RelativePath = analysisReference(id, name)
		frames = append(frames, frame)
	}
	if len(frames) == 0 && ctx.Err() == nil {
		fallback, missing := prepareLocalOCRInputs(root, id, shots, tempDir)
		return fallback, append(warnings, missing...)
	}
	return frames, warnings
}

// OCR images stay on disk and are not MCP image responses. Keep a separate,
// bounded decode budget so subtitles need not fit the 48 KiB/640px gallery.
func decodeLocalOCRFrame(ctx context.Context, ffmpeg, input string, stamp float64) ([]byte, error) {
	args := analysisFFmpegInput(input)
	args = append(args[:len(args)-2], "-ss", strconv.FormatFloat(stamp, 'f', 6, 64), "-i", input)
	args = append(args, "-map", "0:v:0", "-frames:v", "1", "-an", "-sn", "-dn", "-vf", "scale=w='min(iw,1280)':h='min(ih,1280)':force_original_aspect_ratio=decrease", "-map_metadata", "-1", "-threads", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "2", "pipe:1")
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &analysisOutput{limit: 512 << 10, cancel: cancel}
	cmd := exec.CommandContext(child, ffmpeg, args...)
	cmd.Stdout, cmd.Stderr = output, io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return nil, errFrameDecodeFailed
	}
	if err := validateFrameJPEG(output.Bytes(), 1280); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func prepareLocalOCRInputs(root *os.Root, id string, shots []analysisShot, tempDir string) ([]localOCRInput, []string) {
	if root == nil {
		return nil, []string{"ocr_keyframes_unavailable"}
	}
	ocrDir := filepath.Join(tempDir, "ocr-frames")
	if err := os.MkdirAll(ocrDir, 0700); err != nil {
		return nil, []string{"ocr_keyframes_unavailable"}
	}
	inputs := make([]localOCRInput, 0, min(len(shots)*len(analysisKeyframeFractions), maxLocalOCRFrames))
	warnings := []string{}
	for _, shot := range shots {
		for frameIndex, frame := range shot.Keyframes {
			if len(inputs) >= maxLocalOCRFrames {
				return inputs, appendUniqueLocalString(warnings, "video_ocr_frame_limit")
			}
			data, err := readAnalysisKeyframe(root, id, frame.RelativePath)
			if err != nil {
				warnings = appendUniqueLocalString(warnings, "ocr_keyframes_unavailable")
				continue
			}
			path := filepath.Join(ocrDir, fmt.Sprintf("shot_%03d_%d.jpg", shot.Index, frameIndex+1))
			if err := os.WriteFile(path, data, 0600); err != nil {
				warnings = appendUniqueLocalString(warnings, "ocr_keyframes_unavailable")
				continue
			}
			inputs = append(inputs, localOCRInput{ShotIndex: shot.Index, Timestamp: frame.Timestamp, RelativePath: frame.RelativePath, Path: path})
		}
	}
	return inputs, warnings
}

type localPythonCommand struct {
	Executable string
	PrefixArgs []string
}

type localEnrichmentSettings struct {
	Enabled   bool
	ASRModel  string
	ASRDevice string
	Compute   string
	Timeout   time.Duration
	Python    localPythonCommand
}

type localProcessOutput struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *localProcessOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.overflow = true
		return 0, errors.New("local_process_output_limit")
	}
	return b.Buffer.Write(p)
}

func loadLocalEnrichmentSettings() (localEnrichmentSettings, error) {
	settings := localEnrichmentSettings{
		Enabled:   true,
		ASRModel:  defaultLocalASRModel,
		ASRDevice: defaultLocalASRDevice,
		Compute:   defaultLocalASRComputeType,
		Timeout:   15 * time.Minute,
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_ENRICHMENT"))) {
	case "0", "false", "no", "off":
		settings.Enabled = false
	case "", "1", "true", "yes", "on":
	default:
		return settings, errors.New("invalid_argument: WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_ENRICHMENT must be true or false")
	}
	if value := strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_ASR_MODEL")); value != "" {
		settings.ASRModel = value
	}
	if value := strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_ASR_DEVICE")); value != "" {
		settings.ASRDevice = value
	}
	if value := strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_ASR_COMPUTE_TYPE")); value != "" {
		settings.Compute = value
	}
	if raw := strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_TIMEOUT_SECONDS")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 10 || seconds > 1800 {
			return settings, errors.New("invalid_argument: WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_TIMEOUT_SECONDS must be between 10 and 1800")
		}
		settings.Timeout = time.Duration(seconds) * time.Second
	}
	for _, value := range []string{settings.ASRModel, settings.ASRDevice, settings.Compute} {
		if strings.ContainsAny(value, "\x00\r\n") || len([]rune(value)) > 256 {
			return settings, errors.New("invalid_argument: local video enhancement settings are invalid")
		}
	}
	python, err := findLocalPython()
	if err != nil {
		return settings, nil
	}
	settings.Python = python
	return settings, nil
}

func findLocalPython() (localPythonCommand, error) {
	configured := strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_PYTHON"))
	candidates := []string{}
	if configured != "" {
		candidates = append(candidates, configured)
	} else {
		// Like FFmpeg, an optional project-local Python runtime takes priority
		// over an unrelated system installation. Explicit configuration wins.
		directories := []string{}
		if executable, err := os.Executable(); err == nil {
			directories = append(directories, filepath.Dir(executable))
		}
		if cwd, err := os.Getwd(); err == nil {
			directories = append(directories, cwd)
		}
		for _, dir := range directories {
			for _, name := range []string{"Scripts/python.exe", "python.exe", "bin/python3"} {
				candidates = append(candidates, filepath.Join(dir, "tools", "python", filepath.FromSlash(name)))
			}
		}
		candidates = append(candidates, "python", "python3", "py")
	}
	for _, candidate := range candidates {
		if strings.ContainsAny(candidate, "\x00\r\n") {
			continue
		}
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		prefix := []string{}
		if base == "py" {
			prefix = []string{"-3"}
		}
		return localPythonCommand{Executable: path, PrefixArgs: prefix}, nil
	}
	return localPythonCommand{}, errors.New("python_unavailable")
}

func localEnrichmentCapability() map[string]any {
	settings, err := loadLocalEnrichmentSettings()
	capability := map[string]any{
		"enabled":          err == nil && settings.Enabled,
		"default":          "local",
		"asr_backend":      "faster-whisper",
		"ocr_backend":      "paddleocr",
		"python_available": settings.Python.Executable != "",
		"model":            settings.ASRModel,
		"device":           settings.ASRDevice,
		"compute_type":     settings.Compute,
		"status":           "python_ready",
		"backend_probe":    "deferred_until_analysis",
	}
	if err != nil {
		capability["status"] = "configuration_error"
		capability["error"] = strings.SplitN(err.Error(), ":", 2)[0]
	} else if !settings.Enabled {
		capability["status"] = "disabled"
	} else if settings.Python.Executable == "" {
		capability["status"] = "python_unavailable"
	}
	return capability
}

func (s *Server) runLocalEnrichment(ctx context.Context, ffmpeg, input, tempDir string, audio map[string]any, frames []localOCRInput) localEnrichmentReport {
	settings, err := loadLocalEnrichmentSettings()
	report := localEnrichmentReport{Enabled: true}
	if err != nil {
		report.Status = "unavailable"
		report.Enabled = false
		report.ASR = localASRReport{Status: "unavailable", Backend: "faster-whisper", MissingReason: "local_enrichment_configuration_invalid", Error: "local_enrichment_configuration_invalid"}
		report.OCR = localOCRReport{Status: "unavailable", Backend: "paddleocr", MissingReason: "local_enrichment_configuration_invalid", Error: "local_enrichment_configuration_invalid"}
		report.MissingReasons = []string{"local_enrichment_configuration_invalid"}
		report.Warnings = []string{"local_enrichment_configuration_invalid"}
		return report
	}
	if !settings.Enabled {
		report.Status = "disabled"
		report.Enabled = false
		report.ASR = localASRReport{Status: "disabled", MissingReason: "local_enrichment_disabled"}
		report.OCR = localOCRReport{Status: "disabled", MissingReason: "local_enrichment_disabled"}
		report.MissingReasons = []string{"local_enrichment_disabled"}
		report.Warnings = []string{"local_enrichment_disabled"}
		return report
	}
	report.ASR = localASRReport{Status: "unavailable", Backend: "faster-whisper", Model: settings.ASRModel, Device: settings.ASRDevice, MissingReason: "python_unavailable"}
	report.OCR = localOCRReport{Status: "unavailable", Backend: "paddleocr", MissingReason: "python_unavailable"}
	if settings.Python.Executable == "" {
		return finalizeLocalEnrichment(report)
	}

	workCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()
	if audioStreamPresent(audio) {
		audioPath := filepath.Join(tempDir, "audio.wav")
		if err := extractAnalysisAudio(workCtx, ffmpeg, input, audioPath); err != nil {
			report.ASR.MissingReason = "audio_extract_failed"
			report.ASR.Error = "audio_extract_failed"
		} else {
			report.ASR = runLocalASR(workCtx, settings, audioPath)
		}
	} else if audioStatus(audio) == "no_audio" {
		report.ASR = localASRReport{Status: "no_audio", Backend: "faster-whisper", Model: settings.ASRModel, Device: settings.ASRDevice, MissingReason: "audio_not_present"}
	} else if !audioStreamPresent(audio) {
		report.ASR = localASRReport{Status: "unavailable", Backend: "faster-whisper", Model: settings.ASRModel, Device: settings.ASRDevice, MissingReason: "audio_probe_unavailable", Error: "audio_probe_unavailable"}
	}
	if len(frames) == 0 {
		report.OCR = localOCRReport{Status: "unavailable", Backend: "paddleocr", MissingReason: "no_keyframes"}
	} else {
		report.OCR = runLocalOCR(workCtx, settings, tempDir, frames)
	}
	return finalizeLocalEnrichment(report)
}

func audioStatus(audio map[string]any) string {
	if audio == nil {
		return "unavailable"
	}
	status, _ := audio["status"].(string)
	return status
}

func audioStreamPresent(audio map[string]any) bool {
	if audio == nil {
		return false
	}
	present, _ := audio["stream_present"].(bool)
	if present {
		return true
	}
	status, _ := audio["status"].(string)
	return status == "available"
}

func extractAnalysisAudio(ctx context.Context, ffmpeg, input, output string) error {
	args := analysisFFmpegInput(input)
	// Let Go write the private temporary file. Some FFmpeg Windows builds
	// reject native file outputs, while decoding to a pipe works correctly.
	args = append(args, "-map", "0:a:0", "-vn", "-sn", "-dn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-f", "wav", "pipe:1")
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("audio_extract_failed")
	}
	defer file.Close()
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	writer := &analysisAudioWriter{file: file, cancel: cancel}
	cmd := exec.CommandContext(child, ffmpeg, args...)
	cmd.Stdout = writer
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("audio_extract_failed")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 44 {
		return errors.New("audio_extract_failed")
	}
	return nil
}

type analysisAudioWriter struct {
	file   *os.File
	bytes  int64
	cancel context.CancelFunc
}

func (w *analysisAudioWriter) Write(data []byte) (int, error) {
	// 30 minutes of 16 kHz mono 16-bit PCM plus a WAV header.
	if int64(len(data)) > (60<<20)-w.bytes {
		w.cancel()
		return 0, errors.New("audio_output_limit")
	}
	n, err := w.file.Write(data)
	w.bytes += int64(n)
	return n, err
}

func runLocalASR(ctx context.Context, settings localEnrichmentSettings, audioPath string) localASRReport {
	result := localASRReport{Status: "unavailable", Backend: "faster-whisper", Model: settings.ASRModel, Device: settings.ASRDevice, MissingReason: "asr_process_failed"}
	raw, err := runLocalPythonJSON(ctx, settings.Python, localASRScript, audioPath, settings.ASRModel, settings.ASRDevice, settings.Compute)
	if err != nil {
		result.MissingReason = classifyLocalBackendError(err, "asr_process_failed")
		result.Error = result.MissingReason
		return result
	}
	var payload struct {
		Status       string                   `json:"status"`
		Reason       string                   `json:"reason"`
		Backend      string                   `json:"backend"`
		Model        string                   `json:"model"`
		Device       string                   `json:"device"`
		Language     string                   `json:"language"`
		LanguageProb float64                  `json:"language_probability"`
		Duration     float64                  `json:"duration_seconds"`
		Segments     []localTranscriptSegment `json:"segments"`
		Truncated    bool                     `json:"truncated"`
	}
	if err := decodeLocalJSON(raw, &payload); err != nil {
		result.MissingReason = "asr_response_invalid"
		result.Error = result.MissingReason
		return result
	}
	if payload.Status != "completed" {
		result.MissingReason = firstNonEmpty(payload.Reason, "asr_model_unavailable")
		result.Error = result.MissingReason
		return result
	}
	result.Status = "completed"
	result.MissingReason = ""
	result.Backend = firstNonEmpty(payload.Backend, "faster-whisper")
	result.Model = firstNonEmpty(payload.Model, settings.ASRModel)
	result.Device = firstNonEmpty(payload.Device, settings.ASRDevice)
	result.Language = boundLocalText(payload.Language, 64)
	result.LanguageProb = boundedProbability(payload.LanguageProb)
	result.DurationSeconds = boundedSeconds(payload.Duration)
	result.Truncated = payload.Truncated
	for _, segment := range payload.Segments {
		if len(result.Segments) >= maxLocalTranscriptSegments {
			result.Truncated = true
			break
		}
		text := boundLocalText(strings.TrimSpace(segment.Text), maxLocalTranscriptRunes)
		if text == "" {
			continue
		}
		start, end := boundedSeconds(segment.Start), boundedSeconds(segment.End)
		if end < start {
			continue
		}
		result.Segments = append(result.Segments, localTranscriptSegment{Start: start, End: end, Text: text})
	}
	result.SegmentCount = len(result.Segments)
	return result
}

func runLocalOCR(ctx context.Context, settings localEnrichmentSettings, tempDir string, frames []localOCRInput) localOCRReport {
	result := localOCRReport{Status: "unavailable", Backend: "paddleocr", FrameCount: len(frames), MissingReason: "ocr_process_failed"}
	if len(frames) > maxLocalOCRFrames {
		frames = frames[:maxLocalOCRFrames]
		result.Truncated = true
	}
	requestPath := filepath.Join(tempDir, "ocr_request.json")
	request := make([]map[string]any, 0, len(frames))
	for _, frame := range frames {
		request = append(request, map[string]any{"shot_index": frame.ShotIndex, "timestamp_seconds": frame.Timestamp, "relative_path": frame.RelativePath, "path": frame.Path})
	}
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > maxLocalPythonOutputBytes {
		result.MissingReason = "ocr_request_too_large"
		result.Error = result.MissingReason
		return result
	}
	if err := os.WriteFile(requestPath, encoded, 0600); err != nil {
		result.MissingReason = "ocr_request_unavailable"
		result.Error = result.MissingReason
		return result
	}
	raw, err := runLocalPythonJSON(ctx, settings.Python, localOCRScript, requestPath)
	if err != nil {
		result.MissingReason = classifyLocalBackendError(err, "ocr_process_failed")
		result.Error = result.MissingReason
		return result
	}
	var payload struct {
		Status  string `json:"status"`
		Reason  string `json:"reason"`
		Backend string `json:"backend"`
		Frames  []struct {
			Status string `json:"status"`
			Texts  []struct {
				Text       string  `json:"text"`
				Confidence float64 `json:"confidence"`
			} `json:"texts"`
		} `json:"frames"`
	}
	if err := decodeLocalJSON(raw, &payload); err != nil {
		result.MissingReason = "ocr_response_invalid"
		result.Error = result.MissingReason
		return result
	}
	if payload.Status != "completed" {
		result.MissingReason = firstNonEmpty(payload.Reason, "ocr_model_unavailable")
		result.Error = result.MissingReason
		return result
	}
	result.Status = "completed"
	result.MissingReason = ""
	result.Backend = firstNonEmpty(payload.Backend, "paddleocr")
	for index, frame := range frames {
		item := localOCRFrame{ShotIndex: frame.ShotIndex, Timestamp: boundedSeconds(frame.Timestamp), RelativePath: frame.RelativePath, Status: "completed"}
		if index >= len(payload.Frames) {
			item.Status = "unavailable"
			item.MissingReason = "ocr_frame_missing"
		} else {
			item.Status = firstNonEmpty(payload.Frames[index].Status, "completed")
			for _, text := range payload.Frames[index].Texts {
				if len(item.Texts) >= maxLocalOCRTextItems {
					result.Truncated = true
					break
				}
				value := boundLocalText(strings.TrimSpace(text.Text), maxLocalOCRTextRunes)
				if value == "" {
					continue
				}
				item.Texts = append(item.Texts, localOCRText{Text: value, Confidence: boundedProbability(text.Confidence)})
			}
		}
		if item.Status != "completed" {
			item.MissingReason = firstNonEmpty(item.MissingReason, "ocr_frame_failed")
		}
		result.TextCount += len(item.Texts)
		result.Frames = append(result.Frames, item)
	}
	failed := 0
	for _, frame := range result.Frames {
		if frame.Status != "completed" {
			failed++
		}
	}
	if failed > 0 && failed < len(result.Frames) {
		result.Status = "partial"
	} else if failed == len(result.Frames) && len(result.Frames) > 0 {
		result.Status = "partial"
		result.MissingReason = "ocr_frame_failed"
	}
	return result
}

func finalizeLocalEnrichment(report localEnrichmentReport) localEnrichmentReport {
	// These summaries are derived from this run, never from an earlier attempt.
	report.MissingReasons = nil
	report.Warnings = nil
	if report.ASR.Status == "completed" {
		report.ASR.MissingReason, report.ASR.Error = "", ""
	}
	if report.OCR.Status == "completed" {
		report.OCR.MissingReason, report.OCR.Error = "", ""
	}
	if report.ASR.Status != "completed" && report.ASR.MissingReason != "" {
		report.MissingReasons = appendUniqueLocalString(report.MissingReasons, report.ASR.MissingReason)
	}
	if report.OCR.Status != "completed" && report.OCR.MissingReason != "" {
		report.MissingReasons = appendUniqueLocalString(report.MissingReasons, report.OCR.MissingReason)
	}
	switch {
	case report.ASR.Status == "completed" || report.OCR.Status == "completed" || report.OCR.Status == "partial":
		report.Status = "partial"
	case report.Status == "disabled":
	default:
		report.Status = "unavailable"
	}
	if report.ASR.Status == "completed" && report.OCR.Status == "completed" {
		report.Status = "completed"
	}
	if report.ASR.Status == "completed" && report.ASR.Truncated {
		report.Warnings = appendUniqueLocalString(report.Warnings, "audio_transcript_truncated")
	}
	if report.OCR.Status == "completed" && report.OCR.TextCount == 0 {
		report.Warnings = appendUniqueLocalString(report.Warnings, "video_ocr_no_text")
	}
	if report.OCR.Truncated {
		report.Warnings = appendUniqueLocalString(report.Warnings, "video_ocr_truncated")
	}
	for _, reason := range report.MissingReasons {
		report.Warnings = appendUniqueLocalString(report.Warnings, reason)
	}
	return report
}

func attachLocalEnrichmentToShots(manifest *videoAnalysisManifest) {
	if manifest == nil {
		return
	}
	if manifest.Audio != nil {
		manifest.Audio["speech_transcribed"] = manifest.LocalEnrichment.ASR.Status == "completed" && len(manifest.LocalEnrichment.ASR.Segments) > 0
	}
	for index := range manifest.Shots {
		shot := &manifest.Shots[index]
		if shot.Evidence == nil {
			shot.Evidence = map[string]any{}
		}
		delete(shot.Evidence, "local_transcript")
		delete(shot.Evidence, "local_ocr")
		transcript := make([]localTranscriptSegment, 0, 12)
		for _, segment := range manifest.LocalEnrichment.ASR.Segments {
			if segment.End < shot.Start || segment.Start > shot.End {
				continue
			}
			transcript = append(transcript, segment)
			if len(transcript) >= 30 {
				break
			}
		}
		if len(transcript) > 0 {
			shot.Evidence["local_transcript"] = transcript
		}
		ocrFrames := make([]localOCRFrame, 0, len(shot.Keyframes))
		for _, frame := range manifest.LocalEnrichment.OCR.Frames {
			if frame.ShotIndex == shot.Index {
				ocrFrames = append(ocrFrames, frame)
			}
		}
		if len(ocrFrames) > 0 {
			shot.Evidence["local_ocr"] = ocrFrames
		}
		shot.Evidence["local_enrichment_status"] = manifest.LocalEnrichment.Status
	}
}

func runLocalPythonJSON(ctx context.Context, python localPythonCommand, script string, args ...string) ([]byte, error) {
	if python.Executable == "" {
		return nil, errors.New("python_unavailable")
	}
	commandArgs := append([]string{}, python.PrefixArgs...)
	// With Python -c, argv[0] is "-c" and every following value is a
	// script argument. An extra "--" would shift both backend protocols.
	commandArgs = append(commandArgs, "-c", script)
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, python.Executable, commandArgs...)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "PYTHONIOENCODING=utf-8", "PADDLE_PDX_DISABLE_PROGRESS_BAR=1")
	stdout := &localProcessOutput{limit: maxLocalPythonOutputBytes}
	stderr := &localProcessOutput{limit: maxLocalPythonErrorBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if stdout.Len() > 0 {
			if status, parseErr := localPythonStatus(stdout.Bytes()); parseErr == nil && status != "" {
				return stdout.Bytes(), nil
			}
		}
		if stderr.overflow || stdout.overflow {
			return nil, errors.New("local_process_output_limit")
		}
		return nil, errors.New("local_python_failed")
	}
	if stdout.overflow || stderr.overflow {
		return nil, errors.New("local_process_output_limit")
	}
	return stdout.Bytes(), nil
}

func localPythonStatus(raw []byte) (string, error) {
	var payload struct {
		Status string `json:"status"`
	}
	if err := decodeLocalJSON(raw, &payload); err != nil {
		return "", err
	}
	return payload.Status, nil
}

func decodeLocalJSON(raw []byte, target any) error {
	lines := strings.Split(string(raw), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if err := json.Unmarshal([]byte(line), target); err == nil {
			return nil
		}
	}
	text := strings.TrimSpace(string(raw))
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return errors.New("local_json_invalid")
	}
	return json.Unmarshal([]byte(text[start:end+1]), target)
}

func classifyLocalBackendError(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "local_enrichment_timeout"
	}
	switch strings.SplitN(err.Error(), ":", 2)[0] {
	case "python_unavailable", "local_process_output_limit":
		return strings.SplitN(err.Error(), ":", 2)[0]
	}
	return fallback
}

func boundLocalText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > maxRunes {
		return string([]rune(value)[:maxRunes]) + "…"
	}
	return value
}

func boundedProbability(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func boundedSeconds(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendUniqueLocalString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

const localASRScript = `
import json, sys

audio_path, model_name, device, compute_type = sys.argv[1:5]
try:
    from faster_whisper import WhisperModel
except ModuleNotFoundError:
    print(json.dumps({"status": "unavailable", "reason": "asr_backend_unavailable", "backend": "faster-whisper"}, ensure_ascii=False))
    raise SystemExit(0)
try:
    model = WhisperModel(model_name, device=device, compute_type=compute_type)
    segments, info = model.transcribe(audio_path, beam_size=5, vad_filter=True, condition_on_previous_text=False)
    items = []
    truncated = False
    for segment in segments:
        text = str(getattr(segment, "text", "") or "").strip()
        if not text:
            continue
        if len(items) >= 400:
            truncated = True
            break
        items.append({"start_seconds": float(getattr(segment, "start", 0) or 0), "end_seconds": float(getattr(segment, "end", 0) or 0), "text": text})
    print(json.dumps({"status": "completed", "backend": "faster-whisper", "model": model_name, "device": device, "language": str(getattr(info, "language", "") or ""), "language_probability": float(getattr(info, "language_probability", 0) or 0), "duration_seconds": float(getattr(info, "duration", 0) or 0), "segments": items, "truncated": truncated}, ensure_ascii=False))
except Exception:
    print(json.dumps({"status": "unavailable", "reason": "asr_model_unavailable", "backend": "faster-whisper", "model": model_name}, ensure_ascii=False))
`

const localOCRScript = `
import json, os, sys

request_path = sys.argv[1]
with open(request_path, "r", encoding="utf-8") as stream:
    request = json.load(stream)
try:
    from paddleocr import PaddleOCR
except ModuleNotFoundError:
    print(json.dumps({"status": "unavailable", "reason": "ocr_backend_unavailable", "backend": "paddleocr"}, ensure_ascii=False))
    raise SystemExit(0)

def value_from(item, key, default=None):
    value = getattr(item, key, default)
    if callable(value):
        value = value()
    return value

def add_texts(rows, texts, scores):
    for index, text in enumerate(texts or []):
        text = str(text or "").strip()
        if not text:
            continue
        score = 0
        if scores is not None and index < len(scores):
            try:
                score = float(scores[index])
            except Exception:
                score = 0
        rows.append({"text": text, "confidence": score})
        if len(rows) >= 20:
            break

try:
    try:
        options = dict(lang="ch", device="cpu", cpu_threads=min(4, os.cpu_count() or 1), use_doc_orientation_classify=False, use_doc_unwarping=False, use_textline_orientation=False)
        # Windows Paddle 3.x oneDNN can initialize but fail on the first
        # inference with unsupported PIR attributes. Use the CPU kernels.
        if sys.platform == "win32":
            options["enable_mkldnn"] = False
        engine = PaddleOCR(**options)
    except TypeError:
        engine = PaddleOCR(lang="ch", use_angle_cls=True, show_log=False, use_gpu=False, enable_mkldnn=False, cpu_threads=min(4, os.cpu_count() or 1))
except Exception:
    print(json.dumps({"status": "unavailable", "reason": "ocr_model_unavailable", "backend": "paddleocr"}, ensure_ascii=False))
    raise SystemExit(0)

frames = []
for item in request:
    rows = []
    status = "completed"
    try:
        if hasattr(engine, "predict"):
            predicted = engine.predict(item["path"])
            for result in predicted:
                data = value_from(result, "json", result)
                if isinstance(data, str):
                    data = json.loads(data)
                if isinstance(data, dict) and isinstance(data.get("res"), dict):
                    data = data["res"]
                if isinstance(data, dict):
                    add_texts(rows, data.get("rec_texts", []), data.get("rec_scores", []))
        else:
            legacy = engine.ocr(item["path"], cls=True)
            page = legacy[0] if legacy else []
            for pair in page or []:
                if len(pair) >= 2 and len(pair[1]) >= 2:
                    rows.append({"text": str(pair[1][0] or "").strip(), "confidence": float(pair[1][1] or 0)})
                    if len(rows) >= 20:
                        break
    except Exception:
        status = "unavailable"
    frames.append({"status": status, "texts": rows})
print(json.dumps({"status": "completed", "backend": "paddleocr", "frames": frames}, ensure_ascii=False))
`
