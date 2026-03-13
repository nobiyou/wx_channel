# Hub 同步 WebSocket 推送测试指南

## 测试环境准备

### 1. Hub Server 配置

确保 Hub Server 的同步服务已启用：

```go
// hub_server/main.go
services.InitSyncService(services.SyncConfig{
    Enabled:    true,
    Interval:   5 * time.Minute,
    Token:      "test-sync-token",
    MaxRetries: 3,
    Timeout:    30 * time.Second,
    BatchSize:  1000,
    Hub:        hub, // 传递 WebSocket Hub
})
```

### 2. 客户端配置

编辑 `config.yaml`：

```yaml
# 启用云端连接
cloud_enabled: true
cloud_hub_url: "ws://localhost:8080/ws/client"
cloud_secret: ""

# 启用 Hub 同步推送
hub_sync:
  enabled: true
  token: "test-sync-token"
  allowed_ips:
    - "127.0.0.1"
    - "::1"
  
  # WebSocket 推送配置
  push_enabled: true
  push_interval: 1m      # 测试时使用较短间隔
  push_batch_size: 100   # 测试时使用较小批量
```

## 测试步骤

### 步骤 1: 启动 Hub Server

```bash
cd hub_server
./hub_server_test.exe
```

预期输出：
```
[SyncService] Starting sync service...
WebSocket Hub started
Server started on :8080
```

### 步骤 2: 启动客户端

```bash
cd wx_channel
./wx_channel_test.exe
```

预期输出：
```
正在启动云端连接器 (ID: xxx, URL: ws://localhost:8080/ws/client)
✓ Hub 同步推送器已启动 (间隔: 1m0s, 批量: 100)
[SyncPusher] 启动同步推送器 (间隔: 1m0s)
✓ 已连接到云端 Hub
```

### 步骤 3: 验证 WebSocket 连接

在 Hub Server 日志中查看：
```
WebSocket 连接建立: ClientID=xxx, IP=127.0.0.1
连接统计: ClientID=xxx, Uptime=xxx, Ping=xxx, Pong=xxx
```

### 步骤 4: 触发数据同步

#### 方法 A: 等待自动推送
等待 1 分钟（push_interval），观察日志：

客户端日志：
```
[SyncPusher] 推送 X 条浏览记录
[SyncPusher] 推送 X 条下载记录
```

Hub Server 日志：
```
成功同步 X 条 browse 记录 (客户端: xxx)
成功同步 X 条 download 记录 (客户端: xxx)
[SyncService] Saved X browse records for device: xxx
[SyncService] Saved X download records for device: xxx
```

#### 方法 B: 手动触发（添加测试数据）
在客户端添加一些测试数据，然后等待下一次推送。

### 步骤 5: 验证数据库

查询 Hub Server 数据库：

```bash
cd hub_server
sqlite3 hub_server.db
```

```sql
-- 查看同步状态
SELECT * FROM sync_status;

-- 查看浏览记录
SELECT COUNT(*) FROM hub_browse_history;
SELECT * FROM hub_browse_history ORDER BY synced_at DESC LIMIT 10;

-- 查看下载记录
SELECT COUNT(*) FROM hub_download_records;
SELECT * FROM hub_download_records ORDER BY synced_at DESC LIMIT 10;

-- 查看同步历史
SELECT * FROM sync_history ORDER BY sync_time DESC LIMIT 10;
```

预期结果：
- `sync_status` 表中有客户端的同步状态记录
- `hub_browse_history` 和 `hub_download_records` 表中有同步的数据
- `sync_history` 表中有成功的同步记录

## 测试场景

### 场景 1: 正常同步（NAT 环境）

**目的**: 验证 WebSocket 推送在 NAT 环境下正常工作

**步骤**:
1. 客户端在内网，无公网 IP
2. 启用 `push_enabled: true`
3. 观察客户端主动推送数据
4. 验证 Hub Server 接收并保存数据

**预期**: 数据成功同步，无需 Hub 主动访问客户端

### 场景 2: 回退到 HTTP 拉取

**目的**: 验证当 WebSocket 推送失败时，系统回退到 HTTP 拉取

**步骤**:
1. 禁用客户端推送：`push_enabled: false`
2. 配置客户端 HTTP API 可访问
3. 观察 Hub Server 通过 HTTP 拉取数据

**预期**: Hub Server 通过 HTTP API 成功拉取数据

### 场景 3: 大批量数据同步

**目的**: 验证大量数据的同步性能

**步骤**:
1. 在客户端生成大量测试数据（如 10000 条记录）
2. 设置 `push_batch_size: 1000`
3. 观察分批推送过程

**预期**: 
- 数据分批推送（每批 1000 条）
- 所有数据最终都被同步
- 无内存溢出或性能问题

### 场景 4: 网络中断恢复

**目的**: 验证网络中断后的数据同步

**步骤**:
1. 正常运行一段时间
2. 断开网络连接（关闭 Hub Server）
3. 在客户端生成新数据
4. 恢复网络连接（重启 Hub Server）
5. 观察数据同步

**预期**: 
- 网络恢复后，客户端重新连接
- 断网期间的数据被同步
- 无数据丢失

### 场景 5: 增量同步

**目的**: 验证只同步新增数据，不重复同步

**步骤**:
1. 第一次同步 100 条记录
2. 添加 50 条新记录
3. 第二次同步

**预期**: 
- 第二次只同步 50 条新记录
- 不重复同步已同步的 100 条记录

## 性能测试

### 测试指标

1. **同步延迟**: 从数据生成到同步完成的时间
2. **吞吐量**: 每秒同步的记录数
3. **内存占用**: 同步过程中的内存使用
4. **CPU 占用**: 同步过程中的 CPU 使用

### 测试工具

使用 Prometheus 监控（如果启用）：

```bash
# 查看客户端指标
curl http://localhost:9090/metrics | grep sync

# 查看 Hub Server 指标
curl http://localhost:8080/metrics | grep sync
```

### 性能基准

- **小批量** (100 条): < 1 秒
- **中批量** (1000 条): < 5 秒
- **大批量** (10000 条): < 30 秒
- **内存增长**: < 50MB
- **CPU 占用**: < 10%

## 故障排查

### 问题 1: 客户端无法连接到 Hub

**症状**: 
```
云端连接失败 (重试 1): dial tcp: connection refused
```

**解决**:
1. 检查 Hub Server 是否运行
2. 检查 `cloud_hub_url` 配置是否正确
3. 检查防火墙设置

### 问题 2: 同步数据未保存

**症状**: 
```
成功同步 X 条记录
```
但数据库中无数据

**解决**:
1. 检查 Hub Server 数据库连接
2. 查看 Hub Server 错误日志
3. 验证数据格式是否正确

### 问题 3: 推送频率过高

**症状**: 
```
[SyncPusher] 推送 0 条浏览记录
[SyncPusher] 推送 0 条下载记录
```
频繁出现

**解决**:
1. 增加 `push_interval`（如 5m 或 10m）
2. 只在有新数据时推送（已实现）

### 问题 4: 内存持续增长

**症状**: 客户端或 Hub Server 内存持续增长

**解决**:
1. 检查是否有内存泄漏
2. 减小 `push_batch_size`
3. 增加 `push_interval`
4. 检查数据库连接是否正确关闭

## 日志分析

### 客户端关键日志

```
✓ Hub 同步推送器已启动 (间隔: 5m0s, 批量: 1000)
[SyncPusher] 启动同步推送器 (间隔: 5m0s)
[SyncPusher] 推送 X 条浏览记录
[SyncPusher] 推送 X 条下载记录
```

### Hub Server 关键日志

```
[SyncService] Starting sync service...
成功同步 X 条 browse 记录 (客户端: xxx)
[SyncService] Saved X browse records for device: xxx
[SyncService] Sync completed for device: xxx (status: success)
```

### 错误日志

```
[SyncPusher] 推送浏览记录失败: connection closed
解析同步数据失败: ClientID=xxx, Error=xxx
处理同步数据失败: ClientID=xxx, Error=xxx
```

## 监控和告警

### 推荐监控指标

1. **同步成功率**: 成功同步次数 / 总同步次数
2. **同步延迟**: 平均同步完成时间
3. **数据量**: 每次同步的记录数
4. **错误率**: 同步失败次数 / 总同步次数

### 告警规则

1. 同步成功率 < 95%
2. 同步延迟 > 60 秒
3. 连续 3 次同步失败
4. 内存占用 > 500MB

## 最佳实践

1. **生产环境配置**:
   - `push_interval: 5m` - 平衡实时性和性能
   - `push_batch_size: 1000` - 避免单次数据过大
   - 启用 `token` 验证
   - 配置 `allowed_ips` 白名单

2. **开发环境配置**:
   - `push_interval: 1m` - 快速测试
   - `push_batch_size: 100` - 便于观察
   - 可以不设置 `token`

3. **性能优化**:
   - 根据数据量调整 `push_interval`
   - 根据网络情况调整 `push_batch_size`
   - 启用数据压缩（`compression_enabled: true`）

4. **安全建议**:
   - 使用强密码作为 `token`
   - 限制 `allowed_ips` 只包含 Hub Server IP
   - 使用 HTTPS/WSS 加密传输（生产环境）

## 相关文档

- [Hub 同步 API 文档](hub-sync-api.md)
- [WebSocket 推送实现](hub-sync-websocket-push.md)
- [NAT 穿透解决方案](hub-sync-nat-solution.md)
