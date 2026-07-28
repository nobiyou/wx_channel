package poc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCollectCommentsMapsTopLevelAndReplies(t *testing.T) {
	api := &fixturePageAPI{responses: [][]byte{
		readFixture(t, "comments-top.json"),
		[]byte(`{"data":{"commentInfo":[],"countInfo":{"commentCount":2},"lastBuffer":""}}`),
		readFixture(t, "comments-replies.json"),
	}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "comments-map-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-work-1", "fixture-nonce-1", 1))
	if err != nil {
		t.Fatal(err)
	}
	if summary.TopLevel != 2 || summary.Replies != 2 {
		t.Fatalf("summary=%+v", summary)
	}
	if !summary.Partial || !containsReason(summary.Reasons, "missing_root_comment_id") {
		t.Fatalf("missing-ID reply gap not reported: %+v", summary)
	}
	if api.calls != 3 {
		t.Fatalf("calls=%d", api.calls)
	}
	reply := findComment(t, comments, "fixture-reply-2")
	if reply.Level != 2 || dereference(reply.ParentCommentID) != "fixture-top-1" || dereference(reply.RootCommentID) != "fixture-top-1" {
		t.Fatalf("reply relation=%+v", reply)
	}
	if dereference(reply.RetrievalRootCommentID) != "fixture-top-1" {
		t.Fatalf("retrieval root=%v", reply.RetrievalRootCommentID)
	}
	if dereference(reply.Account.AvatarURL) != "https://fixture.invalid/avatar/reply-2.png" {
		t.Fatalf("unsafe avatar=%v", reply.Account.AvatarURL)
	}
	if countCommentID(comments, "fixture-reply-1") != 1 {
		t.Fatalf("embedded duplicate was not removed: %+v", comments)
	}
	for _, comment := range comments {
		if comment.CommentID == nil && comment.Content.MediaType.Normalized != "image" {
			t.Fatalf("missing-ID media comment=%+v", comment)
		}
	}
}

func TestMissingCommentIDIsNotSynthesizedOrCrossPageDeduped(t *testing.T) {
	first := commentPage(t, []map[string]any{{"content": "fixture-missing-1", "contentType": 1}}, "fixture-next")
	second := commentPage(t, []map[string]any{{"content": "fixture-missing-2", "contentType": 1}}, "")
	api := &fixturePageAPI{responses: [][]byte{first, second}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "missing-id-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-work-2", "fixture-nonce-2", 1))
	if err != nil {
		t.Fatal(err)
	}
	if summary.TopLevel != 2 || len(comments) != 2 {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
	}
	for _, comment := range comments {
		if comment.CommentID != nil {
			t.Fatalf("comment ID was synthesized: %q", *comment.CommentID)
		}
	}
}

func TestTopLevelStopsAt100AndMarksTruncated(t *testing.T) {
	items := make([]map[string]any, 101)
	for index := range items {
		items[index] = map[string]any{"commentId": fmt.Sprintf("fixture-top-%03d", index+1), "content": "fixture-limit", "contentType": 1}
	}
	api := &fixturePageAPI{responses: [][]byte{commentPage(t, items, "fixture-more")}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "top-limit-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-work-3", "fixture-nonce-3", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 100 || summary.TopLevel != 100 || !summary.Truncated || !containsReason(summary.Reasons, "top_level_limit") {
		t.Fatalf("count=%d summary=%+v", len(comments), summary)
	}
	if api.calls != 1 {
		t.Fatalf("calls=%d", api.calls)
	}
}

func TestExactTopLevelLimitStillMarksTruncated(t *testing.T) {
	items := make([]map[string]any, 100)
	for index := range items {
		items[index] = map[string]any{"commentId": fmt.Sprintf("fixture-exact-top-%03d", index+1), "content": "fixture-exact-limit"}
	}
	api := &fixturePageAPI{responses: [][]byte{commentPage(t, items, "")}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "exact-top-limit-job"), &fixtureClock{})
	_, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-work-exact", "fixture-nonce-exact", 1))
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Truncated || !containsReason(summary.Reasons, "top_level_limit") {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestRepliesStopAt200PerWorkAndMarksTruncated(t *testing.T) {
	top := []map[string]any{{"commentId": "fixture-root", "content": "fixture-root", "contentType": 1, "expandCommentCount": 201}}
	replies := make([]map[string]any, 201)
	for index := range replies {
		replies[index] = map[string]any{
			"commentId":      fmt.Sprintf("fixture-reply-%03d", index+1),
			"replyCommentId": "fixture-root",
			"rootCommentId":  "fixture-root",
			"content":        "fixture-reply-limit",
			"contentType":    1,
		}
	}
	api := &fixturePageAPI{responses: [][]byte{commentPage(t, top, ""), commentPage(t, replies, "")}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "reply-limit-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-work-4", "fixture-nonce-4", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 201 || summary.TopLevel != 1 || summary.Replies != 200 || !summary.Truncated || !containsReason(summary.Reasons, "reply_limit") {
		t.Fatalf("count=%d summary=%+v", len(comments), summary)
	}
	if api.calls != 2 {
		t.Fatalf("calls=%d", api.calls)
	}
}

func TestParentAndRootRemainNullWhenSourceOmitsThem(t *testing.T) {
	workID := "fixture-work-5"
	retrievalRoot := "fixture-retrieval-root"
	comment, _ := mapComment(map[string]any{
		"commentId": "fixture-reply-without-relations",
		"content":   "fixture-reply",
	}, 2, &workID, &retrievalRoot, SourceRef{Method: commentListMethod, EvidenceRef: "evidence/fixture.json", Ordinal: 1})
	if comment.ParentCommentID != nil || comment.RootCommentID != nil {
		t.Fatalf("source relations were invented: %+v", comment)
	}
	if dereference(comment.RetrievalRootCommentID) != retrievalRoot {
		t.Fatalf("retrieval root=%v", comment.RetrievalRootCommentID)
	}

	zero, _ := mapComment(map[string]any{"replyCommentId": "0", "rootCommentId": "0"}, 2, &workID, nil, SourceRef{})
	if zero.ParentCommentID != nil || zero.RootCommentID != nil {
		t.Fatalf("zero relations were not normalized: %+v", zero)
	}
}

func TestIPRegionAndTimePreserveRawSourceSemantics(t *testing.T) {
	workID := "fixture-work-6"
	valid, fields := mapComment(map[string]any{
		"createtime":   "1715760000",
		"ipRegion":     "fixture-fallback-region",
		"ipRegionInfo": map[string]any{"regionText": "fixture-primary-region"},
	}, 1, &workID, nil, SourceRef{})
	if dereference(valid.CreatedAt.Raw) != "1715760000" || valid.CreatedAt.UnixSeconds == nil || *valid.CreatedAt.UnixSeconds != 1715760000 {
		t.Fatalf("valid time=%+v", valid.CreatedAt)
	}
	if dereference(valid.CreatedAt.ISO8601) != "2024-05-15T08:00:00Z" {
		t.Fatalf("ISO time=%v", valid.CreatedAt.ISO8601)
	}
	if dereference(valid.IPLocation.Label) != "fixture-primary-region" || statusFor(fields, "comments[].created_at") != FieldPresent {
		t.Fatalf("IP/time fields=%+v results=%+v", valid, fields)
	}

	invalid, invalidFields := mapComment(map[string]any{"createtime": "fixture-not-unix", "ipRegion": "fixture-fallback-region"}, 1, &workID, nil, SourceRef{})
	if dereference(invalid.CreatedAt.Raw) != "fixture-not-unix" || invalid.CreatedAt.UnixSeconds != nil || invalid.CreatedAt.ISO8601 != nil {
		t.Fatalf("invalid time semantics=%+v", invalid.CreatedAt)
	}
	if statusFor(invalidFields, "comments[].created_at") != FieldInvalidFormat || dereference(invalid.IPLocation.Label) != "fixture-fallback-region" {
		t.Fatalf("invalid fields=%+v comment=%+v", invalidFields, invalid)
	}
}

func fixtureWork(id, nonce string, rank int) Work {
	return Work{WorkID: &id, ObjectNonceID: &nonce, Locator: WorkLocator{Keyword: "fixture-keyword", SearchRank: rank}}
}

func commentPage(t *testing.T, items []map[string]any, marker string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"data": map[string]any{
		"commentInfo": items,
		"countInfo":   map[string]any{"commentCount": len(items)},
		"lastBuffer":  marker,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func findComment(t *testing.T, comments []Comment, id string) Comment {
	t.Helper()
	for _, comment := range comments {
		if comment.CommentID != nil && *comment.CommentID == id {
			return comment
		}
	}
	t.Fatalf("comment %q not found", id)
	return Comment{}
}

func countCommentID(comments []Comment, id string) int {
	count := 0
	for _, comment := range comments {
		if comment.CommentID != nil && *comment.CommentID == id {
			count++
		}
	}
	return count
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func statusFor(results []FieldResult, path string) FieldStatus {
	for _, result := range results {
		if strings.EqualFold(result.Path, path) {
			return result.Status
		}
	}
	return ""
}

func TestCommentFixtureTimestampIsStable(t *testing.T) {
	if time.Unix(1715760000, 0).UTC().Format(time.RFC3339) != "2024-05-15T08:00:00Z" {
		t.Fatal("fixture timestamp changed")
	}
}
