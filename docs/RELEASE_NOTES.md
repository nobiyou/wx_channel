# 版本更新说明

## 当前版本

- 版本号：`v5.7.0`
- 代码版本常量：`internal/version/version.go`
- 最新文档整理时间：`2026-08-05`

## 最新已发布版本：v5.7.0（2026-08-05）

### 重点更新

- Insight Hub 集成：Cloud 版默认连接本地 `18081/ws/client`，支持显式配置远程 Insight 地址。
- 云端连接稳定：支持 WebSocket 断线切换、API 调用取消和边缘节点能力状态同步。
- 评论异步导出：通过任务状态接口轮询，避免长请求完成后出现 `Failed to fetch` 误报。
- 进度保护：评论导出支持分页上限、`.partial.json` 检查点和完整统计结果。
- 独立 `wx_channel_hub` 不再作为运行依赖；雷达默认关闭，雷达能力使用独立雷达版。

## 上一发布版本：v5.6.8（2026-06-05）

- 修复 `/api/channels/shared_feed/profile` 分享链接详情链路，恢复历史上的兼容接口行为。
- 新增 `/api/channels/share/resolve` 解析接口，支持自动、视频号页面、Cookie/Worker 纯后端三种模式。
- Web 控制台批量下载页新增分享链接导入入口，可解析后直接追加到下载列表。
- 设置接口补充 `sharedFeedBackendEnabled` 与 `sharedFeedBackendType`，页面可直接展示后端解析是否已配置。
- 补齐分享短链 `eid` fallback，页面接口异常时可回退到短链 ID 继续解析。
- 同步更新启动横幅、版本元数据与版本说明。

## 详细记录

- 完整更新日志：[`../CHANGELOG.md`](../CHANGELOG.md)
- Web 端版本说明：[`../web/docs/RELEASE_NOTES.md`](../web/docs/RELEASE_NOTES.md)
