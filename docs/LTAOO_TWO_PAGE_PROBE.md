# ltaoo 两页评论可行性探针

本探针只回答一个问题：当前 PC 微信和 ltaoo 能否通过同一个 `lastBuffer` 连续取得两页一级评论。它不接入完整采集器、不请求回复、不请求第三页，也不保存评论正文或原始响应。

## 安全边界

- 必须使用从已审核 ltaoo 源码自行构建的 Windows EXE，不能使用本仓库中的 `wx_channel*.exe`。
- Clash 可以保持运行，但需要由操作员按照 ltaoo 官方协同方式手工把目标流量转发到 `127.0.0.1:2023`，并让 ltaoo 进程自身直连以避免代理回环。
- 探针生成的 ltaoo 配置固定为 `system: false`、`tun: false`、`skipInstallRootCert: true`。
- 探针不会修改 Clash、系统代理、WinHTTP 代理或路由。
- 临时 CA 只安装到 `CurrentUser\Root`。私钥只写入本次运行的 Git 忽略目录。
- 不要把视频号分享链接粘贴到 issue、提交信息、保留日志或聊天记录中。
- 自动测试不会安装 CA；只有准备脚本显示指纹并收到精确确认文本后，真实安装才会发生。

## 前置检查

1. 从已审核的 `ltaoo/wx_channels_download` 源码构建 `wx_video_download.exe`。
2. 登录 PC 微信，确认可以人工打开视频号页面。
3. 在 Clash 中配置 ltaoo 官方转发规则，保持 Clash TUN 与 ltaoo TUN 不同时启用。
4. 选择一个明显超过一页公开评论的作品，但暂时不要把链接写入文件。
5. 在仓库根目录打开 Windows PowerShell 5.1。

## 1. 准备、记录基线并启动 ltaoo

把下面 EXE 路径替换为实际的源码构建产物：

```powershell
$prepareJson = & powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File scripts/prepare-ltaoo-probe.ps1 `
  -LtaooExePath 'D:\Tools\ltaoo-src\wx_video_download.exe'

$run = $prepareJson | ConvertFrom-Json
$run
```

脚本会显示本次 `run_id`、CA 指纹、API 和代理端点，并明确说明不会修改外部网络设置。只有输入屏幕上显示的精确文本才会继续，例如：

```text
INSTALL 20260812-103000-a1b2c3d4e5f6
```

确认后，脚本把本次公钥证书安装到 `CurrentUser\Root`，启动 ltaoo，并输出只包含非敏感运行身份的 JSON。

此时打开或刷新一个视频号页面，使 ltaoo 页面 socket 初始化。若页面打不开、出现证书错误或 ltaoo 未就绪，不要继续探针，直接执行清理。

## 2. 只验证一页和一次翻页

通过交互输入把分享链接只保存在当前 PowerShell 进程内存中：

```powershell
$shareUrl = Read-Host 'Paste the selected public WeChat Channels share URL'

& powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File scripts/probe-ltaoo-comments.ps1 `
  -RunId $run.run_id `
  -ShareUrl $shareUrl

$shareUrl = $null
```

脚本固定执行：状态检查、详情请求、评论第一页，以及最多一次使用第一页原始 `lastBuffer` 的第二页请求。没有重试循环，不会请求回复或第三页。

结果写入：

```text
.tmp_runtime/ltaoo-probe/<run_id>/probe-summary.json
```

摘要只包含计数、状态、长度和哈希，不包含分享链接、作品 ID、评论 ID、原始游标、昵称、正文、头像 URL 或原始响应。

### 状态协议兼容

探针识别两种 `/api/status` 结构：

- 新版必须同时报告 API 和代理监听成功，摘要记录 `status_schema=modern`、`readiness_proof=listeners_and_profile`。
- 已验证旧版的 `channels.available` 存在恒为 `false` 的实现缺陷。探针只用非空版本和该字段的存在确认旧版结构，再以共享页面 profile 成功和有效 `oid/nid` 证明页面 WebSocket 可用；摘要记录 `status_schema=legacy`、`readiness_proof=profile`。

未知或残缺状态结构立即以 `status_schema_error` 停止。新版监听失败以 `ltaoo_not_ready` 停止。两种模式都不会因为状态兼容而增加重试、回复请求或第三页请求。

## 3. 无论结果如何都清理

成功、不确定、失败、手工取消或准备后不再继续时，都运行：

```powershell
& powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File scripts/cleanup-ltaoo-probe.ps1 `
  -RunId $run.run_id
```

如果当前终端已经关闭，请从准备阶段屏幕记录或 `.tmp_runtime/ltaoo-probe/` 下的本次目录名恢复精确 `run_id`，不要猜测或使用通配符。

清理脚本只会：

- 在 PID、启动时间和 EXE SHA-256 同时匹配时停止本次 ltaoo；
- 按 manifest 中的精确指纹删除本次 `CurrentUser\Root` CA；
- 删除本次精确记录的证书文件、私钥和配置；
- 报告 Clash、代理、路由、证书集合和相关进程是否相对基线漂移。

它不会自动恢复或覆盖 Clash、系统代理、WinHTTP 或路由配置。

检查：

```text
.tmp_runtime/ltaoo-probe/<run_id>/cleanup-receipt.json
```

必须确认：

```text
cleanup_success = true
ca_absent = true
private_key_absent = true
certificate_file_absent = true
config_absent = true
```

`external_drift_warnings` 需要人工审阅，但警告本身不会触发自动恢复。若 `cleanup_success=false`，不要继续下一次测试；先处理清理失败。

清理可以用同一 `run_id` 再执行一次，第二次应是安全的幂等检查。

## 结果解释

| 状态 | 含义 | 下一步 |
|---|---|---|
| `verified_two_pages` | 两次评论请求成功，第二次请求游标与第一页响应游标的带盐哈希一致 | 可以开始设计 ltaoo 适配器与 nobiyou 分页/去重逻辑的接口 |
| `inconclusive_no_second_page` | 第一页成功但没有非空游标 | 清理后使用评论更多的作品和新的 `run_id` 重试 |
| `failed` | 某个安全、页面、接口、结构或游标闸门失败 | 清理并按 `reason_code` 诊断；不要接入完整采集器 |

任何状态都不能替代 `cleanup-receipt.json` 的成功清理证明。
