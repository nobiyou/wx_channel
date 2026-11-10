package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wx_channel/internal/database"
)

func localContractServer(t *testing.T) *Server {
	t.Helper()
	t.Cleanup(setupMCPDatabase(t))
	return &Server{deps: Dependencies{DownloadsDir: t.TempDir()}}
}

func localContractBrowse(t *testing.T, id, authorID string) {
	t.Helper()
	err := database.NewBrowseHistoryRepository().Create(&database.BrowseRecord{
		ID: id, Title: "stored video", Author: "author", AuthorID: authorID,
		BrowseTime: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), LikeCount: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func localContractDownload(t *testing.T, id, path string) {
	t.Helper()
	err := database.NewDownloadRecordRepository().Create(&database.DownloadRecord{
		ID: id, VideoID: id, Title: "stored video", Author: "author", FilePath: path,
		Status:       database.DownloadStatusCompleted,
		DownloadTime: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), ForwardCount: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLocalMediaAssociatesDownloadRecordByRelativePath(t *testing.T) {
	s := localContractServer(t)
	path := filepath.Join(s.deps.DownloadsDir, "nested", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	localContractDownload(t, "download-by-path", path)

	media, err := s.openLocalMedia(context.Background(), localMediaMetadataInput{RelativePath: "nested/video.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	defer media.file.Close()
	if media.record == nil || media.record.ID != "download-by-path" || media.record.Source != "download" {
		t.Fatalf("relative path record = %#v", media.record)
	}
}

func localContractJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func localContractWireSize(t *testing.T, result map[string]any) int {
	t.Helper()
	return len(localContractJSON(t, &mcp.CallToolResult{
		StructuredContent: result,
		Content:           []mcp.Content{&mcp.TextContent{Text: string(localContractJSON(t, result))}},
	}))
}

func TestLocalContractMetricsAndSharedProjection(t *testing.T) {
	s := localContractServer(t)
	path := filepath.Join(s.deps.DownloadsDir, "video.mp4")
	if err := os.WriteFile(path, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	localContractBrowse(t, "browse", "account")
	localContractDownload(t, "download", path)
	_, err := database.GetDB().Exec(`UPDATE download_records SET cover_url = ?, error_message = ?, comment_count = -1 WHERE id = ?`,
		"https://cdn.example/cover.jpg?token=secret", "open "+path+": token=secret", "download")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetDB().Exec("UPDATE browse_history SET cover_url = ?, browse_time = ? WHERE id = ?", "https://cdn.example/cover.jpg?cache_id=secret#secret", time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), "browse"); err != nil {
		t.Fatal(err)
	}
	_, result, err := s.listLocalVideos(context.Background(), nil, listLocalVideosInput{})
	if err != nil {
		t.Fatal(err)
	}
	items := result["data"].(map[string]any)["items"].([]map[string]any)
	if got := result["observed_at"]; got != "2026-11-01T12:00:00Z" {
		t.Errorf("list observed_at = %v, want stored observation time", got)
	}
	for _, item := range items {
		t.Run(item["source"].(string), func(t *testing.T) {
			for _, key := range []string{"play_count", "comment_count", "favorite_count"} {
				value, exists := item[key]
				if !exists || value != nil {
					t.Errorf("%s = %#v (present %v), want explicit null", key, value, exists)
				}
			}
			if !strings.Contains(string(localContractJSON(t, result["warnings"])), "stored_zero_metrics_unknown") {
				t.Error("missing ambiguous-zero warning")
			}
			if item["source"] == "browse" && item["like_count"] != int64(7) {
				t.Errorf("nonzero like_count = %v", item["like_count"])
			}
			if item["source"] == "download" {
				if item["forward_count"] != int64(9) {
					t.Errorf("nonzero forward_count = %v", item["forward_count"])
				}
				file, ok := item["file"].(map[string]any)
				if !ok || file["relative_path"] != "video.mp4" || file["exists"] != true {
					t.Errorf("safe list file reference = %v", item["file"])
				}
			}
			_, detail, err := s.getLocalVideo(context.Background(), nil, getLocalVideoInput{ID: item["id"].(string), Source: item["source"].(string)})
			if err != nil {
				t.Fatal(err)
			}
			if string(localContractJSON(t, item)) != string(localContractJSON(t, detail["data"])) {
				t.Error("list/detail projections differ")
			}
			if detail["observed_at"] != item["observed_at"] {
				t.Errorf("detail observed_at = %v, want stored %v", detail["observed_at"], item["observed_at"])
			}
		})
	}
	encoded := string(localContractJSON(t, result))
	if strings.Contains(encoded, "secret") || strings.Contains(encoded, "error_message") || strings.Contains(encoded, "netdisk") {
		t.Errorf("unsafe stored fields leaked: %.350s", encoded)
	}
	if !strings.Contains(encoded, "[redacted_signed_url]") {
		t.Error("cover URL was not sanitized")
	}
	_, empty, err := s.listLocalVideos(context.Background(), nil, listLocalVideosInput{Query: "no matching record"})
	if err != nil {
		t.Fatal(err)
	}
	if empty["observed_at"] != nil {
		t.Errorf("empty list observed_at = %v, want null", empty["observed_at"])
	}
}

func TestLocalContractValidationFiltersAndOrder(t *testing.T) {
	s := localContractServer(t)
	localContractBrowse(t, "same", "account")
	localContractBrowse(t, "z", "account-extra")
	localContractDownload(t, "same", "")
	for _, input := range []listLocalVideosInput{
		{PageSize: -1}, {PageSize: 51},
		{StartDate: "2026-11-03", EndDate: "2026-11-01"},
		{StartDate: "2026-11-02T00:00:00+08:00", EndDate: "2026-11-01T00:00:00+08:00"},
		{StartDate: "2026-11-03T00:00:00Z", EndDate: "2026-11-01"},
	} {
		_, _, err := s.listLocalVideos(context.Background(), nil, input)
		if err == nil || !strings.Contains(err.Error(), "invalid_argument:") {
			t.Errorf("input %+v: error = %v", input, err)
		}
	}
	for _, tc := range []struct {
		input string
		want  int64
	}{
		{`{"status":"stored"}`, 2},
		{`{"status":"completed"}`, 1},
		{`{"author_id":"account"}`, 1},
		{`{"author_id":"account","source":"download"}`, 0},
		{`{"author_id":"acc%"}`, 0},
		{`{"start_date":"2026-11-01","end_date":"2026-11-01"}`, 3},
	} {
		var input listLocalVideosInput
		if err := json.Unmarshal([]byte(tc.input), &input); err != nil {
			t.Fatal(err)
		}
		_, result, err := s.listLocalVideos(context.Background(), nil, input)
		if err != nil {
			t.Fatal(err)
		}
		if got := result["data"].(map[string]any)["total"]; got != tc.want {
			t.Errorf("filter %s: total = %v, want %d", tc.input, got, tc.want)
		}
	}
	var got []string
	cursor := ""
	for i := 0; i < 3; i++ {
		_, result, err := s.listLocalVideos(context.Background(), nil, listLocalVideosInput{PageSize: 1, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		data := result["data"].(map[string]any)
		item := data["items"].([]map[string]any)[0]
		got = append(got, item["source"].(string)+":"+item["id"].(string))
		cursor = data["next_cursor"].(string)
	}
	if strings.Join(got, ",") != "browse:z,download:same,browse:same" {
		t.Errorf("union order = %v", got)
	}
}

func TestLocalContractCancellationAndNullColumns(t *testing.T) {
	s := localContractServer(t)
	localContractBrowse(t, "browse", "")
	localContractDownload(t, "download", "")
	if _, err := database.GetDB().Exec("UPDATE browse_history SET like_count = NULL, comment_count = NULL, duration = NULL, size = NULL"); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.listLocalVideos(context.Background(), nil, listLocalVideosInput{})
	if err != nil {
		t.Errorf("nullable historical columns: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = s.listLocalVideos(ctx, nil, listLocalVideosInput{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("list cancellation = %v", err)
	}
	_, _, err = s.getLocalVideo(ctx, nil, getLocalVideoInput{ID: "download"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("detail cancellation = %v", err)
	}
	_, _, err = s.getLocalMediaMetadata(ctx, nil, localMediaMetadataInput{RecordID: "download"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("metadata cancellation = %v", err)
	}
}

func TestLocalContractBoundedPagesPreserveEveryRecord(t *testing.T) {
	s := localContractServer(t)
	large := strings.Repeat("界\x00", 9000)
	for i := 0; i < 18; i++ {
		id := fmt.Sprintf("video-%02d", i)
		localContractBrowse(t, id, "account")
		if _, err := database.GetDB().Exec("UPDATE browse_history SET title = ?, author = ?, resolution = ? WHERE id = ?", large, large, large, id); err != nil {
			t.Fatal(err)
		}
	}
	cursor := ""
	seen := map[string]bool{}
	byteLimited := false
	for page := 0; page < 20; page++ {
		_, result, err := s.listLocalVideos(context.Background(), nil, listLocalVideosInput{PageSize: 50, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if size := len(localContractJSON(t, result)); size > 256<<10 {
			t.Fatalf("page size = %d, exceeds 256 KiB", size)
		}
		if size := localContractWireSize(t, result); size > 256<<10 {
			t.Fatalf("full tool result size = %d, exceeds 256 KiB", size)
		}
		data := result["data"].(map[string]any)
		items := data["items"].([]map[string]any)
		if len(items) == 0 {
			t.Fatal("nonprogressing empty page")
		}
		for _, item := range items {
			id := item["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate record %q", id)
			}
			seen[id] = true
			for key, value := range item {
				if text, ok := value.(string); ok && (!utf8.ValidString(text) || utf8.RuneCountInString(text) > 4096) {
					t.Fatalf("field %s is not bounded valid UTF-8", key)
				}
			}
		}
		if data["has_more"] == false {
			break
		}
		byteLimited = true
		next := data["next_cursor"].(string)
		offset, err := decodeCursor(next)
		if err != nil || offset != len(seen) {
			t.Fatalf("next offset = %d, seen = %d, error = %v", offset, len(seen), err)
		}
		cursor = next
	}
	if len(seen) != 18 || !byteLimited {
		t.Errorf("pagination saw %d records, byte-limited = %v", len(seen), byteLimited)
	}
	_, detail, err := s.getLocalVideo(context.Background(), nil, getLocalVideoInput{ID: "video-00", Source: "browse"})
	if err != nil {
		t.Fatal(err)
	}
	if localContractWireSize(t, detail) > 256<<10 || utf8.RuneCountInString(detail["data"].(map[string]any)["title"].(string)) > 4096 {
		t.Error("detail is not bounded")
	}
}

func TestLocalContractMediaAllowlistAndSafeText(t *testing.T) {
	s := localContractServer(t)
	for _, name := range []string{"video.mp4", "sound.mp3", "cover.png", ".wx_channel_mcp_token", "records.db", "config.yaml", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(s.deps.DownloadsDir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := s.getLocalMediaMetadata(context.Background(), nil, localMediaMetadataInput{RelativePath: name})
		media := strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, ".png")
		if media && err != nil || !media && err == nil {
			t.Errorf("metadata %q: %v", name, err)
		}
	}
	for i, path := range []string{"config.yaml", "../outside.mp4", `C:\private\video.mp4`} {
		id := fmt.Sprintf("unsafe-%d", i)
		localContractDownload(t, id, path)
		_, detail, err := s.getLocalVideo(context.Background(), nil, getLocalVideoInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if detail["data"].(map[string]any)["file"] != nil {
			t.Errorf("unsafe file reference exposed for %q", path)
		}
	}
	localContractDownload(t, "sensitive-text", "")
	if _, err := database.GetDB().Exec("UPDATE download_records SET title = ?, author = ?, resolution = ? WHERE id = ?", `failed C:\Users\private\clip.mp4`, "/home/private/author", `\\server\private\file`, "sensitive-text"); err != nil {
		t.Fatal(err)
	}
	_, detail, err := s.getLocalVideo(context.Background(), nil, getLocalVideoInput{ID: "sensitive-text"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(localContractJSON(t, detail)), "private") {
		t.Error("absolute path text was exposed")
	}
	for _, title := range []string{"路径 /home/用户/私密/视频.mp4", "[/home/用户/私密/视频.mp4]", "https://example.com/video", "/topic"} {
		if _, err := database.GetDB().Exec("UPDATE download_records SET title = ? WHERE id = ?", title, "sensitive-text"); err != nil {
			t.Fatal(err)
		}
		_, detail, err := s.getLocalVideo(context.Background(), nil, getLocalVideoInput{ID: "sensitive-text"})
		if err != nil {
			t.Fatal(err)
		}
		got := detail["data"].(map[string]any)["title"].(string)
		if strings.Contains(title, "私密") && strings.Contains(got, "私密") {
			t.Errorf("non-ASCII absolute path exposed: %s", got)
		}
		if (strings.HasPrefix(title, "https:") || title == "/topic") && got != title {
			t.Errorf("ordinary URL title was redacted: %s", got)
		}
	}
}
