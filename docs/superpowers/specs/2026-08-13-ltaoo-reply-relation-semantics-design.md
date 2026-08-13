# ltaoo 回复关系语义修订设计

## 1. 背景

真实受限探针成功读取一级评论第一页和回复第一页，但因一条回复的 `replyCommentId` 不等于选中根评论而停止。nobiyou 现有模型分别保存 `ParentCommentID`、`RootCommentID` 和 `RetrievalRootCommentID`，说明 `replyCommentId` 是直接父评论，不应被强制解释为一级根评论。

本修订只调整回复关系判定和匿名摘要。请求上限、首个合格根评论选择、游标连续性、去重、安全清理和不保存原始内容的边界保持不变。

## 2. 采用方案

探针分别处理三种关系：

- **检索根**：操作员选择并传给回复接口的一级评论 ID，只存在于进程内存；摘要只保留带盐哈希。
- **来源根**：回复对象的 `rootCommentId`。非空且不为 `"0"` 时必须等于检索根。
- **直接父项**：回复对象的 `replyCommentId`。它可以指向检索根，也可以指向另一条回复。

不采用“忽略直接父项”，因为它会丢失嵌套回复能力证据；也不继续复用旧的单一匹配/缺口/冲突分类，因为它无法表达父项和根项的不同语义。

## 3. 来源根规则

每条独立回复的 `rootCommentId` 分类为：

- `match`：显式等于检索根；
- `gap`：字段缺失、空字符串或 `"0"`；
- `mismatch`：存在其他显式值。

任意 `root_mismatch` 都以 `reply_root_relation_mismatch` 失败，并在发送下一次评论请求前停止。`gap` 只表示来源未提供能力证据，不单独失败。

## 4. 直接父项规则

在受限请求结束后，使用内嵌回复和所有已获取回复页的全部非空回复 ID，对每条回复的 `replyCommentId` 分类：

- `root`：等于检索根；
- `known_reply`：等于已观察到的另一条回复 ID；允许父项出现在当前回复之前或之后；
- `unresolved`：是其他显式值，但不在当前受限窗口内；记录能力缺口，不失败；
- `gap`：字段缺失、空字符串或 `"0"`；
- `self`：等于回复自身 ID；以 `reply_parent_self_reference` 失败。

探针不尝试证明完整祖先链，不把 `unresolved` 误判为冲突，也不为了寻找父项增加页面请求。

## 5. 匿名摘要

每个回复页和总计增加以下计数：

- `root_relation_match_count`
- `root_relation_gap_count`
- `root_relation_mismatch_count`
- `parent_to_root_count`
- `parent_to_known_reply_count`
- `parent_unresolved_count`
- `parent_gap_count`
- `parent_self_reference_count`

保留现有 `relation_match_count`、`relation_gap_count` 和 `relation_mismatch_count` 作为兼容汇总：

- `relation_match_count = root match + parent root + parent known_reply`
- `relation_gap_count = root gap + parent gap + parent unresolved`
- `relation_mismatch_count = root mismatch + parent self`

摘要不得包含原始回复 ID、父 ID、根 ID、内容、昵称、账号或原始游标。现有带盐 ID 哈希与游标哈希规则不变。

## 6. 请求与停止语义

- 回复第一页解析后立即检查 `root mismatch` 和 `parent self`；任一命中都会阻止第二页请求。
- 合法的 `parent known_reply` 和 `parent unresolved` 不阻止第二页。
- 回复第一页存在非空 `lastBuffer` 时，第二页请求必须原样复用该游标；评论接口总请求数仍不得超过三次。
- 所有受限请求结束后，以内嵌回复、第一页和第二页的完整已观察 ID 集合统一生成父项分类，因此第一页父项可以被第二页中出现的 ID 解析；原始 ID 只保留在进程内存。

## 7. 自动化验收

测试必须证明：

1. 父项指向根评论时通过；
2. 父项指向同页另一回复时通过，包括父项出现在子项之后；
3. 父项跨页指向另一回复时通过，包括第一页指向第二页、第二页指向第一页；
4. 父项无法在受限窗口解析时只增加 `parent_unresolved_count`，仍可请求第二页；
5. 父项自引用时在下一次请求前失败；
6. 来源根显式冲突时在下一次请求前失败；
7. 来源根或父项缺失、为空或为 `"0"` 时只记录 gap；
8. 新旧汇总计数一致，原始关系值不出现在摘要或终端；
9. 现有请求上限、游标、去重、状态、清理和 PowerShell 语法测试继续通过。

## 8. 下一次真实验收

自动化测试通过后，使用新的 `run_id` 和公开多回复样本重新执行一次受限探针。只有取得 `verified_reply_two_pages`，并独立确认 Clash、临时 CA、进程、端口、密钥、配置和剪贴板均无残留，才认为两页回复与游标连续性已被真实验证。本修订不授权运行完整采集器。
