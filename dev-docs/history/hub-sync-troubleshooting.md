# Hub 同步故障排查指南

## 快速诊断

### 1. 检查同步状态

从你的截图看到：
- ✅ 总设备: 1
- ✅ 同步中: 0
- ✅ 成功: 0
- ❌ 失败: 1

这表明有一个设备同步失败。

### 2. 查看失败原因

点击失败设备的"查看详情"按钮，查看错误信息。

## 常见问题和解决方案

### 问题 1: WebSocket 连接未建立

**症状**: 设备显示"失败"，错误信息包含 "connection" 相关字样

**原因**: 客户端未连接到 Hub Server

**解决方案**:

1. **检查客户端配置** (`config.yaml`):
```yaml
cloud_enabled: true
cloud_hub_url: "ws://your-hub-server:8080/ws/client"

hub_sync:
  enabled: true
  push_enabled: true
```

2. **检查客户端日志**:
```bash
# 查看日志文件
tail -f logs/wx_channel.log | grep -E "SyncPusher|云端连接"
```

应该看到：
```
✓ 已连接到云端 Hub
✓ Hub 同步推送器已启动 (间隔: 5m0s, 批量: 1000)
```

3. **检查网络连接**:
```bash
# 测试 Hub Server 是否可访问
curl http://your-hub-server:8080/health
```

### 问题 2: 同步推送器未启动

**症状**: 客户端已连接，但没有推送数据

**原因**: `hub_sync.enabled` 或 `hub_sync.push_enabled` 未启用

**解决方案**:

1. **检查配置**:
```yaml
hub_sync:
  enabled: true        # 必须为 true
  push_enabled: true   # 必须为 true
```

2. **重启客户端**:
```bash
# 停止客户端
Ctrl+C

# 重新启动
./wx_channel.exe
```

3. **查看启动日志**:
```
[SyncPusher] 启动同步推送器 (间隔: 5m0s)
```

### 问题 3: 数据库查询失败

**症状**: 推送失败，错误信息包含 "database" 或 "SQL"

**原因**: 数据库连接问题或数据损坏

**解决方案**:

1. **检查数据库文件**:
```bash
# 检查数据库是否存在
ls -lh downloads/records.db
```

2. **检查数据库完整性**:
```bash
sqlite3 downloads/records.db "PRAGMA integrity_check;"
```

3. **查看客户端日志**:
```bash
tail -f logs/wx_channel.log | grep -E "database|SQL"
```

### 问题 4: Hub Server 同步服务未启动

**症状**: 客户端推送成功，但 Hub Server 没有保存数据

**原因**: Hub Server 的同步服务未初始化

**解决方案**:

1. **检查 Hub Server 日志**:
```bash
# 查看启动日志
tail -f logs/hub_server.log | grep SyncService
```

应该看到：
```
[SyncService] Starting sync service...
```

2. **检查 Hub Server 配置** (`main.go`):
```go
services.InitSyncService(services.SyncConfig{
    Enabled:    true,  // 必须为 true
    Interval:   5 * time.Minute,
    MaxRetries: 3,
    Hub:        hub,
})
```

3. **重启 Hub Server**:
```bash
./hub_server.exe
```

### 问题 5: 消息解析失败

**症状**: Hub Server 日志显示 "解析同步数据失败" 或 "解析记录失败"

**原因**: 客户端和 Hub Server 的数据结构不匹配

**解决方案**:

1. **检查版本兼容性**:
   - 确保客户端和 Hub Server 使用相同版本的代码
   - 重新编译两端

2. **查看详细错误**:
```bash
# Hub Server 日志
tail -f logs/hub_server.log | grep "解析.*失败"
```

3. **检查数据格式**:
   - 客户端发送的数据格式应该与 `services.BrowseRecord` 和 `services.DownloadRecord` 匹配

### 问题 6: 权限问题

**症状**: 错误信息包含 "permission denied" 或 "access denied"

**原因**: 数据库文件或日志文件权限不足

**解决方案**:

1. **检查文件权限**:
```bash
# 客户端
ls -l downloads/records.db
ls -l logs/wx_channel.log

# Hub Server
ls -l hub_server.db
ls -l logs/hub_server.log
```

2. **修复权限**:
```bash
# Windows (以管理员身份运行)
icacls downloads /grant Users:F /T
icacls logs /grant Users:F /T
```

## 调试步骤

### 步骤 1: 检查客户端状态

```bash
# 1. 查看客户端是否运行
ps aux | grep wx_channel

# 2. 查看客户端日志
tail -n 100 logs/wx_channel.log

# 3. 查看最近的推送记录
tail -f logs/wx_channel.log | grep SyncPusher
```

### 步骤 2: 检查 Hub Server 状态

```bash
# 1. 查看 Hub Server 是否运行
ps aux | grep hub_server

# 2. 查看 Hub Server 日志
tail -n 100 logs/hub_server.log

# 3. 查看同步相关日志
tail -f logs/hub_server.log | grep -E "SyncService|同步"
```

### 步骤 3: 检查 WebSocket 连接

```bash
# 1. 查看 WebSocket 连接状态
# 客户端日志
grep "已连接到云端" logs/wx_channel.log

# Hub Server 日志
grep "WebSocket 连接建立" logs/hub_server.log

# 2. 查看心跳
# 客户端
grep "心跳" logs/wx_channel.log | tail -n 5

# Hub Server
grep "Heartbeat" logs/hub_server.log | tail -n 5
```

### 步骤 4: 检查数据库

```bash
# 1. 客户端数据库
sqlite3 downloads/records.db "SELECT COUNT(*) FROM browse_history;"
sqlite3 downloads/records.db "SELECT COUNT(*) FROM download_records;"

# 2. Hub Server 数据库
sqlite3 hub_server.db "SELECT * FROM sync_status;"
sqlite3 hub_server.db "SELECT COUNT(*) FROM hub_browse_history;"
sqlite3 hub_server.db "SELECT COUNT(*) FROM hub_download_records;"
```

### 步骤 5: 测试推送

```bash
# 1. 在客户端添加测试数据
# 浏览一些视频，触发浏览记录

# 2. 等待推送（默认 5 分钟）或重启客户端立即推送

# 3. 查看推送日志
tail -f logs/wx_channel.log | grep "推送.*条"

# 4. 查看 Hub Server 接收日志
tail -f logs/hub_server.log | grep "成功同步.*条"
```

## 日志关键字

### 客户端正常日志

```
✓ 已连接到云端 Hub
✓ Hub 同步推送器已启动 (间隔: 5m0s, 批量: 1000)
[SyncPusher] 启动同步推送器 (间隔: 5m0s)
[SyncPusher] 推送 10 条浏览记录
[SyncPusher] 推送 5 条下载记录
```

### Hub Server 正常日志

```
[SyncService] Starting sync service...
WebSocket 连接建立: ClientID=xxx, IP=xxx
成功同步 10 条 browse 记录 (客户端: xxx)
[SyncService] Saved 10 browse records for device: xxx
```

### 客户端错误日志

```
[SyncPusher] 推送浏览记录失败: connection closed
[SyncPusher] 推送下载记录失败: EOF
云端连接失败: dial tcp: connection refused
```

### Hub Server 错误日志

```
解析同步数据失败: ClientID=xxx, Error=invalid JSON
解析浏览记录失败: ClientID=xxx, Error=xxx
处理同步数据失败: ClientID=xxx, Error=xxx
同步服务不可用: ClientID=xxx
```

## 性能检查

### 检查推送频率

```bash
# 统计推送次数
grep "推送.*条" logs/wx_channel.log | wc -l

# 查看推送时间间隔
grep "推送.*条" logs/wx_channel.log | tail -n 10
```

### 检查数据量

```bash
# 客户端
sqlite3 downloads/records.db "
SELECT 
  (SELECT COUNT(*) FROM browse_history) as browse_count,
  (SELECT COUNT(*) FROM download_records) as download_count;
"

# Hub Server
sqlite3 hub_server.db "
SELECT 
  machine_id,
  browse_record_count,
  download_record_count,
  last_sync_status
FROM sync_status;
"
```

## 配置检查清单

### 客户端配置 (`config.yaml`)

- [ ] `cloud_enabled: true`
- [ ] `cloud_hub_url` 配置正确
- [ ] `hub_sync.enabled: true`
- [ ] `hub_sync.push_enabled: true`
- [ ] `hub_sync.push_interval` 合理（建议 5m-15m）
- [ ] `hub_sync.push_batch_size` 合理（建议 500-2000）

### Hub Server 配置 (`main.go`)

- [ ] `services.InitSyncService()` 已调用
- [ ] `Enabled: true`
- [ ] `Hub: hub` 已传递
- [ ] 数据库已初始化

## 恢复步骤

### 如果同步完全失败

1. **停止所有服务**:
```bash
# 停止客户端
Ctrl+C

# 停止 Hub Server
Ctrl+C
```

2. **检查配置**:
```bash
# 客户端
cat config.yaml | grep -A 5 "hub_sync"

# Hub Server
grep "InitSyncService" main.go
```

3. **清理并重启**:
```bash
# 客户端
rm -f logs/wx_channel.log
./wx_channel.exe

# Hub Server
rm -f logs/hub_server.log
./hub_server.exe
```

4. **验证连接**:
```bash
# 等待 30 秒，然后检查日志
tail -n 50 logs/wx_channel.log
tail -n 50 logs/hub_server.log
```

## 联系支持

如果以上步骤都无法解决问题，请提供以下信息：

1. **客户端日志** (`logs/wx_channel.log` 最后 100 行)
2. **Hub Server 日志** (`logs/hub_server.log` 最后 100 行)
3. **配置文件** (`config.yaml`)
4. **错误截图** (前端显示的错误信息)
5. **数据库状态**:
```bash
sqlite3 hub_server.db "SELECT * FROM sync_status;"
sqlite3 hub_server.db "SELECT * FROM sync_history ORDER BY sync_time DESC LIMIT 10;"
```

## 预防措施

### 1. 定期检查

```bash
# 每天检查一次同步状态
sqlite3 hub_server.db "
SELECT 
  machine_id,
  last_sync_status,
  last_sync_error,
  datetime(updated_at, 'localtime') as last_update
FROM sync_status
WHERE last_sync_status != 'success';
"
```

### 2. 监控日志

```bash
# 设置日志监控
tail -f logs/hub_server.log | grep -E "失败|错误|Error|Failed"
```

### 3. 定期备份

```bash
# 备份数据库
cp hub_server.db hub_server.db.backup.$(date +%Y%m%d)
```

### 4. 配置告警

如果有监控系统，可以配置以下告警：
- 同步失败次数 > 3
- 连续 1 小时没有新数据
- 错误日志出现频率 > 10/分钟
