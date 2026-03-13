# 数据库管理功能测试指南

## 测试环境准备

### 1. 启动 Hub Server

```bash
cd wx_channel/hub_server
./hub_server_db.exe
```

### 2. 登录管理员账号

访问 `http://localhost:8080` 并使用管理员账号登录。

## 测试用例

### 测试 1: 查看数据库统计

**步骤**:
1. 进入"管理后台"页面
2. 点击"数据库管理"标签
3. 等待统计信息加载

**预期结果**:
- ✅ 显示数据库大小（MB）
- ✅ 显示总记录数
- ✅ 显示数据表数量
- ✅ 显示各表的详细统计（记录数、占用空间）
- ✅ 数据正确无误

**验证 API**:
```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/admin/database/stats
```

预期响应:
```json
{
  "code": 0,
  "data": {
    "size_mb": "260.50",
    "total_records": 15234,
    "tables": [
      {
        "table_name": "浏览记录",
        "record_count": 12450,
        "oldest_record": "2025-08-20 10:30:00",
        "newest_record": "2026-02-20 21:07:27"
      },
      {
        "table_name": "下载记录",
        "record_count": 2784,
        "oldest_record": "2025-09-15 14:20:00",
        "newest_record": "2026-02-20 20:45:00"
      },
      {
        "table_name": "同步历史",
        "record_count": 0,
        "oldest_record": "0001-01-01 00:00:00",
        "newest_record": "0001-01-01 00:00:00"
      }
    ]
  }
}
```

---

### 测试 2: 数据库优化

**步骤**:
1. 在"数据库管理"标签中
2. 找到"数据库优化"区域
3. 点击"立即优化"按钮
4. 在确认对话框中点击"确认"
5. 等待优化完成

**预期结果**:
- ✅ 显示确认对话框，说明优化操作
- ✅ 显示加载状态（按钮显示 loading）
- ✅ 优化完成后显示成功提示
- ✅ 统计信息自动刷新
- ✅ 数据库大小可能减小（如果有碎片）

**验证 API**:
```bash
curl -X POST -H "Authorization: Bearer <token>" http://localhost:8080/api/admin/database/optimize
```

预期响应:
```json
{
  "code": 0,
  "message": "优化完成",
  "data": [
    {
      "operation": "设置缓存大小",
      "duration": "0.01s",
      "success": true
    },
    {
      "operation": "启用内存映射",
      "duration": "0.00s",
      "success": true
    },
    {
      "operation": "更新统计信息",
      "duration": "2.34s",
      "success": true
    },
    {
      "operation": "清理碎片",
      "duration": "15.67s",
      "success": true
    }
  ]
}
```

**性能验证**:
- 优化前后查询速度对比
- 数据库文件大小对比

---

### 测试 3: 数据归档（谨慎测试）

**警告**: 此操作会永久删除数据，建议先备份数据库！

**备份数据库**:
```bash
# 停止 Hub Server
# 复制数据库文件
cp hub_server.db hub_server.db.backup
```

**步骤**:
1. 在"数据库管理"标签中
2. 找到"数据归档"区域
3. 配置保留时间：
   - 浏览记录保留: 6 个月
   - 下载记录保留: 1 年
   - 同步历史保留: 3 个月
4. 点击"执行归档"按钮
5. 在确认对话框中查看将要删除的数据范围
6. 点击"确认删除"
7. 等待归档完成

**预期结果**:
- ✅ 显示确认对话框，说明删除范围
- ✅ 显示加载状态（按钮显示 loading）
- ✅ 归档完成后显示删除的记录数
- ✅ 统计信息自动刷新
- ✅ 总记录数减少
- ✅ 数据库大小减小

**验证 API**:
```bash
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"browse_months":6,"download_years":1,"history_months":3}' \
  http://localhost:8080/api/admin/database/archive
```

预期响应:
```json
{
  "code": 0,
  "message": "归档完成",
  "data": {
    "deleted_browse": 3450,
    "deleted_download": 892,
    "deleted_history": 156,
    "total_deleted": 4498
  }
}
```

**数据验证**:
```sql
-- 检查浏览记录最早时间
SELECT MIN(browse_time) FROM hub_browse_history;
-- 应该是 6 个月前左右

-- 检查下载记录最早时间
SELECT MIN(download_time) FROM hub_download_records;
-- 应该是 1 年前左右

-- 检查同步历史最早时间
SELECT MIN(sync_time) FROM sync_history;
-- 应该是 3 个月前左右
```

---

### 测试 4: 权限验证

**步骤**:
1. 使用普通用户账号登录
2. 尝试访问数据库管理 API

**预期结果**:
- ✅ 返回 403 Forbidden 错误
- ✅ 提示需要管理员权限

**验证 API**:
```bash
# 使用普通用户 token
curl -H "Authorization: Bearer <user_token>" http://localhost:8080/api/admin/database/stats
```

预期响应:
```json
{
  "code": 403,
  "message": "需要管理员权限"
}
```

---

### 测试 5: UI 响应式测试

**步骤**:
1. 在不同屏幕尺寸下测试界面
   - 桌面（1920x1080）
   - 平板（768x1024）
   - 手机（375x667）

**预期结果**:
- ✅ 统计卡片自适应布局
- ✅ 表格可横向滚动
- ✅ 按钮和输入框大小合适
- ✅ 对话框居中显示
- ✅ 文字清晰可读

---

### 测试 6: 错误处理

**测试场景 1: 网络错误**
- 断开网络连接
- 尝试加载统计信息
- 预期: 显示错误提示

**测试场景 2: 数据库锁定**
- 在另一个进程中锁定数据库
- 尝试执行优化
- 预期: 显示错误提示

**测试场景 3: 磁盘空间不足**
- 模拟磁盘空间不足
- 尝试执行 VACUUM
- 预期: 显示错误提示

---

## 性能测试

### 优化效果测试

**测试步骤**:
1. 记录优化前的查询时间
2. 执行数据库优化
3. 记录优化后的查询时间
4. 计算性能提升百分比

**测试查询**:
```sql
-- 查询 1: 浏览记录统计
SELECT device_id, COUNT(*) as count 
FROM hub_browse_history 
GROUP BY device_id 
ORDER BY count DESC;

-- 查询 2: 下载记录统计
SELECT device_id, COUNT(*) as count 
FROM hub_download_records 
GROUP BY device_id 
ORDER BY count DESC;

-- 查询 3: 复杂查询
SELECT 
  h.device_id,
  COUNT(DISTINCT h.id) as browse_count,
  COUNT(DISTINCT d.id) as download_count
FROM hub_browse_history h
LEFT JOIN hub_download_records d ON h.device_id = d.device_id
GROUP BY h.device_id;
```

**预期结果**:
- 查询时间减少 20-50%
- 数据库文件大小减小（如果有碎片）

---

### 归档效果测试

**测试步骤**:
1. 记录归档前的数据库大小
2. 执行数据归档
3. 记录归档后的数据库大小
4. 计算空间节省百分比

**预期结果**:
- 数据库大小显著减小
- 查询速度提升
- 记录数减少符合预期

---

## 回归测试

执行归档和优化后，验证其他功能是否正常：

- ✅ 同步功能正常
- ✅ 浏览记录查询正常
- ✅ 下载记录查询正常
- ✅ 设备管理正常
- ✅ 用户管理正常

---

## 测试报告模板

```
测试日期: 2026-02-21
测试人员: [姓名]
Hub Server 版本: hub_server_db.exe

测试结果:
- [ ] 测试 1: 查看数据库统计 - 通过/失败
- [ ] 测试 2: 数据库优化 - 通过/失败
- [ ] 测试 3: 数据归档 - 通过/失败
- [ ] 测试 4: 权限验证 - 通过/失败
- [ ] 测试 5: UI 响应式 - 通过/失败
- [ ] 测试 6: 错误处理 - 通过/失败

性能测试:
- 优化前查询时间: [X]ms
- 优化后查询时间: [Y]ms
- 性能提升: [Z]%
- 归档前数据库大小: [A]MB
- 归档后数据库大小: [B]MB
- 空间节省: [C]%

问题记录:
1. [问题描述]
2. [问题描述]

建议:
1. [改进建议]
2. [改进建议]
```

---

## 常见问题

### Q1: 优化需要多长时间？
A: 取决于数据库大小，通常 1-5 分钟。260MB 数据库约需 2-3 分钟。

### Q2: 优化会影响正在运行的服务吗？
A: 会有短暂的性能下降，建议在低峰期执行。

### Q3: 归档操作可以撤销吗？
A: 不可以，数据会被永久删除。请务必先备份。

### Q4: 多久执行一次优化？
A: 建议每月一次，或在大量删除数据后执行。

### Q5: 如何恢复误删的数据？
A: 从备份文件恢复：
```bash
# 停止 Hub Server
# 恢复备份
cp hub_server.db.backup hub_server.db
# 重启 Hub Server
```
