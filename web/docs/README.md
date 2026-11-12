# 文档目录

微信视频号下载助手文档。

## 📑 文档索引

完整索引：[INDEX.md](INDEX.md)

## 文档导航

### 基础文档
- [介绍](INTRODUCTION.md) - 项目基本信息、功能特性、系统要求
- [安装指南](INSTALLATION.md) - 安装配置、证书安装、代理配置
- [版本说明](RELEASE_NOTES.md) - 版本更新记录

### 配置文档
- [配置说明](CONFIGURATION.md) - 环境变量、命令行参数、高级配置

### 功能使用
- [批量下载指南](BATCH_DOWNLOAD_GUIDE.md) - 批量下载完整功能说明
- [评论列表 API](COMMENT_CAPTURE.md) - 评论列表接口与使用方式
- [Web 控制台](WEB_CONSOLE.md) - 浏览器控制台使用

### 开发文档
- [API 文档](API.md) - HTTP API 接口说明
- [构建打包](BUILD.md) - 从源码构建和打包指南

### 帮助支持
- [故障排除](TROUBLESHOOTING.md) - 常见问题解决方案

## 快速开始

1. 下载程序或 `go build` 编译
2. 运行 `wx_channel.exe`
3. 设置浏览器代理为 `127.0.0.1:2025`
4. 安装证书 `downloads/SunnyRoot.cer`
5. 打开微信视频号页面开始使用

## 获取帮助

- GitHub Issues: [提交问题](https://github.com/nobiyou/wx_channel/issues)

---

当前版本：v5.7.16

最近更新：
- 修复直播详情页空白并支持 FFmpeg 下载直播回放
- 下载记录和导出数据保留点赞、评论、收藏和转发指标
- 移除无效的页面日志按钮，统一查看本地日志文件
- 新增 Codex 视频号 MCP，可查询本地记录和已就绪微信页面数据
- 新增下载优先的视频内容分析流程，支持抽帧、画面拆解和分镜复盘
- 播放量从 MCP 公共契约移除，不展示缺失播放量或补成 0
