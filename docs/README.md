# 文档目录

欢迎使用微信视频号下载助手文档。本文档将帮助您了解如何安装、配置和使用本工具。

## 文档导航

### 基础文档

#### [介绍](./INTRODUCTION.md)

了解微信视频号下载助手的基本信息：
- 什么是微信视频号下载助手
- 主要功能
- 工作原理
- 适用场景
- 系统要求

#### [安装指南](./INSTALLATION.md)

详细的安装和配置步骤：
- 安装方式（预编译版本/源码编译）
- 首次运行配置
- 证书安装
- 浏览器代理配置
- 自定义配置
- 升级和卸载

### 配置文档

#### [配置概览](./CONFIGURATION.md)

了解如何配置微信视频号下载助手，包括：
- 环境变量配置
- 命令行参数
- 证书配置
- 日志配置
- 安全配置
- 高级配置选项
- 配置优先级和示例

### 帮助与支持

#### [故障排除](./TROUBLESHOOTING.md)

详细的故障排除指南：
- 证书相关问题
- 代理连接问题
- 下载问题
- 配置问题
- 性能问题

#### [常见问题](./COMMON_ISSUES.md)

快速查找常见问题的解决方案：
- 证书问题
- 代理问题
- 下载问题
- 配置问题
- 使用问题

### 开发者文档

#### [优化建议](./OPTIMIZATION.md)

查看项目的优化建议、实施计划和变更记录。

## 快速开始

1. **下载程序**
   - 从 [GitHub 仓库](https://github.com/nobiyou/wx_channel) 下载最新版本
   - 或使用 `go build` 自行编译

2. **运行程序**
   ```bash
   wx_channel.exe
   ```

3. **配置浏览器代理**
   - 设置 HTTP 代理为 `127.0.0.1:2025`
   - 或按照程序启动时的提示操作

4. **安装证书**
   - 程序会自动尝试安装证书
   - 如果失败，请手动安装 `downloads/SunnyRoot.cer`

5. **开始使用**
   - 打开微信视频号页面
   - 使用注入的前端面板进行下载

## 常见问题

### 如何更改代理端口？

使用命令行参数：
```bash
wx_channel.exe -p 8080
```

或设置环境变量：
```bash
$env:WX_CHANNEL_PORT=8080
wx_channel.exe
```

### 如何启用安全认证？

设置环境变量：
```bash
$env:WX_CHANNEL_TOKEN="your_secret_token"
wx_channel.exe
```

### 日志文件在哪里？

默认位置：`logs/wx_channel.log`

可以通过环境变量自定义：
```bash
$env:WX_CHANNEL_LOG_FILE="custom_path.log"
wx_channel.exe
```

### 如何卸载证书？

```bash
wx_channel.exe --uninstall
```

## 获取帮助

- **GitHub Issues**: [提交问题](https://github.com/nobiyou/wx_channel/issues)
- **项目地址**: https://github.com/nobiyou/wx_channel

## 版本信息

当前版本：v20251108

查看版本信息：
```bash
wx_channel.exe -v
```

