# 版本更新说明

## 当前版本

- 版本号：`v5.6.6`
- 代码版本常量：`internal/version/version.go`
- 最新文档整理时间：`2026-05-26`

## 最新已发布版本：v5.6.6（2026-05-26）

### 重点更新

- 补齐下载文件名模板 `{size}` 占位符说明，并与实际代码支持保持一致。
- 统一批量下载、下载队列、队列转批量下载链路的命名元数据透传，减少文件名表现不一致。
- 更新 `/api/settings`、控制台设置页、配置文档中的 `downloadFilenameTemplate` / `radarEnabled` 说明。
- 清理“控制台即时启停雷达”的过时表述，统一改为“修改 `config.yaml` 后重启生效”。
- 启动阶段的日志初始化提示改为安全格式化输出，避免日志路径或提示文案中的 `%` 被误当成格式占位符。

## 上一发布版本：v5.6.5（2026-05-15）

- 完善原始视频下载，修复获取不到原始视频链接的问题。
- 新增评论列表 API，支持获取视频评论列表和回复分页。
- 下载文件名默认不再附带视频 ID，同标题文件自动追加序号避免覆盖。
- 支持 `download_filename_template`，可使用 `{date} {datetime} {author} {title} {duration} {video_id} {size}` 自定义命名。
- 对标雷达默认关闭，当前以 `config.yaml` 中的 `radar_enabled` 为准，控制台仅展示状态。

## 详细记录

- 完整更新日志：[`../CHANGELOG.md`](../CHANGELOG.md)
- Web 端版本说明：[`../web/docs/RELEASE_NOTES.md`](../web/docs/RELEASE_NOTES.md)
