package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
	"wx_channel/internal/utils"

	"github.com/GopeedLab/gopeed/pkg/base"
	"github.com/GopeedLab/gopeed/pkg/download"
	_ "github.com/GopeedLab/gopeed/pkg/protocol/http" // Register HTTP protocol
	httpProtocol "github.com/GopeedLab/gopeed/pkg/protocol/http"
)

var ErrTaskPaused = errors.New("gopeed task paused")

type GopeedTaskSnapshot struct {
	ID         string
	Status     base.Status
	ActualPath string
	Downloaded int64
	Total      int64
}

// GopeedService wraps the Gopeed downloader engine
type GopeedService struct {
	Downloader *download.Downloader
	mu         sync.RWMutex
	tasks      map[string]string // Maps internal ID to Gopeed Task ID
	taskErrors map[string]error
}

// NewGopeedService creates a new GopeedService
// Note: We bypass store for now due to dependency issues or signature changes
func NewGopeedService(storageDir string) *GopeedService {
	// Create downloader config
	dlCfg := &download.DownloaderConfig{
		// Default config is acceptable
	}

	// Create a downloader instance
	d := download.NewDownloader(dlCfg)

	// Try to setup
	if err := d.Setup(); err != nil {
		utils.Warn("Gopeed Setup failed: %v", err)
	}

	service := &GopeedService{
		Downloader: d,
		tasks:      make(map[string]string),
		taskErrors: make(map[string]error),
	}
	d.Listener(func(event *download.Event) {
		if event == nil || event.Key != download.EventKeyError || event.Task == nil || event.Err == nil {
			return
		}
		service.mu.Lock()
		service.taskErrors[event.Task.ID] = event.Err
		service.mu.Unlock()
	})
	return service
}

func normalizeConnections(connections int) int {
	if connections <= 0 {
		return 8
	}
	return connections
}

func buildOptions(path string, connections int) *base.Options {
	return &base.Options{
		Path: filepath.Dir(path),
		Name: filepath.Base(path),
		Extra: &httpProtocol.OptsExtra{
			Connections: normalizeConnections(connections),
		},
	}
}

func buildRequest(url string, headers map[string]string) *base.Request {
	req := &base.Request{URL: url}
	if len(headers) == 0 {
		return req
	}

	reqHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		reqHeaders[k] = v
	}
	if len(reqHeaders) > 0 {
		req.Extra = &httpProtocol.ReqExtra{
			Header: reqHeaders,
		}
	}
	return req
}

// CreateTask creates a download task and starts it immediately.
func (s *GopeedService) CreateTask(url string, path string, connections int, headers map[string]string) (string, error) {
	if s.Downloader == nil {
		return "", fmt.Errorf("downloader not initialized")
	}
	return s.Downloader.CreateDirect(buildRequest(url, headers), buildOptions(path, connections))
}

func (s *GopeedService) PauseTask(taskID string) error {
	if s.Downloader == nil {
		return fmt.Errorf("downloader not initialized")
	}
	if strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("task id is empty")
	}
	return s.Downloader.Pause(&download.TaskFilter{IDs: []string{taskID}})
}

func (s *GopeedService) ContinueTask(taskID string) error {
	if s.Downloader == nil {
		return fmt.Errorf("downloader not initialized")
	}
	if strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("task id is empty")
	}
	return s.Downloader.Continue(&download.TaskFilter{IDs: []string{taskID}})
}

// DeleteTask removes a download task
func (s *GopeedService) DeleteTask(taskID string, removeFiles bool) error {
	if s.Downloader == nil {
		return fmt.Errorf("downloader not initialized")
	}
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	filter := &download.TaskFilter{IDs: []string{taskID}}
	return s.Downloader.Delete(filter, removeFiles)
}

func (s *GopeedService) GetTaskSnapshot(taskID string) (*GopeedTaskSnapshot, error) {
	if s.Downloader == nil {
		return nil, fmt.Errorf("downloader not initialized")
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("task id is empty")
	}

	task := s.Downloader.GetTask(taskID)
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	snapshot := &GopeedTaskSnapshot{
		ID:     taskID,
		Status: task.Status,
	}
	if task.Meta != nil && task.Meta.Res != nil {
		snapshot.ActualPath = task.Meta.SingleFilepath()
	}
	if task.Progress != nil {
		snapshot.Downloaded = task.Progress.Downloaded
	}

	// Use reflection to safely extract total size from internal meta types.
	func() {
		defer func() {
			if r := recover(); r != nil {
				utils.Warn("反射获取文件大小失败: %v", r)
			}
		}()

		v := reflect.ValueOf(task).Elem()
		metaField := v.FieldByName("Meta")
		if metaField.IsValid() && !metaField.IsNil() {
			resField := metaField.Elem().FieldByName("Res")
			if resField.IsValid() && !resField.IsNil() {
				sizeField := resField.Elem().FieldByName("Size")
				if sizeField.IsValid() {
					snapshot.Total = sizeField.Int()
				}
			}
		}
	}()

	return snapshot, nil
}

func (s *GopeedService) takeTaskError(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.taskErrors[taskID]
	delete(s.taskErrors, taskID)
	return err
}

func (s *GopeedService) clearTaskError(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.taskErrors, taskID)
}

func (s *GopeedService) WaitTask(ctx context.Context, taskID string, onProgress func(progress float64, downloaded int64, total int64)) (string, error) {
	if s.Downloader == nil {
		return "", fmt.Errorf("downloader not initialized")
	}

	// Poll status
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			snapshot, err := s.GetTaskSnapshot(taskID)
			if err != nil {
				return "", err
			}

			if onProgress != nil {
				progress := 0.0
				if snapshot.Total > 0 {
					progress = float64(snapshot.Downloaded) / float64(snapshot.Total)
				}
				onProgress(progress, snapshot.Downloaded, snapshot.Total)
			}

			switch snapshot.Status {
			case base.DownloadStatusDone:
				return snapshot.ActualPath, nil
			case base.DownloadStatusError:
				if taskErr := s.takeTaskError(taskID); taskErr != nil {
					return snapshot.ActualPath, fmt.Errorf("download task failed: %w", taskErr)
				}
				return snapshot.ActualPath, fmt.Errorf("download task failed")
			case base.DownloadStatusPause:
				return snapshot.ActualPath, ErrTaskPaused
			case base.DownloadStatusRunning, base.DownloadStatusReady, base.DownloadStatusWait:
				continue
			default:
				continue
			}
		}
	}
}

// Gopeed v1.8.3 probes HTTP resources with Range: bytes=0-0. Some signed
// video endpoints reject that probe while accepting a normal GET request.
func isGopeedRangeProbeFailure(err error) bool {
	return err != nil && strings.Contains(err.Error(), "http request fail,code:400")
}

func downloadHTTPFallback(ctx context.Context, url string, path string, headers map[string]string, onProgress func(progress float64, downloaded int64, total int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create fallback request failed: %w", err)
	}
	for key, value := range headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
	if req.Header.Get("Accept-Language") == "" {
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	}
	if req.Header.Get("Cache-Control") == "" {
		req.Header.Set("Cache-Control", "no-cache")
	}

	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return "", fmt.Errorf("fallback request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("fallback request returned HTTP %s", resp.Status)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("create fallback file failed: %w", err)
	}
	defer file.Close()

	total := resp.ContentLength
	var downloaded int64
	lastReport := time.Time{}
	buffer := make([]byte, 256*1024)
	for {
		read, readErr := resp.Body.Read(buffer)
		if read > 0 {
			if _, err := file.Write(buffer[:read]); err != nil {
				return "", fmt.Errorf("write fallback file failed: %w", err)
			}
			downloaded += int64(read)
			if onProgress != nil {
				now := time.Now()
				if now.Sub(lastReport) >= 300*time.Millisecond {
					progress := 0.0
					if total > 0 {
						progress = float64(downloaded) / float64(total)
					}
					onProgress(progress, downloaded, total)
					lastReport = now
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("read fallback response failed: %w", readErr)
		}
	}
	if onProgress != nil {
		progress := 0.0
		if total > 0 {
			progress = float64(downloaded) / float64(total)
		}
		onProgress(progress, downloaded, total)
	}
	return path, nil
}

// DownloadSync downloads a file synchronously (blocking until done)
// and returns the actual output path used by Gopeed.
func (s *GopeedService) DownloadSync(ctx context.Context, url string, path string, connections int, headers map[string]string, onProgress func(progress float64, downloaded int64, total int64)) (string, error) {
	id, err := s.CreateTask(url, path, connections, headers)
	if err != nil {
		return "", fmt.Errorf("failed to create task: %v", err)
	}
	defer s.clearTaskError(id)

	actualPath, waitErr := s.WaitTask(ctx, id, onProgress)
	if waitErr != nil {
		if actualPath == "" {
			actualPath = path
		}
		_ = s.DeleteTask(id, true)
		if isGopeedRangeProbeFailure(waitErr) {
			utils.Warn("Gopeed HTTP 400 拒绝 Range 探测，回退到后端单流下载")
			fallbackPath, fallbackErr := downloadHTTPFallback(ctx, url, path, headers, onProgress)
			if fallbackErr == nil {
				return fallbackPath, nil
			}
			return actualPath, fmt.Errorf("%w; backend fallback failed: %v", waitErr, fallbackErr)
		}
		return actualPath, waitErr
	}
	if actualPath == "" {
		actualPath = path
	}
	if err := s.DeleteTask(id, false); err != nil {
		utils.Warn("清理 Gopeed 任务失败: %v", err)
	}
	return actualPath, nil
}
