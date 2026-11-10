#!/usr/bin/env bash
# macOS 一键启动微信视频号下载助手：编译 → 装证书 → 设系统代理 → 运行 → 退出时关代理
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/wx_channel_mac"
PORT=2025
REBUILD=0

usage() {
  cat <<USAGE
用法: $(basename "$0") [-p 端口] [-b] [-h]

  -p 端口   代理端口，默认 2025（控制台 http://127.0.0.1:端口/console）
  -b        强制重新编译
  -h        显示本帮助

流程: 编译 macOS 二进制 → 安装 SunnyNet 根证书(首次需 sudo) → 设置系统 HTTP/HTTPS 代理 → 启动
      之后打开微信视频号页面即可点击下载；Ctrl+C 退出时自动关闭系统代理
USAGE
}

while getopts "p:bh" opt; do
  case $opt in
    p) PORT=$OPTARG ;;
    b) REBUILD=1 ;;
    h) usage; exit 0 ;;
    *) usage >&2; exit 1 ;;
  esac
done

[[ "$(uname)" == Darwin ]] || { echo "仅支持 macOS" >&2; exit 1; }
command -v go >/dev/null || { echo "未找到 go，请先安装: https://go.dev/dl/" >&2; exit 1; }

# sunnynet 用 go:embed 引用 Windows DLL（已被 .gitignore 排除），mac 上用空文件占位即可编译
for arch in win32 x64; do
  dll="$ROOT/pkg/sunnynet/Resource/nfapi/dll/$arch/nfapi.dll"
  [[ -f "$dll" ]] || { mkdir -p "$(dirname "$dll")"; : > "$dll"; }
done

if [[ $REBUILD == 1 || ! -x "$BIN" ]]; then
  echo "==> 编译 $BIN"
  # -tags libsqlite3: 用系统 libsqlite3，避免 mattn/go-sqlite3 与 gopeed 依赖的 crawshaw 各自内置的 sqlite3.c 在 ld64 上重复定义
  (cd "$ROOT" && go build -tags libsqlite3 -ldflags="-s -w" -o "$BIN" .)
fi

# 证书先装进系统钥匙串，程序启动时就无需 root
if ! security find-certificate -c SunnyNet /Library/Keychains/System.keychain >/dev/null 2>&1; then
  echo "==> 安装 SunnyNet 根证书（需要管理员密码）"
  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$ROOT/internal/assets/certs/SunnyRoot.cer"
fi

# 当前联网的网络服务名（Wi-Fi / Ethernet ...）
iface=$(route -n get default 2>/dev/null | awk '/interface:/{print $2}')
SERVICE=$(networksetup -listallhardwareports | awk -v i="$iface" '/^Hardware Port:/{p=substr($0,16)} /^Device:/ && $2==i{print p; exit}')
SERVICE=${SERVICE:-Wi-Fi}

proxy_off() {
  networksetup -setwebproxystate "$SERVICE" off
  networksetup -setsecurewebproxystate "$SERVICE" off
  echo "==> 已关闭 $SERVICE 的系统代理"
}
trap proxy_off EXIT

echo "==> 设置 $SERVICE 系统代理 -> 127.0.0.1:$PORT"
networksetup -setwebproxy "$SERVICE" 127.0.0.1 "$PORT"
networksetup -setsecurewebproxy "$SERVICE" 127.0.0.1 "$PORT"

echo "==> 启动，控制台: http://127.0.0.1:$PORT/console  (Ctrl+C 退出)"
cd "$ROOT" && "$BIN" -p "$PORT"
