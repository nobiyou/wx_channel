# 微信视频号评论 POC：X509Store 证书冒烟测试规格

状态：待用户复核

## 背景与证据

受限 POC 在隔离 VM 中完成了源码构建、安全审计和 preflight。唯一一次真实验证在操作员输入 `APPLY` 后立即以白名单错误码 `certificate_import_reported_false` 结束，随后清理路径执行成功。再次运行 preflight 时，隔离状态、证书基线、SunnyNet 驱动状态和监听器基线全部通过，VM 随后回滚到 `wechat-login-baseline`。

该结果排除了宿主机与 VM 的基本连通性问题，也表明失败发生在 bridge、proxy、驱动和评论采集启动之前。现有 PowerShell `Import-Certificate` 命令没有报告系统错误，但其返回结果未满足“恰好一张对象且 RawData SHA-256 一致”的严格协议。因此当前结论是 `capability_status: inconclusive`、`coverage_status: not_started`，不能据此判断上游评论采集能力。

## 决策变更

上一版规格明确选择继续使用 `Import-Certificate`。真实验证已经推翻这一兼容性假设，因此本规格显式取代该决定：Windows 证书存储实现改为 PowerShell 调用 `.NET System.Security.Cryptography.X509Certificates.X509Store`，不再调用 `Import-Certificate`，也不回退到 `certutil`、原项目自动安装入口或 `LocalMachine`。

这不是对真实采集范围的扩大。本轮只验证 CurrentUser 临时根证书的可控生命周期；即使冒烟成功，也不自动授权代理、驱动、微信数据访问或真实评论采集。

## 目标与非目标

目标：

- 在可销毁 Windows VM 中，以普通登录用户实际安装一张单次任务 CA 到 `CurrentUser\Root`。
- 按证书 RawData 的 SHA-256 在安装前、安装后、删除前和删除后进行精确枚举。
- 证明安装后恰好存在一个匹配，删除后不存在匹配。
- 无论成功或失败都销毁证书文件、私钥和运行时状态，并产生不含敏感信息的收据。
- 冒烟前后均通过安全 preflight，随后关机并回滚 `wechat-login-baseline`。

非目标：

- 不启动 SunnyNet、bridge、proxy、NFAPI 驱动或 `WeChatAppEx.exe` 进程规则。
- 不读取微信登录状态，不打开微信，不搜索作品，不采集任何评论。
- 不在宿主机安装或删除证书，不在宿主机运行冒烟命令。
- 不修改作品、评论、验证 JSON 模型或 TrendRadar。
- 不允许冒烟成功后自动进入真实采集。

## 组件与边界

### Windows 证书存储适配器

现有 `CertificateStore` 边界继续负责安装、精确查询和删除。Windows 实现通过已审查的 PowerShell 参数传递机制调用 `.NET X509Store`：

- `StoreName.Root`；
- `StoreLocation.CurrentUser`；
- `OpenFlags.ReadWrite`；
- 输入仅允许规范化后的 64 位十六进制 SHA-256 和经过现有路径、重解析点、文件类型、DER、CA 用途及 SHA-256 校验的 `job-ca.cert`；
- 输出只允许精确的 `true` 或 `false`；
- Warning、Information 和 Progress 流保持抑制；
- 禁止出现 `LocalMachine`、`Import-Certificate` 或 `certutil`。

安装脚本用 `X509Certificate2` 读取已验证 DER，调用 `X509Store.Add()`，但不依赖 `Add()` 的返回值。它重新枚举 Root 存储，对每张证书的 RawData 计算 SHA-256，并仅在匹配数恰好为一时返回 `true`。

删除脚本只处理 RawData SHA-256 精确匹配。零个匹配表示目标已不存在；恰好一个匹配时才允许调用 `X509Store.Remove()`；多个匹配不执行扩大删除，而是报告清理失败并依赖 VM 回滚消除状态。

### 独立 `cert-smoke` 入口

新增 `cert-smoke --ack-isolated-vm`，与 `run` 完全分离。它必须拒绝管理员或已提升权限的进程，以证明 CurrentUser 证书操作不依赖系统级权限。运行前显示的计划只能包含 `certificate=CurrentUser\Root`，操作员必须输入精确的 `CERT_APPLY`。

该入口只能装配以下依赖：preflight、单次 CA、受限存储、证书存储、私密文件清理和安全收据。它不能创建或引用 bridge、proxy、SunnyNet、驱动控制器、微信进程规则或采集器。

### 冒烟收据

结果写入 Git 忽略的任务目录中的 `certificate-smoke-receipt.json`，同时向控制台输出同一份脱敏状态。允许字段固定为：schema version、job ID、整体 `success`、`preflight_passed`、`not_elevated`、`preinstall_absent`、`install_verified`、`remove_verified`、`secrets_destroyed`、`runtime_state_destroyed`、单个白名单错误码和完成时间。禁止包含文件路径、证书指纹、证书内容、私钥、PowerShell/.NET 原始异常、Token、Cookie、URL 或微信数据。

## 固定数据流

1. 运行 preflight，要求隔离 VM、Git 忽略、可执行文件名、端口、SunnyNet 驱动、证书和监听器基线全部通过。
2. 额外检查当前进程未提升权限；提升状态直接以 `smoke_preflight_failed` 结束，不生成 CA。
3. 创建 job ID、单次 CA、DER 证书文件和私钥，并应用现有受限 ACL。
4. 显示唯一允许的 CurrentUser 变更计划，等待精确的 `CERT_APPLY`。
5. 按目标 SHA-256 进行安装前检查；必须为零个匹配。已有匹配时不得删除既有证书。
6. 调用 `.NET X509Store.Add()`，再枚举并要求恰好一个匹配。
7. 仅对该精确匹配对象调用 `Remove()`，再枚举并要求零个匹配。
8. 删除证书文件、私钥、秘密目录和运行时状态，写入冒烟收据。
9. 操作员再次运行 preflight；所有基线必须通过。
10. VM 正常关机并强制回滚 `wechat-login-baseline`。即使所有清理检查成功，也不得复用该 VM 状态进行真实采集。

## 错误与清理语义

CLI、日志和收据只允许以下错误码：

- `smoke_preflight_failed`
- `smoke_approval_rejected`
- `smoke_certificate_preexisting`
- `smoke_install_failed`
- `smoke_install_verification_failed`
- `smoke_remove_failed`
- `smoke_remove_verification_failed`
- `smoke_secrets_cleanup_failed`

底层异常只用于进程内控制流，不得通过 `error.Error()` 进入普通输出。若同时存在主操作错误和清理错误，清理错误优先决定最终失败码，因为无法证明恢复基线比安装失败本身风险更高。

安装前匹配数为零是所有权依据。若 `Add()` 的结果不确定，清理路径必须重新枚举：零个匹配表示无需删除；恰好一个匹配表示本次任务拥有并必须删除；多个匹配时不得批量删除，结果为 `smoke_remove_verification_failed`，随后立即关机并回滚快照。

未输入精确 `CERT_APPLY` 时使用 `smoke_approval_rejected`，不得执行证书安装。无论确认被拒绝、安装失败、验证失败还是删除失败，证书文件和私钥都必须进入同一个幂等清理路径。只有目标匹配数为零、秘密文件已删除且运行时状态已移除时，冒烟收据的 `success` 才能为 `true`。

## 测试策略

主机侧测试不得修改任何真实证书库：

- 静态边界测试要求脚本包含 `X509Store`、`Root`、`CurrentUser`、`ReadWrite`、`Add`、`Remove` 和 SHA-256 枚举；拒绝 `Import-Certificate`、`certutil` 与 `LocalMachine`。
- 纯 Go fake 覆盖安装前已有证书、安装命令失败、安装后零/多匹配、删除失败、删除后仍存在、秘密清理失败和幂等清理。
- CLI 测试证明未提升权限与精确 `CERT_APPLY` 均为硬门槛。
- 依赖边界测试证明 `cert-smoke` 不创建 bridge、proxy、SunnyNet、驱动或采集组件。
- 输出扫描测试用包含路径、指纹、Token 和异常文本的 fixture，证明普通输出只保留白名单字段和错误码。
- 现有 POC 单元/集成测试、源码构建和 `poc-security-audit.ps1` 必须全部通过。

实际写证书测试只允许发生在从 `wechat-login-baseline` 启动的隔离 VM 中。新介质必须从精确提交生成只读 ISO，挂载后核对 source commit、bundle SHA-256、清单中文件大小和全部 SHA-256，并在 VM 内从源码构建与重复安全审计。

## VM 验收流程

1. 从 `wechat-login-baseline` 启动 VM，确认微信无需参与本轮测试。
2. 挂载已验证的只读 ISO，在 VM 内源码构建并运行安全审计。
3. 运行 preflight 并保存仅包含布尔值和哈希的安全输出。
4. 以普通用户运行 `cert-smoke --ack-isolated-vm`，核对计划后输入 `CERT_APPLY`。
5. 要求 `certificate-smoke-receipt.json` 的整体 success 和安装、安装验证、删除、删除验证、秘密清理字段全部为 `true`，错误码为空。
6. 再次运行 preflight，要求全部布尔值为 `true`、`reason_codes:null`，证书、驱动、动态端口和监听器基线哈希与运行前一致。
7. 正常关闭 VM 并回滚 `wechat-login-baseline`。

任一条件失败均记为 `inconclusive`，不在宿主机复现、不运行真实采集，并直接回滚或销毁 VM。

## 成功标准与后续门禁

冒烟成功只证明以下能力：受限 POC 可以在隔离 VM 的 `CurrentUser\Root` 中创建、精确验证并删除单次 CA，且能证明敏感文件和系统状态恢复基线。

它不证明微信搜索、作品发现、一级评论、二级回复或字段映射可用。冒烟成功、VM 回滚后，必须由用户重新明确确认，才能编写或批准下一次真实采集运行计划。
