# 版本更新说明

## 当前版本

- 版本号：`v5.7.4`
- 代码版本常量：`internal/version/version.go`
- 最新文档整理时间：`2026-08-24`

## 最新版本：v5.7.4（2026-08-25）

### 重点更新

- 启动时检测 `Weixin.exe`，微信未运行时只等待，不启动或结束微信进程。
- 微信运行时通过 `weixin://dl/channels` 打开视频号页面，并由 wx_channel 负责页面刷新与恢复。
- 新增 `runtime.lifecycle` 诊断、定向页面刷新/导航命令和有界退避恢复。
- 移除页面固定 15 分钟自动刷新，保留评论导出刷新锁和手动刷新能力。
- Cloud 版继续使用 Insight Edge 默认链路，现有配置兼容不变。

## 上一发布版本：v5.7.2（2026-08-23）

- 完成 Insight Edge 默认链路适配并退役停止开发的 Hub 独有功能。
- 收敛远程协议边界，保留现有配置兼容和本地监控能力。

## 详细记录

- 完整更新日志：[`../CHANGELOG.md`](../CHANGELOG.md)
- Web 端版本说明：[`../web/docs/RELEASE_NOTES.md`](../web/docs/RELEASE_NOTES.md)
