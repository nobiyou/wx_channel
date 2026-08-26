# 微信视频号采集容量扩展 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 wx_channel 与 TrendRadar 微信视频号的有效采集上限统一到 `30 / 500 / 100 / 200`，并用分页边界测试证明扩容不会破坏截断、重复 marker 和部分完成语义。

**Architecture:** wx_channel 批次协议继续负责请求边界，评论采集器沿用现有分页循环，只放宽微信批次 limits 校验并补充边界 fixture。TrendRadar 保留其它平台的通用 AdminLimits，通过平台专属容量合同为微信视频号选择 `30 / 500 / 100 / 200`，配置页面在平台切换时同步微信专属 max，后端保存和运行前校验使用同一组值。

**Tech Stack:** Go 1.24.3、TDM-GCC 10.3.0、Python 3.11、pytest、FastAPI/Jinja 管理页面、Ruff。

---

### Task 1: 固化 wx_channel 扩容边界测试

**Files:**
- Modify: `D:/Agent/projects/wechat-channel-comment-poc/internal/poc/ltaoo_batch_test.go`
- Modify: `D:/Agent/projects/wechat-channel-comment-poc/internal/poc/comments_test.go`
- Modify: `D:/Agent/projects/wechat-channel-comment-poc/internal/poc/model_test.go`

- [ ] 添加批次 limits 边界测试：接受 `works=30, top_level_comments_per_work=500, replies_per_comment=100, replies_per_work=200`，分别拒绝 501、101、201。
- [ ] 添加达到上限后的截断测试：构造超过上限的一级评论和回复，断言结果数量、`Truncated` 和安全原因。
- [ ] 运行红灯测试：`go test ./internal/poc -run 'TestLoadBatchRequest|TestTopLevel|TestReplies' -count=1`，新增边界断言应在实现前失败。

### Task 2: 放宽 wx_channel 批次校验并保持分页安全规则

**Files:**
- Modify: `D:/Agent/projects/wechat-channel-comment-poc/internal/poc/ltaoo_batch.go:156-160`
- Test: `D:/Agent/projects/wechat-channel-comment-poc/internal/poc/ltaoo_batch_test.go`
- Test: `D:/Agent/projects/wechat-channel-comment-poc/internal/poc/comments_test.go`

- [ ] 将请求校验改为 `30 / 500 / 100 / 200`，旧 limits 请求继续有效。
- [ ] 确认 `CollectComments` 只依据 `Options.Limits` 停止，不新增第二套硬编码上限；保留 marker 重复、无新增数据和关系异常的安全停止规则。
- [ ] 使用批准的 Go/TDM-GCC 工具链运行 `go test ./internal/poc ./scripts -count=1`。
- [ ] 提交：`feat: expand WeChat batch limits`。

### Task 3: 增加 TrendRadar 微信视频号专属容量边界

**Files:**
- Modify: `D:/Agent/services/trendradar-monitor/trendradar/social/capacity_contract.py`
- Modify: `D:/Agent/services/trendradar-monitor/trendradar/admin/models.py`
- Test: `D:/Agent/services/trendradar-monitor/tests/test_social_capacity_contract.py`
- Test: `D:/Agent/services/trendradar-monitor/tests/test_admin_profile_defaults.py`
- Test: `D:/Agent/services/trendradar-monitor/tests/test_admin_profile_routes.py`

- [ ] 保留其它平台的通用 AdminLimits；为 `wechat_channels` 使用 `30 / 500 / 100 / 200` 的专属边界。
- [ ] 后端接受微信边界值，拒绝 31、501、101、201 对应的超限值；其它平台继续使用原有上限。
- [ ] 运行 `pytest -q tests/test_social_capacity_contract.py tests/test_admin_profile_defaults.py tests/test_admin_profile_routes.py`，先验证红灯再验证绿灯。

### Task 4: 让管理页面按平台显示正确 max

**Files:**
- Modify: `D:/Agent/services/trendradar-monitor/trendradar/admin/templates/profile_edit.html`
- Modify: `D:/Agent/services/trendradar-monitor/trendradar/admin/static/admin.js`
- Test: `D:/Agent/services/trendradar-monitor/tests/test_admin_wechat_channels_ui.py`
- Test: `D:/Agent/services/trendradar-monitor/tests/test_admin_e2e.py`

- [ ] 为评论、单评论回复、作品回复字段增加微信专属 max 数据属性，分别为 500、100、200；作品数继续为 30。
- [ ] 增加 `syncTargetCapacityInputs(row)`，平台切换到微信时使用专属 max，切换到其它平台时恢复通用 max；不静默改写用户输入。
- [ ] 覆盖已有目标和新增目标的初始化、平台切换和保存校验。
- [ ] 增加 UI 与端到端断言，确认微信和其它平台显示不同边界。

### Task 5: 端到端验证、文档和提交

**Files:**
- Modify: `D:/Agent/services/trendradar-monitor/docs/wechat-channels-runtime.md`
- Modify: `D:/Agent/services/trendradar-monitor/docs/social-comment-monitoring.md`
- Test: both repositories' focused suites

- [ ] 更新文档，说明微信上限为 `30 / 500 / 100 / 200`，其它平台不继承微信专属上限，并提示请求数、运行时间和存储占用会增加。
- [ ] 运行 TrendRadar focused pytest suite 和 targeted Ruff。
- [ ] 使用 fixture 发送较大 limits，检查批次 schema、截断回执、部分完成状态和清理回执；不把 fixture 当作真实 PC 微信容量证明。
- [ ] 提交 TrendRadar：`feat: expand WeChat capacity contract`。
- [ ] 检查 `config/profiles/music-festival/config.yaml`、`output/`、`.tmp-*` 等用户现场文件未被纳入提交。
