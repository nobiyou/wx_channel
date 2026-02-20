# Hub 同步前端开发指南

## 概述

已完成 Hub Server 前端的数据同步管理页面开发，用户可以通过 Web 界面管理和监控所有设备的数据同步状态。

## 页面功能

### 1. 同步状态概览

**路径**: `/sync`

**功能特性**:
- 实时显示所有设备的同步状态
- 统计卡片展示：总设备数、同步中、成功、失败
- 自动刷新（30秒间隔）
- 支持手动刷新

### 2. 设备同步列表

**数据展示**:
- 同步状态（成功/失败/进行中/未同步）
- 设备名称和 Machine ID
- 浏览记录数量
- 下载记录数量
- 最后同步时间（浏览/下载）

**操作功能**:
- 立即同步单个设备
- 查看同步详情
- 查看同步历史
- 全局搜索和状态筛选

### 3. 同步详情对话框

**显示内容**:
- 设备基本信息
- 同步统计数据（浏览/下载记录数）
- 最后同步时间
- 错误信息（如果有）
- 快速同步按钮

### 4. 同步历史对话框

**历史记录**:
- 同步时间
- 同步类型（浏览/下载）
- 同步数量
- 状态（成功/失败）
- 错误信息
- 分页展示

## 技术实现

### 组件结构

```
src/views/Sync.vue
├── 统计卡片区域
│   ├── 总设备数
│   ├── 同步中数量
│   ├── 成功数量
│   └── 失败数量
├── 筛选面板
│   ├── 全局搜索
│   └── 状态筛选
├── 数据表格
│   ├── 状态列
│   ├── 设备信息列
│   ├── 记录统计列
│   ├── 同步时间列
│   └── 操作列
└── 对话框
    ├── 详情对话框
    └── 历史对话框
```

### 使用的组件

**PrimeVue 组件**:
- `DataTable` - 数据表格
- `Dialog` - 对话框
- `Button` - 按钮
- `Tag` - 标签
- `Toast` - 提示消息
- `ConfirmDialog` - 确认对话框
- `Select` - 下拉选择
- `InputText` - 文本输入

**图标**:
- `pi-sync` - 同步图标
- `pi-check-circle` - 成功图标
- `pi-times-circle` - 失败图标
- `pi-spinner` - 加载图标
- `pi-eye` - 浏览图标
- `pi-download` - 下载图标

### API 接口

页面需要以下后端 API 支持：

#### 1. 获取同步状态列表
```
GET /api/sync/status
```

**响应格式**:
```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 1,
      "machine_id": "device-001",
      "device_name": "设备1",
      "last_browse_sync_time": "2026-02-20T10:00:00Z",
      "last_download_sync_time": "2026-02-20T10:00:00Z",
      "browse_record_count": 1000,
      "download_record_count": 500,
      "last_sync_status": "success",
      "last_sync_error": "",
      "created_at": "2026-02-20T09:00:00Z",
      "updated_at": "2026-02-20T10:00:00Z"
    }
  ]
}
```

#### 2. 触发同步
```
POST /api/sync/trigger
```

**请求体**:
```json
{
  "machine_id": "device-001",  // 单个设备
  "sync_all": false            // 或同步所有设备
}
```

**响应格式**:
```json
{
  "code": 0,
  "message": "同步已开始"
}
```

#### 3. 获取同步历史
```
GET /api/sync/history/:machine_id
```

**响应格式**:
```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 1,
      "machine_id": "device-001",
      "sync_time": "2026-02-20T10:00:00Z",
      "sync_type": "browse",
      "records_synced": 100,
      "status": "success",
      "error_message": ""
    }
  ]
}
```

## 路由配置

**路由路径**: `/sync`

**权限要求**: 
- 需要登录 (`requiresAuth: true`)
- 使用侧边栏布局 (`layout: 'Sidebar'`)

**路由配置**:
```javascript
{
  path: '/sync',
  name: 'Sync',
  component: () => import('../views/Sync.vue'),
  meta: { requiresAuth: true, layout: 'Sidebar' }
}
```

## 侧边栏菜单

**位置**: Management 分组

**菜单项**:
```javascript
{
  label: '数据同步',
  icon: 'pi pi-sync',
  route: '/sync'
}
```

## 用户体验优化

### 1. 自动刷新
- 页面加载后自动刷新同步状态
- 每 30 秒自动刷新一次
- 页面卸载时清理定时器

### 2. 加载状态
- 数据加载时显示骨架屏
- 按钮操作时显示加载动画
- 同步进行中显示旋转图标

### 3. 错误处理
- API 错误时显示 Toast 提示
- 同步失败时显示错误信息
- 支持重试操作

### 4. 确认对话框
- 同步所有设备前显示确认对话框
- 防止误操作

### 5. 响应式设计
- 支持桌面和移动端
- 使用 Tailwind CSS 响应式类
- 表格在小屏幕上可横向滚动

## 样式规范

### 颜色方案

**状态颜色**:
- 成功: `text-green-500`, `bg-green-50`
- 失败: `text-red-500`, `bg-red-50`
- 进行中: `text-blue-500`, `bg-blue-50`
- 未同步: `text-text-muted`, `bg-surface-100`

**统计卡片**:
- 总设备: 蓝色 (`blue-500`)
- 同步中: 蓝色 (`blue-500`)
- 成功: 绿色 (`green-500`)
- 失败: 红色 (`red-500`)

### 间距规范

- 页面边距: `p-4 lg:p-12`
- 卡片间距: `gap-3 lg:gap-6`
- 组件内边距: `p-4 lg:p-6`

## 开发建议

### 1. 状态管理
如果需要跨页面共享同步状态，可以考虑使用 Pinia store：

```javascript
// store/sync.js
import { defineStore } from 'pinia'

export const useSyncStore = defineStore('sync', {
  state: () => ({
    syncStatuses: [],
    lastRefresh: null
  }),
  actions: {
    async fetchSyncStatus() {
      // 实现逻辑
    }
  }
})
```

### 2. WebSocket 实时更新
如果需要实时更新同步状态，可以集成 WebSocket：

```javascript
const ws = new WebSocket('ws://hub-server/ws/sync')
ws.onmessage = (event) => {
  const data = JSON.parse(event.data)
  // 更新同步状态
}
```

### 3. 性能优化
- 使用虚拟滚动处理大量设备
- 实现分页加载
- 缓存同步历史数据

## 测试建议

### 单元测试
- 测试计算属性（统计数量）
- 测试筛选逻辑
- 测试时间格式化函数

### 集成测试
- 测试 API 调用
- 测试错误处理
- 测试用户交互流程

### E2E 测试
- 测试完整的同步流程
- 测试多设备场景
- 测试错误恢复

## 后续优化

### 功能增强
1. 批量操作（批量同步、批量删除历史）
2. 同步计划（定时同步配置）
3. 同步报告（生成同步统计报告）
4. 数据可视化（同步趋势图表）
5. 导出功能（导出同步历史）

### 性能优化
1. 虚拟滚动（处理大量设备）
2. 懒加载（按需加载历史记录）
3. 缓存策略（减少 API 调用）
4. 增量更新（只更新变化的数据）

### 用户体验
1. 快捷键支持
2. 拖拽排序
3. 自定义列显示
4. 保存筛选条件
5. 深色模式优化

## 文件清单

### 新增文件
- `src/views/Sync.vue` - 同步管理页面

### 修改文件
- `src/router/index.js` - 添加同步路由
- `src/components/Sidebar.vue` - 添加同步菜单项

## 版本历史

- **v1.0** (2026-02-20): 初始版本
  - 基础同步状态展示
  - 触发同步功能
  - 查看详情和历史
  - 自动刷新
