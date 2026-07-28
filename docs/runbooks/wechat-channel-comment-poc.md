# 微信视频号公开评论采集 POC：隔离虚拟机运行手册

本手册仅适用于验证公开作品评论的只读采集能力。关键词固定为“青云装饰”，最多处理 10 个作品；每个作品最多保留 100 条一级评论和 200 条二级回复。允许视频、图文及其他公开作品。禁止点赞、评论、回复、发布、上传或验证码绕过。

## 1. 创建隔离基线

1. 创建可销毁的 Windows 虚拟机。不要挂载或复制宿主机微信数据目录、Cookie、证书或登录状态。
2. 在虚拟机中安装 PC 微信，完全由操作员人工登录。
3. 确认视频号可以访问；记录此时不存在 POC CA、`SunnyFilter2` 驱动和 POC 监听端口的基线。
4. 创建名为 `wechat-login-baseline` 的虚拟机快照。后续 POC 只复用该快照内的登录状态。

## 2. 取得并审计源码

1. 从受控 fork 检出 `codex/wechat-channel-comment-poc` 分支，核对批准的提交列表。
2. 准备官方 TDM-GCC 10.3.0。安装器 SHA-256 必须为：

   `819C7A1F74D45AD04E10662E1A2C3124D13D9A2BCA508847692251242CD455C3`

   只允许安装在虚拟机中，或解压到被 Git 忽略的 `.poc-tools/tdm-gcc-10.3.0-2/`；不要修改宿主机或系统全局 PATH。
3. 从源码构建并审计，不能运行仓库提交的任何 `wx_channel*.exe`：

   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-poc.ps1
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/poc-security-audit.ps1
   ```

4. 只运行刚生成的受限入口预检：

   ```powershell
   .\.poc-build\wx_channel_poc.exe preflight
   ```

   任一预检或审计失败都必须停止，不得进入 `run`。

## 3. 真实运行检查点

只有在用户看过开发证据并再次明确批准真实验证后，才能执行：

```powershell
.\.poc-build\wx_channel_poc.exe run --ack-isolated-vm
```

程序显示的变更摘要只能包含 CurrentUser 证书范围、两个回环端口和 `WeChatAppEx.exe`。核对后由操作员输入精确文本 `APPLY`。

- 登录、真人验证或目标页面切换只能人工完成，不使用验证码识别、UI 自动化或规避手段。
- 默认等待 300 秒。需要时输入 `EXTEND`，每个等待事件最多延长一次、固定 300 秒；输入 `CANCEL` 取消。
- 等待期间不发送微信请求。只读请求串行执行，固定至少间隔 1 秒。
- 不在终端、聊天或工单中粘贴作品 ID、账号、昵称、正文、IP 属地、URL、Token、Cookie 或证书信息。

## 4. 只查看状态和数量

下面的命令不打印 dataset 内容：

```powershell
$jobDir = Get-ChildItem -LiteralPath '.poc-data' -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
$manifest = Get-Content -LiteralPath (Join-Path $jobDir.FullName 'manifest.json') -Raw | ConvertFrom-Json
$validation = Get-Content -LiteralPath (Join-Path $jobDir.FullName 'validation.json') -Raw | ConvertFrom-Json
[pscustomobject]@{
    job_status = $manifest.status
    capability_status = $manifest.capability_status
    coverage_status = $manifest.coverage_status
    works = $manifest.counts.works
    top_level_comments = $manifest.counts.top_level_comments
    replies = $manifest.counts.replies
    cleanup_success = $manifest.cleanup_success
    field_reason_codes = @($validation.reason_codes)
} | ConvertTo-Json -Depth 4
```

少于 10 个作品但搜索明确耗尽时，`coverage_status` 应为 `source_exhausted_below_target`；这不等于采集能力失败。能力、覆盖和作业状态必须分别判断。

## 5. 清理核验

1. 检查 `cleanup-receipt.json`，确认请求、代理、bridge、进程规则、任务自有驱动、CurrentUser POC CA、私钥和临时加密证据的清理状态成功。
2. 再执行一次幂等清理并验证任务目录消失：

   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/poc-cleanup.ps1 -JobId $manifest.job_id
   ```

3. 确认 `.poc-secrets/<job-id>`、`.poc-runtime/<job-id>` 和所有 `.enc` 文件均不存在。若无法确认驱动或证书已移除，将 `cleanup_success` 视为 false 并直接销毁虚拟机。

## 6. 保留与虚拟机终结

获准复制到受控、Git 外或被 Git 忽略位置的文件仅限：

- `dataset.json`
- `validation.json`
- `manifest.json`
- `evidence/*.json`
- `cleanup-receipt.json`
- 通过秘密扫描的 `run.log`

最多保留 7 天。不得复制 `.enc`、CA、私钥、Token、Cookie、微信登录状态或原始响应。

复制获准的普通结果后，回滚到 `wechat-login-baseline` 或销毁虚拟机。快照回滚/销毁是清除 VM 内残余登录状态、驱动和证书的最终保证；如果清理状态存在任何不确定性，必须选择销毁。
