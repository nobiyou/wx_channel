package poc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type simulatedPOCResult struct {
	dataset Dataset
	store   *Store
}

func TestSimulatedPOCWritesSafeStructuredOutput(t *testing.T) {
	result := runSimulatedPOC(t)
	if result.dataset.Job.Status != JobCompleted || result.dataset.Job.CapabilityStatus != CapabilityVerified || result.dataset.Job.CoverageStatus != CoverageSourceExhausted {
		t.Fatalf("job=%+v", result.dataset.Job)
	}
	if len(result.dataset.Works) != 3 || len(result.dataset.Comments) != 6 {
		t.Fatalf("works=%d comments=%d", len(result.dataset.Works), len(result.dataset.Comments))
	}
	if _, err := os.Stat(filepath.Join(result.store.JobDir(), "raw-evidence")); !os.IsNotExist(err) {
		t.Fatalf("raw evidence unexpectedly persisted: %v", err)
	}
}

func TestSimulatedPOCEnforces100And200Caps(t *testing.T) {
	result := runSimulatedPOC(t)
	for _, work := range result.dataset.Works {
		if work.TopLevelCommentCount > 100 || work.ReplyCount > 200 {
			t.Fatalf("work exceeded approved caps: %+v", work)
		}
	}
}

func TestSimulatedPOCOutputReferencesEveryEvidenceHash(t *testing.T) {
	result := runSimulatedPOC(t)
	for _, comment := range result.dataset.Comments {
		path := filepath.Join(result.store.JobDir(), filepath.FromSlash(comment.Source.EvidenceRef))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var evidence Evidence
		if err := json.Unmarshal(raw, &evidence); err != nil || len(evidence.SourceResponseSHA256) != 64 {
			t.Fatalf("invalid evidence reference %q", comment.Source.EvidenceRef)
		}
	}
}

func TestSimulatedPOCOrdinaryFilesPassSecretScanner(t *testing.T) {
	result := runSimulatedPOC(t)
	err := filepath.WalkDir(result.store.JobDir(), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return ScanOrdinaryOutput(raw)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSimulatedPOCContextWaitTimeoutCleansUp(t *testing.T) {
	clock := newManualClock()
	waiter := NewWaitController(clock, HumanWaitPolicy{Timeout: 300 * time.Second}, nil)
	done := make(chan WaitResult, 1)
	go func() { done <- waiter.Wait(context.Background(), WaitTargetContext, 1, nil) }()
	clock.waitTimer(t)
	clock.Advance(300 * time.Second)
	if result := <-done; result != WaitTimedOut {
		t.Fatalf("result=%s", result)
	}
}

func runSimulatedPOC(t *testing.T) simulatedPOCResult {
	t.Helper()
	server, httpServer, token := newTestBridge(t)
	conn := connectReadyBridge(t, server, httpServer.URL, token, map[string]bool{
		"finderSearch": true, "finderUserPage": true, "finderGetCommentDetail": true, "finderGetCommentList": true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = conn.CloseNow()
		_ = server.Close()
		httpServer.Close()
	})
	go serveSimulatedPage(ctx, conn)

	store := newTestStore(t, "simulated-poc-job")
	collector := NewCollector(server, NewEvidenceRecorder(nil), store, &fixtureClock{})
	options := approvedTestOptions()
	works, coverage, err := collector.CollectWorks(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	comments := make([]Comment, 0)
	for index := range works {
		collected, summary, err := collector.CollectComments(context.Background(), options, works[index])
		if err != nil {
			t.Fatal(err)
		}
		works[index].CollectionStatus = "completed"
		works[index].TopLevelCommentCount = summary.TopLevel
		works[index].ReplyCount = summary.Replies
		comments = append(comments, collected...)
	}
	job, capability, finalCoverage, reasons := EvaluateOutcome(OutcomeInput{
		SearchComplete: true, SourceExhausted: coverage == CoverageSourceExhausted, ValidWorks: len(works), CompletedWorks: len(works),
		TopLevelComments: 3, Replies: 3, RequiredFieldStatuses: requiredFieldStatuses(comments),
	})
	dataset := Dataset{SchemaVersion: SchemaVersion, Job: Job{
		JobID: "simulated-poc-job", Keyword: options.Keyword, Status: job, CapabilityStatus: capability,
		CoverageStatus: finalCoverage, Limits: options.Limits,
	}, Works: works, Comments: comments}
	validation := Validation{JobID: "simulated-poc-job", CapabilityStatus: capability, CoverageStatus: finalCoverage, ReasonCodes: reasons}
	if err := store.WriteDataset(dataset); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteValidation(validation); err != nil {
		t.Fatal(err)
	}
	return simulatedPOCResult{dataset: dataset, store: store}
}

func serveSimulatedPage(ctx context.Context, conn *websocket.Conn) {
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var envelope struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Type != "api_call" {
			continue
		}
		var call struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Body   map[string]any `json:"body"`
		}
		if json.Unmarshal(envelope.Data, &call) != nil {
			continue
		}
		var data any
		switch call.Method {
		case "finderSearch":
			data = map[string]any{"data": map[string]any{"objectList": []any{
				map[string]any{"id": "fixture-sim-work-1", "objectNonceId": "fixture-sim-nonce-1", "objectDesc": map[string]any{"mediaType": 2}},
				map[string]any{"id": "fixture-sim-work-2", "objectNonceId": "fixture-sim-nonce-2", "objectDesc": map[string]any{"mediaType": 4}},
				map[string]any{"id": "fixture-sim-work-3", "objectNonceId": "fixture-sim-nonce-3", "objectDesc": map[string]any{"mediaType": 9}},
			}, "lastBuffer": ""}}
		case "finderUserPage":
			data = map[string]any{"data": map[string]any{"object": []any{}, "lastBuffer": ""}}
		case "finderGetCommentList":
			workID, _ := call.Body["object_id"].(string)
			rootID := "fixture-sim-top-" + strings.TrimPrefix(workID, "fixture-sim-work-")
			data = map[string]any{"data": map[string]any{"commentInfo": []any{map[string]any{
				"commentId": rootID, "content": "fixture-sim-content", "contentType": 1,
				"username": "fixture-sim-account", "nickname": "fixture-sim-name", "createtime": "1715760000", "ipRegion": "fixture-sim-region",
				"expandCommentCount": 1, "levelTwoComment": []any{map[string]any{
					"commentId": "fixture-sim-reply-" + strings.TrimPrefix(workID, "fixture-sim-work-"), "replyCommentId": rootID, "rootCommentId": rootID,
					"content": "fixture-sim-reply-content", "contentType": 1, "username": "fixture-sim-reply-account", "createtime": "1715760001", "ipRegion": "fixture-sim-region",
				}},
			}}, "countInfo": map[string]any{"commentCount": 1}, "lastBuffer": ""}}
		default:
			data = map[string]any{}
		}
		response := map[string]any{"type": "api_response", "data": map[string]any{"id": call.ID, "data": data, "errCode": 0}}
		encoded, _ := json.Marshal(response)
		if conn.Write(ctx, websocket.MessageText, encoded) != nil {
			return
		}
	}
}
