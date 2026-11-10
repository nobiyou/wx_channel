package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"image/jpeg"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"wx_channel/internal/config"
	"wx_channel/internal/database"
)

type analysisFixtureDownloadEngine struct{ payload []byte }

func (e *analysisFixtureDownloadEngine) DownloadSync(ctx context.Context, _, target string, _ int, _ map[string]string, _ func(float64, int64, int64)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return "", err
	}
	return target, os.WriteFile(target, e.payload, 0600)
}

func TestAnalysisRejectsUnconfinedID(t *testing.T) {
	s := &Server{deps: Dependencies{DownloadsDir: t.TempDir()}}
	for _, id := range []string{"../other", `..\other`, "nested/id", "C:/data", ".", "not-a-uuid"} {
		if _, err := s.loadAnalysisJob(id); err == nil || !strings.HasPrefix(err.Error(), "invalid_argument:") {
			t.Errorf("id %q: %v", id, err)
		}
	}
}

func TestAnalysisRestartDoesNotLeaveJobRunning(t *testing.T) {
	s, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())
	id := "8d1ab3f0-87bd-4b76-820d-e2f6ea62e606"
	dir := filepath.Join(s.deps.DownloadsDir, analysisArtifactDir, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(videoAnalysisManifest{AnalysisID: id, Status: "running", RelativePath: "sample.mp4"})
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	job, err := s.loadAnalysisJob(id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Manifest.Status != "interrupted" {
		t.Fatalf("status after restart = %s", job.Manifest.Status)
	}
}

func TestVideoAnalysisRealDownloadToReviewReport(t *testing.T) {
	// Keep the decoder/report integration deterministic. Local ASR/OCR is
	// separately opt-in for this test because model weights may be unavailable
	// or require a lengthy first-time download on CI.
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_LOCAL_ENRICHMENT", "false")
	ffmpeg := os.Getenv("WX_CHANNEL_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("set WX_CHANNEL_TEST_FFMPEG for real decoder validation")
	}
	fixtureFFmpeg := firstNonEmpty(os.Getenv("WX_CHANNEL_TEST_FIXTURE_FFMPEG"), ffmpeg)
	source := frameDecoderFFmpegFixture(t, fixtureFFmpeg, "-f", "lavfi", "-i", "color=c=red:s=160x120:r=10:d=1", "-f", "lavfi", "-i", "color=c=blue:s=160x120:r=10:d=1", "-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v]", "-map", "[v]", "-c:v", "libx264", "-pix_fmt", "yuv420p")
	dir := t.TempDir()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := setupMCPDatabase(t)
	defer cleanup()
	browse := &database.BrowseRecord{ID: "analysis-source", Title: "镜头测试", Author: "测试", VideoURL: "https://finder.video.qq.com/251/20302/stodownload?token=fixture", BrowseTime: time.Now()}
	if err := database.NewBrowseHistoryRepository().Create(browse); err != nil {
		t.Fatal(err)
	}
	downloader := NewLocalVideoDownloader(VideoDownloaderDependencies{Config: &config.Config{}, DownloadsDir: dir, Engine: &analysisFixtureDownloadEngine{payload: data}})
	var modelCalls atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"description\":\"测试画面\",\"evidence\":[\"可见纯色背景\"],\"replication\":{\"shooting_plan\":\"固定机位\"}}"}}]}`))
	}))
	defer modelServer.Close()
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_BASE_URL", modelServer.URL)
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_API_KEY", "test-key")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_MODEL", "test-model")
	t.Setenv("WX_CHANNEL_VIDEO_ANALYSIS_TIMEOUT_SECONDS", "10")
	s, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: dir, FFmpegPath: ffmpeg, VideoDownload: downloader.Download})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())
	// Exercise the independent download action and completed record before analysis.
	deadline := time.Now().Add(15 * time.Second)
	var downloadID string
	for time.Now().Before(deadline) {
		_, out, err := s.downloadVideoForAnalysis(t.Context(), nil, downloadVideoForAnalysisInput{RecordID: browse.ID})
		if err != nil {
			t.Fatal(err)
		}
		item := out["data"].(map[string]any)
		if item["status"] == database.DownloadStatusCompleted {
			downloadID = item["download_record_id"].(string)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if downloadID == "" {
		t.Fatal("download action did not complete")
	}
	_, out, err := s.startVideoAnalysis(context.Background(), videoAnalysisInput{RecordID: downloadID, Mode: "hybrid"})
	if err != nil {
		t.Fatal(err)
	}
	id := out["data"].(map[string]any)["analysis_id"].(string)
	manifest := waitAnalysisFixture(t, s, id)
	if modelCalls.Load() != 0 || manifest.ModelStatus != "not_requested" {
		t.Fatalf("implicit model upload: %d/%s", modelCalls.Load(), manifest.ModelStatus)
	}
	if len(manifest.Shots) != 2 || math.Abs(manifest.Shots[1].Start-1) > 0.12 {
		t.Fatalf("cuts = %+v", manifest.Shots)
	}
	if manifest.SampleCount != 14 || manifest.VideoDetails.Title != "镜头测试" || manifest.VideoDetails.Author != "测试" {
		t.Fatalf("details/sampling missing: %+v", manifest.VideoDetails)
	}
	for _, shot := range manifest.Shots {
		if len(shot.Keyframes) != 7 {
			t.Fatalf("keyframes=%d want 7", len(shot.Keyframes))
		}
		for _, frame := range shot.Keyframes {
			if frame.Timestamp < shot.Start || frame.Timestamp >= shot.End {
				t.Fatalf("frame outside shot: %+v", frame)
			}
			raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(frame.RelativePath)))
			if err != nil {
				t.Fatal(err)
			}
			picture, err := jpeg.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			red, _, blue, _ := picture.At(0, 0).RGBA()
			if (shot.Index == 1 && red <= blue) || (shot.Index == 2 && blue <= red) {
				t.Fatalf("frame at %.3f crossed shot %d boundary", frame.Timestamp, shot.Index)
			}
		}
	}
	images, shotsOut, err := s.getVideoAnalysisShots(context.Background(), nil, videoAnalysisShotsInput{AnalysisID: id, Page: 2, PageSize: 1, IncludeImages: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(shotsOut["data"].(map[string]any)["items"].([]analysisShot)) != 1 {
		t.Fatal("missing shot page")
	}
	imageCount := 0
	for _, content := range images.Content {
		if frame, ok := content.(*mcp.ImageContent); ok {
			imageCount++
			picture, err := jpeg.Decode(bytes.NewReader(frame.Data))
			if err != nil {
				t.Fatal(err)
			}
			red, _, blue, _ := picture.At(0, 0).RGBA()
			if blue <= red {
				t.Fatal("second shot contains a neighboring red frame")
			}
		}
	}
	if imageCount != 3 {
		t.Fatalf("image count = %d, want 3", imageCount)
	}
	path := filepath.Join(dir, filepath.FromSlash(manifest.ReportHTML))
	htmlData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlData), "<img") {
		t.Fatal("HTML report has no frame preview")
	}
	if manifest.LocalEnrichment.Status != "disabled" || !strings.Contains(string(htmlData), "本地增强") {
		t.Fatalf("local enrichment report missing: status=%q", manifest.LocalEnrichment.Status)
	}
	// The public MCP writeback must persist Codex's review in both reports.
	session := analysisTestSession(t, s)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "save_video_analysis_review", Arguments: map[string]any{"analysis_id": id, "shot_index": 1, "description": "红色测试画面", "evidence": []string{"0.1 秒：画面为红色"}, "confidence": 0.8, "shooting_plan": "固定机位，柔光", "video_prompt": "红色背景"}})
	if err != nil || result.IsError {
		t.Fatalf("review writeback: %v %+v", err, result)
	}
	htmlData, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlData), "固定机位") {
		t.Fatal("HTML report did not include the saved review")
	}
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(manifest.ReportJSON)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "固定机位") {
		t.Fatal("JSON report did not include the saved review")
	}
	// A second server can read the completed result without repeating analysis.
	second, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: dir, FFmpegPath: ffmpeg})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Shutdown(context.Background())
	restored, err := second.loadAnalysisJob(id)
	if err != nil || restored.Manifest.Status != "completed" {
		t.Fatalf("restored result: %v", err)
	}
	if restored.Manifest.ReviewCount != 1 {
		t.Fatalf("restored review count = %d", restored.Manifest.ReviewCount)
	}
	_, cached, err := second.startVideoAnalysis(t.Context(), videoAnalysisInput{RecordID: downloadID, Mode: "hybrid"})
	if err != nil || cached["data"].(map[string]any)["analysis_id"] != id {
		t.Fatalf("completed cache not reused: %v, %#v", err, cached)
	}
	_, external, err := s.startVideoAnalysis(t.Context(), videoAnalysisInput{RecordID: downloadID, Mode: "hybrid", UseExternalModel: true})
	if err != nil {
		t.Fatal(err)
	}
	externalManifest := waitAnalysisFixture(t, s, external["data"].(map[string]any)["analysis_id"].(string))
	if modelCalls.Load() != 2 || externalManifest.ModelStatus != "completed" {
		t.Fatalf("explicit model run = %d/%s", modelCalls.Load(), externalManifest.ModelStatus)
	}
}

func waitAnalysisFixture(t *testing.T, s *Server, id string) videoAnalysisManifest {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		job, err := s.loadAnalysisJob(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Manifest.Status == "completed" {
			return job.Manifest
		}
		if job.Manifest.Status == "failed" || job.Manifest.Status == "interrupted" {
			t.Fatalf("analysis failed: %s/%s", job.Manifest.Status, job.Manifest.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("analysis did not finish")
	return videoAnalysisManifest{}
}

func analysisTestSession(t *testing.T, s *Server) *mcp.ClientSession {
	t.Helper()
	server, client := mcp.NewInMemoryTransports()
	serverSession, err := s.mcpServer.Connect(context.Background(), server, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	c := mcp.NewClient(&mcp.Implementation{Name: "analysis-test", Version: "test"}, nil)
	session, err := c.Connect(context.Background(), client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}
