# 日志工具索引

## 📚 文档导航

| 文档 | 说明 |
|------|------|
| [README.md](README.md) | 完整的工具使用说明 |
| [QUICK_REFERENCE.md](QUICK_REFERENCE.md) | 快速参考卡片 |
| [INDEX.md](INDEX.md) | 本文档 - 工具索引 |

## 🛠️ 工具列表

### 1. analyze_logs.ps1 - 日志分析工具

**功能**：生成完整的日志统计报告

**使用**：
```powershell
.\scripts\analyze_logs.ps1
```

**输出**：
- 基本统计（总日志条数）
- 系统事件（启动/关闭）
- 页面访问统计
- 下载统计
- 数据采集统计
- 错误统计
- 最近活动

**适用场景**：
- 每日工作总结
- 生成使用报告
- 了解系统整体运行情况

---

### 2. view_logs.ps1 - 日志查看工具

**功能**：多功能日志查看，支持实时监控、搜索、统计

**使用**：
```powershell
# 实时查看
.\scripts\view_logs.ps1 -Action tail

# 搜索
.\scripts\view_logs.ps1 -Action search -Pattern "关键词"

# 快速统计
.\scripts\view_logs.ps1 -Action stats

# 查看错误
.\scripts\view_logs.ps1 -Action errors
```

**参数**：
- `-Action`: 操作类型（tail/search/stats/errors）
- `-Pattern`: 搜索关键词（search时必需）
- `-Lines`: 显示行数（默认50）

**适用场景**：
- 实时调试
- 搜索特定操作
- 快速查看统计
- 问题排查

---

### 3. clean_logs.ps1 - 日志清理工具

**功能**：管理日志文件，支持备份、清理、归档

**使用**：
```powershell
# 查看信息
.\scripts\clean_logs.ps1 -Action info

# 备份
.\scripts\clean_logs.ps1 -Action backup

# 清理
.\scripts\clean_logs.ps1 -Action clean -KeepDays 7

# 归档（推荐）
.\scripts\clean_logs.ps1 -Action archive -KeepDays 7
```

**参数**：
- `-Action`: 操作类型（info/backup/clean/archive）
- `-KeepDays`: 保留天数（默认7天）

**适用场景**：
- 定期清理旧日志
- 备份重要日志
- 释放磁盘空间
- 日志归档管理

---

## 🎯 快速开始

### 新手推荐流程

1. **启动程序后，实时查看日志**
   ```powershell
   .\scripts\view_logs.ps1 -Action tail
   ```

2. **每天结束时，生成统计报告**
   ```powershell
   .\scripts\analyze_logs.ps1
   ```

3. **每周清理一次旧日志**
   ```powershell
   .\scripts\clean_logs.ps1 -Action archive -KeepDays 7
   ```

### 问题排查流程

1. **先查看错误日志**
   ```powershell
   .\scripts\view_logs.ps1 -Action errors
   ```

2. **搜索相关操作**
   ```powershell
   .\scripts\view_logs.ps1 -Action search -Pattern "关键词"
   ```

3. **实时监控新日志**
   ```powershell
   .\scripts\view_logs.ps1 -Action tail
   ```

---

## 📊 日志类型说明

### 系统日志
- `[系统启动]` - 服务启动
- `[系统关闭]` - 服务关闭
- `[配置加载]` - 配置文件加载

### 页面日志
- `[页面加载]` - 页面访问（Feed/Home/Profile/Search）
- `[页面快照]` - 页面快照保存

### 视频日志
- `[视频信息]` - 视频信息获取
- `[视频详情]` - 视频详情API拦截
- `[视频播放]` - 视频播放器加载

### 下载日志
- `[下载记录]` - 视频下载完成
- `[下载封面]` - 封面下载
- `[格式下载]` - 特定格式下载

### 数据日志
- `[评论采集]` - 评论数据采集
- `[评论保存]` - 评论数据保存
- `[CSV操作]` - CSV文件操作

### Profile页日志
- `[Profile视频采集]` - 视频列表采集
- `[Profile批量下载]` - 批量下载操作

### 搜索页日志
- `[搜索页面]` - 搜索页面加载
- `[搜索关键词]` - 搜索关键词记录

### 导出日志
- `[导出动态]` - 导出功能（TXT/JSON/Markdown）

---

## 💡 使用技巧

### 1. 组合使用
```powershell
# 先生成报告，再查看详细信息
.\scripts\analyze_logs.ps1
.\scripts\view_logs.ps1 -Action search -Pattern "下载"
```

### 2. 保存输出
```powershell
# 保存统计报告
.\scripts\analyze_logs.ps1 > report.txt

# 保存搜索结果
.\scripts\view_logs.ps1 -Action search -Pattern "下载" > downloads.txt
```

### 3. 定期维护
建议每周执行一次归档：
```powershell
.\scripts\clean_logs.ps1 -Action archive -KeepDays 7
```

### 4. 调试模式
开发或调试时，保持实时查看：
```powershell
.\scripts\view_logs.ps1 -Action tail -Lines 100
```

---

## 🔗 相关链接

- [完整使用说明](README.md)
- [快速参考](QUICK_REFERENCE.md)
- [日志系统总结](../日志系统完成总结.md)

---

**提示**：建议将 [QUICK_REFERENCE.md](QUICK_REFERENCE.md) 保存为书签，方便日常使用！
