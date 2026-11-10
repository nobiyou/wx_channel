package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"wx_channel/internal/handlers"
)

type openedLocalMedia struct {
	file         *os.File
	info         os.FileInfo
	relativePath string
	mimeType     string
	record       *storedRecord
}

// Both header probing and frame extraction use this confined handle. Neither
// decoder may reopen a caller-supplied source path after validation.
func (s *Server) openLocalMedia(ctx context.Context, input localMediaMetadataInput) (*openedLocalMedia, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	baseDir, err := filepath.Abs(s.deps.DownloadsDir)
	if err != nil {
		return nil, errors.New("local_file_unavailable: cannot resolve downloads directory")
	}
	var record *storedRecord
	rawPath := strings.TrimSpace(input.RelativePath)
	if strings.TrimSpace(input.RecordID) != "" {
		record, err = s.findStoredRecord(ctx, strings.TrimSpace(input.Source), strings.TrimSpace(input.RecordID))
		if err != nil {
			return nil, err
		}
		rawPath = record.FilePath
	} else {
		if rawPath == "" {
			return nil, errors.New("invalid_argument: relative_path or record_id is required")
		}
		if filepath.IsAbs(rawPath) || filepath.VolumeName(rawPath) != "" || strings.HasPrefix(rawPath, `\`) {
			return nil, errors.New("invalid_argument: relative_path must stay relative to the downloads directory")
		}
	}
	if rawPath == "" {
		return nil, errors.New("local_file_unavailable: the record has no local file path")
	}
	candidate := rawPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseDir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return nil, errors.New("invalid_argument: invalid local file path")
	}
	relative, err := filepath.Rel(baseDir, candidate)
	if err != nil || !isSafeRelativePath(relative) {
		return nil, errors.New("path_forbidden: the media path is outside the configured downloads directory")
	}
	if record == nil {
		record, err = s.findDownloadRecordByPath(ctx, relative)
		if err != nil {
			return nil, err
		}
	}
	if !isLocalMediaPath(candidate) || utf8.RuneCountInString(relative) > maxFieldRunes {
		return nil, errors.New("path_forbidden: only video, audio, and image files are available")
	}
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("local_file_not_found: the referenced video file does not exist")
		}
		return nil, errors.New("local_file_unavailable: cannot access local media")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("invalid_argument: the selected local path is not a regular file")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, resolved, err := handlers.OpenFileInBase(baseDir, candidate)
	if err != nil {
		return nil, errors.New("path_forbidden: the media path is outside the configured downloads directory")
	}
	if !isLocalMediaPath(resolved) {
		_ = file.Close()
		return nil, errors.New("path_forbidden: only video, audio, and image files are available")
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("local_file_unavailable: cannot inspect local media")
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &openedLocalMedia{file: file, info: info, relativePath: filepath.ToSlash(relative), mimeType: mediaMIMEType(candidate), record: record}, nil
}
