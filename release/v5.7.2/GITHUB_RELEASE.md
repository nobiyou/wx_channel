# wx_channel v5.7.2

发布日期：2026-08-23

## 本次更新

- Cloud 版默认对接 `wx_channel_insight` 的 Insight Edge，默认地址为 `ws://127.0.0.1:18081/ws/client`。
- 删除停止开发的 `wx_channel_hub` 独有设备绑定、`hub_sync` 主动同步和远端指标推送链路。
- 远程协议收敛为 `heartbeat`、`command`、`response`，继续支持 gzip、远程 API 调用、页面能力状态和断线重连。
- 保留 `cloud_hub_url` 配置键，现有配置无需迁移。
- 本地 Prometheus `/metrics` 保持可用，不依赖 Hub 服务。
- 配置样例、监控说明、启动日志和 About 页面统一使用 Insight Edge 语义。

## 下载选择

- `wx_channel_v5.7.2.zip`：普通版，不主动连接 Insight Edge。
- `wx_channel_cloud_v5.7.2.zip`：Insight Edge / Cloud 版，适合与 `wx_channel_insight` 配合使用。
- `wx_channel_radar_v5.7.2.zip`：雷达版，默认启用雷达能力。

每个 ZIP 均包含对应 EXE、`README.md`、`config.yaml.example` 和 `config.yaml.full`。

## 兼容性说明

- 本地页面客户端与下载、评论导出、分享链接解析能力保持不变。
- `wx_channel_insight` 的 Hub JWT bridge 仍可作为显式兼容路径使用，但不是团队版默认依赖。
- 本次版本不包含 `wx_channel_insight` 工作区中其他尚未独立发布的功能。
