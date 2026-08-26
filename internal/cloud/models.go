package cloud

import json "github.com/json-iterator/go"

// MessageType 消息类型
type MessageType string

const (
	MsgTypeHeartbeat    MessageType = "heartbeat"     // 心跳
	MsgTypeHeartbeatAck MessageType = "heartbeat_ack" // 心跳响应
	MsgTypeCommand      MessageType = "command"       // 指令
	MsgTypeResponse     MessageType = "response"      // 响应
)

// CloudMessage 云端消息包装
type CloudMessage struct {
	ID         string          `json:"id"`                   // 消息唯一标识
	Type       MessageType     `json:"type"`                 // 消息类型
	ClientID   string          `json:"client_id"`            // 客户端 ID (机器识别码)
	Payload    json.RawMessage `json:"payload"`              // 载荷
	Timestamp  int64           `json:"timestamp"`            // 时间戳
	Compressed bool            `json:"compressed,omitempty"` // 是否压缩
}

// HeartbeatPayload 心跳载荷
type HeartbeatPayload struct {
	Hostname            string          `json:"hostname"`                        // 主机名
	Version             string          `json:"version"`                         // 软件版本
	Status              string          `json:"status"`                          // 运行状态
	HardwareFingerprint string          `json:"hardware_fingerprint,omitempty"`  // 硬件指纹 JSON
	PagePath            string          `json:"page_path,omitempty"`             // 当前优先页面路径
	Href                string          `json:"href,omitempty"`                  // 当前优先页面 URL
	APIReady            bool            `json:"api_ready,omitempty"`             // 是否至少有一个就绪页面
	WSClients           int             `json:"ws_clients,omitempty"`            // 本地 WS 页面数量
	ReadyClients        int             `json:"ready_clients,omitempty"`         // 就绪页面数量
	SearchReadyClients  int             `json:"search_ready_clients,omitempty"`  // 搜索可用页面数量
	FeedReadyClients    int             `json:"feed_ready_clients,omitempty"`    // 视频列表可用页面数量
	ProfileReadyClients int             `json:"profile_ready_clients,omitempty"` // 视频详情可用页面数量
	SupportsSearch      bool            `json:"supports_search,omitempty"`       // 是否支持搜索
	SupportsFeed        bool            `json:"supports_feed,omitempty"`         // 是否支持视频列表
	SupportsProfile     bool            `json:"supports_profile,omitempty"`      // 是否支持视频详情
	Methods             map[string]bool `json:"methods,omitempty"`               // 本地页面方法合集
}

// CommandPayload 指令载荷
type CommandPayload struct {
	Action string          `json:"action"` // 执行动作 (e.g., "api_call")
	Data   json.RawMessage `json:"data"`   // 动作参数
}

// ResponsePayload 响应载荷
type ResponsePayload struct {
	RequestID string          `json:"request_id"` // 原始指令 ID
	Success   bool            `json:"success"`    // 是否成功
	Data      json.RawMessage `json:"data"`       // 返回数据
	Error     string          `json:"error"`      // 错误信息
}
