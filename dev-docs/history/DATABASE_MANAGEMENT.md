# 数据库管理功能

## 概述

Hub Server 现已集成完整的数据库管理功能，允许管理员通过 Web 界面轻松监控、优化和维护数据库。

## 快速开始

### 1. 启动服务

```bash
cd wx_channel/hub_server
./hub_server_db.exe
```

### 2. 访问管理界面

1. 打开浏览器访问 `http://localhost:8080`
2. 使用管理员账号登录
3. 进入"管理后台"页面
4. 点击"数据库管理"标签

### 3. 查看统计信息

进入数据库管理标签后，系统会自动显示：
- 数据库总大小
- 总记录数
- 各数据表的详细统计

## 主要功能

### 📊 数据库统计

实时监控数据库状态：
- 数据库文件大小（MB）
- 总记录数
- 各表记录数和占用空间
- 最早和最新记录时间

### ⚡ 数据库优化

一键优化数据库性能：
- 执行 ANALYZE（更新查询统计）
- 执行 VACUUM（回收空间）
- 重建索引
- 提升查询性能 20-50%

**建议频率**: 每月一次

### 🗑️ 数据归档

自动清理旧数据：
- 可配置浏览记录保留时间（1-24 个月）
- 可配置下载记录保留时间（1-5 年）
- 可配置同步历史保留时间（1-12 个月）
- 自动执行优化操作

**警告**: 归档操作不可恢复，请先备份！

## API 接口

### 获取统计信息

```http
GET /api/admin/database/stats
Authorization: Bearer <token>
```

### 优化数据库

```http
POST /api/admin/database/optimize
Authorization: Bearer <token>
```

### 归档旧数据

```http
POST /api/admin/database/archive
Authorization: Bearer <token>
Content-Type: application/json

{
  "browse_months": 6,
  "download_years": 1,
  "history_months": 3
}
```

## 性能参考

### 数据库大小建议

| 大小 | 性能 | 建议 |
|------|------|------|
| < 500 MB | 优秀 | 无需特别优化 |
| 500 MB - 1 GB | 良好 | 每月优化一次 |
| 1 GB - 2 GB | 可接受 | 每周优化一次 |
| > 2 GB | 需要优化 | 执行归档操作 |

### 优化效果

- 查询性能提升: 20-50%
- 空间回收: 取决于碎片程度
- 优化时间: 1-5 分钟（取决于数据库大小）

## 备份建议

在执行归档操作前，强烈建议备份数据库：

```bash
# 方法 1: 直接复制文件
cp hub_server.db hub_server.db.backup

# 方法 2: 使用 SQLite 备份命令
sqlite3 hub_server.db ".backup hub_server.db.backup"
```

## 故障排除

### 优化失败

**可能原因**:
- 数据库被锁定
- 磁盘空间不足
- 数据库文件损坏

**解决方法**:
1. 停止所有客户端连接
2. 检查磁盘空间（需要数据库大小的 2 倍）
3. 使用 SQLite 工具检查完整性

### 归档失败

**可能原因**:
- 数据库连接超时
- 数据量过大

**解决方法**:
1. 减少保留时间，分批删除
2. 在低峰期执行
3. 检查数据库日志

## 文件说明

### 新增文件

```
hub_server/
├── controllers/
│   └── database.go              # 数据库管理控制器
├── docs/
│   ├── database-management-guide.md      # 使用指南
│   ├── database-management-testing.md    # 测试指南
│   └── database-performance.md           # 性能优化指南
├── scripts/
│   ├── optimize-database.sql             # 优化脚本
│   ├── archive-old-data.sql              # 归档脚本
│   └── analyze-performance.sql           # 性能分析脚本
├── frontend/src/views/
│   └── Admin.vue                         # 管理界面（已更新）
├── CHANGELOG.md                          # 更新日志
├── DATABASE_MANAGEMENT.md                # 本文件
└── hub_server_db.exe                     # 编译产物
```

### 修改文件

- `main.go` - 添加了 3 个数据库管理 API 路由
- `Admin.vue` - 添加了数据库管理标签和 UI

## 技术实现

### 后端

- 使用 GORM 执行原生 SQL
- 实现事务保证数据一致性
- 添加管理员权限验证
- 返回详细的操作结果

### 前端

- 使用 Vue 3 Composition API
- 集成 PrimeVue 组件库
- 实现响应式布局
- 友好的加载状态和错误提示

### 数据库

- SQLite 3
- 使用 PRAGMA 优化性能
- 使用 datetime() 函数处理时间
- 使用事务保证原子性

## 相关文档

- [使用指南](docs/database-management-guide.md) - 详细的功能说明和使用方法
- [测试指南](docs/database-management-testing.md) - 完整的测试用例和验证方法
- [性能优化](docs/database-performance.md) - 数据库性能优化最佳实践
- [更新日志](CHANGELOG.md) - 版本更新记录

## 版本信息

- 功能版本: 1.0.0
- Hub Server 版本: 1.0.3+
- 编译产物: `hub_server_db.exe`
- 发布日期: 2026-02-21

## 许可证

与 Hub Server 主项目相同

## 支持

如有问题或建议，请联系开发团队。
