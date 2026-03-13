package cloud

import json "github.com/json-iterator/go"

// MessageType 消息类型
type MessageType string

const (
	MsgTypeHeartbeat MessageType = "heartbeat" // 心跳
	MsgTypeCommand   MessageType = "command"   // 指令
	MsgTypeResponse  MessageType = "response"  // 响应
	MsgTypeEvent     MessageType = "event"     // 事件告警
	MsgTypeBind      MessageType = "bind"      // 设备绑定
	MsgTypeSyncData  MessageType = "sync_data" // 同步数据推送
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
	Hostname            string `json:"hostname"`                       // 主机名
	Version             string `json:"version"`                        // 软件版本
	Status              string `json:"status"`                         // 运行状态
	HardwareFingerprint string `json:"hardware_fingerprint,omitempty"` // 硬件指纹 JSON
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

// SyncDataPayload 同步数据载荷
type SyncDataPayload struct {
	SyncType string          `json:"sync_type"` // "browse" or "download"
	Records  json.RawMessage `json:"records"`   // 记录数组
	Count    int             `json:"count"`     // 记录数量
	HasMore  bool            `json:"has_more"`  // 是否还有更多数据
}
