# ltaoo 回复两页可行性探针

本探针只验证一条受限链路：读取一个作品的一级评论第一页，按源顺序选择第一个仍有未展开回复的根评论，读取回复第一页；仅当回复游标非空时，再读取回复第二页。它不运行完整采集器，不读取一级评论第二页，不选择第二个根评论，也不读取回复第三页。

## 安全边界

- 仅使用从已审核源码提交 `c0c2b8cc36af52ab2c3eb50cb7dc08b7d963efb0` 构建的 ltaoo Windows EXE。
- 本地 API 必须是 `http://127.0.0.1:<port>`；状态和 profile 最多各请求一次。
- 评论接口总请求最多三次：一级评论一次，回复最多两次。脚本不重试。
- 临时 CA 只安装到 `CurrentUser\Root`；ltaoo 配置固定为 `system: false`、`tun: false`、`skipInstallRootCert: true`。
- 探针不会修改 Clash、系统代理、WinHTTP、路由或 TUN。Clash 的临时规则及恢复由操作员管理。
- 分享链接、`oid/nid`、评论 ID、回复 ID 和原始游标只存在于进程内存。摘要只记录计数和哈希，不保存正文、昵称、账号、头像、Cookie、请求头、代理凭据或原始响应。
- 自动测试只访问 `httptest` 回环服务，不启动 ltaoo、不安装 CA、不修改 Clash。

## 1. 前置条件

1. PC 微信已登录，人工打开视频号页面正常。
2. 已从固定旧提交构建并核对 `wx_video_download.exe` 的 SHA-256。
3. 选择一个公开、一级评论中明显存在“展开更多回复”的作品。
4. 保留 Clash 当前配置的精确备份。ltaoo 进程自身必须直连，目标微信流量临时转发至 `127.0.0.1:2023`，避免代理回环。
5. 不把分享链接写入文件、提交、issue、日志或聊天记录。

## 2. 准备、记录基线并启动 ltaoo

在仓库根目录的 Windows PowerShell 5.1 中执行：

```powershell
$prepareJson = & powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File scripts/prepare-ltaoo-probe.ps1 `
  -LtaooExePath $ltaooExe

$run = $prepareJson | ConvertFrom-Json
$run
```

准备脚本显示本次 `run_id`、临时 CA 指纹、API 和代理端点。只有人工输入屏幕显示的精确确认文本才会安装 CA 并启动进程，例如：

```text
INSTALL 20260813-103000-a1b2c3d4e5f6
```

不得生成、代填或绕过 `INSTALL` 确认。安装范围只能是 `CurrentUser\Root`。

启动后打开或刷新目标视频号页面，让 ltaoo 页面 socket 初始化。若页面打不开、出现证书错误或 ltaoo 未就绪，立即进入清理，不运行回复探针。

### 状态证据

- 新版状态必须同时报告 API 和代理监听成功；摘要记录 `status_schema=modern`、`readiness_proof=listeners_and_profile`。
- 固定旧版的 `channels.available` 不作为就绪判据；非空版本证明旧结构，profile 成功和有效 `oid/nid` 作为页面 WebSocket 的最终证据；摘要记录 `status_schema=legacy`、`readiness_proof=profile`。
- 未知、残缺或监听失败的状态立即停止，不增加评论请求。

## 3. 临时配置 Clash

Clash 可以保持运行，但必须先备份当前实际配置，再加入本次运行专用的最小规则：

- ltaoo EXE 直连；
- 目标微信流量进入 `127.0.0.1:2023`；
- 不修改 Clash TUN、系统代理、WinHTTP 或系统路由；
- 规则必须与本次 `run_id` 绑定，便于精确删除；
- 热重载后先验证没有代理循环，再刷新微信页面。

探针脚本不会自动修改或恢复 Clash。无论结果如何，操作员都必须先恢复 Clash 的精确备份，再运行清理脚本。

## 4. 只验证一个根评论和最多两页回复

分享链接只读入当前 PowerShell 进程内存：

```powershell
$shareUrl = Read-Host 'Paste the selected public WeChat Channels share URL'
try {
  & powershell.exe -NoProfile -ExecutionPolicy Bypass `
    -File scripts/probe-ltaoo-replies.ps1 `
    -RunId $run.run_id `
    -ShareUrl $shareUrl
} finally {
  $shareUrl = $null
  Add-Type -AssemblyName System.Windows.Forms
  [System.Windows.Forms.Clipboard]::Clear()
  & powershell.exe -NoProfile -ExecutionPolicy Bypass `
    -File scripts/cleanup-ltaoo-probe.ps1 `
    -RunId $run.run_id
}
```

重要：真实操作中，必须在 `finally` 的清理脚本之前先恢复 Clash 精确备份；示例不自动完成这项外部恢复动作。

脚本固定执行：状态一次、profile 一次、一级评论第一页一次、回复第一页一次；回复第一页存在非空 `lastBuffer` 时，才以原始值标准 URL 编码后执行一次回复第二页。第二页返回的游标永远不会继续使用。

结果写入：

```text
.tmp_runtime/ltaoo-probe/<run_id>/reply-probe-summary.json
```

必须重点核对 `comment_request_count`、`reply_request_count`、`cursor_continuity`、重复计数和关系计数。原始分享链接、ID 和游标不得出现在摘要或终端输出中。

## 5. 结果解释

| 状态 | 精确请求数 | 含义 | 下一步 |
|---|---:|---|---|
| `verified_reply_two_pages` | 评论接口 3，回复 2 | 两页回复成功，第二次请求游标与第一页响应游标的带盐哈希一致，且无显式关系冲突 | 清理后，可设计独立的异常/重复游标探针 |
| `inconclusive_no_eligible_root` | 评论接口 1，回复 0 | 当前一级页没有“展开数大于有效内嵌数”的根评论 | 清理后换回复更多的公开样本和新 `run_id` |
| `inconclusive_no_second_reply_page` | 评论接口 2，回复 1 | 回复第一页可用，但没有第二页游标 | 清理后换回复更多的公开样本和新 `run_id` |
| `failed` | 以摘要计数为准，最多 3 | 状态、profile、结构、请求、游标、安全边界或显式关系失败 | 按 `reason_code` 诊断，不运行完整采集器 |

关系字段按不同语义处理：`rootCommentId` 是来源根，若存在、非空且不是 `"0"`，必须等于选定根评论 ID；显式冲突以 `reply_root_relation_mismatch` 立即失败。`replyCommentId` 是直接父项，可以指向选定根评论或任一已观察回复；指向受限窗口外对象时只增加 `parent_unresolved_count`，不会为了寻找父项增加请求。只有父项等于自身 `commentId` 时，才以 `reply_parent_self_reference` 立即失败。

每页和总计分别记录：`root_relation_match_count`、`root_relation_gap_count`、`root_relation_mismatch_count`、`parent_to_root_count`、`parent_to_known_reply_count`、`parent_unresolved_count`、`parent_gap_count`、`parent_self_reference_count`。兼容计数按以下方式汇总：

- `relation_match_count = root match + parent to root + parent to known reply`；
- `relation_gap_count = root gap + parent gap + parent unresolved`；
- `relation_mismatch_count = root mismatch + parent self reference`。

所有原始父 ID、根 ID 和回复 ID 只存在于进程内存；摘要只保留计数与既有带盐哈希。

## 6. 无论结果如何都清理

顺序固定：

1. 恢复 Clash 精确备份并确认临时规则消失；
2. 清空分享链接变量和剪贴板；
3. 用精确 `run_id` 执行 `cleanup-ltaoo-probe.ps1`；
4. 检查 `cleanup-receipt.json`；
5. 独立检查 CA、私钥、配置、进程、端口和 Clash 备份均无残留。

清理命令：

```powershell
& powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File scripts/cleanup-ltaoo-probe.ps1 `
  -RunId $run.run_id
```

必须确认：

```text
cleanup_success = true
ca_absent = true
private_key_absent = true
certificate_file_absent = true
config_absent = true
```

还要确认 ltaoo 进程不存在、`2022/2023` 不再监听、临时 Clash 规则与备份均已移除。任何残留都会使本次验收失败，即使 API 摘要为 `verified_reply_two_pages`。

本阶段通过只授权下一项独立验证，不授权运行或接入完整采集器。异常游标、去重和长列表中断仍需分别验证。
