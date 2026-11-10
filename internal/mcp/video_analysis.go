package mcpserver

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxAnalysisVideoBytes     int64 = 1 << 30
	maxAnalysisShots                = 80
	analysisSampleWidth             = 640
	analysisMinShotSeconds          = 0.35
	analysisBoundaryThreshold       = 0.30
	analysisArtifactDir             = ".wx_channel_analysis"
	analysisPromptVersion           = "shooting-reconstruction-v3"
)

var analysisKeyframeFractions = [...]float64{0.05, 0.20, 0.35, 0.50, 0.65, 0.80, 0.95}

type videoAnalysisInput struct {
	RelativePath     string `json:"relative_path,omitempty" jsonschema:"local video path relative to the downloads directory; use only when no download record id is available because metadata and metrics may be unavailable"`
	RecordID         string `json:"record_id,omitempty" jsonschema:"completed download record id; preferred because it preserves title, author, identity, and captured metrics"`
	Source           string `json:"source,omitempty" jsonschema:"download when using record_id; analysis requires a completed download"`
	Mode             string `json:"mode,omitempty" jsonschema:"hybrid uses the current Codex session to analyze frames and save reviews without an API key (default); local extracts evidence only"`
	UseExternalModel bool   `json:"use_external_model,omitempty" jsonschema:"explicitly upload keyframe images to the configured external model; default false"`
	Force            bool   `json:"force,omitempty" jsonschema:"start a new analysis even when a matching completed analysis exists"`
}

type videoAnalysisStatusInput struct {
	AnalysisID string `json:"analysis_id" jsonschema:"analysis id returned by start_video_analysis"`
}

type videoAnalysisShotsInput struct {
	AnalysisID    string `json:"analysis_id" jsonschema:"analysis id returned by start_video_analysis"`
	Page          int    `json:"page,omitempty" jsonschema:"one-based page number, default 1"`
	PageSize      int    `json:"page_size,omitempty" jsonschema:"number of shots, maximum 12"`
	IncludeImages bool   `json:"include_images,omitempty" jsonschema:"include up to three images per call; use page_size 1 and image_offset to review every keyframe"`
	ImageOffset   int    `json:"image_offset,omitempty" jsonschema:"zero-based image offset within this shot page; follow next_image_offset until images_has_more is false"`
}

type videoAnalysisArtifactInput struct {
	AnalysisID string `json:"analysis_id" jsonschema:"analysis id returned by start_video_analysis"`
	Format     string `json:"format,omitempty" jsonschema:"summary, json, or html; default summary"`
}

type analysisKeyframe struct {
	Timestamp    float64 `json:"timestamp_seconds"`
	RelativePath string  `json:"relative_path"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
}

type analysisShot struct {
	Index        int                `json:"index"`
	Start        float64            `json:"start_seconds"`
	End          float64            `json:"end_seconds"`
	Duration     float64            `json:"duration_seconds"`
	TransitionIn string             `json:"transition_in"`
	Keyframes    []analysisKeyframe `json:"keyframes"`
	Evidence     map[string]any     `json:"evidence"`
	Vision       map[string]any     `json:"vision,omitempty"`
	Review       map[string]any     `json:"review,omitempty"`
	Replication  map[string]any     `json:"replication,omitempty"`
}

type localTranscriptSegment struct {
	Start float64 `json:"start_seconds"`
	End   float64 `json:"end_seconds"`
	Text  string  `json:"text"`
}

type localASRReport struct {
	Status          string                   `json:"status"`
	Backend         string                   `json:"backend,omitempty"`
	Model           string                   `json:"model,omitempty"`
	Device          string                   `json:"device,omitempty"`
	Language        string                   `json:"language,omitempty"`
	LanguageProb    float64                  `json:"language_probability,omitempty"`
	Segments        []localTranscriptSegment `json:"segments,omitempty"`
	SegmentCount    int                      `json:"segment_count"`
	Error           string                   `json:"error,omitempty"`
	MissingReason   string                   `json:"missing_reason,omitempty"`
	DurationSeconds float64                  `json:"duration_seconds,omitempty"`
	Truncated       bool                     `json:"truncated,omitempty"`
}

type localOCRText struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence,omitempty"`
}

type localOCRFrame struct {
	ShotIndex     int            `json:"shot_index"`
	Timestamp     float64        `json:"timestamp_seconds"`
	RelativePath  string         `json:"relative_path"`
	Texts         []localOCRText `json:"texts,omitempty"`
	Status        string         `json:"status"`
	MissingReason string         `json:"missing_reason,omitempty"`
}

type localOCRReport struct {
	Status        string          `json:"status"`
	Backend       string          `json:"backend,omitempty"`
	FrameCount    int             `json:"frame_count"`
	TextCount     int             `json:"text_count"`
	Frames        []localOCRFrame `json:"frames,omitempty"`
	Error         string          `json:"error,omitempty"`
	MissingReason string          `json:"missing_reason,omitempty"`
	Truncated     bool            `json:"truncated,omitempty"`
}

type localEnrichmentReport struct {
	Status         string         `json:"status"`
	Enabled        bool           `json:"enabled"`
	ASR            localASRReport `json:"asr"`
	OCR            localOCRReport `json:"ocr"`
	MissingReasons []string       `json:"missing_reasons,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// videoAnalysisDetails is the safe, report-facing identity of the analyzed
// media. It intentionally contains only a path relative to the configured
// downloads directory; source URLs, referers, decrypt keys, and absolute
// filesystem paths never belong in an analysis artifact.
type videoAnalysisDetails struct {
	Title               string            `json:"title,omitempty"`
	Author              string            `json:"author,omitempty"`
	VideoID             string            `json:"video_id,omitempty"`
	RecordID            string            `json:"record_id,omitempty"`
	Source              string            `json:"source,omitempty"`
	RelativePath        string            `json:"relative_path,omitempty"`
	FileName            string            `json:"file_name,omitempty"`
	FileSizeBytes       int64             `json:"file_size_bytes,omitempty"`
	Format              string            `json:"format,omitempty"`
	MIMEType            string            `json:"mime_type,omitempty"`
	DurationSeconds     float64           `json:"duration_seconds,omitempty"`
	Width               int               `json:"width,omitempty"`
	Height              int               `json:"height,omitempty"`
	Resolution          string            `json:"resolution,omitempty"`
	AudioStreamPresent  *bool             `json:"audio_stream_present,omitempty"`
	AudioStatus         string            `json:"audio_status,omitempty"`
	LikeCount           *int64            `json:"like_count"`
	CommentCount        *int64            `json:"comment_count"`
	FavoriteCount       *int64            `json:"favorite_count"`
	ForwardCount        *int64            `json:"forward_count"`
	MetricsSource       string            `json:"metrics_source,omitempty"`
	MetricsObservedAt   *string           `json:"metrics_observed_at"`
	MetricsVerification string            `json:"metrics_source_verification,omitempty"`
	MetricsStatus       string            `json:"metrics_status"`
	MetricsAvailability map[string]string `json:"metrics_availability"`
	AnalysisStatus      string            `json:"analysis_status,omitempty"`
	AnalysisStartedAt   string            `json:"analysis_started_at,omitempty"`
	AnalysisCompletedAt string            `json:"analysis_completed_at,omitempty"`
	AnalysisProvider    string            `json:"analysis_provider,omitempty"`
	Model               string            `json:"model,omitempty"`
}

type videoAnalysisManifest struct {
	AnalysisID       string                `json:"analysis_id"`
	RecordID         string                `json:"record_id,omitempty"`
	RecordSource     string                `json:"record_source,omitempty"`
	RelativePath     string                `json:"relative_path"`
	Status           string                `json:"status"`
	Mode             string                `json:"mode"`
	UseExternalModel bool                  `json:"use_external_model"`
	AnalysisProvider string                `json:"analysis_provider"`
	ModelConfigKey   string                `json:"model_config_key,omitempty"`
	ReviewStatus     string                `json:"review_status"`
	SourceSize       int64                 `json:"source_size"`
	SourceModified   string                `json:"source_modified"`
	ReviewCount      int                   `json:"review_count"`
	CreatedAt        string                `json:"created_at"`
	UpdatedAt        string                `json:"updated_at"`
	CompletedAt      string                `json:"completed_at,omitempty"`
	Progress         float64               `json:"progress"`
	DurationSeconds  float64               `json:"duration_seconds,omitempty"`
	Width            int                   `json:"width,omitempty"`
	Height           int                   `json:"height,omitempty"`
	Resolution       string                `json:"resolution,omitempty"`
	SampleCount      int                   `json:"sample_count"`
	Shots            []analysisShot        `json:"shots,omitempty"`
	ModelStatus      string                `json:"model_status"`
	Model            string                `json:"model,omitempty"`
	PromptVersion    string                `json:"prompt_version"`
	ReportJSON       string                `json:"report_json"`
	ReportHTML       string                `json:"report_html"`
	Error            string                `json:"error,omitempty"`
	Warnings         []string              `json:"warnings,omitempty"`
	Audio            map[string]any        `json:"audio,omitempty"`
	LocalEnrichment  localEnrichmentReport `json:"local_enrichment"`
	VideoDetails     videoAnalysisDetails  `json:"video_details"`
}

type videoAnalysisJob struct {
	ModelConfig  *videoAnalysisModelConfig `json:"-"`
	ID           string
	RelativePath string
	RecordID     string
	Source       string
	Mode         string
	mu           sync.Mutex
	Manifest     videoAnalysisManifest
}

func registerVideoAnalysisTools(s *Server) {
	startTool := &mcp.Tool{
		Name:        "start_video_analysis",
		Description: "Prepare analysis of a completed local download. FFmpeg extracts shots, keyframes, audio evidence, and local enrichment (faster-whisper/PaddleOCR when installed). By default the current Codex session analyzes returned shot images and saves findings with save_video_analysis_review; no additional model API key is needed. Poll status, then read every shot with images and save the shooting reconstruction. External APIs (including Gemini) run only with use_external_model=true. Downloading is independent.",
	}
	startTool.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)}
	mcp.AddTool(s.mcpServer, startTool, func(ctx context.Context, req *mcp.CallToolRequest, input videoAnalysisInput) (*mcp.CallToolResult, map[string]any, error) {
		result, out, err := s.startVideoAnalysis(ctx, input)
		if err != nil {
			return analysisToolFailure(err)
		}
		return result, out, nil
	})
	registerAnalysisReviewTool(s)
	registerReadTool(s, &mcp.Tool{
		Name:        "get_video_analysis_status",
		Description: "Return the status and bounded summary of a local video analysis job.",
	}, s.getVideoAnalysisStatus)
	registerReadTool(s, &mcp.Tool{
		Name:        "get_video_analysis_shots",
		Description: "Return a bounded page of shots and up to three keyframe images. With include_images=true use page_size=1 and follow next_image_offset until images_has_more=false to review every frame, then advance the shot page. Save visual fields, timestamped transcript and storyboard with save_video_analysis_review.",
	}, s.getVideoAnalysisShots)
	registerReadTool(s, &mcp.Tool{
		Name:        "get_video_analysis_report",
		Description: "Return a bounded analysis summary or safe relative references to the generated JSON/HTML report.",
	}, s.getVideoAnalysisReport)
}

func analysisToolFailure(err error) (*mcp.CallToolResult, map[string]any, error) {
	var typed *toolFailure
	if errors.Is(err, context.Canceled) {
		err = failure("canceled")
	} else if errors.Is(err, context.DeadlineExceeded) {
		err = failure("analysis_timeout")
	}
	if !errors.As(err, &typed) {
		code := strings.SplitN(err.Error(), ":", 2)[0]
		typed = failure(code)
	}
	out := envelope("local_video_analysis", "live", map[string]any{}, nil)
	out["partial"] = true
	out["error"] = typed
	return &mcp.CallToolResult{IsError: true}, out, nil
}

func boolPtr(v bool) *bool { return &v }
