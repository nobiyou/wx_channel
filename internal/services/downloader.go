package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wx_channel/internal/config"
	"wx_channel/internal/models"
	"wx_channel/internal/storage"
	"wx_channel/internal/utils"
)

type DownloadTask struct {
	ID         string
	URL        string
	Filename   string
	AuthorName string
	Decryptor  []byte // 可选：前缀解密数组
}

type DownloadResult struct {
	Task   DownloadTask
	Path   string
	SizeMB float64
	Err    error
}

type Downloader struct {
	cfg    *config.Config
	csv    *storage.CSVManager
	queue  chan DownloadTask
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	total   int
	done    int
	failed  int
	running int
	results []DownloadResult

	// 去重集合（进程内），按任务ID或URL
	seenKeys map[string]struct{}
}

func NewDownloader(cfg *config.Config, csv *storage.CSVManager) *Downloader {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Downloader{
		cfg:      cfg,
		csv:      csv,
		queue:    make(chan DownloadTask, 1024),
		ctx:      ctx,
		cancel:   cancel,
		seenKeys: make(map[string]struct{}),
	}
	// 启动工作协程
	workers := cfg.DownloadConcurrency
	if workers <= 0 {
		workers = 2
	}
	for i := 0; i < workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
	return d
}

func (d *Downloader) Enqueue(tasks []DownloadTask) {
	d.mu.Lock()
	// 去重：同ID优先，否则用URL
	filtered := make([]DownloadTask, 0, len(tasks))
	for _, t := range tasks {
		key := taskKey(t)
		if key == "" {
			continue
		}
		if _, ok := d.seenKeys[key]; ok {
			continue
		}
		d.seenKeys[key] = struct{}{}
		filtered = append(filtered, t)
	}
	d.total += len(filtered)
	d.mu.Unlock()
	for _, t := range filtered {
		select {
		case d.queue <- t:
		case <-d.ctx.Done():
			return
		}
	}
}

func (d *Downloader) worker() {
	defer d.wg.Done()
	client := &http.Client{Timeout: 0}
	for {
		select {
		case <-d.ctx.Done():
			return
		case task := <-d.queue:
			d.mu.Lock()
			d.running++
			d.mu.Unlock()
			res := d.downloadOne(client, task)
			d.mu.Lock()
			d.results = append(d.results, res)
			d.running--
			if res.Err != nil {
				d.failed++
			} else {
				d.done++
			}
			d.mu.Unlock()
		}
	}
}

func (d *Downloader) downloadOne(client *http.Client, task DownloadTask) DownloadResult {
	var lastErr error
	retries := d.cfg.DownloadRetryCount
	if retries <= 0 {
		retries = 1
	}

	for attempt := 1; attempt <= retries; attempt++ {
		path, size, err := d.tryDownload(client, task)
		if err == nil {
			return DownloadResult{Task: task, Path: path, SizeMB: size, Err: nil}
		}
		lastErr = err
		utils.Warn("下载失败(%d/%d): %s -> %v", attempt, retries, task.URL, err)
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
	}
	return DownloadResult{Task: task, Err: lastErr}
}

func (d *Downloader) tryDownload(client *http.Client, task DownloadTask) (string, float64, error) {
	// 计算保存路径
	baseDir, err := utils.GetBaseDir()
	if err != nil {
		return "", 0, err
	}
	authorFolder := utils.CleanFolderName(task.AuthorName)
	downloadsDir := filepath.Join(baseDir, d.cfg.DownloadsDir)
	saveDir := filepath.Join(downloadsDir, authorFolder)
	if err := utils.EnsureDir(saveDir); err != nil {
		return "", 0, err
	}

	cleanFilename := utils.CleanFilename(task.Filename)
	cleanFilename = utils.EnsureExtension(cleanFilename, ".mp4")
	// 优先使用固定文件名，若已存在且非空则直接返回；否则再生成唯一名
	preferredPath := filepath.Join(saveDir, cleanFilename)
	if fi, err := os.Stat(preferredPath); err == nil {
		if fi.Size() > 0 {
			return preferredPath, float64(fi.Size()) / (1024 * 1024), nil
		}
	}
	finalPath := preferredPath
	if _, err := os.Stat(finalPath); err == nil {
		finalPath = utils.GenerateUniqueFilename(saveDir, cleanFilename, 1000)
	}

	// 发起下载
	req, err := http.NewRequestWithContext(d.ctx, http.MethodGet, task.URL, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("http_status_%d", resp.StatusCode)
	}

	tmpPath := finalPath + ".downloading"
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()
	var n int64
	if len(task.Decryptor) > 0 {
		buf := make([]byte, 64*1024)
		var offset int64 = 0
		max := int64(len(task.Decryptor))
		for {
			nr, er := resp.Body.Read(buf)
			if nr > 0 {
				wb := buf[:nr]
				if offset < max {
					limit := int64(nr)
					if offset+limit > max {
						limit = max - offset
					}
					for i := int64(0); i < limit; i++ {
						wb[i] ^= task.Decryptor[offset+i]
					}
				}
				wn, ew := out.Write(wb)
				n += int64(wn)
				if ew != nil {
					return "", 0, ew
				}
				if wn != nr {
					return "", 0, io.ErrShortWrite
				}
				offset += int64(nr)
			}
			if er != nil {
				if er == io.EOF {
					break
				}
				return "", 0, er
			}
		}
	} else {
		n, err = io.Copy(out, resp.Body)
		if err != nil {
			return "", 0, err
		}
	}
	_ = out.Close()
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", 0, err
	}

	// 写入 CSV 记录
	if d.csv != nil {
		rec := &models.VideoDownloadRecord{
			ID:         task.ID,
			Title:      task.Filename,
			Author:     task.AuthorName,
			URL:        task.URL,
			PageURL:    "",
			FileSize:   fmt.Sprintf("%.2f MB", float64(n)/(1024*1024)),
			DownloadAt: time.Now(),
		}
		_ = d.csv.AddRecord(rec)
	}

	return finalPath, float64(n) / (1024 * 1024), nil
}

func (d *Downloader) Progress() (total, done, failed, running int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.total, d.done, d.failed, d.running
}

func (d *Downloader) Results() []DownloadResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]DownloadResult, len(d.results))
	copy(out, d.results)
	return out
}

func (d *Downloader) Cancel() {
	d.cancel()
}

// taskKey 生成任务去重键
func taskKey(t DownloadTask) string {
	if t.ID != "" {
		return "id:" + t.ID
	}
	if t.URL != "" {
		return "url:" + t.URL
	}
	return ""
}

// FailedResults 返回失败结果副本
func (d *Downloader) FailedResults() []DownloadResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := []DownloadResult{}
	for _, r := range d.results {
		if r.Err != nil {
			out = append(out, r)
		}
	}
	return out
}

// ClearResults 清空历史结果与计数（不影响进行中的任务）
func (d *Downloader) ClearResults() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.results = nil
	d.total = 0
	d.done = 0
	d.failed = 0
}
