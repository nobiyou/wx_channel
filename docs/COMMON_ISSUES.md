# 常见问题

本文档列出用户最常遇到的问题和快速解决方案。

## 快速索引

- [证书问题](#证书问题)
- [代理问题](#代理问题)
- [下载问题](#下载问题)
- [配置问题](#配置问题)
- [使用问题](#使用问题)

### 证书问题

#### Q: 如何安装证书？

**A:** 程序首次运行时会自动尝试安装证书。如果失败：

1. 找到 `downloads/SunnyRoot.cer` 文件
2. 双击证书文件
3. 选择"安装证书"
4. 选择"本地计算机" → "将所有证书放入以下存储" → "受信任的根证书颁发机构"
5. 完成安装后重新打开视频号

#### Q: 如何卸载证书？

**A:** 使用以下命令：

```bash
wx_channel.exe --uninstall
```

### 代理问题

#### Q: 如何更改代理端口？

**A:** 使用命令行参数：

```bash
wx_channel.exe -p 8080
```

或设置环境变量：

```bash
# Windows PowerShell
$env:WX_CHANNEL_PORT=8080

# Linux/macOS
export WX_CHANNEL_PORT=8080
```

#### Q: 端口被占用怎么办？

**A:**

1. 检查端口占用：`netstat -ano | findstr :2025`（Windows）
2. 关闭占用端口的程序
3. 或使用其他端口：`wx_channel.exe -p 8080`

#### Q: 代理设置不生效？

**A:**

1. 确认程序正在运行
2. 确认代理地址和端口正确
3. 确认使用的是 HTTP 代理
4. 尝试使用代理扩展而非系统代理

### 下载问题

#### Q: 视频无法下载？

**A:**

1. 查看面板错误
2. 确认脚本已正确注入
3. 检查 `downloads/` 目录权限
4. 查看日志文件：`logs/wx_channel.log`

#### Q: 下载的文件在哪里？

**A:** 默认保存在 `downloads/` 目录下，按作者名称分类：

```
downloads/
├── <作者名>/
│   └── <视频文件名>.mp4
└── download_records.csv
```

#### Q: 如何批量下载？

**A:**

1. 打开视频号主页
2. 点击注入面板中的"编辑选择"
3. 勾选要下载的视频
4. 选择"仅选中-前端下载"或"仅选中-后端下载"

#### Q: 下载记录在哪里？

**A:** 所有下载记录保存在 `downloads/download_records.csv` 文件中，包含视频的详细信息。

#### Q: 如何导出视频链接？

**A:**

1. 打开视频号主页
2. 点击"导出链接"
3. 选择格式：TXT / JSON / Markdown

### 配置问题

#### Q: 如何启用安全认证？

**A:** 设置环境变量：

```bash
# Windows PowerShell
$env:WX_CHANNEL_TOKEN="your_secret_token"
wx_channel.exe
```

设置后，所有 API 请求需要在请求头携带：`X-Local-Auth: your_secret_token`

#### Q: 如何自定义下载目录？

**A:** 设置环境变量：

```bash
$env:WX_CHANNEL_DOWNLOADS_DIR="D:\Videos"
```

#### Q: 如何配置日志？

**A:**

```bash
# 自定义日志路径
$env:WX_CHANNEL_LOG_FILE="custom.log"

# 自定义日志大小（MB）
$env:WX_CHANNEL_LOG_MAX_MB=10
```

#### Q: 环境变量不生效？

**A:**

1. 确认变量名正确（以 `WX_CHANNEL_` 开头，全大写）
2. 环境变量需要在程序启动前设置
3. 设置后重新启动程序

### 使用问题

#### Q: 程序启动后要做什么？

**A:**

1. 配置查看代理：`127.0.0.1:2025`
2. 安装证书（如果未自动安装）
3. 打开微信视频号页面
4. 使用注入的操作面板进行下载

#### Q: 如何查看程序版本？

**A:**

```bash
wx_channel.exe -v
# 或
wx_channel.exe --version
```

#### Q: 如何查看帮助信息？

**A:**

```bash
wx_channel.exe --help
```

#### Q: 程序占用资源高怎么办？

**A:**

1. 降低并发数：

   ```bash
   $env:WX_CHANNEL_UPLOAD_CHUNK_CONCURRENCY=2
   $env:WX_CHANNEL_DOWNLOAD_CONCURRENCY=1
   ```
2. 清理日志文件
3. 关闭不必要的功能

#### Q: 支持哪些操作系统？

**A:**

* Windows 10+

#### Q: 可以下载哪些内容？

**A:** 目前支持下载微信视频号的视频内容，包括：

* 单个视频
* 主页视频列表
* 视频信息（标题、作者、互动数据等）

### 高级问题

#### Q: 如何自定义分片大小？

**A:** 设置环境变量（字节）：

```bash
$env:WX_CHANNEL_CHUNK_SIZE=4194304  # 4MB
```

#### Q: 如何调整并发数？

**A:**

```bash
# 分片上传并发
$env:WX_CHANNEL_UPLOAD_CHUNK_CONCURRENCY=4

# 分片合并并发
$env:WX_CHANNEL_UPLOAD_MERGE_CONCURRENCY=1

# 批量下载并发
$env:WX_CHANNEL_DOWNLOAD_CONCURRENCY=2
```

#### Q: 如何设置 Origin 白名单？

**A:**

```bash
$env:WX_CHANNEL_ALLOWED_ORIGINS="https://channels.weixin.qq.com"
```

多个 Origin 用逗号分隔。

#### Q: 下载失败如何重试？

**A:**

1. 使用批量下载的失败清单功能
2. 导出失败清单：`batch_failed` API
3. 根据清单重新下载

### 获取更多帮助

如果以上问题无法解决您的问题：

1. **查看详细文档**
   * 配置概览
   * 故障排除
2. **查看日志**
   * 检查 `logs/wx_channel.log` 文件
3. **提交 Issue**
   * [GitHub Issues](https://github.com/nobiyou/wx_channel/issues)
   * 提供详细的错误信息和日志
