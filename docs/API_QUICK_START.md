# 微信视频号 HTTP API 快速开始

## 简介

本项目提供了完整的 HTTP API 接口，可以通过标准的 HTTP 请求获取微信视频号的数据。

## 架构

- **后端**: Go + WebSocket Hub + HTTP API
- **前端**: JavaScript + WebSocket Client
- **通信**: WebSocket 双向通信
- **端口**: 
  - 代理端口: 2025（默认）
  - API 端口: 2026（代理端口 + 1）

## 快速开始

### 1. 启动程序

```bash
.\wx_channel.exe
```

### 2. 打开微信视频号页面

在浏览器中打开任意微信视频号页面，等待页面加载完成。

### 3. 调用 API

#### 搜索账号

```bash
curl "http://127.0.0.1:2026/api/channels/contact/search?keyword=纪录片"
```

#### 获取账号视频列表

```bash
curl "http://127.0.0.1:2026/api/channels/contact/feed/list?username=账号username"
```

#### 获取视频详情

```bash
curl "http://127.0.0.1:2026/api/channels/feed/profile?object_id=视频ID&nonce_id=视频NonceID"
```

## API 端点

### 1. 搜索账号

**端点**: `GET /api/channels/contact/search`

**参数**:
- `keyword` (必需): 搜索关键词

**响应**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "infoList": [
      {
        "contact": {
          "username": "账号唯一标识",
          "nickname": "账号昵称",
          "headUrl": "头像URL"
        }
      }
    ]
  }
}
```

### 2. 获取账号视频列表

**端点**: `GET /api/channels/contact/feed/list`

**参数**:
- `username` (必需): 账号的 username（从搜索结果获取）
- `next_marker` (可选): 分页标记

**响应**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "object": [
      {
        "id": "视频ID",
        "objectNonceId": "视频NonceID",
        "objectDesc": {
          "description": "视频标题",
          "mediaType": 4
        }
      }
    ],
    "lastBuffer": "下一页标记"
  }
}
```

### 3. 获取视频详情

**端点**: `GET /api/channels/feed/profile`

**参数**:
- `object_id` (必需): 视频ID
- `nonce_id` (可选): 视频 NonceID
- `url` (可选): 视频页 URL 或分享链接；与 `object_id` 二选一，分享链接会自动走分享详情链路

**响应**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "object": {
      "objectDesc": {
        "description": "视频标题",
        "media": [{
          "url": "视频URL",
          "urlToken": "URL令牌"
        }]
      }
    }
  }
}
```

## Python 示例

```python
import requests

# 1. 搜索账号
response = requests.get('http://127.0.0.1:2026/api/channels/contact/search',
                       params={'keyword': '纪录片'})
accounts = response.json()['data']['infoList']

# 2. 获取第一个账号的视频列表
username = accounts[0]['contact']['username']
response = requests.get('http://127.0.0.1:2026/api/channels/contact/feed/list',
                       params={'username': username})
videos = response.json()['data']['object']

# 3. 获取第一个视频的详情
video = videos[0]
response = requests.get('http://127.0.0.1:2026/api/channels/feed/profile',
                       params={
                           'object_id': video['id'],
                           'nonce_id': video['objectNonceId']
                       })
detail = response.json()['data']['object']
```

## 注意事项

1. **必须先打开微信视频号页面** - API 通过 WebSocket 与页面通信
2. **使用正确的 username** - 从搜索结果中获取，不是昵称
3. **请求频率** - 建议在请求之间添加适当延迟（0.5-1秒）
4. **错误处理** - 检查 `code`，0 表示成功
5. **对标雷达默认关闭** - 雷达监控需要运行 `wx_channel_radar.exe`，且默认以 `config.yaml` 中的 `radar_enabled` 为准

## 故障排查

### API 返回超时

- 确认微信视频号页面已打开
- 检查后端日志是否有 WebSocket 连接
- 刷新页面重新建立连接

### 返回空数据

- 确认使用的是 `username` 而不是 `nickname`
- 检查参数是否正确
- 查看 `BaseResponse.Ret` 是否为 0

### 视频详情接口返回 400

- `GET /api/channels/feed/profile` 当前使用 `object_id` / `nonce_id` 参数名，不使用 `objectId` / `nonceId`
- 也可以直接传 `url`，用于视频页地址或分享链接解析

### 雷达监控获取不到视频列表

- 普通版 `wx_channel.exe` 默认不启用对标雷达
- 需要使用 `wx_channel_radar.exe`
- 并确认 `config.yaml` 中 `radar_enabled: true` 后重启程序

## 更多信息

详细的 API 文档请参考：
- `docs/API_README.md` - API 功能说明
- `docs/CONFIGURATION.md` - 配置项说明

---

**版本**: 1.0.0  
**状态**: ✅ 生产就绪
