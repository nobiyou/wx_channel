# API 文档

## 概述

微信视频号下载助手提供了一套完整的 HTTP API，支持视频下载、批量处理、评论采集等功能。

## 基础信息

### 服务地址

```
http://127.0.0.1:2025
```

默认端口为 2025，可通过命令行参数 `-p` 或环境变量 `WX_CHANNEL_PORT` 修改。

### 认证

如果配置了 `WX_CHANNEL_TOKEN`，所有 API 请求需要在请求头中携带：

```
X-Local-Auth: your_secret_token
```

### CORS 支持

支持跨域请求，可通过 `WX_CHANNEL_ALLOWED_ORIGINS` 配置允许的来源。

---

## 视频下载 API

### 1. 分片上传初始化

**接口**：`POST /__wx_channels_api/init_upload`

**功能**：初始化分片上传任务

**请求体**：

```json
{
  "filename": "视频文件名.mp4",
  "totalSize": 10485760,
  "chunkSize": 2097152
}
```

**响应**：

```json
{
  "uploadId": "unique_upload_id",
  "chunkSize": 2097152
}
```

### 2. 上传分片

**接口**：`POST /__wx_channels_api/upload_chunk`

**功能**：上传视频文件分片

**请求体**：

```json
{
  "uploadId": "unique_upload_id",
  "chunkIndex": 0,
  "data": "base64_encoded_chunk_data"
}
```

**响应**：

```json
{
  "success": true
}
```

### 3. 完成上传

**接口**：`POST /__wx_channels_api/complete_upload`

**功能**：完成分片上传，合并文件

**请求体**：

```json
{
  "uploadId": "unique_upload_id",
  "filename": "视频文件名.mp4",
  "authorName": "作者名称",
  "videoInfo": {
    "id": "video_id",
    "title": "视频标题",
    "url": "视频URL"
  }
}
```

**响应**：

```json
{
  "success": true,
  "path": "downloads/作者名称/视频文件名.mp4"
}
```

### 4. 查询上传状态

**接口**：`GET /__wx_channels_api/upload_status?uploadId=xxx`

**功能**：查询已上传的分片列表

**响应**：

```json
{
  "uploadId": "unique_upload_id",
  "chunks": [0, 1, 2, 3]
}
```

### 5. 直接保存视频

**接口**：`POST /__wx_channels_api/save_video`

**功能**：直接保存小视频文件（不分片）

**请求体**：

```json
{
  "filename": "视频文件名.mp4",
  "data": "base64_encoded_video_data",
  "authorName": "作者名称",
  "videoInfo": {
    "id": "video_id",
    "title": "视频标题",
    "url": "视频URL"
  }
}
```

**响应**：

```json
{
  "success": true,
  "path": "downloads/作者名称/视频文件名.mp4"
}
```

---

## 批量下载 API

### 1. 开始批量下载

**接口**：`POST /__wx_channels_api/batch_start`

**功能**：提交批量下载任务，支持视频解密

**请求体**：

```json
{
  "videos": [
    {
      "id": "视频ID",
      "url": "视频下载地址",
      "title": "视频标题",
      "filename": "文件名（可选）",
      "authorName": "作者名称",
      "decryptorPrefix": "Base64编码的解密密钥（可选）",
      "prefixLen": 1024
    }
  ],
  "forceRedownload": false
}
```

**参数说明**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| videos | Array | 是 | 视频列表 |
| videos[].id | String | 是 | 视频唯一标识 |
| videos[].url | String | 是 | 视频下载地址 |
| videos[].title | String | 是 | 视频标题 |
| videos[].filename | String | 否 | 自定义文件名 |
| videos[].authorName | String | 是 | 作者名称 |
| videos[].decryptorPrefix | String | 否 | Base64 编码的解密密钥 |
| videos[].prefixLen | Number | 否 | 解密长度 |
| forceRedownload | Boolean | 否 | 是否强制重新下载 |

**响应**：

```json
{
  "success": true,
  "total": 10
}
```

### 2. 查询下载进度

**接口**：`GET /__wx_channels_api/batch_progress`

**功能**：查询当前批量下载的进度

**响应**：

```json
{
  "success": true,
  "total": 10,
  "done": 5,
  "failed": 1,
  "running": 4,
  "currentTask": {
    "title": "正在下载的视频标题",
    "progress": 45.5,
    "downloaded": 4718592,
    "total": 10485760
  }
}
```

### 3. 取消批量下载

**接口**：`POST /__wx_channels_api/batch_cancel`

**功能**：取消当前正在进行的批量下载

**响应**：

```json
{
  "success": true,
  "message": "批量下载已取消"
}
```

### 4. 导出失败清单

**接口**：`GET /__wx_channels_api/batch_failed`

**功能**：导出下载失败的视频清单

**响应**：

```json
{
  "success": true,
  "failed": 2,
  "json": "downloads/batch_failed_20251123_143000.json"
}
```

---

## 下载记录 API

### 1. 记录下载信息

**接口**：`POST /__wx_channels_api/record_download`

**功能**：记录视频下载信息到 CSV

**请求体**：

```json
{
  "id": "video_id",
  "title": "视频标题",
  "finderNickname": "视频号名称",
  "finderCategory": "视频号分类",
  "officialAccountName": "公众号名称",
  "videoUrl": "视频链接",
  "pageUrl": "页面链接",
  "fileSize": "10.5 MB",
  "duration": "180",
  "readCount": "1000",
  "likeCount": "100",
  "commentCount": "50",
  "favCount": "20",
  "forwardCount": "10",
  "createTime": "2025-11-23 14:30:00",
  "ipLocation": "北京",
  "searchKeyword": "搜索关键词"
}
```

**响应**：

```json
{
  "success": true
}
```

### 2. 导出视频列表（TXT）

**接口**：`POST /__wx_channels_api/export_video_list`

**功能**：导出视频链接为 TXT 格式

**请求体**：

```json
{
  "videos": [
    {
      "url": "https://example.com/video1.mp4",
      "title": "视频1"
    }
  ]
}
```

**响应**：

```json
{
  "success": true,
  "file": "downloads/video_links_20251123_143000.txt"
}
```

### 3. 导出视频列表（JSON）

**接口**：`POST /__wx_channels_api/export_video_list_json`

**功能**：导出视频信息为 JSON 格式

**请求体**：同上

**响应**：

```json
{
  "success": true,
  "file": "downloads/video_list_20251123_143000.json"
}
```

### 4. 导出视频列表（Markdown）

**接口**：`POST /__wx_channels_api/export_video_list_md`

**功能**：导出视频信息为 Markdown 格式

**请求体**：同上

**响应**：

```json
{
  "success": true,
  "file": "downloads/video_list_20251123_143000.md"
}
```

### 5. 查询批量下载状态

**接口**：`GET /__wx_channels_api/batch_download_status`

**功能**：查询前端批量下载状态

**响应**：

```json
{
  "isRunning": true,
  "total": 10,
  "completed": 5,
  "failed": 1
}
```

---

## 评论采集 API

### 保存评论数据

**接口**：`POST /__wx_channels_api/save_comment_data`

**功能**：保存视频评论数据

**请求体**：

```json
{
  "videoId": "video_id",
  "videoTitle": "视频标题",
  "comments": [
    {
      "commentId": "comment_id",
      "content": "评论内容",
      "nickname": "用户昵称",
      "createTime": "2025-11-23 14:30:00",
      "likeCount": 10,
      "replyCount": 2
    }
  ],
  "originalCommentCount": 100,
  "timestamp": 1700730000000
}
```

**响应**：

```json
{
  "success": true,
  "file": "downloads/comments/video_id_20251123_143000.json"
}
```

---

## 页面数据 API

### 1. 保存页面内容

**接口**：`POST /__wx_channels_api/save_page_content`

**功能**：保存页面完整 HTML 内容

**请求体**：

```json
{
  "url": "https://channels.weixin.qq.com/...",
  "html": "<html>...</html>",
  "timestamp": 1700730000000
}
```

**响应**：

```json
{
  "success": true
}
```

### 2. 保存搜索数据

**接口**：`POST /__wx_channels_api/save_search_data`

**功能**：保存搜索页面结构化数据

**请求体**：

```json
{
  "url": "https://channels.weixin.qq.com/...",
  "keyword": "搜索关键词",
  "profiles": [],
  "liveResults": [],
  "feedResults": [],
  "timestamp": 1700730000000
}
```

**响应**：

```json
{
  "success": true
}
```

---

## 内部 API

### 1. 获取 Profile 信息

**接口**：`POST /__wx_channels_api/profile`

**功能**：获取视频号 Profile 信息（内部使用）

### 2. 前端提示

**接口**：`POST /__wx_channels_api/tip`

**功能**：前端提示信息（内部使用）

### 3. 页面 URL

**接口**：`POST /__wx_channels_api/page_url`

**功能**：记录当前页面 URL（内部使用）

---

## 静态文件

### 1. Web 控制台

**接口**：`GET /console`

**功能**：访问 Web 控制台界面

### 2. JavaScript 库

- `GET /jszip.min.js` - JSZip 库
- `GET /FileSaver.min.js` - FileSaver 库

---

## 视频解密

### 解密原理

项目支持对加密视频进行前缀解密（XOR 解密）：

1. 视频文件的前 N 个字节使用密钥进行 XOR 加密
2. 下载时提供解密密钥，程序会自动解密
3. 解密后的视频可以正常播放

### 解密参数

**decryptorPrefix**：
- Base64 编码的解密密钥
- 例如：`AQIDBA==` 解码后为 `[1, 2, 3, 4]`

**prefixLen**：
- 需要解密的字节数
- 如果不指定，使用整个密钥长度
- 通常为 1024 或 2048

### 解密示例

```javascript
// JavaScript 示例
const videos = [
  {
    id: "encrypted_video",
    url: "https://example.com/encrypted.mp4",
    title: "加密视频",
    authorName: "作者",
    decryptorPrefix: btoa(String.fromCharCode(1, 2, 3, 4)),
    prefixLen: 1024
  }
];

fetch("http://127.0.0.1:2025/__wx_channels_api/batch_start", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ videos })
});
```

```python
# Python 示例
import requests
import base64

decryptor = bytes([1, 2, 3, 4])
decryptor_b64 = base64.b64encode(decryptor).decode()

videos = [{
    "id": "encrypted_video",
    "url": "https://example.com/encrypted.mp4",
    "title": "加密视频",
    "authorName": "作者",
    "decryptorPrefix": decryptor_b64,
    "prefixLen": 1024
}]

response = requests.post(
    "http://127.0.0.1:2025/__wx_channels_api/batch_start",
    json={"videos": videos}
)
print(response.json())
```

---

## 完整工作流程

### 1. 分片上传流程

```javascript
// 1. 初始化上传
const initResponse = await fetch("/__wx_channels_api/init_upload", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    filename: "video.mp4",
    totalSize: file.size,
    chunkSize: 2 * 1024 * 1024
  })
});
const { uploadId } = await initResponse.json();

// 2. 上传分片
for (let i = 0; i < totalChunks; i++) {
  const chunk = file.slice(i * chunkSize, (i + 1) * chunkSize);
  const reader = new FileReader();
  reader.onload = async (e) => {
    const base64 = btoa(String.fromCharCode(...new Uint8Array(e.target.result)));
    await fetch("/__wx_channels_api/upload_chunk", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        uploadId,
        chunkIndex: i,
        data: base64
      })
    });
  };
  reader.readAsArrayBuffer(chunk);
}

// 3. 完成上传
await fetch("/__wx_channels_api/complete_upload", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    uploadId,
    filename: "video.mp4",
    authorName: "作者名称",
    videoInfo: { id: "video_id", title: "视频标题", url: "..." }
  })
});
```

### 2. 批量下载流程

```javascript
// 1. 提交下载任务
const startResponse = await fetch("/__wx_channels_api/batch_start", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ videos: [...] })
});

// 2. 轮询进度
const checkProgress = setInterval(async () => {
  const progress = await fetch("/__wx_channels_api/batch_progress");
  const data = await progress.json();
  console.log("进度:", data);

  if (data.done + data.failed === data.total) {
    clearInterval(checkProgress);
    
    // 3. 导出失败清单
    if (data.failed > 0) {
      const failed = await fetch("/__wx_channels_api/batch_failed");
      console.log("失败清单:", await failed.json());
    }
  }
}, 2000);
```

---

## 配置选项

### 环境变量

```bash
# 代理端口
WX_CHANNEL_PORT=2025

# 下载目录
WX_CHANNEL_DOWNLOADS_DIR=downloads

# 安全配置
WX_CHANNEL_TOKEN=your_secret_token
WX_CHANNEL_ALLOWED_ORIGINS=https://example.com

# 并发配置
WX_CHANNEL_UPLOAD_CHUNK_CONCURRENCY=4
WX_CHANNEL_DOWNLOAD_CONCURRENCY=2
WX_CHANNEL_DOWNLOAD_RETRY_COUNT=3

# 日志配置
WX_CHANNEL_LOG_FILE=logs/wx_channel.log
WX_CHANNEL_LOG_MAX_MB=5
```

---

## 错误处理

### HTTP 状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 成功 |
| 401 | 未授权（需要 Token） |
| 500 | 服务器错误 |

### 错误响应

```json
{
  "success": false,
  "error": "错误信息"
}
```

### 常见错误

| 错误 | 原因 | 解决方案 |
|------|------|----------|
| unauthorized | 缺少或错误的 Token | 检查 X-Local-Auth 请求头 |
| http_status_404 | 视频地址无效 | 检查 URL 是否正确 |
| http_status_403 | 访问被拒绝 | 可能需要特殊的请求头 |
| file_exists | 文件已存在 | 使用 forceRedownload: true |

---

## 文件保存

### 目录结构

```
downloads/
├── download_records.csv          # 下载记录
├── batch_failed_*.json           # 失败清单
├── video_links_*.txt             # 导出的链接（TXT）
├── video_list_*.json             # 导出的列表（JSON）
├── video_list_*.md               # 导出的列表（Markdown）
├── comments/                     # 评论数据
│   └── video_id_*.json
├── page_snapshots/               # 页面快照
│   └── 2025-11-23/
└── <作者名>/                     # 按作者分类
    └── <文件名>.mp4
```

### 文件名处理

- 自动清理非法字符
- 文件名限制为 100 字符
- 文件夹名限制为 50 字符
- 自动添加 `.mp4` 扩展名
- 重名文件自动编号

---

## 注意事项

1. **并发限制**
   - 默认并发数为 2
   - 过高的并发可能导致网络问题

2. **重试机制**
   - 失败会自动重试 3 次
   - 可通过配置调整重试次数

3. **文件去重**
   - 相同 ID 的视频只会下载一次
   - 使用 `forceRedownload: true` 可强制重新下载

4. **解密性能**
   - 解密会略微降低下载速度
   - 只解密前缀部分，影响较小

5. **内存使用**
   - 使用流式下载，内存占用低
   - 大文件下载不会占用大量内存

---

## Web 控制台增强 API

以下 API 端点用于支持增强版 Web 控制台的功能。

### 浏览记录 API

#### 1. 获取浏览记录列表

**接口**：`GET /__wx_channels_api/browse`

**功能**：获取浏览记录列表（分页）

**查询参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | Number | 否 | 页码，默认 1 |
| pageSize | Number | 否 | 每页数量，默认 20 |
| search | String | 否 | 搜索关键词（标题或作者） |

**响应**：

```json
{
  "success": true,
  "data": [
    {
      "id": "video_id",
      "title": "视频标题",
      "author": "作者名称",
      "duration": 180,
      "size": 10485760,
      "coverUrl": "https://...",
      "videoUrl": "https://...",
      "browseTime": "2025-11-23T14:30:00Z",
      "likeCount": 100,
      "commentCount": 50,
      "shareCount": 20
    }
  ],
  "total": 100,
  "page": 1,
  "pageSize": 20
}
```

#### 2. 获取单条浏览记录

**接口**：`GET /__wx_channels_api/browse/:id`

**功能**：获取单条浏览记录详情

#### 3. 删除浏览记录

**接口**：`DELETE /__wx_channels_api/browse`

**功能**：批量删除浏览记录

**请求体**：

```json
{
  "ids": ["id1", "id2", "id3"]
}
```

#### 4. 清空浏览记录

**接口**：`DELETE /__wx_channels_api/browse/clear`

**功能**：清空所有浏览记录

#### 5. 按日期清理浏览记录

**接口**：`DELETE /__wx_channels_api/browse/cleanup`

**功能**：删除指定日期之前的浏览记录

**请求体**：

```json
{
  "beforeDate": "2025-11-01"
}
```

---

### 下载记录 API

#### 1. 获取下载记录列表

**接口**：`GET /__wx_channels_api/downloads`

**功能**：获取下载记录列表（分页、筛选）

**查询参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | Number | 否 | 页码，默认 1 |
| pageSize | Number | 否 | 每页数量，默认 20 |
| status | String | 否 | 状态筛选：completed, failed, in_progress |
| startDate | String | 否 | 开始日期 |
| endDate | String | 否 | 结束日期 |

**响应**：

```json
{
  "success": true,
  "data": [
    {
      "id": "record_id",
      "videoId": "video_id",
      "title": "视频标题",
      "author": "作者名称",
      "duration": 180,
      "fileSize": 10485760,
      "filePath": "downloads/作者/视频.mp4",
      "format": "mp4",
      "resolution": "1080p",
      "status": "completed",
      "downloadTime": "2025-11-23T14:30:00Z"
    }
  ],
  "total": 50,
  "page": 1,
  "pageSize": 20
}
```

#### 2. 获取单条下载记录

**接口**：`GET /__wx_channels_api/downloads/:id`

**功能**：获取单条下载记录详情

#### 3. 删除下载记录

**接口**：`DELETE /__wx_channels_api/downloads`

**功能**：批量删除下载记录

**请求体**：

```json
{
  "ids": ["id1", "id2", "id3"],
  "deleteFiles": false
}
```

#### 4. 清空下载记录

**接口**：`DELETE /__wx_channels_api/downloads/clear`

**功能**：清空所有下载记录

**请求体**：

```json
{
  "deleteFiles": false
}
```

#### 5. 按日期清理下载记录

**接口**：`DELETE /__wx_channels_api/downloads/cleanup`

**功能**：删除指定日期之前的下载记录

**请求体**：

```json
{
  "beforeDate": "2025-11-01",
  "deleteFiles": false
}
```

---

### 下载队列 API

#### 1. 获取下载队列

**接口**：`GET /__wx_channels_api/queue`

**功能**：获取当前下载队列

**响应**：

```json
{
  "success": true,
  "data": [
    {
      "id": "queue_item_id",
      "videoId": "video_id",
      "title": "视频标题",
      "author": "作者名称",
      "videoUrl": "https://...",
      "totalSize": 10485760,
      "downloadedSize": 5242880,
      "status": "downloading",
      "priority": 1,
      "addedTime": "2025-11-23T14:30:00Z",
      "speed": 1048576,
      "chunksTotal": 10,
      "chunksCompleted": 5
    }
  ]
}
```

#### 2. 添加到队列

**接口**：`POST /__wx_channels_api/queue`

**功能**：添加视频到下载队列

**请求体**：

```json
{
  "videos": [
    {
      "id": "video_id",
      "url": "https://...",
      "title": "视频标题",
      "authorName": "作者名称"
    }
  ]
}
```

#### 3. 暂停下载

**接口**：`PUT /__wx_channels_api/queue/:id/pause`

**功能**：暂停指定的下载任务

#### 4. 恢复下载

**接口**：`PUT /__wx_channels_api/queue/:id/resume`

**功能**：恢复指定的下载任务

#### 5. 从队列移除

**接口**：`DELETE /__wx_channels_api/queue/:id`

**功能**：从队列中移除指定任务

#### 6. 重新排序队列

**接口**：`PUT /__wx_channels_api/queue/reorder`

**功能**：重新排序下载队列

**请求体**：

```json
{
  "ids": ["id1", "id2", "id3"]
}
```

---

### 设置 API

#### 1. 获取设置

**接口**：`GET /__wx_channels_api/settings`

**功能**：获取当前设置

**响应**：

```json
{
  "success": true,
  "data": {
    "downloadDir": "downloads",
    "chunkSize": 10485760,
    "concurrentLimit": 3,
    "autoCleanupEnabled": false,
    "autoCleanupDays": 30,
    "maxRetries": 3
  }
}
```

#### 2. 更新设置

**接口**：`PUT /__wx_channels_api/settings`

**功能**：更新设置

**请求体**：

```json
{
  "downloadDir": "downloads",
  "chunkSize": 10485760,
  "concurrentLimit": 3,
  "autoCleanupEnabled": true,
  "autoCleanupDays": 30
}
```

---

### 统计 API

#### 1. 获取统计数据

**接口**：`GET /__wx_channels_api/stats`

**功能**：获取统计概览

**响应**：

```json
{
  "success": true,
  "data": {
    "totalBrowseCount": 1000,
    "totalDownloadCount": 500,
    "todayDownloadCount": 10,
    "storageUsed": 10737418240,
    "recentBrowse": [...],
    "recentDownload": [...]
  }
}
```

#### 2. 获取图表数据

**接口**：`GET /__wx_channels_api/stats/chart`

**功能**：获取过去7天的下载活动数据

**响应**：

```json
{
  "success": true,
  "data": {
    "labels": ["11-17", "11-18", "11-19", "11-20", "11-21", "11-22", "11-23"],
    "values": [5, 10, 8, 15, 12, 20, 18]
  }
}
```

---

### 导出 API

#### 1. 导出浏览记录

**接口**：`GET /__wx_channels_api/export/browse`

**功能**：导出浏览记录

**查询参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| format | String | 否 | 格式：json 或 csv，默认 json |
| ids | String | 否 | 逗号分隔的 ID 列表（选择性导出） |

#### 2. 导出下载记录

**接口**：`GET /__wx_channels_api/export/downloads`

**功能**：导出下载记录

**查询参数**：同上

---

### 搜索 API

#### 全局搜索

**接口**：`GET /__wx_channels_api/search`

**功能**：跨浏览记录和下载记录搜索

**查询参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| q | String | 是 | 搜索关键词 |

**响应**：

```json
{
  "success": true,
  "data": {
    "browseResults": [...],
    "browseCount": 10,
    "downloadResults": [...],
    "downloadCount": 5
  }
}
```

---

### 健康检查 API

#### 健康检查

**接口**：`GET /__wx_channels_api/health`

**功能**：检查服务状态

**响应**：

```json
{
  "success": true,
  "status": "healthy",
  "version": "1.0.0"
}
```

---

### WebSocket API

#### 实时更新

**接口**：`WS /__wx_channels_api/ws`

**功能**：WebSocket 连接，用于接收实时更新

**消息类型**：

1. **下载进度更新**

```json
{
  "type": "download_progress",
  "queueId": "queue_item_id",
  "downloaded": 5242880,
  "total": 10485760,
  "speed": 1048576,
  "status": "downloading"
}
```

2. **队列变化**

```json
{
  "type": "queue_change",
  "action": "add|remove|update|reorder",
  "item": {...},
  "queue": [...]
}
```

3. **统计更新**

```json
{
  "type": "stats_update",
  "stats": {...}
}
```

---

## 相关文档

- [批量下载使用指南](BATCH_DOWNLOAD_GUIDE.md) - 批量下载完整功能说明
- [Web 控制台](WEB_CONSOLE.md) - Web 控制台使用指南
- [配置概览](CONFIGURATION.md) - 配置选项说明
- [故障排除](TROUBLESHOOTING.md) - 常见问题解决方案

---

最后更新：2025-12-03
