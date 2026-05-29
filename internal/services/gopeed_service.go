package services

import (
	"context"
	"errors"
	"fmt"
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

	return &GopeedService{
		Downloader: d,
		tasks:      make(map[string]string),
	}
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

// DownloadSync downloads a file synchronously (blocking until done)
// and returns the actual output path used by Gopeed.
func (s *GopeedService) DownloadSync(ctx context.Context, url string, path string, connections int, headers map[string]string, onProgress func(progress float64, downloaded int64, total int64)) (string, error) {
	id, err := s.CreateTask(url, path, connections, headers)
	if err != nil {
		return "", fmt.Errorf("failed to create task: %v", err)
	}

	actualPath, waitErr := s.WaitTask(ctx, id, onProgress)
	if waitErr != nil {
		if actualPath == "" {
			actualPath = path
		}
		_ = s.DeleteTask(id, true)
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
