# 微信视频号下载助手

<p align="center">
  <a href="https://github.com/nobiyou/wx_channel/releases"><img src="https://img.shields.io/github/v/release/nobiyou/wx_channel?style=flat-square&label=Version" alt="Release"></a>
  <a href="https://github.com/nobiyou/wx_channel/releases"><img src="https://img.shields.io/github/release-date/nobiyou/wx_channel?style=flat-square&label=Released" alt="Release Date"></a>
  <img src="https://img.shields.io/badge/Go-1.23+-00ADD8.svg?style=flat-square&logo=go">
  <img src="https://img.shields.io/badge/Platform-Windows-lightgrey.svg?style=flat-square">
  <img src="https://img.shields.io/github/license/nobiyou/wx_channel?style=flat-square" alt="License">
  <a href="https://github.com/nobiyou/wx_channel/stargazers"><img src="https://img.shields.io/github/stars/nobiyou/wx_channel?style=flat-square" alt="Stars"></a>
</p>

<p align="center">
  <b>一键下载微信视频号所有页面视频，支持批量下载、加密视频解密、自动去重</b>
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
- ✅ **无感雷达**：默认关闭，通过 `config.yaml` 中的 `radar_enabled` 显式启用
- ✅ **完整记录**：自动记录所有下载信息，便于管理
- ✅ **命名模板**：支持通过 `download_filename_template` 自定义批量下载文件名

---

## 🎬 效果演示

![主界面](assets/jietu.png)
修复wx视频号新版本界面，加了我的搜藏点赞的下载。

### Web 控制台界面

访问 `http://localhost:2025/console` 使用 Web 控制台：

- **浏览记录**：查看和管理所有浏览过的视频
- **下载记录**：查看历史下载记录和统计
- **下载队列**：实时管理下载任务
- **批量下载**：批量提交下载任务
- **数据导出**：导出 JSON/CSV 格式数据

| web控制台亮色 | web控制台暗色 |
| --- | --- | 
|  ![亮色](assets/liang.png)  | ![暗色](assets/an.png)  |

> 💡 **提示**：更多功能说明请查看 [文档目录](docs/README.md) 和 [Web 控制台指南](docs/WEB_CONSOLE.md)

### Hub 联动

`wx_channel` 仍然支持连接独立 Hub，用于远程设备管理、同步推送和订阅协同：

- 客户端通过 `cloud_hub_url` 连接 Hub 的 `/ws/client`
- 可使用绑定命令把客户端接入 Hub 账号体系
- 本地配置中的 `hub_sync` 仍控制同步推送相关行为

Hub 服务端和 Hub 前端已经拆分到独立仓库：

- [wx_channel_hub](https://github.com/nobiyou/wx_channel_hub)

如果你需要部署 Hub，请在 `wx_channel_hub` 仓库中构建和运行，不再通过 `wx_channel` 仓库启动 `http://localhost:8080` 的 Hub 前端。


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

📖 **详细教程**：[使用文档](docs/README.md) | [开发文档](dev-docs/README.md) | [更新日志](CHANGELOG.md)

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
| **数据导出** | 支持JSON格式数据导出 |
| **评论列表** | 可通过本地 API 获取视频评论列表与回复分页 |

### 🎨 用户体验

| 功能 | 说明 |
|------|------|
| **Web 控制台** | 现代化界面，支持深色模式，响应式设计 |
| **浏览记录** | 查看所有浏览历史，支持搜索和筛选 |
| **下载队列** | 实时管理下载任务，支持批量操作 |
| **数据导出** | 支持 JSON/CSV 格式导出数据 |
| **实时日志** | 详细的操作日志，问题一目了然 |
| **进度显示** | 实时显示下载进度和状态 |

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

# 基本编译
go build -o wx_channel.exe

# 优化体积编译（推荐）
go build -ldflags="-s -w" -o wx_channel_mini.exe
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
WX_CHANNEL_DOWNLOAD_CONCURRENCY=5
WX_CHANNEL_DOWNLOAD_TIMEOUT=30
```

📖 **完整配置**：[配置文档](docs/CONFIGURATION.md)

---

## 📚 文档

### 📖 用户文档
所有使用相关的文档都在 `docs/` 目录：

- **快速开始**: [安装指南](docs/INSTALLATION.md) | [配置说明](docs/CONFIGURATION.md)
- **功能使用**: [批量下载](docs/BATCH_DOWNLOAD_GUIDE.md) | [监控功能](docs/MONITORING_QUICKSTART.md)
- **测试指南**: [前端测试](docs/FRONTEND_TEST_GUIDE.md)
- **故障排除**: [常见问题](docs/TROUBLESHOOTING.md)

📖 **查看所有用户文档**: [docs/INDEX.md](docs/INDEX.md)

### 🔧 开发文档
所有开发相关的文档都在 `dev-docs/` 目录：

- **修复历史**: [FIX_HISTORY.md](dev-docs/FIX_HISTORY.md) - 所有修复记录 ⭐
- **完整文档**: [DOCUMENTATION.md](dev-docs/DOCUMENTATION.md) - 项目完整文档
- **API 文档**: [api_documentation.md](dev-docs/api_documentation.md) - API 接口文档
- **优化记录**: WebSocket、超时、性能等优化文档

📖 **查看所有开发文档**: [dev-docs/INDEX.md](dev-docs/INDEX.md)

### 快速入门
- [安装指南](docs/INSTALLATION.md) - 详细的安装步骤
- [项目介绍](docs/INTRODUCTION.md) - 功能特性和工作原理
- [故障排除](docs/TROUBLESHOOTING.md) - 快速解决问题

### 进阶功能
- [批量下载](docs/BATCH_DOWNLOAD_GUIDE.md) - 批量下载完整指南
- [Web 控制台](docs/WEB_CONSOLE.md) - Web 界面使用指南（推荐）
- [API 文档](docs/API_README.md) - HTTP API 接口
- [API 快速开始](docs/API_QUICK_START.md) - API 快速上手

### 开发文档
- [构建指南](docs/BUILD.md) - 从源码构建
- [配置说明](docs/CONFIGURATION.md) - 所有配置选项
- [更新日志](CHANGELOG.md) - 版本更新记录
- [技术文档](dev-docs/README.md) - 更多开发文档

---

## 🎉 最新版本 v5.6.9

### 🚀 核心升级（v5.6.9）

本次补丁版本聚焦评论导出稳定性，重点解决“已经导出成功但页面还报失败”以及长评论任务容易被自动刷新打断的问题：

- **分段保存进度**：评论导出过程新增 `.partial.json` 检查点文件，中途中断后仍可保留已抓到的评论结果。
- **避免刷新打断**：导出期间临时锁住页面 15 分钟自动刷新，减少长评论列表任务被意外重置。
- **误报修复**：后端成功导出后，不再因为前端状态更新异常弹出 `获取评论列表失败<Failed to fetch>`。
- **成功提示更清晰**：导出完成后会直接展示一级评论数、回复数和合计条数，便于快速核对。
- **版本展示同步**：启动横幅、README、版本文档、About 页面与 Windows 文件版本信息统一更新到本次发布。

### ✨ 主要特性

- **稳定下载**: 集成 Gopeed 引擎，支持高并发、断点续传与元数据自动修正。
- **智能管理**: 完备的后台管理系统，支持设备远程控制、任务实时监控与用户权限管理。
- **多端适配**: 响应式设计，完美支持 PC、平板与手机端访问。


## 💖 支持项目

如果这个项目对你有帮助，欢迎：

- ⭐ 给项目点个 Star
- 🐛 提交 Bug 报告和功能建议
- 📖 完善文档和教程
- 💰 赞赏支持开发

### 赞赏支持

<img src="assets/zanshang.png" width="300" alt="赞赏码">

### 赞赏名单

感谢以下用户的支持：

| 日期       | 昵称      | 金额 | 留言                     |
| ---------- | --------- | ---- | ------------------------ |
| 2025-09-30 | 潘*君 | ￥5.00   | 未留言 |
| 2025-10-12 | 三*家 | ￥5.00   | 请大佬喝杯饮料 |
| 2025-10-31 | wang***yu | ￥1.00   | 真棒 |
| 2025-11-01 | 倪*孔 | ￥20.00   | 自动下载增加暂停？已下载跳过？ |
| 2025-11-03 | 清***工作室 | ￥1.00   | 你可是太牛逼了 |
| 2025-11-05 | 李*辰 | ￥5.00   | 有群吗 v:**** |
| 2025-11-10 | 我**我在 | ￥1.00   | 希望可以一键批量下载某视频号特定时间范围内的所有视频 |
| 2025-11-17 | 方* | ￥100.00   | 加油，真心感谢您的付出，谢谢！ |
| 2025-11-19 | 匿名 | ￥10.00   | 非常给力。就是当版本不能用了可以发个提示啥的 |
| 2025-11-23 | 逆* | ￥5.00   | 好用 希望能坚持住 |
| 2025-11-29 | 保* | ￥18.80   | 未留言 |
| 2025-12-08 | 加*** | ￥18.80   | 感谢，很有用 |
| 2025-12-11 | v* | ￥1.00   | 膜拜到老 |
| 2025-12-23 | 麦* | ￥10.00   | 希望能出一个小程序视频下载的版本 |
| 2025-12-24 | 夜* | ￥1.00   | 谢谢大佬 |
| 2025-12-31 | 陈* | ￥10.00   | 点赞，加油哦，已经加星 |
| 2026-01-03 | 颀* | ￥1.00   | 今天第一次用，能不能做成docker? |
| 2026-01-05 | 土* | ￥5.00   | 感谢大佬 |
| 2026-01-07 | 空* | ￥20.00   | 厉害了大佬 |
| 2026-01-13 | 凌* | ￥1.00   | 牛逼 |
| 2026-01-18 | 漪* | ￥20.00   | 给大佬加杯咖啡 |
| 2026-01-20 | 匿名 | ￥10.00   | 问题反馈，找不到操作栏 |
| 2026-01-28 | 阿* | ￥18.88   | 加个微信 |
| 2026-01-30 | 杰* | ￥5.00   | 大佬请喝茶 |
| 2026-02-02 | ￥* | ￥20.00   | 超过200个视频会消耗完内存 |
| 2026-02-03 | 朱* | ￥10.00   | 很方便 |
| 2026-02-04 | 磐* | ￥1.00   | 未留言 |
| 2026-02-26 | 辰* | ￥20.00   | 非常厉害的工具！大佬继续努力！ |
| 2026-02-26 | 相* | ￥10.00   | 加油啊，！！！ |
| 2026-03-05 | G* | ￥10.00   | 请收下我的膝盖 |
| 2026-03-05 | 苏* | ￥5.00   | 未留言 |
| 2026-03-16 | 匿名 | ￥50.00   | 很好的工具，加我** |
| 2026-03-21 | 匿名 | ￥1.00   | 赞 |
| 2026-03-26 | 匿名 | ￥10.00   | 感谢大佬 |
| 2026-04-01 | 未* | ￥6.60   | 感谢 |
| 2026-04-05 | 陈* | ￥5.00   | 加个群 |
| 2026-04-13 | X* | ￥5.00   | 未留言 |
| 2026-04-13 | 匿名 | ￥10.00   | 大佬牛啊 |
| 2026-04-17 | Hou* | ￥1.00   | 好棒 |
| 2026-04-24 | ** | ￥18.88   | 未留言 |
| 2026-05-06 | 匿名 | ￥5.00   | 小白的我给你点赞 |
| 2026-05-08 | 温* | ￥5.00   | 好用 |
| 2026-05-13 | 剩* | ￥1.00   | 未留言 |
| 2026-05-14 | S* | ￥50.00   | 非常感谢，很有用！ |

> 💝 感谢每一位支持者！你们的支持是项目持续更新的动力。

---

## ⚠️ 免责声明

本工具仅供学习和研究使用。请遵守相关法律法规，尊重内容创作者的版权。使用本工具下载的内容请勿用于商业用途或非法传播。

---

## 📄 许可证

本项目采用 [MIT License](LICENSE) 许可证。

---

## 🙏 致谢

- 开发者说明：仓库已内置 `SunnyNet v1.0.3` 源码，位于 [pkg/sunnynet](/pkg/sunnynet:1)。根模块通过 `go.mod` 的 `replace` 指向本地版本，便于其他开发者直接构建与调试。
- [SunnyNet](https://github.com/qtgolang/SunnyNet) - HTTP/HTTPS 代理库
- [Go](https://golang.org/) - 编程语言
- 所有贡献者和支持者

---

## 📞 联系方式

- **GitHub Issues**：[提交问题](https://github.com/nobiyou/wx_channel/issues)
- **个人微信**：tutuixiu（备注：视频号下载）
- **项目地址**：https://github.com/nobiyou/wx_channel

### 交流群

<img src="assets/wxq.png" width="300" alt="微信交流群">

---

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=nobiyou/wx_channel&type=date&legend=top-left)](https://www.star-history.com/#nobiyou/wx_channel&type=date&legend=top-left)

<p align="center">
  <b>如果这个项目对你有帮助，请给个 ⭐ Star 支持一下！</b>
</p>

<p align="center">
  Made with ❤️ by <a href="https://github.com/nobiyou">nobiyou</a>
</p>
