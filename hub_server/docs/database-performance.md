# 数据库性能优化指南

## 当前状态
- 数据库大小：260MB
- 数据库类型：SQLite
- 记录数：约 500+ 浏览记录 + 90+ 下载记录

## 性能评估

### SQLite 性能特点
- **小型数据库（< 1GB）**：性能优秀 ✅
- **中型数据库（1-10GB）**：性能良好，需要优化
- **大型数据库（> 10GB）**：建议迁移到 PostgreSQL/MySQL

### 当前性能预期
260MB 对于 SQLite 来说是**小型数据库**，性能应该很好。但随着数据增长，需要注意以下几点：

## 已实施的优化

### 1. WAL 模式（Write-Ahead Logging）
```sql
PRAGMA journal_mode=WAL;
```
- ✅ 已启用
- 优点：读写不互相阻塞，提高并发性能
- 适用场景：多用户同时访问

### 2. 连接池限制
```go
sqlDB.SetMaxOpenConns(1)
```
- ✅ 已配置
- 原因：SQLite 不支持真正的并发写入
- 效果：避免 "database is locked" 错误

### 3. 索引配置
```go
gorm:"index:idx_browse_machine_updated"
gorm:"index:idx_download_machine_updated"
```
- ✅ 已创建复合索引
- 覆盖：machine_id + updated_at

## 建议的额外优化

### 1. 增加缓存大小
```sql
PRAGMA cache_size=-64000;  -- 64MB 缓存
```
**影响**：减少磁盘 I/O，加速查询

### 2. 内存映射
```sql
PRAGMA mmap_size=268435456;  -- 256MB 内存映射
```
**影响**：加速大文件读取

### 3. 创建额外索引
```sql
-- 按浏览时间查询（前端常用）
CREATE INDEX idx_browse_time ON hub_browse_history(machine_id, browse_time DESC);

-- 按下载时间查询
CREATE INDEX idx_download_time ON hub_download_records(machine_id, download_time DESC);
```

### 4. 定期维护
```sql
-- 更新统计信息（每周）
ANALYZE;

-- 清理碎片（每月）
VACUUM;
```

## 执行优化

### 方式1：运行优化脚本
```bash
cd hub_server
sqlite3 hub_server.db < scripts/optimize-database.sql
```

### 方式2：在代码中配置
修改 `hub_server/database/db.go`：
```go
// 增加缓存
sqlDB.Exec("PRAGMA cache_size=-64000;")
// 启用内存映射
sqlDB.Exec("PRAGMA mmap_size=268435456;")
```

## 性能监控

### 1. 查询性能分析
```bash
sqlite3 hub_server.db < scripts/analyze-performance.sql
```

### 2. 慢查询检测
在代码中启用 GORM 日志：
```go
DB, err = gorm.Open(sqlite.Open(path), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Info),
})
```

### 3. 监控指标
- 查询响应时间：< 100ms（正常）
- 数据库大小增长：< 10MB/天（正常）
- 索引命中率：> 90%（良好）

## 何时需要迁移到 PostgreSQL？

考虑迁移的信号：
1. 数据库大小 > 5GB
2. 并发用户 > 50
3. 查询响应时间 > 500ms
4. 频繁出现 "database is locked" 错误

## 数据归档策略

### 自动清理旧数据
```sql
-- 删除 6 个月前的浏览记录
DELETE FROM hub_browse_history 
WHERE browse_time < datetime('now', '-6 months');

-- 删除 1 年前的同步历史
DELETE FROM sync_history 
WHERE sync_time < datetime('now', '-1 year');
```

### 实施建议
- 保留最近 6 个月的浏览记录
- 保留最近 1 年的下载记录
- 保留最近 3 个月的同步历史
- 定期导出归档数据

## 性能基准测试

### 预期性能（260MB 数据库）
- 简单查询（按 ID）：< 1ms
- 分页查询（20 条）：< 10ms
- 复杂查询（JOIN）：< 50ms
- 全表扫描：< 500ms

### 测试方法
```bash
# 测试查询性能
time sqlite3 hub_server.db "SELECT COUNT(*) FROM hub_browse_history;"

# 测试索引效果
sqlite3 hub_server.db "EXPLAIN QUERY PLAN SELECT * FROM hub_browse_history WHERE machine_id='xxx' LIMIT 20;"
```

## 总结

当前 260MB 的数据库规模：
- ✅ 性能良好，无需担心
- ✅ 已有基本优化配置
- 📝 建议执行额外优化脚本
- 📊 建议定期监控性能
- 🗄️ 建议实施数据归档策略

预计可以支持到 **1-2GB** 数据量，之后再考虑迁移。
