package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wx_channel/internal/database"
)

const (
	maxBackgroundDownloadDuration = 30 * time.Minute
	maxQueuedDownloads            = 4
)

func (s *Server) downloadVideoForAnalysis(ctx context.Context, _ *mcp.CallToolRequest, input downloadVideoForAnalysisInput) (*mcp.CallToolResult, map[string]any, error) {
	recordID := strings.TrimSpace(input.RecordID)
	if recordID == "" || len(recordID) > 512 {
		return nil, nil, errors.New("invalid_argument: record_id is required")
	}
	if s.deps.VideoDownload == nil {
		return nil, nil, failure("download_unavailable")
	}
	if database.GetDB() == nil {
		return nil, nil, failure("local_database_unavailable")
	}

	browse, err := database.NewBrowseHistoryRepository().GetByID(recordID)
	if err != nil {
		return nil, nil, errors.New("local_database_query_failed: cannot read the source record")
	}
	if browse == nil {
		return nil, nil, failure("download_record_not_found")
	}
	if !isDownloadableStoredURL(strings.TrimSpace(browse.VideoURL)) {
		return nil, nil, failure("download_source_unavailable")
	}

	downloadRecord, err := database.NewDownloadRecordRepository().GetByVideoID(recordID)
	if err != nil {
		return nil, nil, errors.New("local_database_query_failed: cannot read the download record")
	}
	if completed, ok := s.completedDownloadResult(recordID, downloadRecord); ok {
		return nil, envelope("local_file", "stored", completed, nil), nil
	}

	s.downloadMu.Lock()
	_, active := s.activeDownloads[recordID]
	if !active && len(s.activeDownloads) >= maxQueuedDownloads {
		s.downloadMu.Unlock()
		return nil, nil, failure("busy")
	}
	if !active {
		s.activeDownloads[recordID] = struct{}{}
	}
	s.downloadMu.Unlock()
	if active {
		downloadID := recordID
		if existingID := downloadRecordID(downloadRecord); existingID != "" {
			downloadID = existingID
		}
		return nil, envelope("local_download", "live", map[string]any{
			"record_id": recordID, "download_record_id": downloadID,
			"status": database.DownloadStatusInProgress, "started": false, "skipped": false,
		}, nil), nil
	}

	downloadID := recordID
	if existingID := downloadRecordID(downloadRecord); existingID != "" {
		downloadID = existingID
	}
	go s.runVideoDownload(recordID)
	return nil, envelope("local_download", "live", map[string]any{
		"record_id": recordID, "download_record_id": downloadID,
		"status": database.DownloadStatusInProgress, "started": true, "skipped": false,
	}, nil), nil
}

func (s *Server) runVideoDownload(recordID string) {
	defer func() {
		s.downloadMu.Lock()
		delete(s.activeDownloads, recordID)
		s.downloadMu.Unlock()
	}()

	select {
	case s.downloadSlots <- struct{}{}:
		defer func() { <-s.downloadSlots }()
	case <-s.downloadCtx.Done():
		s.markActionDownloadFailed(recordID)
		return
	}

	ctx, cancel := context.WithTimeout(s.downloadCtx, maxBackgroundDownloadDuration)
	defer cancel()
	_, err := s.deps.VideoDownload(ctx, VideoDownloadRequest{RecordID: recordID})
	if err != nil {
		s.markActionDownloadFailed(recordID)
	}
}

func (s *Server) completedDownloadResult(recordID string, record *database.DownloadRecord) (map[string]any, bool) {
	if record == nil || record.Status != database.DownloadStatusCompleted {
		return nil, false
	}
	file, ok := s.safeFileReference(record.FilePath)
	if !ok || file["exists"] != true {
		return nil, false
	}
	warnings := []string{}
	title := record.Title
	if localAbsolutePath.MatchString(title) {
		title = "[redacted_local_path]"
		warnings = appendLocalWarnings(warnings, "stored_text_redacted")
	}
	data := map[string]any{
		"record_id": recordID, "download_record_id": record.ID,
		"title": boundedText(title, &warnings), "status": database.DownloadStatusCompleted,
		"started": false, "skipped": true, "file": file,
	}
	if size, ok := file["size_bytes"].(int64); ok {
		data["size_bytes"] = size
	}
	if len(warnings) > 0 {
		data["warnings"] = warnings
	}
	return data, true
}

func downloadRecordID(record *database.DownloadRecord) string {
	if record == nil {
		return ""
	}
	return record.ID
}

func (s *Server) markActionDownloadFailed(recordID string) {
	repo := database.NewDownloadRecordRepository()
	record, err := repo.GetByVideoID(recordID)
	if err != nil || record == nil || record.Status == database.DownloadStatusCompleted {
		return
	}
	record.Status = database.DownloadStatusFailed
	record.FilePath = ""
	record.FileSize = 0
	record.ErrorMessage = "download_failed"
	_ = repo.Update(record)
}
