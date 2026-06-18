package cloud

import json "github.com/json-iterator/go"

type mappedAPICall struct {
	Key  string      `json:"key"`
	Body interface{} `json:"body"`
}

type mappedSearchCommand struct {
	Keyword    string `json:"keyword"`
	Type       int    `json:"type,omitempty"`
	NextMarker string `json:"next_marker,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
}

type mappedDownloadCommand struct {
	VideoURL     string            `json:"videoUrl,omitempty"`
	URL          string            `json:"url,omitempty"`
	VideoID      string            `json:"videoId,omitempty"`
	Title        string            `json:"title,omitempty"`
	Author       string            `json:"author,omitempty"`
	SourceURL    string            `json:"sourceUrl,omitempty"`
	PageURL      string            `json:"pageUrl,omitempty"`
	UserAgent    string            `json:"userAgent,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Key          string            `json:"key,omitempty"`
	DecryptKey   string            `json:"decryptKey,omitempty"`
	ForceSave    bool              `json:"forceSave,omitempty"`
	Resolution   string            `json:"resolution,omitempty"`
	Width        int               `json:"width,omitempty"`
	Height       int               `json:"height,omitempty"`
	FileFormat   string            `json:"fileFormat,omitempty"`
	LikeCount    int64             `json:"likeCount,omitempty"`
	CommentCount int64             `json:"commentCount,omitempty"`
	ForwardCount int64             `json:"forwardCount,omitempty"`
	FavCount     int64             `json:"favCount,omitempty"`
}

func mapCommandToAPICall(action string, data json.RawMessage) (json.RawMessage, bool, error) {
	switch action {
	case "search_channels":
		var req mappedSearchCommand
		if len(data) > 0 {
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, true, err
			}
		}
		req.Type = 1
		payload, err := buildMappedAPICallPayload("key:channels:contact_list", req)
		return payload, true, err
	case "search_videos":
		var req mappedSearchCommand
		if len(data) > 0 {
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, true, err
			}
		}
		req.Type = 3
		payload, err := buildMappedAPICallPayload("key:channels:contact_list", req)
		return payload, true, err
	case "download_video":
		var req mappedDownloadCommand
		if len(data) > 0 {
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, true, err
			}
		}
		payload, err := buildMappedAPICallPayload("key:channels:download_video", normalizeDownloadCommand(req))
		return payload, true, err
	default:
		return nil, false, nil
	}
}

func buildMappedAPICallPayload(key string, body interface{}) (json.RawMessage, error) {
	return json.Marshal(mappedAPICall{
		Key:  key,
		Body: body,
	})
}

func normalizeDownloadCommand(req mappedDownloadCommand) map[string]interface{} {
	videoURL := req.VideoURL
	if videoURL == "" {
		videoURL = req.URL
	}

	sourceURL := req.SourceURL
	if sourceURL == "" {
		sourceURL = req.PageURL
	}

	key := req.Key
	if key == "" {
		key = req.DecryptKey
	}

	body := map[string]interface{}{
		"videoUrl": videoURL,
	}

	if req.VideoID != "" {
		body["videoId"] = req.VideoID
	}
	if req.Title != "" {
		body["title"] = req.Title
	}
	if req.Author != "" {
		body["author"] = req.Author
	}
	if sourceURL != "" {
		body["sourceUrl"] = sourceURL
	}
	if req.UserAgent != "" {
		body["userAgent"] = req.UserAgent
	}
	if len(req.Headers) > 0 {
		body["headers"] = req.Headers
	}
	if key != "" {
		body["key"] = key
	}
	if req.ForceSave {
		body["forceSave"] = true
	}
	if req.Resolution != "" {
		body["resolution"] = req.Resolution
	}
	if req.Width > 0 {
		body["width"] = req.Width
	}
	if req.Height > 0 {
		body["height"] = req.Height
	}
	if req.FileFormat != "" {
		body["fileFormat"] = req.FileFormat
	}
	if req.LikeCount != 0 {
		body["likeCount"] = req.LikeCount
	}
	if req.CommentCount != 0 {
		body["commentCount"] = req.CommentCount
	}
	if req.ForwardCount != 0 {
		body["forwardCount"] = req.ForwardCount
	}
	if req.FavCount != 0 {
		body["favCount"] = req.FavCount
	}

	return body
}
