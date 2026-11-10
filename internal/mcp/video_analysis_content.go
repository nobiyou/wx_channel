package mcpserver

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type analysisTranscriptLine struct {
	Start   float64 `json:"start_seconds"`
	End     float64 `json:"end_seconds"`
	Speaker string  `json:"speaker,omitempty"`
	Text    string  `json:"text"`
	Kind    string  `json:"kind" jsonschema:"speech subtitle or onscreen_text"`
	Source  string  `json:"source" jsonschema:"codex_visible_text codex_audio_review or codex_inference; never claim ASR without ASR evidence"`
}

type analysisFrameObservation struct {
	Timestamp   float64 `json:"timestamp_seconds"`
	Description string  `json:"description"`
}

type analysisStoryboardRow struct {
	ShotIndex       int     `json:"shot_index,omitempty"`
	Start           float64 `json:"start_seconds"`
	End             float64 `json:"end_seconds"`
	Picture         string  `json:"picture"`
	Dialogue        string  `json:"dialogue,omitempty"`
	Action          string  `json:"action,omitempty"`
	Camera          string  `json:"camera,omitempty"`
	Sound           string  `json:"sound,omitempty"`
	ProductionNotes string  `json:"production_notes,omitempty"`
}

type analysisSpeechSubtitles struct {
	ASR      localASRReport           `json:"asr"`
	OCR      localOCRReport           `json:"ocr"`
	Timeline []analysisTranscriptLine `json:"timeline"`
}

type analysisBreakdownRow struct {
	ShotIndex int            `json:"shot_index"`
	Start     float64        `json:"start_seconds"`
	End       float64        `json:"end_seconds"`
	Fields    map[string]any `json:"fields"`
}

type videoAnalysisReportData struct {
	videoAnalysisManifest
	SpeechSubtitles analysisSpeechSubtitles `json:"speech_subtitles"`
	ShotBreakdown   []analysisBreakdownRow  `json:"shot_breakdown"`
	Storyboard      []analysisStoryboardRow `json:"storyboard"`
}

func validateAnalysisReviewContent(input videoAnalysisReviewInput, shot analysisShot) error {
	invalid := func() error {
		return errors.New("invalid_argument: review timeline must contain bounded text and ordered timestamps inside this shot")
	}
	if len(input.Transcript) > 40 || len(input.Storyboard) > 40 || len(input.FrameAnalyses) > len(shot.Keyframes) {
		return invalid()
	}
	validTime := func(start, end float64) bool {
		return !math.IsNaN(start) && !math.IsNaN(end) && !math.IsInf(start, 0) && !math.IsInf(end, 0) && start >= shot.Start && end >= start && end <= shot.End
	}
	validText := func(values ...string) bool {
		for _, value := range values {
			if utf8.RuneCountInString(value) > maxModelValueRunes {
				return false
			}
		}
		return true
	}
	previous := shot.Start
	for _, line := range input.Transcript {
		if !validTime(line.Start, line.End) || line.Start < previous || strings.TrimSpace(line.Text) == "" || !validText(line.Text, line.Speaker) {
			return invalid()
		}
		if line.Kind != "speech" && line.Kind != "subtitle" && line.Kind != "onscreen_text" {
			return errors.New("invalid_argument: transcript kind must be speech, subtitle, or onscreen_text")
		}
		if line.Source != "codex_visible_text" && line.Source != "codex_audio_review" && line.Source != "codex_inference" {
			return errors.New("invalid_argument: transcript source must identify visible text, audio review, or inference")
		}
		if line.Kind == "speech" && line.Source == "codex_visible_text" {
			return errors.New("invalid_argument: visible subtitles must not be labeled as heard speech")
		}
		previous = line.Start
	}
	previous = shot.Start
	for _, row := range input.Storyboard {
		if !validTime(row.Start, row.End) || row.End <= row.Start || row.Start < previous || strings.TrimSpace(row.Picture) == "" || !validText(row.Picture, row.Dialogue, row.Action, row.Camera, row.Sound, row.ProductionNotes) {
			return invalid()
		}
		previous = row.End
	}
	seen := map[float64]bool{}
	for _, frame := range input.FrameAnalyses {
		matched := false
		for _, source := range shot.Keyframes {
			if math.Abs(source.Timestamp-frame.Timestamp) < 0.001 {
				matched = true
				break
			}
		}
		if !matched || seen[frame.Timestamp] || strings.TrimSpace(frame.Description) == "" || !validText(frame.Description) {
			return errors.New("invalid_argument: frame_analyses must reference unique keyframe timestamps")
		}
		seen[frame.Timestamp] = true
	}
	return nil
}

func analysisFieldSlice[T any](value any) []T {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out []T
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func buildVideoAnalysisReport(manifest videoAnalysisManifest) videoAnalysisReportData {
	report := videoAnalysisReportData{videoAnalysisManifest: manifest, SpeechSubtitles: analysisSpeechSubtitles{ASR: manifest.LocalEnrichment.ASR, OCR: manifest.LocalEnrichment.OCR, Timeline: []analysisTranscriptLine{}}, ShotBreakdown: []analysisBreakdownRow{}, Storyboard: []analysisStoryboardRow{}}
	for _, line := range manifest.LocalEnrichment.ASR.Segments {
		report.SpeechSubtitles.Timeline = append(report.SpeechSubtitles.Timeline, analysisTranscriptLine{Start: line.Start, End: line.End, Text: line.Text, Kind: "speech", Source: "local_asr"})
	}
	for _, frame := range manifest.LocalEnrichment.OCR.Frames {
		for _, text := range frame.Texts {
			report.SpeechSubtitles.Timeline = append(report.SpeechSubtitles.Timeline, analysisTranscriptLine{Start: frame.Timestamp, End: frame.Timestamp, Text: text.Text, Kind: "onscreen_text", Source: "local_ocr"})
		}
	}
	for _, shot := range manifest.Shots {
		values := preferredShotValues(shot)
		lines := analysisFieldSlice[analysisTranscriptLine](values["transcript"])
		report.SpeechSubtitles.Timeline = append(report.SpeechSubtitles.Timeline, lines...)
		fields := map[string]any{}
		for _, key := range []string{"description", "shot_size", "subject", "action", "camera_angle", "camera_move", "composition", "lighting", "color", "overlay_text", "evidence", "confidence"} {
			if value, ok := values[key]; ok {
				fields[key] = value
			}
		}
		report.ShotBreakdown = append(report.ShotBreakdown, analysisBreakdownRow{ShotIndex: shot.Index, Start: shot.Start, End: shot.End, Fields: fields})
		rows := analysisFieldSlice[analysisStoryboardRow](values["storyboard"])
		if len(rows) == 0 {
			rows = []analysisStoryboardRow{{Start: shot.Start, End: shot.End, Picture: firstNonEmpty(reportValueText(values["description"]), "待视觉复核"), Dialogue: reportValueText(values["overlay_text"]), Action: reportValueText(values["action"]), Camera: strings.TrimSpace(reportValueText(values["camera_angle"]) + " " + reportValueText(values["shot_size"]) + " " + reportValueText(values["camera_move"])), Sound: reportValueText(values["audio_description"]), ProductionNotes: firstNonEmpty(reportValueText(values["shooting_plan"]), reportValueText(values["video_prompt"]), "待生成拍摄方案")}}
		}
		for _, row := range rows {
			row.ShotIndex = shot.Index
			if row.Dialogue == "" {
				var texts []string
				for _, line := range append(append([]analysisTranscriptLine{}, lines...), report.SpeechSubtitles.Timeline...) {
					if line.Start < row.End && line.End >= row.Start {
						value := line.Text + "（" + analysisTranscriptSource(line.Source) + "）"
						if !containsReportText(texts, value) {
							texts = append(texts, value)
						}
					}
				}
				row.Dialogue = strings.Join(texts, "\n")
			}
			report.Storyboard = append(report.Storyboard, row)
		}
	}
	sort.SliceStable(report.SpeechSubtitles.Timeline, func(i, j int) bool {
		return report.SpeechSubtitles.Timeline[i].Start < report.SpeechSubtitles.Timeline[j].Start
	})
	return report
}

func containsReportText(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func analysisTranscriptSource(source string) string {
	if label, ok := map[string]string{"local_asr": "本地语音识别，待校对", "local_ocr": "本地画面文字识别，采样时刻", "codex_visible_text": "Codex 画面字幕复核", "codex_audio_review": "Codex 听音复核", "codex_inference": "推测，待确认"}[source]; ok {
		return label
	}
	return "来源未确认"
}

var analysisURLText = regexp.MustCompile(`https?://[^\s<>"']+`)

func safeAnalysisDetail(value string) string {
	if localAbsolutePath.MatchString(value) {
		return "[redacted_local_path]"
	}
	value = analysisURLText.ReplaceAllStringFunc(value, func(raw string) string { clean, _ := sanitizeURL(raw); return clean })
	warnings := []string{}
	return boundedText(value, &warnings)
}
