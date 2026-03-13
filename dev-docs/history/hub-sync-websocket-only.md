# Hub 同步 - WebSocket 推送模式（简化版）

## 概述

本系统采用 **WebSocket 推送模式** 实现客户端与 Hub Server 之间的数据同步。客户端通过已建立的 WebSocket 连接主动推送数据，无需公网 IP 或端口转发，完美适配内网环境。

## 架构设计

```
┌─────────────┐                    ┌──────────────┐
│   Client    │ ─── WebSocket ───> │  Hub Server  │
│  (内网)     │ <── Commands ───── │              │
│             │ ─── Push Data ───> │              │
└─────────────┘                    └──────────────┘
```

### 核心特点

1. **单向推送**: 客户端主动推送，Hub Server 被动接收
2. **无需公网 IP**: 客户端只需能访问 Hub Server
3. **连接复用**: 使用已有的 WebSocket 连接
4. **增量同步**: 只同步新增数据
5. **批量处理**: 可配置批量大小

## 实现组件

### 客户端

#### 1. 同步推送器 (`internal/cloud/sync_pusher.go`)

负责定时从数据库获取增量数据并推送：

```go
type SyncPusher struct {
    connector         *Connector
    browseRepo        *database.BrowseHistoryRepository
    downloadRepo      *database.DownloadRecordRepository
    syncInterval      time.Duration
    lastBrowseSync    time.Time
    lastDownloadSync  time.Time
    batchSize         int
}
```

核心方法：
- `Start()`: 启动定时推送任务
- `pushBrowseHistory()`: 推送浏览记录
- `pushDownloadRecords()`: 推送下载记录

#### 2. 消息类型 (`internal/cloud/models.go`)

```go
const MsgTypeSyncData MessageType = "sync_data"

type SyncDataPayload struct {
    SyncType string          // "browse" or "download"
    Records  json.RawMessage // 记录数组
    Count    int             // 记录数量
    HasMore  bool            // 是否还有更多数据
}
```

#### 3. 配置 (`internal/config/config.go`)

```go
type HubSyncConfig struct {
    Enabled       bool          // 是否启用同步
    PushEnabled   bool          // 是否启用推送
    PushInterval  time.Duration // 推送间隔
    PushBatchSize int           // 批量大小
}
```

### Hub Server

#### 1. WebSocket 消息处理 (`hub_server/ws/client.go`)

在 `handleMessage()` 中添加 `MsgTypeSyncData` 处理：

```go
case MsgTypeSyncData:
    var payload SyncDataPayload
    json.Unmarshal(msg.Payload, &payload)
    
    // 解析记录
    if payload.SyncType == "browse" {
        var browseRecords []services.BrowseRecord
        json.Unmarshal(payload.Records, &browseRecords)
        records = browseRecords
    } else if payload.SyncType == "download" {
        var downloadRecords []services.DownloadRecord
        json.Unmarshal(payload.Records, &downloadRecords)
        records = downloadRecords
    }
    
    // 处理同步数据
    syncService.HandleSyncDataFromClient(c.ID, payload.SyncType, records)
```

#### 2. 同步服务 (`hub_server/services/sync_service.go`)

简化后的同步服务，只处理客户端推送的数据：

```go
func (s *SyncService) HandleSyncDataFromClient(
    machineID string, 
    syncType string, 
    records interface{},
) error {
    // 获取同步状态
    syncStatus, _ := s.getOrCreateSyncStatus(machineID)
    
    // 根据类型保存记录
    switch syncType {
    case "browse":
        return s.saveBrowseRecords(machineID, records, syncStatus)
    case "download":
        return s.saveDownloadRecords(machineID, records, syncStatus)
    }
}
```

## 数据流程

### 推送流程

```
1. 客户端定时器触发 (每 5 分钟)
   ↓
2. SyncPusher 从数据库获取增量数据
   ↓
3. 序列化为 SyncDataPayload
   ↓
4. 通过 WebSocket 发送到 Hub Server
   ↓
5. Hub Server 的 WebSocket Client 接收消息
   ↓
6. 解析并调用 SyncService.HandleSyncDataFromClient()
   ↓
7. 保存到 Hub 数据库
   ↓
8. 更新同步状态和历史记录
```

### 增量同步机制

客户端记录最后同步时间，只查询新增数据：

```go
// 获取自上次同步以来的新记录
records, err := sp.browseRepo.GetRecordsSince(
    sp.lastBrowseSync,  // 上次同步时间
    sp.batchSize,       // 批量大小
)

// 推送成功后更新时间
sp.lastBrowseSync = time.Now()
```

## 配置示例

### 客户端配置 (`config.yaml`)

```yaml
# 启用云端连接
cloud_enabled: true
cloud_hub_url: "ws://hub.example.com/ws/client"

# Hub 同步配置
hub_sync:
  enabled: true           # 启用同步
  push_enabled: true      # 启用推送
  push_interval: 5m       # 每 5 分钟推送一次
  push_batch_size: 1000   # 每次最多 1000 条
```

### Hub Server 配置 (`main.go`)

```go
services.InitSyncService(services.SyncConfig{
    Enabled:    true,
    Interval:   5 * time.Minute,
    MaxRetries: 3,
    Hub:        hub, // 传递 WebSocket Hub
})
```

## 性能优化

### 1. 增量同步

只同步新增数据，避免重复传输：
- 记录最后同步时间
- 使用 `GetRecordsSince()` 查询增量数据

### 2. 批量处理

使用 `push_batch_size` 限制单次传输量：
- 避免单次消息过大
- 防止内存占用过高
- 减少网络传输超时

### 3. 异步处理

推送在独立 goroutine 中执行：
- 不阻塞主流程
- 提高并发性能

### 4. 连接复用

复用云端连接器的 WebSocket 连接：
- 避免重复建立连接
- 降低资源消耗

## 监控和日志

### 客户端日志

```
✓ Hub 同步推送器已启动 (间隔: 5m0s, 批量: 1000)
[SyncPusher] 启动同步推送器 (间隔: 5m0s)
[SyncPusher] 推送 10 条浏览记录
[SyncPusher] 推送 5 条下载记录
```

### Hub Server 日志

```
成功同步 10 条 browse 记录 (客户端: xxx)
[SyncService] Saved 10 browse records for device: xxx
成功同步 5 条 download 记录 (客户端: xxx)
[SyncService] Saved 5 download records for device: xxx
```

### 错误日志

```
[SyncPusher] 推送浏览记录失败: connection closed
解析同步数据失败: ClientID=xxx, Error=invalid JSON
处理同步数据失败: ClientID=xxx, Error=database error
```

## 故障处理

### 推送失败

**现象**: `[SyncPusher] 推送失败: connection closed`

**原因**: WebSocket 连接断开

**处理**: 
- 系统会在下次定时任务时自动重试
- 使用增量同步，不会丢失数据
- 连接恢复后继续推送

### 数据重复

**现象**: 数据库中出现重复记录

**原因**: 推送成功但确认失败

**处理**:
- 数据库使用唯一约束 (`id + machine_id`)
- `FirstOrCreate` 自动去重
- 不会产生重复记录

### 内存增长

**现象**: 客户端或 Hub Server 内存持续增长

**原因**: 批量大小过大或推送频率过高

**处理**:
- 减小 `push_batch_size`（如 500）
- 增加 `push_interval`（如 10m）
- 检查数据库连接是否正确关闭

## 最佳实践

### 生产环境

```yaml
hub_sync:
  enabled: true
  push_enabled: true
  push_interval: 5m        # 平衡实时性和性能
  push_batch_size: 1000    # 避免单次数据过大
```

### 开发环境

```yaml
hub_sync:
  enabled: true
  push_enabled: true
  push_interval: 1m        # 快速测试
  push_batch_size: 100     # 便于观察
```

### 大数据量环境

```yaml
hub_sync:
  enabled: true
  push_enabled: true
  push_interval: 10m       # 降低推送频率
  push_batch_size: 2000    # 增加批量大小
```

## 安全建议

1. **使用 WSS 加密**: `wss://hub.example.com/ws/client`
2. **配置密钥**: 设置 `cloud_secret`
3. **定期更新**: 定期更换密钥
4. **监控日志**: 及时发现异常推送

## 优势总结

### vs HTTP 拉取模式

| 特性 | WebSocket 推送 | HTTP 拉取 |
|------|---------------|-----------|
| 公网 IP | ❌ 不需要 | ✅ 需要 |
| 端口转发 | ❌ 不需要 | ✅ 需要 |
| 实时性 | ✅ 好 | ⚠️ 一般 |
| 配置复杂度 | ✅ 简单 | ⚠️ 复杂 |
| 安全性 | ✅ 高 | ⚠️ 需要额外配置 |
| 适用场景 | ✅ 内网环境 | ⚠️ 公网环境 |

### 核心优势

1. **零配置**: 无需配置公网 IP、端口转发、防火墙规则
2. **高安全**: 所有通信通过加密的 WebSocket，不暴露 HTTP 端口
3. **低延迟**: 利用已有连接，无需建立新连接
4. **高可靠**: 增量同步 + 自动重试，确保数据不丢失
5. **易维护**: 配置简单，日志清晰，易于排查问题

## 相关文档

- [快速入门指南](hub-sync-quick-start.md)
- [测试指南](hub-sync-websocket-push-testing.md)
- [完整实现文档](hub-sync-websocket-push-implementation.md)
