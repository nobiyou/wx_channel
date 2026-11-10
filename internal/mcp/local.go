package mcpserver

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"wx_channel/internal/database"
	"wx_channel/internal/handlers"
)

// Only recognize clear local paths; a slash in a title (e.g. /topic) is ordinary text.
var localAbsolutePath = regexp.MustCompile(`(?i)\b[a-z]:[\\/]|\\\\|(?:^|[\s"'=:(\[{])/(?:home|users|tmp|var|etc|private|root|mnt|media|volumes)/`)

type storedRecord struct {
	MetricsSnapshot string
	Source          string
	ID              string
	VideoID         string
	Title           string
	Author          string
	AuthorID        string
	CoverURL        string
	Duration        int64
	SizeBytes       int64
	FileSize        int64
	FilePath        string
	Format          string
	Resolution      string
	Status          string
	ObservedAt      time.Time
	LikeCount       int64
	CommentCount    int64
	FavCount        int64
	ForwardCount    int64
	ErrorMessage    string
}

func (s *Server) listLocalVideos(ctx context.Context, _ *mcp.CallToolRequest, input listLocalVideosInput) (*mcp.CallToolResult, map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	db := database.GetDB()
	if db == nil {
		return nil, nil, errors.New("local_database_unavailable: the wx_channel database is not initialized")
	}
	query, args, err := buildLocalUnionQuery(input)
	if err != nil {
		return nil, nil, err
	}
	offset, err := decodeCursor(input.Cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid_argument: cursor is invalid: %w", err)
	}
	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize < 0 || pageSize > 50 {
		return nil, nil, errors.New("invalid_argument: page_size must be between 1 and 50")
	}

	countQuery := "SELECT COUNT(*) FROM (" + query + ") AS local_records"
	var total int64
	if err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, nil, localQueryError(ctx, err)
	}
	rowsQuery := "SELECT * FROM (" + query + ") AS local_records ORDER BY observed_at DESC, id DESC, source DESC LIMIT ? OFFSET ?"
	rowsArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := db.QueryContext(ctx, rowsQuery, rowsArgs...)
	if err != nil {
		return nil, nil, localQueryError(ctx, err)
	}
	defer rows.Close()

	items := make([]map[string]any, 0, pageSize)
	warnings := []string{}
	pageResult := func(pageItems []map[string]any, pageWarnings []string) map[string]any {
		nextCursor := ""
		if int64(len(pageItems)) < total-int64(offset) {
			nextCursor = encodeCursor(offset + len(pageItems))
		}
		data := map[string]any{"items": pageItems, "total": total, "page_size": pageSize, "next_cursor": nextCursor, "has_more": nextCursor != ""}
		result := envelope("local_database", "stored", data, pageWarnings)
		result["observed_at"] = nil
		if len(pageItems) != 0 {
			result["observed_at"] = pageItems[0]["observed_at"]
		}
		return result
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		record, err := scanStoredRecord(rows)
		if err != nil {
			return nil, nil, localQueryError(ctx, err)
		}
		item := s.recordMap(*record)
		nextItems := append(items, item)
		nextWarnings := appendLocalWarnings(append([]string{}, warnings...), item["warnings"].([]string)...)
		// Include the limit warning now so stopping at the next record cannot
		// push an accepted page over the wire bound.
		budgetWarnings := appendLocalWarnings(append([]string{}, nextWarnings...), "response_page_size_limited")
		size, err := serializedToolResultSize(pageResult(nextItems, budgetWarnings))
		if err != nil {
			return nil, nil, errors.New("local_response_unavailable: cannot encode local response")
		}
		if size > maxToolResponseBytes {
			if len(items) == 0 {
				return nil, nil, errors.New("response_too_large: a local record exceeds the response limit")
			}
			warnings = appendLocalWarnings(warnings, "response_page_size_limited")
			break
		}
		items, warnings = nextItems, nextWarnings
	}
	if err := rows.Err(); err != nil {
		return nil, nil, localQueryError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	return boundedLocalResult(pageResult(items, warnings))
}

func (s *Server) getLocalVideo(ctx context.Context, _ *mcp.CallToolRequest, input getLocalVideoInput) (*mcp.CallToolResult, map[string]any, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return nil, nil, errors.New("invalid_argument: id is required")
	}
	record, err := s.findStoredRecord(ctx, strings.TrimSpace(input.Source), id)
	if err != nil {
		return nil, nil, err
	}
	data := s.recordMap(*record)
	result := envelope("local_database", "stored", data, data["warnings"].([]string))
	result["observed_at"] = data["observed_at"]
	return boundedLocalResult(result)
}

func (s *Server) getLocalMediaMetadata(ctx context.Context, _ *mcp.CallToolRequest, input localMediaMetadataInput) (*mcp.CallToolResult, map[string]any, error) {
	media, err := s.openLocalMedia(ctx, input)
	if err != nil {
		return nil, nil, err
	}
	defer media.file.Close()
	file, info, record := media.file, media.info, media.record
	data := map[string]any{
		"relative_path": media.relativePath,
		"exists":        true,
		"size_bytes":    info.Size(),
		"modified_at":   info.ModTime().UTC().Format(time.RFC3339),
		"mime_type":     media.mimeType,
	}
	metadata, warnings, err := probeLocalMedia(ctx, file, info.Size())
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, errors.New("local_file_unavailable: cannot read local media headers")
	}
	for key, value := range metadata {
		data[key] = value
	}
	if record != nil {
		stored := s.recordMap(*record)
		data["record_id"] = stored["id"]
		data["record_source"] = record.Source
		sources := data["metadata_sources"].(map[string]string)
		if data["duration_seconds"] == nil && record.Duration > 0 {
			data["duration_seconds"] = record.Duration
			sources["duration_seconds"] = "local_database"
			warnings = appendLocalWarnings(warnings, "stored_media_metadata_fallback")
		}
		if data["resolution"] == nil && record.Resolution != "" {
			data["resolution"] = stored["resolution"]
			sources["resolution"] = "local_database"
			warnings = appendLocalWarnings(warnings, "stored_media_metadata_fallback")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return boundedLocalResult(envelope("local_file", "stored", data, warnings))
}

func (s *Server) findStoredRecord(ctx context.Context, source, id string) (*storedRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("local_database_unavailable: the wx_channel database is not initialized")
	}
	query, args, err := buildLocalUnionQuery(listLocalVideosInput{Source: source})
	if err != nil {
		return nil, err
	}
	// Reuse the list projection without selecting credentials from either table.
	query = "SELECT * FROM (" + query + ") WHERE id = ? ORDER BY source DESC LIMIT 1"
	record, err := scanStoredRecord(db.QueryRowContext(ctx, query, append(args, id)...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("local_record_not_found: no matching local record was found")
	}
	if err != nil {
		return nil, localQueryError(ctx, err)
	}
	return record, nil
}

func (s *Server) findDownloadRecordByPath(ctx context.Context, relativePath string) (*storedRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db := database.GetDB()
	if db == nil {
		return nil, nil
	}
	baseDir, err := filepath.Abs(s.deps.DownloadsDir)
	if err != nil {
		return nil, nil
	}
	target, err := filepath.Abs(filepath.Join(baseDir, filepath.FromSlash(relativePath)))
	if err != nil {
		return nil, nil
	}
	query, args, err := buildLocalUnionQuery(listLocalVideosInput{Source: "download"})
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, localQueryError(ctx, err)
	}
	defer rows.Close()
	for rows.Next() {
		record, scanErr := scanStoredRecord(rows)
		if scanErr != nil {
			return nil, localQueryError(ctx, scanErr)
		}
		storedPath := record.FilePath
		if !filepath.IsAbs(storedPath) {
			storedPath = filepath.Join(baseDir, storedPath)
		}
		storedPath, pathErr := filepath.Abs(storedPath)
		if pathErr != nil {
			continue
		}
		if !sameLocalPath(target, storedPath) {
			continue
		}
		return record, nil
	}
	if err := rows.Err(); err != nil {
		return nil, localQueryError(ctx, err)
	}
	return nil, nil
}

func sameLocalPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if os.PathSeparator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (s *Server) recordMap(record storedRecord) map[string]any {
	data := map[string]any{
		"source":           record.Source,
		"id":               record.ID,
		"video_id":         record.VideoID,
		"title":            record.Title,
		"author":           record.Author,
		"author_id":        record.AuthorID,
		"cover_url":        record.CoverURL,
		"duration_seconds": record.Duration,
		"size_bytes":       record.SizeBytes,
		"file_size_bytes":  record.FileSize,
		"format":           record.Format,
		"resolution":       record.Resolution,
		"status":           record.Status,
		"observed_at":      record.ObservedAt.UTC().Format(time.RFC3339),
	}
	warnings := projectStoredMetrics(data, record)
	cover, redacted := sanitizeURL(record.CoverURL)
	if redacted {
		warnings = appendLocalWarnings(warnings, "sensitive_fields_redacted")
	} else if cover != "" {
		clean, ok := safePublicURL(cover).(string)
		if !ok {
			warnings = appendLocalWarnings(warnings, "stored_text_redacted")
		} else if clean != cover {
			warnings = appendLocalWarnings(warnings, "sensitive_fields_redacted")
		}
		cover = clean
	}
	data["cover_url"] = cover
	if record.ErrorMessage != "" {
		warnings = appendLocalWarnings(warnings, "stored_error_redacted")
	}
	// Visit fields in a fixed order so warning order is stable across requests.
	for _, key := range []string{"id", "video_id", "title", "author", "author_id", "cover_url", "format", "resolution", "status"} {
		text := data[key].(string)
		if key != "cover_url" && localAbsolutePath.MatchString(text) {
			text = "[redacted_local_path]"
			warnings = appendLocalWarnings(warnings, "stored_text_redacted")
		}
		data[key] = boundedText(text, &warnings)
	}
	if record.FilePath != "" {
		if file, ok := s.safeFileReference(record.FilePath); ok {
			data["file"] = file
		} else {
			warnings = appendLocalWarnings(warnings, "stored_file_path_unavailable_or_outside_downloads")
		}
	}
	data["warnings"] = warnings
	return data
}

func (s *Server) safeFileReference(rawPath string) (map[string]any, bool) {
	baseDir, err := filepath.Abs(s.deps.DownloadsDir)
	if err != nil || strings.TrimSpace(rawPath) == "" {
		return nil, false
	}
	candidate := rawPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseDir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return nil, false
	}
	relative, err := filepath.Rel(baseDir, candidate)
	if err != nil || !isSafeRelativePath(relative) || utf8.RuneCountInString(relative) > maxFieldRunes || !isLocalMediaPath(candidate) {
		return nil, false
	}
	exists := false
	size := int64(0)
	if info, statErr := os.Stat(candidate); statErr == nil {
		resolved, validateErr := handlers.ValidateFilePathInBase(baseDir, candidate)
		if validateErr != nil || !isLocalMediaPath(resolved) || !info.Mode().IsRegular() {
			return nil, false
		}
		exists = true
		size = info.Size()
	} else if !os.IsNotExist(statErr) {
		return nil, false
	} else {
		if _, linkErr := os.Lstat(candidate); linkErr == nil || !os.IsNotExist(linkErr) {
			return nil, false
		}
		// Resolve the nearest existing ancestor before reporting a missing file:
		// a missing child of an escaping junction is still outside downloads.
		ancestor := filepath.Dir(candidate)
		for {
			if _, statErr := os.Lstat(ancestor); statErr == nil {
				realBase, baseErr := filepath.EvalSymlinks(baseDir)
				realAncestor, ancestorErr := filepath.EvalSymlinks(ancestor)
				if baseErr != nil || ancestorErr != nil {
					return nil, false
				}
				rel, relErr := filepath.Rel(realBase, realAncestor)
				if relErr != nil || (rel != "." && !isSafeRelativePath(rel)) {
					return nil, false
				}
				break
			} else if !os.IsNotExist(statErr) || filepath.Dir(ancestor) == ancestor {
				return nil, false
			}
			ancestor = filepath.Dir(ancestor)
		}
	}
	return map[string]any{
		"relative_path": filepath.ToSlash(relative),
		"exists":        exists,
		"size_bytes":    size,
	}, true
}

func buildLocalUnionQuery(input listLocalVideosInput) (string, []any, error) {
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "all"
	}
	if source != "all" && source != "browse" && source != "download" {
		return "", nil, errors.New("invalid_argument: source must be browse, download, or all")
	}
	start, hasStart, err := parseDateFilter(input.StartDate, false)
	if err != nil {
		return "", nil, fmt.Errorf("invalid_argument: start_date: %w", err)
	}
	end, hasEnd, err := parseDateFilter(input.EndDate, true)
	if err != nil {
		return "", nil, fmt.Errorf("invalid_argument: end_date: %w", err)
	}
	_, dateOnlyErr := time.Parse("2006-01-02", strings.TrimSpace(input.EndDate))
	if hasStart && hasEnd && (start.After(end) || (dateOnlyErr == nil && start.Equal(end))) {
		return "", nil, errors.New("invalid_argument: start_date must not be after end_date")
	}
	branches := []string{}
	args := []any{}
	if source == "all" || source == "browse" {
		branch, branchArgs := buildLocalBranch("browse", "browse_time", true, input, start, hasStart, end, hasEnd)
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	if source == "all" || source == "download" {
		branch, branchArgs := buildLocalBranch("download", "download_time", true, input, start, hasStart, end, hasEnd)
		branches = append(branches, branch)
		args = append(args, branchArgs...)
	}
	return strings.Join(branches, " UNION ALL "), args, nil
}

func buildLocalBranch(source, timeColumn string, include bool, input listLocalVideosInput, start time.Time, hasStart bool, end time.Time, hasEnd bool) (string, []any) {
	selectSQL := "SELECT '" + source + "' AS source, id, "
	if source == "browse" {
		selectSQL += "id AS video_id, title, author, COALESCE(author_id, '') AS author_id, COALESCE(cover_url, '') AS cover_url, duration AS duration, size AS size_bytes, size AS file_size_bytes, '' AS file_path, COALESCE(file_format, '') AS format, COALESCE(resolution, '') AS resolution, 'stored' AS status, browse_time AS observed_at, like_count AS like_count, comment_count AS comment_count, COALESCE(fav_count, 0) AS fav_count, COALESCE(forward_count, 0) AS forward_count, '' AS error_message, metrics_snapshot FROM browse_history"
	} else {
		selectSQL += "video_id, title, author, '' AS author_id, COALESCE(cover_url, '') AS cover_url, duration AS duration, file_size AS size_bytes, file_size AS file_size_bytes, COALESCE(file_path, '') AS file_path, COALESCE(format, '') AS format, COALESCE(resolution, '') AS resolution, status AS status, download_time AS observed_at, like_count AS like_count, comment_count AS comment_count, COALESCE(fav_count, 0) AS fav_count, COALESCE(forward_count, 0) AS forward_count, COALESCE(error_message, '') AS error_message, metrics_snapshot FROM download_records"
	}
	conditions := []string{}
	args := []any{}
	if !include {
		conditions = append(conditions, "1 = 0")
	}
	if query := strings.TrimSpace(input.Query); query != "" {
		pattern := "%" + query + "%"
		conditions = append(conditions, "(title LIKE ? OR author LIKE ?)")
		args = append(args, pattern, pattern)
	}
	if author := strings.TrimSpace(input.Author); author != "" {
		conditions = append(conditions, "author LIKE ?")
		args = append(args, "%"+author+"%")
	}
	if authorID := strings.TrimSpace(input.AuthorID); authorID != "" {
		if source == "browse" {
			conditions = append(conditions, "author_id = ?")
			args = append(args, authorID)
		} else {
			conditions = append(conditions, "1 = 0")
		}
	}
	if hasStart {
		conditions = append(conditions, timeColumn+" >= ?")
		args = append(args, start)
	}
	if hasEnd {
		operator := "<="
		if _, dateOnlyErr := time.Parse("2006-01-02", strings.TrimSpace(input.EndDate)); dateOnlyErr == nil {
			operator = "<"
		}
		conditions = append(conditions, timeColumn+" "+operator+" ?")
		args = append(args, end)
	}
	if status := strings.TrimSpace(input.Status); status != "" {
		if source == "download" {
			conditions = append(conditions, "status = ?")
			args = append(args, status)
		} else if status != "stored" {
			conditions = append(conditions, "1 = 0")
		}
	}
	if len(conditions) == 0 {
		return selectSQL, args
	}
	return selectSQL + " WHERE " + strings.Join(conditions, " AND "), args
}

func parseDateFilter(raw string, end bool) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, true, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("use YYYY-MM-DD or RFC3339")
	}
	if end {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return parsed, true, nil
}

func scanStoredRecord(scanner interface{ Scan(...any) error }) (*storedRecord, error) {
	record := &storedRecord{}
	var filePath, format, resolution, errorMessage, coverURL, authorID sql.NullString
	var duration, size, fileSize, likes, comments, favorites, forwards sql.NullInt64
	if err := scanner.Scan(&record.Source, &record.ID, &record.VideoID, &record.Title, &record.Author, &authorID, &coverURL, &duration, &size, &fileSize, &filePath, &format, &resolution, &record.Status, &record.ObservedAt, &likes, &comments, &favorites, &forwards, &errorMessage, &record.MetricsSnapshot); err != nil {
		return nil, err
	}
	record.AuthorID = authorID.String
	record.CoverURL = coverURL.String
	record.FilePath = filePath.String
	record.Format = format.String
	record.Resolution = resolution.String
	record.ErrorMessage = errorMessage.String
	record.Duration, record.SizeBytes, record.FileSize = duration.Int64, size.Int64, fileSize.Int64
	record.LikeCount, record.CommentCount, record.FavCount, record.ForwardCount = likes.Int64, comments.Int64, favorites.Int64, forwards.Int64
	return record, nil
}

func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("v1:" + strconv.Itoa(offset)))
}

func decodeCursor(cursor string) (int, error) {
	if strings.TrimSpace(cursor) == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || !strings.HasPrefix(string(decoded), "v1:") {
		return 0, errors.New("expected an opaque cursor returned by list_local_videos")
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(string(decoded), "v1:"))
	if err != nil || offset < 0 {
		return 0, errors.New("cursor offset is invalid")
	}
	return offset, nil
}

func isSafeRelativePath(relative string) bool {
	return relative != "" && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func mediaMIMEType(path string) string {
	if value := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); value != "" {
		return value
	}
	return "application/octet-stream"
}

func isLocalMediaPath(path string) bool {
	// Extension-based metadata only; no file contents or credentials are read.
	name := strings.ToLower(filepath.Base(path))
	if strings.Contains(name, "token") || strings.HasPrefix(name, "config.") || strings.HasPrefix(name, "config_") || strings.Contains(name, ".db.") {
		return false
	}
	kind := strings.SplitN(mediaMIMEType(path), "/", 2)[0]
	return kind == "video" || kind == "audio" || kind == "image"
}

func appendLocalWarnings(warnings []string, values ...string) []string {
	for _, value := range values {
		addWarning(&warnings, value)
	}
	return warnings
}

func boundedLocalResult(result map[string]any) (*mcp.CallToolResult, map[string]any, error) {
	size, err := serializedToolResultSize(result)
	if err != nil {
		return nil, nil, errors.New("local_response_unavailable: cannot encode local response")
	}
	if size > maxToolResponseBytes {
		return nil, nil, errors.New("response_too_large: local response exceeds the response limit")
	}
	return nil, result, nil
}

func localQueryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New("local_database_query_failed: cannot read local records")
}
