# 批量下载功能使用指南

## 概述

批量下载功能允许一次性下载多个视频号视频，支持普通视频和加密视频的自动处理。

### 主要特性

- 批量下载多个视频
- 自动解密加密视频
- WASM 集成（自动处理密钥）
- 重试机制（网络问题自动恢复）
- 实时进度显示
- Web 控制台界面

## 快速开始

### 1. 启动程序

```bash
./wx_channel.exe
```

### 2. 打开 Web 控制台

在浏览器中访问：
```
http://127.0.0.1:2025/console
```

### 3. 准备视频列表

#### 选项 A：批量下载格式（标准）

```json
[
  {
    "id": "video_001",
    "url": "https://finder.video.qq.com/xxx/video.mp4",
    "title": "我的视频",
    "authorName": "作者名"
  }
]
```

#### 选项 B：账户导出格式（自动转换）

```json
{
  "videos": [
    {
      "id": "video_001",
      "url": "https://finder.video.qq.com/xxx/video.mp4",
      "title": "我的视频",
      "author": "作者名",
      "key": "123456789"
    }
  ]
}
```

### 4. 开始下载

1. 将视频列表粘贴到文本框
2. 点击"开始下载"按钮
3. 等待下载完成

### 5. 查看结果

下载的视频保存在：
```
downloads/作者名/视频标题.mp4
```

## 使用方法

### 方法一：Web 控制台（推荐）

1. 访问 `http://127.0.0.1:2025/console`
2. 填写 API 地址（默认：`http://127.0.0.1:2025`）
3. 如果配置了授权令牌，填写令牌
4. 粘贴视频列表 JSON
5. 点击"开始下载"
6. 实时查看下载进度

### 方法二：Profile 页面

1. 打开视频号 Profile 页面
2. 等待视频列表加载
3. 点击"后端批量下载"按钮
4. 系统自动处理加密密钥并下载

### 方法三：API 调用

```bash
# 开始下载
curl -X POST http://127.0.0.1:2025/__wx_channels_api/batch_start \
  -H "Content-Type: application/json" \
  -d @videos.json

# 查询进度
curl http://127.0.0.1:2025/__wx_channels_api/batch_progress

# 取消下载
curl -X POST http://127.0.0.1:2025/__wx_channels_api/batch_cancel

# 导出失败清单
curl http://127.0.0.1:2025/__wx_channels_api/batch_failed
```

## 数据格式

### 批量下载格式（标准）

```json
[
  {
    "id": "video_001",
    "url": "https://example.com/video.mp4",
    "title": "视频标题",
    "authorName": "作者名称",
    "decryptorPrefix": "AQIDBAUG...",
    "prefixLen": 1024
  }
]
```

### 账户导出格式（自动转换）

```json
{
  "videos": [
    {
      "id": "video_001",
      "url": "https://example.com/video.mp4",
      "title": "视频标题",
      "author": "作者名称",
      "key": "123765583"
    }
  ]
}
```

**自动转换**：
- `author` → `authorName`
- `key` → `decryptorPrefix`（通过 WASM 处理）
- 自动添加 `prefixLen`

## 核心功能

### 1. 批量下载

- 支持一次下载多个视频
- 串行下载（避免服务器压力）
- 自动创建作者文件夹
- 文件名冲突自动处理

### 2. 加密视频支持

- XOR 解密算法
- Base64 密钥解码
- 前缀解密（prefixLen）
- 完整视频保存

### 3. WASM 集成

- 自动加载 WASM 库
- 处理 key 字段
- 生成 decryptorPrefix
- 支持账户导出格式

### 4. 重试机制

- 自动重试（最多3次）
- 递增延迟策略（2s, 4s, 6s）
- Context 超时控制（10分钟）
- 详细的重试日志

### 5. 错误处理

- 失败任务记录
- 导出失败清单
- 详细的错误信息
- 文件完整性验证

### 6. 用户体验

- Web 控制台界面
- 实时进度更新
- 格式自动转换
- 清晰的状态提示

## 常用操作

### 查看进度

进度会自动更新显示：
- 总任务数
- 已完成
- 失败
- 进行中

### 取消下载

点击"取消下载"按钮即可停止当前下载任务。

### 导出失败清单

如果有下载失败的视频，点击"导出失败清单"按钮，失败清单会保存到：
```
downloads/batch_failed_YYYYMMDD_HHMMSS.json
```

### 强制重新下载

勾选"强制重新下载"选项，即使文件已存在也会重新下载。

## 使用示例

### 示例 1：下载单个视频

```json
[
  {
    "id": "14792935438852163674",
    "url": "https://finder.video.qq.com/251/20302/stodownload?encfilekey=...",
    "title": "听说大家想看船上大厨做饭",
    "authorName": "中国船员"
  }
]
```

### 示例 2：批量下载多个视频

```json
[
  {
    "id": "video_001",
    "url": "https://example.com/video1.mp4",
    "title": "视频1",
    "authorName": "作者A"
  },
  {
    "id": "video_002",
    "url": "https://example.com/video2.mp4",
    "title": "视频2",
    "authorName": "作者B"
  },
  {
    "id": "video_003",
    "url": "https://example.com/video3.mp4",
    "title": "视频3",
    "authorName": "作者A"
  }
]
```

### 示例 3：下载加密视频

```json
[
  {
    "id": "encrypted_001",
    "url": "https://example.com/encrypted.mp4",
    "title": "加密视频",
    "authorName": "作者名",
    "decryptorPrefix": "AQIDBAUG",
    "prefixLen": 1024
  }
]
```

### 示例 4：使用账户导出格式

```json
{
  "videos": [
    {
      "id": "video_001",
      "url": "https://example.com/encrypted.mp4",
      "title": "加密视频",
      "author": "作者名",
      "key": "123765583"
    }
  ]
}
```

## 高级选项

### 自定义 API 地址

如果程序运行在其他端口，修改"API 地址"：
```
http://127.0.0.1:YOUR_PORT
```

### 使用授权令牌

如果配置了 `WX_CHANNEL_TOKEN`，在"授权令牌"框中填写。

## 性能特点

### 下载性能
- 并发：串行下载（避免服务器压力）
- 重试：最多 3 次自动重试
- 超时：10 分钟（适合大文件）
- 缓冲：32KB 缓冲区

### 内存使用
- WASM 库：约 200KB
- 缓冲区：32KB × 任务数
- 总体：很小，适合长时间运行

### 网络使用
- 连接池：复用 HTTP 连接
- 超时控制：避免长时间占用
- 重试策略：递增延迟

## 最佳实践

### 1. 数据准备
- 使用 Profile 页面导出数据（最可靠）
- 或使用 Web 控制台自动转换
- 确保 URL 有效
- 确保密钥正确

### 2. 下载管理
- 批量下载前检查磁盘空间
- 网络不稳定时减少并发数
- 定期查看下载进度
- 及时处理失败任务

### 3. 错误处理
- 查看失败清单
- 检查错误原因
- 修正后重新下载
- 保存日志用于排查

### 4. 性能优化
- 大文件增加超时时间
- 网络不稳定增加重试次数
- 使用有线网络（更稳定）
- 避免高峰时段下载

## 常见问题

### Q: 下载速度慢？
A: 批量下载采用串行方式，避免对服务器造成压力。请耐心等待。

### Q: 下载失败怎么办？
A: 点击"导出失败清单"查看失败原因，修正后重新下载。

### Q: 视频无法播放？
A: 检查是否是加密视频，确保提供了正确的解密密钥。

### Q: 文件保存在哪里？
A: 默认保存在 `downloads/作者名/` 目录下。

### Q: 如何处理加密视频？
A: 使用 Profile 页面的"后端批量下载"功能，或提供正确的 decryptorPrefix。

### Q: 可以并发下载吗？
A: 当前版本采用串行下载，未来版本会支持可配置的并发下载。

## 故障排除

### 下载失败
1. 检查视频 URL 是否有效
2. 检查网络连接
3. 查看失败清单中的错误信息
4. 检查磁盘空间

### 解密失败
1. 确认 `decryptorPrefix` 是正确的 Base64 编码
2. 确认 `prefixLen` 值正确（通常为 1024）
3. 检查密钥是否与视频匹配
4. 使用 Profile 页面重新下载

### 授权失败
1. 确认配置文件中的 `WX_CHANNEL_TOKEN` 设置
2. 确认请求头中包含正确的 `X-Local-Auth` 值
3. 检查令牌是否包含特殊字符

### WASM 加载失败
1. 检查网络连接
2. 确认可以访问微信 CDN
3. 使用现代浏览器（Chrome 90+、Firefox 88+）
4. 使用 Profile 页面作为备选

## 安全性

### 授权验证
如果配置了 `WX_CHANNEL_TOKEN`，所有 API 请求需要包含授权头：
```
X-Local-Auth: your-token
```

### CORS 支持
配置 `ALLOWED_ORIGINS` 限制允许的请求来源。

### 数据安全
- 解密密钥在内存中处理
- 临时文件自动清理
- 不保存敏感信息

## 相关文档

- [Web 控制台使用指南](WEB_CONSOLE.md) - Web 控制台完整指南
- [API 文档](API.md) - HTTP API 接口说明
- [配置概览](CONFIGURATION.md) - 配置选项说明
- [故障排除](TROUBLESHOOTING.md) - 常见问题解决方案

### 详细文档（开发者参考）

- [批量下载索引](fix/BATCH_DOWNLOAD_INDEX.md) - 完整文档导航
- [批量下载快速开始](fix/BATCH_DOWNLOAD_QUICK_START.md) - 5分钟上手
- [批量下载 API](fix/BATCH_DOWNLOAD_API.md) - API 详细文档
- [批量下载加密说明](fix/BATCH_DOWNLOAD_ENCRYPTION.md) - 加密详解

---

最后更新：2025-11-23
