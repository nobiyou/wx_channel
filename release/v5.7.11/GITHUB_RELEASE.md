# wx_channel v5.7.11

发布日期：2026-10-06

## 原始视频统一使用 Gopeed 下载

- 单个原始视频不再优先通过微信页面 `fetch` 直连，统一提交本地 `/__wx_channels_api/download_video` 接口，由 Gopeed 后端负责下载。
- 移除页面直连视频的保存组件、浏览器抓流进度和页面端解密路径，降低页面会话、超时和浏览器内存压力带来的不稳定。
- 原始视频继续强制单连接，并保留后端源文件大小校验，疑似低码率流会被拒绝。
- 单个下载和批量下载统一进入 Gopeed 任务链路，沿用请求头、进度、文件命名、落盘和历史记录处理。

## 验证

- `node --test internal/assets/inject/download.normalize.test.js`
- `go test ./...`
- 三版本构建脚本成功，exe 内嵌版本为 `5.7.11`

## 发布文件

- `wx_channel_v5.7.11.exe`
- `wx_channel_cloud_v5.7.11.exe`
- `wx_channel_radar_v5.7.11.exe`
