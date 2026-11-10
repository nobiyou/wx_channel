package mcpserver

import (
	"regexp"
	"testing"
)

func TestParseAnalysisSceneTimesBuildsBoundedSegments(t *testing.T) {
	segments, err := parseAnalysisSceneTimes("[Parsed_showinfo_0] pts_time:0.800000\n[Parsed_showinfo_0] pts_time:1.500000\n", 2.5)
	if err != nil {
		t.Fatalf("parseAnalysisSceneTimes() error = %v", err)
	}
	if len(segments) != 3 {
		t.Fatalf("segment count = %d, want 3", len(segments))
	}
	want := [][2]float64{{0, 0.8}, {0.8, 1.5}, {1.5, 2.5}}
	for i, segment := range segments {
		if segment.Start != want[i][0] || segment.End != want[i][1] {
			t.Fatalf("segment %d = %#v, want %.3f–%.3f", i, segment, want[i][0], want[i][1])
		}
	}
}

func TestParseAnalysisSceneTimesIgnoresTooCloseOrOutOfRangePoints(t *testing.T) {
	segments, err := parseAnalysisSceneTimes("pts_time:0.100000\npts_time:0.400000\npts_time:2.490000\npts_time:3.000000\n", 2.5)
	if err != nil {
		t.Fatalf("parseAnalysisSceneTimes() error = %v", err)
	}
	if len(segments) != 2 || segments[0].End != 0.4 || segments[1].Start != 0.4 {
		t.Fatalf("segments = %#v, want boundary at 0.4", segments)
	}
}

func TestAnalysisShotPromptRequiresEvidenceAndShootingPlan(t *testing.T) {
	prompt := analysisShotPrompt(3, analysisSegment{Start: 1.25, End: 2.75})
	for _, term := range []string{"实拍", "AI 视频", "景别", "灯光", "证据", "复刻提示词"} {
		if !containsText(prompt, term) {
			t.Fatalf("prompt does not contain %q: %s", term, prompt)
		}
	}
}

func containsText(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestVideoAnalysisModelStatusDoesNotPretendToCallModel(t *testing.T) {
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_BASE_URL", "")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_API_KEY", "")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_MODEL", "")
	status, model := videoAnalysisModelStatus("hybrid")
	if status != "not_configured" || model != "" {
		t.Fatalf("model status = %q/%q, want not_configured/empty", status, model)
	}
	status, model = videoAnalysisModelStatus("local")
	if status != "not_requested" || model != "" {
		t.Fatalf("local model status = %q/%q, want not_requested/empty", status, model)
	}
}

func TestFirstAudioNumberParsesFFmpegDiagnostic(t *testing.T) {
	value := firstAudioNumber(regexp.MustCompile(`mean_volume:\s*(-?[0-9.]+)\s*dB`), "[Parsed_volumedetect] mean_volume: -18.4 dB")
	if value == nil || *value != -18.4 {
		t.Fatalf("parsed value = %v, want -18.4", value)
	}
}
