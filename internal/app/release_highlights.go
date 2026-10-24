package app

const releaseHighlightsVersion = "5.7.13"

var releaseHighlights = [...]string{
	"视频回放分栏 - 个人主页批量下载分为视频和直播回放，默认显示视频",
	"分类操作独立 - 分页、全选、下载、导出和清空仅作用于当前分类",
	"下载状态保留 - 下载中关闭重开保留分类、勾选和进度，并补齐新采集数据",
	"日志启动保护 - 文件日志不可用时继续输出控制台日志，避免空指针崩溃",
	"自动化验证补齐 - CI 跟随 go.mod，覆盖 Windows 构建、Go 和 JavaScript 测试",
}
