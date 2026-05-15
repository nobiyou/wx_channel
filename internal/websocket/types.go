package websocket

import json "github.com/json-iterator/go"

// WebSocket 消息类型
type WSMessageType string

const (
	WSMessageTypeAPICall     WSMessageType = "api_call"
	WSMessageTypeAPIResponse WSMessageType = "api_response"
	WSMessageTypePing        WSMessageType = "ping"
	WSMessageTypePong        WSMessageType = "pong"
	WSMessageTypeCommand     WSMessageType = "cmd"
	WSMessageTypeClientState WSMessageType = "client_state"
)

// WebSocket 消息
type WSMessage struct {
	Type WSMessageType   `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// API 调用请求
type APICallRequest struct {
	ID   string      `json:"id"`
	Key  string      `json:"key"`
	Body interface{} `json:"body"`
}

// API 调用响应
type APICallResponse struct {
	ID      string          `json:"id"`
	Data    json.RawMessage `json:"data"`
	ErrCode int             `json:"errCode,omitempty"`
	ErrMsg  string          `json:"errMsg,omitempty"`
}

// 搜索账号请求体
type SearchContactBody struct {
	Keyword    string `json:"keyword"`
	Type       int    `json:"type"`        // 1=User, 2=Live, 3=Video
	NextMarker string `json:"next_marker"` // for pagination (lastBuff)
	RequestId  string `json:"request_id"`
}

// 获取账号视频列表请求体
type FeedListBody struct {
	Username   string `json:"username"`
	NextMarker string `json:"next_marker"`
}

// 获取视频详情请求体
type FeedProfileBody struct {
	ObjectID string `json:"objectId"`
	NonceID  string `json:"nonceId"`
	URL      string `json:"url"`
}

// 获取视频评论列表请求体
type FeedCommentListBody struct {
	ObjectID   string `json:"object_id"`
	NonceID    string `json:"nonce_id"`
	CommentID  string `json:"comment_id"`
	NextMarker string `json:"next_marker"`
}

// ClientStateBody 前端客户端状态
type ClientStateBody struct {
	PagePath   string          `json:"pagePath"`
	Href       string          `json:"href"`
	APIReady   bool            `json:"apiReady"`
	Methods    map[string]bool `json:"methods"`
	Timestamp  int64           `json:"timestamp"`
	UserAgent  string          `json:"userAgent,omitempty"`
	Visible    bool            `json:"visible,omitempty"`
}

type ClientStatus struct {
	RemoteAddr      string          `json:"remote_addr"`
	PagePath        string          `json:"page_path"`
	Href            string          `json:"href"`
	APIReady        bool            `json:"api_ready"`
	Methods         map[string]bool `json:"methods"`
	ActiveRequests  int             `json:"active_requests"`
	LastSeenAt      string          `json:"last_seen_at"`
	LastPingAt      string          `json:"last_ping_at"`
	SupportsSearch  bool            `json:"supports_search"`
	SupportsFeed    bool            `json:"supports_feed"`
	SupportsProfile bool            `json:"supports_profile"`
	SupportsComment bool            `json:"supports_comment"`
}
