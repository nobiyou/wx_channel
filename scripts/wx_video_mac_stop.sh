#!/usr/bin/env bash
# macOS 一键停止微信视频号下载助手：结束进程 → 关闭系统代理
# 配合 wx_video_mac_download.sh 使用；在其被强杀/终端意外关闭、代理残留时也可单独执行
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/wx_channel_mac"
PORT=${1:-2025}   # 与启动脚本 -p 保持一致，用法: wx_video_mac_stop.sh [端口]

if pids=$(pgrep -f "(^|/)$(basename "$BIN")( |$)"); then
  echo "==> 结束进程: $pids"
  kill $pids
  for _ in 1 2 3 4 5; do pgrep -f "(^|/)$(basename "$BIN")( |$)" >/dev/null || break; sleep 1; done
  pgrep -f "(^|/)$(basename "$BIN")( |$)" >/dev/null && { echo "==> 进程未退出，强制结束"; kill -9 $pids; }
else
  echo "==> 未发现运行中的 $(basename "$BIN")"
fi

# 关闭所有网络服务上指向 127.0.0.1:$PORT 的 HTTP/HTTPS 代理（只碰本工具设的，不动 Clash 等）
networksetup -listallnetworkservices | tail -n +2 | sed 's/^\*//' | while IFS= read -r svc; do
  for kind in webproxy securewebproxy; do
    if [[ $(networksetup -get$kind "$svc" 2>/dev/null | grep -cx -e "Enabled: Yes" -e "Server: 127.0.0.1" -e "Port: $PORT") -eq 3 ]]; then
      networksetup -set${kind}state "$svc" off
      echo "==> 已关闭 $svc 的 $kind"
    fi
  done
done
echo "==> 完成"
