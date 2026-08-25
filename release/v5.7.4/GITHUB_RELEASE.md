# wx_channel v5.7.4

发布日期：2026-08-25

## 本次更新

- 启动时检测 `Weixin.exe`；微信未运行时只等待，不启动或结束微信进程。
- 微信运行时通过 `weixin://dl/channels` 打开视频号页面，并由 wx_channel 负责页面刷新与恢复。
- 新增 `runtime.lifecycle` 诊断、定向页面刷新/导航命令和有界退避恢复。
- 移除页面固定 15 分钟自动刷新，保留评论导出刷新锁和手动刷新能力。
- Cloud 版继续使用 Insight Edge 默认链路，现有配置兼容不变。
- 移除本地 Prometheus `/metrics` 运行层，运行健康统一通过 `/api/channels/status`、`/ws/health` 和 Insight 业务指标提供。

## 下载选择

- `wx_channel_v5.7.4.zip`：普通版，不主动连接 Insight Edge。
- `wx_channel_cloud_v5.7.4.zip`：Insight Edge / Cloud 版，适合与 `wx_channel_insight` 配合使用。
- `wx_channel_radar_v5.7.4.zip`：雷达版，默认启用雷达能力。

每个 ZIP 均包含对应 EXE、`README.md`、`config.yaml.example` 和 `config.yaml.full`。

## 兼容性说明

- 本地页面客户端与下载、评论导出、分享链接解析能力保持不变。
- `cloud_hub_url` 配置键继续有效，未调整远程协议或端口默认值。
- 本次版本不包含 `wx_channel_insight` 工作区中其他尚未独立发布的功能。
