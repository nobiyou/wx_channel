package app

const releaseHighlightsVersion = "5.7.12"

var releaseHighlights = [...]string{
	"原始地址优先 - 微信提供 fullUrl 时下载原始流，未提供时自动选择最高可用画质",
	"批量画质统一 - 单个与批量共用规格选择，按码率优先，缺失时沿用微信规格顺序",
	"签名参数保留 - 后端切换画质仅处理规格参数，保留其他签名参数的顺序与编码",
	"HTTP 400 回退 - 单个下载遇到 Gopeed Range 探测被拒绝时，由后端改用单流下载",
	"实际画质提示 - 明确区分原始视频与最高可用画质，原始模式继续保留大小校验",
}
