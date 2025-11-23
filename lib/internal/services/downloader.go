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
	ID             string
	URL            string
	Filename       string
	AuthorName     string
	Decryptor      []byte // 可选：前缀解密数组
	ForceRedownload bool  // 是否强制重新下载（即使文件已存在）
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
	
	// worker 数量（用于跟踪）
	workerCount int
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
	d.workerCount = workers
	for i := 0; i < workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
	return d
}

func (d *Downloader) Enqueue(tasks []DownloadTask) {
	d.mu.Lock()
	// 检查是否有强制重新下载的任务
	hasForceRedownload := false
	for _, t := range tasks {
		if t.ForceRedownload {
			hasForceRedownload = true
			break
		}
	}
	// 如果有强制重新下载的任务，清空去重集合，允许重新下载
	if hasForceRedownload {
		utils.Info("[批量下载] 检测到强制重新下载任务，清空去重集合")
		d.seenKeys = make(map[string]struct{})
	}
	
	// 去重：同ID优先，否则用URL（但强制重新下载的任务不受此限制）
	filtered := make([]DownloadTask, 0, len(tasks))
	for _, t := range tasks {
		key := taskKey(t)
		if key == "" {
			continue
		}
		// 如果强制重新下载，跳过去重检查
		if !t.ForceRedownload {
			if _, ok := d.seenKeys[key]; ok {
				continue
			}
		}
		// 只有非强制重新下载的任务才加入去重集合
		if !t.ForceRedownload {
			d.seenKeys[key] = struct{}{}
		}
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
	urlShort := task.URL
	if len(urlShort) > 80 {
		urlShort = urlShort[:80] + "..."
	}
	utils.Info("[批量下载] 开始下载: ID=%s, 标题=%s, 作者=%s, URL=%s", task.ID, task.Filename, task.AuthorName, urlShort)
	var lastErr error
	retries := d.cfg.DownloadRetryCount
	if retries <= 0 {
		retries = 1
	}

	for attempt := 1; attempt <= retries; attempt++ {
		path, size, err := d.tryDownload(client, task)
		if err == nil {
			utils.Info("[批量下载] 下载成功: ID=%s, 标题=%s, 路径=%s, 大小=%.2fMB", task.ID, task.Filename, path, size)
			// 记录详细下载日志
			utils.LogDownload(task.ID, task.Filename, task.AuthorName, task.URL, int64(size*1024*1024), true)
			return DownloadResult{Task: task, Path: path, SizeMB: size, Err: nil}
		}
		lastErr = err
		utils.Warn("[批量下载] 下载失败(%d/%d): ID=%s, 标题=%s, 错误=%v", attempt, retries, task.ID, task.Filename, err)
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
	}
	utils.Error("[批量下载] 最终失败: ID=%s, 标题=%s, 作者=%s, 错误=%v", task.ID, task.Filename, task.AuthorName, lastErr)
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
	if !task.ForceRedownload {
		// 如果不强制重新下载，检查文件是否已存在
		if fi, err := os.Stat(preferredPath); err == nil {
			if fi.Size() > 0 {
				utils.Info("[批量下载] 文件已存在，跳过: ID=%s, 路径=%s, 大小=%.2fMB", task.ID, preferredPath, float64(fi.Size())/(1024*1024))
				return preferredPath, float64(fi.Size()) / (1024 * 1024), nil
			}
		}
	} else {
		// 强制重新下载：如果文件已存在，删除旧文件
		if fi, err := os.Stat(preferredPath); err == nil {
			if fi.Size() > 0 {
				utils.Info("[批量下载] 强制重新下载: ID=%s, 删除旧文件: %s (%.2fMB)", task.ID, preferredPath, float64(fi.Size())/(1024*1024))
				if err := os.Remove(preferredPath); err != nil {
					utils.Warn("[批量下载] 删除旧文件失败: ID=%s, 路径=%s, 错误=%v", task.ID, preferredPath, err)
				}
			}
		}
	}
	finalPath := preferredPath
	// 只有在不强制重新下载且文件已存在时，才生成唯一文件名
	if !task.ForceRedownload {
		if _, err := os.Stat(finalPath); err == nil {
			finalPath = utils.GenerateUniqueFilename(saveDir, cleanFilename, 1000)
		}
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
	hasDecryptor := len(task.Decryptor) > 0
	utils.Info("[批量下载] 开始写入文件: ID=%s, 路径=%s, 是否解密=%v, 解密长度=%d", task.ID, finalPath, hasDecryptor, len(task.Decryptor))
	if hasDecryptor {
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
	sizeMB := float64(n) / (1024 * 1024)
	utils.Info("[批量下载] 文件保存完成: ID=%s, 路径=%s, 大小=%.2fMB", task.ID, finalPath, sizeMB)

	// 写入 CSV 记录
	if d.csv != nil {
		rec := &models.VideoDownloadRecord{
			ID:         task.ID,
			Title:      task.Filename,
			Author:     task.AuthorName,
			URL:        task.URL,
			PageURL:    "",
			FileSize:   fmt.Sprintf("%.2f MB", sizeMB),
			DownloadAt: time.Now(),
		}
		if err := d.csv.AddRecord(rec); err != nil {
			utils.Warn("[批量下载] CSV记录保存失败: ID=%s, 错误=%v", task.ID, err)
		} else {
			utils.Info("[批量下载] CSV记录已保存: ID=%s", task.ID)
		}
	}

	return finalPath, sizeMB, nil
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
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
	}
	// 清空队列中的剩余任务
	queueLen := len(d.queue)
	for i := 0; i < queueLen; i++ {
		select {
		case <-d.queue:
		default:
			break
		}
	}
	// 清空去重集合，允许重新下载相同的任务
	d.seenKeys = make(map[string]struct{})
	// 重置统计计数（但保留已完成的下载记录）
	// 注意：不重置 total/done/failed，因为这些是历史统计
	// 但需要重置 running，因为任务已取消
	d.running = 0
	utils.Info("[批量下载] 取消操作：已清空队列（%d个任务）、去重集合，重置运行计数", queueLen)
	// 注意：不在这里重新创建 context，而是在下次 Enqueue 时检查并重置
}

// Reset 重置下载器状态，重新创建 context 和启动 worker
func (d *Downloader) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	// 如果 context 已取消，重新创建
	needReset := false
	if d.ctx != nil {
		select {
		case <-d.ctx.Done():
			// context 已取消，需要重新创建
			needReset = true
		default:
			// context 未取消，无需重置
		}
	} else {
		// context 不存在，需要创建
		needReset = true
	}
	
	if needReset {
		utils.Info("[批量下载] 重置下载器：清空状态并重新创建 context 和 worker")
		// 清空队列中的剩余任务
		queueLen := len(d.queue)
		for i := 0; i < queueLen; i++ {
			select {
			case <-d.queue:
			default:
				break
			}
		}
		// 清空去重集合，允许重新下载相同的任务
		d.seenKeys = make(map[string]struct{})
		// 重置统计计数
		d.total = 0
		d.done = 0
		d.failed = 0
		d.running = 0
		d.results = nil
		utils.Info("[批量下载] 已清空队列（%d个任务）、去重集合和统计计数", queueLen)
		
		d.ctx, d.cancel = context.WithCancel(context.Background())
		// 重新启动 worker
		workers := d.cfg.DownloadConcurrency
		if workers <= 0 {
			workers = 2
		}
		d.workerCount = workers
		for i := 0; i < workers; i++ {
			d.wg.Add(1)
			go d.worker()
		}
	}
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
