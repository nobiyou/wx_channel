package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"wx_channel/internal/websocket"
)

// callLive reuses the application's ready-client selection and never returns
// the raw page response. Projection is deliberately fail-closed on schema drift.
func (s *Server) callLive(ctx context.Context, key string, body any) (*mcp.CallToolResult, map[string]any, error) {
	if s.deps.Hub == nil {
		return nil, nil, failure("runtime_unavailable")
	}
	if _, err := s.deps.Hub.GetClientForKey(key); err != nil {
		return nil, nil, failure("no_ready_client")
	}
	callCtx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	raw, err := s.deps.Hub.CallAPIContext(callCtx, key, body, 60*time.Second)
	if err != nil {
		return nil, nil, classifyLiveError(err)
	}
	if err := callCtx.Err(); err != nil {
		return nil, nil, classifyLiveError(err)
	}
	if len(raw) > maxLiveResponseBytes {
		return nil, nil, failure("response_too_large")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil, failure("empty_live_response")
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber() // object and comment IDs routinely exceed float64 precision.
	if err := decoder.Decode(&root); err != nil {
		return nil, nil, failure("invalid_live_response")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, nil, failure("invalid_live_response")
	}
	if root == nil {
		return nil, nil, failure("empty_live_response")
	}
	if code := text(root, "errCode"); code != "" && code != "0" {
		if code == "-70003" {
			return nil, nil, failure("page_context_mismatch")
		}
		return nil, nil, failure("live_query_failed")
	}
	data, present := root["data"]
	if present && data == nil {
		return nil, nil, failure("empty_live_response")
	}
	page := object(data)
	if page == nil {
		return nil, nil, failure("unsupported_live_schema")
	}
	for _, container := range []map[string]any{root, page} {
		if base := object(container["BaseResponse"]); base != nil {
			if code := text(base, "Ret", "ret"); code != "" && code != "0" {
				return nil, nil, failure("live_query_failed")
			}
		}
	}
	warnings := []string{}
	var output map[string]any
	switch key {
	case "key:channels:contact_list":
		search, ok := body.(websocket.SearchContactBody)
		if !ok {
			return nil, nil, failure("invalid_argument")
		}
		if search.Type == 1 {
			output, err = projectPage(page, "infoList", projectAccount, &warnings)
		} else {
			output, err = projectPage(page, "objectList", projectVideo, &warnings)
		}
	case "key:channels:feed_list":
		output, err = projectPage(page, "object", projectVideo, &warnings)
		// finderUserPage versions use either object or objectList.
		if _, ok := page["object"]; !ok {
			output, err = projectPage(page, "objectList", projectVideo, &warnings)
		}
	case "key:channels:fetch_feed_comment_list":
		output, err = projectPage(page, "commentInfo", projectComment, &warnings)
	default:
		if _, present := page["object"]; !present {
			return nil, nil, failure("unsupported_live_schema")
		}
		if page["object"] == nil {
			return nil, nil, failure("empty_live_response")
		}
		video := object(page["object"])
		if video == nil {
			return nil, nil, failure("unsupported_live_schema")
		}
		if strings.HasPrefix(key, "key:channels:shared_feed_") {
			output = projectSharedVideo(page, &warnings)
		} else {
			output = projectVideo(video, &warnings)
		}
		if output == nil {
			return nil, nil, failure("unsupported_live_schema")
		}
	}
	if err != nil {
		return nil, nil, err
	}
	source := "live_wechat_page"
	if strings.HasPrefix(key, "key:channels:shared_feed_") {
		source = "shared_page"
	}
	result := envelope(source, "live", output, warnings)
	result["partial"] = len(warnings) > 0
	size, err := serializedToolResultSize(result)
	if err != nil || size > maxToolResponseBytes {
		return nil, nil, failure("response_too_large")
	}
	return nil, result, nil
}

type projector func(map[string]any, *[]string) map[string]any

func projectPage(page map[string]any, field string, project projector, warnings *[]string) (map[string]any, error) {
	raw, present := page[field]
	if !present {
		return nil, failure("unsupported_live_schema")
	}
	list, ok := raw.([]any)
	if !ok && raw != nil {
		return nil, failure("unsupported_live_schema")
	}
	// Reject the entire page rather than return its upstream next marker while
	// silently discarding records beyond the bound.
	if len(list) > 50 {
		return nil, failure("response_too_large")
	}
	items := make([]map[string]any, 0, len(list))
	for _, entry := range list {
		item := project(object(entry), warnings)
		if item == nil {
			return nil, failure("unsupported_live_schema")
		}
		items = append(items, item)
	}
	marker := text(page, "lastBuffer", "lastBuff")
	if len(marker) > maxMarkerBytes {
		return nil, failure("response_too_large")
	}
	more := marker != ""
	if flag, exists := page["continueFlag"]; exists {
		if value, ok := flag.(bool); ok {
			more = value
		} else if n := number(page, "continueFlag"); n != nil {
			more = n.(int64) != 0
		} else {
			return nil, failure("unsupported_live_schema")
		}
	}
	if more && marker == "" {
		return nil, failure("pagination_unavailable")
	}
	if !more {
		marker = ""
	}
	// The existing JS bridge applies decodeURIComponent exactly once. Escape the
	// upstream buffer once here; callers must pass next_marker back unchanged.
	return map[string]any{"items": items, "count": len(items), "has_more": more, "next_marker": url.PathEscape(marker)}, nil
}

func projectAccount(entry map[string]any, warnings *[]string) map[string]any {
	contact := object(entry["contact"])
	if contact == nil {
		contact = entry
	}
	username := text(contact, "username")
	if username == "" {
		return nil
	}
	return map[string]any{"username": username, "nickname": boundedText(text(contact, "nickname"), warnings), "avatar_url": safePublicURL(text(contact, "headUrl")), "description": boundedText(text(contact, "signature"), warnings)}
}

func projectVideo(video map[string]any, warnings *[]string) map[string]any {
	id := text(video, "id", "objectId")
	if id == "" {
		return nil
	}
	return projectVideoFields(video, warnings)
}

func projectVideoFields(video map[string]any, warnings *[]string) map[string]any {
	contact, desc := object(video["contact"]), object(video["objectDesc"])
	author := text(contact, "nickname")
	if author == "" {
		author = text(video, "nickname")
	}
	authorID := text(contact, "username")
	if authorID == "" {
		authorID = text(video, "username")
	}
	media := map[string]any{}
	if list, ok := desc["media"].([]any); ok && len(list) > 0 {
		media = object(list[0])
	}
	return map[string]any{
		"object_id": text(video, "id", "objectId"), "nonce_id": text(video, "objectNonceId", "nonceId"),
		"title": boundedText(text(desc, "description"), warnings), "author": boundedText(author, warnings), "author_id": authorID,
		"published_at": unixTimestamp(video, "createtime", "createTime"), "cover_url": safePublicURL(text(media, "thumbUrl", "coverUrl")),
		"duration_seconds": mediaDuration(media), "resolution": text(media, "videoResolution"), "width": number(media, "width"), "height": number(media, "height"), "size_bytes": number(media, "fileSize", "cdnFileSize"),
		"like_count": number(video, "likeCount"), "comment_count": number(video, "commentCount"), "favorite_count": number(video, "favCount"), "forward_count": number(video, "forwardCount"),
	}
}

// The share resolver's compatibility object may use an export reference (or
// "shared_feed") as id. Neither identifies a video for normal profile/comments.
func projectSharedVideo(page map[string]any, warnings *[]string) map[string]any {
	video := object(page["object"])
	desc := object(video["objectDesc"])
	if len(desc) == 0 {
		return nil
	}
	result := projectVideoFields(video, warnings)
	id := text(video, "id", "objectId")
	_, compat := page["feedInfo"]
	if id == "" || id == "shared_feed" || compat {
		result["object_id"], result["nonce_id"] = nil, nil
		result["identity_kind"] = "share_reference"
		ref := text(object(page["sceneInfo"]), "dynamicExportId")
		result["share_reference"] = nil
		if ref != "" && ref != "shared_feed" {
			result["share_reference"] = ref
		}
		addWarning(warnings, "video_identity_unavailable_use_share_url")
	}
	if compat {
		feed := object(page["feedInfo"])
		result["duration_seconds"] = mediaDuration(feed)
		result["size_bytes"] = number(feed, "fileSize", "cdnFileSize")
	}
	return result
}

func mediaDuration(media map[string]any) any {
	for _, key := range []string{"durationMs", "videoDuration"} {
		if ms := number(media, key); ms != nil && ms.(int64) > 0 {
			return float64(ms.(int64)) / 1000
		}
	}
	if seconds := number(media, "videoPlayLen"); seconds != nil && seconds.(int64) > 0 {
		return seconds
	}
	return nil
}

func projectComment(comment map[string]any, warnings *[]string) map[string]any {
	id := text(comment, "commentId", "id")
	if id == "" {
		return nil
	}
	if replies, ok := comment["levelTwoComment"].([]any); ok && len(replies) > 0 {
		addWarning(warnings, "inline_replies_not_included_use_comment_id")
	}
	return map[string]any{"comment_id": id, "content": boundedText(text(comment, "content"), warnings), "author": boundedText(text(comment, "nickname"), warnings), "author_id": text(comment, "username"), "published_at": unixTimestamp(comment, "createtime", "createTime"), "like_count": number(comment, "likeCount"), "reply_count": number(comment, "expandCommentCount"), "reply_to_comment_id": text(comment, "replyCommentId")}
}

func object(value any) map[string]any { result, _ := value.(map[string]any); return result }

func text(value map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := value[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case json.Number:
			return string(v)
		}
	}
	return ""
}

// Counts are nullable. An explicit upstream zero is different from absence,
// null, a negative sentinel, or an unparseable value.
func number(value map[string]any, keys ...string) any {
	for _, key := range keys {
		s := text(value, key)
		if s == "" {
			continue
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err == nil && n >= 0 {
			return n
		}
	}
	return nil
}

func unixTimestamp(value map[string]any, keys ...string) any {
	if n := number(value, keys...); n != nil && n.(int64) > 0 {
		return time.Unix(n.(int64), 0).UTC().Format(time.RFC3339)
	}
	return nil
}

func boundedText(value string, warnings *[]string) string {
	if runes := []rune(value); len(runes) > maxFieldRunes {
		addWarning(warnings, "text_fields_truncated")
		return string(runes[:maxFieldRunes])
	}
	return value
}

func addWarning(warnings *[]string, value string) {
	for _, existing := range *warnings {
		if existing == value {
			return
		}
	}
	*warnings = append(*warnings, value)
}

func safePublicURL(raw string) any {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil
	}
	if _, changed := sanitizeURL(raw); changed {
		return nil
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func classifyLiveError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return failure("canceled")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, websocket.ErrRequestTimeout):
		return failure("timeout")
	case errors.Is(err, websocket.ErrNoAvailableClient), errors.Is(err, websocket.ErrNoReadyClient):
		return failure("no_ready_client")
	}
	// Legacy Hub errors carry a numeric page error in text. Never echo it to MCP.
	if strings.Contains(err.Error(), "code=-70003") || strings.Contains(err.Error(), "JSAPI_JSONPARSE_FAILED") {
		return failure("page_context_mismatch")
	}
	return failure("live_query_failed")
}
