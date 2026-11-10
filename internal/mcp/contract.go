package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type toolFailure struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Recovery string `json:"recovery"`
}

func (e *toolFailure) Error() string { return e.Code + ": " + e.Message + "; " + e.Recovery }

// Only fixed messages reach the client; underlying errors can contain URLs,
// database paths, or request credentials.
func failure(code string) *toolFailure {
	message, recovery := "The read-only query failed.", "Check runtime status and retry."
	switch code {
	case "no_ready_client":
		message = "No compatible WeChat page is ready."
		recovery = "Open or refresh a supported WeChat Channels page, wait for its API, then retry."
	case "runtime_unavailable":
		message = "The WebSocket Hub is unavailable."
		recovery = "Start wx_channel and check runtime status."
	case "timeout":
		message = "The query timed out."
		recovery = "Check page readiness and retry one page."
	case "canceled":
		message = "The query was canceled."
		recovery = "Retry if the result is still needed."
	case "page_context_mismatch":
		message = "The WeChat page context does not match this query."
		recovery = "Open the target video or share page in WeChat, refresh it, then retry."
	case "empty_live_response":
		message = "The page returned no response data."
		recovery = "Refresh the supported WeChat page and retry; this is not a confirmed empty result list."
	case "invalid_live_response", "unsupported_live_schema":
		message = "The WeChat response has an unsupported structure."
		recovery = "Refresh the page and retry; if persistent, update wx_channel for the changed WeChat response."
	case "pagination_unavailable":
		message = "WeChat reports more results without a continuation marker."
		recovery = "Refresh and repeat the query; do not treat this page as a complete list."
	case "response_too_large":
		message = "The response exceeds the bounded page or byte limit."
		recovery = "Narrow the query or reduce local page_size; no continuation was advanced."
	case "busy":
		message = "The MCP query concurrency limit is reached."
		recovery = "Wait for the running queries to finish, then retry."
	case "invalid_argument":
		message = "One or more query arguments are invalid."
		recovery = "Check the tool input schema, value limits, dates, source, URL and cursor."
	case "path_forbidden":
		message = "The media path is outside the allowed downloads directory."
		recovery = "Use a relative media path or a stored record in the configured downloads directory."
	case "local_file_not_found", "local_file_unavailable":
		message = "The referenced local media file is unavailable."
		recovery = "Check the stored record and file in the configured downloads directory."
	case "media_decoder_unavailable":
		message = "The local frame decoder is unavailable."
		recovery = "Provide FFmpeg in tools/ffmpeg/bin beside wx_channel, on PATH, or through WX_CHANNEL_FFMPEG_PATH, then retry."
	case "media_format_unsupported":
		message = "This media format is not supported for frame sampling."
		recovery = "Use a downloaded MP4 or MOV video."
	case "media_duration_unavailable":
		message = "The local video duration could not be read."
		recovery = "Choose one to three explicit timestamps_seconds or check the video file."
	case "media_file_too_large":
		message = "The video exceeds the local frame extraction size limit."
		recovery = "Use a nonempty local video no larger than 256 MiB."
	case "media_decode_failed":
		message = "No usable image was decoded from the requested video position."
		recovery = "Check that the video plays and the requested offsets are inside its duration."
	case "media_decode_timeout":
		message = "Local video frame extraction timed out."
		recovery = "Request fewer frames or a smaller max_width, check local FFmpeg, then retry."
	case "local_record_not_found":
		message = "The local record was not found."
		recovery = "List local records to obtain a valid id and source."
	case "local_database_unavailable", "local_database_query_failed":
		message = "The local database query is unavailable."
		recovery = "Check wx_channel database initialization and retry."
	case "live_query_failed":
		message = "The WeChat page query failed."
		recovery = "Check runtime status, refresh the supported page, and retry."
	case "download_unavailable":
		message = "The local video downloader is unavailable."
		recovery = "Start wx_channel with its Gopeed downloader and retry."
	case "download_record_not_found":
		message = "The requested browse record was not found."
		recovery = "Call list_local_videos and pass a browse record id."
	case "download_source_unavailable":
		message = "The browse record has no supported video source."
		recovery = "Open the video in WeChat and capture it again before retrying."
	case "download_failed":
		message = "The video download failed."
		recovery = "Check the WeChat page session and source availability, then retry the download action."
	case "download_record_save_failed":
		message = "The local download record could not be saved."
		recovery = "Check the wx_channel database and downloads directory, then retry."
	case "download_timeout":
		message = "The video download timed out."
		recovery = "Retry the independent download action; a completed local file is required before analysis."
	case "analysis_not_found":
		message = "The requested video analysis was not found."
		recovery = "Start a new analysis with a completed local download, then use its returned analysis_id."
	case "analysis_not_ready":
		message = "The video analysis is still running or failed."
		recovery = "Poll get_video_analysis_status and retry after status becomes completed."
	case "analysis_artifact_unavailable":
		message = "The video analysis artifact could not be saved or read."
		recovery = "Check the configured downloads directory is writable, then retry the analysis."
	case "analysis_failed":
		message = "The local video analysis failed."
		recovery = "Check the completed video and FFmpeg availability, then retry the analysis."
	case "analysis_limit_exceeded":
		message = "The video exceeds the bounded local analysis limits."
		recovery = "Use a video up to 1 GiB and 30 minutes with at most 80 candidate shots."
	case "analysis_unavailable":
		message = "The local analysis runtime is unavailable."
		recovery = "Restart wx_channel and retry the analysis."
	case "analysis_interrupted":
		message = "The video analysis was interrupted."
		recovery = "Start a new analysis from the completed local video."
	case "analysis_timeout":
		message = "The video analysis timed out."
		recovery = "Use a shorter video or retry after checking local FFmpeg."
	case "download_required":
		message = "Video analysis requires a completed local download."
		recovery = "Finish the independent download action before starting analysis."
	case "model_not_configured":
		message = "The external video analysis model is not configured."
		recovery = "Configure its base URL, API key and model, or omit use_external_model for Codex review."
	default:
		code = "query_failed"
	}
	return &toolFailure{Code: code, Message: message, Recovery: recovery}
}

func registerDownloadTool(s *Server) {
	destructive := false
	openWorld := false
	tool := &mcp.Tool{
		Name:        "download_video_for_analysis",
		Description: "Start or inspect an independent download for a stored browse record. Pass only a browse record_id from list_local_videos; call this tool again until status is completed, then use get_local_media_metadata and get_local_video_frames. It never accepts a raw URL and analysis tools never download implicitly.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &destructive, OpenWorldHint: &openWorld},
	}
	mcp.AddTool(s.mcpServer, tool, func(ctx context.Context, req *mcp.CallToolRequest, input downloadVideoForAnalysisInput) (*mcp.CallToolResult, map[string]any, error) {
		if err := ctx.Err(); err != nil {
			return downloadToolFailure(err)
		}
		result, out, err := s.downloadVideoForAnalysis(ctx, req, input)
		if err != nil {
			return downloadToolFailure(err)
		}
		if size, sizeErr := serializedCallToolResultSize(result, out); sizeErr != nil || size > maxToolResponseBytes {
			return downloadToolFailure(failure("response_too_large"))
		}
		return result, out, nil
	})
}

func downloadToolFailure(err error) (*mcp.CallToolResult, map[string]any, error) {
	var typed *toolFailure
	if errors.Is(err, context.Canceled) {
		err = failure("canceled")
	} else if errors.Is(err, context.DeadlineExceeded) {
		err = failure("download_timeout")
	}
	if !errors.As(err, &typed) {
		code := strings.SplitN(err.Error(), ":", 2)[0]
		typed = failure(code)
	}
	out := envelope("local_download", "live", map[string]any{}, nil)
	out["partial"] = true
	out["error"] = typed
	return &mcp.CallToolResult{IsError: true}, out, nil
}

func registerReadTool[I any](s *Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[I, map[string]any]) {
	tool.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	mcp.AddTool(s.mcpServer, tool, func(ctx context.Context, req *mcp.CallToolRequest, input I) (*mcp.CallToolResult, map[string]any, error) {
		fail := func(err error) (*mcp.CallToolResult, map[string]any, error) {
			var typed *toolFailure
			if tool.Name == "get_local_video_frames" && errors.Is(err, context.DeadlineExceeded) {
				err = failure("media_decode_timeout")
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				err = classifyLiveError(err)
			}
			if !errors.As(err, &typed) {
				typed = failure(strings.SplitN(err.Error(), ":", 2)[0])
			}
			source, freshness := "local_database", "stored"
			switch tool.Name {
			case "get_runtime_status":
				source, freshness = "runtime_diagnostics", "live"
			case "get_local_media_metadata", "get_local_video_frames":
				source = "local_file"
			case "get_video_analysis_status", "get_video_analysis_shots", "get_video_analysis_report":
				source, freshness = "local_video_analysis", "stored"
			case "resolve_channel_share_link":
				source, freshness = "shared_page", "unknown"
			case "list_local_videos", "get_local_video":
			default:
				source, freshness = "live_wechat_page", "unknown"
			}
			out := envelope(source, freshness, map[string]any{}, nil)
			out["partial"] = true
			out["error"] = typed
			return &mcp.CallToolResult{IsError: true}, out, nil
		}
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		select {
		case s.querySlots <- struct{}{}:
			defer func() { <-s.querySlots }()
		default:
			return fail(failure("busy"))
		}
		ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
		defer cancel()
		result, out, err := handler(ctx, req, input)
		if err != nil {
			return fail(err)
		}
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		size, err := serializedCallToolResultSize(result, out)
		if err != nil || size > maxToolResponseBytes {
			return fail(failure("response_too_large"))
		}
		return result, out, nil
	})
}

// Include the SDK's automatic text fallback alongside structuredContent, plus
// headroom for the JSON-RPC envelope and request id.
func serializedToolResultSize(out map[string]any) (int, error) {
	return serializedCallToolResultSize(nil, out)
}

func serializedCallToolResultSize(result *mcp.CallToolResult, out map[string]any) (int, error) {
	encoded, err := json.Marshal(out)
	if err != nil {
		return 0, err
	}
	copy := mcp.CallToolResult{}
	if result != nil {
		copy = *result
	}
	copy.StructuredContent = json.RawMessage(encoded)
	if copy.Content == nil {
		copy.Content = []mcp.Content{&mcp.TextContent{Text: string(encoded)}}
	}
	wire, err := json.Marshal(&copy)
	return len(wire) + 256, err
}
