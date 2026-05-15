#!/bin/bash

echo "================================"
echo "清理同步状态记录"
echo "================================"
echo ""

echo "警告: 此操作将删除所有同步状态记录"
echo "数据同步历史不会被删除"
echo ""
read -p "确认继续? (y/n): " confirm

if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
    echo "操作已取消"
    exit 0
fi

echo ""
echo "正在清理..."

# 检查 sqlite3 是否安装
if ! command -v sqlite3 &> /dev/null; then
    echo "[错误] 未安装 sqlite3"
    echo "请安装: sudo apt-get install sqlite3 (Ubuntu/Debian)"
    echo "或: brew install sqlite3 (macOS)"
    exit 1
fi

# 切换到 hub_server 目录
cd "$(dirname "$0")/.." || exit 1

if [ ! -f "hub_server.db" ]; then
    echo "[错误] 数据库文件不存在: hub_server.db"
    exit 1
fi

echo "删除同步状态记录..."
sqlite3 hub_server.db "DELETE FROM sync_status;"

if [ $? -eq 0 ]; then
    echo "[成功] 同步状态记录已清理"
    echo ""
    echo "请重启 Hub Server 以重新初始化同步状态"
else
    echo "[失败] 清理失败"
    exit 1
fi

echo ""
