# P1-A PC 微信视频号页面自动刷新实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 ltaoo 和代理就绪后，自动激活并刷新已打开的 PC 微信视频号页面，稳定建立 profile 桥接，同时保持三链接采集、分页、幂等入库和安全清理行为不变。

**Architecture:** 保留现有批处理时序和 `Invoke-WeChatPageRefresh.ps1` 边界，在窗口选择之后增加可验证的窗口恢复/激活动作，再发送一次 F5。PowerShell helper 只操作受控微信窗口；TrendRadar 继续通过显式 `-AutoRefreshWechatPage` 参数调用它，profile 就绪仍由批处理器负责共享 60 秒等待。

**Tech Stack:** Windows PowerShell 5.1、Win32 `user32.dll`、Go 运行时脚本测试、TrendRadar Python/pytest、PC 微信 `Weixin`/`WeChatAppEx` 窗口。

---

## 文件结构与职责

- Modify: `scripts/Invoke-WeChatPageRefresh.ps1` — 窗口枚举、选择、恢复、激活和一次 F5 刷新。
- Modify: `scripts/trendradar_runtime_script_test.go` — 静态安全约束和脚本契约测试。
- Modify: `scripts/Invoke-LtaooTrendRadarBatch.ps1` — 仅在 helper 契约变化时调整启动期错误传播；保持已有 60 秒 profile 等待不变。
- Modify: `docs/trendradar-runtime.md` — 记录激活动作、窗口选择和 P1-A 非目标。
- Modify: `CHANGELOG.md` — 记录未发布变更。
- Modify: `D:/Agent/services/trendradar-monitor/trendradar/social/wechat_channels_collector.py` — 仅在命令参数或错误映射需要同步时修改；不改主工作区用户配置。
- Test: `D:/Agent/services/trendradar-monitor/tests/test_social_wechat_channels_collector.py` — 验证 TrendRadar 只对微信视频号命令追加自动刷新参数。

## Task 1: 为窗口激活契约补充失败测试

**Files:**
- Modify: `scripts/trendradar_runtime_script_test.go`
- Modify: `scripts/Invoke-WeChatPageRefresh.ps1`

- [ ] **Step 1: 明确静态契约测试内容**

在 `TestTrendRadarRuntimeScripts` 中增加以下必需字符串检查：

```go
for _, required := range []string{
    "setforegroundwindow",
    "showwindow",
    "sw_restore",
    "wechat_window_activation_failed",
} {
    if !strings.Contains(strings.ToLower(refresh), required) {
        t.Fatalf("refresh helper missing %q", required)
    }
}
```

同时保留现有禁止项：`sendkeys`、`clipboard`、普通浏览器 URL、`invoke-expression` 和 `cmd /c`。

- [ ] **Step 2: 运行测试确认当前 helper 失败**

```powershell
$go = '.poc-tools\go1.24.3\go\bin\go.exe'
& $go test ./scripts -run 'TestTrendRadarRuntime' -count=1
```

预期：失败并指出缺少 `setforegroundwindow` 或相关激活契约，形成明确的 RED 基线。

- [ ] **Step 3: 提交测试基线**

```powershell
git add scripts/trendradar_runtime_script_test.go
git commit -m "test: require explicit WeChat window activation"
```

## Task 2: 实现安全的窗口恢复和激活

**Files:**
- Modify: `scripts/Invoke-WeChatPageRefresh.ps1`

- [ ] **Step 1: 扩展 Win32 声明**

在现有 `TrendRadar.WeChatWindow` C# 类型中加入：

```csharp
public const int SW_RESTORE = 9;

[DllImport("user32.dll")]
public static extern bool ShowWindow(IntPtr hWnd, int command);

[DllImport("user32.dll")]
public static extern bool SetForegroundWindow(IntPtr hWnd);
```

- [ ] **Step 2: 在发送 F5 前恢复并激活选中窗口**

在选中窗口后执行以下顺序：

```powershell
if (-not [TrendRadar.WeChatWindow]::ShowWindow(
        $selected.Handle,
        [TrendRadar.WeChatWindow]::SW_RESTORE)) {
    throw 'wechat_window_activation_failed'
}
if (-not [TrendRadar.WeChatWindow]::SetForegroundWindow($selected.Handle)) {
    throw 'wechat_window_activation_failed'
}
Start-Sleep -Milliseconds 150
```

随后保留现有单次 `WM_KEYDOWN` / `WM_KEYUP` F5 发送；不得改为 `SendKeys` 或广播给多个窗口。

- [ ] **Step 3: 为激活动作保留稳定错误码**

任何 `ShowWindow` 或 `SetForegroundWindow` 返回失败都只抛出 `wechat_window_activation_failed`，由父运行时进入既有安全清理，不继续发送批处理请求。

- [ ] **Step 4: 运行脚本测试和 PowerShell 解析**

```powershell
& $go test ./scripts -run 'TestTrendRadarRuntime' -count=1
foreach ($file in 'scripts/Invoke-WeChatPageRefresh.ps1','scripts/Invoke-LtaooTrendRadarBatch.ps1') {
    [System.Management.Automation.Language.Parser]::ParseFile(
        (Resolve-Path $file), [ref]$null, [ref]$null) | Out-Null
}
```

预期：Go 测试通过，两个脚本均能解析。

- [ ] **Step 5: 提交 helper 实现**

```powershell
git add scripts/Invoke-WeChatPageRefresh.ps1 scripts/trendradar_runtime_script_test.go
git commit -m "feat: activate WeChat page before refresh"
```

## Task 3: 同步 TrendRadar 调用契约和错误展示

**Files:**
- Modify: `D:/Agent/services/trendradar-monitor/trendradar/social/wechat_channels_collector.py`
- Modify: `D:/Agent/services/trendradar-monitor/tests/test_social_wechat_channels_collector.py`

- [ ] **Step 1: 先运行现有 TrendRadar 回归**

```powershell
$venv = 'D:\Agent\services\trendradar-monitor\.venv\Scripts'
& "$venv\python.exe" -m pytest tests/test_social_wechat_channels_collector.py -q
```

预期：合并后的基线测试通过。

- [ ] **Step 2: 仅在契约需要时补充 `wechat_window_activation_failed` 映射**

如果 helper 抛出新错误码，则在 `_RUNTIME_FAILURE_ISSUES` 增加：

```python
"wechat_window_activation_failed": ("startup", "wechat_page_unavailable"),
```

并在 `tests/test_social_wechat_channels_collector.py` 增加对应 stderr 映射断言。不得改变已有 `profile_not_ready`、`comment_response_invalid` 或清理错误的语义。

- [ ] **Step 3: 验证命令只作用于微信视频号**

```powershell
& "$venv\python.exe" -m pytest tests/test_social_wechat_channels_collector.py -q
& "$venv\ruff.exe" check trendradar/social/wechat_channels_collector.py tests/test_social_wechat_channels_collector.py
```

- [ ] **Step 4: 提交 TrendRadar 契约变更**

```powershell
git add trendradar/social/wechat_channels_collector.py tests/test_social_wechat_channels_collector.py
git commit -m "fix: classify WeChat window activation failures"
```

## Task 4: 更新运行时文档和交付脚本

**Files:**
- Modify: `docs/trendradar-runtime.md`
- Modify: `CHANGELOG.md`
- Copy for local ignored delivery: `D:/Agent/services/trendradar-monitor/tools/wechat-channels/Invoke-WeChatPageRefresh.ps1`

- [ ] **Step 1: 文档写明新时序和非目标**

明确记录：激活后只发送一次 F5；共享 60 秒等待仍然存在；不支持搜索、输入链接和普通浏览器；窗口失败会安全清理。

- [ ] **Step 2: 更新本地交付 helper 并核对哈希**

```powershell
Copy-Item scripts/Invoke-WeChatPageRefresh.ps1 ` 
  'D:\Agent\services\trendradar-monitor\tools\wechat-channels\Invoke-WeChatPageRefresh.ps1' -Force
Get-FileHash 'D:\Agent\services\trendradar-monitor\tools\wechat-channels\Invoke-WeChatPageRefresh.ps1' -Algorithm SHA256
```

只更新该 helper；不覆盖已授权的 ltaoo、路由器或批处理二进制。

- [ ] **Step 3: 提交文档变更**

```powershell
git add docs/trendradar-runtime.md CHANGELOG.md
git commit -m "docs: document activated WeChat page refresh"
```

## Task 5: 受控真实可行性验证

**Files:**
- Runtime output: `D:/Agent/services/trendradar-monitor/output/admin/wechat-channels-runtime/<run-id>`
- Database: `D:/Agent/services/trendradar-monitor/output/weixinshipinhao/social/comments.db`

- [ ] **Step 1: 验证前置状态**

确认本机授权状态为 `authorized`，PC 微信内部已打开一个目标视频号页面，只有一个可用 `WeChatAppEx` 页面候选；不读取或输出 Cookie、响应正文和秘密。

- [ ] **Step 2: 执行单轮真实任务**

使用现有 `weixinshipinhao` 配置运行 `festival-weixinshipinhao` 目标，记录运行 ID、3 个输入索引、作品数、一级评论数、回复数、稳定错误码和 cleanup receipt。

预期：无需人工按 F5；profile 就绪后批处理开始；若内容接口异常，只显示 `comment_response_invalid` 等内容级错误，不误报窗口或桥接错误。

- [ ] **Step 3: 执行第二轮幂等复跑**

使用同一配置再次运行，检查：

```sql
SELECT COUNT(*) FROM social_contents;
SELECT comment_id, COUNT(*)
FROM social_comments
WHERE comment_id IS NOT NULL
GROUP BY comment_id
HAVING COUNT(*) > 1;
```

预期：作品不重复，重复 `comment_id` 查询返回 0 行，历史数据保留。

- [ ] **Step 4: 验证失败路径清理**

在没有可见微信页面窗口的受控场景中，确认返回 `wechat_window_not_found` 或 `wechat_window_ambiguous`，并核对 `safe=true`、`process_stopped=true`、`ports_released=true`、`router_restored=true`。

- [ ] **Step 5: 记录验收结论**

只记录无秘密的摘要：是否自动刷新、profile 是否就绪、作品/评论/回复计数、错误码、清理布尔值和重复 ID 计数。

## Task 6: 合并前最终回归

- [ ] **Step 1: 运行 Go 测试**

```powershell
& $go test ./internal/poc ./cmd/wx_channel_ltaoo_batch ./scripts -count=1
```

预期：全部通过。

- [ ] **Step 2: 运行 TrendRadar 相关测试**

```powershell
& "$venv\python.exe" -m pytest `
  tests/test_social_wechat_channels_collector.py `
  tests/test_social_wechat_channels_batch.py `
  tests/test_social_wechat_channels_pipeline.py `
  tests/test_social_wechat_channels_execution.py `
  tests/test_social_schedule.py `
  tests/test_admin_schedules.py -q
```

预期：无失败；允许既有环境标记的 skipped 测试。

- [ ] **Step 3: 检查工作区和提交内容**

```powershell
git diff --check
git status --short
```

确保不包含运行输出、授权数据、临时 CA、秘密文件或 TrendRadar 主工作区的无关用户修改。

