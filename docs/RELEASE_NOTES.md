# 版本更新说明

## 当前版本

- 版本号：`v5.7.2`
- 代码版本常量：`internal/version/version.go`
- 最新文档整理时间：`2026-08-23`

## 最新版本：v5.7.2（2026-08-23）

### 重点更新

- Insight Edge 成为 Cloud 版默认远程数据面，默认连接 `ws://127.0.0.1:18081/ws/client`。
- 删除旧 Hub 的设备绑定、记录主动同步和远端指标推送链路。
- 保留 `heartbeat`、`command`、`response`、gzip、远程 API 调用及页面能力状态上报。
- 继续兼容 `cloud_hub_url` 配置键，既有配置无需迁移；本地 Prometheus `/metrics` 不受影响。
- 配置样例、监控说明和启动日志统一改为 Insight Edge 语义。

## 上一发布版本：v5.7.1（2026-08-11）

- 分享链接统一通过已登录的微信桌面端视频号页面解析。
- Web 控制台可将解析结果直接加入批量下载列表。
- 移除 Cookie、Worker 和其他纯后端分享链接解析链路。

## 详细记录

- 完整更新日志：[`../CHANGELOG.md`](../CHANGELOG.md)
- Web 端版本说明：[`../web/docs/RELEASE_NOTES.md`](../web/docs/RELEASE_NOTES.md)
