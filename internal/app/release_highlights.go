package app

const releaseHighlightsVersion = "5.7.4"

var releaseHighlights = [...]string{
	"视频号生命周期 - 启动检测 Weixin.exe，微信运行时自动打开视频号页面",
	"页面主动恢复 - 90 秒健康阈值、退避重试和冷却状态避免恢复风暴",
	"定向命令传输 - 只向匹配的视频号页面发送刷新或导航命令",
	"刷新责任收敛 - 移除页面固定 15 分钟刷新，保留导出刷新锁",
	"Insight Edge 默认链路 - Cloud 版继续连接本地 Insight :18081",
}
