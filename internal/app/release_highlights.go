package app

const releaseHighlightsVersion = "5.7.3"

var releaseHighlights = [...]string{
	"Insight Edge 默认链路 - Cloud 版默认连接本地 Insight :18081，现有 cloud_hub_url 配置继续有效",
	"Hub 独有功能退役 - 移除设备绑定、浏览/下载记录主动同步和远端指标推送",
	"协议边界收敛 - 保留心跳、远程命令/API 调用、gzip 传输和页面能力状态",
	"本地监控保留 - Prometheus /metrics 继续由本机提供，不依赖 Hub 服务",
	"运行语义统一 - 配置样例、监控说明和启动日志统一指向 Insight Edge",
}
