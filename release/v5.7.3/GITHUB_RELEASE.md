# wx_channel v5.7.3

发布日期：2026-08-24

## 本次更新

- 修正启动界面仍显示 v5.7.0 更新要点的问题。
- 启动界面现在完整展示 Insight Edge 默认链路、Hub 独有功能退役、协议边界收敛、本地监控保留和运行语义统一五项说明。
- 新增启动更新要点与应用版本一致性检查，并阻止旧版评论导出文案再次混入。
- 修复 UTC+8 凌晨时段“今日下载”和下载趋势可能把本地记录归到前一天的问题。
- 运行行为继续沿用 v5.7.2 的 Insight Edge 融合方案，无配置迁移要求。

## 下载选择

- `wx_channel_v5.7.3.zip`：普通版，不主动连接 Insight Edge。
- `wx_channel_cloud_v5.7.3.zip`：Insight Edge / Cloud 版，适合与 `wx_channel_insight` 配合使用。
- `wx_channel_radar_v5.7.3.zip`：雷达版，默认启用雷达能力。

每个 ZIP 均包含对应 EXE、`README.md`、`config.yaml.example` 和 `config.yaml.full`。

## 兼容性说明

- 本地页面客户端与下载、评论导出、分享链接解析能力保持不变。
- `cloud_hub_url` 配置键继续有效，未调整远程协议或端口默认值。
- 本次版本不包含 `wx_channel_insight` 工作区中其他尚未独立发布的功能。
