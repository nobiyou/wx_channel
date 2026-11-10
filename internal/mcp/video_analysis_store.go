package mcpserver

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
)

const maxAnalysisManifestBytes = 16 << 20

func analysisStorageError() error {
	return errors.New("analysis_artifact_unavailable: cannot access analysis artifacts")
}

// Pin every directory and reject symlinks/junctions outside downloads, for
// writes as well as reads. Never reopen untrusted paths from a manifest.
func (s *Server) analysisRoot(id string, create bool) (*os.Root, error) {
	if _, err := uuid.Parse(id); err != nil || strings.ContainsAny(id, `/\`) {
		return nil, errors.New("invalid_argument: analysis_id must be a UUID")
	}
	root, err := os.OpenRoot(s.deps.DownloadsDir)
	if err != nil {
		return nil, analysisStorageError()
	}
	defer root.Close()
	if create {
		if err := root.Mkdir(analysisArtifactDir, 0700); err != nil && !os.IsExist(err) {
			return nil, analysisStorageError()
		}
	}
	store, err := root.OpenRoot(analysisArtifactDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("analysis_not_found: analysis does not exist")
		}
		return nil, analysisStorageError()
	}
	defer store.Close()
	if create {
		if err := store.Mkdir(id, 0700); err != nil {
			return nil, analysisStorageError()
		}
	}
	job, err := store.OpenRoot(id)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("analysis_not_found: analysis does not exist")
		}
		return nil, analysisStorageError()
	}
	if create {
		if err := job.Mkdir("shots", 0700); err != nil {
			job.Close()
			return nil, analysisStorageError()
		}
	}
	return job, nil
}

func readAnalysisFile(root *os.Root, name string, limit int64) ([]byte, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, analysisStorageError()
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, analysisStorageError()
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, analysisStorageError()
	}
	return data, nil
}

func readAnalysisKeyframe(root *os.Root, id, reference string) ([]byte, error) {
	prefix := analysisReference(id, "") + "/"
	if !strings.HasPrefix(reference, prefix) {
		return nil, analysisStorageError()
	}
	name := strings.TrimPrefix(reference, prefix)
	if !isSafeRelativePath(name) || !strings.HasPrefix(name, "shots/") {
		return nil, analysisStorageError()
	}
	data, err := readAnalysisFile(root, name, maxFrameJPEGBytes)
	if err != nil {
		return nil, err
	}
	if err := validateFrameJPEG(data, analysisSampleWidth); err != nil {
		return nil, analysisStorageError()
	}
	return data, nil
}

func writeAnalysisFile(root *os.Root, name string, data []byte) error {
	// O_NONBLOCK and post-open type checks reject FIFOs and other special files.
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return analysisStorageError()
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return analysisStorageError()
	}
	if err := file.Truncate(0); err != nil {
		return analysisStorageError()
	}
	if _, err := file.Write(data); err != nil {
		return analysisStorageError()
	}
	if err := file.Sync(); err != nil {
		return analysisStorageError()
	}
	return nil
}

func (s *Server) saveAnalysisManifestLocked(m videoAnalysisManifest) error {
	m.AnalysisProvider = analysisProvider(m)
	m.ReviewStatus = analysisReviewStatus(m)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil || len(data) > maxAnalysisManifestBytes {
		return analysisStorageError()
	}
	root, err := s.analysisRoot(m.AnalysisID, false)
	if err != nil {
		return err
	}
	defer root.Close()
	return writeAnalysisFile(root, "manifest.json", data)
}

func (s *Server) updateAnalysisJob(job *videoAnalysisJob, update func(*videoAnalysisManifest)) error {
	s.analysisMu.Lock()
	defer s.analysisMu.Unlock()
	job.mu.Lock()
	defer job.mu.Unlock()
	update(&job.Manifest)
	return s.saveAnalysisManifestLocked(job.Manifest)
}

func (s *Server) loadAnalysisJob(id string) (*videoAnalysisJob, error) {
	s.analysisMu.Lock()
	defer s.analysisMu.Unlock()
	return s.loadAnalysisJobLocked(strings.TrimSpace(id))
}

func (s *Server) loadAnalysisJobLocked(id string) (*videoAnalysisJob, error) {
	root, err := s.analysisRoot(id, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	raw, err := readAnalysisFile(root, "manifest.json", maxAnalysisManifestBytes)
	if err != nil {
		return nil, err
	}
	var m videoAnalysisManifest
	if err := json.Unmarshal(raw, &m); err != nil || m.AnalysisID != id || len(m.Shots) > maxAnalysisShots || !isSafeRelativePath(m.RelativePath) {
		return nil, analysisStorageError()
	}
	if m.Status == "running" || m.Status == "queued" {
		if _, active := s.analysisJobs[id]; !active {
			m.Status = "interrupted"
			m.Error = "analysis_interrupted"
			if err := s.saveAnalysisManifestLocked(m); err != nil {
				return nil, err
			}
		}
	}
	m.ReportJSON = analysisReference(id, "report.json")
	m.ReportHTML = analysisReference(id, "report.html")
	for i := range m.Shots {
		if m.Shots[i].Index != i+1 || len(m.Shots[i].Keyframes) > len(analysisKeyframeFractions) {
			return nil, analysisStorageError()
		}
	}
	ensureVideoAnalysisDetails(&m)
	m.AnalysisProvider = analysisProvider(m)
	m.ReviewStatus = analysisReviewStatus(m)
	if !m.UseExternalModel {
		m.ModelStatus = "not_requested"
	}
	return &videoAnalysisJob{ID: id, RelativePath: m.RelativePath, RecordID: m.RecordID, Source: m.RecordSource, Mode: m.Mode, Manifest: m}, nil
}

func analysisReference(id, name string) string {
	return filepath.ToSlash(filepath.Join(analysisArtifactDir, id, name))
}

func analysisJobSummary(m videoAnalysisManifest) map[string]any {
	reviewStatus := analysisReviewStatus(m)
	nextTool, nextShot := "get_video_analysis_report", 0
	if reviewStatus == "preparing" {
		nextTool = "get_video_analysis_status"
	}
	if reviewStatus == "awaiting_codex_review" || reviewStatus == "partial" {
		nextTool = "get_video_analysis_shots"
		for _, shot := range m.Shots {
			if shot.Review == nil && shot.Vision == nil {
				nextShot = shot.Index
				break
			}
		}
	}
	if reviewStatus == "unavailable" {
		nextTool = ""
	}
	return map[string]any{"analysis_id": m.AnalysisID, "record_id": m.RecordID, "relative_path": m.RelativePath, "status": m.Status, "mode": m.Mode, "analysis_provider": analysisProvider(m), "review_status": reviewStatus, "next_tool": nextTool, "next_unreviewed_shot": nextShot, "progress": m.Progress, "duration_seconds": m.DurationSeconds, "sample_count": m.SampleCount, "shot_count": len(m.Shots), "model_status": m.ModelStatus, "model": m.Model, "review_count": m.ReviewCount, "video_details": m.VideoDetails, "local_enrichment_status": m.LocalEnrichment.Status, "local_missing_reasons": m.LocalEnrichment.MissingReasons, "report_json": m.ReportJSON, "report_html": m.ReportHTML, "error": m.Error, "updated_at": m.UpdatedAt, "warnings": m.Warnings}
}

func analysisProvider(m videoAnalysisManifest) string {
	if m.Mode == "local" {
		return "local"
	}
	if !m.UseExternalModel {
		return "codex"
	}
	if m.AnalysisProvider != "" {
		return m.AnalysisProvider
	}
	return "external"
}

// Local preprocessing completion does not mean the Codex session has reviewed
// the images. A saved Codex review or external vision result completes a shot.
func analysisReviewStatus(m videoAnalysisManifest) string {
	if m.Mode == "local" {
		return "not_requested"
	}
	if m.Status != "completed" {
		if m.Status == "failed" || m.Status == "interrupted" {
			return "unavailable"
		}
		return "preparing"
	}
	reviewed := 0
	for _, shot := range m.Shots {
		if shot.Review != nil || shot.Vision != nil {
			reviewed++
		}
	}
	if reviewed > 0 && reviewed == len(m.Shots) {
		return "completed"
	}
	if reviewed > 0 {
		return "partial"
	}
	return "awaiting_codex_review"
}
