# wx_channel v5.7.0

本次版本完成 Insight Hub 集成，并集中优化云端连接和评论导出稳定性。

## 更新内容

- Cloud 版改为连接 `wx_channel_insight` 内置的边缘网关，默认地址为 `ws://127.0.0.1:18081/ws/client`，显式配置仍优先。
- 增强 WebSocket 断线切换、可取消 API 调用和边缘节点能力状态同步。
- 评论导出改为后台任务，新增状态轮询接口，避免长 HTTP 请求完成后出现 `Failed to fetch` 误报。
- 评论导出增加分页上限、`.partial.json` 检查点和一级评论、回复、总数统计。
- 独立 `wx_channel_hub` 不再作为运行依赖；雷达默认关闭，雷达能力继续通过独立雷达版提供。

## 下载说明

- `wx_channel_v5.7.0.zip` / `wx_channel_v5.7.0.exe`
  - 标准版
  - 默认关闭云端和雷达

- `wx_channel_cloud_v5.7.0.zip` / `wx_channel_cloud_v5.7.0.exe`
  - Cloud / Insight 版
  - 默认连接 Insight `18081/ws/client`
  - 默认关闭雷达

- `wx_channel_radar_v5.7.0.zip` / `wx_channel_radar_v5.7.0.exe`
  - 雷达版
  - 默认启用雷达能力

## 使用提醒

- `cloud_hub_url` 可在 `config.yaml` 中配置，显式配置优先于本地默认地址。
- 评论导出过程中出现 `.partial.json` 文件属于正常现象，表示当前进度检查点。
- 需要 Insight 云端协同前，请先启动 Insight API 和边缘网关。

## 本地验证

- `go test -count=1 ./...`
- `go test -race ./internal/api ./internal/websocket`
- `powershell -ExecutionPolicy Bypass -File .\scripts\build-variants.ps1`
- Insight 节点状态：`running`，`readyClients=1`，`feedReadyClients=1`
