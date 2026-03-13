# 浏览记录播放修复测试指南

## 问题描述
浏览记录播放视频时返回 404 错误，原因是 URL 中缺少 `X-snsvideoflag` 参数（如 `xWT128`）。

## 修复内容

### 1. 后端修改（客户端）
- **文件**: `wx_channel/internal/handlers/video_handler.go`
  - 修改 `processVideoData` 函数，从前端发送的 `media.spec` 中提取 `fileFormat` 字段
  - 修改 `saveBrowseRecord` 函数签名，添加 `fileFormat` 参数
  - 保存 `fileFormat` 到数据库

- **文件**: `wx_channel/internal/database/models.go`
  - 已添加 `FileFormat` 字段到 `BrowseRecord` 结构体

- **文件**: `wx_channel/internal/database/migrations.go`
  - 添加迁移版本 9：为 `browse_history` 表添加 `file_format` 列

### 2. 后端修改（Hub Server）
- **文件**: `wx_channel/hub_server/models/sync.go`
  - 已添加 `FileFormat` 字段到 `HubBrowseHistory` 结构体
  - GORM 会自动创建该列

### 3. 前端修改
- **文件**: `wx_channel/hub_server/frontend/src/views/VideoDetail.vue`
  - 优先使用保存的 `file_format` 字段构建视频 URL
  - 如果没有 `file_format`，回退到根据分辨率推测（向后兼容）

- **文件**: `wx_channel/hub_server/frontend/src/views/Sync.vue`
  - 在跳转到视频详情页时，传递 `file_format` 字段

## 测试步骤

### 步骤 1: 编译客户端
```bash
cd wx_channel
go build -o wx_channel_file_format_fix.exe
```

### 步骤 2: 编译 Hub Server
```bash
cd hub_server
go build -o hub_server_file_format_fix.exe
```

### 步骤 3: 启动客户端
1. 停止旧的客户端
2. 启动新编译的客户端：`wx_channel_file_format_fix.exe`
3. 数据库迁移会自动运行，添加 `file_format` 列

### 步骤 4: 启动 Hub Server
1. 停止旧的 Hub Server
2. 启动新编译的 Hub Server：`hub_server_file_format_fix.exe`
3. GORM 会自动添加 `file_format` 列到 `hub_browse_history` 表

### 步骤 5: 测试新浏览记录
1. 在微信视频号中播放一个新视频
2. 检查客户端日志，确认提取了 `fileFormat`：
   ```
   [视频格式] 从spec获取: xWT128
   ```
3. 检查数据库，确认 `file_format` 字段已保存：
   ```sql
   SELECT id, title, resolution, file_format FROM browse_history ORDER BY browse_time DESC LIMIT 5;
   ```

### 步骤 6: 等待同步
1. 等待客户端自动同步数据到 Hub Server（30秒间隔）
2. 或者重启客户端触发立即同步

### 步骤 7: 测试播放
1. 打开 Hub Server 前端：http://localhost:8080
2. 进入"数据同步"页面
3. 点击"浏览记录"按钮
4. 找到刚才播放的视频，点击"播放"按钮
5. 检查浏览器控制台，确认使用了 `file_format`：
   ```
   [VideoDetail] Using saved file_format: xWT128
   ```
6. 视频应该能正常播放，不再返回 404 错误

### 步骤 8: 测试向后兼容
1. 查看旧的浏览记录（没有 `file_format` 字段的记录）
2. 点击播放按钮
3. 应该能看到日志：
   ```
   [VideoDetail] No file_format saved, guessing from resolution: 1080x1920
   ```
4. 视频应该能播放（使用推测的格式）

## 验证数据库结构

### 客户端数据库
```sql
-- 检查 browse_history 表结构
PRAGMA table_info(browse_history);

-- 应该看到 file_format 列
-- 检查数据
SELECT id, title, resolution, file_format FROM browse_history ORDER BY browse_time DESC LIMIT 10;
```

### Hub Server 数据库
```sql
-- 检查 hub_browse_history 表结构
PRAGMA table_info(hub_browse_history);

-- 应该看到 file_format 列
-- 检查数据
SELECT id, title, resolution, file_format FROM hub_browse_history ORDER BY browse_time DESC LIMIT 10;
```

## 预期结果

1. 新浏览的视频会保存 `file_format` 字段（如 `xWT128`, `xWT111`）
2. 从浏览记录播放视频时，URL 会包含正确的 `X-snsvideoflag` 参数
3. 视频能正常播放，不再返回 404 错误
4. 旧的浏览记录（没有 `file_format`）仍然能播放（使用推测的格式）

## 故障排查

### 问题 1: 数据库迁移失败
- 检查客户端启动日志，查看迁移错误信息
- 手动运行迁移 SQL：
  ```sql
  ALTER TABLE browse_history ADD COLUMN file_format TEXT DEFAULT '';
  ```

### 问题 2: file_format 字段为空
- 检查客户端日志，确认是否提取了 `fileFormat`
- 检查前端发送的数据是否包含 `media.spec`
- 可能是某些视频的 spec 数组为空

### 问题 3: 视频仍然返回 404
- 检查浏览器控制台，查看构建的 URL
- 确认 `X-snsvideoflag` 参数是否正确
- 尝试手动在 URL 中添加不同的格式标识（xWT128, xWT111, xWT110）

### 问题 4: Hub Server 数据库没有 file_format 列
- 重启 Hub Server，GORM 会自动添加列
- 或手动运行：
  ```sql
  ALTER TABLE hub_browse_history ADD COLUMN file_format TEXT DEFAULT '';
  ```

## 日志关键字

搜索以下关键字来诊断问题：
- `[视频格式]` - 客户端提取 fileFormat
- `[VideoDetail] Using saved file_format` - 前端使用保存的格式
- `[VideoDetail] No file_format saved` - 前端使用推测的格式
- `Applied migration 9` - 数据库迁移成功

## 下一步

如果测试成功，可以：
1. 将修复合并到主分支
2. 发布新版本
3. 通知用户更新客户端和 Hub Server
