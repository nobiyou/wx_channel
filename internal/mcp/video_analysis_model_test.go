package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseModelJSONAcceptsFencedResponseAndBoundsText(t *testing.T) {
	result, err := parseModelJSON("```json\n{\"description\":\"hello\",\"replication\":{\"video_prompt\":\"world\"}}\n```")
	if err != nil {
		t.Fatalf("parseModelJSON() error = %v", err)
	}
	if result["description"] != "hello" {
		t.Fatalf("description = %#v", result["description"])
	}
	if _, ok := result["replication"].(map[string]any); !ok {
		t.Fatalf("replication = %#v", result["replication"])
	}
}

func TestCallVideoAnalysisModelSendsBoundedPromptAndParsesJSON(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	root, err := s.analysisRoot(id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := writeAnalysisFile(root, "shots/shot_001_1.jpg", frameDecoderTestJPEG(t, 160, 120)); err != nil {
		t.Fatal(err)
	}
	job := &videoAnalysisJob{ID: id}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "test-model" || r.URL.Path != "/chat/completions" {
			t.Errorf("request = %s %#v", r.URL.Path, body)
		}
		parts := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
		if len(parts) != 2 || parts[0].(map[string]any)["type"] != "image_url" {
			t.Errorf("model content = %#v", parts)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"description\":\"a shot\",\"replication\":{\"video_prompt\":\"move\"}}"}}]}`))
	}))
	defer server.Close()
	shot := analysisShot{Index: 1, Start: 0, End: 1, Keyframes: []analysisKeyframe{{RelativePath: analysisReference(id, "shots/shot_001_1.jpg")}}, Replication: map[string]any{"prompt": "analyze"}}
	result, err := s.callVideoAnalysisModel(t.Context(), videoAnalysisModelConfig{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"}, job, shot)
	if err != nil {
		t.Fatalf("callVideoAnalysisModel() error = %v", err)
	}
	if result["description"] != "a shot" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAnalysisKeyframeRejectsEscapesAndOtherAnalysis(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	root, err := s.analysisRoot(id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, ref := range []string{"../outside.jpg", analysisReference("other", "shots/image.jpg"), analysisReference(id, "manifest.json"), analysisReference(id, "shots/../../outside.jpg")} {
		if _, err := readAnalysisKeyframe(root, id, ref); err == nil {
			t.Errorf("accepted %q", ref)
		}
	}
}

func TestAnalysisKeyframeRejectsOutsideSymlink(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.jpg")
	if err := os.WriteFile(outside, frameDecoderTestJPEG(t, 160, 120), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.deps.DownloadsDir, analysisArtifactDir, id, "shots", "escape.jpg")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	root, err := s.analysisRoot(id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := readAnalysisKeyframe(root, id, analysisReference(id, "shots/escape.jpg")); err == nil {
		t.Fatal("read outside symlink")
	}
}

func TestVideoAnalysisModelDoesNotFollowUploadRedirect(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	var followed atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Add(1); w.Write([]byte(`{}`)) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := s.callVideoAnalysisModel(t.Context(), videoAnalysisModelConfig{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"}, &videoAnalysisJob{ID: id}, analysisShot{Index: 1})
	if err == nil || followed.Load() != 0 {
		t.Fatalf("redirect result: %v; follow count %d", err, followed.Load())
	}
}

func TestParseModelJSONRejectsEmptyAndBoundsDescription(t *testing.T) {
	for _, raw := range []string{`{}`, `{"description":" "}`, `{"description":123}`} {
		if _, err := parseModelJSON(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	raw, _ := json.Marshal(map[string]any{"description": strings.Repeat("字", 10000)})
	result, err := parseModelJSON(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(result["description"].(string))) > maxModelValueRunes+1 {
		t.Fatal("description not bounded")
	}
}
