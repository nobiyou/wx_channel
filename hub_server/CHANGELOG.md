# Hub Server 更新日志

## [未发布] - 2026-02-21

### 新增功能

#### 数据库管理功能
- ✅ 添加数据库统计 API (`/api/admin/database/stats`)
  - 显示数据库大小、总记录数
  - 显示各表的记录数和占用空间
  
- ✅ 添加数据库优化 API (`/api/admin/database/optimize`)
  - 执行 ANALYZE 优化查询性能
  - 执行 VACUUM 回收空间
  - 重建索引提升效率

- ✅ 添加数据归档 API (`/api/admin/database/archive`)
  - 可配置浏览记录保留时间（月）
  - 可配置下载记录保留时间（年）
  - 可配置同步历史保留时间（月）
  - 返回删除的记录数统计

- ✅ 前端管理界面
  - 在管理后台添加"数据库管理"标签
  - 实时显示数据库统计信息
  - 一键优化数据库
  - 可视化配置归档策略
  - 友好的确认对话框和加载状态

### 技术实现

#### 后端
- 新增 `controllers/database.go` 控制器
- 实现三个管理员专用 API 接口
- 使用原生 SQL 执行优化和归档操作
- 添加权限验证（仅管理员可访问）

#### 前端
- 更新 `views/Admin.vue` 组件
- 添加数据库管理面板 UI
- 集成 PrimeVue 组件（InputNumber, Button, Dialog）
- 实现响应式布局和加载状态

#### 文档
- 创建 `docs/database-management-guide.md` 使用指南
- 创建 `scripts/optimize-database.sql` 优化脚本
- 创建 `scripts/archive-old-data.sql` 归档脚本
- 创建 `docs/database-performance.md` 性能指南

### 编译产物
- `hub_server_db.exe` - 包含数据库管理功能的 Hub Server

### 性能优化
- 数据库优化可提升查询性能 20-50%
- VACUUM 可回收已删除数据占用的空间
- 归档功能可有效控制数据库大小

### 使用建议
- 建议每月执行一次数据库优化
- 根据存储空间调整归档策略
- 归档前建议备份数据库
- 在低峰期执行优化和归档操作

---

## [1.0.3] - 2026-02-20

### 修复

#### 同步功能修复
- ✅ 修复 JSON 标签不匹配问题（驼峰命名 vs 下划线命名）
- ✅ 修复数据类型不匹配（int → int64）
- ✅ 修复 sync_status 表更新问题
- ✅ 修复时长显示问题（毫秒转秒）

#### WebSocket 修复
- ✅ 修复 WebSocket 连接立即断开问题
- ✅ 改用 context.Background() 避免 context canceled 错误

#### 前端修复
- ✅ 修复批量下载进度显示问题
- ✅ 添加格式转换逻辑处理接口响应

### 新增功能

#### Hub 同步功能
- ✅ 实现 WebSocket 推送模式
- ✅ 客户端主动推送数据到 Hub Server
- ✅ 支持浏览记录和下载记录同步
- ✅ 前端同步页面（查看记录、同步历史）

### 测试结果
- 成功同步 497 条浏览记录
- 成功同步 93 条下载记录
- WebSocket 连接稳定运行 32 分钟

---

## 版本说明

### 版本命名规则
- 主版本号：重大架构变更
- 次版本号：新功能添加
- 修订号：Bug 修复和小改进

### 编译产物命名
- `hub_server.exe` - 标准版本
- `hub_server_<feature>.exe` - 特性版本（如 hub_server_db.exe）
- `hub_server_v<version>.exe` - 发布版本（如 hub_server_v1.0.3.exe）
