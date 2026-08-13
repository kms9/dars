# lightweight-data-baseline Specification

## Purpose
定义全新 `dars_lightweight` PostgreSQL 数据库的身份保护、26 表最小结构、跨表一致性、敏感配置加密以及 Fresh Install 和恢复契约。
## Requirements
### Requirement: 从空库安装独立 Lightweight baseline
目标数据库 SHALL 默认为 `dars_lightweight`，并从空数据库安装新的 Lightweight baseline；启动链 MUST NOT 回放完整产品的 293 个 up migrations，也不得自动升级、导入、重命名、覆盖或复用原 `dars` 数据库。

#### Scenario: Fresh Install
- **WHEN** migrate 连接空的 `dars_lightweight`
- **THEN** 安装 Lightweight baseline 与 schema marker
- **THEN** Server、Web 和 Daemon 可以完成全部 P0 主流程

#### Scenario: 原完整产品数据库保留
- **WHEN** 全部目标验收运行完成
- **THEN** 原 `dars` 数据库内容与 revision checksum 未发生变化

### Requirement: DDL 前执行数据库身份保护
`migrate` SHALL 在任何 DDL 前校验 PostgreSQL `server_version_num >= 150000` 和当前数据库名；空库只在版本与名称匹配时安装 baseline，非空库必须先验证现有 marker。Server SHALL 在监听业务端口或执行任何业务写入前验证 PostgreSQL 版本、数据库名等于 `EXPECTED_DATABASE_NAME`、`schema_metadata.edition == dars_lightweight` 且 schema version 位于支持范围。测试后缀库必须显式配置预期名称并使用正确 marker。默认部署使用 PostgreSQL 17；低于 15 的实例不兼容 baseline 的 `NULLS NOT DISTINCT`，不得进入目标建库或迁移流程。

#### Scenario: 连接错误名称或 edition
- **WHEN** migrate 或 Server 连接 `dars`、其他未授权名称、缺少 marker、错误 edition 或不支持版本
- **THEN** 启动以 `database_identity_mismatch` 类诊断失败
- **THEN** 在失败前未执行 DDL 或业务写入

#### Scenario: PostgreSQL 版本过低
- **WHEN** migrate 或 Server 连接 `server_version_num < 150000` 的实例
- **THEN** 在任何 DDL、业务写入、Worker 或监听前失败并报告最低版本要求

#### Scenario: Reset 保护
- **WHEN** `make db-reset` 的目标不是 `dars_lightweight` 或 `dars_lightweight_<suffix>`
- **THEN** 命令拒绝且目标数据库不被修改

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

### Requirement: 数据约束同时由数据库与 Service 保证
系统 MUST 对适用的行内规则使用 CHECK/唯一索引，并由 Service 事务验证跨表租户关系。至少保证：所有枚举仅接受冻结集合；`max_concurrent_tasks >= 1`；`max_attempts >= attempt >= 1`；计数、seq、elapsed 和 usage 非负；Task 的 `issue_id` 与 `chat_session_id` 恰好一个非空；只有 Issue Task 可设置 Squad/Leader/trigger 字段；终态要求 `completed_at` 且不可回退；idempotency key 与 request hash 同空同非空。

#### Scenario: 写入非法 Task
- **WHEN** 写入双根或无根 Task、非法状态、倒置 attempt、伪造完成时间或负计数
- **THEN** 数据库或 Service 拒绝，事务不产生部分行

### Requirement: 跨表关系由显式事务维护
任何 migration MUST NOT 添加数据库 Foreign Key或 Cascade。Service SHALL 验证 Comment/Issue Workspace、Task/Runtime Workspace、Agent/Runtime Workspace、Builder/Agent/Chat Workspace、Squad/Leader/Agent member Workspace、Mention target Workspace、Invocation permission与 Task Token范围；需要原子性的 Create/Update/Delete/Archive MUST 使用事务和必要锁。

#### Scenario: 跨租户关系写入
- **WHEN** 请求把 Agent、Runtime、Builder、Squad Agent member、Issue、Comment、Task或 Mention关联到不同 Workspace
- **THEN** Service拒绝整个事务且不留下孤儿或跨租户引用

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
数据库中的 `custom_env` 每个 value 与完整 `mcp_config` document MUST 使用 `{v,kid,nonce,ciphertext}` envelope 加密，密钥来自合法的 `DARS_AGENT_SECRET_KEY`。Server 只可在授权 Env Reveal 或生成 Daemon Claim 时解密；普通 DTO、列表、日志、Realtime 与审计只返回 key/redacted metadata，MCP 不提供明文 Reveal API。Env Reveal/Update MUST 写入审计。

#### Scenario: Secret at rest 与授权使用
- **WHEN** 用户保存 Env/MCP Secret 后直接查询数据库并检查日志/Realtime
- **THEN** 仅可见版本化密文或 redacted metadata
- **WHEN** Agent Owner 或 Workspace Owner/Admin Reveal Env，或 Daemon Claim 需要执行配置
- **THEN** 授权路径可解密且 activity_log 记录审计

#### Scenario: 生产密钥缺失
- **WHEN** production 未配置合法 base64 32-byte `DARS_AGENT_SECRET_KEY`
- **THEN** `/readyz` 不 Ready 且系统不以明文降级

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

### Requirement: 配置、CI 与恢复只使用新库契约
环境样例、Makefile、Compose、环境脚本、CI、检查脚本和 migrate 命令 SHALL 默认使用 `POSTGRES_DB=dars_lightweight`、对应 `DATABASE_URL`、`EXPECTED_DATABASE_NAME=dars_lightweight` 和 `DARS_EDITION=lightweight`。`make check` MUST 使用独立 Fresh Database，CI MUST NOT 创建或迁移名为 `dars` 的数据库。

#### Scenario: Fresh CI
- **WHEN** CI 从空环境执行 `make check`
- **THEN** 它创建独立 Lightweight 测试库、安装新 baseline 并完成验证
- **THEN** 不连接或迁移 `dars`

### Requirement: Backup/Restore 恢复结构与 Secret 可用性
备份与恢复 SHALL 使用新库 DSN；加密密钥 MUST 通过独立 Secret Store 恢复，不写入数据库、日志或备份包。恢复后 MUST 重新执行 name/edition/version guard、Secret decrypt probe 和 P0 smoke test。

#### Scenario: 备份恢复
- **WHEN** 备份 `dars_lightweight` 并恢复到授权目标库，同时从 Secret Store 注入原密钥
- **THEN** schema guard 通过、Secret 可正确解密且 P0 主流程继续运行

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
- **WHEN** 合法 `dars_lightweight`数据库从前一支持版本执行 migration
- **THEN** 现有 user Agent、Agent-only Squad、Issue、Task、Chat和 Skill数据保持可读可执行
- **THEN** 新 manifest精确为27表且不连接或修改原 `dars`数据库

### Requirement: Builder draft 只持久化批准的非 Secret 配置
`agent_builder_draft` SHALL 只持久化 session/workspace关联、版本、Instructions、Skill IDs及其他批准的非 Secret Agent draft字段；它 MUST NOT 接受 Env Secret、MCP配置、Integration配置、owner/identity/history或只读 lifecycle字段。

#### Scenario: Builder draft 夹带未批准字段
- **WHEN** Builder draft payload包含 Env value、MCP、Integration、owner、archive或运行历史字段
- **THEN** Server拒绝整个保存且数据库、chat metadata、日志中不出现这些值

### Requirement: Tool Registry 使用 Workspace-scoped 专用数据模型
Lightweight PostgreSQL SHALL 以专用 Tool Source、Tool Definition、ToolBundle、ToolBundle Item、Agent current Bundle pointer、Task/Task Token Bundle pin、Tool Source Artifact 与 Tool Source Secret 数据保存 Server-side Gateway 状态。每行 SHALL 带有直接或可验证的 Workspace scope；Source revision、Definition public name、Bundle identity/item、Agent pointer、Task pin 与 Secret/Artifact identity MUST 具有阻止同 Workspace 重复或跨 Workspace 关联的唯一约束。

数据库 MUST NOT 添加 Foreign Key、Cascade 或通用 Attachment/Integration 表。Server SHALL 在应用层验证 Source/Definition/Bundle/Bundle Item/Artifact/Secret、Agent、Task、Token 与 Workspace 关系；需要原子性的 Source revision publish、Bundle publish、Agent pointer switch、Task pin、disable/revoke/delete 与 Workspace cleanup MUST 使用事务、必要锁和显式依赖顺序。

#### Scenario: 创建 Source 并发布 Bundle
- **WHEN** Admin 在 Workspace 创建 ready Source 并为同 Workspace Agent 发布选择快照
- **THEN** 专用表保存一致的 Workspace scope、revision、definitions、不可变 Bundle/items 与 Agent pointer，且不会构造通用 Integration/Attachment 数据

#### Scenario: 跨 Workspace 或悬空关系
- **WHEN** 请求尝试把其他 Workspace Definition 放入 Bundle、把 Bundle 固定到其他 Workspace Task，或删除 retained Bundle 仍引用的 Source/Artifact/Secret
- **THEN** Server 在应用层事务中拒绝或按冻结顺序显式清理，且不产生部分写入

### Requirement: ToolBundle 与 Task pin 具有数据库级不可变契约
ToolBundle header/item SHALL 只通过 publish transaction 创建，普通 update query MUST NOT 修改 manifest、item exported name、Definition canonical name、Source revision、schema、invocation plan 或 artifact digest。Agent current Bundle pointer MAY 原子切换；`agent_task_queue.tool_bundle_id` 在 Task 创建后 MUST NOT 随 Agent pointer、Claim 或重新 Claim 改变；Task Token 记录 SHALL 保存或可权威解析同一 Bundle scope。

Bundle content hash SHALL 用于完整性检查和诊断，不得作为历史 Bundle ID 复用键。Bundle revoke、Source/tool disable、Secret rotation 与运行 policy SHALL 使用独立状态，MUST NOT 通过改写 Bundle items 实现。

#### Scenario: 配置变化与 Task 并发
- **WHEN** Agent 发布 B2 的同时已有 Task 固定 B1，或同一 Task 重新 Claim
- **THEN** 事务和必要锁保证 Agent head、Task pin、Token scope 与 Claim URL 各自一致，不出现 B1/B2 混合

#### Scenario: 尝试更新 Bundle item
- **WHEN** 任何 query 或服务尝试就地修改已发布 Bundle item
- **THEN** 写入路径不存在或被拒绝，变更必须发布新 Bundle ID

#### Scenario: 只修改 Agent exported name
- **WHEN** Admin 保持 Definition selection 不变但修改任一 exported name
- **THEN** 新 Bundle item 保存新 exported name 与同一 canonical identity，历史 Bundle item 和 Task pin 不被改写

### Requirement: Tool Source Secret 使用版本化密文 envelope
Tool Source 的 auth headers、tokens、gRPC metadata、TLS private material 与 Remote MCP credential SHALL 使用 `{v,kid,nonce,ciphertext}` authenticated encryption envelope 持久化，密钥来自生产 readiness 已校验的部署 Secret。数据库、备份、普通 DTO、MCP payload、Claim、日志、metrics、Realtime、validation error 与 activity audit MUST NOT 保存或返回 credential 明文。

Server SHALL 只在授权 Source validation 或 invocation scope 内解密，并在使用结束后释放明文引用。Task Claim 只能包含 Server `/bundles/{bundleId}/mcp` 所需的当前 task-scoped `dat_` token，MUST NOT 包含任何 upstream credential。

#### Scenario: Secret at rest 与调用时解密
- **WHEN** Admin 保存 Source credential 后检查持久化与观测输出，并由授权 Task 调用 tool
- **THEN** 静态输出仅可见版本化密文或 redacted metadata，只有 Server outbound invocation 临时获得明文

#### Scenario: 生产加密配置缺失
- **WHEN** production 缺少合法 Tool Source secret encryption key material
- **THEN** Server readiness 不通过，且不以明文、弱加密或禁用审计方式降级

### Requirement: Task Token 与动态 MCP 配置不得持久化明文
Task Token SHALL 继续只持久化 hash 与非 Secret scope metadata，其中 MAY 包含 Task 固定 Bundle ID。Claim 动态生成的 `Authorization: Bearer dat_...` Gateway entry MUST 只存在于当前 Claim response 和受保护的 Runtime task config 生命周期，MUST NOT 写回 `agent.mcp_config`、`runtime_mcp_overlay`、ToolBundle、Tool Source Secret、Tool Definition、activity log、Realtime 或数据库备份。

#### Scenario: Claim 后检查数据库与日志
- **WHEN** Server 为绑定 Agent Claim 动态注入 Gateway MCP entry 后检查所有相关表、日志、Realtime 与审计
- **THEN** 只能找到 Task Token hash、Task/Agent/Workspace/Bundle scope 和不含 token 的配置 metadata，不能恢复 Claim 中的明文 token

### Requirement: Tool Registry 索引使用独立并发迁移
Tool Source、Definition、Bundle、Bundle Item、Agent pointer、Task/Token pin、Artifact 与 Secret 的每个普通或唯一索引 SHALL 使用独立单语句 `CREATE INDEX CONCURRENTLY` 或 `CREATE UNIQUE INDEX CONCURRENTLY` migration，并在后续 migration 中绑定需要的 unique/primary constraint。Schema manifest、query contract、Workspace delete/Bundle retention 顺序与迁移验收 SHALL 同步更新。

#### Scenario: 验证新迁移
- **WHEN** 在 Fresh Lightweight PostgreSQL 执行全部 up migrations 并扫描新增 schema
- **THEN** 不存在 Foreign Key/Cascade，所有新增索引均来自独立 concurrent migration，manifest 与实际 schema 一致
