package app

const releaseHighlightsVersion = "5.7.7"

var releaseHighlights = [...]string{
	"批量下载修复 - 无原始 fullUrl 时自动选择最高可用画质",
	"下载模式对齐 - 批量下载与单个下载共用原始流判定",
	"签名参数保持 - 回退到具体画质时保留完整 CDN 签名地址",
	"低码率防误保存 - 疑似转码流继续被大小校验拦截",
	"边界回归覆盖 - 覆盖原始流、最高画质和低码率大小边界",
}
