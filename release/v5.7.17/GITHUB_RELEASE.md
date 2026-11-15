# wx_channel v5.7.17

本版本完善直播切换、直播录制和运行状态展示，并改进 Cloud 官方登录页的状态同步。

## 主要更新

- 修复主页和直播列表切换直播时只更新流地址、未更新场次信息的问题；切换后重新捕获直播流、详情和下载状态。
- 从嵌套 `liveInfo`、`bizUserInfo`、`anchorContact` 和页面 DOM 补全直播 ID、标题、主播及状态。
- 下载目录使用真实主播名称，并按直播场次恢复下载状态。
- FFmpeg 断流或异常退出时，自动保存已有内容的录制片段；空文件继续报告失败。
- 恢复启动信息、版本说明和视频详情摘要日志，并增加直播详情摘要。
- 改进 Cloud 官方登录页的扫码等待、账号切换和连接状态同步。

## 下载包

- `wx_channel_v5.7.17.zip`：标准版
- `wx_channel_cloud_v5.7.17.zip`：Cloud 版
- `wx_channel_radar_v5.7.17.zip`：Radar 版
- `wx_channel_video_analysis_v5.7.17.zip`：标准版 + FFmpeg 视频分析工具

## 验证

- Go 全量测试：通过
- JavaScript 注入测试：106 项通过
- 三个 Windows 可执行文件版本信息：5.7.17
- ZIP 内可执行文件与独立 EXE 的 SHA-256：一致
- 发布包不包含运行时 `logs/`、`downloads/` 或 `config.yaml`
