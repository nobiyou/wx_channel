# wx_channel v5.7.19

本版本完善 Codex、Claude Code、DeepSeek Harness 的本地 MCP 接入，并提供独立 Windows 环境检查与修复工具，同时修复 MCP 在 Linux 上对 Windows 路径的识别。

## 主要更新

- 三类本地 Agent 共用 Streamable HTTP MCP 服务和 Bearer Token 鉴权，默认地址为 `http://127.0.0.1:2027/mcp`。
- 修正 Claude Code JSON 示例的 `type: http`，新增 Codex TOML 和 DeepSeek Harness YAML 配置模板，随三个主程序 ZIP 包分发。
- 提供 17 个工具，覆盖账号搜索、视频搜索、账号视频列表、视频详情、评论、分享链接、本地记录、下载及视频分析。
- 视频分析使用当前连接的 MCP 客户端会话模型；画面复核需要客户端模型支持图片输入，外部模型 API 继续要求显式授权。
- 新增独立环境诊断工具，内嵌检查和修复菜单，无需另附检查或修复脚本。
- 环境诊断区分页面心跳与微信扩展 API 就绪状态，避免把“页面已连接”误报为“业务接口可用”。

- 修复 Linux 将 Windows 盘符、UNC 和根路径当作相对文件名的问题；本地文件引用和媒体读取统一拒绝越界写法，保留下载目录内合法的原生路径。

v5.7.18 为未正式发布的候选版本；本次正式发布包含其全部更新。

## 下载包

- `wx_channel_v5.7.19.zip`：标准版。
- `wx_channel_cloud_v5.7.19.zip`：Cloud 版。
- `wx_channel_radar_v5.7.19.zip`：Radar 版。
- `wx_channel_video_analysis_v5.7.19.zip`：标准版和 FFmpeg 视频分析工具。
- `wx-channel-environment.zip`：独立 Windows 环境检查与修复工具，适用于 Windows 10/11。
- `SHA256SUMS.txt`：上述五个 ZIP 文件的 SHA-256 校验值。

## MCP 接入

解压后按客户端选择 `mcp.codex.toml.example`、`mcp.json.example` 或 `mcp.dsh.yaml.example`。鉴权值来自配置下载目录内的 `.wx_channel_mcp_token`；配置示例不包含真实 Token。

本机需要运行 wx_channel；实时搜索、详情和评论还要求微信视频号页面及对应 API 就绪。配置模板不会自动修改客户端的全局配置。

## 验证

- Windows 与 Linux Go CI 验证，以及 JavaScript 注入与 Web 测试 106 项通过。
- 本地客户端兼容验证：Codex 原生连接发现 17 个工具；Claude Code 原生连接检查成功；DeepSeek Harness 的 17 个工具输入结构校验和运行状态调用通过。
- 三个 Windows 主程序的内嵌版本与文件版本均为 `5.7.19`。
- ZIP 中主程序与对应独立 EXE 的 SHA-256 一致，三端 MCP 模板和工作流 Skill 均包含在主程序包内。
- 发布包不包含运行时日志、下载记录、私有配置、MCP Token 或硬件身份文件。
