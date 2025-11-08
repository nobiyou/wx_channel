## 项目概览

本项目是一个基于 Go 与 SunnyNet 的本地 HTTP 代理工具，用于拦截微信视频号网页流量并进行脚本注入与本地交互：
- 注入前端脚本采集视频信息、触发下载、分片上传
- 本地提供 `__wx_channels_api/*` 接口用于信息展示、记录与保存视频
- 将下载记录保存为 CSV，并按作者/文件名组织本地文件

主要目录：
- `main.go`：入口、证书安装/卸载、代理启动、HTTP 回调与路由分发
- `internal/config`：配置（端口、目录、分片大小等）
- `internal/handlers`：API 接口（视频信息、上传、记录、导出）
- `internal/storage`：CSV 记录管理与去重
- `internal/utils`：日志、输出格式、文件名清理、时长/数字格式化
- `inject/main.js`：前端注入脚本（随 HTTP 相应注入）
- `lib/jszip.min.js`, `lib/FileSaver.min.js`：静态依赖，由本地接口返回


## 构建与运行

前置环境：Go 1.23+（toolchain 1.24.x 亦可）

### Windows 本地构建

```bash
# 基本打包
go build -o wx_channel.exe

# 优化体积的打包
go build -ldflags="-s -w" -o wx_channel_mini.exe
```


构建完成后，直接运行可执行文件：

```bash
./wx_channel.exe
```

可选参数：
- `--help` 显示帮助
- `-v, --version` 显示版本
- `-p, --port` 指定代理端口（默认 2025）
- `-d, --dev` 指定网络设备（macOS 代理设置用）
- `--uninstall` 卸载根证书并退出

首次运行会检测根证书（SunnyRoot.cer）：
- 若权限不足安装失败，程序会将证书写入 `downloads/SunnyRoot.cer`，可手动安装
- 安装后需重启浏览器；如需退出服务，使用 Ctrl+C

可选安全配置：
- 本地令牌：设置 `WX_CHANNEL_TOKEN=你的密钥`，前端/调用方需在请求头携带 `X-Local-Auth: 你的密钥`
- Origin 白名单：设置 `WX_CHANNEL_ALLOWED_ORIGINS=https://example.com,https://foo.bar`
  - 启用后，接口会回显 `Access-Control-Allow-Origin` 并支持 `POST, OPTIONS` 预检；允许头：`Content-Type, X-Local-Auth`
 - 并发与限流：
   - 分片并发上限 `UploadChunkConcurrency`（默认 4）
   - 合并并发上限 `UploadMergeConcurrency`（默认 1）

日志配置（可选）：
- 默认开启：日志写入 `logs/wx_channel.log`，支持大小滚动
- `WX_CHANNEL_LOG_FILE`：覆盖默认日志文件路径
- `WX_CHANNEL_LOG_MAX_MB`：单个日志文件最大大小（MB，默认 5）


## 运行时行为

- 代理端口：默认 `127.0.0.1:2025`（可通过参数或配置设置）
- 注入静态资源：
  - 命中包含 `jszip`/`FileSaver.min` 的路径时，直接返回 `lib/` 下本地文件
- 前端→本地 API（POST）：
  - `/__wx_channels_api/profile`：打印视频信息（昵称、标题、大小、时长、互动数据、封面、创建时间、IP 地区等）
  - `/__wx_channels_api/tip`：打印提示
  - `/__wx_channels_api/page_url`：记录并打印当前页面分享链接
  - `/__wx_channels_api/init_upload`：初始化分片上传，返回 `uploadId`
  - `/__wx_channels_api/upload_chunk`：接收分片，保存为 `downloads/.uploads/<uploadId>/<index>.part`
    - 可选校验：表单字段 `checksum` + `algo`（`md5`/`sha256`），`size` 指定期望字节数
  - `/__wx_channels_api/complete_upload`：合并分片为最终 mp4（合并前校验分片存在；按作者名分文件夹，冲突自动编号）
  - `/__wx_channels_api/upload_status`：查询已上传分片列表（用于断点续传）
  - `/__wx_channels_api/save_video`：直接保存整文件（非分片）
  - `/__wx_channels_api/record_download`：将下载信息写入 CSV（去重）
  - `/__wx_channels_api/export_video_list`：导出主页视频链接文本（服务端-基础字段）
  - `/__wx_channels_api/export_video_list_json`：导出主页视频链接为 JSON 文件（服务端-基础字段）
  - `/__wx_channels_api/export_video_list_md`：导出主页视频链接为 Markdown 文件（服务端-基础字段）
  - `/__wx_channels_api/batch_download_status`：打印批量下载进度
  - `/__wx_channels_api/batch_start`：启动批量下载任务（入参 videos: [{id,url,title,filename,authorName}]）
  - `/__wx_channels_api/batch_progress`：查询批量下载进度（返回 total/done/failed/running）
  - `/__wx_channels_api/batch_cancel`：取消批量下载
 - `/__wx_channels_api/batch_failed`：导出失败清单（JSON），返回导出文件路径
  - `/__wx_channels_api/batch_failed`：导出失败清单（JSON），返回导出文件路径


## 下载目录与记录

- 目录结构：
  - `downloads/` 根目录
  - `downloads/download_records.csv`（UTF-8 BOM，首行表头）
  - `downloads/.uploads/<uploadId>/000000.part ...`（分片临时目录）
  - `downloads/<作者（清理后）>/<文件名>.mp4`（最终文件，重名自动 `(...n)`）

文件名/目录将自动清理非法字符，并在缺失扩展名时补 `.mp4`。


## 后端批量下载与解密

- 选择清单提交
  - 注入面板支持“后端批量开始”“仅选中-后端下载”。提交时每条视频携带：
    - `id, url, title, filename, authorName`
    - 若视频含 key：前端生成 `decryptor_array`，取前 128 KiB 作为 `decryptorPrefix`（base64）并附带 `prefixLen`
- 服务端解密与保存
  - `batch_start` 接收 `decryptorPrefix/prefixLen` 后传入任务
  - Downloader 在下载时仅对前 `prefixLen` 字节做 XOR 解密，其余数据原样写入
  - 使用 `<path>.downloading` 临时文件与原子重命名，避免半成品
  - 进程内去重（按 `id` 优先，其次 `url`），同名非空文件直接跳过（避免重复副本）
- 进度与失败
  - 通过 `batch_progress` 查看 `{total, done, failed, running}`
  - 通过 `batch_failed` 导出失败清单 JSON，便于复盘与重试


## 前端批量下载（含解密）与选择下载

- 面板支持：
  - “编辑选择”展开选择列表（可勾选需要下载的视频）；每项展示：标题、封面、创建时间、时长、大小
  - “仅选中-前端下载”：前端解密 → 分片上传保存；可见进度；支持取消
  - “仅选中-后端下载”：将勾选清单提交给后端队列
  - “导出链接”支持多格式选择（前端导出，字段更丰富）：
    - TXT/JSON/Markdown 三种格式
    - 字段包含：标题、ID、URL、KEY、作者、时长、大小、点赞、评论、收藏、转发、创建时间、封面
  - “取消”按钮：面板中提供“取消”操作，可即时停止前端批量；后端批量亦提供“取消”按钮/接口
- 取消
  - 面板包含“取消”按钮：前端批量下载支持即时取消（AbortController），无需刷新；同时会通知后端 `batch_cancel`


## 打包试运行建议

1) 本地构建可执行文件并运行（建议以管理员身份运行，便于证书安装）
2) 浏览器配置使用本地 HTTP 代理 `127.0.0.1:<端口>`（或按提示页面引导）
3) 打开微信视频号相关页面，观察控制台提示与操作日志
4) 在 `downloads/` 下查看导出记录与下载的 mp4 文件


## 最新更新（v20251104）

### UI/UX 优化
- **状态信息栏**：替代浏览器原生 `alert`，提供更美观的提示体验
  - 支持多种消息类型（信息/成功/警告/错误），不同颜色区分
  - 使用半透明背景和柔和色调，视觉更舒适
  - 自动淡入淡出动画，支持自定义显示时长
- **自定义确认对话框**：替代浏览器原生 `confirm`
  - 深色主题，与整体UI风格统一
  - 支持点击遮罩或ESC键取消
  - 更友好的交互体验

### 功能增强
- 主页批量下载与前端取消（支持仅选中下载）
- 导出链接多格式：TXT / JSON / Markdown
- 后端批量下载：去重、失败清单、前缀解密
- 分片上传与并发限流优化
- 日志默认开启（5MB 滚动）


## 备注

- 首次试运行建议关注：证书安装提示、浏览器代理是否生效、控制台是否打印页面 URL 与视频信息、`downloads/` 是否生成 CSV 与 mp4。
- 如需我在上述改进中优先实现某几项，请指出优先级，我可以直接提交对应代码编辑。


