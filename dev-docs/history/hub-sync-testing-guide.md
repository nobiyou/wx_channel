# Hub 同步功能测试指南

## 编译状态 ✅

- ✅ 客户端（wx_channel）编译成功
- ✅ Hub Server 编译成功
- ✅ 前端构建就绪

## 快速测试步骤

### 1. 准备环境

#### 客户端配置
编辑 `wx_channel/config.yaml`:
```yaml
# 启用Hub同步API
hub_sync:
  enabled: true
  token: "test-sync-token-123"
  allowed_ips:
    - "127.0.0.1"
    - "::1"
  max_records: 1000
```

#### Hub Server 配置
编辑 `hub_server/main.go` 中的同步配置（已默认配置）:
```go
services.InitSyncService(services.SyncConfig{
    Enabled:    true,
    Interval:   5 * time.Minute,
    Token:      "test-sync-token-123",  // 与客户端一致
    MaxRetries: 3,
    Timeout:    30 * time.Second,
    BatchSize:  1000,
})
```

### 2. 启动服务

#### 启动客户端
```bash
cd wx_channel
./wx_channel_test.exe
```

客户端应该在 `http://localhost:2025` 启动

#### 启动 Hub Server
```bash
cd hub_server
./hub_server_test.exe
```

Hub Server 应该在 `http://localhost:8080` 启动

### 3. 测试同步 API

#### 测试客户端 API

**获取浏览记录**:
```bash
curl -H "X-Sync-Token: test-sync-token-123" \
  "http://localhost:2025/api/sync/browse?limit=10"
```

**获取下载记录**:
```bash
curl -H "X-Sync-Token: test-sync-token-123" \
  "http://localhost:2025/api/sync/download?limit=10"
```

**获取统计信息**:
```bash
curl -H "X-Sync-Token: test-sync-token-123" \
  "http://localhost:2025/api/sync/stats"
```

#### 测试 Hub Server API

首先需要登录获取 token:
```bash
# 注册用户
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'

# 登录
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'
```

使用返回的 token 访问同步 API:
```bash
# 获取同步状态
curl -H "Authorization: Bearer YOUR_TOKEN" \
  "http://localhost:8080/api/sync/status"

# 触发同步
curl -X POST -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  "http://localhost:8080/api/sync/trigger" \
  -d '{"machine_id":"device-001"}'
```

### 4. 测试前端界面

1. 访问 `http://localhost:8080`
2. 使用测试账号登录
3. 点击侧边栏的"数据同步"菜单
4. 查看同步状态
5. 点击"同步所有设备"按钮
6. 查看同步详情和历史

### 5. 验证数据同步

#### 检查客户端数据
```bash
# 查看客户端数据库
sqlite3 wx_channel.db

# 查询浏览记录
SELECT COUNT(*) FROM browse_history;

# 查询下载记录
SELECT COUNT(*) FROM download_records;
```

#### 检查 Hub Server 数据
```bash
# 查看 Hub 数据库
sqlite3 hub_server/hub_server.db

# 查询同步的浏览记录
SELECT COUNT(*) FROM hub_browse_history;

# 查询同步的下载记录
SELECT COUNT(*) FROM hub_download_records;

# 查询同步状态
SELECT * FROM sync_status;

# 查询同步历史
SELECT * FROM sync_history ORDER BY sync_time DESC LIMIT 10;
```

## 测试场景

### 场景 1: 初始同步
1. 确保客户端有一些浏览和下载记录
2. 启动 Hub Server
3. 等待自动同步（5 分钟）或手动触发
4. 验证数据已同步到 Hub

### 场景 2: 增量同步
1. 在客户端添加新的浏览/下载记录
2. 等待下一次同步
3. 验证只同步了新记录

### 场景 3: 多设备同步
1. 启动多个客户端实例（不同端口）
2. 每个客户端配置不同的 machine_id
3. 触发"同步所有设备"
4. 验证所有设备数据都已同步

### 场景 4: 错误处理
1. 停止客户端
2. 触发同步
3. 验证错误被正确记录
4. 重启客户端
5. 验证同步恢复正常

### 场景 5: 令牌验证
1. 使用错误的令牌访问客户端 API
2. 验证返回 401 错误
3. 使用正确的令牌
4. 验证访问成功

### 场景 6: IP 白名单
1. 从非白名单 IP 访问
2. 验证返回 403 错误
3. 从白名单 IP 访问
4. 验证访问成功

## 性能测试

### 大数据量测试
```sql
-- 在客户端插入测试数据
INSERT INTO browse_history (id, title, author, browse_time, created_at, updated_at)
SELECT 
  'test-' || seq,
  'Test Video ' || seq,
  'Test Author',
  datetime('now'),
  datetime('now'),
  datetime('now')
FROM (
  WITH RECURSIVE cnt(x) AS (
    SELECT 1
    UNION ALL
    SELECT x+1 FROM cnt
    LIMIT 10000
  )
  SELECT x as seq FROM cnt
);
```

测试同步 10000 条记录的性能。

### 并发测试
使用工具（如 Apache Bench）测试并发同步请求:
```bash
ab -n 100 -c 10 -H "X-Sync-Token: test-sync-token-123" \
  "http://localhost:2025/api/sync/browse?limit=100"
```

## 日志监控

### 客户端日志
```bash
tail -f logs/wx_channel.log | grep -i sync
```

### Hub Server 日志
```bash
# 查看同步服务日志
tail -f logs/hub_server.log | grep SyncService
```

## 故障排查

### 问题 1: 同步失败 - 401 错误
**原因**: 令牌不匹配  
**解决**: 检查客户端和 Hub Server 的 token 配置是否一致

### 问题 2: 同步失败 - 403 错误
**原因**: IP 不在白名单  
**解决**: 将 Hub Server IP 添加到客户端的 allowed_ips

### 问题 3: 同步失败 - 连接超时
**原因**: 网络问题或客户端未启动  
**解决**: 检查客户端是否在线，检查网络连接

### 问题 4: 数据未同步
**原因**: 同步服务未启动或配置错误  
**解决**: 检查 Hub Server 日志，确认同步服务已启动

### 问题 5: 重复数据
**原因**: 不应该发生（使用了 FirstOrCreate）  
**解决**: 检查数据库约束，查看日志

## 清理测试数据

### 清理客户端数据
```sql
DELETE FROM browse_history WHERE id LIKE 'test-%';
DELETE FROM download_records WHERE id LIKE 'test-%';
```

### 清理 Hub Server 数据
```sql
DELETE FROM hub_browse_history;
DELETE FROM hub_download_records;
DELETE FROM sync_status;
DELETE FROM sync_history;
```

## 测试检查清单

- [ ] 客户端编译成功
- [ ] Hub Server 编译成功
- [ ] 客户端 API 可访问
- [ ] Hub Server API 可访问
- [ ] 前端页面可访问
- [ ] 令牌验证工作正常
- [ ] IP 白名单工作正常
- [ ] 初始同步成功
- [ ] 增量同步成功
- [ ] 多设备同步成功
- [ ] 错误处理正确
- [ ] 同步历史记录正确
- [ ] 前端状态显示正确
- [ ] 手动触发同步工作
- [ ] 自动同步工作

## 下一步

测试通过后：
1. 合并到 main 分支
2. 创建 release tag
3. 更新用户文档
4. 部署到生产环境

## 注意事项

1. 测试时使用测试数据库，不要使用生产数据
2. 测试完成后清理测试数据
3. 记录测试结果和发现的问题
4. 性能测试应在接近生产环境的配置下进行
