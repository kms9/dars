## Purpose

定义全新 `multica_lightweight` PostgreSQL 数据库的身份保护、26 表最小结构、跨表一致性、敏感配置加密以及 Fresh Install 和恢复契约。

## ADDED Requirements

### Requirement: 从空库安装独立 Lightweight baseline
目标数据库 SHALL 默认为 `multica_lightweight`，并从空数据库安装新的 Lightweight baseline；启动链 MUST NOT 回放完整产品的 293 个 up migrations，也不得自动升级、导入、重命名、覆盖或复用原 `multica` 数据库。

#### Scenario: Fresh Install
- **WHEN** migrate 连接空的 `multica_lightweight`
- **THEN** 安装 Lightweight baseline 与 schema marker
- **THEN** Server、Web 和 Daemon 可以完成全部 P0 主流程

#### Scenario: 原完整产品数据库保留
- **WHEN** 全部目标验收运行完成
- **THEN** 原 `multica` 数据库内容与 revision checksum 未发生变化

### Requirement: DDL 前执行数据库身份保护
`migrate` SHALL 在任何 DDL 前校验 PostgreSQL `server_version_num >= 150000` 和当前数据库名；空库只在版本与名称匹配时安装 baseline，非空库必须先验证现有 marker。Server SHALL 在监听业务端口或执行任何业务写入前验证 PostgreSQL 版本、数据库名等于 `EXPECTED_DATABASE_NAME`、`schema_metadata.edition == multica_lightweight` 且 schema version 位于支持范围。测试后缀库必须显式配置预期名称并使用正确 marker。默认部署使用 PostgreSQL 17；低于 15 的实例不兼容 baseline 的 `NULLS NOT DISTINCT`，不得进入目标建库或迁移流程。

#### Scenario: 连接错误名称或 edition
- **WHEN** migrate 或 Server 连接 `multica`、其他未授权名称、缺少 marker、错误 edition 或不支持版本
- **THEN** 启动以 `database_identity_mismatch` 类诊断失败
- **THEN** 在失败前未执行 DDL 或业务写入

#### Scenario: PostgreSQL 版本过低
- **WHEN** migrate 或 Server 连接 `server_version_num < 150000` 的实例
- **THEN** 在任何 DDL、业务写入、Worker 或监听前失败并报告最低版本要求

#### Scenario: Reset 保护
- **WHEN** `make db-reset` 的目标不是 `multica_lightweight` 或 `multica_lightweight_<suffix>`
- **THEN** 命令拒绝且目标数据库不被修改

### Requirement: Baseline 恰好创建 26 张应用表
目标 schema SHALL 恰好包含以下 26 张应用表；migration 工具自己的版本表不计入数量，未列出的完整产品表 MUST NOT 创建。

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
- **WHEN** 验收查询 `pg_catalog` 的应用表
- **THEN** 表集合与上述清单精确相等且数量为 26
- **THEN** 不存在 Invitation、Attachment、Project、Autopilot、Channel、Inbox、Billing、VCS、Quick Action、Label、Property、Subscriber、Reaction 或 Usage Rollup 表

### Requirement: 字段结构支持冻结的运行协议
Baseline SHALL 提供以下字段组，并遵守：UUID id 默认生成；时间为 UTC `timestamptz`；JSON 集合默认 `{}` 或 `[]` 而非 SQL NULL；`created_at/updated_at` 在适用表上非空。字段名、可空性、枚举和默认值 SHALL 与 `docs/prd/multica_lightweight_runtime_docs/03_database_retention_spec.md` 第 1.3 节冻结清单精确一致。

- 身份与 Workspace：schema edition/version；User profile；验证码 hash/expiry/attempts/use；Workspace settings/repos/issue counter；Member role；PAT、Daemon Token 与 Task Token 的 hash、作用域和有效期。
- Runtime 与 Agent：Local Runtime identity/status/profile/last seen；Agent runtime binding、模型、thinking/service tier、permission、concurrency、skills、invocation targets、archive 与加密配置。
- Squad 与 Run：唯一 Leader roster；Issue 状态/assignee/creator/criteria/context/number/idempotency；append-only Comment 归因；Task 状态、lease、recovery、comment delivery、session、usage、attribution 与生命周期时间。
- Direct Chat：Session 的 Agent/Runtime/status/session/workdir；Message 的 role/content/task/failure/idempotency；取消后的 Draft Restore。

#### Scenario: 生成代码对齐 baseline
- **WHEN** 从新 baseline 生成 sqlc model/query 并运行目标 contract tests
- **THEN** 所有目标 DTO 和访问路径可由冻结字段表达
- **THEN** 不再引用已退出表或旧字段

### Requirement: 数据约束同时由数据库与 Service 保证
系统 MUST 对适用的行内规则使用 CHECK/唯一索引，并由 Service 事务验证跨表租户关系。至少保证：所有枚举仅接受冻结集合；`max_concurrent_tasks >= 1`；`max_attempts >= attempt >= 1`；计数、seq、elapsed 和 usage 非负；Task 的 `issue_id` 与 `chat_session_id` 恰好一个非空；只有 Issue Task 可设置 Squad/Leader/trigger 字段；终态要求 `completed_at` 且不可回退；idempotency key 与 request hash 同空同非空。

#### Scenario: 写入非法 Task
- **WHEN** 写入双根或无根 Task、非法状态、倒置 attempt、伪造完成时间或负计数
- **THEN** 数据库或 Service 拒绝，事务不产生部分行

### Requirement: 跨表关系由显式事务维护
任何 migration MUST NOT 添加数据库 Foreign Key 或 Cascade。Service SHALL 验证 Comment/Issue Workspace、Task/Runtime Workspace、Agent/Runtime Workspace、Squad/Leader Workspace、Mention target Workspace、Invocation permission 与 Task Token 范围；需要原子性的 Create/Update/Delete/Archive MUST 使用事务和必要锁。

#### Scenario: 跨租户关系写入
- **WHEN** 请求把 Agent、Runtime、Squad、Issue、Comment、Task 或 Mention 关联到不同 Workspace
- **THEN** Service 拒绝整个事务且不留下孤儿或跨租户引用

### Requirement: Squad、Comment 与 Task 数据不变量
Leader SHALL 存在于同一 Squad roster 且 `role=leader`，任一未归档 Squad 恰好一个 Leader。Comment SHALL append-only，不生成 Update/Delete SQL；其 `source_task_id` 存在时必须指向同一 Workspace/Issue Task。`completed/failed/cancelled` SHALL 为 Task 终态。

#### Scenario: Leader 一致性冲突
- **WHEN** 写入零个或多个 Leader、跨 Workspace Leader 或删除当前 Leader 而未替换
- **THEN** Service 拒绝并保持原 roster

#### Scenario: 修改 Comment 或回退 Task
- **WHEN** 代码尝试更新/删除 Comment 或把终态 Task 改回运行态
- **THEN** 数据访问契约拒绝该操作

### Requirement: Issue 编号和幂等键保持唯一
`issue.number` SHALL 在 Workspace 事务内递增，`workspace_id + number` 唯一。Issue、Comment 与 Chat Message 的幂等唯一性 SHALL 按冻结 Actor/Workspace/route 范围实施；同 key 不同 hash MUST 返回 409。

#### Scenario: 并发创建 Issue
- **WHEN** 同一 Workspace 并发创建多个 Issue
- **THEN** 每个 Issue 获得唯一递增 number，且不会覆盖或重复

### Requirement: Token 和验证码只以 hash 比较
验证码、PAT、Daemon Token 和 Task Token SHALL 只持久化 hash；Token 明文不得进入普通 API、日志或备份。过期、已使用、revoked 或非当前 Task 的凭据 MUST fail closed。

#### Scenario: 检查数据库与备份
- **WHEN** 创建并使用所有 Token 后检查数据库、日志与备份
- **THEN** 只能找到 hash/prefix/metadata，不能恢复 Token 或验证码明文

### Requirement: Agent Secret 使用版本化密文 envelope
数据库中的 `custom_env` 每个 value 与完整 `mcp_config` document MUST 使用 `{v,kid,nonce,ciphertext}` envelope 加密，密钥来自合法的 `MULTICA_AGENT_SECRET_KEY`。Server 只可在授权 Env Reveal 或生成 Daemon Claim 时解密；普通 DTO、列表、日志、Realtime 与审计只返回 key/redacted metadata，MCP 不提供明文 Reveal API。Env Reveal/Update MUST 写入审计。

#### Scenario: Secret at rest 与授权使用
- **WHEN** 用户保存 Env/MCP Secret 后直接查询数据库并检查日志/Realtime
- **THEN** 仅可见版本化密文或 redacted metadata
- **WHEN** Agent Owner 或 Workspace Owner/Admin Reveal Env，或 Daemon Claim 需要执行配置
- **THEN** 授权路径可解密且 activity_log 记录审计

#### Scenario: 生产密钥缺失
- **WHEN** production 未配置合法 base64 32-byte `MULTICA_AGENT_SECRET_KEY`
- **THEN** `/readyz` 不 Ready 且系统不以明文降级

### Requirement: 索引采用并发独立迁移
每个索引，包括唯一索引和新表索引，MUST 使用 `CREATE [UNIQUE] INDEX CONCURRENTLY`，且每个 index build 独占一个单语句 migration。Table migration 不得通过 PK/UNIQUE 隐式建索引；唯一约束 SHALL 先并发建索引，再在后续 migration 使用该索引绑定。索引集合 MUST 覆盖冻结的 Token lookup、Workspace ownership、幂等键、stable cursor、Task claim/lease/deferred、Chat/Comment/Activity 顺序与 Agent/Squad 访问路径。

#### Scenario: Migration 静态审计
- **WHEN** 发布流程检查全部 Lightweight migrations
- **THEN** 无 FK/CASCADE、无隐式索引、无多语句 concurrent index 文件
- **THEN** `03_database_retention_spec.md` 第 17 节列出的唯一和访问索引全部存在

### Requirement: Workspace Delete 显式且隔离清理
Workspace Delete SHALL 仅允许 Owner，在无 Active Task 和在线 Daemon时，以事务按依赖顺序显式删除所有直接或间接 Workspace 数据；不得删除 `schema_metadata/user/verification_code/personal_access_token`，不得影响同一 User 的其他 Workspace，也不得自动删除失去最后 Workspace 的 User。

#### Scenario: 删除空闲 Workspace
- **WHEN** Owner 删除无 Active Task、无在线 Daemon的测试 Workspace
- **THEN** 26 表中的该 Workspace 业务数据被显式清理，无敏感残留
- **THEN** User、PAT 与其他 Workspace 保持不变

### Requirement: 配置、CI 与恢复只使用新库契约
环境样例、Makefile、Compose、环境脚本、CI、检查脚本和 migrate 命令 SHALL 默认使用 `POSTGRES_DB=multica_lightweight`、对应 `DATABASE_URL`、`EXPECTED_DATABASE_NAME=multica_lightweight` 和 `MULTICA_EDITION=lightweight`。`make check` MUST 使用独立 Fresh Database，CI MUST NOT 创建或迁移名为 `multica` 的数据库。

#### Scenario: Fresh CI
- **WHEN** CI 从空环境执行 `make check`
- **THEN** 它创建独立 Lightweight 测试库、安装新 baseline 并完成验证
- **THEN** 不连接或迁移 `multica`

### Requirement: Backup/Restore 恢复结构与 Secret 可用性
备份与恢复 SHALL 使用新库 DSN；加密密钥 MUST 通过独立 Secret Store 恢复，不写入数据库、日志或备份包。恢复后 MUST 重新执行 name/edition/version guard、Secret decrypt probe 和 P0 smoke test。

#### Scenario: 备份恢复
- **WHEN** 备份 `multica_lightweight` 并恢复到授权目标库，同时从 Secret Store 注入原密钥
- **THEN** schema guard 通过、Secret 可正确解密且 P0 主流程继续运行
