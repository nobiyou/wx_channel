package app

const releaseHighlightsVersion = "5.7.11"

var releaseHighlights = [...]string{
	"Gopeed 下载统一 - 单个原始视频与批量任务统一交由 Gopeed 后端处理",
	"页面直连退役 - 原始视频不再由微信页面 fetch，减少直连超时和不稳定问题",
	"原始视频保护 - 原始模式继续单连接并保留源文件大小校验，拦截低码率流",
	"请求链路稳定 - 后端保留来源 Referer、Origin 和 User-Agent，使用完整请求上下文",
	"下载结果一致 - 前端统一提交本地下载接口，进度、命名和历史记录走同一链路",
}
