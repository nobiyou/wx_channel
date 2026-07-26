# HTTP API 功能说明

## 概述

本项目实现了完整的 HTTP API 接口，允许通过标准 HTTP 请求获取微信视频号客户端代理拦截到的数据。

## 快速开始

1. 启动程序：`.\wx_channel.exe`
2. 启动微信视频号客户端并保持代理拦截可用
3. 调用 API：`curl "http://127.0.0.1:2026/api/channels/contact/search?keyword=纪录片"`

## API 端点

- `GET /api/channels/contact/search` - 搜索账号
- `GET /api/channels/feed/search` - 搜索视频
- `GET /api/channels/contact/feed/list` - 获取账号视频列表
- `GET /api/channels/feed/profile` - 获取视频详情
- `GET /api/channels/feed/comment/list` - 获取评论列表
- `POST /api/channels/feed/comment/export` - 导出评论
- `GET /api/channels/status` - 查询连接状态

## 详细文档

- **快速开始**: `API_QUICK_START.md`
- **配置说明**: `CONFIGURATION.md`
- **Web 控制台**: `WEB_CONSOLE.md`

## 注意事项

1. 必须先启动微信视频号客户端，并确认程序代理已拦截视频号 API
2. 使用 `username` 而不是 `nickname`
3. 建议请求间隔 0.5-1 秒
4. 检查响应体中的 `code` 判断成功/失败
5. 对标雷达默认关闭；如需使用，请运行 `wx_channel_radar.exe`，并在 `config.yaml` 中开启 `radar_enabled`

## 示例

```python
import requests

# 搜索账号
r = requests.get('http://127.0.0.1:2026/api/channels/contact/search',
                 params={'keyword': '纪录片'})
username = r.json()['data']['infoList'][0]['contact']['username']

# 搜索视频池
r = requests.get('http://127.0.0.1:2026/api/channels/feed/search',
                 params={'keyword': 'AI'})
searched_videos = r.json()['data']['data']['objectList']

# 获取视频列表
r = requests.get('http://127.0.0.1:2026/api/channels/contact/feed/list',
                 params={'username': username})
videos = r.json()['data']['object']
```

---

**版本**: 1.0.0 | **状态**: ✅ 生产就绪
