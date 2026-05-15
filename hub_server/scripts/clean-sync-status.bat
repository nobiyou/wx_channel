@echo off
chcp 65001 >nul
echo ================================
echo 清理同步状态记录
echo ================================
echo.

echo 警告: 此操作将删除所有同步状态记录
echo 数据同步历史不会被删除
echo.
set /p confirm="确认继续? (y/n): "

if /i not "%confirm%"=="y" (
    echo 操作已取消
    pause
    exit /b
)

echo.
echo 正在清理...

where sqlite3 >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 未安装 sqlite3
    echo 请从 https://www.sqlite.org/download.html 下载安装
    pause
    exit /b 1
)

cd ..
if not exist "hub_server.db" (
    echo [错误] 数据库文件不存在: hub_server.db
    pause
    exit /b 1
)

echo 删除同步状态记录...
sqlite3 hub_server.db "DELETE FROM sync_status;"

if %errorlevel% equ 0 (
    echo [成功] 同步状态记录已清理
    echo.
    echo 请重启 Hub Server 以重新初始化同步状态
) else (
    echo [失败] 清理失败
)

echo.
pause
