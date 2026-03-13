# 1. 彻底清除之前的 Linux 环境变量设置
$env:GOOS=""; $env:GOARCH=""; $env:CGO_ENABLED="1"; go build -mod=vendor -ldflags="-w -s -extldflags '-static'" -o wx_channel_cloud.exe main.go

# 2. 确保你在根目录 (wx_channel)
# 如果已经在根目录则忽略这一步
cd .. 

# 3. 再次执行 Windows 打包指令
go build -mod=vendor -ldflags="-w -s" -o wx_channel_cloud.exe main.go

# 4. 打包 Linux 服务器
cd hub_server
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; go build -o hub_server main.go