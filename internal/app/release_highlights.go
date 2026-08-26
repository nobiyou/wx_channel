package app

const releaseHighlightsVersion = "5.7.6"

var releaseHighlights = [...]string{
	"WebSocket 保活修复 - Cloud 客户端正确确认 heartbeat_ack，避免约 150 秒误断线",
	"连接状态可观测 - 增加 fresh、last_pong 和平台可用状态，过期客户端不参与调度",
	"分享链接批量解析 - 支持去重、并发、单项超时和结构化错误码",
	"分享地址校验强化 - 仅接受规范 HTTPS 微信视频号分享地址",
	"API 调度错误收敛 - 统一识别无可用客户端、无就绪客户端和请求超时",
}
