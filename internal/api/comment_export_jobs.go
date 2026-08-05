package api

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"wx_channel/internal/utils"
)

const (
	commentExportQueued   = "queued"
	commentExportRunning  = "running"
	commentExportSuccess  = "succeeded"
	commentExportFailed   = "failed"
	commentExportQueueMax = 4
)

// CommentExportProgress describes the durable part of an export's progress.
type CommentExportProgress struct {
	Stage            string `json:"stage"`
	TopLevelCount    int    `json:"top_level_count"`
	ReplyCount       int    `json:"reply_count"`
	ReportedCount    int    `json:"reported_count"`
	CompletedReplies int    `json:"completed_replies"`
	TotalReplies     int    `json:"total_replies"`
}

// CommentExportJobStatus is returned to the browser while an export runs.
type CommentExportJobStatus struct {
	JobID     string                    `json:"job_id"`
	Status    string                    `json:"status"`
	Progress  CommentExportProgress     `json:"progress"`
	Result    *ExportFeedCommentsResult `json:"result,omitempty"`
	Error     string                    `json:"error,omitempty"`
	CreatedAt string                    `json:"created_at"`
	UpdatedAt string                    `json:"updated_at"`
}

type commentExportJob struct {
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	request ExportFeedCommentsRequest
	status  CommentExportJobStatus
}

// CommentExportJobManager serializes comment exports against the page client.
// The WeChat page API is stateful, so unbounded parallel exports are unsafe.
type CommentExportJobManager struct {
	service *SearchService
	ctx     context.Context
	cancel  context.CancelFunc
	queue   chan *commentExportJob

	mu   sync.RWMutex
	jobs map[string]*commentExportJob
	seq  uint64
}

func NewCommentExportJobManager(service *SearchService) *CommentExportJobManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &CommentExportJobManager{
		service: service,
		ctx:     ctx,
		cancel:  cancel,
		queue:   make(chan *commentExportJob, commentExportQueueMax),
		jobs:    make(map[string]*commentExportJob),
	}
	go m.worker()
	return m
}

func (m *CommentExportJobManager) Submit(req ExportFeedCommentsRequest) (CommentExportJobStatus, error) {
	if m == nil || m.service == nil {
		return CommentExportJobStatus{}, fmt.Errorf("comment export service is not available")
	}

	now := time.Now()
	jobID := fmt.Sprintf("comment-%d-%d", now.UnixNano(), atomic.AddUint64(&m.seq, 1))
	ctx, cancel := context.WithCancel(m.ctx)
	job := &commentExportJob{
		ctx:     ctx,
		cancel:  cancel,
		request: req,
		status: CommentExportJobStatus{
			JobID:     jobID,
			Status:    commentExportQueued,
			Progress:  CommentExportProgress{Stage: "queued"},
			CreatedAt: now.Format(time.RFC3339),
			UpdatedAt: now.Format(time.RFC3339),
		},
	}

	m.mu.Lock()
	m.pruneLocked(now)
	m.jobs[jobID] = job
	m.mu.Unlock()

	select {
	case m.queue <- job:
		return job.snapshot(), nil
	default:
		job.setFailed("comment export queue is full")
		return CommentExportJobStatus{}, fmt.Errorf("comment export queue is full, please retry later")
	}
}

func (m *CommentExportJobManager) Get(jobID string) (CommentExportJobStatus, bool) {
	if m == nil {
		return CommentExportJobStatus{}, false
	}
	m.mu.RLock()
	job, ok := m.jobs[jobID]
	m.mu.RUnlock()
	if !ok {
		return CommentExportJobStatus{}, false
	}
	return job.snapshot(), true
}

func (m *CommentExportJobManager) worker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case job := <-m.queue:
			if job == nil {
				continue
			}
			m.run(job)
		}
	}
}

func (m *CommentExportJobManager) run(job *commentExportJob) {
	job.setStatus(commentExportRunning, CommentExportProgress{Stage: "starting"})
	result, err := m.service.exportFeedCommentsContext(job.ctx, job.request, func(progress CommentExportProgress) {
		job.setProgress(progress)
	})
	if err != nil {
		message := normalizePageContextAPIError(err).Error()
		job.setFailed(message)
		status := job.snapshot()
		utils.LogComment(status.JobID, job.request.Title, status.Progress.TopLevelCount+status.Progress.ReplyCount, false)
		return
	}
	job.setSuccess(result)
}

func (j *commentExportJob) snapshot() CommentExportJobStatus {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status
}

func (j *commentExportJob) setStatus(status string, progress CommentExportProgress) {
	j.mu.Lock()
	j.status.Status = status
	j.status.Progress = progress
	j.status.UpdatedAt = time.Now().Format(time.RFC3339)
	j.mu.Unlock()
}

func (j *commentExportJob) setProgress(progress CommentExportProgress) {
	j.mu.Lock()
	j.status.Progress = progress
	j.status.UpdatedAt = time.Now().Format(time.RFC3339)
	j.mu.Unlock()
}

func (j *commentExportJob) setSuccess(result *ExportFeedCommentsResult) {
	j.mu.Lock()
	j.status.Status = commentExportSuccess
	j.status.Progress.Stage = "completed"
	j.status.Result = result
	j.status.UpdatedAt = time.Now().Format(time.RFC3339)
	j.mu.Unlock()
	j.cancel()
}

func (j *commentExportJob) setFailed(message string) {
	j.mu.Lock()
	j.status.Status = commentExportFailed
	j.status.Error = message
	j.status.Progress.Stage = "failed"
	j.status.UpdatedAt = time.Now().Format(time.RFC3339)
	j.mu.Unlock()
	j.cancel()
}

func (m *CommentExportJobManager) pruneLocked(now time.Time) {
	cutoff := now.Add(-30 * time.Minute)
	for id, job := range m.jobs {
		status := job.snapshot()
		if status.Status != commentExportSuccess && status.Status != commentExportFailed {
			continue
		}
		updated, err := time.Parse(time.RFC3339, status.UpdatedAt)
		if err == nil && updated.Before(cutoff) {
			delete(m.jobs, id)
		}
	}
}
