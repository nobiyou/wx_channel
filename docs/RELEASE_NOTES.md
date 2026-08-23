# 版本更新说明

## 当前版本

- 版本号：`v5.7.3`
- 代码版本常量：`internal/version/version.go`
- 最新文档整理时间：`2026-08-24`

## 最新版本：v5.7.3（2026-08-24）

### 重点更新

- 修正启动界面仍显示旧版更新要点的问题，启动时完整展示下列五项融合说明。
- 修复 UTC+8 凌晨时段今日下载统计与趋势图的日期偏移。
- Insight Edge 成为 Cloud 版默认远程数据面，默认连接 `ws://127.0.0.1:18081/ws/client`。
- 删除旧 Hub 的设备绑定、记录主动同步和远端指标推送链路。
- 保留 `heartbeat`、`command`、`response`、gzip、远程 API 调用及页面能力状态上报。
- 继续兼容 `cloud_hub_url` 配置键，既有配置无需迁移；本地 Prometheus `/metrics` 不受影响。
- 配置样例、监控说明和启动日志统一改为 Insight Edge 语义。

## 上一发布版本：v5.7.2（2026-08-23）

- 完成 Insight Edge 默认链路适配并退役停止开发的 Hub 独有功能。
- 收敛远程协议边界，保留现有配置兼容和本地监控能力。

## 详细记录

- 完整更新日志：[`../CHANGELOG.md`](../CHANGELOG.md)
- Web 端版本说明：[`../web/docs/RELEASE_NOTES.md`](../web/docs/RELEASE_NOTES.md)
