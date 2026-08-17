# ltaoo Profile 就绪等待与安全诊断设计

## 1. 目标

修复 TrendRadar 微信视频号批次在 ltaoo HTTP API 已监听、但 PC 微信页面桥尚未就绪时立即请求 profile 并失败的问题，同时保留足以区分常见失败原因的封闭错误码。

本阶段还要在同一台机器、同一微信页面、同一 Mihomo 配置和同一组三个分享链接下，对当前 ltaoo 构建与历史成功构建执行 A/B。任何真实运行都必须保留安全清理回执。

## 2. 已知事实

- 当前 PowerShell 运行时只等待 `/api/status` 返回 HTTP 200。
- Go 批次随后立即调用 `/api/channels/feed/profile`。
- 旧版 ltaoo 的 `/api/status` 中 `channels.available` 不可信；历史成功探针使用 profile 成功作为页面桥的最终就绪证明。
- 当前 `CollectWorksFromURLs` 把所有 profile 错误统一写成 `profile_unavailable`。
- 2026-08-14 的真实成功探针使用 ltaoo SHA-256 `F5D411004CA873F9D2B9EAF5DB26041D3644A9729FD66D45554169D81FCD898D`。
- TrendRadar 当前 ltaoo SHA-256 为 `18F0E4CA43CC16F8235C8CFD35031B6A822853C87B2493FBC2BCF87DDDAD9F40`。

## 3. 方案选择

在 Go 批次层实现共享 profile 就绪窗口。PowerShell 继续只管理授权、CA、Mihomo 路由、进程、端口和清理，不解析 profile 业务协议。

不采用以下方案：

- 每个链接独立等待 30 秒：三个链接最坏等待 90 秒，并把同一个页面桥故障重复放大。
- 在 PowerShell 中等待 profile：会复制 Go 客户端的 HTTP、JSON、业务码和安全错误分类逻辑。
- 仅延长 `/api/status` 等待：旧版状态字段不能证明页面桥可用。

## 4. 共享就绪窗口

批次在 `/api/status` 通过后，用配置顺序中的第一个尚未得到终态错误的链接证明 profile 就绪。

- 总预算固定为 30 秒，不按链接重置。
- 第一次请求立即发送。
- 只有传输失败、HTTP 503 或其他 5xx 归类为可重试错误。
- 可重试请求采用有界短间隔退避；每次睡眠不能越过总截止时间。
- 一旦任一 profile 成功，该 Work 直接进入结果，不重复请求该链接。
- 页面桥证明就绪后，其余链接各请求一次，不再使用 30 秒等待。
- 401、403、429、非法分享链接、成功响应结构错误和其他非重试错误立即终止该链接，不消耗重复请求。
- 上下文取消立即停止，不得继续请求。

如果第一个链接得到非重试终态错误，批次可以继续用下一个链接尝试证明页面桥；所有尝试共享同一个 30 秒预算。预算耗尽后，当前及尚未尝试的链接记录 `profile_not_ready`，不得为每个剩余链接重新等待。

## 5. 安全错误分类

Go 客户端只根据受信任的本地 HTTP 状态、封闭解析结果和现有 `ErrorCategory` 分类，不保存或透传 ltaoo 原始错误正文。

| wx_channel 批次错误码 | 条件 | TrendRadar 映射 |
| --- | --- | --- |
| `profile_not_ready` | 共享 30 秒窗口内可重试错误始终未恢复 | `detail / wechat_page_unavailable` |
| `profile_access_denied` | HTTP 401 或 403 | `detail / login_required` |
| `profile_rate_limited` | HTTP 429 | `search / rate_limited` |
| `profile_schema_mismatch` | HTTP 200 但外层、业务层、object ID 或 nonce 结构无效 | `detail / page_structure_changed` |
| `profile_unavailable` | 其他封闭但无法进一步分类的失败 | `detail / content_unavailable` |

非法 URL 继续使用现有 `invalid_share_url`。任何错误码都不得包含 URL、作品 ID、nonce、Cookie、响应正文、文件路径或进程信息。

## 6. 批次与 TrendRadar 协议

- 批次 schema 继续使用 `wechat-channels-batch/1`；只扩展允许的 issue/reason code 集合。
- `issues.jsonl` 对每个受影响输入记录稳定错误码和 `input_index`。
- `manifest.reason_codes` 只包含实际出现的稳定错误码并保持去重。
- `targets` 在没有成功作品时继续序列化为 `[]`。
- TrendRadar 验证器显式允许新错误码并映射为现有控制台诊断，不接受未知码。
- 部分链接成功时继续保留 `partial` 状态和已验证数据。

## 7. 测试设计

Go 回环 HTTP 测试覆盖：

1. profile 前两次返回 503、第三次成功；只产生一个 Work，且成功请求不重复。
2. 多个链接共享同一截止时间，不能各自获得 30 秒。
3. 第一个链接为非重试错误时，第二个链接仍可证明页面桥就绪。
4. 共享窗口耗尽时，当前和未尝试链接均得到 `profile_not_ready`。
5. 401/403、429、HTTP 200 结构错误和未知失败分别映射到规定错误码。
6. 上下文取消停止重试。
7. 现有顺序、URL 去重、Work 去重和上限语义不变。

TrendRadar 测试覆盖：

1. 五种 profile 错误码均通过封闭 schema 验证。
2. 每种错误码映射到规定的 `CollectionIssue`。
3. 未知 profile 错误码仍被拒绝。
4. 无成功目标时 `targets: []` 继续有效。

## 8. A/B 设计

真实 A/B 固定以下变量：

- PC 微信账号、视频号页面和刷新操作；
- 三个分享链接及顺序；
- TrendRadar 配置、批次 EXE、PowerShell 模块；
- Mihomo 进程、配置路径和运行规则；
- 30 秒共享 profile 就绪策略。

唯一变量是 ltaoo EXE：

- A：当前构建，SHA-256 `18F0E4CA43CC16F8235C8CFD35031B6A822853C87B2493FBC2BCF87DDDAD9F40`。
- B：历史成功构建，SHA-256 `F5D411004CA873F9D2B9EAF5DB26041D3644A9729FD66D45554169D81FCD898D`。

每轮记录：批次状态、profile 稳定错误码、作品数、一级评论数、回复数、执行耗时和 ltaoo 哈希。不得保存原始 profile 或评论响应。

每轮结束必须确认：

- `cleanup.safe=true`；
- 临时 CA 不存在；
- Mihomo 路由已恢复且无本轮标记；
- ltaoo 进程已停止；
- 2022/2023 端口已释放；
- 临时秘密文件已删除。

如果任一清理条件失败，停止后续 A/B，不得切换二进制继续运行。

## 9. 验收标准

- 自动测试证明 30 秒预算为整批共享，而不是按链接累计。
- 可重试 profile 在窗口内恢复时批次继续采集。
- 五种安全错误码端到端通过 wx_channel 与 TrendRadar 封闭验证。
- 原始错误正文和敏感值不进入批次、日志或控制台。
- 当前版和历史成功版各完成一次同环境受控运行，或明确记录由外部人工确认阻塞的 A/B 步骤。
- 两轮都具有完整安全清理证明。
