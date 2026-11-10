package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAnalysisReportFiveSectionsAndStructuredReview(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	session := analysisTestSession(t, s)
	args := map[string]any{
		"analysis_id": id, "shot_index": 1, "description": "女子看手机，男子起身",
		"shot_size": "中远景", "camera_angle": "床尾平视", "camera_move": "固定机位",
		"composition": "两人居中", "action": "男子起身", "lighting": "室内柔光", "color": "暖白",
		"overlay_text": "当我想去喝酒", "shooting_plan": "床尾固定手机，留出出画空间", "video_prompt": "卧室喜剧",
		"transcript": []map[string]any{{"start_seconds": 0.1, "end_seconds": 0.8, "speaker": "女子", "text": "去呗", "kind": "subtitle", "source": "codex_visible_text"}},
		"storyboard": []map[string]any{
			{"start_seconds": 0.0, "end_seconds": 0.5, "picture": "两人躺卧", "action": "看手机", "camera": "平视固定", "dialogue": "去呗", "sound": "原声待核对", "production_notes": "先铺垫"},
			{"start_seconds": 0.5, "end_seconds": 1.0, "picture": "男子出画", "action": "迅速起身", "camera": "同一机位", "production_notes": "同一镜头内动作反转"},
		},
	}
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "save_video_analysis_review", Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("save review: %v / %+v", err, result)
	}
	root := filepath.Join(s.deps.DownloadsDir, analysisArtifactDir, id)
	htmlData, err := os.ReadFile(filepath.Join(root, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(htmlData)
	for _, term := range []string{"一、视频详情", "二、视频画面截图及分析", "三、视频语音或字幕还原", "四、画面拆解", "五、视频还原及分镜脚本", "中远景", "床尾平视", "去呗", "同一镜头内动作反转"} {
		if !strings.Contains(page, term) {
			t.Errorf("HTML missing %q", term)
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		SpeechSubtitles struct {
			Timeline []analysisTranscriptLine `json:"timeline"`
		} `json:"speech_subtitles"`
		Storyboard []analysisStoryboardRow `json:"storyboard"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Storyboard) != 2 || len(report.SpeechSubtitles.Timeline) != 1 {
		t.Fatalf("structured report incomplete: %s", raw)
	}
	if report.SpeechSubtitles.Timeline[0].Source != "codex_visible_text" {
		t.Fatal("visible subtitle misrepresented as ASR")
	}
}

func TestSpeechReportPrioritizesReviewedSubtitlesAndKeepsRawEvidence(t *testing.T) {
	m := videoAnalysisManifest{
		AnalysisID:      "8d1ab3f0-87bd-4b76-820d-e2f6ea62e606",
		LocalEnrichment: localEnrichmentReport{ASR: localASRReport{Status: "completed", Segments: []localTranscriptSegment{{Start: 0, End: 2, Text: "香菇陪伴"}}}, OCR: localOCRReport{Status: "completed", FrameCount: 1, Frames: []localOCRFrame{{Timestamp: 1, RelativePath: ".wx_channel_analysis/8d1ab3f0-87bd-4b76-820d-e2f6ea62e606/ocr/frame_001.jpg", Texts: []localOCRText{{Text: "相互陪伴", Confidence: 0.99}}}}}},
		Shots:           []analysisShot{{Index: 1, Start: 0, End: 2, Review: map[string]any{"transcript": []analysisTranscriptLine{{Start: 1, End: 1, Text: "相互陪伴", Kind: "subtitle", Source: "codex_visible_text"}}}}},
	}
	var b strings.Builder
	writeSpeechSubtitleSection(&b, &m)
	page := b.String()
	reviewed := strings.Index(page, "<h3>字幕复核结果</h3>")
	raw := strings.Index(page, "<summary>语音识别原稿")
	if reviewed < 0 || raw < 0 || reviewed > raw {
		t.Fatal("machine error precedes reviewed subtitle")
	}
	for _, term := range []string{"字幕复核结果", "<details", "语音识别原稿", "OCR 识别原稿", "ocr/frame_001.jpg"} {
		if !strings.Contains(page, term) {
			t.Errorf("missing %s", term)
		}
	}
}

func TestAnalysisImagesCanReachEveryFrame(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	job, _ := s.loadAnalysisJob(id)
	root, _ := s.analysisRoot(id, false)
	defer root.Close()
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("shots/shot_001_%d.jpg", i+1)
		if err := writeAnalysisFile(root, name, frameDecoderTestJPEG(t, 160, 120)); err != nil {
			t.Fatal(err)
		}
		job.Manifest.Shots[0].Keyframes = append(job.Manifest.Shots[0].Keyframes, analysisKeyframe{Timestamp: float64(i+1) / 10, RelativePath: analysisReference(id, name), Width: 160, Height: 120})
	}
	if err := s.saveAnalysisManifestLocked(job.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := s.writeAnalysisReports(&job.Manifest); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for offset := 0; ; {
		result, out, err := s.getVideoAnalysisShots(t.Context(), nil, videoAnalysisShotsInput{AnalysisID: id, PageSize: 1, IncludeImages: true, ImageOffset: offset})
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, c := range result.Content {
			if _, ok := c.(*mcp.ImageContent); ok {
				count++
			}
		}
		if count < 1 || count > 3 {
			t.Fatalf("image count = %d", count)
		}
		seen += count
		if size, err := serializedCallToolResultSize(result, out); err != nil || size > maxToolResponseBytes {
			t.Fatalf("unbounded result: %d / %v", size, err)
		}
		data := out["data"].(map[string]any)
		if data["images_has_more"] != true {
			break
		}
		next := data["next_image_offset"].(int)
		if next <= offset {
			t.Fatal("image cursor did not advance")
		}
		offset = next
	}
	if seen != 7 {
		t.Fatalf("only %d / 7 frames reachable", seen)
	}
	page, _ := os.ReadFile(filepath.Join(s.deps.DownloadsDir, analysisArtifactDir, id, "report.html"))
	if strings.Count(string(page), "<img ") != 7 {
		t.Fatal("gallery omitted frames")
	}
}

func TestAnalysisUnavailableTranscriptionIsNotEmptySuccess(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	job, _ := s.loadAnalysisJob(id)
	job.Manifest.LocalEnrichment = localEnrichmentReport{Status: "unavailable", ASR: localASRReport{Status: "unavailable", MissingReason: "asr_model_unavailable"}, OCR: localOCRReport{Status: "partial", MissingReason: "ocr_frame_failed", Frames: []localOCRFrame{{ShotIndex: 1, Timestamp: 0.2, Status: "completed", Texts: []localOCRText{{Text: "部分字幕", Confidence: 0.8}}}}}}
	if err := s.writeAnalysisReports(&job.Manifest); err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(filepath.Join(s.deps.DownloadsDir, analysisArtifactDir, id, "report.html"))
	for _, term := range []string{"asr_model_unavailable", "ocr_frame_failed", "部分字幕"} {
		if !strings.Contains(string(page), term) {
			t.Errorf("missing failure/partial evidence: %s", term)
		}
	}
}

func TestAnalysisDetailsRedactPrivatePathsAndSignedURLs(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	job, _ := s.loadAnalysisJob(id)
	job.Manifest.VideoDetails = videoAnalysisDetails{Title: `failed C:\Users\private\clip.mp4`, Author: "https://finder.video.qq.com/clip?token=private-token", VideoID: "video-safe", RelativePath: "sample.mp4", FileSizeBytes: 100}
	if err := s.writeAnalysisReports(&job.Manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "report.html"} {
		raw, _ := os.ReadFile(filepath.Join(s.deps.DownloadsDir, analysisArtifactDir, id, name))
		for _, secret := range []string{"private-token", `Users`, `C:\\`} {
			if strings.Contains(string(raw), secret) {
				t.Errorf("%s contains private detail %q", name, secret)
			}
		}
		if !strings.Contains(string(raw), "video-safe") {
			t.Errorf("%s lost safe identity", name)
		}
	}
}

func TestAnalysisReviewRejectsOutOfShotTimeline(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	_, _, err := s.saveVideoAnalysisReview(t.Context(), videoAnalysisReviewInput{AnalysisID: id, ShotIndex: 1, Description: "test", Transcript: []analysisTranscriptLine{{Start: 0.5, End: 2, Text: "outside", Kind: "speech", Source: "codex_audio_review"}}})
	if err == nil || !strings.HasPrefix(err.Error(), "invalid_argument:") {
		t.Fatalf("accepted transcript beyond shot: %v", err)
	}
}
