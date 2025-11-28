# 日志工具快速参考

## 🎯 最常用命令

```powershell
# 1️⃣ 实时查看日志（调试时必备）
.\scripts\view_logs.ps1 -Action tail

# 2️⃣ 生成统计报告
.\scripts\analyze_logs.ps1

# 3️⃣ 搜索特定内容
.\scripts\view_logs.ps1 -Action search -Pattern "关键词"

# 4️⃣ 查看错误
.\scripts\view_logs.ps1 -Action errors

# 5️⃣ 查看日志文件信息
.\scripts\clean_logs.ps1 -Action info

# 6️⃣ 归档旧日志（备份+清理）
.\scripts\clean_logs.ps1 -Action archive -KeepDays 7
```

## 📋 常用搜索关键词

| 关键词 | 说明 |
|--------|------|
| `下载记录` | 查看所有视频下载记录 |
| `评论采集` | 查看评论采集记录 |
| `批量下载` | 查看批量下载操作 |
| `系统启动` | 查看系统启动记录 |
| `系统关闭` | 查看系统关闭记录 |
| `ERROR` | 查看错误日志 |
| `失败` | 查看失败操作 |
| `Profile` | 查看Profile页面操作 |
| `Search` | 查看搜索页面操作 |
| `导出` | 查看导出操作 |

## 🔍 搜索示例

```powershell
# 查看今天下载的所有视频
.\scripts\view_logs.ps1 -Action search -Pattern "下载记录"

# 查看某个作者的视频
.\scripts\view_logs.ps1 -Action search -Pattern "闲聊北京"

# 查看批量下载操作
.\scripts\view_logs.ps1 -Action search -Pattern "批量下载"

# 查看评论采集
.\scripts\view_logs.ps1 -Action search -Pattern "评论采集"

# 查看Profile页面操作
.\scripts\view_logs.ps1 -Action search -Pattern "Profile"

# 查看导出操作
.\scripts\view_logs.ps1 -Action search -Pattern "导出动态"
```

## 💡 使用技巧

### 1. 调试新功能
启动程序后，开另一个终端实时查看日志：
```powershell
.\scripts\view_logs.ps1 -Action tail
```

### 2. 每日统计
每天结束时查看统计：
```powershell
.\scripts\analyze_logs.ps1
```

### 3. 问题排查
发现问题时先查看错误：
```powershell
.\scripts\view_logs.ps1 -Action errors
```

### 4. 保存报告
将统计结果保存到文件：
```powershell
.\scripts\analyze_logs.ps1 > daily_report.txt
```

### 5. 查看更多行
默认显示50行，可以增加：
```powershell
.\scripts\view_logs.ps1 -Action tail -Lines 200
```

## 📊 日志文件位置

- 主日志文件：`logs\wx_channel.log`
- 自动轮转：超过10MB自动创建新文件
- 保留数量：最近5个日志文件

## ⚡ 快捷键

- `Ctrl+C` - 退出实时查看模式
- `Ctrl+F` - 在终端中搜索（部分终端支持）

## 🎨 输出说明

- 🟢 绿色 - 正常信息
- 🟡 黄色 - 警告信息
- 🔴 红色 - 错误信息
- 🔵 蓝色 - 标题和分隔符

---

**提示**：将此文件保存为书签，方便随时查阅！
