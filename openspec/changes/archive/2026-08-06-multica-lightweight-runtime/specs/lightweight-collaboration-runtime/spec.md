## Purpose

定义本地 Runtime 上的 Agent、Direct Chat、Agent-only Squad、Lightweight Run、Comment 与可靠任务队列共同形成的可观察协作闭环。

## ADDED Requirements

### Requirement: Local Runtime 驱动 Agent 执行
系统 SHALL 只允许 Agent 绑定同一 Workspace 中可用的 Local Runtime；Agent 创建时 Runtime 必填。Runtime 离线时任务 MUST 保留为明确可恢复状态或返回稳定错误，不得静默丢失。

#### Scenario: 创建可执行 Agent
- **WHEN** 有权限的用户使用同 Workspace 的 Local Runtime 创建 Agent
- **THEN** Agent 创建成功并可被授权调用

#### Scenario: Runtime 无效
- **WHEN** 创建或执行 Agent 时 Runtime 缺失、跨 Workspace、已删除或不可用
- **THEN** 系统拒绝请求或保留可恢复任务，并返回稳定错误

### Requirement: Runtime 删除需确认待解绑 Agent 集合
`DELETE /api/runtimes/{runtimeId}` SHALL 在该 Runtime 仍有未归档 Agent 时拒绝，并返回当前绑定集合供客户端展示。`POST /api/runtimes/{runtimeId}/unbind-agents-and-delete` SHALL 要求客户端提交 `expected_active_agent_ids`，并在事务内重新枚举实际未归档 Agent；集合不一致时 MUST 拒绝并返回最新集合，要求用户重新确认。由 Runtime Profile 派生的 Runtime 实例 MUST 拒绝直接删除，改为删除其 Profile。

因为 baseline 不使用外键，事务 MUST 通过对 Runtime 行与相关 Agent 行的显式锁来阻止并发新增或改绑，MUST NOT 依赖外键校验隐含的行锁。

#### Scenario: 确认集合与实际一致
- **WHEN** 用户确认的 Agent 集合与事务内枚举结果一致
- **THEN** 系统在同一事务内解绑这些 Agent 并删除 Runtime

#### Scenario: 确认期间集合发生变化
- **WHEN** 另一成员在用户确认期间为该 Runtime 新增或归档 Agent
- **THEN** 系统拒绝本次删除并返回最新集合
- **THEN** Runtime 与全部 Agent 绑定保持不变

### Requirement: Squad Leader 评估结果可记录
`POST /api/issues/{issueId}/squad-evaluated` SHALL 允许当前 Task 的 Squad Leader 记录一次评估结论，`outcome` 仅接受 `action/no_action/failed`，并可附带简短 `reason`。Issue 未分配给 Squad 时 MUST 拒绝。归因 MUST 来自可信 Task 上下文，客户端 MUST NOT 指定 Actor 或目标 Squad。

#### Scenario: Leader 记录无需行动
- **WHEN** Leader 判定新 Comment 不需要委派并提交 `no_action`
- **THEN** 系统记录评估结论且不创建新 Task

#### Scenario: 非 Squad Issue 或非法 outcome
- **WHEN** 目标 Issue 未分配给 Squad，或 `outcome` 不在冻结集合内
- **THEN** 系统返回 400 且不写入评估记录

### Requirement: Direct Chat 独立于 Issue
用户 SHALL 能为一个可调用 Agent 创建 Direct Chat Session、发送纯文本消息并产生 Chat Task。Chat Task MUST 仅关联 `chat_session_id`；Runtime 输出 SHALL 流式可见，完成、失败与取消 SHALL 收敛为持久化消息或可恢复草稿。

#### Scenario: 完成一轮聊天
- **WHEN** 用户向可用 Agent 发送带 Idempotency-Key 的消息且 Runtime 成功执行
- **THEN** 系统创建 Session、User Message 和单个 Chat Task
- **THEN** 流式输出最终收敛为一条 Assistant Message

#### Scenario: 取消运行中的聊天
- **WHEN** 用户取消 queued 或 running Chat Task
- **THEN** Task 最终进入 cancelled，Daemon 获得取消指令，未完成输入通过 Draft Restore 恢复

#### Scenario: 刷新后恢复进行中的回答
- **WHEN** 客户端重新加载 Session 并查询 pending task
- **THEN** 存在未终态 Chat Task 时返回其 `task_id/status/created_at`，客户端据此恢复流式展示
- **THEN** 不存在未终态 Task 时返回空结果而不是错误

### Requirement: Squad 只由 Agent 组成且恰好一个 Leader
Squad SHALL 只接受同 Workspace 且调用权限满足的 Agent Member。每个未归档 Squad MUST 始终恰好有一个 Leader，Leader MUST 同时存在于 roster 且角色为 `leader`；更换 Leader SHALL 原子更新新旧角色。

#### Scenario: 创建 Squad
- **WHEN** 用户选择合法 Agent 作为 Leader 创建 Squad
- **THEN** Squad 与 Leader roster 记录在同一事务中创建
- **THEN** roster 中恰好一个 `leader`

#### Scenario: 添加非法成员
- **WHEN** 用户尝试添加 Human、跨 Workspace Agent 或不可调用的私有 Agent
- **THEN** 系统拒绝操作且 roster 不发生部分变更

#### Scenario: 更换 Leader
- **WHEN** 用户把 Leader 更换为同 Workspace 且可调用的 Agent
- **THEN** 新 Leader 被原子 upsert 为 `leader`，旧 Leader 降为 `member`

### Requirement: Squad 归档保护历史与活动任务
`DELETE /api/squads/{squadId}` SHALL 表示归档而非物理删除。存在 `deferred/queued/dispatched/waiting_local_directory/running` Task 时 MUST 返回 409；成功归档后 MUST 拒绝新分配并保留历史 Issue 的 Squad Assignee，P0 不提供 Restore。

#### Scenario: 归档活动 Squad
- **WHEN** Squad 仍有 Active Task
- **THEN** 系统返回 `409 active_tasks_exist`
- **THEN** Squad、Task 与历史 Issue Assignee 均保持不变

#### Scenario: 归档空闲 Squad
- **WHEN** Squad 仅有终态或无 Task
- **THEN** Squad 被归档且不可接收新 Run
- **THEN** 历史 Issue 仍可按原 Squad Assignee 渲染

### Requirement: Lightweight Run 使用受限 Issue 状态机
Run SHALL 以 Issue 为根对象，支持 `backlog/todo/in_progress/in_review/done/blocked/cancelled` 状态以及 `agent/squad` Assignee。`blocked` SHALL 原生保存和展示；一次 Task 完成 MUST NOT 自动等同于 Issue 完成。

#### Scenario: 创建 Squad Run
- **WHEN** 用户创建 `todo` Issue 并分配给可用 Squad
- **THEN** 系统持久化 Issue 并只创建一个 Leader Task

#### Scenario: Backlog 推进
- **WHEN** 用户创建 `backlog` Issue 后更新为 `todo`
- **THEN** 初次创建不触发执行，状态推进时触发单个 Leader Task

#### Scenario: Leader 仅完成委派
- **WHEN** Leader Task 完成但只发布了 Member 委派
- **THEN** Issue 保持适当的非终态，而不会自动变成 `done`

### Requirement: Comment 追加写入并以结构化 Mention 触发
Issue Comment SHALL 仅支持 List/Create 且 append-only；Author、Workspace、Source Task 等归因必须来自可信上下文。只有结构化 `mention://agent` 或 `mention://squad` SHALL 触发任务，普通 `@name` 不触发。重复、跨 Workspace 和 Leader 自触发 MUST 被阻止。

#### Scenario: Leader 委派 Agent Member
- **WHEN** Leader 的结果 Comment 包含合法 `mention://agent` 且关联当前 Task
- **THEN** 系统为目标 Member 创建至多一个新 Task并记录 `source_task_id`
- **THEN** 不为 Leader 自身重复创建 Task

#### Scenario: 修改历史 Comment
- **WHEN** 客户端调用 Comment Update、Delete、Resolve 或 Reaction 路径
- **THEN** 路由返回 404 且原 Comment 不变

### Requirement: Leader-Member-Leader 闭环可重入
Member Task 的结果 Comment SHALL 能重新唤醒 Squad Leader；Leader MUST 能继续委派、要求修正或将 Issue 推进到 `in_review`。运行期间新增的用户 Comment 或显式 Mention MUST 通过 delivery/reconciliation 协议被有序处理，不得永久遗漏或造成 Leader 自触发循环。

#### Scenario: 单成员闭环
- **WHEN** 用户提交 Squad Run，Leader 委派 Member，Member 写回结果
- **THEN** Leader 被重新触发并能依据完整 Comment 上下文推进到 `in_review`

#### Scenario: Task 运行期间补充信息
- **WHEN** Agent 运行期间连续新增用户 Comment 或目标 Mention
- **THEN** 当前或 follow-up Task 按 `delivered_comment_ids/coalesced_comment_ids` 收敛处理
- **THEN** 补充内容不会永久遗漏或被重复执行

### Requirement: Task Queue 保持可靠生命周期
Task SHALL 使用 `deferred → queued → dispatched → waiting_local_directory/running → completed/failed/cancelled` 生命周期，支持 prepare lease、有限重试、并发单 Claim、orphan recovery、deferred promotion 与取消收敛。终态操作 SHALL 幂等且不得回退。

#### Scenario: 并发 Claim
- **WHEN** 两个 Daemon 同时 Claim 同一 eligible Task
- **THEN** 只有一个 Daemon 原子获得该 Task

#### Scenario: Lease 与目录等待
- **WHEN** dispatched Task 需要较慢目录准备或同目录已有运行
- **THEN** Daemon 可续 prepare lease 或进入 `waiting_local_directory`
- **THEN** Task 不会被错误重领或丢失

#### Scenario: 重复完成或失败
- **WHEN** Daemon 对同一 Task 重复发送 Complete 或 Fail
- **THEN** 系统返回同一终态结果且不重复 Comment、Message、Retry 或 Usage

### Requirement: Run 历史可稳定发现
Run List SHALL 使用 opaque cursor，默认 30、范围 1..100，并按 `updated_at DESC, id DESC` 稳定排序。Run Detail SHALL 分别分页返回按 `created_at ASC, id ASC` 排序的 Flat Comments 和 Task Runs；Task Run SHALL 携带 Agent/Runtime/Squad、Leader 标记、attempt、failure、usage 和生命周期时间。

#### Scenario: 同时间戳跨页浏览
- **WHEN** 多个 Run 具有相同 `updated_at` 且用户连续请求两页
- **THEN** 结果无重复、无遗漏且客户端不需要解析 cursor

#### Scenario: 查看 Run 详情
- **WHEN** 一个 Issue 同时存在 Comments 与多个 Task Runs
- **THEN** 客户端通过两个独立集合渲染完整历史，不调用 `/timeline` 或单 Task Detail
