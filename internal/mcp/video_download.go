package mcpserver

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/database"
	"wx_channel/internal/handlers"
	"wx_channel/internal/services"
	"wx_channel/internal/utils"
)

const maxDownloadSourceURLLength = 16 << 10

// VideoDownloadRequest identifies a stored browse record. Callers never
// provide a signed URL, decryption key, request headers, or a filesystem path.
type VideoDownloadRequest struct {
	RecordID string
}

// VideoDownloadResult is deliberately safe to return to MCP callers. Paths
// are relative to the configured downloads directory and sensitive source
// details are omitted.
type VideoDownloadResult struct {
	RecordID         string
	DownloadRecordID string
	Title            string
	Status           string
	RelativePath     string
	SizeBytes        int64
	Skipped          bool
}

// VideoDownloadFunc is the synchronous application-owned download operation.
// The MCP action runs it in a bounded background task so a large video does not
// occupy the MCP request until the HTTP server times out.
type VideoDownloadFunc func(context.Context, VideoDownloadRequest) (VideoDownloadResult, error)

// VideoDownloadEngine is the small part of Gopeed needed by the local action.
// It keeps the downloader testable without starting the Gopeed runtime.
type VideoDownloadEngine interface {
	DownloadSync(context.Context, string, string, int, map[string]string, func(float64, int64, int64)) (string, error)
}

// VideoDownloaderDependencies wires the existing app-owned downloader and
// configuration into the MCP adapter.
type VideoDownloaderDependencies struct {
	Config       *config.Config
	DownloadsDir string
	Engine       VideoDownloadEngine
}

// LocalVideoDownloader downloads a browser-captured video into wx_channel's
// configured downloads directory and records the completed local file.
type LocalVideoDownloader struct {
	cfg          *config.Config
	downloadsDir string
	engine       VideoDownloadEngine
}

func NewLocalVideoDownloader(deps VideoDownloaderDependencies) *LocalVideoDownloader {
	dir, _ := filepath.Abs(strings.TrimSpace(deps.DownloadsDir))
	return &LocalVideoDownloader{cfg: deps.Config, downloadsDir: dir, engine: deps.Engine}
}

func (d *LocalVideoDownloader) Download(ctx context.Context, request VideoDownloadRequest) (VideoDownloadResult, error) {
	if err := ctx.Err(); err != nil {
		return VideoDownloadResult{}, err
	}
	if d == nil || d.engine == nil {
		return VideoDownloadResult{}, errors.New("download_unavailable: the Gopeed downloader is unavailable")
	}
	recordID := strings.TrimSpace(request.RecordID)
	if recordID == "" || len(recordID) > 512 {
		return VideoDownloadResult{}, errors.New("invalid_argument: record_id is required")
	}
	if strings.TrimSpace(d.downloadsDir) == "" {
		return VideoDownloadResult{}, errors.New("download_unavailable: the downloads directory is unavailable")
	}
	if database.GetDB() == nil {
		return VideoDownloadResult{}, errors.New("local_database_unavailable: the wx_channel database is not initialized")
	}

	browseRepo := database.NewBrowseHistoryRepository()
	browse, err := browseRepo.GetByID(recordID)
	if err != nil {
		return VideoDownloadResult{}, errors.New("local_database_query_failed: cannot read the source record")
	}
	if browse == nil {
		return VideoDownloadResult{}, errors.New("download_record_not_found: no matching browse record was found")
	}
	videoURL := strings.TrimSpace(browse.VideoURL)
	if !isDownloadableStoredURL(videoURL) {
		return VideoDownloadResult{}, errors.New("download_source_unavailable: the browse record has no supported video URL")
	}

	downloadRepo := database.NewDownloadRecordRepository()
	existing, err := downloadRepo.GetByVideoID(recordID)
	if err != nil {
		return VideoDownloadResult{}, errors.New("local_database_query_failed: cannot read the download record")
	}
	if existing != nil && existing.Status == database.DownloadStatusCompleted {
		if safePath, info, ok := d.existingLocalFile(existing.FilePath); ok && info.Size() > 0 {
			return VideoDownloadResult{
				RecordID: recordID, DownloadRecordID: existing.ID, Title: existing.Title,
				Status: database.DownloadStatusCompleted, RelativePath: d.relativePath(safePath),
				SizeBytes: info.Size(), Skipped: true,
			}, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return VideoDownloadResult{}, err
	}
	if err := os.MkdirAll(d.downloadsDir, 0755); err != nil {
		return VideoDownloadResult{}, errors.New("download_unavailable: cannot prepare the downloads directory")
	}
	authorDir := filepath.Join(d.downloadsDir, utils.CleanFolderName(browse.Author))
	if err := d.ensureDirectoryInsideDownloads(authorDir); err != nil {
		return VideoDownloadResult{}, errors.New("path_forbidden: the download target is outside the configured downloads directory")
	}
	if err := os.MkdirAll(authorDir, 0755); err != nil {
		return VideoDownloadResult{}, errors.New("download_unavailable: cannot prepare the author directory")
	}

	filename, requiredSuffix := d.buildFilename(*browse)
	finalPath := utils.BuildDownloadFilePath(authorDir, filename, requiredSuffix)
	tmpPath := utils.BuildTempDownloadPath(finalPath, recordID)
	_ = os.Remove(tmpPath)

	downloadRecord := existing
	if downloadRecord == nil {
		downloadRecord = &database.DownloadRecord{ID: recordID, VideoID: recordID}
	}
	if strings.TrimSpace(downloadRecord.ID) == "" {
		downloadRecord.ID = recordID
	}
	downloadRecord.VideoID = recordID
	downloadRecord.Title = browse.Title
	downloadRecord.Author = browse.Author
	downloadRecord.CoverURL = browse.CoverURL
	downloadRecord.Duration = browse.Duration
	downloadRecord.FileSize = 0
	downloadRecord.FilePath = finalPath
	downloadRecord.Format = "mp4"
	downloadRecord.Resolution = browse.Resolution
	downloadRecord.Status = database.DownloadStatusInProgress
	downloadRecord.DownloadTime = time.Now()
	downloadRecord.ErrorMessage = ""
	downloadRecord.LikeCount = browse.LikeCount
	downloadRecord.CommentCount = browse.CommentCount
	downloadRecord.ForwardCount = browse.ForwardCount
	downloadRecord.FavCount = browse.FavCount
	downloadRecord.MetricsSnapshot = browse.MetricsSnapshot
	if err := downloadRepo.Create(downloadRecord); err != nil {
		return VideoDownloadResult{}, errors.New("download_record_save_failed: cannot create the download record")
	}

	normalizedURL, mode := handlers.NormalizeDownloadURL(videoURL, "original")
	connections := 1
	if d.cfg != nil && d.cfg.DownloadConnections > 0 {
		connections = handlers.ResolveDownloadConnections(mode, d.cfg.DownloadConnections)
	}
	headers := map[string]string{"Origin": "https://channels.weixin.qq.com"}
	if referer := safeDownloadReferer(browse.PageURL); referer != "" {
		headers["Referer"] = referer
	}
	actualPath, downloadErr := d.engine.DownloadSync(ctx, normalizedURL, tmpPath, connections, headers, nil)
	if downloadErr != nil {
		d.removeDownloadArtifact(actualPath, tmpPath)
		d.markDownloadFailed(downloadRepo, downloadRecord)
		return VideoDownloadResult{}, errors.New("download_failed: the video download failed")
	}
	if strings.TrimSpace(actualPath) == "" {
		actualPath = tmpPath
	}
	safeActualPath, info, ok := d.existingLocalFile(actualPath)
	if !ok || info.Size() <= 0 {
		d.removeDownloadArtifact(actualPath, tmpPath)
		d.markDownloadFailed(downloadRepo, downloadRecord)
		return VideoDownloadResult{}, errors.New("download_failed: the downloaded file is unavailable")
	}
	if browse.DecryptKey != "" {
		if err := utils.DecryptFileInPlace(safeActualPath, browse.DecryptKey, "", 0); err != nil {
			d.removeDownloadArtifact(safeActualPath, tmpPath)
			d.markDownloadFailed(downloadRepo, downloadRecord)
			return VideoDownloadResult{}, errors.New("download_failed: the downloaded file could not be decrypted")
		}
	}

	finalPath, err = utils.MoveFileToAvailablePathWithSuffix(safeActualPath, finalPath, requiredSuffix)
	if err != nil {
		d.removeDownloadArtifact(safeActualPath, tmpPath)
		d.markDownloadFailed(downloadRepo, downloadRecord)
		return VideoDownloadResult{}, errors.New("download_failed: the downloaded file could not be stored")
	}
	safeFinalPath, info, ok := d.existingLocalFile(finalPath)
	if !ok || info.Size() <= 0 {
		d.markDownloadFailed(downloadRepo, downloadRecord)
		return VideoDownloadResult{}, errors.New("download_failed: the stored file is unavailable")
	}
	downloadRecord.FilePath = safeFinalPath
	downloadRecord.FileSize = info.Size()
	downloadRecord.Status = database.DownloadStatusCompleted
	downloadRecord.DownloadTime = time.Now()
	downloadRecord.ErrorMessage = ""
	if err := downloadRepo.Update(downloadRecord); err != nil {
		return VideoDownloadResult{}, errors.New("download_record_save_failed: cannot finalize the download record")
	}
	return VideoDownloadResult{
		RecordID: recordID, DownloadRecordID: downloadRecord.ID, Title: browse.Title,
		Status: database.DownloadStatusCompleted, RelativePath: d.relativePath(safeFinalPath), SizeBytes: info.Size(),
	}, nil
}

func (d *LocalVideoDownloader) buildFilename(record database.BrowseRecord) (string, string) {
	includeVideoID := true
	if repo := database.NewSettingsRepository(); repo != nil && repo.GetDBUnsafe() != nil {
		if settings, err := repo.Load(); err == nil && settings != nil {
			includeVideoID = settings.DownloadFilenameWithVideoID
		}
	}
	template := ""
	if d.cfg != nil {
		template = d.cfg.DownloadFilenameTemplate
	}
	filename := utils.BuildVideoFilename(utils.VideoFilenameMeta{
		Title: record.Title, VideoID: record.ID, Author: record.Author,
		Duration: time.Duration(record.Duration) * time.Millisecond, CreateTime: record.BrowseTime, SizeBytes: record.Size,
	}, includeVideoID, template)
	filename = utils.EnsureExtension(filename, ".mp4")
	requiredSuffix := ""
	if includeVideoID && strings.TrimSpace(template) == "" {
		requiredSuffix = utils.VideoFilenameRequiredSuffix(filename, record.ID)
	}
	return filename, requiredSuffix
}

func (d *LocalVideoDownloader) existingLocalFile(rawPath string) (string, os.FileInfo, bool) {
	if strings.TrimSpace(rawPath) == "" {
		return "", nil, false
	}
	candidate := rawPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(d.downloadsDir, candidate)
	}
	safePath, err := handlers.ValidateFilePathInBase(d.downloadsDir, candidate)
	if err != nil {
		return "", nil, false
	}
	info, err := os.Stat(safePath)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil, false
	}
	return safePath, info, true
}

func (d *LocalVideoDownloader) ensureDirectoryInsideDownloads(path string) error {
	base, err := filepath.Abs(d.downloadsDir)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(path)
	if err != nil || !pathWithin(base, target) {
		return errors.New("directory outside downloads")
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return err
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil || !pathWithin(realBase, realTarget) {
		return errors.New("directory resolves outside downloads")
	}
	return nil
}

func (d *LocalVideoDownloader) removeDownloadArtifact(paths ...string) {
	for _, path := range paths {
		if _, _, ok := d.existingLocalFile(path); ok {
			_ = os.Remove(path)
		}
	}
}

func (d *LocalVideoDownloader) markDownloadFailed(repo *database.DownloadRecordRepository, record *database.DownloadRecord) {
	if repo == nil || record == nil {
		return
	}
	record.Status = database.DownloadStatusFailed
	record.FilePath = ""
	record.FileSize = 0
	record.ErrorMessage = "download_failed"
	_ = repo.Update(record)
}

func (d *LocalVideoDownloader) relativePath(path string) string {
	relative, err := filepath.Rel(d.downloadsDir, path)
	if err != nil || !isSafeRelativePath(relative) {
		return ""
	}
	return filepath.ToSlash(relative)
}

func isDownloadableStoredURL(raw string) bool {
	if len(raw) == 0 || len(raw) > maxDownloadSourceURLLength {
		return false
	}
	parsed, err := url.Parse(raw)
	return err == nil && strings.EqualFold(parsed.Scheme, "https") && parsed.Hostname() != "" && parsed.User == nil
}

func safeDownloadReferer(raw string) string {
	if len(raw) == 0 || len(raw) > maxDownloadSourceURLLength {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func pathWithin(base, target string) bool {
	relative, err := filepath.Rel(base, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

var _ VideoDownloadEngine = (*services.GopeedService)(nil)
