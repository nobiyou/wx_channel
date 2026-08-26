package app

const releaseHighlightsVersion = "5.7.5"

var releaseHighlights = [...]string{
	"原始流识别修复 - 仅微信明确提供 fullUrl 时标记为原始视频",
	"最高画质回退 - 未提供原始流时自动选择 xWT111 等最高可用规格",
	"下载链路加固 - 保留完整签名 URL，页面直连失败自动回退后端",
	"低码率防误报 - 按源文件大小校验，拒绝把转码文件冒充原片",
	"界面提示对齐 - 明确区分原始视频与最高可用画质",
}
