# ltaoo 新旧状态协议适配设计

## 1. 目标

让现有两页评论探针同时兼容 ltaoo 新、旧两种 `/api/status` 响应，并保留已经验证过的安全边界。旧版状态接口中的 `channels.available` 不作为硬性就绪条件；共享页面解析成功才是旧版页面 WebSocket 可用的最终证据。

本阶段只修正状态判定。它不增加回复请求、第三页、自动重试、完整采集器、代理配置、CA 管理或 Clash 修改。

## 2. 已知事实

当前探针只接受新版状态结构：

```json
{
  "code": 0,
  "data": {
    "api": { "listening": true },
    "proxy": { "listening": true }
  }
}
```

已验证可工作的旧版提交 `c0c2b8cc36af52ab2c3eb50cb7dc08b7d963efb0` 返回：

```json
{
  "code": 0,
  "data": {
    "channels": { "available": false },
    "version": "<non-empty>"
  }
}
```

该旧版处理函数即使 WebSocket 已连接也不会把 `channels.available` 改为 `true`。同一次真实验证中，共享页面解析、第一页评论和携带 `lastBuffer` 的第二页评论均成功，因此该字段不能代表旧版实际可用性。

## 3. 方案选择

在 `scripts/probe-ltaoo-comments.ps1` 内增加一个小型状态规范化函数。它只有一个调用方，不拆分新的 PowerShell 模块。

未采用的方案：

- 独立适配模块：当前只有一个调用方，额外文件和加载边界没有收益。
- 删除状态检查：会丢失新版 API/代理监听异常的提前诊断。

## 4. 状态规范化

规范化函数接收已经解析的状态响应，返回只包含非敏感枚举的对象。

### 4.1 新版结构

满足以下全部条件时识别为 `modern`：

- 外层 `code` 为 `0`；
- `data.api.listening` 字段存在；
- `data.proxy.listening` 字段存在。

两个监听值都必须为 `true`，否则抛出 `ltaoo_not_ready`。通过后，规范化结果为：

```text
status_schema = modern
readiness_proof = listeners_and_profile
```

共享页面解析仍是后续必经步骤，因此最终证据同时包含监听状态和 profile 成功。

### 4.2 旧版结构

满足以下全部条件时识别为 `legacy`：

- 外层 `code` 为 `0`；
- `data.version` 是非空字符串；
- `data.channels.available` 字段存在。

`channels.available` 的值只用于确认旧版结构，不作为就绪判断。规范化结果为：

```text
status_schema = legacy
readiness_proof = profile
```

随后必须成功调用共享页面解析接口，并取得非空 `object.id` 和 `object.objectNonceId`。这一步失败时不得请求评论。

### 4.3 未知或残缺结构

外层 `code` 非零时抛出 `ltaoo_not_ready`。外层成功但既不满足完整新版结构，也不满足完整旧版结构时抛出 `status_schema_error`。不得猜测版本，不得把缺失字段转换成默认 `false` 后继续。

## 5. 数据流与请求上限

请求顺序固定为：

1. 请求 `/api/status` 并规范化结构。
2. 请求共享页面 profile。
3. 只有 profile HTTP、业务码及 `oid/nid` 都成功时，请求第一页评论。
4. 只有第一页返回非空 `lastBuffer` 时，使用原始游标值请求第二页。
5. 第二页后无条件停止。

状态兼容不得增加任何请求循环。评论请求上限仍为两次；profile 失败前评论请求数必须为零；第一页无游标时评论请求数必须为一次。

## 6. 脱敏摘要

成功或不确定结果的 `probe-summary.json` 增加：

```text
status_schema: modern | legacy
readiness_proof: listeners_and_profile | profile
```

这两个字段都是固定枚举，不含原始响应。现有脱敏规则保持不变：不得保存或输出分享链接、`oid/nid`、原始评论 ID、原始游标、评论正文、昵称、账号或响应体。

在已经识别状态结构后发生的失败可以记录相同枚举；在识别前失败时不写伪造值。失败摘要继续使用稳定原因码，并保持 `comment_request_count` 准确。

## 7. 错误语义

- 新版任一监听值不是 `true`：`ltaoo_not_ready`。
- 状态外层业务码非零：`ltaoo_not_ready`。
- 旧版版本为空、必要字段缺失或结构未知：`status_schema_error`。
- profile HTTP、结构或业务失败：沿用现有 profile 原因码。
- profile 缺少 `oid/nid`：`profile_schema_error`。
- profile 未证明页面通道可用前，不发送评论请求。

不增加自动重试，也不把 profile 失败降级为成功。

## 8. 自动测试

Windows Go 测试通过回环 `httptest.Server` 驱动 PowerShell 探针，覆盖：

1. 完整新版状态能够完成两页，摘要记录 `modern` 和 `listeners_and_profile`。
2. 旧版 `channels.available=false` 能够通过 profile 证明就绪并完成两页，摘要记录 `legacy` 和 `profile`。
3. 新版监听未就绪时不请求 profile 或评论。
4. 旧版版本为空或结构残缺时不请求 profile 或评论。
5. 旧版 profile 失败时评论请求数为零。
6. 两种状态模式都保持最多两次评论请求，并验证第二次请求的游标连续性。
7. 两种模式的摘要和终端输出都不包含测试诱饵秘密。
8. 现有第一页无游标停止、远程 API 地址拒绝、清理安全和 PowerShell 语法测试继续通过。

## 9. 验收标准

- 新旧状态 fixture 全部通过。
- 真实旧版状态不再被错误的 `channels.available=false` 阻断。
- 新版监听失败仍被提前阻断。
- profile 失败时评论请求数严格为零。
- 成功路径评论请求数不超过两次。
- 新增摘要字段只有规定枚举。
- `go test ./scripts -v -count=1` 通过。
- Git 工作树只包含本阶段批准的脚本、测试和文档变更。
