package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxModelShots         = 80
	maxModelResponseBytes = 256 << 10
	maxModelValueRunes    = 4000
)

type videoAnalysisModelConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	Timeout  time.Duration
}

func loadVideoAnalysisModelConfig() (videoAnalysisModelConfig, bool, error) {
	cfg := videoAnalysisModelConfig{
		Provider: strings.ToLower(strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_PROVIDER"))),
		BaseURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_BASE_URL")), "/"),
		APIKey:   strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_API_KEY")),
		Model:    strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_MODEL")),
		Timeout:  90 * time.Second,
	}
	if raw := strings.TrimSpace(os.Getenv("WX_CHANNEL_VIDEO_ANALYSIS_TIMEOUT_SECONDS")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 5 || seconds > 600 {
			return videoAnalysisModelConfig{}, false, errors.New("invalid_argument: WX_CHANNEL_VIDEO_ANALYSIS_TIMEOUT_SECONDS must be between 5 and 600")
		}
		cfg.Timeout = time.Duration(seconds) * time.Second
	}
	configured := cfg.Provider != "" || cfg.BaseURL != "" || cfg.APIKey != "" || cfg.Model != ""
	if !configured {
		return cfg, false, nil
	}
	if cfg.Provider == "" {
		cfg.Provider = "openai_compatible"
	}
	switch cfg.Provider {
	case "openai_compatible":
	case "gemini":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
		}
	default:
		return videoAnalysisModelConfig{}, false, errors.New("model_not_configured: external provider must be openai_compatible or gemini")
	}
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return videoAnalysisModelConfig{}, false, errors.New("model_not_configured: video analysis model requires base URL, API key, and model")
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return videoAnalysisModelConfig{}, false, errors.New("model_not_configured: video analysis base URL is invalid")
	}
	return cfg, true, nil
}

// API keys remain in memory only and do not influence persisted cache identity.
func analysisModelConfigKey(cfg videoAnalysisModelConfig) string {
	identity, _ := json.Marshal([]string{cfg.Provider, cfg.BaseURL, cfg.Model})
	return fmt.Sprintf("%x", sha256.Sum256(identity))
}

func (s *Server) enrichAnalysisWithModel(ctx context.Context, job *videoAnalysisJob, manifest *videoAnalysisManifest) {
	if job.ModelConfig == nil {
		manifest.ModelStatus = "not_configured"
		manifest.Warnings = append(manifest.Warnings, "model_not_configured_codex_review_available")
		return
	}
	cfg := *job.ModelConfig
	manifest.ModelStatus = "running"
	manifest.Model = cfg.Model
	answered := 0
	limit := len(manifest.Shots)
	if limit > maxModelShots {
		limit = maxModelShots
		manifest.Warnings = append(manifest.Warnings, "model_shot_count_limited")
	}
	for i := 0; i < limit; i++ {
		if err := ctx.Err(); err != nil {
			manifest.ModelStatus = "partial"
			manifest.Warnings = append(manifest.Warnings, "model_analysis_canceled")
			return
		}
		vision, callErr := s.callVideoAnalysisModel(ctx, cfg, job, manifest.Shots[i])
		if callErr != nil {
			manifest.Warnings = appendLocalWarnings(manifest.Warnings, "model_call_failed")
			continue
		}
		manifest.Shots[i].Vision = vision
		if replication, ok := vision["replication"].(map[string]any); ok {
			manifest.Shots[i].Replication = replication
		}
		answered++
		manifest.Progress = 0.75 + 0.2*float64(i+1)/float64(limit)
		manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.updateAnalysisJob(job, func(current *videoAnalysisManifest) {
			current.Progress = manifest.Progress
			current.ModelStatus = "running"
			current.Model = cfg.Model
			current.UpdatedAt = manifest.UpdatedAt
		}); err != nil {
			manifest.ModelStatus = "partial"
			manifest.Warnings = appendLocalWarnings(manifest.Warnings, "analysis_progress_save_failed")
			return
		}
	}
	switch {
	case answered == 0:
		manifest.ModelStatus = "failed"
	case answered < limit:
		manifest.ModelStatus = "partial"
	default:
		manifest.ModelStatus = "completed"
	}
	if limit < len(manifest.Shots) && answered > 0 {
		manifest.ModelStatus = "partial"
	}
}

func (s *Server) callVideoAnalysisModel(ctx context.Context, cfg videoAnalysisModelConfig, job *videoAnalysisJob, shot analysisShot) (map[string]any, error) {
	root, err := s.analysisRoot(job.ID, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	parts := make([]map[string]any, 0, len(shot.Keyframes)+1)
	for _, frame := range shot.Keyframes {
		data, err := readAnalysisKeyframe(root, job.ID, frame.RelativePath)
		if err != nil {
			return nil, err
		}
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)}})
	}
	parts = append(parts, map[string]any{"type": "text", "text": shotPromptForModel(shot)})
	body := map[string]any{
		"model":       cfg.Model,
		"messages":    []any{map[string]any{"role": "user", "content": parts}},
		"temperature": 0.2,
		"max_tokens":  2500,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
	if err != nil || len(responseBody) > maxModelResponseBytes {
		return nil, errors.New("model_response_too_large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("model_http_error")
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil || len(envelope.Choices) == 0 {
		return nil, errors.New("model_response_invalid")
	}
	content := modelContentText(envelope.Choices[0].Message.Content)
	return parseModelJSON(content)
}

func shotPromptForModel(shot analysisShot) string {
	prompt, _ := shot.Replication["prompt"].(string)
	if prompt == "" {
		prompt = "请逐镜头分析视频画面。"
	}
	if localEvidence, err := json.Marshal(map[string]any{"transcript": shot.Evidence["local_transcript"], "ocr": shot.Evidence["local_ocr"]}); err == nil && len(localEvidence) > 2 && string(localEvidence) != `{"ocr":null,"transcript":null}` && string(localEvidence) != `{"transcript":null,"ocr":null}` {
		if len(localEvidence) > 12<<10 {
			localEvidence = localEvidence[:12<<10]
		}
		prompt += " 本地音频转写与关键帧 OCR 仅作为辅助证据，不能替代画面核验：" + string(localEvidence)
	}
	return prompt + " 只返回 JSON，字段包括 label_probs、evidence、description、shot_size、camera_angle、camera_move、composition、subject、action、lighting、color、overlay_text、material_origin、embedded_assets、ai_edits、replication。replication 包含 shooting_plan 和 video_prompt。按关键帧给出 frame_analyses（timestamp_seconds、description），按叙事段落给出 storyboard（start_seconds、end_seconds、picture、dialogue、action、camera、sound、production_notes），连续镜头可以分段但不要虚构剪辑。证据不足时返回待确认，不要编造具体设备；未提供音频时不得声称听到对白。"
}

func modelContentText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	parts, ok := value.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if object, ok := part.(map[string]any); ok {
			if text, ok := object["text"].(string); ok {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

func parseModelJSON(content string) (map[string]any, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```JSON")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, errors.New("model_response_missing_json")
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(content[start:end+1]), &result); err != nil {
		return nil, errors.New("model_response_invalid_json")
	}
	if description, ok := result["description"].(string); !ok || strings.TrimSpace(description) == "" {
		return nil, errors.New("model_response_missing_description")
	}
	bounded := boundModelMap(result)
	encoded, err := json.Marshal(bounded)
	if err != nil || len(encoded) > 64<<10 {
		return nil, errors.New("model_result_too_large")
	}
	return bounded, nil
}

func boundModelMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = boundModelValue(item)
	}
	return out
}

func boundModelValue(value any) any {
	switch item := value.(type) {
	case string:
		if len([]rune(item)) > maxModelValueRunes {
			return string([]rune(item)[:maxModelValueRunes]) + "…"
		}
		return item
	case []any:
		if len(item) > 40 {
			item = item[:40]
		}
		out := make([]any, len(item))
		for i, child := range item {
			out[i] = boundModelValue(child)
		}
		return out
	case map[string]any:
		return boundModelMap(item)
	default:
		return value
	}
}
