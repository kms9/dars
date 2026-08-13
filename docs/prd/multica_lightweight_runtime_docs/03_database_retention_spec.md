# Multica 数据库字段保留规范

> 基准提交：`736fbc8a5f1b22d48354a0e55baa00661e9e4326`  
> 校准日期：`2026-08-04`  
> 目标数据库：`multica_lightweight`  
> 文档状态：`当前 Schema 依赖已校准 / 27 表字段、约束、索引与配置契约已冻结`

## 1. 原则

目标不对原完整产品数据库做原地裁剪。当前 Schema 仅作为代码依赖盘点来源；目标从空数据库 `multica_lightweight` 创建全新的精简 baseline。

字段分为：

- 必须保留；
- 当前过渡依赖；
- 目标不创建；
- 最终可删除。

当前仓库包含 293 个 up migrations，但它们不进入目标数据库启动链。下文的“必须保留”表示新 baseline 的运行依赖；未列入目标表清单的完整产品表不在新库创建。所有关系继续由应用层维护；baseline/migration 不使用数据库外键或级联删除，新建索引使用独立的 `CREATE [UNIQUE] INDEX CONCURRENTLY` migration。

### 1.1 数据库身份保护

目标必须新增轻量 schema marker，例如：

```text
schema_metadata
├── edition = multica_lightweight
└── schema_version = <baseline/version>
```

数据库写入分为两个入口：

- 目标 PostgreSQL 最低版本为 15（默认部署使用 PostgreSQL 17）；baseline 使用 PostgreSQL 15 引入的 `NULLS NOT DISTINCT`，旧版本必须在任何 DDL 前失败；
- `migrate` 在任何 DDL 前先校验 PostgreSQL 版本与数据库名；若数据库为空，则安装 Lightweight baseline 和 marker；若数据库非空，则必须先校验现有 marker/version，禁止“猜测”其 edition；
- Server 不负责安装 baseline，在开始监听业务端口和执行任何业务写入前，必须完成以下三项校验。

Server 校验项：

- `server_version_num >= 150000`；
- 当前数据库名等于配置的预期名称，默认 `multica_lightweight`；
- `schema_metadata.edition == multica_lightweight`；
- schema version 位于服务支持范围。

任何一项不满足都应启动失败，避免把精简代码误连到原完整产品数据库。测试库可以使用其他名称，但必须显式配置 expected database name 并带有正确 edition marker。

### 1.2 目标 baseline 表清单

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

目标固定为以上 26 张应用表。Workspace Invitation、Attachment、Project、Autopilot、Channel、Inbox、Billing、VCS、Quick Action、Label、Property、Subscriber、Reaction 和 Usage Rollup 表不创建。Migration tool 自身的版本表不计入 26 张应用表。

### 1.3 字段级 baseline 契约

以下契约是目标 Schema 的权威输入。`?` 表示可空；`JSON` 表示 `jsonb`；`TS` 表示 `timestamptz`。除显式说明外：

- `id` 为 `uuid NOT NULL DEFAULT gen_random_uuid()`；
- `created_at/updated_at` 为 UTC `TS NOT NULL DEFAULT now()`；
- JSON 集合默认 `{}` 或 `[]`，不得使用 SQL NULL 表达空集合；
- 不创建数据库 Foreign Key 或 Cascade；关系、租户一致性和删除顺序由 Service 事务保证；
- Table migration 不声明会隐式建索引的 PK/UNIQUE；唯一索引使用独立单语句 `CREATE UNIQUE INDEX CONCURRENTLY` migration，再用后续 migration 绑定约束；
- Secret/Token 只持久化 hash 或密文；API 不返回 token/hash 或验证码 hash；Custom Env 明文只允许经专用 Env Reveal 权限与审计端点返回，MCP Secret 不提供明文 Reveal API。

`custom_env` 与 `mcp_config` 仍是 JSON API 字段，但数据库 JSONB 不得保存原文：`custom_env` 的每个 value、`mcp_config` 的完整 document 使用版本化 envelope `{v,kid,nonce,ciphertext}`。Server 只在授权 Env Reveal 或生成 Daemon Claim 时解密；列表、日志、Realtime 和审计只使用 key/redacted metadata。

#### Schema、身份与 Workspace

```text
schema_metadata
  id:smallint=1, edition:text="multica_lightweight",
  schema_version:int, installed_at:TS, updated_at:TS

user
  id, name:text, email:text, language:text="en",
  timezone:text?, created_at, updated_at

verification_code
  id, email:text, code_hash:text, expires_at:TS,
  used_at:TS?, attempts:int=0, created_at

workspace
  id, name:text, slug:text, description:text?, context:text="",
  settings:JSON={}, repos:JSON=[], issue_prefix:text,
  issue_counter:int=0, attribution_fail_closed:boolean=false,
  created_at, updated_at

member
  id, workspace_id:uuid, user_id:uuid,
  role:text(owner|admin|member), created_at

personal_access_token
  id, user_id:uuid, name:text, token_hash:text, token_prefix:text,
  expires_at:TS?, last_used_at:TS?, revoked:boolean=false, created_at

daemon_token
  id, token_hash:text, workspace_id:uuid, daemon_id:text,
  expires_at:TS, created_at

task_token
  id, token_hash:text, task_id:uuid, agent_id:uuid,
  workspace_id:uuid, user_id:uuid, expires_at:TS, created_at
```

首次验证码登录只创建 User；`POST /api/workspaces` 在一个事务内创建 Workspace、Owner Member 和 `issue_prefix/issue_counter` 初值。P0 不创建 Invitation。

#### Runtime、Agent 与 Skill

```text
runtime_profile
  id, workspace_id:uuid, display_name:text, protocol_family:text,
  command_name:text, description:text?, fixed_args:JSON=[],
  created_by:uuid, enabled:boolean=true, created_at, updated_at

agent_runtime
  id, workspace_id:uuid, daemon_id:text, name:text,
  runtime_mode:text(local)="local", provider:text, status:text(online|offline)="offline",
  device_info:text="", metadata:JSON={}, last_seen_at:TS?,
  owner_id:uuid, profile_id:uuid?, custom_name:text?,
  created_at, updated_at

agent
  id, workspace_id:uuid, runtime_id:uuid?, owner_id:uuid,
  name:text, description:text="", instructions:text="", avatar_url:text?,
  runtime_mode:text(local)="local", runtime_config:JSON={},
  status:text(idle|working|blocked|error|offline)="offline",
  max_concurrent_tasks:int=1, custom_env:JSON={}, custom_args:JSON=[],
  mcp_config:JSON={}, model:text?, thinking_level:text?, service_tier:text?,
  permission_mode:text(private|public_to), disabled_runtime_skills:JSON=[],
  archived_at:TS?, archived_by:uuid?, created_at, updated_at

skill
  id, workspace_id:uuid, name:text, description:text="",
  content:text="", config:JSON={}, created_by:uuid, created_at, updated_at

skill_file
  id, skill_id:uuid, path:text, content:text, created_at, updated_at

agent_skill
  agent_id:uuid, skill_id:uuid, enabled:boolean=true, created_at

agent_invocation_target
  id, agent_id:uuid, target_type:text(workspace|member),
  target_id:uuid, created_by:uuid, created_at
```

P0 不创建 Cloud Runtime、Composio allowlist、legacy daemon ID、Agent Label 或 Runtime Usage 字段/表。

#### Squad、Run、Comment 与 Task

```text
squad
  id, workspace_id:uuid, name:text, description:text="",
  instructions:text="", leader_id:uuid, creator_id:uuid,
  avatar_url:text?, archived_at:TS?, archived_by:uuid?,
  created_at, updated_at

squad_member
  id, squad_id:uuid, agent_id:uuid, role:text, created_at

issue
  id, workspace_id:uuid, title:text, description:text="",
  status:text(backlog|todo|in_progress|in_review|done|blocked|cancelled),
  assignee_type:text(agent|squad), assignee_id:uuid,
  creator_type:text(member|agent), creator_id:uuid,
  acceptance_criteria:JSON=[], context_refs:JSON=[], number:int,
  idempotency_key:text?, request_hash:text?,
  first_executed_at:TS?, created_at, updated_at

comment
  id, workspace_id:uuid, issue_id:uuid,
  author_type:text(member|agent|system), author_id:uuid?,
  content:text, type:text(message)="message", source_task_id:uuid?,
  idempotency_key:text?, request_hash:text?, created_at

agent_task_queue
  id, workspace_id:uuid, agent_id:uuid, runtime_id:uuid,
  issue_id:uuid?, chat_session_id:uuid?, squad_id:uuid?,
  is_leader_task:boolean=false,
  status:text(deferred|queued|dispatched|waiting_local_directory|running|completed|failed|cancelled),
  priority:int=0, attempt:int=1, max_attempts:int=2,
  parent_task_id:uuid?, session_id:text?, work_dir:text?,
  context:JSON={}, result:JSON?, error:text?, failure_reason:text?,
  trigger_summary:text?, force_fresh_session:boolean=false,
  handoff_note:text?, wait_reason:text?, prepare_lease_expires_at:TS?, fire_at:TS?,
  runtime_mcp_overlay:JSON={}, initiator_user_id:uuid?,
  originator_user_id:uuid?, accountable_user_id:uuid?, originator_source:text?,
  trigger_comment_id:uuid?, coalesced_comment_ids:uuid[]=[],
  delivered_comment_ids:uuid[]=[], chat_input_task_id:uuid?,
  chat_finalize_deferred_at:TS?, escalation_for_task_id:uuid?,
  delegated_from_task_id:uuid?, retry_of_task_id:uuid?, rerun_of_task_id:uuid?,
  trigger_evidence_kind:text?, trigger_evidence_ref_id:uuid?,
  session_rollout_missing:boolean=false, retired_session_id:text?,
  dispatched_at:TS?, started_at:TS?, completed_at:TS?, created_at

task_message
  id, task_id:uuid, seq:int, type:text, tool:text?,
  content:text?, input:JSON?, output:text?, created_at

task_usage
  id, task_id:uuid, provider:text, model:text,
  input_tokens:bigint=0, output_tokens:bigint=0,
  cache_read_tokens:bigint=0, cache_write_tokens:bigint=0,
  cost_usd_ticks:bigint?, created_at, updated_at

activity_log
  id, workspace_id:uuid, issue_id:uuid?,
  actor_type:text(member|agent|system), actor_id:uuid?,
  action:text, details:JSON={}, created_at
```

Active Task 定义固定为 `deferred/queued/dispatched/waiting_local_directory/running`。每个 Task 必须且只能关联 `issue_id` 或 `chat_session_id` 之一。Squad Member 只允许 Agent。

#### Direct Chat

```text
chat_session
  id, workspace_id:uuid, agent_id:uuid, creator_id:uuid,
  runtime_id:uuid, title:text, session_id:text?, work_dir:text?,
  status:text(active|archived), created_at, updated_at

chat_message
  id, chat_session_id:uuid, role:text(user|assistant|system),
  content:text, task_id:uuid?, failure_reason:text?, elapsed_ms:bigint?,
  message_kind:text(message|no_response)="message",
  idempotency_key:text?, request_hash:text?,
  created_at

chat_draft_restore
  id, chat_session_id:uuid, task_id:uuid,
  content:text, created_at
```

P0 不创建 Attachment、Quick Action、Pinned/Unread、Project Context 或 Channel Media 字段。

## 2. 身份、成员与令牌

目标运行时必须保留以下表或等价能力：

| 表 | 用途 |
|---|---|
| `user` | 登录身份和资源所有者 |
| `member` | Workspace 成员与角色 |
| `personal_access_token` | CLI/用户 API 身份 |
| `daemon_token` | Daemon 身份和 Workspace 边界 |
| `task_token` | Agent 执行期间的最小权限身份 |
| `verification_code` | 当前登录/验证流程 |

这些表属于 Auth、Workspace Guard、Daemon/Task Actor Context 的硬依赖。P0 不创建 Workspace Invitation。

## 3. `workspace`

必须保留：

- `id`
- `slug/name`
- `context/settings`
- `description`
- `repos`
- `issue_prefix`
- `issue_counter`
- `attribution_fail_closed`
- `created_at`
- `updated_at`

`issue_prefix`/`issue_counter` 支持可读 Issue 编号；`attribution_fail_closed` 参与运行归因安全策略，不能因为 UI 不展示而删除。

## 4. `agent_runtime` 与 `runtime_profile`

必须保留：

- `id`
- `workspace_id`
- `daemon_id`
- `name`
- `runtime_mode`
- `provider`
- `status`
- `device_info`
- `metadata`
- `last_seen_at`
- `owner_id`
- `profile_id`
- `custom_name`
- `created_at`
- `updated_at`

当前模型没有独立 `version`、`device_name` 或 `last_heartbeat_at` 列；相关信息可能存在于 `device_info`、`metadata` 或 `last_seen_at`。

新 baseline 不创建 `legacy_daemon_id`；目标 Daemon 和 Server 只使用正式 `daemon_id`。

自定义 Runtime 能力依赖 `runtime_profile` 的 `protocol_family/command_name/fixed_args/enabled` 等目标字段。

## 5. `agent`、Skills 与调用权限

必须保留：

- `id`
- `workspace_id`
- `runtime_id`
- `name`
- `description`
- `instructions`
- `runtime_mode`
- `runtime_config`
- `model`
- `thinking_level`
- `service_tier`
- `custom_args`
- `custom_env`
- `mcp_config`
- `max_concurrent_tasks`
- `permission_mode`
- `owner_id`
- `disabled_runtime_skills`
- `status`
- `archived_at`
- `archived_by`
- `created_at`
- `updated_at`

同时必须保留：

| 表 | 用途 |
|---|---|
| `skill` | Workspace Skill 定义 |
| `skill_file` | Skill 内容文件 |
| `agent_skill` | Agent-Skill 绑定与 enabled 状态 |
| `agent_invocation_target` | `public_to` Agent 的调用 allowlist |

`permission_mode` 不能脱离 `agent_invocation_target` 单独保留，否则会破坏 Agent 调用权限语义。

新 baseline 不创建旧 `visibility`、`kind/system_key` 或 `composio_toolkit_allowlist`；Agent Builder 的隐藏 System Agent 不进入 P0，调用授权只由 `permission_mode + agent_invocation_target` 决定。

## 6. `squad`

必须保留：

- `id`
- `workspace_id`
- `name`
- `description`
- `instructions`
- `leader_id`
- `creator_id`
- `avatar_url`
- `archived_at`
- `archived_by`
- `created_at`
- `updated_at`

## 7. `squad_member`

必须保留：

- `id`
- `squad_id`
- `agent_id`
- `role`
- `created_at`

目标只允许 Agent Member，不保留 `member_type/member_id` 多态关系。

## 8. `issue`

### 必须保留

| 字段 | 用途 |
|---|---|
| `id` | Squad Run 根 ID |
| `workspace_id` | 租户隔离 |
| `title` | 任务标题 |
| `description` | 目标、约束、验收 |
| `status` | 整体协作状态 |
| `assignee_type` | Agent/Squad |
| `assignee_id` | 实际负责人 |
| `creator_type` | Member/Agent |
| `creator_id` | 发起者 |
| `number` | CLI/页面可读 ID |
| `acceptance_criteria` | 当前验收结构 |
| `context_refs` | 当前上下文引用 |
| `idempotency_key/request_hash` | Create 防重与 Body 冲突检测 |
| `first_executed_at` | 首次运行时间 |
| `created_at` | 创建与排序时间 |
| `updated_at` | 活跃、排序、GC |

### 当前存在、目标 baseline 不创建

| 字段 | 处理 |
|---|---|
| `priority` | 轻量列表不使用优先级 |
| `parent_issue_id` | 不支持 Child Issue |
| `project_id` | 不支持 Project |
| `stage` | 不支持 Child Stage |
| `metadata` | 目标触发/归因改用显式字段 |
| `origin_type/origin_id` | 目标使用 Task attribution/evidence |
| `position` | 目标列表按 `updated_at/id` 排序 |
| `start_date/due_date` | 不支持项目日程 |
| `properties` | 不支持自定义属性 |

目标合法状态固定为 `backlog`、`todo`、`in_progress`、`in_review`、`done`、`blocked`、`cancelled`。

## 9. `comment`

### 必须保留

| 字段 | 用途 |
|---|---|
| `id` | Comment/Event ID |
| `issue_id` | 所属 Squad Run |
| `workspace_id` | 租户隔离 |
| `author_type` | member/agent/system |
| `author_id` | 作者和防自触发 |
| `content` | 命令、结果、补充 |
| `type` | 消息类型 |
| `source_task_id` | 结果来源 |
| `idempotency_key/request_hash` | Create 防重与 Body 冲突检测 |
| `created_at` | 顺序和增量 |

### 当前存在、目标 baseline 不创建

- `parent_id`；
- `quick_action_id`；
- `resolved_at`；
- `resolved_by_type`；
- `resolved_by_id`；
- Thread、Resolution、Quick Action 和 Reaction 关联表/索引。

`source_task_id` 是 Agent 结果归因、自触发抑制和 Leader 重入的重要证据，不得降级成普通展示字段。

## 10. `chat_session`

必须保留：

- `id`
- `workspace_id`
- `agent_id`
- `creator_id`
- `title`
- `session_id`
- `work_dir`
- `runtime_id`
- `status`
- `created_at`
- `updated_at`

目标 baseline 不创建：

- `project_id`
- External Channel Binding；
- 非必要 Agent Intro 字段。

## 11. `chat_message` 与取消恢复

必须保留：

- `id`
- `chat_session_id`
- `role`
- `content`
- `task_id`
- `failure_reason`
- `elapsed_ms`
- `message_kind`
- `idempotency_key`
- `request_hash`
- `created_at`

目标 baseline 不创建：

- `quick_actions`
- Channel Media；
- Channel Ingested；
- 外部渠道专用字段。

P0 保留 CH-06/CH-07“取消后输入可恢复”，因此保留 `chat_draft_restore` 及其幂等 consume 语义；目标记录不包含 Attachment IDs。

## 12. `agent_task_queue` 与任务附属表

必须保留：

- `id`
- `workspace_id`
- `agent_id`
- `runtime_id`
- `issue_id`
- `chat_session_id`
- `squad_id`
- `is_leader_task`
- `status`
- `priority`
- `attempt`
- `max_attempts`
- `parent_task_id`
- `session_id`
- `work_dir`
- `result`
- `error`
- `failure_reason`
- `context`
- `trigger_summary`
- `force_fresh_session`
- `handoff_note`
- `wait_reason`
- `prepare_lease_expires_at`
- `fire_at`
- `runtime_mcp_overlay`
- `initiator_user_id`
- `originator_user_id`
- `accountable_user_id`
- `originator_source`
- `trigger_comment_id`
- `coalesced_comment_ids`
- `delivered_comment_ids`
- `chat_input_task_id`
- `chat_finalize_deferred_at`
- `escalation_for_task_id`
- `delegated_from_task_id`
- `retry_of_task_id`
- `rerun_of_task_id`
- `trigger_evidence_kind`
- `trigger_evidence_ref_id`
- `session_rollout_missing`
- `retired_session_id`
- `created_at`
- `dispatched_at`
- `started_at`
- `completed_at`

停止使用：

- `autopilot_run_id`
- Quick Create Context；
- External Channel 专用上下文；
- Child Stage 专用字段。

新 baseline 同时不创建 `runtime_connected_apps`、`rule_version_id` 和 Quick Action regeneration 字段。`runtime_mcp_overlay` 仍保留，服务于目标 Agent Task 的运行期 MCP 配置，不绑定 Composio。

当前 Task 状态不仅有 `queued/dispatched/running/completed/failed/cancelled`，还包含 `deferred` 与 `waiting_local_directory`。以上 lease、comment delivery、retry lineage、attribution 和 chat finalize 字段是这些可靠性语义的一部分。

Task Usage 不在 `agent_task_queue.usage` 单列中，必须保留或替代：

| 表 | 用途 |
|---|---|
| `task_message` | Daemon/用户可读运行消息 |
| `task_usage` | 单 Task token/cost/usage 明细 |
| `task_usage_*` rollup/dirty/state | 目标 baseline 不创建 |
| `runtime_usage` 相关表 | 目标 baseline 不创建 |

目标退出 Dashboard 聚合报表，但保留 Agent Task 的原始消息和必要 Usage 记录。

## 13. `activity_log`

当前不仅记录 `squad_leader_evaluated`，Agent Env reveal/update 等安全操作也写入该表。目标保留：

- `workspace_id`
- `issue_id`
- `actor_type`
- `actor_id`
- `action`，固定覆盖 `squad_leader_evaluated`、`agent_env_revealed`、`agent_env_updated` 和 `agent_mcp_updated`
- `details`
- `created_at`

目标不得记录纯项目管理 Activity。新增安全/归因 action 必须先更新本清单、权限矩阵和验收用例。

## 14. P0 明确不创建的可选能力

以下能力已确定不进入 P0：

| 能力/表 | P0 处理 |
|---|---|
| Attachment | P0 只支持纯文本 Chat/Comment |
| `workspace_invitation` | P0 Member List 只读 |
| Agent Template/Builder | P0 使用普通 Agent Create/Edit |
| Cloud Runtime | P0 只支持 Local Runtime |
| Task/Runtime Usage Rollups | P0 只保留原始 `task_usage` |

## 15. 目标 baseline 不创建的表族

解耦完成后可以删除：

- Issue Properties；
- Labels；
- Subscribers；
- Reactions；
- Autopilot；
- Autopilot Run；
- Channel Installation/Binding；
- Inbox/Notification；
- Billing；
- VCS Issue Link；
- Child Stage 专用表；
- Quick Actions。

对应 Handler、Service、Realtime Event、Web、CLI、内置 Skill 和 sqlc query 必须在切换新 baseline 前解除依赖。因为目标使用新数据库，不创建 Drop Table migration，也不修改原完整产品数据库。

## 16. 关键约束

必须保证：

```text
comment.workspace_id == issue.workspace_id
task.runtime_id belongs to task workspace
agent.runtime_id belongs to agent workspace
squad.leader_id belongs to squad workspace
mention target belongs to issue workspace
task token may only operate on its authorized task/issue/session
agent invocation must satisfy permission_mode + invocation targets
```

仓库约定不新增数据库 FK/CASCADE；以上约束由应用层校验，并在需要原子性的操作中使用事务。

## 17. 目标访问路径与索引审计

目标 baseline 必须建立：

- 每张有 `id` 的表的唯一 ID 索引；
- `user(email)`、`workspace(slug)`；
- `member(workspace_id,user_id)`；
- `personal_access_token(token_hash)`、`daemon_token(token_hash)`、`daemon_token(workspace_id,daemon_id)`、`task_token(token_hash)`；
- `runtime_profile(workspace_id,display_name)`；
- `agent_runtime(workspace_id,daemon_id,provider,profile_id) NULLS NOT DISTINCT`；
- `agent_skill(agent_id,skill_id)`；
- `skill_file(skill_id,path)`；
- `agent_invocation_target(agent_id,target_type,target_id)`；
- `squad_member(squad_id,agent_id)`；
- `issue(workspace_id,number)`；
- `issue(workspace_id,creator_type,creator_id,idempotency_key) WHERE idempotency_key IS NOT NULL`；
- `comment(workspace_id,issue_id,author_type,author_id,idempotency_key) WHERE idempotency_key IS NOT NULL`；
- `chat_message(chat_session_id,idempotency_key) WHERE idempotency_key IS NOT NULL`；
- `task_message(task_id,seq)`；
- `task_usage(task_id,provider,model)`；
- `chat_draft_restore(chat_session_id,task_id)`。

同时建立以下访问索引：

```sql
issue(workspace_id, updated_at DESC, id DESC)
issue(workspace_id, status, updated_at DESC, id DESC)
issue(workspace_id, assignee_type, assignee_id, updated_at DESC, id DESC)
comment(issue_id, created_at, id)
comment(workspace_id, issue_id)
member(user_id, workspace_id)
verification_code(email, created_at DESC)
personal_access_token(user_id, revoked, expires_at)
daemon_token(workspace_id, daemon_id, expires_at)
task_token(task_id, expires_at)
runtime_profile(workspace_id, enabled, display_name)
agent(workspace_id, archived_at, updated_at DESC)
skill(workspace_id, updated_at DESC)
agent_task_queue(runtime_id, status, priority DESC, created_at)
agent_task_queue(workspace_id, status, created_at)
agent_task_queue(issue_id, created_at, id)
agent_task_queue(issue_id, agent_id, status)
agent_task_queue(chat_session_id, status)
agent_task_queue(status, fire_at) WHERE status = 'deferred'
agent_task_queue(status, prepare_lease_expires_at) WHERE status IN ('dispatched','waiting_local_directory')
agent_runtime(workspace_id, status, last_seen_at)
squad(workspace_id)
squad_member(squad_id)
chat_session(workspace_id, creator_id, updated_at)
chat_message(chat_session_id, created_at, id)
activity_log(workspace_id, created_at, id)
activity_log(issue_id, created_at, id)
```

每个 `CREATE [UNIQUE] INDEX CONCURRENTLY` 独占一个单语句 migration 文件。Primary/Unique Constraint 如需要，先并发创建唯一索引，再用后续 migration `USING INDEX` 绑定，不得让 Table DDL 隐式创建索引。

## 18. 已冻结的 baseline 策略

1. 创建新的 PostgreSQL 数据库，默认名 `multica_lightweight`；
2. 从空库执行新的 Lightweight baseline；
3. Table migration 与 concurrent index migration 按仓库约束拆分；
4. 基于 baseline 重新生成 sqlc models/queries；
5. Server、Web、CLI、Daemon 只面向新结构联调；
6. 不执行旧库 Upgrade，不在启动时自动导入旧数据；
7. 原数据库保持只读、不改名、不复用 DSN；
8. 发布验证覆盖 Fresh Install、Workspace Delete、Backup/Restore、并发 Claim 和 edition/name guard；
9. 上述验证通过后，从目标启动链和代码库清理不再使用的旧 migrations/queries/generated models。

如未来需要旧数据，只能另立“显式导出 → 映射 → 导入”工具需求；它不是当前轻量版发布范围。

## 19. 配置与命令契约

以下入口必须同步到新数据库：

```text
.env.example
Makefile
docker-compose.yml
docker-compose.selfhost.yml
scripts/ensure-postgres.sh
scripts/local-env.sh
scripts/init-worktree-env.sh
scripts/check.sh
.github/workflows/ci.yml
server/cmd/migrate
```

默认配置：

```text
POSTGRES_DB=multica_lightweight
DATABASE_URL=postgres://<user>:<password>@<host>:<port>/multica_lightweight?sslmode=<mode>
EXPECTED_DATABASE_NAME=multica_lightweight
MULTICA_EDITION=lightweight
MULTICA_AGENT_SECRET_KEY=<base64-encoded-32-byte-key>
```

- Worktree/Test DB 使用 `multica_lightweight_<suffix>`，同时显式设置 `EXPECTED_DATABASE_NAME`；
- `make setup/start/check` 只运行 Lightweight baseline；
- `make check` 每次使用独立 Fresh Database，不复用开发库；
- `make db-reset` 只允许数据库名为 `multica_lightweight` 或 `multica_lightweight_<suffix>`，否则拒绝；
- `migrate` 按第 1.1 节处理空库/非空库；Server 只接受已安装且 name + edition + schema version 均匹配的数据库；
- 远端 PostgreSQL 低于 15 时不得创建或迁移目标库，应先升级服务或提供 PostgreSQL 15+（推荐 17）的独立实例；
- CI 不启动或迁移名为 `multica` 的数据库；
- Production 缺少合法 `MULTICA_AGENT_SECRET_KEY` 时 `/readyz` 不 Ready；密钥不得写入数据库、日志或备份包；
- Backup/Restore 使用新库 DSN，密钥通过独立 Secret Store 恢复，并在恢复后重新执行 edition/schema guard、Secret decrypt probe 与 P0 Smoke Test。

## 20. 必须落入 DDL/Service 的约束

Baseline 和 Service 必须共同保证：

- 所有枚举值只接受本文列出的集合；`max_concurrent_tasks >= 1`、`max_attempts >= attempt >= 1`，Issue/Workspace counter、message seq、elapsed 和 usage 计数不得为负；
- `agent_task_queue` 的 `issue_id` 与 `chat_session_id` 必须且只能有一个非空；Issue Task 才允许 `squad_id/is_leader_task/trigger_comment_id`；
- `completed/failed/cancelled` 为终态并要求 `completed_at` 非空；非终态不得伪造完成时间；
- `comment` 是 append-only；目标不生成 Update/Delete SQL，`source_task_id` 如存在必须与同一 Workspace/Issue 的 Task 匹配；
- `squad_member.agent_id` 必须属于 Squad Workspace；Leader 必须同时存在于 roster 且 `role=leader`；
- `issue.number` 由 Workspace 事务内递增产生；`workspace_id + number` 唯一；
- Issue/Comment/Chat Message 的 `idempotency_key` 与 `request_hash` 必须同时为空或同时非空；命中相同 key 且 hash 不同返回 409；
- Token 比较只使用 hash；验证码过期/已使用、PAT revoked/expired、Daemon Token expired、Task Token 非当前 Task 时全部 fail closed；
- 无 FK/CASCADE 不等于弱一致性：Create/Update/Delete/Archive 需要跨表一致性时必须使用显式事务和锁；Workspace Delete 必须按清单显式删除所有 `workspace_id` 直接或间接关联行；
- Workspace Delete 不删除 `schema_metadata/user/verification_code/personal_access_token`；同一 User 的其他 Workspace 不受影响，失去最后一个 Workspace 的 User 也不自动删除；
- 以上规则必须同时具有数据库 CHECK/唯一索引（适用时）和 Service contract test；跨表租户关系由 Service/事务测试覆盖。
