package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"wx_channel/internal/config"
)

func completedAnalysisFixture(t *testing.T) (*Server, string) {
	t.Helper()
	s, err := NewServer(Dependencies{Config: &config.Config{Port: 2025}, DownloadsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	id := "8d1ab3f0-87bd-4b76-820d-e2f6ea62e606"
	root, err := s.analysisRoot(id, true)
	if err != nil {
		t.Fatal(err)
	}
	root.Close()
	m := videoAnalysisManifest{AnalysisID: id, Status: "completed", RelativePath: "sample.mp4", Shots: []analysisShot{{Index: 1, Start: 0, End: 1, Duration: 1, Replication: map[string]any{"status": "pending_visual_review"}}}}
	if err := s.saveAnalysisManifestLocked(m); err != nil {
		t.Fatal(err)
	}
	return s, id
}

func TestAnalysisConcurrentReviewAndReportReads(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	_, _, err := s.saveVideoAnalysisReview(t.Context(), videoAnalysisReviewInput{AnalysisID: id, ShotIndex: 1, Description: "initial"})
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for i := 0; i < 20; i++ {
			_, _, err := s.saveVideoAnalysisReview(t.Context(), videoAnalysisReviewInput{AnalysisID: id, ShotIndex: 1, Description: strings.Repeat("review", 400)})
			if err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer group.Done()
		for i := 0; i < 40; i++ {
			_, out, err := s.getVideoAnalysisReport(t.Context(), nil, videoAnalysisArtifactInput{AnalysisID: id, Format: "json"})
			if err != nil {
				t.Error(err)
				return
			}
			raw := out["data"].(map[string]any)["content"].(string)
			var report videoAnalysisManifest
			if err := json.Unmarshal([]byte(raw), &report); err != nil {
				t.Error(err)
				return
			}
			if report.ReviewCount != 1 || report.Shots[0].Review == nil {
				t.Errorf("partial report: %#v", report)
				return
			}
		}
	}()
	group.Wait()
}

func TestAnalysisPaginationHandlesMaximumInteger(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	_, out, err := s.getVideoAnalysisShots(context.Background(), nil, videoAnalysisShotsInput{AnalysisID: id, Page: int(^uint(0) >> 1), PageSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	data := out["data"].(map[string]any)
	if len(data["items"].([]analysisShot)) != 0 || data["has_more"] != false {
		t.Fatalf("page = %#v", data)
	}
}

func TestAnalysisReviewUpdatesUniqueShotCountAndCompletion(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	for _, description := range []string{"first", "updated <script>alert(1)</script>"} {
		_, _, err := s.saveVideoAnalysisReview(context.Background(), videoAnalysisReviewInput{AnalysisID: id, ShotIndex: 1, Description: description, ShootingPlan: "固定机位"})
		if err != nil {
			t.Fatal(err)
		}
	}
	job, err := s.loadAnalysisJob(id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Manifest.ReviewCount != 1 {
		t.Fatalf("reviewed shots = %d, want 1", job.Manifest.ReviewCount)
	}
	if job.Manifest.Shots[0].Replication["status"] != "reviewed" {
		t.Fatalf("replication = %#v", job.Manifest.Shots[0].Replication)
	}
	_, out, err := s.getVideoAnalysisReport(context.Background(), nil, videoAnalysisArtifactInput{AnalysisID: id, Format: "html"})
	if err != nil {
		t.Fatal(err)
	}
	content := out["data"].(map[string]any)["content"].(string)
	if strings.Contains(content, "<script>") || !strings.Contains(content, "固定机位") {
		t.Fatalf("HTML did not escape review or omitted plan: %s", content)
	}
}

func TestAnalysisReviewBoundsEvidenceAndText(t *testing.T) {
	values := make([]string, 100)
	for i := range values {
		values[i] = fmt.Sprintf("evidence %d", i)
	}
	if got := boundReviewStrings(values); len(got) > 20 {
		t.Fatalf("evidence count = %d", len(got))
	}
	s, id := completedAnalysisFixture(t)
	_, _, err := s.saveVideoAnalysisReview(context.Background(), videoAnalysisReviewInput{AnalysisID: id, ShotIndex: 1, Description: strings.Repeat("字", maxModelValueRunes+1)})
	if err == nil {
		t.Fatal("accepted oversized review")
	}
}

func TestAnalysisLargeReportReturnsCompleteArtifactReference(t *testing.T) {
	s, id := completedAnalysisFixture(t)
	root, err := s.analysisRoot(id, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := writeAnalysisFile(root, "report.html", []byte(strings.Repeat("内容", 20000))); err != nil {
		t.Fatal(err)
	}
	_, out, err := s.getVideoAnalysisReport(context.Background(), nil, videoAnalysisArtifactInput{AnalysisID: id, Format: "html"})
	if err != nil {
		t.Fatal(err)
	}
	data := out["data"].(map[string]any)
	if data["content_omitted"] != true || data["artifact_relative_path"] != analysisReference(id, "report.html") || data["content"] != nil {
		t.Fatalf("large report = %#v", data)
	}
	if size, err := serializedToolResultSize(out); err != nil || size > maxToolResponseBytes {
		t.Fatalf("wire size = %d/%v", size, err)
	}
}
