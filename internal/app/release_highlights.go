package app

const releaseHighlightsVersion = "5.7.14"

var releaseHighlights = [...]string{
	"下载目录安全校验 - 阻止 Windows Junction 和符号链接跳出下载目录",
	"视频流句柄固定 - 校验与读取使用同一个受限文件句柄",
	"路径兼容性完善 - 支持合法内部跳转、根目录别名、长路径和 UNC 路径",
	"越界请求明确拒绝 - 解析失败不再回退词法路径继续访问",
	"跨平台回归验证 - 覆盖 Windows Junction、WSL 符号链接和 Go 1.26",
}
