# wx_channel v5.7.5

发布日期：2026-08-26

## 本次更新

- 修复微信 CDN 原始流规则变化后，低码率转码文件被误标为“原始视频”的问题。
- 仅当微信页面数据明确提供 `fullUrl` 时才进入原始视频模式。
- 微信未返回原始流时自动选择最高可用画质，当前桌面端优先 `xWT111`，并在界面明确提示。
- 保留完整签名 CDN URL，页面直连失败时自动回退后端下载。
- 增加原始视频大小校验，拦截明显低于源文件提示的转码流。
- 兼容 v5.7.4 的视频号生命周期管理、Cloud/Insight Edge 配置和已有下载记录。

## 下载包

- `wx_channel_v5.7.5.zip`：普通版，不主动连接 Insight Edge。
- `wx_channel_cloud_v5.7.5.zip`：Insight Edge / Cloud 版，适合与 `wx_channel_insight` 配合使用。
- `wx_channel_radar_v5.7.5.zip`：雷达版，默认启用雷达能力。

每个 ZIP 均包含对应 EXE、`README.md`、`config.yaml.example` 和 `config.yaml.full`。

## 兼容性说明

- 当前微信接口未明确下发原始流时，软件不会把转码文件冒充原片；实际下载为微信返回的最高可用规格。
- `cloud_hub_url` 配置键继续有效，Cloud 版默认连接 `ws://127.0.0.1:18081/ws/client`。
- 本版本不删除已有下载记录或历史文件。
