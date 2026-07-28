# 微信视频号评论 POC：证书安装诊断修订规格

状态：已确认

## 背景

真实验证在操作员输入 `APPLY` 后、`ca_installed` 事件之前失败，并成功执行清理。此前已修复 PowerShell 参数传递和 PEM 文件格式问题；VM 中的安全阶段日志仍只显示 `approval_received`、`cleanup_started`、`cleanup_completed`。因此需要在不泄露证书、路径或系统错误原文的前提下，使 CurrentUser 根证书导入在非交互运行中可判定。

## 决策

继续使用已审查的 `Import-Certificate` 路径，不改用 .NET `X509Store`，也不回退到 `LocalMachine`。导入命令必须：

- 固定目标为 `Cert:\CurrentUser\Root`；
- 显式使用 `-Confirm:$false`；
- 抑制 Warning、Information 和 Progress 输出；
- 标准输出只允许精确的 `true` 或 `false`；
- 继续对导入后的证书 RawData 计算 SHA-256，并与本次任务内存中的预期值比较；
- 失败或异常时仍执行现有的精确清理路径。

## 安全错误码

CLI 只能显示以下预定义错误码之一，不得输出底层 PowerShell 错误、文件路径、证书指纹、证书内容、私钥、Token、Cookie 或微信数据：

- `certificate_precheck_command_failed`
- `certificate_already_present`
- `certificate_import_command_failed`
- `certificate_import_reported_false`
- `certificate_postcheck_failed`

所有其他错误继续使用现有通用消息。错误码通过封闭的类型或白名单映射产生，不能直接使用 `error.Error()` 作为 CLI 输出。

## 数据流与清理

运行时在 `APPLY` 后调用证书存储。证书存储先按 SHA-256 检查 CurrentUser Root 不存在相同证书，再执行非交互导入，随后重新枚举并验证恰好一张匹配证书。任一步失败均返回白名单错误码；运行时继续执行现有 defer 清理。若导入实际成功但输出解析失败，后检查或清理必须识别并删除本次证书。

不启动 bridge、proxy、驱动或采集，除非证书安装与后检查均成功。

## 测试与验收

实现前新增失败测试，覆盖：

1. 安装脚本固定包含 CurrentUser Root、`-Confirm:$false` 和输出抑制，不包含 LocalMachine。
2. PowerShell 参数通过单一脚本块安全传递。
3. 生成的单证书 DER `.cert` 可通过 `Import-Certificate -WhatIf` 解析。
4. 命令失败、返回 `false`、后检查失败分别映射到不同白名单错误码。
5. CLI 只输出白名单错误码，不透传任意底层错误文本。
6. 已安装但后续失败时，清理收据确认 CurrentUser CA、私钥和临时证据均已移除。

完成后必须运行 POC 单元/集成测试、源码构建、`poc-security-audit.ps1`，重新制作只读 ISO，并挂载核对清单、提交号和全部 SHA-256。真实 VM 只允许再执行一次验证；若仍失败，使用新错误码形成 `inconclusive` 结论并进入最终清理，不再盲目重试。
