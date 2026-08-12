# ltaoo 两页评论可行性探针设计

## 1. 文档状态

- 状态：已批准
- 日期：2026-08-12
- 目标上游：`https://github.com/ltaoo/wx_channels_download`
- 当前工作仓库：`wechat-channel-comment-poc`

## 2. 目标

在不接入 nobiyou 完整采集器的前提下，验证当前 PC 微信版本与 ltaoo 组合是否能够完成以下最小真实链路：

1. 使用视频号分享链接取得作品详情；
2. 从详情响应中提取 `oid` 和 `nid`；
3. 获取一级评论第一页；
4. 将第一页返回的 `lastBuffer` 原样用于第二页请求；
5. 生成不含评论数据和原始响应的脱敏证明；
6. 精确清理由本次探针创建的 CA、私钥、配置和 ltaoo 进程。

该探针是技术路线的生死闸门。只有它成功后，才考虑复用 nobiyou 的分页、去重、中断恢复和长列表处理逻辑。

## 3. 非目标

- 不接入或修改 nobiyou 的完整采集器。
- 不搜索作品，不自动选择样本。
- 不请求二级回复。
- 不请求第三页或循环到列表结束。
- 不保存评论正文、昵称、账号、头像 URL、原始评论 ID、原始作品 ID、原始游标、分享链接或原始响应。
- 不修改 Clash 配置、系统代理、WinHTTP 代理或路由表。
- 不启用 ltaoo TUN，不让 ltaoo 接管系统代理。
- 不运行当前仓库提交的 `wx_channel*.exe`。
- 不按证书名称模糊删除证书，不自动覆盖恢复外部网络设置。

## 4. 选定方案

采用 PowerShell 三段式探针，共享一个本次运行的 `manifest.json`：

- 准备和基线记录；
- 两页评论探测；
- 精确清理和漂移检查。

探针直接调用 ltaoo 官方本地 HTTP API，不经过 nobiyou HTTP 服务。输入固定为一个操作员选定的视频号分享链接。该样本应有足够多的公开一级评论，使第一页返回非空 `lastBuffer`。

没有选择 Go CLI，因为本阶段只需要 Windows 环境闸门，增加命令入口和构建层会扩大变量。没有直接修改 nobiyou，因为这会把 ltaoo 兼容性与适配代码正确性混在一次失败中。

## 5. 文件与职责

### 5.1 版本控制内文件

#### `scripts/prepare-ltaoo-probe.ps1`

- 创建唯一 `run_id` 和运行目录。
- 记录运行前安全基线。
- 生成本次独立临时 CA。
- 限制私钥文件 ACL，仅允许当前 Windows 用户访问。
- 生成 ltaoo 专用配置。
- 经显式人工确认后，仅将公钥证书安装到 `CurrentUser\Root`。
- 启动用户显式传入的源码构建 ltaoo EXE，并记录进程身份。
- 不修改 Clash、系统代理、WinHTTP 代理或路由。

#### `scripts/probe-ltaoo-comments.ps1`

- 接收视频号分享链接和 `run_id`。
- 调用 ltaoo 详情接口取得 `oid/nid`。
- 调用评论第一页。
- 仅在第一页存在非空 `lastBuffer` 时调用第二页。
- 生成脱敏 `probe-summary.json`。
- 不请求回复、第三页或完整列表。

#### `scripts/cleanup-ltaoo-probe.ps1`

- 读取指定 `run_id` 的 manifest。
- 只停止身份仍与 manifest 精确匹配的 ltaoo 进程。
- 只删除 manifest 精确记录的 CurrentUser CA 指纹。
- 只删除 manifest 精确记录的证书、私钥和配置文件。
- 重新计算基线摘要并报告外部状态漂移。
- 不自动覆盖恢复 Clash、代理、路由或外部进程状态。

#### `scripts/ltaoo_probe_script_test.go`

- 使用仅监听回环地址的模拟 HTTP 服务测试脚本。
- 测试不得安装证书、修改代理或启动真实 ltaoo。

#### `docs/LTAOO_TWO_PAGE_PROBE.md`

- 记录准备、人工确认、探测、清理的固定顺序。
- 记录通过标准、停止条件和结果解释。

### 5.2 运行目录

所有运行数据位于已被 Git 忽略的目录：

```text
.tmp_runtime/ltaoo-probe/<run_id>/
├── manifest.json
├── baseline.json
├── probe-summary.json
├── cleanup-receipt.json
├── ltaoo-probe.yaml
└── secrets/
    ├── ca-cert.pem
    └── ca-key.pem
```

`secrets` 仅允许当前 Windows 用户访问。成功清理后，证书、私钥和临时配置必须消失。为保留可审计结果，`manifest.json`、`baseline.json`、`probe-summary.json` 和 `cleanup-receipt.json` 可以继续留在该 Git 忽略目录中；manifest 中的秘密文件路径仍只允许指向本次运行目录。

## 6. ltaoo 运行边界

准备脚本强制生成以下配置语义：

```yaml
api:
  protocol: http
  hostname: 127.0.0.1
  port: 2022

proxy:
  enabled: true
  system: false
  hostname: 127.0.0.1
  port: 2023
  tun: false
  skipInstallRootCert: true

cert:
  file: <本次 ca-cert.pem 的绝对路径>
  key: <本次 ca-key.pem 的绝对路径>
  name: <包含 run_id 的唯一名称>
```

脚本不配置 Clash。操作员在 Clash 中按 ltaoo 官方协同方式把目标流量转发到 `127.0.0.1:2023`。ltaoo 进程自身必须直连，防止代理回环。

传入的 ltaoo EXE 必须由用户从审核过的 ltaoo 源码构建。准备脚本拒绝当前仓库中的 `wx_channel.exe`、`wx_channel_cloud.exe`、`wx_channel_radar.exe` 及同名前缀文件。脚本记录 ltaoo EXE 的 SHA-256、PID 和启动时间，但不保存 EXE 绝对路径或完整命令行。

## 7. 临时 CA

- 使用 Windows/.NET 生成 2048-bit RSA 自签名 CA。
- CA 名称包含本次 `run_id`，有效期应保持短期。
- ltaoo 使用 PEM 公钥证书和 PEM 私钥。
- 安装前显示 CA 指纹及变更摘要。
- 操作员必须输入包含本次 `run_id` 的精确确认文本。
- 只安装公钥证书到 `CurrentUser\Root`。
- 私钥不得导入证书库、打印、复制到同步目录或写入摘要。
- ltaoo 设置 `skipInstallRootCert:true`，不得触发其自动安装逻辑。

## 8. Manifest

`manifest.json` 只保存清理和身份校验所需字段：

```text
schema_version
run_id
created_at
repo_commit
runtime_root
CA:
  store = CurrentUser\Root
  thumbprint
  subject
  certificate_file
  private_key_file
ltaoo:
  executable_sha256
  pid
  process_start_time
  config_file
  api_base = http://127.0.0.1:2022
  proxy_endpoint = 127.0.0.1:2023
```

manifest 不保存分享链接、`oid/nid`、原始游标、响应内容、PAC URL、代理凭据或完整进程命令行。

## 9. 基线

`baseline.json` 记录下列规范化状态的 SHA-256 和必要的非敏感状态标签：

- 当前用户系统代理配置；
- WinHTTP 代理配置；
- 路由表；
- `CurrentUser\Root` 和 `LocalMachine\Root` 的证书指纹集合；
- `127.0.0.1:2022` 和 `127.0.0.1:2023` 的监听者；
- Clash、微信和 ltaoo 相关进程的身份摘要。

PAC URL、代理凭据和完整命令行不得明文落盘。基线用于清理后的漂移检测，不作为自动恢复来源。

## 10. 两页探测数据流

1. 校验 API 基址只指向 `http://127.0.0.1:<port>`。
2. 调用 `/api/status`，确认 ltaoo API 和代理处于预期监听状态。
3. 对分享链接进行标准 URL 查询编码，调用 `/api/channels/feed/profile?url=...`。
4. 仅在内存中解析详情响应，取得作品 `id` 与 `objectNonceId`。
5. 调用 `/api/channels/feed/comment/list?oid=...&nid=...` 获取第一页。
6. 在内存中统计评论数量、评论 ID 哈希和第一页游标摘要。
7. 如果第一页 `lastBuffer` 为空，停止并输出 `inconclusive_no_second_page`。
8. 使用第一页原始 `lastBuffer` 值，通过标准 URL 编码器构造第二页请求。
9. 调用第二页并计算相同类别的脱敏摘要。
10. 立即释放响应对象，只写入 `probe-summary.json`。

每个请求超时 30 秒。详情请求最多一次，评论请求最多两次。探针不自动重试，不调用回复接口，不请求第三页。

## 11. 脱敏摘要

`probe-summary.json` 只包含：

- 分享链接 SHA-256；
- `oid/nid` SHA-256；
- 详情、第一页和第二页的 HTTP 状态与业务错误码；
- 每页评论数量；
- 使用本次随机盐计算的评论 ID 哈希；
- 页内与跨页重复数量；
- `lastBuffer` 是否存在、长度和带盐哈希；
- 第二页请求游标的带盐哈希；
- `cursor_continuity`；
- 最终状态和原因码。

随机盐只在探针进程内存中存在，不写入文件。因此哈希只用于本次摘要内部证明相等性，不能被后续运行关联。

脚本不得向终端、日志或错误对象输出响应体、评论正文、昵称、账号、头像 URL、原始评论 ID、原始作品 ID、原始游标或原始分享链接。

## 12. 结果状态

### `verified_two_pages`

必须同时满足：

- 详情接口 HTTP 和业务响应成功；
- 第一页成功并至少包含一条评论；
- 第一页返回非空 `lastBuffer`；
- 第二页 HTTP 和业务响应成功；
- 第二次请求游标哈希等于第一页响应游标哈希；
- 评论接口请求总数精确为两次；
- 未发生敏感数据落盘或基线边界违规。

### `inconclusive_no_second_page`

第一页成功，但没有非空 `lastBuffer`。这不证明分页失败，应换用评论更多的作品重新运行新的 `run_id`。

### `failed`

包括但不限于：

- ltaoo API 或代理未就绪；
- 页面 socket 未初始化；
- 详情或评论接口业务失败；
- 当前用户 CA 不被微信页面接受；
- 响应结构无法识别；
- 第二页请求失败或游标连续性不成立；
- 发出超过两次评论请求；
- 敏感数据进入普通输出；
- 发生任何被禁止的系统修改。

## 13. 停止条件

遇到以下任一情况立即停止新请求并进入清理：

- API 基址不是回环 HTTP 地址；
- `2022/2023` 端口在准备前已被非目标进程占用；
- CA 安装或微信信任失败；
- 页面 socket 未初始化；
- 详情、第一页或第二页超时、限流、拒绝或结构漂移；
- 第一页游标为空；
- manifest 身份不完整或运行目录校验失败；
- 检测到脚本试图修改 Clash、系统代理、WinHTTP 或路由。

## 14. 精确清理

清理脚本必须幂等，并按以下顺序执行：

1. 校验 `run_id` 只包含允许字符且长度受限。
2. 解析运行目录，确认其严格位于 `.tmp_runtime/ltaoo-probe/` 下且路径中没有重解析点。
3. 读取并校验 manifest schema、`run_id` 和所有目标路径。
4. 仅当 PID、启动时间和当前进程映像的 SHA-256 同时匹配时停止 ltaoo；进程已退出视为无需操作，身份不匹配只报警且不终止该进程。
5. 仅按 manifest 中的精确指纹从 `CurrentUser\Root` 删除 CA；不得按名称或模式删除。
6. 验证该指纹已不在 `CurrentUser\Root`。
7. 逐个删除 manifest 中精确列出的证书、私钥和配置文件；每个路径必须位于本次运行目录。
8. 仅在目录为空且仍通过边界检查时删除 `secrets` 空目录；保留非秘密审计文件。
9. 重新计算基线摘要并生成 `cleanup-receipt.json`。

Clash、系统代理、WinHTTP、路由和外部进程与基线不同时，只记录类别级警告，不自动覆盖恢复。CA 或私钥不能清除时，`cleanup_success=false`。

## 15. 自动测试

Windows 测试使用 Go 启动回环模拟 HTTP 服务，再调用 PowerShell 脚本。覆盖：

- 分享链接由标准 URL 编码器处理；
- `oid/nid` 仅在内存使用；
- 包含 `+`、`/`、`=` 的游标原样进入第二次请求；
- 评论请求总数精确为两次；
- 不请求回复或第三页；
- 第一页无游标时不发送第二次评论请求；
- 摘要不包含正文、昵称、URL、原始游标、原始 ID 和模拟响应中的诱饵秘密；
- API 基址不是回环地址时拒绝运行；
- 外部路径、路径重解析点、伪造 PID、错误启动时间、错误 EXE 哈希和非精确证书目标被拒绝；
- 清理可以重复执行；
- 测试模式不安装证书、不改代理、不改路由、不启动真实 ltaoo。

还需执行静态断言，确认脚本中不存在：

- `LocalMachine\Root` 写入；
- ltaoo `system:true` 或 `tun:true`；
- 按证书名称模糊删除；
- 未验证目标的递归删除；
- 打印 HTTP 响应体的代码路径。

## 16. 人工验收顺序

1. 从审核过的 ltaoo 源码构建 Windows EXE。
2. 确认微信已登录且可人工打开视频号页面。
3. 确认 Clash 保持运行，并已按 ltaoo 官方方式转发目标流量。
4. 运行准备脚本并核对基线与 CA 指纹。
5. 输入包含 `run_id` 的精确确认文本。
6. 打开或刷新视频号页面，确认 ltaoo 页面 socket 就绪。
7. 选择一个明显超过一页公开评论的作品分享链接。
8. 运行两页探针。
9. 检查 `probe-summary.json` 的状态和 `cursor_continuity`。
10. 无论结果如何都运行清理脚本。
11. 检查 `cleanup-receipt.json`，确认 CA、私钥、配置和目标进程均已处理，并审阅外部漂移警告。

## 17. 后续决策

- `verified_two_pages`：允许进入下一阶段，设计 ltaoo 页面/代理适配器与 nobiyou 分页、去重和恢复逻辑的接口。
- `inconclusive_no_second_page`：换用评论更多的作品重新运行，不修改采集代码。
- `failed`：根据失败阶段判断是 Clash 转发、CA 信任、页面注入、socket 初始化、接口结构还是游标问题；在根因明确前不接入完整采集器。
