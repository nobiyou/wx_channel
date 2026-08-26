# wx_channel v5.7.6

发布日期：2026-08-26

## 本次更新

- 修复 Cloud 客户端与 Insight Edge 的 WebSocket 心跳 ACK：Edge 返回同 ID 的 `heartbeat_ack`，客户端确认后更新活跃时间，避免约 150 秒无业务消息时误断线。
- 增加 WebSocket 写入串行化和页面 API 心跳 ACK 超时重连，提升长时间运行稳定性。
- `/api/channels/status` 增加 `fresh_clients`、`schedulable_clients`、`last_pong_at`、平台状态和过期阈值；过期客户端不参与 API 调度。
- 分享链接解析增加规范 HTTPS 校验、批量上限、重复 URL 去重、并发解析、单项/批次超时和结构化错误码。
- 统一无可用客户端、无就绪客户端和请求超时的 API 错误识别。

## 下载包

- `wx_channel_v5.7.6.zip`：普通版，不主动连接 Insight Edge。
- `wx_channel_cloud_v5.7.6.zip`：Insight Edge / Cloud 版，适合与 `wx_channel_insight` 配合使用。
- `wx_channel_radar_v5.7.6.zip`：雷达版，默认启用雷达能力。

每个 ZIP 均包含对应 EXE、`README.md`、`config.yaml.example` 和 `config.yaml.full`。

## 兼容性说明

- `cloud_hub_url` 配置键继续有效，Cloud 版默认连接 `ws://127.0.0.1:18081/ws/client`。
- `wx_channel_insight` 需要支持 `heartbeat_ack` 的 Edge 版本，旧 Edge 版本会触发客户端的 ACK 超时保护并自动重连。
- 本版本不删除已有下载记录或历史文件。
