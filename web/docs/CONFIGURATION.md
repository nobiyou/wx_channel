# 配置概览

## 配置概览

微信视频号下载助手可通过多种方式进行配置。您可以通过以下方式自定义程序行为：

### 环境变量配置

这是推荐的配置方式。

大多数配置选项可通过环境变量进行设置。程序启动时会自动读取这些环境变量。

#### 基本配置

程序支持以下环境变量：

```bash
# 代理端口（默认：2025）
WX_CHANNEL_PORT=2025

# 下载目录（默认：downloads）
WX_CHANNEL_DOWNLOADS_DIR=downloads

# 记录文件名（默认：download_records.csv）
WX_CHANNEL_RECORDS_FILE=download_records.csv
```

#### 安全配置

```bash
# 本地授权令牌（可选）
# 设置后，所有 API 请求需在请求头携带 X-Local-Auth: <token>
WX_CHANNEL_TOKEN=your_secret_token

# Origin 白名单（可选，逗号分隔）
# 启用后，仅允许匹配的 Origin 调用接口
WX_CHANNEL_ALLOWED_ORIGINS=https://example.com,https://foo.bar
```

#### 日志配置

```bash
# 日志文件路径（默认：logs/wx_channel.log）
WX_CHANNEL_LOG_FILE=logs/wx_channel.log

# 单个日志文件最大大小（MB，默认：5）
# 达到大小后会自动滚动
WX_CHANNEL_LOG_MAX_MB=5
```

#### 上传配置

```bash
# 分片大小（字节，默认：2097152，即 2MB）
WX_CHANNEL_CHUNK_SIZE=2097152

# 最大上传大小（字节，默认：67108864，即 64MB）
WX_CHANNEL_MAX_UPLOAD_SIZE=67108864

# 分片上传并发上限（默认：4）
WX_CHANNEL_UPLOAD_CHUNK_CONCURRENCY=4

# 分片合并并发上限（默认：1）
WX_CHANNEL_UPLOAD_MERGE_CONCURRENCY=1
```

#### 下载配置

```bash
# 批量下载并发上限（默认：2）
WX_CHANNEL_DOWNLOAD_CONCURRENCY=2

# 批量下载重试次数（默认：3）
WX_CHANNEL_DOWNLOAD_RETRY_COUNT=3
```

#### UI 功能开关

```bash
# 是否显示左下角日志按钮（默认：false）
WX_CHANNEL_SHOW_LOG_BUTTON=false
```

**说明**：
* 设置为 `true` 或 `1` 或 `yes` 时显示日志按钮
* 默认隐藏，避免干扰正常使用
* 即使隐藏按钮，仍可通过快捷键 `Ctrl+Shift+L` 打开日志面板（桌面浏览器）

### 命令行参数

程序支持以下命令行参数：

```bash
# 显示帮助信息
wx_channel.exe --help

# 显示版本信息
wx_channel.exe -v
wx_channel.exe --version

# 指定代理端口
wx_channel.exe -p 8080
wx_channel.exe --port 8080

# 卸载根证书
wx_channel.exe --uninstall
```

#### 参数说明

* `--help`: 显示帮助信息并退出
* `-v, --version`: 显示版本信息并退出
* `-p, --port`: 设置代理服务器端口（默认：2025）
* `--uninstall`: 卸载根证书并退出

### 证书配置

#### 自动安装

程序首次运行时会自动检测并安装根证书（SunnyRoot.cer）。如果权限不足导致安装失败，程序会将证书文件保存到 `downloads/SunnyRoot.cer`，您可以手动安装。

#### 手动安装

如果自动安装失败，您可以：

1. 找到程序目录下的 `downloads/SunnyRoot.cer` 文件
2. 双击证书文件
3. 按照系统提示完成安装
4. 重新进入视频号

#### 卸载证书

使用 `--uninstall` 参数可以卸载已安装的根证书：

```bash
wx_channel.exe --uninstall
```

**注意**：卸载证书可能需要管理员权限。如果程序仍在运行，请重新进入视频号以确保更改生效。

### 日志配置

#### 默认行为

程序默认开启日志功能，日志文件保存在 `logs/wx_channel.log`。

#### 日志滚动

当日志文件达到指定大小（默认 5MB）时，会自动创建新的日志文件，旧日志会被保留。

#### 自定义日志配置

通过环境变量可以自定义日志配置：

```bash
# 自定义日志文件路径
WX_CHANNEL_LOG_FILE=my_custom.log

# 自定义日志文件最大大小（MB）
WX_CHANNEL_LOG_MAX_MB=10
```

### 安全配置

#### 本地授权令牌

为了增强安全性，您可以设置本地授权令牌。设置后，所有 API 请求都需要在请求头中携带正确的令牌。

**设置方式**：

```bash
# Windows PowerShell
$env:WX_CHANNEL_TOKEN="your_secret_token"
wx_channel.exe

# Windows CMD
set WX_CHANNEL_TOKEN=your_secret_token
wx_channel.exe

# Linux/macOS
export WX_CHANNEL_TOKEN=your_secret_token
./wx_channel
```

**使用方式**：

前端或调用方需要在请求头中携带：

```
X-Local-Auth: your_secret_token
```

#### Origin 白名单

如果您需要限制 API 的访问来源，可以设置 Origin 白名单：

```bash
# Windows PowerShell
$env:WX_CHANNEL_ALLOWED_ORIGINS="https://example.com,https://foo.bar"
wx_channel.exe
```

**说明**：

* 启用后，仅允许匹配的 `Origin` 调用接口
* 支持多个 Origin，使用逗号分隔
* 空的 `Origin` 不会被拦截
* 接口会支持 CORS 预检（`POST, OPTIONS`）
* 允许的请求头：`Content-Type, X-Local-Auth`

### 下载目录结构

程序会自动创建以下目录结构：

```
downloads/
├── download_records.csv          # 下载记录 CSV 文件
├── SunnyRoot.cer                 # 根证书文件（如果自动安装失败）
├── .uploads/                     # 分片上传临时目录
│   └── <uploadId>/
│       ├── 000000.part
│       ├── 000001.part
│       └── ...
└── <作者名>/                     # 按作者分类的视频文件
    ├── <文件名>.mp4
    └── <文件名>(1).mp4           # 重名文件自动编号
```

#### 文件名处理

* 文件名和目录名会自动清理非法字符
* 如果文件名缺少扩展名，会自动补充 `.mp4`
* 重名文件会自动添加编号，如 `(1)`, `(2)` 等

### 高级配置

#### 并发控制

程序支持多种并发控制选项，可以通过环境变量进行配置：

```bash
# 分片上传并发上限（默认：4）
WX_CHANNEL_UPLOAD_CHUNK_CONCURRENCY=4

# 分片合并并发上限（默认：1）
WX_CHANNEL_UPLOAD_MERGE_CONCURRENCY=1

# 批量下载并发上限（默认：2）
WX_CHANNEL_DOWNLOAD_CONCURRENCY=2
```

#### 重试配置

```bash
# 批量下载重试次数（默认：3）
WX_CHANNEL_DOWNLOAD_RETRY_COUNT=3
```

#### 上传配置

```bash
# 分片大小（字节，默认：2MB）
WX_CHANNEL_CHUNK_SIZE=2097152

# 最大上传大小（字节，默认：64MB）
WX_CHANNEL_MAX_UPLOAD_SIZE=67108864
```

### 配置优先级

配置的优先级从高到低为：

1. **命令行参数**（最高优先级）
2. **环境变量**
3. **默认值**（最低优先级）

例如，如果同时设置了环境变量和命令行参数，命令行参数会覆盖环境变量。

### 配置示例

#### 示例 1：基本使用

```bash
# 使用默认配置启动
wx_channel.exe
```

#### 示例 2：自定义端口

```bash
# 方式 1：命令行参数
wx_channel.exe -p 8080

# 方式 2：环境变量
$env:WX_CHANNEL_PORT=8080
wx_channel.exe
```

#### 示例 3：启用安全配置

```bash
# 设置授权令牌和 Origin 白名单
$env:WX_CHANNEL_TOKEN="my_secret_token_123"
$env:WX_CHANNEL_ALLOWED_ORIGINS="https://channels.weixin.qq.com"
wx_channel.exe -p 2025
```

#### 示例 4：自定义日志配置

```bash
# 自定义日志路径和大小
$env:WX_CHANNEL_LOG_FILE="custom_logs/app.log"
$env:WX_CHANNEL_LOG_MAX_MB=10
wx_channel.exe
```

### 故障排除

#### 证书安装失败

如果证书自动安装失败：

1. 检查是否以管理员身份运行程序
2. 查看 `downloads/SunnyRoot.cer` 文件是否存在
3. 手动双击证书文件进行安装
4. 安装完成后重新打开视频号

#### 代理无法启动

如果代理服务无法启动：

1. 检查端口是否被占用：`netstat -ano | findstr :2025`
2. 尝试使用其他端口：`wx_channel.exe -p 8080`
3. 检查防火墙设置
4. 确保以管理员身份运行（Windows）

#### 日志文件未生成

如果日志文件未生成：

1. 检查 `logs/` 目录是否存在
2. 检查程序是否有写入权限
3. 查看环境变量 `WX_CHANNEL_LOG_FILE` 是否正确设置

### 相关文档

* README.md - 项目概览和快速开始
* OPTIMIZATION.md - 优化建议和实施计划
