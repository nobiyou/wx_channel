# 版本更新说明

## 当前版本

- 版本号：`v5.6.9`
- 代码版本常量：`internal/version/version.go`
- 最新文档整理时间：`2026-06-19`

## 最新已发布版本：v5.6.9（2026-06-19）

### 重点更新

- 评论导出改为分段落盘，导出时会保留 `.partial.json` 检查点文件，降低长评论任务中断后的重复抓取成本。
- 评论导出期间会临时锁住页面自动刷新，减少 15 分钟保活刷新打断导出的情况。
- 修复“后端已经导出成功，但页面仍提示 `获取评论列表失败<Failed to fetch>`”的前端误报。
- 成功提示补充一级评论、回复和合计条数，便于直接核对本次导出结果。
- 同步更新 README、About 页面、Windows 文件版本信息与版本说明。

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
