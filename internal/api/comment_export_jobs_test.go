package api

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestCommentExportJobManagerCompletesAndPersistsResult(t *testing.T) {
	t.Parallel()

	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			if key != "key:channels:fetch_feed_comment_list" {
				t.Fatalf("unexpected API key: %s", key)
			}
			return []byte(`{"errCode":0,"data":{"commentInfo":[{"commentId":"c1","content":"hello","expandCommentCount":0}],"countInfo":{"commentCount":1},"lastBuffer":""}}`), nil
		},
		resolveDownloadsDir: func() (string, error) {
			return t.TempDir(), nil
		},
	}
	manager := NewCommentExportJobManager(service)
	job, err := manager.Submit(ExportFeedCommentsRequest{
		ObjectID: "oid-1",
		NonceID:  "nid-1",
		Title:    "异步评论测试",
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if job.Status != commentExportQueued || job.JobID == "" {
		t.Fatalf("unexpected queued job: %#v", job)
	}

	var completed CommentExportJobStatus
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, ok := manager.Get(job.JobID)
		if !ok {
			t.Fatalf("job %q disappeared", job.JobID)
		}
		if status.Status == commentExportSuccess || status.Status == commentExportFailed {
			completed = status
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if completed.Status != commentExportSuccess {
		t.Fatalf("job status = %q, error = %q", completed.Status, completed.Error)
	}
	if completed.Result == nil || completed.Result.TotalCount != 1 {
		t.Fatalf("unexpected result: %#v", completed.Result)
	}
	if _, err := os.Stat(completed.Result.SavedPath); err != nil {
		t.Fatalf("saved result missing: %v", err)
	}

	raw, err := os.ReadFile(completed.Result.SavedPath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	var saved commentExportFile
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(saved.CommentInfo) != 1 || saved.CommentInfo[0].Content != "hello" {
		t.Fatalf("unexpected saved comments: %#v", saved.CommentInfo)
	}
}
