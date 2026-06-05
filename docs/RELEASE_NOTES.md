# 版本更新说明

## 当前版本

- 版本号：`v5.6.8`
- 代码版本常量：`internal/version/version.go`
- 最新文档整理时间：`2026-06-05`

## 最新已发布版本：v5.6.8（2026-06-05）

### 重点更新

- 修复 `/api/channels/shared_feed/profile` 分享链接详情链路，恢复历史上的兼容接口行为。
- 新增 `/api/channels/share/resolve` 解析接口，支持自动、视频号页面、Cookie/Worker 纯后端三种模式。
- Web 控制台批量下载页新增分享链接导入入口，可解析后直接追加到下载列表。
- 设置接口补充 `sharedFeedBackendEnabled` 与 `sharedFeedBackendType`，页面可直接展示后端解析是否已配置。
- 补齐分享短链 `eid` fallback，页面接口异常时可回退到短链 ID 继续解析。
- 同步更新启动横幅、版本元数据与版本说明。

## 上一发布版本：v5.6.7（2026-05-29）

- 加固批量下载文件处理流程，下载、解密、校验与最终落盘分阶段完成。
- 修复部分批量下载视频无法播放的问题，减少异常中断后产出损坏成品文件。
- 批量任务暂停与恢复改为真实调用 Gopeed 任务级 `Pause` / `Continue`。
- 优化并发下载场景下的状态一致性，降低任务恢复后文件链路异常的概率。
- 同步更新启动横幅与版本元数据，避免运行时提示和文档版本不一致。

## 详细记录

- 完整更新日志：[`../CHANGELOG.md`](../CHANGELOG.md)
- Web 端版本说明：[`../web/docs/RELEASE_NOTES.md`](../web/docs/RELEASE_NOTES.md)
