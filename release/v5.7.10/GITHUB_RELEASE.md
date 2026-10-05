# wx_channel v5.7.10

发布日期：2026-10-05

## 公众号推送列表可靠性修复

- 从公众号文章页面实际发出的 `getmsg` 请求捕获最新 `biz`、`uin`、`key`、`pass_ticket` 和 Cookie，并按当前请求传递给本地服务，避免使用过期缓存凭证。
- 后端请求补齐微信桌面端请求头、Referer 和 Cookie，兼容 `general_msg_list` 与 `home_page_list` 返回格式，并支持文章页凭证通过 POST 传递。
- 监听文章异步渲染和页面切换，首次打开公众号文章即可显示下载和推送列表入口。
- 微信上游明确返回空列表时显示当前打开文章并标注来源；不会把当前文章伪装成历史推送。
- 注入 HTML 后清理旧压缩、长度和缓存响应头，避免 WebView 复用未注入的旧页面。

## 验证

- `node --test internal/assets/inject/officialaccount.test.js`
- `go test ./...`
- `git diff --check`

## 发布文件

- `wx_channel_v5.7.10.exe`
- `wx_channel_cloud_v5.7.10.exe`
- `wx_channel_radar_v5.7.10.exe`
