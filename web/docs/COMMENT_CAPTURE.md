# Feed 页面评论列表 API

## 功能概述

当前版本不再使用页面 DOM / Store 评论采集。

评论能力已经切换为统一的评论列表 API 链路：

1. 前端通过注入脚本调用微信页面内的 `finderGetCommentList`
2. 本地 WebSocket Hub 转发请求
3. 本地 HTTP API 返回评论列表结果

这条链路支持：
- 获取视频一级评论列表
- 按 `comment_id` 获取某条评论的回复列表
- 使用 `next_marker` 分页

## API 端点

### 获取评论列表

- 路径：`/api/channels/feed/comment/list`
- 方法：`GET` / `POST`

查询参数或请求体字段：

```json
{
  "object_id": "视频 object_id",
  "nonce_id": "视频 nonce_id",
  "comment_id": "",
  "next_marker": ""
}
```

字段说明：
- `object_id`：必填，视频 ID
- `nonce_id`：获取一级评论时必填
- `comment_id`：获取某条评论回复列表时使用
- `next_marker`：分页标记，使用上一次返回的 `lastBuffer`

### 响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "errCode": 0,
    "errMsg": "ok",
    "data": {
      "commentInfo": [
        {
          "username": "v2_xxx@finder",
          "nickname": "示例用户",
          "content": "评论内容",
          "commentId": "14901879320262215928",
          "replyCommentId": "0",
          "headUrl": "https://wx.qlogo.cn/...",
          "levelTwoComment": [],
          "createtime": "1776442446",
          "likeCount": 131,
          "expandCommentCount": 7
        }
      ],
      "countInfo": {
        "commentCount": 128
      },
      "lastBuffer": "分页标记"
    }
  }
}
```

## 使用方式

### 1. 获取一级评论

```http
GET /api/channels/feed/comment/list?object_id=14885640628103354450&nonce_id=16708144249542964913
```

### 2. 获取下一页评论

```http
GET /api/channels/feed/comment/list?object_id=14885640628103354450&nonce_id=16708144249542964913&next_marker=xxx
```

### 3. 获取某条评论的回复

```http
GET /api/channels/feed/comment/list?object_id=14885640628103354450&comment_id=14901879320262215928
```

## 页面按钮行为

Feed 页面顶部按钮已经从“采集评论”改为“获取评论”。

该按钮会直接调用评论列表 API，不再：
- 自动打开评论侧栏
- 监听页面 Store
- 解析 DOM

点击“获取评论”后，当前版本会将评论导出到本地 `comment_data/` 目录：

- 正常完成时生成最终 `.json` 文件
- 若在长时间导出过程中被中断，会保留一个 `.partial.json` 检查点文件，避免已抓取内容全部丢失

## 技术实现

### 后端

- HTTP 入口：`internal/api/search.go`
- WebSocket 协议：`internal/websocket/types.go`
- 客户端能力选择：`internal/websocket/client.go`

### 前端注入

- API 调用入口：`internal/assets/inject/api_client.js`
- Feed 页面按钮：`internal/assets/inject/feed.js`
- 页面 API 拦截：`internal/handlers/script.go`

## 不再支持的旧能力

以下旧评论采集路径已退役：
- `POST /__wx_channels_api/save_comment_data`
- `/api/control/comment/start`
- 基于 DOM / Pinia / Vuex 的自动评论采集
- 自动滚动评论区和自动展开回复
- 旧的页面侧 `save_comment_data` 保存链路

## 注意事项

1. 获取一级评论需要 `object_id + nonce_id`
2. 获取某条评论的回复时可以只提供 `object_id + comment_id`
3. 分页依赖响应中的 `lastBuffer`
4. 该接口要求存在可用的微信页面，并且页面内 API 已初始化
