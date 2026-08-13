# Multica 代码保留、裁剪与删除清单

> 基准提交：`736fbc8a5f1b22d48354a0e55baa00661e9e4326`  
> 校准日期：`2026-08-04`  
> 文档状态：`当前依赖已盘点 / 目标构建图、退出范围与 Worker 白名单已冻结`

## 1. 分类定义

| 分类 | 含义 |
|---|---|
| 保留 | 目标系统运行的硬依赖 |
| 裁剪 | 文件或模块保留，但删除非核心分支 |
| 停用 | 路由/UI 不再暴露，代码可暂时存在 |
| 删除 | 解耦完成后可以从目标代码库移除 |

“删除”不是本阶段立即执行指令。只有同时满足以下条件才能物理删除：

1. 目标 Core 不再 import、构造或调用该模块；
2. 目标 Router、页面、CLI 和内置 Skills 不再暴露该能力；
3. sqlc queries、Workspace 删除事务和后台清理不再依赖对应数据；
4. 全新安装、备份恢复和旧库只读/误连保护通过验证；
5. 新 `multica_lightweight` baseline、Fresh Install 和旧库误连保护已经通过验证。

## 2. 当前耦合热点

以下文件目前同时承载保留能力与删除候选，不能按目录直接删除：

```text
server/cmd/server/main.go
server/cmd/server/router.go
server/internal/handler/handler.go
server/internal/handler/issue.go
server/internal/handler/comment.go
server/internal/handler/squad.go
server/internal/handler/chat.go
server/internal/service/task.go
server/pkg/db/queries/workspace_delete.sql
packages/core/api/client.ts
apps/desktop/src/renderer/src/routes.tsx
packages/views/layout/app-sidebar.tsx
```

目标不是在这些文件里不断增加 `lightweight` 条件分支，而是先形成可独立注册的 Router、Handler 和 Service Core。

## 3. Desktop（当前阶段排除）

### 当前处理

```text
apps/desktop/src/main/**
apps/desktop/src/preload/**
apps/desktop/src/renderer/**
apps/desktop/electron.vite.config.*
apps/desktop/package.json
```

当前阶段不修改、裁剪或验收上述 Desktop 专属代码，也不为旧 Desktop 兼容 Server/Web/Core 的 breaking change。

目标根构建、CI 和 `make check` 必须显式排除 `@multica/desktop`。Desktop 源码可以暂时留在仓库，但允许因共享 Core/View breaking change 而不可编译；任何后续恢复必须另立需求。

当前 Desktop 中存在、但不进入本阶段构建或验收的能力包括：

- Electron Main/Preload/Renderer；
- Token Sync；
- Daemon Auto Start；
- Daemon Start/Stop/Restart；
- Health Check；
- CLI 发现、下载和升级；
- Desktop Profile；
- Daemon 日志；
- IPC API。

### 后续独立阶段

- 是否保留 Electron 发行版；
- 是否将 Renderer 改为轻量 Views；
- 是否裁剪 Inbox Bridge、Search、Modal 和旧页面；
- 是否保留 Daemon Auto Start、下载升级和 Desktop Profile。

这些事项不阻塞当前 Server + Web + Local Daemon 轻量版发布门槛。

### 目标前端构建图

目标 TypeScript 工作集只包括：

```text
apps/web
packages/core
packages/ui
packages/views
packages/tsconfig
以及上述包的直接构建依赖
```

目标根命令必须满足：

- `pnpm build/typecheck/test/lint` 不执行 Desktop、Mobile 或 Docs app；
- CI Frontend job 只验证目标工作集；
- `make check` 调用目标 TypeScript 命令、Go 测试和 Web Playwright；
- `pnpm-workspace.yaml` 可以暂时保留 Desktop，但 Turbo filter 必须明确；物理删除阶段再移除 Desktop/Mobile/Docs package；
- `CLAUDE.md`、AGENTS 指引和 CI 注释必须同步，不再声称 Web/Desktop 是目标双平台。

## 4. Auth、Workspace 与成员

### 必须保留

```text
server/internal/middleware/**
server/internal/handler/auth*.go
server/internal/handler/workspace*.go
server/pkg/db/queries/user.sql
server/pkg/db/queries/member.sql
server/pkg/db/queries/workspace.sql
server/pkg/db/queries/personal_access_token.sql
server/pkg/db/queries/daemon_token.sql
server/pkg/db/queries/task_token.sql
```

保留登录身份、Workspace CRUD、成员角色、Workspace Membership Guard、PAT、Daemon Token、Task Token、可信 Actor Context 和 Workspace 删除事务。

当前 `DaemonAuth`、`daemon_token` query/cache 已存在，但正式 `mdt_` 签发尚未接入配对主流程，Daemon 实际仍主要使用 Human PAT。目标 Register 签发、持久化、重新配对撤销和过期拒绝属于新增实现，不能把“中间件可识别”误报为已完成。

### 目标裁剪

- 登录只保留邮箱验证码；首次登录后由用户创建 Workspace，并在同一事务内成为 Owner；
- Member List 只读，不注册 Invitation、Member Create/Update/Delete 或 Leave Workspace 路由；
- 旧 Desktop onboarding shim 不进入目标构建；
- Workspace Integrations、GitHub/VCS 和 Billing 设置从目标构建退出。

## 5. Daemon 与 Runtime

### 保留

```text
server/internal/daemon/**
server/internal/daemonws/**
server/internal/handler/daemon.go
server/internal/handler/daemon_ws.go
server/pkg/agent/**
server/pkg/protocol/**
server/cmd/multica/
```

保留 Register、Deregister、Heartbeat、Discovery、Runtime Profile、批量 Claim、Prepare Lease、Skill Bundle Resolve、Start、Wait Local Directory、Progress、Messages、Usage、Complete、Fail、Cancel Ack、Session、WorkDir、Orphan Recovery、GC Check 和 WS Wakeup。

当前 `POST /api/daemon/claim` 是兼容 alias，canonical 批量 Claim 为 `POST /api/daemon/tasks/claim`。目标 Daemon/CLI 同步切换 canonical 路径并删除 alias，不保留旧 Desktop 兼容。

### 删除候选

P0 仅支持本地 Runtime，且不提供 Runtime 自升级，删除：

```text
server/internal/cloudruntime/**
Cloud Fleet/Gateway
Cloud Runtime 购买和配置 UI
Runtime Update 请求、状态存储与 Daemon result callback
```

## 6. Agent 与 Skills

### 保留

```text
server/internal/handler/agent.go
server/internal/handler/agent_access.go
server/pkg/db/queries/agent.sql
Agent Skills
Runtime Profile
Agent Env/MCP/Config
Agent Invocation Target
```

保留 Create/Get/List/Update/Archive/Restore、Runtime 绑定、Model、Skills、MCP、Env、Invocation Permission、Env 审计和 Runtime Required Gate。

当前 Agent 更新使用 `PUT /api/agents/{id}`，归档使用 `POST /archive`，不是 `PATCH/DELETE`。目标允许 breaking change，但 Server、Web、CLI 和 Daemon 必须同批切换；不维护旧 Desktop alias。目标明确保留最小 Skill List/Create/Detail、文件管理和 Agent Binding，因此不能删除 `skill`、`skill_file`、`agent_skill` 或 `agent_invocation_target`。

目标删除 Agent Template/Builder、隐藏 `kind=system/system_key` Agent、Agent Label、Run Count/Activity 聚合和对应 Handler/Query/View。

当前 `custom_env/mcp_config` 以原始 JSONB 保存并仅在响应侧做权限/脱敏；目标 baseline 要求版本化密文 envelope，因此写入、Reveal、Daemon Claim、备份恢复与日志脱敏都需要同步改造。

## 7. Direct Chat

### 保留

```text
server/internal/handler/chat.go
server/pkg/db/queries/chat.sql
Chat Realtime Events
TaskService Chat 分支
packages/views/chat/**
packages/core/chat/**
```

### 裁剪

目标停用：

- Project Context；
- Chat Quick Actions；
- External Channel Binding；
- Channel Media；
- 非必要 Agent Intro。

目标保留“取消后恢复输入”。当前运行中取消依赖 `chat_draft_restore` 和 deferred finalize 语义，因此这部分属于核心；不能只保留 `chat_session`/`chat_message` 两张表。

## 8. Squad

### 保留

```text
server/internal/handler/squad.go
server/internal/handler/squad_briefing.go
server/pkg/db/queries/squad.sql
server/internal/service/builtin_skills/multica-squads/**
server/cmd/multica/cmd_squad.go
packages/views/squads/**
```

保留 Squad CRUD、Leader、Members、Role、Instructions、Roster、Skills、Leader Briefing、Leader Routing、Readiness、Dedup、Self-trigger Guard 和 Squad Activity。

当前更新使用 `PUT /api/squads/{id}`，删除执行归档语义；没有 Restore API。目标沿用该 canonical path 并按冻结的 Active Task 规则收紧语义；必须同步 Web/CLI 与内置 Skills，不保留旧 Desktop 兼容层。

### 裁剪

删除或隔离：

- Active Issues 展示；
- Issue 统计；
- Archive 时转移 Issue Assignee；
- Autopilot Assignee Transfer；
- Child Issue/Stage；
- 通用 Activity Feed。

目标归档规则已经冻结：存在 Active Task 时返回 409；成功归档后禁止新分配；历史 Issue 保留原 Squad Assignee；不做 Leader 或 Autopilot 转移；P0 无 Restore。

## 9. Lightweight Issue

### 保留或拆出 Core

```text
server/internal/handler/issue.go
server/internal/handler/issue_trigger.go
server/pkg/db/queries/issue.sql
server/cmd/multica/cmd_issue.go
```

仅保留：

- List；
- Create；
- Get；
- Update Title/Description；
- Update Assignee；
- Update Status；
- Squad Assignment Trigger；
- List Tasks；
- Workspace Guard；
- Assignee Validation。

当前 Update 使用 `PUT /api/issues/{id}`，Tasks 使用 `GET /api/issues/{id}/task-runs`。目标沿用两条 canonical path，在 Task Runs 增加 cursor 契约；不新增独立 Status Update 路由，不维护 API v1/v2 双栈。

当前 `IssueService`/Handler 的创建更新链路仍包含 Project、Parent、Label、Attachment、Stage、Position、Metadata、Duplicate Guard 和 Autopilot Analytics。需要先抽出 `LightweightIssueService` 或等价 Core，再删除外围分支。

### 删除

```text
server/internal/handler/issue_table_*.go
Issue Board/Table/Group/Facet
Properties
Labels
Subscribers
Reactions
Due/Start Date
Calendar
Complex Search
Bulk Actions
Assignee Frequency
VCS Issue Link
Child Progress
```

## 10. Lightweight Comment

### 保留

```text
server/internal/handler/comment.go
server/pkg/db/queries/comment.sql
Mention Parser
Comment Trigger Pipeline
Comment Reconciliation
```

仅保留 Add、Flat List、Mention Trigger、Assigned Squad Leader Trigger、Self-trigger Guard、Dedup、Reconciliation、Workspace Integrity 和 `source_task_id`。

当前不存在单 Comment GET 路由；目标详情复用 Issue Comment List，不新增单 Comment GET。当前 `GET /api/issues/{id}/timeline` 会混合项目管理事件，目标不注册该路由。

### 删除

- Nested Thread；
- Root Thread；
- Thread Pagination；
- Resolve/Unresolve；
- Reactions；
- Quick Actions；
- Draft；
- Thread Ranking；
- Comment Search。

## 11. TaskService

`server/internal/service/task.go` 不应整体删除。

### 必须保留

- Chat Task；
- Issue Task；
- Squad Leader Task；
- Mention Member Task；
- Claim/Start/Messages/Complete/Fail/Cancel/Retry；
- Session/WorkDir；
- Usage；
- Realtime；
- Workspace Resolution；
- Comment Reconciliation；
- Result Delivery。

同时必须保留当前可靠性状态与数据：`deferred`、`waiting_local_directory`、prepare lease、coalesced/delivered comment IDs、chat deferred finalize、retry/rerun lineage、originator/accountable attribution、task token、task messages 和 task usage。

### 删除分支

- Autopilot；
- Quick Create；
- External Channel；
- Child Done；
- 项目管理 Analytics；
- Thread/Quick Action 相关分支。

### 目标拆分边界

```text
task_core.go
task_claim.go
task_chat.go
task_issue_core.go
task_comment_trigger.go
task_squad.go
task_retry.go
task_realtime.go
```

拆分后的 Core 必须通过依赖扫描证明不再引用 Autopilot、Inbox、Channel、Project、Quick Action SQL 或服务；仅改文件名不算解耦完成。

## 12. Router 与进程启动

### 保留

- Auth/Workspace；
- Daemon/Runtime；
- Agent；
- Chat；
- Squad；
- Lightweight Issue；
- Lightweight Comment；
- Task；
- Realtime。

保留范围还必须显式包括：

- `/api/me`、Workspace 和成员基础 API；
- Runtime Profile；
- Skills 和 Agent-Skill 绑定；
- Issue List，用于 Lightweight Run 发现；
- Task Message/Cancel 和 daemon task lifecycle。

### 删除

- Issue Table/Facet/Group；
- Properties；
- Labels；
- Subscribers；
- Reactions；
- Quick Actions；
- Autopilot；
- Channels；
- Lark；
- Slack；
- Billing；
- Inbox；
- External Webhooks；
- Projects/Dashboard/Complex Search；
- Attachments 与 Upload/Download；
- Agent Template/Builder；
- Runtime Self-update；
- Workspace Invitation/Member Mutation/Leave；
- Google Login/Contact Sales/旧 Onboarding。

目标构建还必须停止构造和启动：

- Autopilot Service/Failure Monitor；
- Webhook Worker；
- Pull Request Refresh；
- Channel Supervisor/Router/Media Reconciler；
- 与删除能力绑定的 Scheduler Jobs。

仅让路由返回 404、但后台 Worker 仍运行，不算完成运行时精简。

### 批准保留的后台能力

```text
runtime/task sweeper
  ├── stale runtime/task recovery
  ├── queued expiry
  └── deferred chat cancellation finalization
batched heartbeat scheduler
realtime in-memory hub
optional Redis realtime relay（仅 REDIS_URL 配置时）
optional metrics HTTP listener（不含 business rollup sampler）
```

目标进程不得启动 DB Stats Logger、Task Usage Rollup Scheduler、Notification/Subscriber Worker、Autopilot、Webhook、PR Refresh、Channel Supervisor 或 Channel Media Reconciler。Deferred Task promotion 继续在 Claim/Task Core 中执行，不依赖通用 Scheduler。

## 13. Frontend

### 保留

```text
packages/views/agents/**
packages/views/chat/**
packages/views/squads/**
packages/core/agents/**
packages/core/chat/**
packages/core/api/**
packages/core/realtime/**
packages/ui/**
```

还需保留或建立 Workspace/Auth、Runtime Profile、Skills、Daemon 状态和 Lightweight Run List 的 Core/View 入口。

### 裁剪保留

`packages/views/issues/**` 与 `packages/core/issues/**` 仅保留：

- Lightweight Run Create；
- Lightweight Run Detail；
- Flat Comments；
- Comment Composer；
- Task Execution List；
- Status；
- Assignee Squad。

当前 `IssuesPage` 依赖 Table、Groups、Facets 和完整筛选模型。目标建立新的 Lightweight Run List/Detail：Run List 使用 opaque cursor；Detail 分为 Flat Comments 和 Task Runs，不使用现有混合 Timeline。

### 共享组件迁出

- Picker；
- Agent Permission Helper；
- Comment Row；
- Task Status Pill；
- Markdown/Avatar。

目标位置：

```text
packages/ui/**
packages/views/common/**
packages/core/agents/**
```

## 14. CLI、内置 Skills 与实时事件

### 保留

- Daemon/Runtime、Agent、Chat、Squad、Issue/Comment/Task 目标命令；
- Squad briefing 与协作协议；
- 目标业务需要的 Realtime 事件；
- `server/internal/service/builtin_skills/**` 中引用目标 API 的内容。

### 校准要求

- 删除 API 时同步修改 CLI 命令和内置 Skill 文档；
- 删除 Event 类型前检查 React Query 更新和客户端状态清理；
- 旧 API、旧 Skill 或旧 CLI 不得静默调用已删除能力。

## 15. 数据库与生成代码

当前仓库包含 293 个 up migrations，且 `workspace_delete.sql` 显式清理大量外围表。目标不复用该迁移链，而是为新数据库 `multica_lightweight` 建立精简 baseline；实施时必须同步：

```text
server/migrations/**
server/pkg/db/queries/**
server/pkg/db/generated/**
server/pkg/db/queries/workspace_delete.sql
sqlc generate
```

目标启动链只接受新 baseline/edition marker；误连原完整产品数据库必须启动失败。旧 migration 可以在新 baseline、Fresh Install 和数据保护验证完成后从目标代码库移除。

新 baseline/migration 不新增数据库外键或级联动作；新索引继续使用单语句 `CREATE [UNIQUE] INDEX CONCURRENTLY` migration。

## 16. 目标退出范围

```text
apps/mobile/**
apps/docs/**
Autopilot
Slack
Lark/Feishu
Channel Engine
Billing/Stripe
Inbox
Notifications
Issue Board/Table
Properties
Labels
Subscribers
Reactions
External Webhooks
Complex Analytics
Cloud Runtime
Attachments
Human Squad Members
Workspace Invitations
Projects/Dashboard/Complex Search
Agent Template/Builder
Runtime Self-update
```

这些是目标产品的退出能力，不代表其目录现在都是叶子模块。Cloud Runtime、Attachments、Human Squad Members 已确定退出；Skills 页面保留最小 List/Create/Detail 与 Agent Binding；Workspace Invitation 不进入 P0；旧数据库迁移不在目标范围。

## 17. 保留判定

模块只有满足以下至少一项才保留：

1. Daemon/Runtime 配对；
2. Agent 创建或执行；
3. Direct Chat；
4. Leader—Member—Leader 闭环；
5. Workspace 安全隔离；
6. Task 可靠执行和恢复；
7. Auth、权限、审计或目标 Web/CLI/Daemon 契约；
8. 目标页面直接依赖。

## 18. 分阶段退出门槛

| 阶段 | 允许动作 | 必须证据 |
|---|---|---|
| Surface | 移除 Web 导航、页面注册、API 注册、Worker 启动 | 路由清单、批准 Worker 清单、Web 构建；Desktop 排除 |
| Core Extraction | 拆 Handler/Service/Client | Import/SQL 依赖扫描、P0/P1 测试、真实 PostgreSQL 并发测试 |
| New Baseline | 建立 `multica_lightweight` 精简 Schema 和 sqlc | Fresh Install、name/edition/version guard、Workspace Delete、Backup/Restore |
| Physical Delete | 删除模块、旧查询、旧 migrations 和依赖 | 目标代码扫描、旧数据库未被修改、全新 DB P0/P1 |
| Release | 清理文档、i18n、CI、包依赖 | `make check`、真实 Daemon E2E、Web 浏览器验收、规模对比 |
