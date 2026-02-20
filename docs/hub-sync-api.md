# Hub 同步 API 文档

## 概述

Hub 同步 API 允许 Hub Server 主动拉取客户端的浏览记录和下载记录数据，实现集中式数据管理。

## 架构设计

### 拉取模式（Pull Model）

- **Hub Server**: 主动发起请求，定时拉取所有在线设备的数据
- **客户端（wx_channel）**: 提供只读 API 接口，响应 Hub Server 的数据请求
- **优势**: 
  - 客户端无需维护同步状态
  - Hub Server 完全控制同步频率和策略
  - 降低客户端资源消耗

## API 端点

### 1. 获取浏览记录

**端点**: `GET /api/sync/browse`

**查询参数**:
- `since` (可选): ISO 8601 时间戳，获取此时间之后的记录
- `limit` (可选): 返回记录数量限制，默认 100，最大 1000

**请求头**:
- `X-Sync-Token` (可选): 同步令牌，如果配置中设置了 token

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "records": [
      {
        "id": "video123",
        "title": "视频标题",
        "author": "作者名",
        "author_id": "author123",
        "duration": 120,
        "size": 10485760,
        "resolution": "1080p",
        "cover_url": "https://...",
        "video_url": "https://...",
        "decrypt_key": "key123",
        "browse_time": "2026-02-20T10:00:00Z",
        "like_count": 100,
        "comment_count": 50,
        "fav_count": 20,
        "forward_count": 10,
        "page_url": "https://...",
        "created_at": "2026-02-20T10:00:00Z",
        "updated_at": "2026-02-20T10:00:00Z"
      }
    ],
    "count": 1,
    "has_more": false
  }
}
```

### 2. 获取下载记录

**端点**: `GET /api/sync/download`

**查询参数**:
- `since` (可选): ISO 8601 时间戳，获取此时间之后的记录
- `limit` (可选): 返回记录数量限制，默认 100，最大 1000

**请求头**:
- `X-Sync-Token` (可选): 同步令牌

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "records": [
      {
        "id": "download123",
        "video_id": "video123",
        "title": "视频标题",
        "author": "作者名",
        "cover_url": "https://...",
        "duration": 120,
        "file_size": 10485760,
        "file_path": "/path/to/video.mp4",
        "format": "mp4",
        "resolution": "1080p",
        "status": "completed",
        "download_time": "2026-02-20T10:00:00Z",
        "error_message": "",
        "like_count": 100,
        "comment_count": 50,
        "forward_count": 10,
        "fav_count": 20,
        "created_at": "2026-02-20T10:00:00Z",
        "updated_at": "2026-02-20T10:00:00Z"
      }
    ],
    "count": 1,
    "has_more": false
  }
}
```

### 3. 获取统计信息

**端点**: `GET /api/sync/stats`

**请求头**:
- `X-Sync-Token` (可选): 同步令牌

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "browse": {
      "total": 1000,
      "latest_timestamp": "2026-02-20T10:00:00Z"
    },
    "download": {
      "total": 500,
      "latest_timestamp": "2026-02-20T10:00:00Z"
    }
  }
}
```

## 配置

在 `config.yaml` 中添加以下配置：

```yaml
hub_sync:
  # 是否启用同步 API
  enabled: true
  
  # 同步令牌（Hub Server 请求时需要提供）
  # 留空则不验证令牌
  token: "your-secret-token"
  
  # IP 白名单（允许访问同步 API 的 IP 地址列表）
  # 留空则允许所有 IP
  allowed_ips:
    - "127.0.0.1"
    - "::1"
    - "192.168.1.100"  # Hub Server IP
  
  # 单次请求最大返回记录数
  max_records: 1000
```

## 安全性

### 1. 令牌验证

如果配置了 `hub_sync.token`，所有同步 API 请求必须在请求头中包含 `X-Sync-Token`。

### 2. IP 白名单

如果配置了 `hub_sync.allowed_ips`，只有白名单中的 IP 地址才能访问同步 API。

### 3. 只读访问

同步 API 只提供数据读取功能，不支持任何写入或修改操作。

## 增量同步策略

### 客户端实现

1. **GetRecordsSince**: 根据 `updated_at` 字段查询指定时间之后的记录
2. **GetLatestTimestamp**: 获取数据库中最新记录的时间戳
3. **排序**: 按 `updated_at` 升序返回，确保增量同步的顺序性

### Hub Server 实现建议

1. **初始同步**: 
   - 首次同步时不传 `since` 参数，获取所有历史数据
   - 分批拉取，每次 `limit=1000`，直到 `has_more=false`

2. **增量同步**:
   - 记录上次同步的最新时间戳
   - 定时任务（如每 5 分钟）使用 `since` 参数拉取新数据
   - 处理 `has_more=true` 的情况，继续拉取剩余数据

3. **错误处理**:
   - 网络错误：重试机制，指数退避
   - 数据冲突：使用 `updated_at` 判断数据新旧
   - 设备离线：跳过，等待下次在线时同步

## 数据库设计

### 浏览记录表 (browse_history)

关键字段：
- `id`: 主键，视频 ID
- `updated_at`: 更新时间，用于增量同步
- 其他字段见 API 响应

### 下载记录表 (download_records)

关键字段：
- `id`: 主键，下载记录 ID
- `video_id`: 视频 ID
- `updated_at`: 更新时间，用于增量同步
- 其他字段见 API 响应

## 性能优化

1. **索引优化**:
   - `updated_at` 字段建立索引，加速增量查询
   - 复合索引：`(updated_at, id)` 提升排序性能

2. **分页查询**:
   - 使用 `LIMIT` 限制单次返回数据量
   - 避免一次性加载大量数据

3. **资源控制**:
   - `max_records` 配置限制最大返回数量
   - 防止恶意请求消耗过多资源

## 测试

### 手动测试

```bash
# 获取浏览记录
curl -H "X-Sync-Token: your-token" \
  "http://localhost:8080/api/sync/browse?limit=10"

# 增量获取
curl -H "X-Sync-Token: your-token" \
  "http://localhost:8080/api/sync/browse?since=2026-02-20T10:00:00Z&limit=10"

# 获取下载记录
curl -H "X-Sync-Token: your-token" \
  "http://localhost:8080/api/sync/download?limit=10"

# 获取统计信息
curl -H "X-Sync-Token: your-token" \
  "http://localhost:8080/api/sync/stats"
```

## 下一步开发

### Hub Server 端

1. **同步服务** (`services/sync_service.go`):
   - 定时任务调度器
   - 设备管理（在线状态、最后同步时间）
   - 增量拉取逻辑
   - 错误重试机制

2. **数据模型** (`models/`):
   - `hub_browse_history.go`: Hub 端浏览记录表
   - `hub_download_record.go`: Hub 端下载记录表
   - `sync_status.go`: 同步状态记录

3. **API 控制器** (`controllers/sync.go`):
   - 手动触发同步
   - 查看同步状态
   - 同步历史记录

4. **前端界面**:
   - 设备列表及同步状态
   - 同步日志查看
   - 手动触发同步按钮
   - 数据统计展示

## 故障排查

### 常见问题

1. **401 Unauthorized**:
   - 检查 `X-Sync-Token` 是否正确
   - 检查配置文件中的 `hub_sync.token`

2. **403 Forbidden**:
   - 检查请求 IP 是否在白名单中
   - 检查配置文件中的 `hub_sync.allowed_ips`

3. **空数据返回**:
   - 检查数据库中是否有数据
   - 检查 `since` 参数是否过新

4. **性能问题**:
   - 减小 `limit` 参数
   - 检查数据库索引
   - 检查网络延迟

## 版本历史

- **v1.0** (2026-02-20): 初始版本
  - 实现基础同步 API
  - 支持增量查询
  - 令牌和 IP 白名单验证
