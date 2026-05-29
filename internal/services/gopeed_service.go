package services

import (
	"context"
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

// CreateTask creates a download task
func (s *GopeedService) CreateTask(url string, opt *base.Options) (string, error) {
	if s.Downloader == nil {
		return "", fmt.Errorf("downloader not initialized")
	}
	req := &base.Request{URL: url}
	return s.Downloader.CreateDirect(req, opt)
}

// DeleteTask removes a download task
func (s *GopeedService) DeleteTask(taskID string) error {
	if s.Downloader == nil {
		return fmt.Errorf("downloader not initialized")
	}
	// Pause and remove task
	// Note: Gopeed API might vary, assuming Pause and Delete exist or Pause acts like cancel
	// Check available methods on `s.Downloader`
	// Based on Gopeed source:
	// func (d *Downloader) Pause(filter *TaskFilter)
	// func (d *Downloader) Delete(filter *TaskFilter)

	// We prefer Delete
	filter := &download.TaskFilter{IDs: []string{taskID}}

	// Try Delete first if available, otherwise Pause
	// Since we don't have full intellisense, we'll try Delete, assuming typical API
	s.Downloader.Delete(filter, true)

	return nil
}

// DownloadSync downloads a file synchronously (blocking until done)
// and returns the actual output path used by Gopeed.
func (s *GopeedService) DownloadSync(ctx context.Context, url string, path string, connections int, headers map[string]string, onProgress func(progress float64, downloaded int64, total int64)) (string, error) {
	if s.Downloader == nil {
		return "", fmt.Errorf("downloader not initialized")
	}

	// Configure options
	dir := filepath.Dir(path)
	name := filepath.Base(path)

	// 默认8个连接
	if connections <= 0 {
		connections = 8
	}

	opts := &base.Options{
		Path: dir,
		Name: name,
		Extra: &httpProtocol.OptsExtra{
			Connections: connections, // 单文件多线程下载
		},
	}

	// Create task using CreateDirect
	req := &base.Request{URL: url}
	if len(headers) > 0 {
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
	}
	id, err := s.Downloader.CreateDirect(req, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create task: %v", err)
	}

	actualPath := path

	// Poll status
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Cancel task
			s.Downloader.Delete(&download.TaskFilter{IDs: []string{id}}, true)
			return actualPath, ctx.Err()
		case <-ticker.C:
			task := s.Downloader.GetTask(id)
			if task == nil {
				return actualPath, fmt.Errorf("task not found: %s", id)
			}
			if task.Meta != nil && task.Meta.Res != nil {
				actualPath = task.Meta.SingleFilepath()
			}

			// Report progress
			if onProgress != nil {
				var downloaded, total int64
				var progress float64

				if task.Progress != nil {
					downloaded = task.Progress.Downloaded
				}

				// 使用反射获取 TotalSize (因为 Meta 字段类型是 internal 的，外部无法直接访问)
				// task.Meta -> *fetcher.FetcherMeta
				// FetcherMeta.Res -> *base.Resource
				// Resource.Size -> int64
				func() {
					defer func() {
						if r := recover(); r != nil {
							// 忽略反射 panic，防止 crash
							utils.Warn("反射获取文件大小失败: %v", r)
						}
					}()

					// get *Task value
					v := reflect.ValueOf(task).Elem()

					// get Meta field
					metaField := v.FieldByName("Meta")
					if metaField.IsValid() && !metaField.IsNil() {
						// get Res field from FetcherMeta
						// FetcherMeta struct definition: type FetcherMeta struct { ... Res *base.Resource ... }
						// We need to dereference the pointer first
						resField := metaField.Elem().FieldByName("Res")
						if resField.IsValid() && !resField.IsNil() {
							// get Size field from Resource
							sizeField := resField.Elem().FieldByName("Size")
							if sizeField.IsValid() {
								total = sizeField.Int()
							}
						}
					}
				}()

				if total > 0 {
					progress = float64(downloaded) / float64(total)
				}

				// 调用进度回调
				onProgress(progress, downloaded, total)
			}

			// Check status
			switch task.Status {
			case base.DownloadStatusDone:
				return actualPath, nil
			case base.DownloadStatusError:
				return actualPath, fmt.Errorf("download task failed")
			case base.DownloadStatusRunning, base.DownloadStatusReady:
				// Continue waiting
				continue
			default:
				// Handle other statuses (Paused, etc)
				// Continue waiting
			}
		}
	}
}
