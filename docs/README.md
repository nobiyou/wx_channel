# 文档目录

欢迎使用微信视频号下载助手文档。本文档将帮助您了解如何安装、配置和使用本工具。

## 📑 文档索引

完整的文档索引请查看：[INDEX.md](INDEX.md)

## 文档导航

### 🚀 基础文档

#### [介绍](INTRODUCTION.md)
了解微信视频号下载助手的基本信息：
- 什么是微信视频号下载助手
- 主要功能和特性
- 工作原理
- 适用场景
- 系统要求

#### [版本更新说明](RELEASE_NOTES.md)
查看版本更新记录：
- 最新版本 v5.0.0
- 新增功能和改进
- 问题修复
- 升级说明

#### [安装指南](INSTALLATION.md)
详细的安装和配置步骤：
- 安装方式（预编译版本/源码编译）
- 首次运行配置
- 证书安装
- 浏览器代理配置
- 自定义配置
- 升级和卸载

### ⚙️ 配置文档

#### [配置概览](CONFIGURATION.md)
了解如何配置微信视频号下载助手：
- 环境变量配置
- 命令行参数
- 证书配置
- 日志配置
- 安全配置
- UI 功能开关
- 高级配置选项
- 配置优先级和示例

#### [日志按钮配置](LOG_BUTTON_CONFIG.md)
日志按钮显示/隐藏配置：
- 配置方式
- 使用快捷键
- 日志面板功能
- 故障排除

### 📖 功能使用

#### [批量下载使用指南](BATCH_DOWNLOAD_GUIDE.md)
批量下载完整功能说明：
- 快速开始
- 使用方法
- 数据格式
- 核心功能
- 常见问题

#### [API 文档](API.md)
HTTP API 接口说明：
- 视频下载 API
- 评论采集 API
- 进度查询
- 错误处理

#### [Web 控制台](WEB_CONSOLE.md)
浏览器控制台使用：
- 图形化界面
- 实时进度监控
- 批量任务管理
- 失败清单导出

#### [评论采集](COMMENT_CAPTURE.md)
评论采集功能使用说明：
- 功能介绍
- 使用方法
- 注意事项

#### [下载视频](DOWNLOADMOVIE.md)
视频下载功能使用说明：
- 单个下载
- 批量下载
- 下载记录
- 文件管理

### 🔧 开发文档

#### [构建打包](BUILD.md)
从源码构建和打包指南：
- 前置要求
- 编译步骤
- Windows 资源配置
- 自动化构建脚本
- 发布流程

#### [优化记录](OPTIMIZATION.md)
查看项目的优化建议、实施计划和变更记录。

### ❓ 帮助与支持

#### [故障排除](TROUBLESHOOTING.md)
详细的故障排除指南：
- 证书相关问题
- 代理连接问题
- 下载问题
- 配置问题
- 性能问题

#### [常见问题](COMMON_ISSUES.md)
快速查找常见问题的解决方案：
- 证书问题
- 代理问题
- 下载问题
- 配置问题
- 使用问题



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

当前版本：v5.0.0

查看版本信息：
```bash
wx_channel.exe -v
```

