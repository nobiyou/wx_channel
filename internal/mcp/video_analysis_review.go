package mcpserver

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type videoAnalysisReviewInput struct {
	AnalysisID       string                     `json:"analysis_id" jsonschema:"analysis id"`
	ShotIndex        int                        `json:"shot_index" jsonschema:"one-based shot index"`
	Description      string                     `json:"description" jsonschema:"Codex visual review of the shot"`
	Evidence         []string                   `json:"evidence,omitempty" jsonschema:"visible evidence with time or frame reference"`
	Confidence       float64                    `json:"confidence,omitempty" jsonschema:"confidence from 0 to 1"`
	ShootingPlan     string                     `json:"shooting_plan,omitempty" jsonschema:"practical camera and lighting plan"`
	VideoPrompt      string                     `json:"video_prompt,omitempty" jsonschema:"remake video prompt"`
	ShotSize         string                     `json:"shot_size,omitempty" jsonschema:"observed shot size"`
	CameraAngle      string                     `json:"camera_angle,omitempty" jsonschema:"observed camera position and angle"`
	CameraMove       string                     `json:"camera_move,omitempty" jsonschema:"observed camera movement; do not infer continuous motion from one still"`
	Composition      string                     `json:"composition,omitempty" jsonschema:"observed framing and composition"`
	Subject          string                     `json:"subject,omitempty" jsonschema:"visible subjects and scene"`
	Action           string                     `json:"action,omitempty" jsonschema:"visible actions in time order"`
	Lighting         string                     `json:"lighting,omitempty" jsonschema:"observed lighting; equipment is an inference"`
	Color            string                     `json:"color,omitempty" jsonschema:"observed color and grading"`
	OverlayText      string                     `json:"overlay_text,omitempty" jsonschema:"visible titles subtitles and stickers"`
	AudioDescription string                     `json:"audio_description,omitempty" jsonschema:"only describe audio actually heard; otherwise state unavailable"`
	Transcript       []analysisTranscriptLine   `json:"transcript,omitempty" jsonschema:"timestamped speech subtitles or onscreen text; distinguish visible text from audio listening or inference"`
	Storyboard       []analysisStoryboardRow    `json:"storyboard,omitempty" jsonschema:"timed reconstruction beats inside this shot; a continuous shot may contain multiple narrative beats without inventing cuts"`
	FrameAnalyses    []analysisFrameObservation `json:"frame_analyses,omitempty" jsonschema:"observations tied to exact returned keyframe timestamps"`
}

func registerAnalysisReviewTool(s *Server) {
	tool := &mcp.Tool{Name: "save_video_analysis_review", Description: "Save Codex's shot review into a five-section JSON/HTML report. Include visual fields, frame_analyses, timestamped transcript with evidence source, and storyboard beats (multiple beats can belong to one continuous shot). Only claim speech was heard when audio was actually reviewed. Re-saving replaces this shot's review. It does not publish or generate video."}
	tool.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)}
	mcp.AddTool(s.mcpServer, tool, func(ctx context.Context, _ *mcp.CallToolRequest, input videoAnalysisReviewInput) (*mcp.CallToolResult, map[string]any, error) {
		result, out, err := s.saveVideoAnalysisReview(ctx, input)
		if err != nil {
			return analysisToolFailure(err)
		}
		return result, out, nil
	})
}

func (s *Server) saveVideoAnalysisReview(ctx context.Context, input videoAnalysisReviewInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if input.ShotIndex < 1 {
		return nil, nil, errors.New("invalid_argument: shot_index must be positive")
	}
	if strings.TrimSpace(input.Description) == "" {
		return nil, nil, errors.New("invalid_argument: description is required")
	}
	for _, value := range []string{input.Description, input.ShootingPlan, input.VideoPrompt, input.ShotSize, input.CameraAngle, input.CameraMove, input.Composition, input.Subject, input.Action, input.Lighting, input.Color, input.OverlayText, input.AudioDescription} {
		if utf8.RuneCountInString(value) > maxModelValueRunes {
			return nil, nil, errors.New("invalid_argument: each review text must be at most 4000 characters")
		}
	}
	if math.IsNaN(input.Confidence) || math.IsInf(input.Confidence, 0) || input.Confidence < 0 || input.Confidence > 1 {
		return nil, nil, errors.New("invalid_argument: confidence must be between 0 and 1")
	}
	s.analysisMu.Lock()
	defer s.analysisMu.Unlock()
	job, err := s.loadAnalysisJobLocked(input.AnalysisID)
	if err != nil {
		return nil, nil, err
	}
	if job.Manifest.Status != "completed" {
		return nil, nil, errors.New("analysis_not_ready: only a completed analysis can accept review")
	}
	if input.ShotIndex > len(job.Manifest.Shots) {
		return nil, nil, errors.New("invalid_argument: shot_index is outside the analysis")
	}
	shot := &job.Manifest.Shots[input.ShotIndex-1]
	if err := validateAnalysisReviewContent(input, *shot); err != nil {
		return nil, nil, err
	}
	shot.Review = map[string]any{"source": "codex_session", "description": strings.TrimSpace(input.Description), "evidence": boundReviewStrings(input.Evidence), "confidence": input.Confidence, "shooting_plan": strings.TrimSpace(input.ShootingPlan), "video_prompt": strings.TrimSpace(input.VideoPrompt), "reviewed_at": time.Now().UTC().Format(time.RFC3339)}
	for key, value := range map[string]string{"shot_size": input.ShotSize, "camera_angle": input.CameraAngle, "camera_move": input.CameraMove, "composition": input.Composition, "subject": input.Subject, "action": input.Action, "lighting": input.Lighting, "color": input.Color, "overlay_text": input.OverlayText, "audio_description": input.AudioDescription} {
		if value = strings.TrimSpace(value); value != "" {
			shot.Review[key] = value
		}
	}
	if len(input.Transcript) > 0 {
		shot.Review["transcript"] = input.Transcript
	}
	if len(input.Storyboard) > 0 {
		shot.Review["storyboard"] = input.Storyboard
	}
	if len(input.FrameAnalyses) > 0 {
		shot.Review["frame_analyses"] = input.FrameAnalyses
	}
	if shot.Replication == nil {
		shot.Replication = map[string]any{}
	}
	shot.Replication["status"] = "reviewed"
	shot.Replication["shooting_plan"] = strings.TrimSpace(input.ShootingPlan)
	shot.Replication["video_prompt"] = strings.TrimSpace(input.VideoPrompt)
	job.Manifest.ReviewCount = 0
	for _, item := range job.Manifest.Shots {
		if item.Review != nil {
			job.Manifest.ReviewCount++
		}
	}
	job.Manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.writeAnalysisReports(&job.Manifest); err != nil {
		return nil, nil, err
	}
	if err := s.saveAnalysisManifestLocked(job.Manifest); err != nil {
		return nil, nil, err
	}
	return nil, envelope("local_video_analysis", "stored", map[string]any{"analysis_id": job.ID, "shot_index": input.ShotIndex, "review_count": job.Manifest.ReviewCount, "review_status": analysisReviewStatus(job.Manifest), "status": "saved", "next": analysisJobSummary(job.Manifest)}, nil), nil
}

func boundReviewStrings(values []string) []string {
	out := make([]string, 0, min(len(values), 20))
	for _, value := range values {
		if len(out) >= 20 {
			break
		}
		value = strings.TrimSpace(value)
		if value != "" {
			if len([]rune(value)) > 1000 {
				value = string([]rune(value)[:1000]) + "…"
			}
			out = append(out, value)
		}
	}
	return out
}
