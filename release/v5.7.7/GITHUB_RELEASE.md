# wx_channel v5.7.7

发布日期：2026-08-26

## 本次更新

- 修复批量下载在微信未提供真实原始 `fullUrl` 时仍按原始视频模式请求的问题。
- 批量下载与单个下载共用最高可用画质回退规则；没有原始流时自动选择最高可用 `spec` 画质。
- 回退到具体画质时保留完整 CDN 签名参数。
- 保留低码率保护，疑似转码流不会被误保存为原片。
- 增加 `4.26 MB -> 0.25 MB` 低码率回归测试和批量回退规范化测试。

## 下载包

- `wx_channel_v5.7.7.zip`：普通版，不主动连接 Insight Edge。
- `wx_channel_cloud_v5.7.7.zip`：Insight Edge / Cloud 版，适合与 `wx_channel_insight` 配合使用。
- `wx_channel_radar_v5.7.7.zip`：雷达版，默认启用雷达能力。

每个 ZIP 均包含对应 EXE、`README.md`、`config.yaml.example` 和 `config.yaml.full`。

## 兼容性说明

- v5.7.6 的 Cloud/Insight Edge 保活、分享解析和运行配置继续兼容。
- 不删除已有下载记录或历史文件。
