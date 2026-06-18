package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"wx_channel/internal/utils"
)

const commentExportCheckpointSuffix = ".partial.json"

type commentExportPersistence struct {
	downloadsDir string
	saveDir      string
	savePath     string
	relativePath string
}

func newCommentExportPersistence(downloadsDir string, req ExportFeedCommentsRequest) (*commentExportPersistence, error) {
	saveDir := filepath.Join(downloadsDir, "comment_data", time.Now().Format("2006-01-02"))
	if err := utils.EnsureDir(saveDir); err != nil {
		return nil, err
	}

	filenameBase := utils.CleanFilename(req.Title)
	if filenameBase == "" {
		filenameBase = "comments_" + req.ObjectID
	}

	savePath := utils.GenerateUniqueFilename(saveDir, utils.EnsureExtension(filenameBase, ".json"), 100)
	relativePath, _ := filepath.Rel(downloadsDir, savePath)

	return &commentExportPersistence{
		downloadsDir: downloadsDir,
		saveDir:      saveDir,
		savePath:     savePath,
		relativePath: relativePath,
	}, nil
}

func (p *commentExportPersistence) SaveCheckpoint(req ExportFeedCommentsRequest, comments []map[string]interface{}, reportedCount, replyCount int, source string) error {
	payload := buildCommentExportFile(req, comments, reportedCount, replyCount, source)
	checkpointPath := p.savePath + commentExportCheckpointSuffix
	return writeCommentExportFile(checkpointPath, payload)
}

func (p *commentExportPersistence) Finalize(req ExportFeedCommentsRequest, comments []map[string]interface{}, reportedCount, replyCount int, source string) (string, string, error) {
	payload := buildCommentExportFile(req, comments, reportedCount, replyCount, source)
	if err := writeCommentExportFile(p.savePath, payload); err != nil {
		return "", "", err
	}

	checkpointPath := p.savePath + commentExportCheckpointSuffix
	if err := os.Remove(checkpointPath); err != nil && !os.IsNotExist(err) {
		return "", "", err
	}

	return p.savePath, p.relativePath, nil
}

func buildCommentExportFile(req ExportFeedCommentsRequest, comments []map[string]interface{}, reportedCount, replyCount int, source string) commentExportFile {
	return commentExportFile{
		ObjectID:             req.ObjectID,
		ObjectNonceID:        req.NonceID,
		Title:                req.Title,
		Author:               req.Author,
		CommentInfo:          formatCommentsForExport(comments),
		CountInfo:            feedCommentExportCountInfo{CommentCount: len(comments)},
		LastBuffer:           "",
		UpContinueFlag:       0,
		DownContinueFlag:     0,
		OriginalCommentCount: reportedCount,
		SavedAt:              time.Now().Format(time.RFC3339),
		Source:               source,
	}
}

func writeCommentExportFile(savePath string, payload commentExportFile) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := utils.BuildTempDownloadPath(savePath, "comment-export")
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	if err := os.Remove(savePath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, savePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
