## REMOVED Requirements

### Requirement: Baseline 恰好创建 26 张应用表
**Reason**: AI Builder需要持久化可续接 draft；固定26表无法表达批准的 Builder生命周期。

**Migration**: 以“Baseline恰好匹配批准的27表 schema manifest”替代，并通过从26表 baseline到27表的 forward migration、数据回填和 manifest contract test完成切换。

## ADDED Requirements

### Requirement: Baseline 恰好匹配批准的 27 表 schema manifest
目标 schema SHALL 恰好包含以下27张应用表；migration工具自己的版本表不计入数量，未列出的完整产品表 MUST NOT 创建。

```text
schema_metadata
user
verification_code
workspace
member
personal_access_token
daemon_token
task_token
runtime_profile
agent_runtime
agent
agent_builder_draft
skill
skill_file
agent_skill
agent_invocation_target
squad
squad_member
issue
comment
agent_task_queue
task_message
task_usage
activity_log
chat_session
chat_message
chat_draft_restore
```

#### Scenario: Schema allowlist
- **WHEN** 验收查询 `pg_catalog`的应用表并与 checked-in schema manifest比较
- **THEN** 表集合与上述清单精确相等且数量为27
- **THEN** 不存在 Integration、Invitation、Attachment、Project、Autopilot、Channel、Inbox、Billing、VCS、Quick Action、Label、Property、Subscriber、Reaction或 Usage Rollup表

#### Scenario: 从 26 表 baseline 升级
- **WHEN** 合法 `multica_lightweight`数据库从前一支持版本执行 migration
- **THEN** 现有 user Agent、Agent-only Squad、Issue、Task、Chat和 Skill数据保持可读可执行
- **THEN** 新 manifest精确为27表且不连接或修改原 `multica`数据库

### Requirement: Builder draft 只持久化批准的非 Secret 配置
`agent_builder_draft` SHALL 只持久化 session/workspace关联、版本、Instructions、Skill IDs及其他批准的非 Secret Agent draft字段；它 MUST NOT 接受 Env Secret、MCP配置、Integration配置、owner/identity/history或只读 lifecycle字段。

#### Scenario: Builder draft 夹带未批准字段
- **WHEN** Builder draft payload包含 Env value、MCP、Integration、owner、archive或运行历史字段
- **THEN** Server拒绝整个保存且数据库、chat metadata、日志中不出现这些值

## MODIFIED Requirements

### Requirement: 字段结构支持冻结的运行协议
Baseline SHALL 提供以下字段组，并遵守：UUID id默认生成；时间为 UTC `timestamptz`；JSON集合默认 `{}`或 `[]`而非 SQL NULL；`created_at/updated_at`在适用表上非空。字段名、可空性、枚举和默认值 SHALL 与本 change更新后的数据库 retention spec和 checked-in schema manifest精确一致。

- 身份与 Workspace：schema edition/version；User profile；验证码 hash/expiry/attempts/use；Workspace settings/repos/issue counter；Member role；PAT、Daemon Token与 Task Token的 hash、作用域和有效期。
- Runtime 与 Agent：Local Runtime identity/status/profile/last seen；Agent `kind=user|system`/system key、avatar、runtime binding、模型、thinking/service tier、permission、concurrency、Instructions、skills、invocation targets、archive与加密配置；Builder draft与 carrier/chat的唯一关联。
- Squad 与 Run：头像、唯一 Agent Leader、Agent-only roster与 role、Instructions；Issue状态/assignee/creator/criteria/context/number/idempotency；append-only Comment归因；Task状态、lease、recovery、comment delivery、session、usage、attribution与生命周期时间。
- Direct Chat：Session的 Agent/Runtime/status/session/workdir；Message的 role/content/task/failure/idempotency；取消后的 Draft Restore。

#### Scenario: 生成代码对齐 baseline
- **WHEN** 从新 baseline生成 sqlc model/query并运行目标 contract tests
- **THEN** 所有目标 DTO和访问路径可由冻结字段表达
- **THEN** 不再引用已退出表或旧字段

#### Scenario: System carrier 默认不可见
- **WHEN** 普通 Agent、Chat、activity、run count或 invocation查询读取数据库
- **THEN** 默认只返回 `kind=user`，Builder system carrier仅通过授权 Builder session路径可达

### Requirement: 跨表关系由显式事务维护
任何 migration MUST NOT 添加数据库 Foreign Key或 Cascade。Service SHALL 验证 Comment/Issue Workspace、Task/Runtime Workspace、Agent/Runtime Workspace、Builder/Agent/Chat Workspace、Squad/Leader/Agent member Workspace、Mention target Workspace、Invocation permission与 Task Token范围；需要原子性的 Create/Update/Delete/Archive MUST 使用事务和必要锁。

#### Scenario: 跨租户关系写入
- **WHEN** 请求把 Agent、Runtime、Builder、Squad Agent member、Issue、Comment、Task或 Mention关联到不同 Workspace
- **THEN** Service拒绝整个事务且不留下孤儿或跨租户引用

### Requirement: 索引采用并发独立迁移
每个索引，包括唯一索引和新表索引，MUST 使用 `CREATE [UNIQUE] INDEX CONCURRENTLY`，且每个 index build独占一个单语句 migration。Table migration不得通过 PK/UNIQUE隐式建索引；唯一约束 SHALL 先并发建索引，再在后续 migration使用该索引绑定。索引集合 MUST 覆盖冻结的 Token lookup、Workspace ownership、Builder session/system key、Agent-only roster/Leader、幂等键、stable cursor、Task claim/lease/deferred、Chat/Comment/Activity顺序与 Agent/Squad访问路径。

#### Scenario: Migration 静态审计
- **WHEN** 发布流程检查全部 Lightweight migrations
- **THEN** 无 FK/CASCADE、无隐式索引、无多语句 concurrent index文件
- **THEN** 更新后的 retention spec和 schema manifest列出的唯一与访问索引全部存在

### Requirement: Workspace Delete 显式且隔离清理
Workspace Delete SHALL 仅允许 Owner，在无 Active Task和在线 Daemon时，以事务按依赖顺序显式删除全部27表中的直接或间接 Workspace数据；不得删除 `schema_metadata/user/verification_code/personal_access_token`，不得影响同一 User的其他 Workspace，也不得自动删除失去最后 Workspace的 User。事务成功后 SHALL 显式清理该 Workspace绑定的 Agent/Squad avatar objects；对象清理失败必须产生可运维诊断，不得把数据库删除伪装成完全清理。

#### Scenario: 删除空闲 Workspace
- **WHEN** Owner删除无 Active Task、无在线 Daemon的测试 Workspace
- **THEN** 27表中的该 Workspace业务数据被显式清理，无 Builder draft或其他敏感残留
- **THEN** User、PAT与其他 Workspace保持不变，Agent/Squad avatar objects被删除或明确报告清理失败
