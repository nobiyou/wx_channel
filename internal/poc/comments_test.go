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

func TestTopLevelRepeatedMarkerWithNoNewComments(t *testing.T) {
	root := map[string]any{"commentId": "fixture-repeat-root", "content": "fixture-root"}
	api := &fixturePageAPI{responses: [][]byte{
		commentPage(t, []map[string]any{root}, "fixture-top-repeat"),
		commentPage(t, []map[string]any{root}, "fixture-top-repeat"),
	}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "top-repeat-terminal-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-top-repeat-work", "fixture-top-repeat-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || summary.TopLevel != 1 || summary.Partial || containsReason(summary.Reasons, "comment_pagination_repeated_marker") {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
	}
	if api.calls != 2 {
		t.Fatalf("calls=%d", api.calls)
	}
}

func TestTopLevelRepeatedMarkerWithNewCommentsRemainsPartial(t *testing.T) {
	api := &fixturePageAPI{responses: [][]byte{
		commentPage(t, []map[string]any{{"commentId": "fixture-repeat-root", "content": "fixture-root"}}, "fixture-top-repeat-new"),
		commentPage(t, []map[string]any{
			{"commentId": "fixture-repeat-root", "content": "fixture-duplicate"},
			{"commentId": "fixture-repeat-new", "content": "fixture-new"},
		}, "fixture-top-repeat-new"),
	}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "top-repeat-new-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-top-repeat-new-work", "fixture-top-repeat-new-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 || summary.TopLevel != 2 || !summary.Partial || !containsReason(summary.Reasons, "comment_pagination_repeated_marker") {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
	}
}

func TestReplyRepeatedMarkerWithNoNewReplies(t *testing.T) {
	top := []map[string]any{{"commentId": "fixture-reply-repeat-root", "expandCommentCount": 2}}
	reply := []map[string]any{{"commentId": "fixture-reply-repeat-one", "replyCommentId": "fixture-reply-repeat-root", "rootCommentId": "fixture-reply-repeat-root"}}
	api := &fixturePageAPI{responses: [][]byte{
		commentPage(t, top, ""),
		commentPage(t, reply, "fixture-reply-repeat"),
		commentPage(t, reply, "fixture-reply-repeat"),
	}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "reply-repeat-terminal-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-reply-repeat-work", "fixture-reply-repeat-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 || summary.Replies != 1 || summary.Partial || containsReason(summary.Reasons, "reply_pagination_repeated_marker") {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
	}
	if api.calls != 3 {
		t.Fatalf("calls=%d", api.calls)
	}
}

func TestReplyRepeatedMarkerWithNewRepliesRemainsPartial(t *testing.T) {
	top := []map[string]any{{"commentId": "fixture-reply-repeat-new-root", "expandCommentCount": 3}}
	api := &fixturePageAPI{responses: [][]byte{
		commentPage(t, top, ""),
		commentPage(t, []map[string]any{{"commentId": "fixture-reply-repeat-new-one", "replyCommentId": "fixture-reply-repeat-new-root", "rootCommentId": "fixture-reply-repeat-new-root"}}, "fixture-reply-repeat-new"),
		commentPage(t, []map[string]any{
			{"commentId": "fixture-reply-repeat-new-one", "replyCommentId": "fixture-reply-repeat-new-root", "rootCommentId": "fixture-reply-repeat-new-root"},
			{"commentId": "fixture-reply-repeat-new-two", "replyCommentId": "fixture-reply-repeat-new-root", "rootCommentId": "fixture-reply-repeat-new-root"},
		}, "fixture-reply-repeat-new"),
	}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "reply-repeat-new-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-reply-repeat-new-work", "fixture-reply-repeat-new-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 3 || summary.Replies != 2 || !summary.Partial || !containsReason(summary.Reasons, "reply_pagination_repeated_marker") {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
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

func TestTopLevelStopsAt500AndMarksTruncated(t *testing.T) {
	items := make([]map[string]any, 501)
	for index := range items {
		items[index] = map[string]any{"commentId": fmt.Sprintf("fixture-expanded-top-%03d", index+1), "content": "fixture-expanded-limit", "contentType": 1}
	}
	options := approvedTestOptions()
	options.Limits.TopLevelCommentsPerWork = 500
	api := &fixturePageAPI{responses: [][]byte{commentPage(t, items, "fixture-more")}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "expanded-top-limit-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), options, fixtureWork("fixture-expanded-top-work", "fixture-expanded-top-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 500 || summary.TopLevel != 500 || !summary.Truncated || !containsReason(summary.Reasons, "top_level_limit") {
		t.Fatalf("count=%d summary=%+v", len(comments), summary)
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
	top := make([]map[string]any, 10)
	responses := make([][]byte, 1, 11)
	for rootIndex := range top {
		rootID := fmt.Sprintf("fixture-root-%02d", rootIndex+1)
		top[rootIndex] = map[string]any{"commentId": rootID, "content": "fixture-root", "contentType": 1, "expandCommentCount": 21}
		replies := make([]map[string]any, 21)
		for replyIndex := range replies {
			replies[replyIndex] = map[string]any{
				"commentId":      fmt.Sprintf("fixture-reply-%02d-%03d", rootIndex+1, replyIndex+1),
				"replyCommentId": rootID,
				"rootCommentId":  rootID,
				"content":        "fixture-reply-limit",
				"contentType":    1,
			}
		}
		responses = append(responses, commentPage(t, replies, ""))
	}
	responses[0] = commentPage(t, top, "")
	api := &fixturePageAPI{responses: responses}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "reply-limit-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), fixtureWork("fixture-work-4", "fixture-nonce-4", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 210 || summary.TopLevel != 10 || summary.Replies != 200 || !summary.Truncated ||
		!containsReason(summary.Reasons, "reply_per_comment_limit") || !containsReason(summary.Reasons, "reply_per_work_limit") {
		t.Fatalf("count=%d summary=%+v", len(comments), summary)
	}
	if api.calls != 11 {
		t.Fatalf("calls=%d", api.calls)
	}
}

func TestRepliesStopAt100PerCommentAndMarksTruncated(t *testing.T) {
	top := []map[string]any{{"commentId": "fixture-expanded-root", "content": "fixture-root", "expandCommentCount": 101}}
	replies := make([]map[string]any, 101)
	for index := range replies {
		replies[index] = map[string]any{
			"commentId": fmt.Sprintf("fixture-expanded-reply-%03d", index+1),
			"replyCommentId": "fixture-expanded-root",
			"rootCommentId": "fixture-expanded-root",
			"content": "fixture-expanded-reply-limit",
			"contentType": 1,
		}
	}
	options := approvedTestOptions()
	options.Limits.RepliesPerComment = 100
	api := &fixturePageAPI{responses: [][]byte{commentPage(t, top, ""), commentPage(t, replies, "")}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "expanded-reply-limit-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), options, fixtureWork("fixture-expanded-reply-work", "fixture-expanded-reply-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 101 || summary.Replies != 100 || !summary.Truncated || !containsReason(summary.Reasons, "reply_per_comment_limit") {
		t.Fatalf("count=%d summary=%+v", len(comments), summary)
	}
}

func TestRepliesStopAtPerCommentLimitAfterDeduplication(t *testing.T) {
	top := []map[string]any{{
		"commentId": "fixture-local-root", "content": "fixture-root", "expandCommentCount": 4,
		"commentList": []any{map[string]any{
			"commentId": "fixture-local-reply-1", "replyCommentId": "fixture-local-root", "rootCommentId": "fixture-local-root",
		}},
	}}
	replies := []map[string]any{
		{"commentId": "fixture-local-reply-1", "replyCommentId": "fixture-local-root", "rootCommentId": "fixture-local-root"},
		{"commentId": "fixture-local-reply-2", "replyCommentId": "fixture-local-root", "rootCommentId": "fixture-local-root"},
		{"commentId": "fixture-local-reply-3", "replyCommentId": "fixture-local-root", "rootCommentId": "fixture-local-root"},
	}
	options := approvedTestOptions()
	options.Limits.RepliesPerComment = 2
	options.Limits.RepliesPerWork = 10
	api := &fixturePageAPI{responses: [][]byte{commentPage(t, top, ""), commentPage(t, replies, "")}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "reply-local-limit-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), options, fixtureWork("fixture-local-work", "fixture-local-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 3 || summary.Replies != 2 || countCommentID(comments, "fixture-local-reply-1") != 1 ||
		!containsReason(summary.Reasons, "reply_per_comment_limit") || containsReason(summary.Reasons, "reply_per_work_limit") {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
	}
}

func TestRepliesStopAtPerWorkLimitAcrossRoots(t *testing.T) {
	top := []map[string]any{
		{"commentId": "fixture-global-root-1", "expandCommentCount": 3},
		{"commentId": "fixture-global-root-2", "expandCommentCount": 3},
	}
	rootReplies := func(root string) []map[string]any {
		return []map[string]any{
			{"commentId": root + "-reply-1", "replyCommentId": root, "rootCommentId": root},
			{"commentId": root + "-reply-2", "replyCommentId": root, "rootCommentId": root},
			{"commentId": root + "-reply-3", "replyCommentId": root, "rootCommentId": root},
		}
	}
	options := approvedTestOptions()
	options.Limits.RepliesPerComment = 2
	options.Limits.RepliesPerWork = 3
	api := &fixturePageAPI{responses: [][]byte{
		commentPage(t, top, ""),
		commentPage(t, rootReplies("fixture-global-root-1"), ""),
		commentPage(t, rootReplies("fixture-global-root-2"), ""),
	}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "reply-global-limit-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), options, fixtureWork("fixture-global-work", "fixture-global-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 5 || summary.TopLevel != 2 || summary.Replies != 3 ||
		!containsReason(summary.Reasons, "reply_per_comment_limit") || !containsReason(summary.Reasons, "reply_per_work_limit") {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
	}
}

func TestZeroReplyLimitsSkipReplyExpansion(t *testing.T) {
	top := []map[string]any{{"commentId": "fixture-no-reply-root", "expandCommentCount": 5}}
	options := approvedTestOptions()
	options.Limits.RepliesPerComment = 0
	options.Limits.RepliesPerWork = 0
	api := &fixturePageAPI{responses: [][]byte{commentPage(t, top, "")}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "reply-disabled-job"), &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), options, fixtureWork("fixture-no-reply-work", "fixture-no-reply-nonce", 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || summary.TopLevel != 1 || summary.Replies != 0 || summary.Truncated || api.calls != 1 {
		t.Fatalf("comments=%+v summary=%+v calls=%d", comments, summary, api.calls)
	}
}

func TestReplyLimitsMustBothBeZeroOrPositive(t *testing.T) {
	for _, limits := range []Limits{
		{TopLevelCommentsPerWork: 1, RepliesPerComment: 0, RepliesPerWork: 1},
		{TopLevelCommentsPerWork: 1, RepliesPerComment: 1, RepliesPerWork: 0},
	} {
		t.Run(fmt.Sprintf("comment-%d-work-%d", limits.RepliesPerComment, limits.RepliesPerWork), func(t *testing.T) {
			options := approvedTestOptions()
			options.Limits = limits
			api := &fixturePageAPI{}
			collector := NewCollector(api, NewEvidenceRecorder(nil), newTestStore(t, "reply-mismatch-job"), &fixtureClock{})
			if _, _, err := collector.CollectComments(context.Background(), options, fixtureWork("fixture-mismatch-work", "fixture-mismatch-nonce", 1)); err == nil {
				t.Fatal("CollectComments() accepted mismatched reply limits")
			}
			if api.calls != 0 {
				t.Fatalf("calls=%d", api.calls)
			}
		})
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

func TestResumeCommentsFromLastCompletePage(t *testing.T) {
	store := newTestStore(t, "comments-resume-job")
	for sequence := 1; sequence <= 2; sequence++ {
		if _, err := store.WriteEvidence(Evidence{RequestSequence: sequence, Method: commentListMethod, RedactionRuleVersion: RedactionRuleVersion}); err != nil {
			t.Fatal(err)
		}
	}
	work := fixtureWork("fixture-resume-work", "fixture-resume-nonce", 1)
	savedID := "fixture-saved-id"
	workID := *work.WorkID
	saved := []Comment{
		{CommentID: &savedID, WorkID: &workID, Level: 1, Source: SourceRef{Method: commentListMethod, EvidenceRef: "evidence/000001.json", Ordinal: 1}},
		{WorkID: &workID, Level: 1, Content: CommentContent{Text: stringPointer("fixture-missing")}, Source: SourceRef{Method: commentListMethod, EvidenceRef: "evidence/000002.json", Ordinal: 1}},
	}
	if err := store.SaveCheckpoint(Checkpoint{
		SchemaVersion: SchemaVersion, JobID: "comments-resume-job", Phase: "comments_top", SearchMarker: "fixture-third-page",
		CurrentWorkRank: 1, Works: []Work{work}, Comments: saved,
	}); err != nil {
		t.Fatal(err)
	}
	thirdPage := commentPage(t, []map[string]any{
		{"commentId": savedID, "content": "fixture-duplicate"},
		{"content": "fixture-missing"},
		{"commentId": "fixture-new-id", "content": "fixture-new"},
	}, "")
	api := &scriptedPageAPI{script: []scriptedCall{{raw: thirdPage}}}
	collector := NewCollector(api, NewEvidenceRecorder(nil), store, &fixtureClock{})
	comments, summary, err := collector.CollectComments(context.Background(), approvedTestOptions(), work)
	if err != nil {
		t.Fatal(err)
	}
	if api.calls != 1 || len(api.bodies) != 1 {
		t.Fatalf("calls=%d", api.calls)
	}
	body, ok := api.bodies[0].(map[string]any)
	if !ok || body["next_marker"] != "fixture-third-page" {
		t.Fatalf("resume body=%+v", api.bodies[0])
	}
	if summary.TopLevel != 4 || countCommentID(comments, savedID) != 1 || countNilCommentIDs(comments) != 2 {
		t.Fatalf("comments=%+v summary=%+v", comments, summary)
	}
	if comments[len(comments)-1].Source.EvidenceRef != "evidence/000003.json" {
		t.Fatalf("evidence sequence did not resume: %+v", comments[len(comments)-1].Source)
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

func countNilCommentIDs(comments []Comment) int {
	count := 0
	for _, comment := range comments {
		if comment.CommentID == nil {
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
