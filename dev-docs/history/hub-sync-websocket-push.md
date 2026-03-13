# Hub 同步 WebSocket 推送方案

## 问题场景

客户端在内网环境，没有公网 IP，也无法配置端口转发，Hub Server 无法主动访问客户端的 HTTP API。

## 解决方案：WebSocket 反向推送

利用客户端已经建立的 WebSocket 连接，让客户端主动推送同步数据给 Hub Server。

```
┌─────────────┐                    ┌──────────────┐
│   Client    │ ─── WebSocket ───> │  Hub Server  │
│  (内网)     │ <── Commands ───── │              │
│             │ ─── Push Data ───> │              │
└─────────────┘                    └──────────────┘
```

## 实现步骤

### 阶段 1: Hub Server 端（已完成 ✅）

#### 1.1 添加消息类型

已在 `hub_server/ws/types.go` 添加：

```go
const (
    MsgTypeSyncData MessageType = "sync_data" // 客户端主动推送同步数据
)

type SyncDataPayload struct {
    SyncType string          `json:"sync_type"` // "browse" or "download"
    Records  json.RawMessage `json:"records"`
    Count    int             `json:"count"`
    HasMore  bool            `json:"has_more"`
}
```

#### 1.2 修改同步服务

已在 `hub_server/services/sync_service.go` 添加：

- `syncViaWebSocket()` - 尝试通过 WebSocket 同步
- `HandleSyncDataFromClient()` - 处理客户端推送的数据
- `saveBrowseRecords()` - 保存浏览记录
- `saveDownloadRecords()` - 保存下载记录

#### 1.3 优先级策略

同步服务现在按以下顺序尝试：
1. **WebSocket 推送** - 优先，适用于 NAT 后的客户端
2. **HTTP 拉取** - 回退方案，适用于可直接访问的客户端

### 阶段 2: 客户端实现（待开发 ⏳）

#### 2.1 添加同步推送模块

在客户端创建 `internal/cloud/sync_pusher.go`：

```go
package cloud

import (
    "encoding/json"
    "log"
    "time"
    "wx_channel/internal/database"
)

type SyncPusher struct {
    client        *CloudClient
    browseRepo    *database.BrowseHistoryRepository
    downloadRepo  *database.DownloadRecordRepository
    syncInterval  time.Duration
    lastBrowseSync time.Time
    lastDownloadSync time.Time
    running       bool
    stopChan      chan struct{}
}

func NewSyncPusher(client *CloudClient) *SyncPusher {
    return &SyncPusher{
        client:       client,
        browseRepo:   database.NewBrowseHistoryRepository(),
        downloadRepo: database.NewDownloadRecordRepository(),
        syncInterval: 5 * time.Minute,
        stopChan:     make(chan struct{}),
    }
}

func (sp *SyncPusher) Start() {
    if sp.running {
        return
    }
    sp.running = true
    
    log.Println("[SyncPusher] Starting sync pusher...")
    
    // 立即执行一次同步
    go sp.pushSyncData()
    
    // 定时推送
    ticker := time.NewTicker(sp.syncInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            go sp.pushSyncData()
        case <-sp.stopChan:
            log.Println("[SyncPusher] Stopping sync pusher...")
            sp.running = false
            return
        }
    }
}

func (sp *SyncPusher) Stop() {
    if !sp.running {
        return
    }
    close(sp.stopChan)
}

func (sp *SyncPusher) pushSyncData() {
    // 推送浏览记录
    if err := sp.pushBrowseHistory(); err != nil {
        log.Printf("[SyncPusher] Failed to push browse history: %v", err)
    }
    
    // 推送下载记录
    if err := sp.pushDownloadRecords(); err != nil {
        log.Printf("[SyncPusher] Failed to push download records: %v", err)
    }
}

func (sp *SyncPusher) pushBrowseHistory() error {
    // 获取增量数据
    records, err := sp.browseRepo.GetRecordsSince(sp.lastBrowseSync, 1000)
    if err != nil {
        return err
    }
    
    if len(records) == 0 {
        return nil
    }
    
    // 构建消息
    recordsJSON, err := json.Marshal(records)
    if err != nil {
        return err
    }
    
    payload := SyncDataPayload{
        SyncType: "browse",
        Records:  recordsJSON,
        Count:    len(records),
        HasMore:  len(records) >= 1000,
    }
    
    payloadJSON, err := json.Marshal(payload)
    if err != nil {
        return err
    }
    
    // 发送消息
    msg := CloudMessage{
        Type:    MsgTypeSyncData,
        Payload: payloadJSON,
    }
    
    if err := sp.client.SendMessage(msg); err != nil {
        return err
    }
    
    // 更新最后同步时间
    sp.lastBrowseSync = time.Now()
    log.Printf("[SyncPusher] Pushed %d browse records", len(records))
    
    return nil
}

func (sp *SyncPusher) pushDownloadRecords() error {
    // 获取增量数据
    records, err := sp.downloadRepo.GetRecordsSince(sp.lastDownloadSync, 1000)
    if err != nil {
        return err
    }
    
    if len(records) == 0 {
        return nil
    }
    
    // 构建消息
    recordsJSON, err := json.Marshal(records)
    if err != nil {
        return err
    }
    
    payload := SyncDataPayload{
        SyncType: "download",
        Records:  recordsJSON,
        Count:    len(records),
        HasMore:  len(records) >= 1000,
    }
    
    payloadJSON, err := json.Marshal(payload)
    if err != nil {
        return err
    }
    
    // 发送消息
    msg := CloudMessage{
        Type:    MsgTypeSyncData,
        Payload: payloadJSON,
    }
    
    if err := sp.client.SendMessage(msg); err != nil {
        return err
    }
    
    // 更新最后同步时间
    sp.lastDownloadSync = time.Now()
    log.Printf("[SyncPusher] Pushed %d download records", len(records))
    
    return nil
}
```

#### 2.2 集成到云端客户端

在 `internal/cloud/client.go` 中添加：

```go
type CloudClient struct {
    // ... 现有字段
    syncPusher *SyncPusher
}

func (c *CloudClient) Start() {
    // ... 现有代码
    
    // 启动同步推送器
    if c.config.CloudEnabled && c.config.HubSync.Enabled {
        c.syncPusher = NewSyncPusher(c)
        go c.syncPusher.Start()
    }
}

func (c *CloudClient) Stop() {
    // ... 现有代码
    
    // 停止同步推送器
    if c.syncPusher != nil {
        c.syncPusher.Stop()
    }
}
```

#### 2.3 添加配置

在 `config.yaml` 中添加：

```yaml
cloud_enabled: true

hub_sync:
  enabled: true
  push_enabled: true      # 启用主动推送
  push_interval: 5m       # 推送间隔
  token: "your-token"
```

### 阶段 3: Hub Server 消息处理（待完善 ⏳）

在 `hub_server/ws/client.go` 的 `ReadPump` 方法中添加处理：

```go
case MsgTypeSyncData:
    var payload SyncDataPayload
    if err := json.Unmarshal(msg.Payload, &payload); err != nil {
        log.Printf("Failed to unmarshal sync data: %v", err)
        continue
    }
    
    // 获取同步服务
    syncService := services.GetSyncService()
    if syncService == nil {
        log.Printf("Sync service not available")
        continue
    }
    
    // 解析记录
    var records interface{}
    if payload.SyncType == "browse" {
        var browseRecords []services.BrowseRecord
        if err := json.Unmarshal(payload.Records, &browseRecords); err != nil {
            log.Printf("Failed to unmarshal browse records: %v", err)
            continue
        }
        records = browseRecords
    } else if payload.SyncType == "download" {
        var downloadRecords []services.DownloadRecord
        if err := json.Unmarshal(payload.Records, &downloadRecords); err != nil {
            log.Printf("Failed to unmarshal download records: %v", err)
            continue
        }
        records = downloadRecords
    }
    
    // 处理同步数据
    if err := syncService.HandleSyncDataFromClient(c.ID, payload.SyncType, records); err != nil {
        log.Printf("Failed to handle sync data: %v", err)
    } else {
        log.Printf("Successfully synced %d %s records from client %s", 
            payload.Count, payload.SyncType, c.ID)
    }
```

## 优势

### 1. 无需公网 IP
- 客户端在任何网络环境都能工作
- 不需要端口转发或 NAT 穿透

### 2. 实时性更好
- 客户端可以在数据变化时立即推送
- 不需要等待 Hub 的定时拉取

### 3. 降低延迟
- 利用已有的 WebSocket 连接
- 无需建立新的 HTTP 连接

### 4. 更安全
- 所有通信都通过加密的 WebSocket
- 不需要暴露 HTTP API 端口

## 配置示例

### 客户端配置

```yaml
# 启用云端连接
cloud_enabled: true
cloud_hub_url: "ws://hub.example.com/ws/client"

# 启用同步推送
hub_sync:
  enabled: true
  push_enabled: true
  push_interval: 5m
  token: "your-sync-token"
```

### Hub Server 配置

```go
// main.go
services.InitSyncService(services.SyncConfig{
    Enabled:    true,
    Interval:   5 * time.Minute,
    Token:      "your-sync-token",
    MaxRetries: 3,
    Timeout:    30 * time.Second,
    BatchSize:  1000,
    Hub:        hub, // 传递 WebSocket Hub
})
```

## 测试步骤

### 1. 启动 Hub Server
```bash
cd hub_server
./hub_server
```

### 2. 启动客户端（启用云端和同步推送）
```bash
cd wx_channel
./wx_channel
```

### 3. 观察日志
```bash
# Hub Server 日志
tail -f logs/hub_server.log | grep -E "SyncService|SyncPusher"

# 客户端日志
tail -f logs/wx_channel.log | grep SyncPusher
```

### 4. 验证数据
```sql
-- 查看 Hub 数据库
sqlite3 hub_server/hub_server.db

SELECT COUNT(*) FROM hub_browse_history;
SELECT COUNT(*) FROM hub_download_records;
SELECT * FROM sync_status;
```

## 回退机制

如果 WebSocket 推送失败，系统会自动回退到 HTTP 拉取模式：

1. Hub 尝试通过 WebSocket 请求同步
2. 如果失败，检查是否配置了 `sync_api_url`
3. 如果有，使用自定义 URL 进行 HTTP 拉取
4. 否则，使用 `IP:Port` 进行 HTTP 拉取

## 开发优先级

- [x] Hub Server 端基础架构
- [x] 客户端同步推送器实现
- [x] Hub Server 消息处理完善
- [x] 配置选项添加
- [ ] 测试和调试
- [ ] 文档完善

## 相关文档

- [Hub 同步 API 文档](hub-sync-api.md)
- [NAT 穿透解决方案](hub-sync-nat-solution.md)
- [测试指南](hub-sync-testing-guide.md)
