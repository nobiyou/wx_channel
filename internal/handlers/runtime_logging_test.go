package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"wx_channel/internal/database"
	"wx_channel/internal/utils"

	"github.com/qtgolang/SunnyNet/SunnyNet"
	sunnyPublic "github.com/qtgolang/SunnyNet/public"
)

// A subprocess isolates the process-wide logger and stdout from other handler tests.
func TestQuietRuntimeLogging(t *testing.T) {
	if os.Getenv("WX_CHANNEL_RUNTIME_LOG_TEST") == "1" {
		if err := utils.InitLoggerWithRotation(utils.INFO, "runtime.log", 5); err != nil {
			t.Fatal(err)
		}
		defer utils.GetLogger().Close()
		t.Cleanup(databaseTestSetupForHandlers(t))
		h := &APIHandler{}
		for _, body := range []string{
			`{"level":"debug","msg":"tip-detail-marker"}`,
			`{"level":"warn","msg":"tip-warning-marker"}`,
			`{"level":"error","msg":"tip-failure-marker"}`,
			`{"msg":"tip-legacy-marker"}`,
		} {
			conn := &SunnyNet.HttpConn{
				Type:    sunnyPublic.HttpSendRequest,
				Request: httptest.NewRequest(http.MethodPost, "/__wx_channels_api/tip", strings.NewReader(body)),
			}
			if !h.HandleTip(conn) || conn.Response == nil || conn.Response.StatusCode != http.StatusOK {
				t.Fatalf("tip endpoint did not accept message: %s", body)
			}
		}
		h.processVideoData(map[string]any{
			"id": "quiet-video", "title": "quiet-title", "nickname": "quiet-author",
			"key": "private-decryption-key", "url": "https://example.com/quiet-video",
			"likeCount": float64(12), "commentCount": float64(3),
		})
		record, err := database.NewBrowseHistoryRepository().GetByID("quiet-video")
		if err != nil || record == nil || record.LikeCount != 12 || record.CommentCount != 3 || record.DecryptKey != "private-decryption-key" {
			t.Fatalf("quiet browsing lost persisted data: %+v, %v", record, err)
		}
		batch := &BatchHandler{tasks: []BatchTask{{ID: "quiet-video", Title: "batch-title", Status: "downloading"}}}
		progress := batch.downloadProgressCallback(0)
		for _, fraction := range []float64{0.09, 0.10, 0.101, 0.109, 0.199, 0.20, 0.209, 1, 1} {
			progress(fraction, int64(fraction*1000), 1000)
		}
		if task := batch.tasks[0]; task.Progress != 100 || task.TotalMB == 0 || task.DownloadedMB != task.TotalMB {
			t.Fatalf("quiet logging lost download progress: %+v", task)
		}
		batch.tasks[0].Status = "done"
		progress(0.5, 500, 1000)
		if batch.tasks[0].Progress != 100 {
			t.Fatal("late progress overwrote completed task")
		}
		content, err := os.ReadFile("runtime.log")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "quiet-title") || !strings.Contains(string(content), "tip-detail-marker") || strings.Contains(string(content), "private-decryption-key") {
			t.Fatalf("unexpected video diagnostic: %s", content)
		}
		if got := strings.Count(string(content), "[批量下载] batch-title 进度:"); got != 3 {
			t.Fatalf("expected one log per crossed 10%% bucket, got %d: %s", got, content)
		}
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestQuietRuntimeLogging$")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "WX_CHANNEL_RUNTIME_LOG_TEST=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("runtime logging regression: %v\n%s", err, output)
	}
	for _, marker := range []string{"quiet-title", "quiet-author", "quiet-video", "batch-title", "private-decryption-key", "视频详细信息", "视频已成功播放", "tip-detail-marker"} {
		if strings.Contains(string(output), marker) {
			t.Errorf("routine diagnostic leaked to console: %s", marker)
		}
	}
	for _, marker := range []string{"tip-warning-marker", "tip-failure-marker", "tip-legacy-marker"} {
		if strings.Count(string(output), marker) != 1 {
			t.Errorf("expected one visible frontend tip: %s\n%s", marker, output)
		}
	}
}
