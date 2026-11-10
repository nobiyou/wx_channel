package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func clearAnalysisModelEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"WX_CHANNEL_VIDEO_ANALYSIS_PROVIDER", "WX_CHANNEL_VIDEO_ANALYSIS_BASE_URL", "WX_CHANNEL_VIDEO_ANALYSIS_API_KEY", "WX_CHANNEL_VIDEO_ANALYSIS_MODEL", "WX_CHANNEL_VIDEO_ANALYSIS_TIMEOUT_SECONDS"} {
		t.Setenv(name, "")
	}
}

func TestGeminiAnalysisSendsImagesToCompatibleRoute(t *testing.T) {
	clearAnalysisModelEnvironment(t)
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/openai/chat/completions" || r.Header.Get("Authorization") != "Bearer gemini-test-key" {
			t.Errorf("Gemini request: %s, authentication correct=%v", r.URL.Path, r.Header.Get("Authorization") == "Bearer gemini-test-key")
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body.Model != "gemini-test-model" || len(body.Messages) != 1 || len(body.Messages[0].Content) != 2 {
			t.Error("invalid Gemini image request")
			return
		}
		image := body.Messages[0].Content[0]
		if image.Type != "image_url" || image.ImageURL.URL != "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(frame) {
			t.Error("Gemini image payload differs from the local keyframe")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"description\":\"Gemini vision result\",\"evidence\":[\"visible image\"]}"}}]}`))
	}))
	defer server.Close()
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_PROVIDER", "gemini")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_BASE_URL", server.URL+"/v1beta/openai/")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_API_KEY", "gemini-test-key")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_MODEL", "gemini-test-model")
	cfg, configured, err := loadVideoAnalysisModelConfig()
	if err != nil || !configured {
		t.Fatalf("Gemini configuration: %v/%v", configured, err)
	}
	shot := analysisShot{Index: 1, Start: 0, End: 1, Keyframes: []analysisKeyframe{{RelativePath: analysisReference(id, "shots/shot_001_1.jpg")}}}
	result, err := s.callVideoAnalysisModel(t.Context(), cfg, &videoAnalysisJob{ID: id}, shot)
	if err != nil || result["description"] != "Gemini vision result" {
		t.Fatalf("Gemini response: %v, %#v", err, result)
	}
}

func TestExternalAnalysisCacheSeparatesProviderModelAndEndpoint(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	job, err := s.loadAnalysisJob(id)
	if err != nil {
		t.Fatal(err)
	}
	cfg := videoAnalysisModelConfig{Provider: "openai_compatible", BaseURL: "https://example.invalid/v1", APIKey: "first-key", Model: "model-a"}
	job.Manifest.UseExternalModel = true
	job.Manifest.AnalysisProvider = cfg.Provider
	job.Manifest.Model = cfg.Model
	job.Manifest.ModelConfigKey = analysisModelConfigKey(cfg)
	job.Manifest.Mode = "hybrid"
	job.Manifest.SourceModified = "fixture"
	job.Manifest.SourceSize = 123
	job.Manifest.PromptVersion = analysisPromptVersion
	if err := s.saveAnalysisManifestLocked(job.Manifest); err != nil {
		t.Fatal(err)
	}
	find := func(config videoAnalysisModelConfig) *videoAnalysisManifest {
		return s.findCachedAnalysisLocked("sample.mp4", "fixture", 123, "hybrid", true, analysisModelConfigKey(config))
	}
	if find(cfg) == nil {
		t.Fatal("matching model cache not found")
	}
	for _, field := range []string{"provider", "model", "endpoint"} {
		changed := cfg
		switch field {
		case "provider":
			changed.Provider = "gemini"
		case "model":
			changed.Model = "model-b"
		case "endpoint":
			changed.BaseURL = "https://other.invalid/v1"
		}
		if find(changed) != nil {
			t.Errorf("cache reused across different %s", field)
		}
	}
	cfg.APIKey = "rotated-key"
	if find(cfg) == nil {
		t.Fatal("API key rotation invalidated model identity")
	}
	if strings.Contains(job.Manifest.ModelConfigKey, "first-key") {
		t.Fatal("cache identity contains API key")
	}
}

func TestQueuedExternalAnalysisKeepsSelectedConfiguration(t *testing.T) {
	clearAnalysisModelEnvironment(t)
	s, id := completedAnalysisFixture(t)
	var selectedCalls, otherCalls atomic.Int32
	selected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selectedCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer selected-key" {
			t.Error("selected credentials changed")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"description\":\"selected model\"}"}}]}`))
	}))
	defer selected.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherCalls.Add(1); w.Write([]byte(`{}`)) }))
	defer other.Close()
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_BASE_URL", other.URL)
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_API_KEY", "other-key")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_MODEL", "other-model")
	job, err := s.loadAnalysisJob(id)
	if err != nil {
		t.Fatal(err)
	}
	job.ModelConfig = &videoAnalysisModelConfig{Provider: "openai_compatible", BaseURL: selected.URL, APIKey: "selected-key", Model: "selected-model"}
	m := job.Manifest
	s.enrichAnalysisWithModel(t.Context(), job, &m)
	if selectedCalls.Load() != 1 || otherCalls.Load() != 0 || m.ModelStatus != "completed" || m.Model != "selected-model" {
		t.Fatalf("model selection changed: %d/%d/%s/%s", selectedCalls.Load(), otherCalls.Load(), m.ModelStatus, m.Model)
	}
}

func TestGeminiAnalysisProviderUsesOfficialCompatibleEndpoint(t *testing.T) {
	clearAnalysisModelEnvironment(t)
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_PROVIDER", "gemini")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_API_KEY", "test-key")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_MODEL", "gemini-test-model")
	cfg, configured, err := loadVideoAnalysisModelConfig()
	if err != nil || !configured {
		t.Fatalf("Gemini configuration: configured=%v err=%v", configured, err)
	}
	if cfg.BaseURL != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Fatalf("Gemini endpoint = %q", cfg.BaseURL)
	}
}

func TestAnalysisRejectsUnsupportedAPIProvider(t *testing.T) {
	clearAnalysisModelEnvironment(t)
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_PROVIDER", "unsupported")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_BASE_URL", "https://example.invalid/v1")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_API_KEY", "test-key")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_MODEL", "test-model")
	if _, _, err := loadVideoAnalysisModelConfig(); err == nil {
		t.Fatal("accepted unsupported external provider")
	}
}

func TestDefaultAnalysisUsesCodexWithoutAPIConfiguration(t *testing.T) {
	clearAnalysisModelEnvironment(t)
	s, id := completedAnalysisFixture(t)
	_, out, err := s.getVideoAnalysisStatus(t.Context(), nil, videoAnalysisStatusInput{AnalysisID: id})
	if err != nil {
		t.Fatal(err)
	}
	data := out["data"].(map[string]any)
	if data["analysis_provider"] != "codex" || data["review_status"] != "awaiting_codex_review" {
		t.Fatalf("default analysis = %#v", data)
	}
	_, runtime, err := s.getRuntimeStatus(context.Background(), nil, emptyInput{})
	if err != nil {
		t.Fatal(err)
	}
	analysis := runtime["data"].(map[string]any)["video_analysis"].(map[string]any)
	if analysis["default_provider"] != "codex" {
		t.Fatalf("runtime default = %#v", analysis)
	}
	_, _, err = s.saveVideoAnalysisReview(t.Context(), videoAnalysisReviewInput{AnalysisID: id, ShotIndex: 1, Description: "由当前 Codex 会话分析", ShootingPlan: "固定机位"})
	if err != nil {
		t.Fatal(err)
	}
	_, out, err = s.getVideoAnalysisStatus(t.Context(), nil, videoAnalysisStatusInput{AnalysisID: id})
	if err != nil {
		t.Fatal(err)
	}
	if out["data"].(map[string]any)["review_status"] != "completed" {
		t.Fatalf("Codex review not marked completed: %#v", out)
	}
}
