package api

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wx_channel/internal/websocket"
)

func TestExportFeedCommentsFetchesAllPagesRepliesAndSavesFile(t *testing.T) {
	tempDir := t.TempDir()
	calls := make([]websocket.FeedCommentListBody, 0, 4)

	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			if key != "key:channels:fetch_feed_comment_list" {
				t.Fatalf("unexpected key: %s", key)
			}

			req, ok := body.(websocket.FeedCommentListBody)
			if !ok {
				t.Fatalf("unexpected body type: %T", body)
			}
			calls = append(calls, req)

			switch {
			case req.CommentID == "" && req.NextMarker == "":
				return []byte(`{"errCode":0,"errMsg":"ok","data":{"commentInfo":[{"commentId":"c1","content":"top-1","expandCommentCount":2,"levelTwoComment":[{"commentId":"r1","content":"reply-1"}],"nickname":"用户A","username":"user_a","headUrl":"https://img/a","createtime":"1715760000","likeCount":12,"ipRegion":"广东","extraField":"drop-me"}],"countInfo":{"commentCount":2},"lastBuffer":"page-2"}}`), nil
			case req.CommentID == "" && req.NextMarker == "page-2":
				return []byte(`{"errCode":0,"errMsg":"ok","data":{"commentInfo":[{"commentId":"c2","content":"top-2","expandCommentCount":0,"levelTwoComment":[],"nickname":"用户B","username":"user_b","createtime":"1715760300","likeCount":1}],"countInfo":{"commentCount":2},"lastBuffer":""}}`), nil
			case req.CommentID == "c1" && req.NextMarker == "":
				return []byte(`{"errCode":0,"errMsg":"ok","data":{"commentInfo":[{"commentId":"r1","content":"reply-1","replyCommentId":"c1","nickname":"回复1","username":"reply_1","createtime":"1715760600","likeCount":3},{"commentId":"r2","content":"reply-2","replyCommentId":"c1","nickname":"回复2","username":"reply_2","createtime":"1715760900","likeCount":4}],"lastBuffer":"reply-2"}}`), nil
			case req.CommentID == "c1" && req.NextMarker == "reply-2":
				return []byte(`{"errCode":0,"errMsg":"ok","data":{"commentInfo":[{"commentId":"r2","content":"reply-2","replyCommentId":"c1","nickname":"回复2","username":"reply_2","createtime":"1715760900","likeCount":4}],"lastBuffer":""}}`), nil
			default:
				t.Fatalf("unexpected request: %+v", req)
				return nil, nil
			}
		},
		resolveDownloadsDir: func() (string, error) {
			return tempDir, nil
		},
	}

	result, err := service.exportFeedComments(ExportFeedCommentsRequest{
		ObjectID: "oid-1",
		NonceID:  "nid-1",
		Title:    "测试标题",
		Author:   "测试作者",
	})
	if err != nil {
		t.Fatalf("exportFeedComments() error = %v", err)
	}

	if result.TopLevelCount != 2 {
		t.Fatalf("TopLevelCount = %d, want 2", result.TopLevelCount)
	}
	if result.ReplyCount != 2 {
		t.Fatalf("ReplyCount = %d, want 2", result.ReplyCount)
	}
	if result.TotalCount != 4 {
		t.Fatalf("TotalCount = %d, want 4", result.TotalCount)
	}
	if len(calls) != 4 {
		t.Fatalf("call count = %d, want 4", len(calls))
	}

	if _, err := os.Stat(result.SavedPath); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}

	raw, err := os.ReadFile(result.SavedPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var saved struct {
		ObjectID      string `json:"objectId"`
		ObjectNonceID string `json:"objectNonceId"`
		Title         string `json:"title"`
		Author        string `json:"author"`
		CountInfo     struct {
			CommentCount int `json:"commentCount"`
		} `json:"countInfo"`
		CommentInfo []struct {
			CommentID          string `json:"commentId"`
			ReplyCommentID     string `json:"replyCommentId"`
			Content            string `json:"content"`
			Nickname           string `json:"nickname"`
			Username           string `json:"username"`
			HeadURL            string `json:"headUrl"`
			Createtime         string `json:"createtime"`
			LikeCount          int    `json:"likeCount"`
			ExpandCommentCount int    `json:"expandCommentCount"`
			ContinueFlag       int    `json:"continueFlag"`
			IPRegionInfo       struct {
				RegionText string `json:"regionText"`
			} `json:"ipRegionInfo"`
			LevelTwoComment []struct {
				CommentID      string `json:"commentId"`
				ReplyCommentID string `json:"replyCommentId"`
				Content        string `json:"content"`
				Nickname       string `json:"nickname"`
				Username       string `json:"username"`
				Createtime     string `json:"createtime"`
				LikeCount      int    `json:"likeCount"`
			} `json:"levelTwoComment"`
		} `json:"commentInfo"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("saved json unmarshal error = %v", err)
	}

	if saved.ObjectID != "oid-1" {
		t.Fatalf("saved objectId = %s, want oid-1", saved.ObjectID)
	}
	if saved.ObjectNonceID != "nid-1" {
		t.Fatalf("saved objectNonceId = %s, want nid-1", saved.ObjectNonceID)
	}
	if saved.CountInfo.CommentCount != 2 {
		t.Fatalf("countInfo.commentCount = %d, want 2", saved.CountInfo.CommentCount)
	}
	if len(saved.CommentInfo) != 2 {
		t.Fatalf("saved commentInfo len = %d, want 2", len(saved.CommentInfo))
	}

	if saved.CommentInfo[0].CommentID != "c1" {
		t.Fatalf("first commentId = %s, want c1", saved.CommentInfo[0].CommentID)
	}
	if saved.CommentInfo[0].HeadURL != "https://img/a" {
		t.Fatalf("first comment headUrl = %s", saved.CommentInfo[0].HeadURL)
	}
	if saved.CommentInfo[0].ExpandCommentCount != 2 {
		t.Fatalf("first comment expandCommentCount = %d, want 2", saved.CommentInfo[0].ExpandCommentCount)
	}
	if saved.CommentInfo[0].IPRegionInfo.RegionText != "广东" {
		t.Fatalf("first comment ipRegionInfo.regionText = %s", saved.CommentInfo[0].IPRegionInfo.RegionText)
	}
	if len(saved.CommentInfo[0].LevelTwoComment) != 2 {
		t.Fatalf("saved levelTwoComment len = %d, want 2", len(saved.CommentInfo[0].LevelTwoComment))
	}
	if saved.CommentInfo[0].LevelTwoComment[0].ReplyCommentID != "c1" {
		t.Fatalf("replyCommentId = %s, want c1", saved.CommentInfo[0].LevelTwoComment[0].ReplyCommentID)
	}
	if saved.CommentInfo[0].LevelTwoComment[0].Content != "reply-1" {
		t.Fatalf("reply content = %s, want reply-1", saved.CommentInfo[0].LevelTwoComment[0].Content)
	}

	if filepath.Base(result.SavedPath) == "" {
		t.Fatalf("SavedPath should include filename")
	}
	if filepath.Base(filepath.Dir(result.SavedPath)) != time.Now().Format("2006-01-02") {
		t.Fatalf("saved directory = %s, want today's folder", filepath.Base(filepath.Dir(result.SavedPath)))
	}
}

func TestExportFeedCommentsSavesCheckpointBeforeReplyFailure(t *testing.T) {
	tempDir := t.TempDir()

	service := &SearchService{
		callAPI: func(key string, body interface{}, timeout time.Duration) ([]byte, error) {
			req, ok := body.(websocket.FeedCommentListBody)
			if !ok {
				t.Fatalf("unexpected body type: %T", body)
			}

			switch {
			case req.CommentID == "" && req.NextMarker == "":
				return []byte(`{"errCode":0,"errMsg":"ok","data":{"commentInfo":[{"commentId":"c1","content":"top-1","expandCommentCount":2,"levelTwoComment":[],"nickname":"用户A","username":"user_a"}],"countInfo":{"commentCount":1},"lastBuffer":""}}`), nil
			case req.CommentID == "c1":
				return nil, errors.New("page reloaded")
			default:
				t.Fatalf("unexpected request: %+v", req)
				return nil, nil
			}
		},
		resolveDownloadsDir: func() (string, error) {
			return tempDir, nil
		},
	}

	_, err := service.exportFeedComments(ExportFeedCommentsRequest{
		ObjectID: "oid-1",
		NonceID:  "nid-1",
		Title:    "超长评论测试",
		Author:   "测试作者",
	})
	if err == nil {
		t.Fatalf("exportFeedComments() error = nil, want failure")
	}

	matches, err := filepath.Glob(filepath.Join(tempDir, "comment_data", time.Now().Format("2006-01-02"), "*.partial.json"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("partial checkpoint count = %d, want 1", len(matches))
	}

	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var saved struct {
		ObjectID      string `json:"objectId"`
		ObjectNonceID string `json:"objectNonceId"`
		Title         string `json:"title"`
		CommentInfo   []struct {
			CommentID       string `json:"commentId"`
			LevelTwoComment []struct{} `json:"levelTwoComment"`
		} `json:"commentInfo"`
		OriginalCommentCount int    `json:"originalCommentCount"`
		Source               string `json:"source"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}

	if saved.ObjectID != "oid-1" {
		t.Fatalf("saved objectId = %s, want oid-1", saved.ObjectID)
	}
	if saved.ObjectNonceID != "nid-1" {
		t.Fatalf("saved objectNonceId = %s, want nid-1", saved.ObjectNonceID)
	}
	if len(saved.CommentInfo) != 1 {
		t.Fatalf("saved commentInfo len = %d, want 1", len(saved.CommentInfo))
	}
	if saved.CommentInfo[0].CommentID != "c1" {
		t.Fatalf("saved commentId = %s, want c1", saved.CommentInfo[0].CommentID)
	}
	if len(saved.CommentInfo[0].LevelTwoComment) != 0 {
		t.Fatalf("saved levelTwoComment len = %d, want 0", len(saved.CommentInfo[0].LevelTwoComment))
	}
	if saved.OriginalCommentCount != 1 {
		t.Fatalf("saved originalCommentCount = %d, want 1", saved.OriginalCommentCount)
	}
	if saved.Source != "finderGetCommentList.partial" {
		t.Fatalf("saved source = %s, want finderGetCommentList.partial", saved.Source)
	}
}
