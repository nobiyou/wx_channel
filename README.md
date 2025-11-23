# 微信视频号下载助手

<p align="center">
  <img src="https://img.shields.io/badge/Version-v5.0.0-blue.svg?style=flat-square">
  <img src="https://img.shields.io/badge/Go-1.23+-00ADD8.svg?style=flat-square&logo=go">
  <img src="https://img.shields.io/badge/Platform-Windows-lightgrey.svg?style=flat-square">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square">
  <a href="https://github.com/nobiyou/wx_channel/stargazers"><img src="https://img.shields.io/github/stars/nobiyou/wx_channel?style=flat-square" alt="Stars"></a>
</p>

<p align="center">
  <b>一键下载微信视频号视频，支持批量下载、加密视频解密、自动去重</b>
</p>

<p align="center">
  <a href="#-快速开始">快速开始</a> •
  <a href="#-核心功能">核心功能</a> •
  <a href="#-使用场景">使用场景</a> •
  <a href="#-文档">文档</a> •
  <a href="#-支持项目">支持项目</a>
</p>

---

## ✨ 为什么选择这个工具？

### 😫 你是否遇到过这些问题？

- ❌ 视频号视频无法直接下载保存
- ❌ 想批量下载某个作者的所有视频，但只能一个个点
- ❌ 加密视频下载后无法播放
- ❌ 需要保存视频做备份或二次创作
- ❌ 想离线观看喜欢的视频内容

### ✅ 这个工具帮你解决

- ✅ **一键下载**：点击即可下载，无需复杂操作
- ✅ **批量处理**：支持批量下载，一次搞定几十上百个视频
- ✅ **自动解密**：加密视频自动解密，下载即可播放
- ✅ **智能去重**：自动识别已下载视频，避免重复
- ✅ **完整记录**：自动记录所有下载信息，便于管理

---

## 🎬 效果演示

![主界面](jietu.png)

> 💡 **提示**：更多演示图片和视频请查看 [使用文档](docs/DOWNLOADMOVIE.md)

---

## 🚀 快速开始

### 三步开始使用

```bash
# 1️⃣ 下载程序
# 访问 https://github.com/nobiyou/wx_channel/releases 下载最新版本

# 2️⃣ 启动程序
wx_channel.exe

# 3️⃣ 打开视频号页面，点击下载按钮
# 就这么简单！
```

### 详细步骤

1. **下载并启动**
   - 从 [Releases](https://github.com/nobiyou/wx_channel/releases) 下载最新版本
   - 解压后双击 `wx_channel.exe` 启动

2. **安装证书**（首次使用）
   - 程序会自动尝试安装证书
   - 如果失败，手动安装 `downloads/SunnyRoot.cer`

3. **开始下载**
   - 打开微信视频号页面
   - 页面会自动注入下载按钮
   - 点击按钮即可下载

📖 **详细教程**：[安装指南](docs/INSTALLATION.md) | [使用教程](docs/DOWNLOADMOVIE.md)

---

## 🎯 核心功能

### 🎥 视频下载

| 功能 | 说明 |
|------|------|
| **单个下载** | 点击按钮即可下载当前视频 |
| **批量下载** | 一次下载多个视频，支持选择下载 |
| **加密视频** | 自动解密加密视频，下载即可播放 |
| **断点续传** | 大文件支持断点续传，不怕中断 |
| **智能去重** | 自动识别已下载视频，避免重复 |

### 📊 数据管理

| 功能 | 说明 |
|------|------|
| **自动分类** | 按作者自动创建文件夹，整理有序 |
| **下载记录** | CSV 格式记录所有下载信息 |
| **多格式导出** | 支持 TXT、JSON、Markdown 格式 |
| **评论采集** | 可选采集视频评论数据 |

### 🎨 用户体验

| 功能 | 说明 |
|------|------|
| **Web 控制台** | 微信风格界面，实时查看进度 |
| **实时日志** | 详细的操作日志，问题一目了然 |
| **进度显示** | 实时显示下载进度和状态 |
| **错误处理** | 自动重试，失败清单导出 |

---

## 💡 使用场景

### 📚 内容创作者

- 备份自己的视频号内容
- 下载素材用于二次创作
- 整理视频资料库

### 🎓 学习研究

- 下载教程视频离线学习
- 收集行业案例分析
- 保存学习资料

### 💼 企业团队

- 备份企业视频号内容
- 下载竞品分析素材
- 整理营销案例库

### 👤 个人用户

- 保存喜欢的视频内容
- 离线观看视频
- 整理收藏的视频

---

## 🆚 对比其他方案

| 特性 | 本工具 | 在线下载网站 | 其他软件 | 录屏软件 |
|------|--------|------------|----------|---------|
| **批量下载** | ✅ | ❌ | ⚠️ 有限 | ❌ |
| **加密视频** | ✅ 自动解密 | ❌ | ❌ | ⚠️ 画质损失 |
| **下载速度** | ✅ 快速 | ⚠️ 较慢 | ✅ 快速 | ❌ 很慢 |
| **隐私安全** | ✅ 本地运行 | ❌ 上传到服务器 | ⚠️ 依赖插件 | ✅ 本地 |
| **自动去重** | ✅ | ❌ | ❌ | ❌ |
| **下载记录** | ✅ CSV 记录 | ❌ | ❌ | ❌ |
| **使用成本** | ✅ 免费开源 | ⚠️ 可能收费 | ⚠️ 可能收费 | ⚠️ 软件费用 |

---

## 📦 安装方式

### 方式一：下载预编译版本（推荐）

1. 访问 [GitHub Releases](https://github.com/nobiyou/wx_channel/releases)
2. 下载最新版本的 `wx_channel.exe`
3. 解压后直接运行

### 方式二：从源码编译

```bash
# 克隆仓库
git clone https://github.com/nobiyou/wx_channel.git
cd wx_channel

# 编译
go build -ldflags="-s -w" -o wx_channel.exe
```

---

## ⚙️ 配置选项

### 基础配置

```bash
# 修改代理端口
wx_channel.exe -p 8080

# 查看版本
wx_channel.exe -v

# 卸载证书
wx_channel.exe --uninstall
```

### 环境变量

```bash
# 下载目录
WX_CHANNEL_DOWNLOADS_DIR=downloads

# 日志配置
WX_CHANNEL_LOG_FILE=logs/wx_channel.log
WX_CHANNEL_LOG_MAX_MB=5

# 并发配置
WX_CHANNEL_DOWNLOAD_CONCURRENCY=2
```

📖 **完整配置**：[配置文档](docs/CONFIGURATION.md)

---

## 📚 文档

### 快速入门
- [安装指南](docs/INSTALLATION.md) - 详细的安装步骤
- [使用教程](docs/DOWNLOADMOVIE.md) - 如何下载视频
- [常见问题](docs/COMMON_ISSUES.md) - 快速解决问题

### 进阶功能
- [批量下载](docs/BATCH_DOWNLOAD_GUIDE.md) - 批量下载完整指南
- [Web 控制台](docs/WEB_CONSOLE.md) - 使用 Web 界面
- [API 文档](docs/API.md) - HTTP API 接口

### 开发文档
- [构建指南](docs/BUILD.md) - 从源码构建
- [配置说明](docs/CONFIGURATION.md) - 所有配置选项
- [故障排除](docs/TROUBLESHOOTING.md) - 问题诊断

---

## 🎉 最新版本 v5.0.0

### 🆕 新增功能

- ✨ **批量下载系统**：完整的批量下载功能，支持多种数据格式
- 🔐 **加密视频支持**：自动解密加密视频，WASM 集成自动处理密钥
- 🎨 **Web 控制台**：微信风格界面，实时进度显示和日志
- 🔄 **重试机制**：网络问题自动重试，Context 超时控制
- 📖 **文档完善**：完整的用户文档和开发文档

### 🐛 问题修复

- 修复文件句柄泄漏问题
- 修复加密视频处理问题
- 修复网络中断恢复问题
- 优化用户体验

📝 **完整更新日志**：[版本更新说明](docs/RELEASE_NOTES.md)

---

## 💖 支持项目

如果这个项目对你有帮助，欢迎：

- ⭐ 给项目点个 Star
- 🐛 提交 Bug 报告和功能建议
- 📖 完善文档和教程
- 💰 赞赏支持开发

### 赞赏支持

<img src="zanshang.png" width="300" alt="赞赏码">

### 赞赏名单

感谢以下用户的支持：

| 日期       | 昵称      | 金额 | 留言                     |
| ---------- | --------- | ---- | ------------------------ |
| 2025-09-30 | 潘*君 | ￥5.00   | 未留言 |
| 2025-10-12 | 三*家 | ￥5.00   | 请大佬喝杯饮料 |
| 2025-10-31 | wang***yu | ￥1.00   | 真棒 |
| 2025-11-01 | 倪*孔 | ￥20.00   | 自动下载增加暂停？已下载跳过？（新版本已经解决详细看文档） |
| 2025-11-03 | 清***工作室 | ￥1.00   | 你可是太牛逼了 |
| 2025-11-05 | 李*辰 | ￥5.00   | 有群吗 v:****（个人微信：tutuixiu） |
| 2025-11-10 | 我**我在 | ￥1.00   | 希望可以一键批量下载某视频号特定时间范围内的所有视频（视频<br>号没有这样的接口，等视频号有这样的功能了我弄上，有其他疑问<br>可以看文档，文档内容还是很丰富的，整理了很久） |
| 2025-11-17 | 方* | ￥100.00   | 加油，真心感谢您的付出，谢谢！（感谢赞赏，功能也在慢慢的添<br>加中，目前在弄的是搜索批量下载和评论保存） |
| 2025-11-19 | 匿名 | ￥10.00   | 非常给力。就是当版本不能用了可以发个提示啥的。（具体可以提issues，描述具体些） |
| 2025-11-23 | 李*辰 | ￥5.00   | 好用 希望能坚持住（个人精力有限，不能保证，感谢大家的赞赏） |


> 💝 感谢每一位支持者！你们的支持是项目持续更新的动力。

---

## ⚠️ 免责声明

本工具仅供学习和研究使用。请遵守相关法律法规，尊重内容创作者的版权。使用本工具下载的内容请勿用于商业用途或非法传播。

---

## 📄 许可证

本项目采用 [MIT License](LICENSE) 许可证。

---

## 🙏 致谢

- [SunnyNet](https://github.com/qtgolang/SunnyNet) - HTTP/HTTPS 代理库
- [Go](https://golang.org/) - 编程语言
- 所有贡献者和支持者

---

## 📞 联系方式

- **GitHub Issues**：[提交问题](https://github.com/nobiyou/wx_channel/issues)
- **个人微信**：tutuixiu（备注：视频号下载）
- **项目地址**：https://github.com/nobiyou/wx_channel

---

<p align="center">
  <b>如果这个项目对你有帮助，请给个 ⭐ Star 支持一下！</b>
</p>

<p align="center">
  Made with ❤️ by <a href="https://github.com/nobiyou">nobiyou</a>
</p>
