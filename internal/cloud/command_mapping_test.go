package cloud

import (
	"testing"

	json "github.com/json-iterator/go"
)

type mappedPayload struct {
	Key  string                 `json:"key"`
	Body map[string]interface{} `json:"body"`
}

func decodeMappedPayload(t *testing.T, raw json.RawMessage) mappedPayload {
	t.Helper()

	var payload mappedPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal(mapped payload): %v", err)
	}
	return payload
}

func TestMapCommandToAPICallSearchChannels(t *testing.T) {
	raw, mapped, err := mapCommandToAPICall("search_channels", json.RawMessage(`{"keyword":"作者A","next_marker":"abc"}`))
	if err != nil {
		t.Fatalf("mapCommandToAPICall: %v", err)
	}
	if !mapped {
		t.Fatalf("mapped = false, want true")
	}

	payload := decodeMappedPayload(t, raw)
	if payload.Key != "key:channels:contact_list" {
		t.Fatalf("payload.Key = %q, want contact_list", payload.Key)
	}
	if got := payload.Body["keyword"]; got != "作者A" {
		t.Fatalf("keyword = %#v, want 作者A", got)
	}
	if got := payload.Body["type"]; got != float64(1) {
		t.Fatalf("type = %#v, want 1", got)
	}
	if got := payload.Body["next_marker"]; got != "abc" {
		t.Fatalf("next_marker = %#v, want abc", got)
	}
}

func TestMapCommandToAPICallSearchVideos(t *testing.T) {
	raw, mapped, err := mapCommandToAPICall("search_videos", json.RawMessage(`{"keyword":"视频关键词"}`))
	if err != nil {
		t.Fatalf("mapCommandToAPICall: %v", err)
	}
	if !mapped {
		t.Fatalf("mapped = false, want true")
	}

	payload := decodeMappedPayload(t, raw)
	if payload.Key != "key:channels:contact_list" {
		t.Fatalf("payload.Key = %q, want contact_list", payload.Key)
	}
	if got := payload.Body["type"]; got != float64(3) {
		t.Fatalf("type = %#v, want 3", got)
	}
}

func TestMapCommandToAPICallDownloadVideoNormalizesAliases(t *testing.T) {
	raw, mapped, err := mapCommandToAPICall("download_video", json.RawMessage(`{"url":"https://cdn.example.com/video.mp4","pageUrl":"https://channels.weixin.qq.com/p/123","decryptKey":"secret","title":"视频标题","author":"作者A"}`))
	if err != nil {
		t.Fatalf("mapCommandToAPICall: %v", err)
	}
	if !mapped {
		t.Fatalf("mapped = false, want true")
	}

	payload := decodeMappedPayload(t, raw)
	if payload.Key != "key:channels:download_video" {
		t.Fatalf("payload.Key = %q, want download_video", payload.Key)
	}
	if got := payload.Body["videoUrl"]; got != "https://cdn.example.com/video.mp4" {
		t.Fatalf("videoUrl = %#v, want normalized url", got)
	}
	if got := payload.Body["sourceUrl"]; got != "https://channels.weixin.qq.com/p/123" {
		t.Fatalf("sourceUrl = %#v, want normalized page url", got)
	}
	if got := payload.Body["key"]; got != "secret" {
		t.Fatalf("key = %#v, want normalized decrypt key", got)
	}
	if _, exists := payload.Body["url"]; exists {
		t.Fatalf("unexpected url alias left in payload")
	}
	if _, exists := payload.Body["pageUrl"]; exists {
		t.Fatalf("unexpected pageUrl alias left in payload")
	}
	if _, exists := payload.Body["decryptKey"]; exists {
		t.Fatalf("unexpected decryptKey alias left in payload")
	}
}

func TestMapCommandToAPICallUnknownAction(t *testing.T) {
	raw, mapped, err := mapCommandToAPICall("api_call", json.RawMessage(`{"key":"key:channels:feed_profile"}`))
	if err != nil {
		t.Fatalf("mapCommandToAPICall: %v", err)
	}
	if mapped {
		t.Fatalf("mapped = true, want false")
	}
	if raw != nil {
		t.Fatalf("raw = %q, want nil", string(raw))
	}
}
