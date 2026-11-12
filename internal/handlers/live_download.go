package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"wx_channel/internal/database"
	"wx_channel/internal/utils"

	"github.com/qtgolang/SunnyNet/SunnyNet"
)

// LiveDownloadRequest describes a live stream recording request from the
// isolated live-page script.
type LiveDownloadRequest struct {
	StreamURL string            `json:"streamUrl"`
	VideoID   string            `json:"videoId"`
	Title     string            `json:"title"`
	Author    string            `json:"author"`
	SourceURL string            `json:"sourceUrl"`
	UserAgent string            `json:"userAgent"`
	Headers   map[string]string `json:"headers"`
}

// HandleDownloadLive creates a local FFmpeg recording task for the current
// live stream. The task is cancellable through cancel_download using VideoID.
func (h *UploadHandler) HandleDownloadLive(conn *SunnyNet.HttpConn) bool {
	if conn == nil || conn.Request == nil || conn.Request.URL == nil || conn.Request.URL.Path != "/__wx_channels_api/download_live" {
		return false
	}
	utils.Info("[直播下载] 收到创建任务请求: method=%s host=%s origin=%s", conn.Request.Method, conn.Request.URL.Host, conn.Request.Header.Get("Origin"))

	cfg := h.getConfig()
	if cfg != nil && cfg.SecretToken != "" && conn.Request.Header.Get("X-Local-Auth") != cfg.SecretToken {
		conn.StopRequest(http.StatusUnauthorized, `{"success":false,"error":"unauthorized"}`, jsonHeaders())
		return true
	}
	if cfg != nil && len(cfg.AllowedOrigins) > 0 {
		origin := conn.Request.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, candidate := range cfg.AllowedOrigins {
				if candidate == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				conn.StopRequest(http.StatusForbidden, `{"success":false,"error":"forbidden_origin"}`, jsonHeaders())
				return true
			}
		}
	}
	if conn.Request.Method != http.MethodPost {
		h.sendErrorResponse(conn, fmt.Errorf("method not allowed: %s", conn.Request.Method))
		return true
	}
	if conn.Request.Body == nil {
		h.sendErrorResponse(conn, fmt.Errorf("request body is nil"))
		return true
	}

	var req LiveDownloadRequest
	if err := json.NewDecoder(conn.Request.Body).Decode(&req); err != nil {
		h.sendErrorResponse(conn, fmt.Errorf("解析download_live JSON: %w", err))
		return true
	}
	_ = conn.Request.Body.Close()

	streamURL, err := validateVideoPlayTargetURL(strings.TrimSpace(req.StreamURL))
	if err != nil {
		h.sendErrorResponse(conn, err)
		return true
	}
	ffmpeg, err := resolveLiveFFmpeg()
	if err != nil {
		h.sendErrorResponse(conn, err)
		return true
	}
	utils.Info("[直播下载] 已找到 FFmpeg: %s", ffmpeg)

	videoID := strings.TrimSpace(req.VideoID)
	if videoID == "" {
		videoID = "live_" + fmt.Sprint(time.Now().UnixNano())
	}
	if _, exists := h.activeDownloads.Load(videoID); exists {
		h.sendJSONResponse(conn, http.StatusOK, map[string]any{"success": true, "started": true, "message": "直播录制任务已在后台进行中"})
		return true
	}

	downloadsDir, err := h.getDownloadsDir()
	if err != nil {
		h.sendErrorResponse(conn, err)
		return true
	}
	authorFolder := utils.CleanFolderName(req.Author)
	if authorFolder == "" {
		authorFolder = "未知作者"
	}
	saveDir := filepath.Join(downloadsDir, authorFolder)
	if err := utils.EnsureDir(saveDir); err != nil {
		h.sendErrorResponse(conn, fmt.Errorf("创建直播下载目录失败: %w", err))
		return true
	}

	settings, _ := h.settingsRepo.Load()
	includeID := settings == nil || settings.DownloadFilenameWithVideoID
	filenameTemplate := ""
	if cfg != nil {
		filenameTemplate = cfg.DownloadFilenameTemplate
	}
	filename := utils.BuildVideoFilename(utils.VideoFilenameMeta{Title: req.Title, VideoID: videoID, Author: req.Author}, includeID, filenameTemplate)
	filename = utils.EnsureExtension(filename, ".flv")
	videoPath := utils.BuildDownloadFilePath(saveDir, filename, "")
	tmpPath := utils.BuildTempDownloadPath(videoPath, videoID)

	ctx, cancel := context.WithCancel(context.Background())
	h.activeDownloads.Store(videoID, cancel)
	go h.recordLiveStream(ctx, cancel, req, streamURL, ffmpeg, videoID, videoPath, tmpPath, downloadsDir)

	h.sendJSONResponse(conn, http.StatusOK, map[string]any{
		"success": true,
		"started": true,
		"videoId": videoID,
		"path":    videoPath,
		"message": "直播录制任务已在后台启动",
	})
	return true
}

func (h *UploadHandler) recordLiveStream(ctx context.Context, cancel context.CancelFunc, req LiveDownloadRequest, streamURL, ffmpeg, videoID, videoPath, tmpPath, downloadsDir string) {
	defer cancel()
	defer h.activeDownloads.Delete(videoID)
	defer os.Remove(tmpPath)

	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	if userAgent := strings.TrimSpace(req.UserAgent); userAgent != "" {
		args = append(args, "-user_agent", userAgent)
	}
	if headerBlock := buildFFmpegHeaderBlock(req.Headers, req.SourceURL); headerBlock != "" {
		args = append(args, "-headers", headerBlock)
	}
	args = append(args, "-i", streamURL, "-c", "copy", "-f", "flv", tmpPath)

	utils.Info("🎥 [直播下载] 开始录制: %s", req.Title)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			if _, statErr := os.Stat(tmpPath); statErr == nil {
				if finalPath, saveErr := h.finalizeLiveRecording(req, videoID, videoPath, tmpPath, downloadsDir); saveErr != nil {
					utils.Warn("⏹️ [直播下载] 已停止，但保存已录制片段失败: %v", saveErr)
				} else {
					utils.Info("⏹️ [直播下载] 已停止并保存片段: %s", finalPath)
				}
			} else {
				utils.Info("⏹️ [直播下载] 已停止，未产生可保存片段: %s", req.Title)
			}
			return
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		utils.Error("❌ [直播下载] 录制失败: %s", message)
		h.broadcastLiveDownloadFailed(videoID, req.Title, message)
		return
	}

	_, err = h.finalizeLiveRecording(req, videoID, videoPath, tmpPath, downloadsDir)
	if err != nil {
		utils.Error("❌ [直播下载] 保存录制文件失败: %v", err)
		h.broadcastLiveDownloadFailed(videoID, req.Title, err.Error())
	}
}

func (h *UploadHandler) finalizeLiveRecording(req LiveDownloadRequest, videoID, videoPath, tmpPath, downloadsDir string) (string, error) {
	stat, err := os.Stat(tmpPath)
	if err != nil {
		return "", err
	}
	if stat.Size() == 0 {
		return "", fmt.Errorf("录制文件为空")
	}
	finalPath, err := utils.MoveFileToAvailablePath(tmpPath, videoPath)
	if err != nil {
		return "", err
	}

	relativePath, _ := filepath.Rel(downloadsDir, finalPath)
	record := &database.DownloadRecord{
		ID:           videoID,
		VideoID:      videoID,
		Title:        req.Title,
		Author:       req.Author,
		FileSize:     stat.Size(),
		FilePath:     finalPath,
		RelativePath: relativePath,
		Format:       "flv",
		Status:       database.DownloadStatusCompleted,
		DownloadTime: time.Now(),
	}
	if h.downloadService != nil {
		if err := h.downloadService.Create(record); err != nil {
			utils.Warn("[直播下载] 保存下载记录失败: %v", err)
		}
	}
	utils.Info("✅ [直播下载] 已保存: %s", finalPath)
	if h.wsHub != nil {
		h.wsHub.BroadcastCommand("download_complete", map[string]any{
			"videoId": videoID, "title": req.Title, "path": finalPath, "relativePath": relativePath,
			"size": float64(stat.Size()) / (1024 * 1024), "live": true,
		})
	}
	return finalPath, nil
}

func (h *UploadHandler) broadcastLiveDownloadFailed(videoID, title, message string) {
	if h.wsHub != nil {
		h.wsHub.BroadcastCommand("download_failed", map[string]any{"videoId": videoID, "title": title, "error": message, "live": true})
	}
}

func resolveLiveFFmpeg() (string, error) {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("WX_CHANNEL_FFMPEG_PATH")); configured != "" {
		candidates = append(candidates, configured)
	}
	if baseDir, err := utils.GetBaseDir(); err == nil {
		binary := "ffmpeg"
		if runtime.GOOS == "windows" {
			binary = "ffmpeg.exe"
		}
		candidates = append(candidates, filepath.Join(baseDir, "tools", "ffmpeg", "bin", binary))
	}
	if fromPath, err := exec.LookPath("ffmpeg"); err == nil {
		candidates = append(candidates, fromPath)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到 FFmpeg，请将 ffmpeg 放入 tools/ffmpeg/bin 或配置 WX_CHANNEL_FFMPEG_PATH")
}

func buildFFmpegHeaderBlock(headers map[string]string, sourceURL string) string {
	values := make(map[string]string, len(headers)+1)
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			values[key] = value
		}
	}
	if _, ok := values["Referer"]; !ok && strings.TrimSpace(sourceURL) != "" {
		values["Referer"] = strings.TrimSpace(sourceURL)
	}
	keys := []string{"Origin", "Referer", "Cookie", "Accept", "Accept-Language"}
	var builder strings.Builder
	for _, key := range keys {
		if value := values[key]; value != "" {
			builder.WriteString(key)
			builder.WriteString(": ")
			builder.WriteString(strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", ""))
			builder.WriteString("\r\n")
		}
	}
	return builder.String()
}

func jsonHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Content-Type-Options", "nosniff")
	return headers
}
