package app

const releaseHighlightsVersion = "5.7.17"

var releaseHighlights = [...]string{
	"直播切换 - 检测流地址变化并刷新当前直播流、详情和下载状态",
	"直播详情 - 读取嵌套数据和页面 DOM，补全直播 ID、标题、主播和状态",
	"断流保存 - FFmpeg 异常断流时自动保留已有录制内容",
	"下载状态 - 按直播场次恢复状态并使用真实主播目录",
	"Cloud 登录同步 - 改进官方登录页扫码等待、账号切换和连接状态",
}
