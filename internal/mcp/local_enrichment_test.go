package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLocalPythonJSONPreservesScriptArguments(t *testing.T) {
	python, err := findLocalPython()
	if err != nil {
		t.Skip("Python is not installed")
	}
	want := []string{"audio with spaces.wav", "small", "cpu", "int8"}
	raw, err := runLocalPythonJSON(t.Context(), python, `import json,sys; print(json.dumps({"args":sys.argv[1:]}))`, want...)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.Args, "|") != strings.Join(want, "|") {
		t.Fatalf("backend received shifted arguments: %q, want %q", result.Args, want)
	}
}

func TestSubtitleOCRSamplesAreIndependentOfShotKeyframes(t *testing.T) {
	frames, limited := subtitleOCRSamples([]analysisShot{{Index: 1, Start: 0, End: 3}, {Index: 2, Start: 3, End: 4.5}})
	if limited || len(frames) != 6 {
		t.Fatalf("samples/limited = %d/%v", len(frames), limited)
	}
	for i, frame := range frames {
		if frame.Timestamp != (float64(i)+0.5)*0.75 || frame.ShotIndex != 1+i/4 {
			t.Fatalf("sample %d = %#v", i, frame)
		}
	}
	frames, limited = subtitleOCRSamples([]analysisShot{{Index: 1, End: 1800}})
	if !limited || len(frames) != maxLocalOCRFrames || frames[len(frames)-1].Timestamp >= 1800 {
		t.Fatalf("long video samples/limited = %d/%v", len(frames), limited)
	}
}

func TestPrepareSubtitleOCRInputsKeepsReadableFrames(t *testing.T) {
	ffmpeg := os.Getenv("WX_CHANNEL_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("set WX_CHANNEL_TEST_FFMPEG for decoder validation")
	}
	source := frameDecoderFFmpegFixture(t, firstNonEmpty(os.Getenv("WX_CHANNEL_TEST_FIXTURE_FFMPEG"), ffmpeg), "-f", "lavfi", "-i", "color=c=white:s=720x1000:r=10:d=2.6", "-c:v", "libx264")
	s, id := completedAnalysisFixture(t)
	root, err := s.analysisRoot(id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	inputs, warnings := prepareSubtitleOCRInputs(t.Context(), root, id, ffmpeg, source, []analysisShot{{Index: 1, End: 2.6}}, t.TempDir())
	if len(inputs) != 4 || len(warnings) != 0 {
		t.Fatalf("inputs/warnings = %d/%v", len(inputs), warnings)
	}
	for _, input := range inputs {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			t.Fatal(err)
		}
		width, height := analysisImageSize(data)
		if width != 720 || height != 1000 {
			t.Fatalf("OCR frame was compressed to %dx%d", width, height)
		}
		if !strings.Contains(input.RelativePath, "/ocr/") {
			t.Fatalf("missing independently verifiable OCR frame: %s", input.RelativePath)
		}
	}
}

func TestExtractAnalysisAudioThroughPipe(t *testing.T) {
	ffmpeg := os.Getenv("WX_CHANNEL_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("set WX_CHANNEL_TEST_FFMPEG for decoder validation")
	}
	fixtureFFmpeg := firstNonEmpty(os.Getenv("WX_CHANNEL_TEST_FIXTURE_FFMPEG"), ffmpeg)
	source := frameDecoderFFmpegFixture(t, fixtureFFmpeg, "-f", "lavfi", "-i", "sine=frequency=440:duration=0.5", "-c:a", "aac")
	output := filepath.Join(t.TempDir(), "audio.wav")
	if err := extractAnalysisAudio(t.Context(), ffmpeg, source, output); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil || len(raw) < 16000 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		t.Fatalf("invalid decoded PCM: %d bytes / %v", len(raw), err)
	}
	if err := extractAnalysisAudio(t.Context(), ffmpeg, source, output); err == nil {
		t.Fatal("overwrote existing audio")
	}
}

func TestAnalysisAudioWriterEnforcesDurationBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	w := analysisAudioWriter{bytes: 60 << 20, cancel: cancel}
	if _, err := w.Write([]byte("overflow")); err == nil || ctx.Err() == nil {
		t.Fatal("audio output was not bounded")
	}
}

func TestDecodeLocalJSONIgnoresPythonLogs(t *testing.T) {
	var payload struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	if err := decodeLocalJSON([]byte("warning from backend\n{\"status\":\"completed\",\"value\":\"字幕\"}\n"), &payload); err != nil {
		t.Fatalf("decodeLocalJSON() error = %v", err)
	}
	if payload.Status != "completed" || payload.Value != "字幕" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRunLocalPythonJSONPreservesChineseUnderWindowsEncoding(t *testing.T) {
	python, err := findLocalPython()
	if err != nil {
		t.Skip("Python is not installed")
	}
	t.Setenv("PYTHONIOENCODING", "cp1252")
	raw, err := runLocalPythonJSON(t.Context(), python, `import json; print(json.dumps({"text":"相互陪伴，去呗"},ensure_ascii=False))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := decodeLocalJSON(raw, &result); err != nil || result.Text != "相互陪伴，去呗" {
		t.Fatalf("lost Chinese output: %q / %v", raw, err)
	}
}

func TestAttachLocalEnrichmentRemovesStaleEvidenceAndRefreshesAudio(t *testing.T) {
	m := &videoAnalysisManifest{
		Audio:           map[string]any{"speech_transcribed": false},
		LocalEnrichment: localEnrichmentReport{Status: "completed", ASR: localASRReport{Status: "completed", Segments: []localTranscriptSegment{{Start: 0, End: 1, Text: "新的语音"}}}},
		Shots:           []analysisShot{{Index: 1, Start: 0, End: 2, Evidence: map[string]any{"local_ocr": "stale", "local_enrichment_status": "disabled"}}},
	}
	attachLocalEnrichmentToShots(m)
	if m.Audio["speech_transcribed"] != true || m.Shots[0].Evidence["local_ocr"] != nil {
		t.Fatalf("stale report evidence: %#v / %#v", m.Audio, m.Shots[0].Evidence)
	}
	m.LocalEnrichment = localEnrichmentReport{Status: "unavailable"}
	attachLocalEnrichmentToShots(m)
	if m.Audio["speech_transcribed"] != false || m.Shots[0].Evidence["local_transcript"] != nil {
		t.Fatal("previous ASR survived a failed refresh")
	}
}

func TestFinalizeLocalEnrichmentClearsRecoveredErrors(t *testing.T) {
	report := finalizeLocalEnrichment(localEnrichmentReport{
		Enabled: true, MissingReasons: []string{"local_enrichment_disabled"}, Warnings: []string{"local_enrichment_disabled"},
		ASR: localASRReport{Status: "completed", MissingReason: "asr_model_unavailable", Error: "old_failure"},
		OCR: localOCRReport{Status: "completed", TextCount: 1, MissingReason: "ocr_backend_unavailable", Error: "old_failure"},
	})
	if report.Status != "completed" || len(report.MissingReasons) != 0 || len(report.Warnings) != 0 || report.ASR.MissingReason != "" || report.ASR.Error != "" || report.OCR.MissingReason != "" || report.OCR.Error != "" {
		t.Fatalf("recovered report retains failures: %#v", report)
	}
}

func TestRunLocalPythonJSONUsesBoundedJSONContract(t *testing.T) {
	python, err := findLocalPython()
	if err != nil {
		t.Skip("Python is not installed")
	}
	raw, err := runLocalPythonJSON(t.Context(), python, `print("backend log")
print('{"status":"completed","value":"ok"}')`)
	if err != nil {
		t.Fatalf("runLocalPythonJSON() error = %v", err)
	}
	var payload struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	if err := decodeLocalJSON(raw, &payload); err != nil || payload.Status != "completed" || payload.Value != "ok" {
		t.Fatalf("raw/payload = %q/%#v, error = %v", raw, payload, err)
	}
}

func TestFinalizeLocalEnrichmentReportsPartialAndMissingReasons(t *testing.T) {
	report := finalizeLocalEnrichment(localEnrichmentReport{
		Enabled: true,
		ASR:     localASRReport{Status: "completed", SegmentCount: 2},
		OCR:     localOCRReport{Status: "unavailable", MissingReason: "ocr_backend_unavailable"},
	})
	if report.Status != "partial" {
		t.Fatalf("status = %q, want partial", report.Status)
	}
	if !containsLocalString(report.MissingReasons, "ocr_backend_unavailable") || !containsLocalString(report.Warnings, "ocr_backend_unavailable") {
		t.Fatalf("missing reasons/warnings = %#v/%#v", report.MissingReasons, report.Warnings)
	}
}

func TestLocalEnrichmentCanBeDisabledWithoutFindingPython(t *testing.T) {
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_ENRICHMENT", "false")
	settings, err := loadLocalEnrichmentSettings()
	if err != nil || settings.Enabled {
		t.Fatalf("settings = %#v, error = %v", settings, err)
	}
	report := (&Server{}).runLocalEnrichment(t.Context(), "missing-ffmpeg", "missing.mp4", t.TempDir(), map[string]any{}, nil)
	if report.Status != "disabled" || report.Enabled || report.ASR.MissingReason != "local_enrichment_disabled" {
		t.Fatalf("disabled report = %#v", report)
	}
}

func TestLocalEnrichmentReportsPythonUnavailable(t *testing.T) {
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_ENRICHMENT", "true")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_PYTHON", filepath.Join(t.TempDir(), "missing-python.exe"))
	report := (&Server{}).runLocalEnrichment(t.Context(), "missing-ffmpeg", "missing.mp4", t.TempDir(), map[string]any{"status": "available", "stream_present": true}, nil)
	if report.Status != "unavailable" || report.ASR.MissingReason != "python_unavailable" || report.OCR.MissingReason != "python_unavailable" {
		t.Fatalf("unavailable report = %#v", report)
	}
}

func TestLocalEnrichmentHandlesNoAudioWithoutLoadingModels(t *testing.T) {
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_ENRICHMENT", "true")
	settings, err := loadLocalEnrichmentSettings()
	if err != nil || settings.Python.Executable == "" {
		t.Skip("Python is not installed")
	}
	report := (&Server{}).runLocalEnrichment(t.Context(), "missing-ffmpeg", "missing.mp4", t.TempDir(), map[string]any{"status": "no_audio", "stream_present": false}, nil)
	if report.ASR.Status != "no_audio" || report.ASR.MissingReason != "audio_not_present" || report.OCR.MissingReason != "no_keyframes" {
		t.Fatalf("no-audio report = %#v", report)
	}
}

func TestAttachLocalEnrichmentToShotsUsesShotTimeAndIndex(t *testing.T) {
	manifest := &videoAnalysisManifest{
		LocalEnrichment: localEnrichmentReport{
			Status: "completed",
			ASR:    localASRReport{Segments: []localTranscriptSegment{{Start: 0.1, End: 0.4, Text: "第一段"}, {Start: 2, End: 3, Text: "第二段"}}},
			OCR:    localOCRReport{Frames: []localOCRFrame{{ShotIndex: 1, Timestamp: 0.2, RelativePath: "shots/one.jpg", Status: "completed", Texts: []localOCRText{{Text: "标题"}}}, {ShotIndex: 2, Timestamp: 2.2, RelativePath: "shots/two.jpg", Status: "completed"}}},
		},
		Shots: []analysisShot{{Index: 1, Start: 0, End: 1, Evidence: map[string]any{}}, {Index: 2, Start: 1, End: 4, Evidence: map[string]any{}}},
	}
	attachLocalEnrichmentToShots(manifest)
	first, ok := manifest.Shots[0].Evidence["local_transcript"].([]localTranscriptSegment)
	if !ok || len(first) != 1 || first[0].Text != "第一段" {
		t.Fatalf("first transcript = %#v", manifest.Shots[0].Evidence["local_transcript"])
	}
	second, ok := manifest.Shots[1].Evidence["local_transcript"].([]localTranscriptSegment)
	if !ok || len(second) != 1 || second[0].Text != "第二段" {
		t.Fatalf("second transcript = %#v", manifest.Shots[1].Evidence["local_transcript"])
	}
	if _, ok := manifest.Shots[0].Evidence["local_ocr"]; !ok || manifest.Shots[0].Evidence["local_enrichment_status"] != "completed" {
		t.Fatalf("first evidence = %#v", manifest.Shots[0].Evidence)
	}
}

func TestPrepareLocalOCRInputsCopiesValidatedFrames(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	frame := frameDecoderTestJPEG(t, 160, 120)
	root, err := s.analysisRoot(id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := writeAnalysisFile(root, "shots/shot_001_1.jpg", frame); err != nil {
		t.Fatal(err)
	}
	shots := []analysisShot{{Index: 1, Keyframes: []analysisKeyframe{{Timestamp: 0.5, RelativePath: analysisReference(id, "shots/shot_001_1.jpg")}}}}
	inputs, warnings := prepareLocalOCRInputs(root, id, shots, t.TempDir())
	if len(warnings) != 0 || len(inputs) != 1 {
		t.Fatalf("inputs/warnings = %#v/%#v", inputs, warnings)
	}
	if _, err := os.Stat(inputs[0].Path); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(inputs[0].Path), "shot_001_1.jpg") {
		t.Fatalf("unexpected OCR path: %s", inputs[0].Path)
	}
}

func containsLocalString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// Opt-in because these are real local models, not CI fixtures. The supplied
// video must contain speech and visible text near its beginning.
func TestLocalEnrichmentRealBackends(t *testing.T) {
	input, ffmpeg := os.Getenv("WX_CHANNEL_TEST_ENRICHMENT_VIDEO"), os.Getenv("WX_CHANNEL_TEST_FFMPEG")
	if input == "" || ffmpeg == "" {
		t.Skip("set WX_CHANNEL_TEST_ENRICHMENT_VIDEO and WX_CHANNEL_TEST_FFMPEG for actual model validation")
	}
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_ENRICHMENT", "true")
	data, err := decodeLocalOCRFrame(t.Context(), ffmpeg, input, 0.375)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	framePath := filepath.Join(dir, "subtitle.jpg")
	if err := os.WriteFile(framePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	report := (&Server{}).runLocalEnrichment(t.Context(), ffmpeg, input, dir, probeAnalysisAudio(t.Context(), ffmpeg, input), []localOCRInput{{ShotIndex: 1, Timestamp: 0.375, RelativePath: "shots/subtitle.jpg", Path: framePath}})
	if report.Status != "completed" || report.ASR.SegmentCount == 0 || report.OCR.TextCount == 0 {
		t.Fatalf("real backends incomplete: ASR=%s (%d), OCR=%s (%d), missing=%v", report.ASR.Status, report.ASR.SegmentCount, report.OCR.Status, report.OCR.TextCount, report.MissingReasons)
	}
	t.Logf("actual backends: ASR %d segments, OCR %d texts; no external model API", report.ASR.SegmentCount, report.OCR.TextCount)
}
