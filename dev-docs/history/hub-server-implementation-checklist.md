# Hub Server 实现清单

## 客户端（wx_channel）- 已完成 ✅

- [x] 配置结构 (`internal/config/config.go`)
  - [x] `HubSyncConfig` 结构体
  - [x] 令牌、IP白名单、最大记录数配置

- [x] 同步 API 处理器 (`internal/handlers/sync_api.go`)
  - [x] `HandleGetBrowseRecords` - 获取浏览记录
  - [x] `HandleGetDownloadRecords` - 获取下载记录
  - [x] `HandleGetStats` - 获取统计信息
  - [x] 令牌验证中间件
  - [x] IP 白名单验证

- [x] 数据库增量查询方法
  - [x] `browse_repository.go`: `GetRecordsSince`, `GetLatestTimestamp`
  - [x] `download_repository.go`: `GetRecordsSince`, `GetLatestTimestamp`

- [x] 路由注册 (`internal/router/api_routes.go`)
  - [x] `/api/sync/browse`
  - [x] `/api/sync/download`
  - [x] `/api/sync/stats`

- [x] 配置示例 (`config.yaml.example`)
  - [x] `hub_sync` 配置段

- [x] 文档
  - [x] API 文档 (`docs/hub-sync-api.md`)

## Hub Server 端 - 待实现 ⏳

### 1. 数据模型 (`hub_server/models/`)

- [ ] `hub_browse_history.go`
  ```go
  type HubBrowseHistory struct {
      ID            string    `json:"id" gorm:"primaryKey"`
      MachineID     string    `json:"machine_id" gorm:"index"`
      Title         string    `json:"title"`
      Author        string    `json:"author"`
      AuthorID      string    `json:"author_id"`
      Duration      int       `json:"duration"`
      Size          int64     `json:"size"`
      Resolution    string    `json:"resolution"`
      CoverURL      string    `json:"cover_url"`
      VideoURL      string    `json:"video_url"`
      DecryptKey    string    `json:"decrypt_key"`
      BrowseTime    time.Time `json:"browse_time"`
      LikeCount     int       `json:"like_count"`
      CommentCount  int       `json:"comment_count"`
      FavCount      int       `json:"fav_count"`
      ForwardCount  int       `json:"forward_count"`
      PageURL       string    `json:"page_url"`
      SourceCreatedAt time.Time `json:"source_created_at"`
      SourceUpdatedAt time.Time `json:"source_updated_at"`
      SyncedAt      time.Time `json:"synced_at"`
      CreatedAt     time.Time `json:"created_at"`
      UpdatedAt     time.Time `json:"updated_at"`
  }
  ```

- [ ] `hub_download_record.go`
  ```go
  type HubDownloadRecord struct {
      ID            string    `json:"id" gorm:"primaryKey"`
      MachineID     string    `json:"machine_id" gorm:"index"`
      VideoID       string    `json:"video_id"`
      Title         string    `json:"title"`
      Author        string    `json:"author"`
      CoverURL      string    `json:"cover_url"`
      Duration      int       `json:"duration"`
      FileSize      int64     `json:"file_size"`
      FilePath      string    `json:"file_path"`
      Format        string    `json:"format"`
      Resolution    string    `json:"resolution"`
      Status        string    `json:"status"`
      DownloadTime  time.Time `json:"download_time"`
      ErrorMessage  string    `json:"error_message"`
      LikeCount     int       `json:"like_count"`
      CommentCount  int       `json:"comment_count"`
      ForwardCount  int       `json:"forward_count"`
      FavCount      int       `json:"fav_count"`
      SourceCreatedAt time.Time `json:"source_created_at"`
      SourceUpdatedAt time.Time `json:"source_updated_at"`
      SyncedAt      time.Time `json:"synced_at"`
      CreatedAt     time.Time `json:"created_at"`
      UpdatedAt     time.Time `json:"updated_at"`
  }
  ```

- [ ] `sync_status.go`
  ```go
  type SyncStatus struct {
      ID                  uint      `json:"id" gorm:"primaryKey"`
      MachineID           string    `json:"machine_id" gorm:"uniqueIndex"`
      LastBrowseSyncTime  time.Time `json:"last_browse_sync_time"`
      LastDownloadSyncTime time.Time `json:"last_download_sync_time"`
      BrowseRecordCount   int64     `json:"browse_record_count"`
      DownloadRecordCount int64     `json:"download_record_count"`
      LastSyncStatus      string    `json:"last_sync_status"` // success, failed, in_progress
      LastSyncError       string    `json:"last_sync_error"`
      CreatedAt           time.Time `json:"created_at"`
      UpdatedAt           time.Time `json:"updated_at"`
  }
  ```

### 2. 同步服务 (`hub_server/services/sync_service.go`)

- [ ] 基础结构
  ```go
  type SyncService struct {
      db          *gorm.DB
      httpClient  *http.Client
      syncToken   string
      syncInterval time.Duration
      maxRetries  int
  }
  ```

- [ ] 核心方法
  - [ ] `Start()` - 启动定时同步任务
  - [ ] `Stop()` - 停止同步服务
  - [ ] `SyncDevice(machineID string)` - 同步单个设备
  - [ ] `SyncAllDevices()` - 同步所有在线设备
  - [ ] `syncBrowseHistory(device *Device)` - 同步浏览记录
  - [ ] `syncDownloadRecords(device *Device)` - 同步下载记录
  - [ ] `updateSyncStatus(machineID string, status *SyncStatus)` - 更新同步状态

- [ ] 增量同步逻辑
  - [ ] 获取上次同步时间戳
  - [ ] 调用客户端 API 拉取新数据
  - [ ] 处理分页（`has_more=true`）
  - [ ] 保存到 Hub 数据库
  - [ ] 更新同步状态

- [ ] 错误处理
  - [ ] 网络错误重试（指数退避）
  - [ ] 设备离线处理
  - [ ] 数据冲突解决（使用 `updated_at` 判断）

### 3. API 控制器 (`hub_server/controllers/sync_controller.go`)

- [ ] `GetSyncStatus(c *gin.Context)` - 获取所有设备同步状态
- [ ] `GetDeviceSyncStatus(c *gin.Context)` - 获取单个设备同步状态
- [ ] `TriggerSync(c *gin.Context)` - 手动触发同步
- [ ] `GetSyncHistory(c *gin.Context)` - 获取同步历史记录

### 4. 路由注册 (`hub_server/routes/routes.go`)

- [ ] 添加同步管理路由
  ```go
  syncGroup := api.Group("/sync")
  {
      syncGroup.GET("/status", syncController.GetSyncStatus)
      syncGroup.GET("/status/:machine_id", syncController.GetDeviceSyncStatus)
      syncGroup.POST("/trigger", syncController.TriggerSync)
      syncGroup.GET("/history", syncController.GetSyncHistory)
  }
  ```

### 5. 前端界面 (`hub_server/web/src/`)

- [ ] 同步状态页面 (`views/Sync.vue`)
  - [ ] 设备列表及同步状态
  - [ ] 最后同步时间
  - [ ] 记录数量统计
  - [ ] 同步进度显示

- [ ] 同步控制组件
  - [ ] 手动触发同步按钮
  - [ ] 自动同步开关
  - [ ] 同步间隔设置

- [ ] 同步日志查看
  - [ ] 同步历史记录
  - [ ] 错误日志
  - [ ] 成功/失败统计

- [ ] 数据展示
  - [ ] 浏览记录聚合视图
  - [ ] 下载记录聚合视图
  - [ ] 跨设备数据统计

### 6. 配置 (`hub_server/config/config.yaml`)

- [ ] 添加同步配置
  ```yaml
  sync:
    enabled: true
    interval: 5m  # 同步间隔
    token: "your-sync-token"  # 与客户端配置一致
    max_retries: 3
    timeout: 30s
    batch_size: 1000
  ```

### 7. 数据库迁移

- [ ] 创建迁移文件
  - [ ] `hub_browse_history` 表
  - [ ] `hub_download_record` 表
  - [ ] `sync_status` 表

- [ ] 索引优化
  - [ ] `machine_id` 索引
  - [ ] `synced_at` 索引
  - [ ] 复合索引：`(machine_id, source_updated_at)`

### 8. 主程序集成 (`hub_server/main.go`)

- [ ] 初始化同步服务
  ```go
  syncService := services.NewSyncService(db, config.Sync)
  go syncService.Start()
  defer syncService.Stop()
  ```

### 9. 测试

- [ ] 单元测试
  - [ ] 同步服务测试
  - [ ] API 控制器测试
  - [ ] 数据模型测试

- [ ] 集成测试
  - [ ] 端到端同步流程测试
  - [ ] 错误恢复测试
  - [ ] 并发同步测试

### 10. 文档

- [ ] Hub Server 部署文档
- [ ] 同步配置说明
- [ ] 故障排查指南
- [ ] API 使用示例

## 开发顺序建议

1. **阶段 1: 数据层** (1-2天)
   - 创建数据模型
   - 数据库迁移
   - 基础 CRUD 操作

2. **阶段 2: 同步服务** (2-3天)
   - 实现同步服务核心逻辑
   - 增量同步算法
   - 错误处理和重试

3. **阶段 3: API 层** (1天)
   - 实现 API 控制器
   - 路由注册
   - 接口测试

4. **阶段 4: 前端界面** (2-3天)
   - 同步状态页面
   - 数据展示组件
   - 交互功能

5. **阶段 5: 测试和优化** (1-2天)
   - 单元测试
   - 集成测试
   - 性能优化

## 预计工作量

- **总计**: 7-11 天
- **核心功能**: 5-7 天
- **测试和优化**: 2-4 天

## 注意事项

1. **数据一致性**: 使用 `source_updated_at` 判断数据新旧，避免覆盖更新的数据
2. **性能优化**: 批量插入数据，使用事务减少数据库压力
3. **资源控制**: 限制并发同步设备数量，避免过载
4. **监控告警**: 添加同步失败告警，及时发现问题
5. **数据隐私**: 确保同步过程中数据安全，使用 HTTPS
