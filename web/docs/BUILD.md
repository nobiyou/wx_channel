# 构建和打包指南

## 概述

本文档详细说明如何从源码构建和打包微信视频号下载助手。

## 前置要求

### 必需工具

1. **Go 语言环境**
   - 版本：1.23 或更高
   - 下载地址：https://golang.org/dl/
   - 验证安装：`go version`

2. **Git**（可选，用于克隆仓库）
   - 下载地址：https://git-scm.com/downloads
   - 验证安装：`git --version`

3. **go-winres**（可选，用于修改 Windows 资源）
   - 安装命令：`go install github.com/tc-hib/go-winres@latest`
   - 用途：生成 Windows 可执行文件的图标、版本信息等

## 快速开始

### 1. 获取源码

```bash
# 方式 1：使用 Git 克隆
git clone https://github.com/nobiyou/wx_channel.git
cd wx_channel

# 方式 2：下载 ZIP 并解压
# 从 GitHub 下载源码 ZIP，解压后进入目录
```

### 2. 基本编译

```bash
# 最简单的编译方式
go build -o wx_channel.exe

# 编译完成后会生成 wx_channel.exe
```

### 3. 运行程序

```bash
# Windows
wx_channel.exe

# 或指定端口
wx_channel.exe -p 2025
```

## 高级构建选项

### 优化体积编译（推荐）

使用 `-ldflags` 参数可以显著减小可执行文件体积：

```bash
# 去除调试信息和符号表
go build -ldflags="-s -w" -o wx_channel.exe

# 说明：
# -s: 去除符号表
# -w: 去除 DWARF 调试信息
# 可以减小约 30-40% 的文件体积
```

### 添加版本信息

```bash
# 在编译时注入版本号和构建时间
go build -ldflags="-s -w -X main.Version=1.0.0 -X main.BuildTime=$(date +%Y%m%d%H%M%S)" -o wx_channel.exe
```

## Windows 资源配置

### 修改程序元数据

程序的图标、版本信息等存储在 `winres/winres.json` 文件中。

#### 1. 编辑配置文件

打开 `winres/winres.json`，修改以下内容：

```json
{
  "RT_VERSION": {
    "#1": {
      "0000": {
        "fixed": {
          "file_version": "1.0.0.0",      // 文件版本
          "product_version": "1.0.0.0"    // 产品版本
        },
        "info": {
          "0409": {
            "Comments": "视频号下载助手",
            "CompanyName": "nobiyou",     // 公司名称
            "FileDescription": "一个用于下载微信视频号内容的工具",
            "FileVersion": "1.0.0.0",
            "ProductName": "视频号下载助手",
            "ProductVersion": "1.0.0.0",
            "LegalCopyright": "© 2023-2025"
          }
        }
      }
    }
  }
}
```

#### 2. 修改图标

将新的图标文件（PNG 格式）放到 `winres/icon.png`，然后重新生成资源。

#### 3. 生成资源文件

```bash
# 安装 go-winres（首次使用）
go install github.com/tc-hib/go-winres@latest

# 生成 Windows 资源文件
go-winres make

# 这会生成 rsrc_windows_amd64.syso 文件
# 该文件会在编译时自动嵌入到可执行文件中
```

#### 4. 重新编译

```bash
# 生成资源文件后重新编译
go build -ldflags="-s -w" -o wx_channel.exe
```

### 资源文件说明

- `winres/winres.json`: 配置文件
- `winres/icon.png`: 程序图标（PNG 格式）
- `rsrc_windows_amd64.syso`: 生成的资源文件（自动生成，不要手动修改）

## 完整打包流程

### 标准打包流程

```bash
# 1. 清理旧文件
rm -f wx_channel.exe
rm -f rsrc_windows_*.syso

# 2. 更新依赖（可选）
go mod tidy
go mod download

# 3. 修改版本信息（如果需要）
# 编辑 winres/winres.json

# 4. 生成 Windows 资源
go-winres make

# 5. 编译程序
go build -ldflags="-s -w" -o wx_channel.exe

# 6. 验证编译结果
./wx_channel.exe --version
```

### 发布版本打包

创建一个完整的发布包：

```bash
# 1. 编译程序
go build -ldflags="-s -w" -o wx_channel.exe

# 2. 创建发布目录
mkdir -p release/wx_channel_v1.0.0

# 3. 复制必要文件
cp wx_channel.exe release/wx_channel_v1.0.0/
cp README.md release/wx_channel_v1.0.0/
cp -r docs release/wx_channel_v1.0.0/

# 4. 创建 ZIP 压缩包
cd release
zip -r wx_channel_v1.0.0.zip wx_channel_v1.0.0/

# 或使用 7-Zip（Windows）
# 7z a wx_channel_v1.0.0.zip wx_channel_v1.0.0/
```

## 自动化构建脚本

### Windows 批处理脚本

创建 `build.bat` 文件：

```batch
@echo off
echo ========================================
echo 微信视频号下载助手 - 构建脚本
echo ========================================
echo.

echo [1/4] 清理旧文件...
if exist wx_channel.exe del wx_channel.exe
if exist rsrc_windows_*.syso del rsrc_windows_*.syso

echo [2/4] 生成 Windows 资源...
go-winres make
if errorlevel 1 (
    echo 错误: 资源生成失败
    pause
    exit /b 1
)

echo [3/4] 编译程序...
go build -ldflags="-s -w" -o wx_channel.exe
if errorlevel 1 (
    echo 错误: 编译失败
    pause
    exit /b 1
)

echo [4/4] 验证编译结果...
wx_channel.exe --version

echo.
echo ========================================
echo 构建完成！
echo 输出文件: wx_channel.exe
echo ========================================
pause
```

### PowerShell 脚本

创建 `build.ps1` 文件：

```powershell
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "微信视频号下载助手 - 构建脚本" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

Write-Host "[1/4] 清理旧文件..." -ForegroundColor Yellow
Remove-Item -Path "wx_channel.exe" -ErrorAction SilentlyContinue
Remove-Item -Path "rsrc_windows_*.syso" -ErrorAction SilentlyContinue

Write-Host "[2/4] 生成 Windows 资源..." -ForegroundColor Yellow
go-winres make
if ($LASTEXITCODE -ne 0) {
    Write-Host "错误: 资源生成失败" -ForegroundColor Red
    exit 1
}

Write-Host "[3/4] 编译程序..." -ForegroundColor Yellow
go build -ldflags="-s -w" -o wx_channel.exe
if ($LASTEXITCODE -ne 0) {
    Write-Host "错误: 编译失败" -ForegroundColor Red
    exit 1
}

Write-Host "[4/4] 验证编译结果..." -ForegroundColor Yellow
.\wx_channel.exe --version

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "构建完成！" -ForegroundColor Green
Write-Host "输出文件: wx_channel.exe" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
```

运行脚本：

```powershell
# 允许执行脚本（首次使用）
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser

# 运行构建脚本
.\build.ps1
```

### Linux/macOS Shell 脚本

创建 `build.sh` 文件：

```bash
#!/bin/bash

echo "========================================"
echo "微信视频号下载助手 - 构建脚本"
echo "========================================"
echo ""

echo "[1/4] 清理旧文件..."
rm -f wx_channel.exe
rm -f rsrc_windows_*.syso

echo "[2/4] 生成 Windows 资源..."
go-winres make
if [ $? -ne 0 ]; then
    echo "错误: 资源生成失败"
    exit 1
fi

echo "[3/4] 编译程序（Windows 版本）..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o wx_channel.exe
if [ $? -ne 0 ]; then
    echo "错误: 编译失败"
    exit 1
fi

echo "[4/4] 显示文件信息..."
ls -lh wx_channel.exe

echo ""
echo "========================================"
echo "构建完成！"
echo "输出文件: wx_channel.exe"
echo "========================================"
```

运行脚本：

```bash
# 添加执行权限
chmod +x build.sh

# 运行构建脚本
./build.sh
```

## 构建优化

### 减小文件体积

1. **使用 ldflags**
   ```bash
   go build -ldflags="-s -w" -o wx_channel.exe
   ```

2. **使用 UPX 压缩**（可选）
   ```bash
   # 安装 UPX: https://upx.github.io/
   
   # 压缩可执行文件
   upx --best --lzma wx_channel.exe
   
   # 注意：UPX 压缩可能被某些杀毒软件误报
   ```

### 加快编译速度

1. **启用编译缓存**
   ```bash
   # Go 1.10+ 默认启用，无需配置
   go env GOCACHE
   ```

2. **并行编译**
   ```bash
   # 设置并行编译数量
   go build -p 8 -o wx_channel.exe
   ```

3. **使用本地模块缓存**
   ```bash
   # 预下载依赖
   go mod download
   ```

## 常见问题

### 问题 1：go-winres 命令不存在

**解决方案**：
```bash
# 安装 go-winres
go install github.com/tc-hib/go-winres@latest

# 确保 GOPATH/bin 在 PATH 中
# Windows PowerShell:
$env:PATH += ";$env:GOPATH\bin"

# Linux/macOS:
export PATH=$PATH:$(go env GOPATH)/bin
```

### 问题 2：编译时提示缺少依赖

**解决方案**：
```bash
# 下载所有依赖
go mod download

# 或清理并重新下载
go clean -modcache
go mod download
```

### 问题 3：资源文件未生效

**解决方案**：
```bash
# 1. 删除旧的资源文件
rm rsrc_windows_*.syso

# 2. 重新生成
go-winres make

# 3. 重新编译
go build -o wx_channel.exe
```

### 问题 4：交叉编译失败

**解决方案**：
```bash
# 确保设置了正确的环境变量
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o wx_channel.exe

# 注意：某些依赖可能需要 CGO，如果失败尝试在目标平台编译
```

## 版本管理

### 版本号规范

建议使用语义化版本号（Semantic Versioning）：

```
主版本号.次版本号.修订号

例如：1.0.0, 1.2.3, 2.0.0
```

### 更新版本号

需要在以下位置更新版本号：

1. **winres/winres.json**
   ```json
   "file_version": "1.2.0.0",
   "product_version": "1.2.0.0",
   "FileVersion": "1.2.0.0",
   "ProductVersion": "1.2.0.0"
   ```

2. **main.go**（如果有版本常量）
   ```go
   const Version = "1.2.0"
   ```

3. **README.md**
   - 更新版本号说明
   - 更新更新日志

## 发布检查清单

在发布新版本前，请确认：

- [ ] 更新了所有位置的版本号
- [ ] 更新了 README.md 和文档
- [ ] 运行了所有测试（如果有）
- [ ] 在 Windows 上测试了编译后的程序
- [ ] 检查了文件体积是否合理
- [ ] 验证了程序图标和版本信息
- [ ] 创建了 Git 标签（如果使用 Git）
- [ ] 准备了发布说明（Release Notes）

## 相关文档

- [安装指南](INSTALLATION.md) - 安装和配置说明
- [配置概览](CONFIGURATION.md) - 配置选项说明
- [故障排除](TROUBLESHOOTING.md) - 常见问题解决

## 参考资源

- Go 官方文档：https://golang.org/doc/
- go-winres 项目：https://github.com/tc-hib/go-winres
- UPX 压缩工具：https://upx.github.io/
- 语义化版本：https://semver.org/lang/zh-CN/
