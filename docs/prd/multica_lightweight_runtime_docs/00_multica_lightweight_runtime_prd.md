# Multica 轻量智能体与小队协作运行时需求文档

> 基础仓库：`kms9/multica`  
> 基准提交：`736fbc8a5f1b22d48354a0e55baa00661e9e4326`  
> 校准日期：`2026-08-04`  
> 文档状态：`现状已校准 / P0 产品与发布契约已冻结 / 目标待实现`  
> 文档目标：以当前代码契约为基线，保留本地 Daemon、Runtime、Agent、Direct Chat、Squad 与最小 Issue/Comment 协作闭环，分阶段退出完整项目管理外围能力。

## 0. 文档口径

本文严格区分三类描述：

- **当前现状**：基准提交中已经存在的代码、路由、数据模型或测试；
- **目标要求**：轻量版本发布时必须达到，但当前可能尚未实现；
- **待决策**：尚未冻结、会改变最终产品范围或验收契约的事项；本版 P0 已无此类开放项，新增范围必须另立变更。

当前仓库仍是完整产品：完整 Router、Handler、`TaskService`、`IssueService` 和数据库迁移历史同时承载项目管理、Autopilot、Inbox、Channel、Webhook、Billing、VCS 等能力。轻量版不是现状描述，而是从该基线抽取出来的目标产品形态。

### 0.1 已冻结决策

- 当前仓库是从完整产品复制出的新版本，直接在本仓库内替换为单一轻量产品；
- 不保留 Full/Light 双模式，不维护旧功能开关；
- 不兼容旧 API、旧 Desktop 客户端或原完整产品数据库；
- 当前阶段不修改、裁剪或验收 Desktop 专属代码，目标操作面以 Web + Server + Local Daemon 为准；
- 新数据库默认名称为 `multica_lightweight`；
- 新数据库使用独立的精简 baseline schema，不回放完整产品的 293 个历史 up migrations；
- 原完整产品数据库不得被迁移、覆盖或复用，可作为只读归档/回退数据源保留；
- P0 只支持 Local Runtime，不保留 Cloud Runtime；
- P0 不支持 Attachment、Human Squad Member 或 Workspace Invitation；
- Issue 保留 `blocked` 状态；
- Squad 只允许 Agent Member；归档时禁止存在 Active Task，历史 Issue 保持原 Squad Assignee；
- Lightweight Run 使用 Issue 作为根对象，Comments 与 Task Runs 分区展示，不提供混合项目事件 Timeline。

## 1. 核心结论

目标系统不是完整的项目管理产品，而是：

```text
Workspace
├── Web / CLI
├── Local Daemon
├── Runtime
├── Agent
├── Direct Chat
├── Squad
├── Lightweight Issue
├── Flat Comments
└── Durable Task Queue
```

其中：

- `Issue` 是一次 Agent/Squad 协作运行的持久化根对象；
- `Comment` 是委派、结果、补充信息和重入触发事件；
- `Task Queue` 是单次 Agent 调用的可靠执行记录；
- `Squad` 是 Leader、成员、角色、Skills 和 Instructions 的静态组织定义；
- `Daemon` 是本地 Runtime 的执行宿主。

### 1.1 当前实现边界

当前代码已经具备目标闭环所需的主要能力，但还没有形成独立的“轻量运行时”边界：

- Desktop/Web 仍挂载完整产品页面和全量 API Client；
- Server 启动时仍会初始化 Autopilot、Webhook、PR Refresh、Channel 等后台服务；
- Issue 创建仍包含 Project、Parent、Label、Attachment、Stage 等校验或写入路径；
- Task 执行仍包含 Autopilot、Quick Create、Inbox、External Channel 和 Quick Action 分支；
- PostgreSQL 仍使用完整历史 Schema 和 293 个 up migrations；
- 当前 API Client、Daemon 和 Web 仍基于完整产品契约，实施时允许进行同步 breaking change；旧 Desktop 不在兼容范围。

因此，本需求采用“先收缩产品表面和运行进程，再抽取核心，最后物理删除”的顺序。

## 2. 产品目标

1. 用户可以启动并配对本地 Daemon；当前阶段使用现有 Daemon/CLI/启动方式，不新增 Desktop 改造要求。
2. Daemon 可以发现、注册和维护本地 Runtime。
3. 用户可以选择 Runtime 创建 Agent。
4. 用户可以和单个 Agent Direct Chat。
5. 用户可以创建 Squad，设置 Leader、成员、Role 和 Instructions。
6. 用户可以向 Squad 提交一项工作。
7. Leader 可以通过 Comment Mention 委派成员。
8. Member 可以执行任务并写回结果 Comment。
9. Member Comment 可以重新唤醒 Leader。
10. Leader 可以继续委派、要求修正或将整体工作推进到 Review。
11. 目标版本不再暴露 Issue Board、Table、Properties、Labels、Subscribers、Reactions、Autopilot 和外部渠道等外围能力。
12. 用户仍能管理 Workspace、Runtime Profile、Skills 以及 Agent 调用权限，并查看只读成员列表。
13. 用户能从轻量运行列表发现历史 Issue/Run，并进入详情分别查看 Flat Comments 与 Task Runs。

## 3. 非目标

本阶段不包含：

- 新建独立 `SquadRun` 数据模型；
- 新建 DAG、BPMN、Temporal 或自动 fan-out/fan-in 引擎；
- 将 Chat Session 立即改造成多 Agent 协作会话；
- 重新设计 Squad 调度协议；
- 保留完整 Linear/Jira 类项目管理能力；
- Cloud Runtime；
- Attachment 上传、下载或引用；
- Human Squad Member 与 Workspace Invitation；
- 混合 Comment/Task/Activity 的统一 Timeline；
- 立即物理删除所有历史字段和 Migration；
- 在同一运行进程中维护 Full/Light 双产品模式；
- 兼容旧 API 或旧 Desktop 客户端；
- 将原完整产品数据库原地升级为轻量数据库；
- 当前阶段修改或裁剪 Desktop 专属代码。

## 4. 核心对象

### 4.1 Workspace

Workspace 是用户、Daemon、Runtime、Agent、Squad、Issue、Comment、Chat Session 和 Task 的隔离边界。

当前实现中 Workspace 还持有 `issue_prefix`、`issue_counter`、成员与角色、鉴权令牌等运行依赖；新 baseline 仍需保留目标运行依赖，但不需要为旧数据库字段或数据兼容兜底。

### 4.2 Runtime

Runtime 表示实际执行 Agent 的提供者实例，例如 Codex、Claude Code、OpenCode、Cursor、OpenClaw 或自定义 Runtime Profile。

当前 `agent_runtime` 使用 `runtime_mode`、`provider`、`device_info`、`metadata`、`last_seen_at`、`owner_id`、`visibility`、`profile_id`、`custom_name` 等字段；目标文档不得假设存在独立的 `version` 或 `last_heartbeat_at` 列。

### 4.3 Agent

Agent 至少保留：

- `name`
- `description`
- `instructions`
- `runtime_id`
- `runtime_mode`
- `model`
- `thinking_level`
- `service_tier`
- `skills`
- `mcp_config`
- `runtime_config`
- `custom_args`
- `custom_env`
- `max_concurrent_tasks`
- `permission_mode`

### 4.4 Squad

Squad 至少保留：

- `name`
- `description`
- `leader_id`
- `instructions`
- Agent Members
- Member Role
- Archive 状态

Squad 本身不是可执行进程。所有 Squad 工作先路由到 Leader，再由 Leader 通过 Comment Mention 派发 Agent Member。用户可以在 Issue/Comment 中参与，但不作为 Squad Member。

目标 Squad 删除接口执行归档语义：存在 `queued/dispatched/waiting_local_directory/running/deferred` Task 时返回 409；归档成功后禁止新分配，历史 Issue 保持原 Squad Assignee，不做 Leader/Autopilot 转移。P0 不提供 Restore API。

### 4.5 Lightweight Issue

Issue 负责：

- 总目标；
- 描述和验收要求；
- 当前 Assignee；
- 整体状态；
- Comments；
- Agent Tasks；
- 发起者和归因。

当前 Issue 仍支持 Project、Parent、Labels、Attachments、Stage、Properties、Subscribers、Reactions、PR Link、搜索、分组和表格接口。Lightweight Issue 是目标服务边界，不是当前已存在的独立模型。

### 4.6 Lightweight Comment

Comment 负责：

- 用户补充；
- Leader 委派；
- Member 结果；
- Leader 判断；
- Mention 触发；
- Leader 重入；
- 运行期间新消息补偿。

当前 Comment API 支持编辑、删除、Resolve 和 Reaction；不存在独立的 `GET /api/comments/{commentId}`。目标只保留 Issue 下的 Comment List/Create，Comment 不可编辑、删除、Resolve 或 Reaction；Run Detail 将 Comments 与 Task Runs 分区展示，不新增单 Comment GET 或混合 Timeline。

### 4.7 Agent Task

Agent Task 负责单次执行状态：

```text
deferred → queued → dispatched → running → completed/failed/cancelled
                         └→ waiting_local_directory ─┘
```

必须保证：

```text
Task completed != Issue completed
```

Leader 的一次 Task 可能只完成了成员委派。

`deferred`、`waiting_local_directory`、prepare lease、comment delivery/reconciliation 和 chat cancellation finalize 都是当前可靠性协议的一部分。轻量 UI 可以折叠显示这些状态，但后端不得在没有替代协议时删除它们。

## 5. 核心流程

### 5.1 Direct Chat

```text
User Message
→ chat_message
→ Chat Task
→ Daemon Claim
→ Runtime Execute
→ Streaming
→ Assistant Message
```

Direct Chat 不依赖 Issue。

### 5.2 Squad Run

```text
Create Issue assigned to Squad
→ Enqueue Leader Task
→ Leader reads Issue + Comments + Squad Briefing
→ Leader posts Comment with @Member mention
→ Enqueue Member Task
→ Member executes and posts result Comment
→ Re-trigger Leader
→ Leader continues or sets Issue in_review
```

## 6. 状态模型

当前 Issue 后端合法状态为：

```text
backlog
todo
in_progress
in_review
done
blocked
cancelled
```

最小 UI 可以将状态分组为：

```text
Active / Review / Done / Cancelled
```

其中 Active 组包含 `backlog/todo/in_progress/blocked`，但必须显示原始状态 Badge。`blocked` 是正式后端和 UI 状态，用于表达等待用户输入、能力缺口或外部条件；不会自动转为 `done/cancelled`。

## 7. 必须保留的可靠性能力

- Mention Parser；
- Assigned Squad Leader Trigger；
- Leader Self-trigger Guard；
- Pending Task Dedup；
- Comment Reconciliation；
- Workspace Tenant Guard；
- Task Complete/Fail 幂等；
- Result Comment/Fallback Comment；
- Session/WorkDir Resume；
- Prepare Lease 与 `waiting_local_directory`；
- Deferred Retry/Fallback；
- Runtime Offline 处理；
- Task Retry；
- Task Token、Invocation Permission 与可信 Actor Context；
- Agent Env/MCP Secret 审计；
- Realtime Task/Comment/Issue Events。

## 8. 页面范围

### 8.1 当前页面现状

Desktop/Web 当前仍包含 Inbox、My Issues、Issues、Projects、Autopilots、Skills、Dashboard/Search 等路由或壳层入口，并挂载全局 Search、Inbox Bridge、Floating Chat 和 Modal Registry。当前 Issues 页面是完整表格/分组/筛选体验，不能直接等同于目标轻量运行列表。

本阶段只改 Web 与共享 Core/View 中目标能力需要的部分；`apps/desktop/**` 不作为当前实施范围或发布门槛。共享包发生 breaking change 时不为旧 Desktop 增加兼容适配。

### 8.2 目标保留

- Local Daemon/CLI 管理能力；
- Web；
- Runtime；
- Agent List/Create/Detail；
- Skill List/Create/Detail、文件管理与 Agent Binding；
- Workspace 基础设置与成员只读列表；P0 不支持 Invitation 或成员变更；
- Direct Chat；
- Squad List/Create/Detail；
- Lightweight Run List/Create；
- Lightweight Run Detail，包括 Flat Comments、Task Runs、Status 和 Assignee。

### 8.3 目标退出

- Issue Board；
- Issue Table；
- Issue Properties；
- Labels；
- Subscribers；
- Reactions；
- Calendar；
- Complex Search；
- Child Issue Tree；
- Autopilot；
- Slack/Feishu；
- Mobile；
- Billing；
- Inbox；
- Cloud Runtime；
- Attachment；
- Workspace Invitation；
- Human Squad Member。

“退出”首先表示目标构建中不再注册页面、导航、API 和后台 Worker；只有在依赖扫描、新 baseline、旧库只读保护和回退验证完成后才进入物理删除。

## 9. API 策略

- 当前路由只作为现状映射，详见 `02_api_reduction_spec.md`；
- 目标可以同步修改 Server、Web、CLI 和 Daemon API，不提供旧路径 alias 或旧 Desktop shim；
- 为降低无效改动，未带来明确简化收益的路径仍优先复用当前 canonical path；
- 所有 breaking change 直接进入目标 route manifest，不维护 v1/v2 双栈；
- 目标构建只注册轻量 API，删除能力统一返回 404。

## 10. 数据库原则

- 目标数据库默认名称为 `multica_lightweight`；
- 目标数据库从空库执行新的精简 baseline schema；
- 目标启动链不回放完整产品的 293 个历史 up migrations；
- 不支持把原完整产品数据库原地升级为目标数据库；
- 原数据库连接只允许用于明确的数据导出工具，不得作为目标服务运行 DSN；
- Auth、Workspace、Permission、Skill、Task Token、Task Message、Task Usage、审计和恢复数据都属于核心依赖；
- baseline 必须包含 edition/schema marker，连接到旧库或错误库时启动失败；
- 精简表结构与 sqlc queries、Workspace 删除事务、Daemon/CLI/Web 必须一次性校准。

## 11. 可量化的轻量化验收

基线固定为提交 `736fbc8a5f1b22d48354a0e55baa00661e9e4326`，在同一 OS、架构、Go/Node 版本和构建参数下比较。发布硬门槛：

- 实际注册的 API 路由及删除路由的 404 证明；
- 启动的后台 Worker 必须等于批准清单，删除能力 Worker 数量为 0；
- Server 二进制大小不超过完整基线的 70%；
- Web production JS 总量不超过完整基线的 60%；
- Server 稳态空闲 RSS 不超过完整基线的 80%；
- 本地 PostgreSQL 已 Ready 时，Server `/readyz` 在 3 秒内返回 Ready；
- 已安装目标 Runtime 时，Daemon 在 5 秒内完成注册并进入 Ready；
- 新库只能包含 26 张批准的应用表和 migration tool 自身元数据表；
- `multica_lightweight` 全新数据库安装、备份和恢复结果。

任何指标无法达到时必须修改 proposal/design 并重新批准，不能在验收时临时放宽。

## 12. 最终验收主流程

1. 通过当前保留的本地启动方式启动 Daemon。
2. Daemon 注册本地 Runtime。
3. 用户选择 Runtime 创建 Leader、Backend Agent 和 Test Agent。
4. 用户创建 Squad。
5. 用户提交 Issue 给 Squad。
6. Leader 被触发并 Mention Backend Agent。
7. Backend Agent 完成并写结果 Comment。
8. Leader 被重新触发并 Mention Test Agent。
9. Test Agent 完成并写验证 Comment。
10. Leader 再次被触发并将 Issue 置为 `in_review`。
11. 用户能在 Flat Comments 与 Task Runs 两个分区看到委派、执行状态、Runtime 和最终结果。

## 13. 已冻结的 P0 产品契约

已经冻结：

1. 本仓库直接替换为单一轻量版；
2. 不兼容旧 API、旧 Desktop 和旧数据库；
3. 当前阶段排除 Desktop 专属修改；
4. 使用新数据库 `multica_lightweight` 和全新精简 baseline。
5. Run List 使用 opaque cursor，按 `updated_at DESC, id DESC` 稳定排序；
6. Run Detail 分为 Flat Comments 与 Task Runs，不提供混合 Timeline；
7. 保留最小 Workspace、Runtime Profile、Skill 与 Agent-Skill 管理页面；
8. 不保留 Attachment、Human Squad Member、Invitation 或 Cloud Runtime；
9. 保留 `blocked` 状态；
10. Squad 归档必须先无 Active Task，历史 Issue 保持原 Assignee；
11. 轻量化量化门槛以第 11 节为准。

## 14. Definition of Ready

本需求可以进入 OpenSpec proposal/design 与实施任务拆分，但不得跳过设计评审。Ready 条件固定为：

- P0 产品范围、页面范围、最终 API manifest、Token 权限、Realtime 白名单和 Worker 白名单均已冻结；
- 26 张应用表的字段、约束、索引、数据库身份保护和 Fresh Install 路径均有权威分册；
- Desktop 明确从目标构建图与发布门槛排除，原数据库明确只读保护；
- 每项 P0/P1 契约都有验收 ID，删除能力有 404、依赖扫描和“不启动 Worker”证据要求；
- 实施仍需在 OpenSpec design 中补充模块拆分、变更顺序、回滚点和任务负责人；这些是实现设计，不是产品范围开放项。
