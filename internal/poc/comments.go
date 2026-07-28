package poc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"time"
)

const commentListMethod = "finderGetCommentList"

type CommentSummary struct {
	TopLevel  int
	Replies   int
	Truncated bool
	Reasons   []string
	Partial   bool
}

type replyRoot struct {
	id string
}

func (c *Collector) CollectComments(ctx context.Context, options Options, work Work) ([]Comment, CommentSummary, error) {
	var summary CommentSummary
	if work.WorkID == nil || *work.WorkID == "" {
		return nil, summary, errors.New("work ID is missing")
	}
	if options.Limits.TopLevelCommentsPerWork <= 0 || options.Limits.RepliesPerWork <= 0 {
		return nil, summary, errors.New("comment limits must be positive")
	}

	comments := make([]Comment, 0)
	topSeen := make(map[string]struct{})
	replySeen := make(map[string]struct{})
	missingSeen := make(map[string]struct{})
	topMarkers := make(map[string]struct{})
	roots := make([]replyRoot, 0)
	queuedRoots := make(map[string]struct{})
	marker := ""
	stopTop := false

	for !stopTop {
		body := map[string]any{"object_id": *work.WorkID, "next_marker": marker}
		if work.ObjectNonceID != nil {
			body["nonce_id"] = *work.ObjectNonceID
		}
		raw, pageSource, err := c.call(ctx, commentListMethod, body)
		if err != nil {
			return comments, summary, err
		}
		items, nextMarker, markerPresent, _, err := parseCommentPage(raw)
		if err != nil {
			return comments, summary, err
		}
		ordinal := 0
		for _, item := range items {
			if summary.TopLevel >= options.Limits.TopLevelCommentsPerWork {
				markTruncated(&summary, "top_level_limit")
				stopTop = true
				break
			}
			ordinal++
			source := pageSource
			source.Ordinal = ordinal
			comment, _ := mapComment(item, 1, work.WorkID, nil, source)
			if !acceptCommentID(comment.CommentID, source, topSeen, missingSeen) {
				continue
			}
			comments = append(comments, comment)
			summary.TopLevel++

			embedded := childComments(item)
			for _, child := range embedded {
				if summary.Replies >= options.Limits.RepliesPerWork {
					markTruncated(&summary, "reply_limit")
					break
				}
				ordinal++
				childSource := pageSource
				childSource.Ordinal = ordinal
				reply, _ := mapComment(child, 2, work.WorkID, nil, childSource)
				if !acceptCommentID(reply.CommentID, childSource, replySeen, missingSeen) {
					continue
				}
				comments = append(comments, reply)
				summary.Replies++
			}

			reportedReplies := integerField(item, "expandCommentCount")
			if reportedReplies > len(embedded) && summary.Replies < options.Limits.RepliesPerWork {
				if comment.CommentID == nil {
					markPartial(&summary, "missing_root_comment_id")
				} else if _, exists := queuedRoots[*comment.CommentID]; !exists {
					queuedRoots[*comment.CommentID] = struct{}{}
					roots = append(roots, replyRoot{id: *comment.CommentID})
				}
			}
		}
		if err := c.saveCommentCheckpoint(work, comments, nextMarker); err != nil {
			return comments, summary, err
		}
		if stopTop || summary.TopLevel >= options.Limits.TopLevelCommentsPerWork {
			if summary.TopLevel >= options.Limits.TopLevelCommentsPerWork {
				markTruncated(&summary, "top_level_limit")
			}
			break
		}
		if !markerPresent {
			markPartial(&summary, "comment_pagination_incomplete")
			break
		}
		if nextMarker == "" {
			break
		}
		if _, repeated := topMarkers[nextMarker]; repeated {
			markPartial(&summary, "comment_pagination_repeated_marker")
			break
		}
		topMarkers[nextMarker] = struct{}{}
		marker = nextMarker
	}

	for _, root := range roots {
		if summary.Replies >= options.Limits.RepliesPerWork {
			markTruncated(&summary, "reply_limit")
			break
		}
		marker = ""
		seenMarkers := make(map[string]struct{})
		for {
			raw, pageSource, err := c.call(ctx, commentListMethod, map[string]any{
				"object_id":   *work.WorkID,
				"comment_id":  root.id,
				"next_marker": marker,
			})
			if err != nil {
				return comments, summary, err
			}
			items, nextMarker, markerPresent, _, err := parseCommentPage(raw)
			if err != nil {
				return comments, summary, err
			}
			for index, item := range items {
				if summary.Replies >= options.Limits.RepliesPerWork {
					markTruncated(&summary, "reply_limit")
					break
				}
				source := pageSource
				source.Ordinal = index + 1
				reply, _ := mapComment(item, 2, work.WorkID, &root.id, source)
				if !acceptCommentID(reply.CommentID, source, replySeen, missingSeen) {
					continue
				}
				comments = append(comments, reply)
				summary.Replies++
			}
			if err := c.saveCommentCheckpoint(work, comments, nextMarker); err != nil {
				return comments, summary, err
			}
			if summary.Replies >= options.Limits.RepliesPerWork {
				markTruncated(&summary, "reply_limit")
				break
			}
			if !markerPresent {
				markPartial(&summary, "reply_pagination_incomplete")
				break
			}
			if nextMarker == "" {
				break
			}
			if _, repeated := seenMarkers[nextMarker]; repeated {
				markPartial(&summary, "reply_pagination_repeated_marker")
				break
			}
			seenMarkers[nextMarker] = struct{}{}
			marker = nextMarker
		}
	}

	return comments, summary, nil
}

func parseCommentPage(raw []byte) (items []map[string]any, lastBuffer string, lastBufferPresent bool, reported int, err error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root map[string]any
	if decodeErr := decoder.Decode(&root); decodeErr != nil {
		return nil, "", false, 0, errors.New("decode comment response")
	}
	var trailing any
	if decodeErr := decoder.Decode(&trailing); !errors.Is(decodeErr, io.EOF) {
		return nil, "", false, 0, errors.New("comment response contains trailing JSON")
	}
	data, ok := root["data"].(map[string]any)
	if !ok {
		return nil, "", false, 0, errors.New("comment response data is missing")
	}
	rawItems, ok := data["commentInfo"].([]any)
	if !ok {
		return nil, "", false, 0, errors.New("comment info is missing")
	}
	items = make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		if item, itemOK := rawItem.(map[string]any); itemOK {
			items = append(items, item)
		} else {
			items = append(items, nil)
		}
	}
	markerValue, present := data["lastBuffer"]
	if !present {
		return items, "", false, reportedCount(data), nil
	}
	marker, ok := markerValue.(string)
	if !ok {
		return nil, "", false, 0, errors.New("comment marker is not a string")
	}
	return items, marker, true, reportedCount(data), nil
}

func reportedCount(data map[string]any) int {
	if countInfo, ok := data["countInfo"].(map[string]any); ok {
		return integerField(countInfo, "commentCount")
	}
	return 0
}

func mapComment(item map[string]any, level int, workID *string, retrievalRoot *string, source SourceRef) (Comment, []FieldResult) {
	comment := Comment{
		CommentID:              normalizedRelation(item, "commentId"),
		WorkID:                 cloneStringPointer(workID),
		Level:                  level,
		ParentCommentID:        normalizedRelation(item, "replyCommentId"),
		RootCommentID:          normalizedRelation(item, "rootCommentId"),
		RetrievalRootCommentID: cloneStringPointer(retrievalRoot),
		Content: CommentContent{
			MediaType: mapCommentMedia(item),
		},
		Source: source,
	}

	text, textPresent := stringField(item, "content")
	textStatus := FieldMissingInSource
	if textPresent {
		comment.Content.Text, textStatus = RedactString(text)
	}
	comment.Account.AccountID = optionalString(item, "username")
	comment.Account.DisplayName = optionalString(item, "nickname")
	avatarStatus := FieldMissingInSource
	if avatar, present := stringField(item, "headUrl"); present {
		comment.Account.AvatarURL, avatarStatus = SafeURL(avatar)
	}

	timeStatus := FieldMissingInSource
	if rawTime, present := stringField(item, "createtime"); present {
		comment.CreatedAt.Raw = stringPointer(rawTime)
		seconds, parseErr := strconv.ParseInt(rawTime, 10, 64)
		if parseErr == nil && seconds > 0 {
			comment.CreatedAt.UnixSeconds = &seconds
			iso := time.Unix(seconds, 0).UTC().Format(time.RFC3339)
			comment.CreatedAt.ISO8601 = &iso
			timeStatus = FieldPresent
		} else {
			timeStatus = FieldInvalidFormat
		}
	}

	if regionInfo, ok := item["ipRegionInfo"].(map[string]any); ok {
		comment.IPLocation.Label = optionalString(regionInfo, "regionText")
	}
	if comment.IPLocation.Label == nil {
		comment.IPLocation.Label = optionalString(item, "ipRegion")
	}

	results := []FieldResult{
		fieldResult("comments[].content.text", textStatus),
		fieldResult("comments[].account.avatar_url", avatarStatus),
		fieldResult("comments[].created_at", timeStatus),
		fieldResult("comments[].ip_location.label", pointerStatus(comment.IPLocation.Label)),
	}
	return comment, results
}

func mapCommentMedia(item map[string]any) MediaType {
	media := MediaType{RawCode: item["contentType"], Normalized: "unknown"}
	switch fmt.Sprint(media.RawCode) {
	case "1":
		media.Normalized = "text"
	case "2":
		media.Normalized = "image"
	}
	return media
}

func childComments(item map[string]any) []map[string]any {
	rawChildren, ok := item["levelTwoComment"].([]any)
	if !ok {
		return nil
	}
	children := make([]map[string]any, 0, len(rawChildren))
	for _, rawChild := range rawChildren {
		if child, ok := rawChild.(map[string]any); ok {
			children = append(children, child)
		}
	}
	return children
}

func acceptCommentID(id *string, source SourceRef, seenIDs, missingSeen map[string]struct{}) bool {
	if id != nil {
		if _, exists := seenIDs[*id]; exists {
			return false
		}
		seenIDs[*id] = struct{}{}
		return true
	}
	key := fmt.Sprintf("%s:%d", source.EvidenceRef, source.Ordinal)
	if _, exists := missingSeen[key]; exists {
		return false
	}
	missingSeen[key] = struct{}{}
	return true
}

func normalizedRelation(item map[string]any, key string) *string {
	value := optionalString(item, key)
	if value == nil || *value == "0" {
		return nil
	}
	return value
}

func integerField(item map[string]any, key string) int {
	value, ok := stringField(item, key)
	if !ok {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	return stringPointer(*value)
}

func fieldResult(path string, status FieldStatus) FieldResult {
	result := FieldResult{Path: path, Status: status, Applicable: 1}
	if status == FieldPresent {
		result.Present = 1
	}
	return result
}

func pointerStatus(value *string) FieldStatus {
	if value == nil {
		return FieldMissingInSource
	}
	return FieldPresent
}

func markTruncated(summary *CommentSummary, reason string) {
	summary.Truncated = true
	appendReason(summary, reason)
}

func markPartial(summary *CommentSummary, reason string) {
	summary.Partial = true
	appendReason(summary, reason)
}

func appendReason(summary *CommentSummary, reason string) {
	for _, existing := range summary.Reasons {
		if existing == reason {
			return
		}
	}
	summary.Reasons = append(summary.Reasons, reason)
}

func (c *Collector) saveCommentCheckpoint(work Work, comments []Comment, marker string) error {
	checkpoint := Checkpoint{
		SchemaVersion:   SchemaVersion,
		JobID:           filepath.Base(c.store.JobDir()),
		Phase:           "comments",
		SearchMarker:    marker,
		CurrentWorkRank: work.Locator.SearchRank,
		Works:           []Work{work},
		Comments:        append([]Comment(nil), comments...),
		SavedAt:         c.clock.Now().UTC(),
	}
	return c.store.SaveCheckpoint(checkpoint)
}
