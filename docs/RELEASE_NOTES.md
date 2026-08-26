# 版本更新说明

## 当前版本

- 版本号：`v5.7.6`
- 代码版本常量：`internal/version/version.go`
- 最新文档整理时间：`2026-08-26`

## 最新版本：v5.7.6（2026-08-26）

### 重点更新

- 修复 Cloud/Insight Edge WebSocket 心跳 ACK，避免长时间无业务消息时误报 `context deadline exceeded` 并断线重连。
- 增加连接活跃度与 `last_pong_at` 状态，过期客户端不参与搜索、列表和分享解析调度。
- 分享链接解析支持严格地址校验、批量并发、重复 URL 去重、超时控制和结构化错误码。
- `/api/channels/status` 增加页面/分享平台状态数量、fresh 客户端数量和可调度客户端数量。

### 运行兼容

- v5.7.5 的下载链路保护、视频号生命周期管理、Cloud/Insight Edge 配置和已有下载记录继续兼容。

## 上一发布版本：v5.7.5（2026-08-26）

- 修复微信 CDN 原始流识别和低码率转码文件误判问题。

## 上一发布版本：v5.7.4（2026-08-25）

- 启动时检测微信并自动打开、恢复视频号页面。
- 移除页面固定 15 分钟刷新，保留评论导出刷新锁和 Cloud/Insight Edge 兼容。

## 详细记录

- 完整更新日志：[`../CHANGELOG.md`](../CHANGELOG.md)
- Web 端版本说明：[`../web/docs/RELEASE_NOTES.md`](../web/docs/RELEASE_NOTES.md)
