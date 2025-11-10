# 安装指南

## 安装指南

本指南将帮助您安装和配置微信视频号下载助手。

### 前置要求

* **操作系统**：Windows 10+
* **Go 环境**：1.23+（仅当需要从源码编译时）
* **微信版本**：最新版

### 安装方式

#### 方式一：使用预编译版本（推荐）

1. **下载程序**
   * 访问 [GitHub Releases](https://github.com/nobiyou/wx_channel/releases)
   * 下载对应操作系统的最新版本
   * Windows 用户下载 `wx_channel.exe`
2. **解压文件**
   * 将下载的文件解压到任意目录（如 `C:\wx_channel\` 或 `~/wx_channel/`）
3. **运行程序**

   ```bash
   # Windows
   wx_channel.exe
   ```

#### 方式二：从源码编译

1. **安装 Go 环境**
   * 访问 <https://golang.org/dl/> 下载并安装 Go 1.23+
   * 验证安装：`go version`
2. **克隆仓库**

   ```bash
   git clone https://github.com/nobiyou/wx_channel.git
   cd wx_channel
   ```
3. **编译程序**

   ```bash
   # 基本编译
   go build -o wx_channel.exe

   # 优化体积编译（推荐）
   go build -ldflags="-s -w" -o wx_channel_mini.exe
   ```

### 首次运行配置

#### 1. 启动程序

以管理员身份运行程序（Windows 推荐，便于自动安装证书）：

```bash
# Windows（以管理员身份运行 PowerShell 或 CMD）
wx_channel.exe
```

#### 2. 安装根证书

程序首次运行时会自动尝试安装根证书（SunnyRoot.cer）。

**自动安装成功**：

* 程序会显示 "✓ 证书安装成功！"
* 重启浏览器后即可使用

**自动安装失败**：

* 程序会将证书保存到 `downloads/SunnyRoot.cer`
* 手动安装步骤：
  1. 找到 `downloads/SunnyRoot.cer` 文件
  2. 双击证书文件
  3. 按照系统提示完成安装
  4. 重新打开视频号

#### 3. 配置代理

**微信浏览器自动代理**

1. 打开视频号
2. 刷新页面
3. 查看日志代理是否成功

#### 4. 验证安装

1. 打开微信视频号页面
2. 刷新页面
3. 如果看到注入的脚本和面板，说明安装成功

### 自定义配置

#### 更改代理端口

如果默认端口 2025 被占用，可以更改：

```bash
# 命令行参数
wx_channel.exe -p 8080

# 或设置环境变量
$env:WX_CHANNEL_PORT=8080  # Windows PowerShell
```

#### 自定义下载目录

```bash
# 设置环境变量
$env:WX_CHANNEL_DOWNLOADS_DIR="D:\Videos"  # Windows
```

#### 启用安全认证

```bash
# 设置授权令牌
$env:WX_CHANNEL_TOKEN="your_secret_token"
wx_channel.exe
```

更多配置选项请参考 配置概览。

### 卸载

#### 卸载证书

```bash
wx_channel.exe --uninstall
```

#### 删除程序

1. 停止运行中的程序（Ctrl+C）
2. 删除程序文件
3. 删除下载目录（可选）：`downloads/`
4. 删除日志目录（可选）：`logs/`

### 升级

#### 升级到新版本

1. **备份数据**（可选）
   * 备份 `downloads/` 目录
   * 备份配置文件（如果有）
2. **下载新版本**
   * 从 GitHub Releases 下载最新版本
   * 或重新编译源码
3. **替换程序文件**
   * 停止旧版本程序
   * 替换可执行文件
4. **运行新版本**
   * 启动程序，配置会自动迁移

#### 从源码升级

```bash
cd wx_channel
git pull
go build -ldflags="-s -w" -o wx_channel.exe
```

### 常见安装问题

#### 证书安装失败

**问题**：程序提示证书安装失败

**解决方案**：

1. 确保以管理员身份运行（Windows）
2. 手动安装证书：双击 `downloads/SunnyRoot.cer`
3. 安装后重新打开视频号

#### 代理无法连接

**问题**：微信浏览器无法连接到代理

**解决方案**：

1. 检查程序是否正在运行
2. 检查端口是否被占用：`netstat -ano | findstr :2025`（Windows）
3. 检查防火墙设置
4. 尝试更改端口：`wx_channel.exe -p 8080`

#### 提示证书错误

**问题**：访问视频号时提示证书错误

**解决方案**：

1. 确认根证书已正确安装
2. 重新打开视频号
3. 清除浏览器缓存
4. 检查系统时间是否正确

更多问题请参考 故障排除。

### 下一步

* 配置概览 - 了解所有配置选项
* 使用指南 - 学习如何使用程序
* 故障排除 - 解决遇到的问题
