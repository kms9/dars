# Multica API 裁剪规范

> 基准提交：`736fbc8a5f1b22d48354a0e55baa00661e9e4326`  
> 校准日期：`2026-08-04`  
> 文档状态：`当前路由已校准 / 最终目标 manifest、权限与事件契约已冻结`

## 1. 目标

API 只服务于：

- Workspace/Auth；
- Daemon/Runtime；
- Agent；
- Direct Chat；
- Squad；
- Lightweight Issue；
- Lightweight Comment；
- Task Execution；
- Realtime。

## 2. 契约原则

- 下文“当前 canonical API”来自基准提交的 Router；
- “目标保留”表示轻量版需要的能力，不表示可以立刻删除同模块其他路径；
- 当前没有的接口不得以“保留”名义写成现状；
- 目标允许 `PUT/PATCH`、`DELETE/archive`、`task-runs/tasks` 等 breaking change；
- Server、Web、CLI 和 Daemon 必须同步切换最终 route manifest，不维护 v1/v2 双栈；
- 跨 Workspace 对象应返回 404，权限不足返回 403；
- 不兼容旧 Desktop；Desktop 专属 API 适配不在当前阶段范围；
- Removed route 直接返回 404，不提供兼容 alias 或 410 迁移期。

## 3. Auth、Workspace 与 Runtime Profile API

### 当前 canonical API

```http
GET    /api/me
PATCH  /api/me

GET    /api/workspaces
POST   /api/workspaces
GET    /api/workspaces/{workspaceId}
PUT    /api/workspaces/{workspaceId}
PATCH  /api/workspaces/{workspaceId}
DELETE /api/workspaces/{workspaceId}
GET    /api/workspaces/{workspaceId}/members
POST   /api/workspaces/{workspaceId}/members
PATCH  /api/workspaces/{workspaceId}/members/{memberId}
DELETE /api/workspaces/{workspaceId}/members/{memberId}
POST   /api/workspaces/{workspaceId}/leave

GET    /api/workspaces/{workspaceId}/runtime-profiles
GET    /api/workspaces/{workspaceId}/runtime-profiles/{profileId}
POST   /api/workspaces/{workspaceId}/runtime-profiles
PATCH  /api/workspaces/{workspaceId}/runtime-profiles/{profileId}
PUT    /api/workspaces/{workspaceId}/runtime-profiles/{profileId}
DELETE /api/workspaces/{workspaceId}/runtime-profiles/{profileId}
```

### 目标保留

目标保留身份信息、Workspace 选择、成员只读列表、Workspace Membership Guard 和 Runtime Profile 管理。P0 只保留邮箱验证码登录；Invitation、Member Mutation、Leave Workspace、Google Login 和旧 Onboarding Shim 均退出目标。

## 4. Agent API

### 保留

```http
GET    /api/agents
POST   /api/agents
GET    /api/agents/{agentId}
PUT    /api/agents/{agentId}
POST   /api/agents/{agentId}/archive
POST   /api/agents/{agentId}/restore
GET    /api/agents/{agentId}/env
PUT    /api/agents/{agentId}/env
GET    /api/agents/{agentId}/skills
PUT    /api/agents/{agentId}/skills
POST   /api/agents/{agentId}/skills/add
PUT    /api/agents/{agentId}/skills/{skillId}/enabled
DELETE /api/agents/{agentId}/skills/{skillId}
```

### Create Agent 必填

```json
{
  "name": "Backend Agent",
  "description": "...",
  "instructions": "...",
  "runtime_id": "<uuid>"
}
```

### 核心校验

- `runtime_id` 必填；
- Runtime 必须属于当前 Workspace；
- Agent Archived 时不得执行；
- Agent 未绑定 Runtime 时不得执行；
- 调用必须通过 Invocation Permission Gate。

`POST /api/agents/from-template`、Agent Builder、Labels、Activity/Run Count 不进入目标 manifest；目标 Core 解耦前不能和 Agent Core 一起整体删除，最终构建不为旧 Desktop 保留这些路径。

## 5. Skills API

当前 Agent 创建、配置和 Daemon Claim 都依赖 Skills：

```http
GET    /api/skills
POST   /api/skills
GET    /api/skills/search
POST   /api/skills/import
GET    /api/skills/{skillId}
PUT    /api/skills/{skillId}
DELETE /api/skills/{skillId}
GET    /api/skills/{skillId}/files
PUT    /api/skills/{skillId}/files
DELETE /api/skills/{skillId}/files/{fileId}
```

Labels 相关 Skill API 退出目标版本。Skill CRUD/File/Agent Binding 至少保留一种可用入口，否则 PRD 中的 Agent Skills 无法配置。

## 6. Runtime/Daemon API

### 当前用户侧 Runtime API

```http
GET    /api/runtimes
PATCH  /api/runtimes/{runtimeId}
POST   /api/runtimes/{runtimeId}/update
GET    /api/runtimes/{runtimeId}/update/{updateId}
POST   /api/runtimes/{runtimeId}/models
GET    /api/runtimes/{runtimeId}/models/{requestId}
POST   /api/runtimes/{runtimeId}/local-skills
GET    /api/runtimes/{runtimeId}/local-skills/{requestId}
POST   /api/runtimes/{runtimeId}/local-skills/import
GET    /api/runtimes/{runtimeId}/local-skills/import/{requestId}
DELETE /api/runtimes/{runtimeId}
POST   /api/runtimes/{runtimeId}/unbind-agents-and-delete
```

初始轻量版不保留 Runtime Usage/Activity 聚合或 Runtime 自升级路径，对应 `runtime_usage`/rollup 表不进入新 baseline。Runtime List、Models、Local Skills 和删除语义进入目标；Runtime Update 及其 result callback 不进入目标 manifest。

### 当前 Daemon API

```http
POST /api/daemon/register
POST /api/daemon/deregister
POST /api/daemon/heartbeat
GET  /api/daemon/ws
GET  /api/daemon/workspaces
GET  /api/daemon/workspaces/{workspaceId}/runtime-profiles

POST /api/daemon/tasks/claim
POST /api/daemon/runtimes/{runtimeId}/tasks/claim
GET  /api/daemon/runtimes/{runtimeId}/tasks/pending
POST /api/daemon/runtimes/{runtimeId}/tasks/{taskId}/prepare-lease
POST /api/daemon/runtimes/{runtimeId}/tasks/{taskId}/skill-bundles/resolve

GET  /api/daemon/tasks/{taskId}/status
POST /api/daemon/tasks/{taskId}/start
POST /api/daemon/tasks/{taskId}/wait-local-directory
POST /api/daemon/tasks/{taskId}/progress
POST /api/daemon/tasks/{taskId}/messages
GET  /api/daemon/tasks/{taskId}/messages
POST /api/daemon/tasks/{taskId}/usage
POST /api/daemon/tasks/{taskId}/complete
POST /api/daemon/tasks/{taskId}/fail
POST /api/daemon/tasks/{taskId}/cancel-ack
POST /api/daemon/tasks/{taskId}/session
POST /api/daemon/runtimes/{runtimeId}/recover-orphans
```

`POST /api/daemon/claim` 是当前兼容 alias，不是 canonical 批量路径。目标 Daemon 只使用 `/api/daemon/tasks/claim`。目标保留 Issue/Chat/Task GC Check、Model/Local Skill result，删除 Autopilot GC Check 和 Runtime Update result；最终清单以第 18.8 节为准。

Desktop 管理的本地 Daemon Health/Config/Start/Stop 接口属于 Daemon 自身监听端口，不在上述 Server Router 路由组中；实施 route manifest 时必须单独盘点。

## 7. Chat API

### 保留

```http
POST   /api/chat/sessions
GET    /api/chat/sessions
GET    /api/chat/sessions/{sessionId}
PATCH  /api/chat/sessions/{sessionId}
DELETE /api/chat/sessions/{sessionId}
GET    /api/chat/sessions/{sessionId}/messages
POST   /api/chat/sessions/{sessionId}/messages
GET    /api/chat/sessions/{sessionId}/pending-task
GET    /api/chat/sessions/{sessionId}/draft-restores
DELETE /api/chat/sessions/{sessionId}/draft-restores/{restoreId}

GET    /api/tasks/{taskId}/messages
POST   /api/tasks/{taskId}/cancel
```

当前没有 Session-scoped Cancel 路由；用户根据 pending task 调用 Task Cancel。目标将 Title/Archive 状态统一收敛到 `PATCH /api/chat/sessions/{sessionId}`，不保留 Pin；Server/Web/Daemon/CLI 同步切换，不提供旧 Desktop 兼容路径。

### Create Session

```json
{
  "agent_id": "<uuid>",
  "title": "New chat"
}
```

`project_id` 不属于最小核心，目标请求和响应均不暴露。

### Send Message

```json
{
  "content": "..."
}
```

P0 只接受纯文本，不接受 Attachment 字段，也不注册 Attachment 路由。

## 8. Squad API

### 保留

```http
GET    /api/squads
POST   /api/squads
GET    /api/squads/{squadId}
PUT    /api/squads/{squadId}
DELETE /api/squads/{squadId}

GET    /api/squads/{squadId}/members
POST   /api/squads/{squadId}/members
DELETE /api/squads/{squadId}/members
PATCH  /api/squads/{squadId}/members/role
GET    /api/squads/{squadId}/members/status
```

当前 DELETE 执行归档/清理语义，且不存在 Restore API。轻量版需先定义归档后的 Issue 处置，再将最终契约同步到 Web/CLI/内置 Skills。

### Create Squad

```json
{
  "name": "Feature Squad",
  "description": "...",
  "leader_id": "<agent-uuid>",
  "instructions": "..."
}
```

### Add Member

```json
{
  "agent_id": "<uuid>",
  "role": "Backend engineer"
}
```

## 9. Lightweight Issue API

### 保留

```http
POST   /api/issues
GET    /api/issues
GET    /api/issues/{issueId}
PUT    /api/issues/{issueId}
DELETE /api/issues/{issueId}
GET    /api/issues/{issueId}/task-runs
GET    /api/issues/{issueId}/active-task
POST   /api/issues/{issueId}/tasks/{taskId}/cancel
```

当前没有独立 Issue Status 路由；状态与标题、描述、Assignee 通过 `PUT /api/issues/{issueId}` 更新。`GET /api/issues` 是 Lightweight Run List 的必要发现接口，不能从白名单遗漏。

### Create Issue

```json
{
  "title": "实现用户认证",
  "description": "目标、约束、验收要求",
  "assignee_type": "squad",
  "assignee_id": "<squad-uuid>",
  "status": "todo",
  "acceptance_criteria": [],
  "context_refs": []
}
```

### Update Issue

只允许：

```json
{
  "title": "...",
  "description": "...",
  "assignee_type": "squad",
  "assignee_id": "<uuid>",
  "status": "in_progress",
  "acceptance_criteria": [],
  "context_refs": []
}
```

### 删除或停用

```text
/api/issues/table/**
/api/issues/grouped
/api/issues/search
/api/issues/children
/api/issues/child-progress
/api/issues/properties
/api/issues/labels
/api/issues/subscribers
/api/issues/reactions
/api/issues/quick-actions
```

当前 `GET /api/issues/{issueId}/timeline` 混合多种完整产品事件。目标不注册该接口；Run Detail 分别调用 Comments 与 Task Runs，不在 Server 或客户端合并成统一 Timeline。

## 10. Lightweight Comment API

### 保留

```http
GET  /api/issues/{issueId}/comments
POST /api/issues/{issueId}/comments
```

当前没有 `GET /api/comments/{commentId}`。现有单 Comment 路由仅包括：

```http
PUT    /api/comments/{commentId}
DELETE /api/comments/{commentId}
POST   /api/comments/{commentId}/resolve
DELETE /api/comments/{commentId}/resolve
POST   /api/comments/{commentId}/reactions
DELETE /api/comments/{commentId}/reactions
```

目标只保留 Issue 下的 List/Create；Run Detail 不需要单 Comment GET，P0 不新增该接口。

### Add Comment

```json
{
  "content": "[@Backend Agent](mention://agent/<uuid>) 实现后端接口。"
}
```

可信字段由服务端 Actor Context 决定：

- `author_type`
- `author_id`
- `source_task_id`
- `workspace_id`

客户端不得伪造。

### 删除或停用

```text
comment resolve/unresolve
comment reactions
comment thread page
comment root list
comment quick actions
comment search
comment draft
```

## 11. Squad Activity API

目标保留：

```http
POST /api/issues/{issueId}/squad-evaluated
```

请求：

```json
{
  "outcome": "action",
  "reason": "delegated backend implementation"
}
```

允许值：

- `action`
- `no_action`
- `failed`

## 12. Task User API

保留：

```http
GET  /api/tasks/{taskId}/messages
POST /api/tasks/{taskId}/cancel
GET  /api/issues/{issueId}/task-runs
```

当前没有用户侧 `GET /api/tasks/{taskId}`。目标 Run Detail 复用 Issue Task Runs，并由该响应返回完整生命周期字段；P0 不新增单 Task Detail。

## 13. Realtime API

目标保留当前 Task、Comment、Issue、Chat Message、Runtime/Daemon 状态所需的 Realtime 连接和事件。删除事件前必须核对：

- React Query server-state 更新；
- 客户端指针清理与 self-event guard；
- Web/Daemon 重连和补拉；
- 不再发出目标清单之外的旧事件。

## 14. 目标退出路由族

完成依赖拆分后，目标构建不再注册：

```text
Issue Table/Facet/Group/Search/Properties/Labels/Subscribers/Reactions/Quick Actions
Projects
Autopilots
Inbox/Notifications
Channels/Slack/Lark/Composio
Cloud Billing/Stripe
GitHub/VCS/Webhooks/PR
Dashboard/Complex Analytics
```

Cloud Runtime、Attachments、Agent Templates/Builder、Invitation 和旧 Desktop Onboarding Shim 均退出目标构建。

## 15. 错误码原则

| 场景 | HTTP |
|---|---:|
| 参数错误 | 400 |
| 未认证 | 401 |
| 无调用权限 | 403 |
| 对象不存在或跨 Workspace | 404 |
| Agent Archived/Runtime Missing | 409 |
| 重复或状态冲突 | 409 |
| Runtime 暂不可用 | 503 |
| 服务端错误 | 500 |

## 16. 幂等要求

以下接口或流程必须幂等或防重：

- Task Claim；
- Task Complete；
- Task Fail；
- Task Cancel；
- Comment Mention Enqueue；
- Assigned Squad Leader Trigger；
- Running-task Comment Reconciliation；
- Squad Activity 记录。

## 17. API 冻结结论

本版通过以下内容完成 API 范围冻结：

1. 第 18 节给出唯一目标 route manifest；
2. 第 18.11 节把目标页面 Mutation 映射到 API 资源与生命周期；
3. 未列出的旧路由统一 404，不提供 alias/410 过渡；
4. Server/Web/CLI/Daemon 必须按同一 manifest 同批切换；
5. 第 18.10 节冻结 Human、Daemon Token、Task Token 权限矩阵；
6. Desktop 专属接口明确不进入当前构建与验收范围。

OpenSpec design 和实施任务必须引用以上条目，不得重新解释为开放决策。

## 18. 最终目标 API 契约

本节是轻量版 P0 的权威契约；第 3–14 节用于解释当前代码来源。目标未列出的当前路由一律不注册。

按 `HTTP method + path template` 归一化（忽略 query string）后，本节目标 manifest 固定为 126 条且无重复（含 Agent/Squad 管理恢复的 10 条 delta）；Router contract snapshot 必须精确匹配。

### 18.1 通用规则

- API 不维护 v1/v2 或旧 Desktop alias；
- ID 使用 UUID 字符串，时间使用 UTC RFC3339；
- JSON 字段使用 `snake_case`；列表永不返回 `null`，无数据返回 `[]`；
- JSON Mutation 必须使用 `Content-Type: application/json`；Create 成功返回 201，Read/Update/Action 返回 200，Delete/Revoke 无 Body 返回 204；
- 目标 `PUT/PATCH` 都采用字段级 merge：省略表示不修改，显式 `null` 只允许清除 DTO 中标为可空的字段；未知字段、只读字段或非法 null 返回 400；
- 所有 Workspace-scoped 请求继续使用可信 Workspace Context；路径或 Body 中的 Workspace ID 不能覆盖认证上下文；
- List 默认 `limit=30`，合法范围 `1..100`；cursor 是服务端生成的 opaque 字符串，客户端不得解析；
- Run List 稳定排序为 `updated_at DESC, id DESC`；Comments/Task Runs/Task Messages 使用 `created_at ASC, id ASC`；
- 错误响应统一为：

```json
{
  "error": {
    "code": "agent_runtime_required",
    "message": "Agent must be bound to a runtime",
    "details": {}
  }
}
```

- `code` 是稳定机器契约，`message` 仅用于展示；
- Secret 字段默认不返回，Agent Env Reveal 继续要求 Owner/Admin 且写入审计；
- Create Comment、Send Chat Message、Task Complete/Fail/Cancel 和 Mention Enqueue 必须幂等或有持久化防重键。

分页响应统一为：

```json
{
  "items": [],
  "next_cursor": null
}
```

### 18.2 Public/Auth

```http
GET  /health
GET  /readyz
GET  /api/config
POST /auth/send-code
POST /auth/verify-code
POST /auth/logout
GET  /ws

GET    /api/tokens
POST   /api/tokens
POST   /api/tokens/current/renew
DELETE /api/tokens/{tokenId}
```

P0 只保留邮箱验证码登录。首次验证成功只创建 User；用户随后调用 `POST /api/workspaces`，在同一事务内创建 Workspace 和 Owner Member。PAT 用于 CLI 与首次 Daemon 配对；不保留 Google Login、Contact Sales、Onboarding Shim 或 Invitation。`/ws` 虽位于全局路由组，仍必须在升级前完成认证与 Workspace Membership 校验。

- `send-code` 对存在/不存在邮箱统一返回 202；验证码只存 hash，10 分钟过期，最多失败 5 次，成功后立即 single-use；
- `verify-code` 成功返回 Human session，Cookie 在生产必须 `HttpOnly + Secure + SameSite=Lax`，状态变更请求继续校验 CSRF；
- 生产 `/readyz` 在邮件发送配置缺失时不得 Ready；本地明文验证码仅允许显式 development 配置，生产构建必须 fail closed；
- PAT 前缀、hash、过期和 revoke 语义以第 18.11 节为准，日志只允许记录 `token_prefix`。
- `/api/config` 只返回 `edition/server_version/auth_mode/daemon_protocol/realtime_enabled` 和 Web 启动必需的非 Secret 值；Cloud/Billing/Integration/Desktop Update 配置不得出现。

### 18.3 Workspace 与 Runtime Profile

```http
GET    /api/me
PATCH  /api/me
GET    /api/workspaces
POST   /api/workspaces
GET    /api/workspaces/{workspaceId}
PUT    /api/workspaces/{workspaceId}
DELETE /api/workspaces/{workspaceId}
GET    /api/workspaces/{workspaceId}/members

GET    /api/workspaces/{workspaceId}/runtime-profiles
GET    /api/workspaces/{workspaceId}/runtime-profiles/{profileId}
POST   /api/workspaces/{workspaceId}/runtime-profiles
PUT    /api/workspaces/{workspaceId}/runtime-profiles/{profileId}
DELETE /api/workspaces/{workspaceId}/runtime-profiles/{profileId}
```

P0 的 Member List 只读，不提供 Invitation、Member Create/Update/Delete 或 Leave Workspace。

### 18.4 Runtime、Agent 与 Skill

```http
GET    /api/runtimes
PATCH  /api/runtimes/{runtimeId}
POST   /api/runtimes/{runtimeId}/models
GET    /api/runtimes/{runtimeId}/models/{requestId}
POST   /api/runtimes/{runtimeId}/local-skills
GET    /api/runtimes/{runtimeId}/local-skills/{requestId}
POST   /api/runtimes/{runtimeId}/local-skills/import
GET    /api/runtimes/{runtimeId}/local-skills/import/{requestId}
DELETE /api/runtimes/{runtimeId}
POST   /api/runtimes/{runtimeId}/unbind-agents-and-delete

GET    /api/agents
POST   /api/agents
GET    /api/agents/{agentId}
PUT    /api/agents/{agentId}
POST   /api/agents/{agentId}/archive
POST   /api/agents/{agentId}/restore
GET    /api/agents/{agentId}/env
PUT    /api/agents/{agentId}/env
GET    /api/agents/{agentId}/skills
PUT    /api/agents/{agentId}/skills

GET    /api/skills
POST   /api/skills
GET    /api/skills/{skillId}
PUT    /api/skills/{skillId}
DELETE /api/skills/{skillId}
GET    /api/skills/{skillId}/files
PUT    /api/skills/{skillId}/files
DELETE /api/skills/{skillId}/files/{fileId}
```

只支持 Local Runtime。Runtime Usage、Cloud Runtime、Agent Template/Builder、Label、Composio 和 Connected App 路由不进入目标 manifest。

### 18.5 Direct Chat

```http
GET    /api/chat/sessions?limit={n}&cursor={opaque}
POST   /api/chat/sessions
GET    /api/chat/sessions/{sessionId}
PATCH  /api/chat/sessions/{sessionId}
DELETE /api/chat/sessions/{sessionId}
GET    /api/chat/sessions/{sessionId}/messages?limit={n}&cursor={opaque}
POST   /api/chat/sessions/{sessionId}/messages
GET    /api/chat/sessions/{sessionId}/pending-task
GET    /api/chat/sessions/{sessionId}/draft-restores
DELETE /api/chat/sessions/{sessionId}/draft-restores/{restoreId}
```

P0 不支持 Attachment、Pinned Agent、Quick Action、Project Context、Channel History 或 Read/Unread。Chat 取消使用第 18.7 节唯一的 `POST /api/tasks/{taskId}/cancel`；取消后的文本草稿通过 `chat_draft_restore` 恢复。

### 18.6 Squad

```http
GET    /api/squads
POST   /api/squads
GET    /api/squads/{squadId}
PUT    /api/squads/{squadId}
DELETE /api/squads/{squadId}
GET    /api/squads/{squadId}/members
POST   /api/squads/{squadId}/members
DELETE /api/squads/{squadId}/members
PATCH  /api/squads/{squadId}/members/role
GET    /api/squads/{squadId}/members/status
```

Member 只允许 Agent。`DELETE` 表示 Archive：存在 Active Task 时返回 409；历史 Issue 不改 Assignee；P0 无 Restore。

Member Mutation DTO 固定为：Add `{agent_id, role}`，Remove `{agent_id}`，Update Role `{agent_id, role}`；Leader 不能通过 Remove 删除，只能先用 `PUT /api/squads/{squadId}` 原子更换为同 Workspace 且可调用的新 Leader。更换时新 Leader 原子 upsert 为 `role=leader`，旧 Leader 降为 `role=member`，任一时刻恰好一个 Leader。

### 18.7 Lightweight Run、Comment 与 Task

```http
GET    /api/issues?limit={n}&cursor={opaque}&status={status}&assignee_type={type}&assignee_id={id}
POST   /api/issues
GET    /api/issues/{issueId}
PUT    /api/issues/{issueId}
DELETE /api/issues/{issueId}
GET    /api/issues/{issueId}/comments?limit={n}&cursor={opaque}
POST   /api/issues/{issueId}/comments
GET    /api/issues/{issueId}/task-runs?limit={n}&cursor={opaque}
GET    /api/issues/{issueId}/active-task
POST   /api/issues/{issueId}/squad-evaluated
GET    /api/tasks/{taskId}/messages?limit={n}&cursor={opaque}
POST   /api/tasks/{taskId}/cancel
```

- Run List 的 `status` 可重复，允许值为 `backlog/todo/in_progress/in_review/done/blocked/cancelled`；
- `assignee_type` 只允许 `agent/squad`；
- Comments 在 P0 中不可编辑、删除、Resolve 或 Reaction；
- Flat Comments 与 Task Runs 分开返回和展示，不保留 `/timeline`；
- `/task-runs` 必须包含 Task ID、Agent/Runtime/Squad、Leader 标记、状态、attempt、failure、usage 和全部生命周期时间；因此 P0 不增加单 Task Detail API。

### 18.8 Daemon 协议

Daemon 注册必须声明唯一协议版本 `lightweight-runtime-v1`；Server 不提供旧协议降级。

目标 Daemon route manifest 固定为：

```http
POST /api/daemon/register
POST /api/daemon/deregister
POST /api/daemon/heartbeat
GET  /api/daemon/ws
GET  /api/daemon/workspaces
GET  /api/daemon/workspaces/{workspaceId}/repos
GET  /api/daemon/workspaces/{workspaceId}/runtime-profiles

POST /api/daemon/tasks/claim
POST /api/daemon/runtimes/{runtimeId}/tasks/{taskId}/prepare-lease
POST /api/daemon/runtimes/{runtimeId}/tasks/{taskId}/skill-bundles/resolve
GET  /api/daemon/runtimes/{runtimeId}/tasks/pending

POST /api/daemon/runtimes/{runtimeId}/models/{requestId}/result
POST /api/daemon/runtimes/{runtimeId}/local-skills/{requestId}/result
POST /api/daemon/runtimes/{runtimeId}/local-skills/import/{requestId}/result

GET  /api/daemon/tasks/{taskId}/status
POST /api/daemon/tasks/{taskId}/start
POST /api/daemon/tasks/{taskId}/wait-local-directory
POST /api/daemon/tasks/{taskId}/progress
POST /api/daemon/tasks/{taskId}/messages
GET  /api/daemon/tasks/{taskId}/messages
POST /api/daemon/tasks/{taskId}/usage
POST /api/daemon/tasks/{taskId}/complete
POST /api/daemon/tasks/{taskId}/fail
POST /api/daemon/tasks/{taskId}/cancel-ack
POST /api/daemon/tasks/{taskId}/session

POST /api/daemon/workspaces/{workspaceId}/issues/gc-check
GET  /api/daemon/issues/{issueId}/gc-check
GET  /api/daemon/chat-sessions/{sessionId}/gc-check
GET  /api/daemon/tasks/{taskId}/gc-check
POST /api/daemon/runtimes/{runtimeId}/recover-orphans
```

批量 Claim 唯一路径为 `/api/daemon/tasks/claim`。目标删除 `/api/daemon/claim`、单 Runtime Claim、Runtime Update Result 与 Autopilot GC Check。首次配对允许 Human JWT/PAT 调用 Register；成功后签发一次性可见的 Workspace/daemon-scoped Daemon Token，后续 Daemon 请求使用该 Token。Deregister 撤销同一 Daemon 的 Token；Token 到期后必须使用 Human JWT/PAT 重新配对，不降级为匿名或跨 Workspace 凭据。

Squad Task 的 Claim 响应必须附带只读、非 Secret 的授权 Squad Context：`id/name/instructions/members[{agent_id,name,role}]`。它是 Leader 形成 canonical Agent Mention 的唯一 roster 来源；Task Token 仍不得调用 Squad 或 Task Run 枚举 API。Server 必须验证 Squad 未归档、roster 恰好一个 Leader、Task Agent 属于 roster，且 Leader Task 的 Agent 等于 Squad Leader，否则 Claim 按 contract violation 失败。

### 18.9 Realtime 与 Daemon WS 事件

Web `/ws` 的 Workspace 事件统一 envelope：

```json
{
  "type": "task:running",
  "event_id": "<uuid>",
  "workspace_id": "<uuid>",
  "occurred_at": "2026-08-04T00:00:00Z",
  "actor_type": "member|agent|system",
  "actor_id": "<uuid-or-null>",
  "payload": {}
}
```

目标 Web `/ws` 事件白名单：

```text
workspace:updated workspace:deleted
agent:created agent:status agent:archived agent:restored
skill:created skill:updated skill:deleted
squad:created squad:updated squad:deleted
issue:created issue:updated issue:deleted
comment:created
task:queued task:dispatch task:waiting_local_directory task:running
task:progress task:message task:completed task:failed task:cancelled
chat:message chat:done chat:cancel_finalized
chat:session_updated chat:session_deleted
daemon:register
```

Web 连接内允许 at-least-once 投递，客户端按 `event_id` 幂等处理。系统不提供历史 Event Replay；Web 重连后必须 invalidate 并重新拉取当前 Workspace 的 Run、Comment、Task、Chat、Agent、Squad、Skill 和 Runtime 查询。目标事件之外的旧事件不得广播。

Daemon `/api/daemon/ws` 使用独立 control envelope，不伪装成 Web Workspace event：

```json
{
  "type": "daemon:rpc_request",
  "request_id": "<uuid-or-null>",
  "payload": {}
}
```

Daemon control event 白名单：

```text
daemon:heartbeat daemon:heartbeat_ack daemon:task_available
daemon:runtime_profiles_changed daemon:workspaces_changed daemon:pending_work
daemon:rpc_request daemon:rpc_response
```

Daemon WS 事件只提供 Wakeup/Control Hint，数据库与 HTTP Task Lifecycle 仍是权威状态；丢失或重复 hint 不得造成重复 Claim。RPC 必须用 `request_id` 关联请求/响应，断线或超时后按同一 Task/Idempotency 规则安全回退到 HTTP。

### 18.10 Actor 与 Token 权限矩阵

| 凭据/Actor | 允许范围 | 明确禁止 |
|---|---|---|
| 未认证 | `/health`、`/readyz`、`/api/config`、邮箱验证码登录 | `/api/**` 业务数据和 Realtime Workspace 订阅 |
| Human JWT / PAT | 所属 Workspace 内的目标 User API；管理自己的 PAT；PAT 继承所属 User 权限 | 非成员 Workspace、伪造 Workspace/Actor、Daemon Task Lifecycle |
| Member | 读取目标资源；创建 Chat/Run/Comment；调用有权限的 Agent；创建并管理自己拥有的 Agent/Skill/Squad | Workspace/Member 管理、他人 Secret、绕过 Invocation Gate |
| Admin | Member 能力；管理 Workspace 设置、Runtime Profile 和 Workspace 内 Agent/Skill/Squad；Reveal/Update 任意 Agent Env | 删除 Workspace、跨 Workspace 访问 |
| Owner | Admin 能力；删除 Workspace和管理所有 Workspace 资源 | 跨 Workspace 访问 |
| Daemon Token | 仅 `/api/daemon/**`；且仅绑定 Workspace、`daemon_id` 及其 Runtime/Task | User API、其他 Daemon/Workspace、签发 PAT/Task Token |
| Task Token | 读取自身 Task 关联的 Issue/Comments 或 Chat Context；读取自身 Task Messages；为自身 Issue 创建 Comment；仅更新自身 Issue 的 `status/assignee`；记录自身 Issue 的 Squad Evaluation | 其他 Issue/Task/Chat、Workspace/Member/Token/Runtime Profile、Agent Env/MCP Secret、创建或删除资源 |

补充规则：

- 资源 Owner 指 `agent.owner_id`、`skill.created_by` 或 `squad.creator_id`；Workspace Owner/Admin 可覆盖资源 Owner 权限；
- Task Token 的“自身”由 Token 行中的 `task_id/workspace_id/agent_id` 解析，并进一步绑定 Task 的 `issue_id` 或 `chat_session_id`，不得接受客户端传入的 Actor/Task ID；
- Task Token 更新 Issue 时，Body 出现 `title`、`description` 或非白名单字段必须返回 403，不得静默忽略；
- 对象不存在或跨 Workspace 统一返回 404；对象存在但当前角色或 Token 动作不被允许返回 403；状态冲突返回 409；
- 所有 Handler 都必须由 route contract test 覆盖其允许凭据和拒绝凭据，不能只依赖前端隐藏入口。

### 18.11 P0 Mutation 与生命周期

未列出的 Body 字段必须返回 400；不得静默接受旧字段。Chat Message、Issue 和 Comment Create 必须携带 `Idempotency-Key`；同一 Actor + Workspace + key + route 在资源存续期内返回同一结果，Body hash 不同则返回 409。Daemon Register 按 `workspace_id + daemon_id + runtime identity` 幂等 upsert；使用 Human JWT/PAT 重试配对时只保留最新签发的 Daemon Token。

| Resource | Create/Update 可写字段 | Delete/Archive 语义 |
|---|---|---|
| User | `name/language/timezone` | P0 无 Delete |
| PAT | Create: `name/expires_at?`; Renew 只允许当前 Human PAT | DELETE=Revoke；Token 明文只在 Create/Renew 成功响应出现一次 |
| Daemon Token | 仅 Register 成功时由 Server 签发，客户端不可指定 Workspace/daemon/expiry | Deregister、Workspace Delete 或重新配对时撤销旧 Token |
| Workspace | Create: `name/slug?`; Update: `name/description/context/settings/repos` | Owner only；存在 Active Task 或在线 Daemon 时 409；否则事务内显式清理 |
| Runtime Profile | `display_name/protocol_family/command_name/description/fixed_args/enabled` | 有 Runtime 引用时 409 |
| Runtime | PATCH 仅 `custom_name` | 有 Agent 引用时 409；`unbind-agents-and-delete` 仅 Owner/Admin，先拒绝 Active Task，再事务内解绑并删除 |
| Agent | `name/description/instructions/runtime_id/model/thinking_level/service_tier/runtime_config/custom_args/mcp_config/max_concurrent_tasks/permission_mode/invocation_targets/disabled_runtime_skills` | Archive 有 Active Task 时 409；Restore 恢复可调用状态；Env 只走独立端点 |
| Skill | `name/description/content/config`；Files 为完整 `path/content` 集合；Binding 为完整 `skill_id/enabled` 集合 | 被 Agent 绑定时 409，必须先解绑 |
| Chat Session | Create: `agent_id/title?`; PATCH: `title/status(active\|archived)`；Message 仅 `content` | DELETE 仅 archived 且无 Active Task；事务内清理 Message、Draft Restore、关联终态 Task、Task Token/Message/Usage |
| Squad | `name/description/leader_id/instructions`；Member 仅 `agent_id/role` | DELETE=Archive；有 Active Task 时 409；无 Restore；历史 Issue Assignee 不变 |
| Issue/Run | Create/Update: `title/description/status/assignee_type/assignee_id/acceptance_criteria/context_refs` | 有 Active Task 时 DELETE 返回 409；否则事务内删除 Comment、Task 附属数据和 Activity，不影响 Agent/Squad |
| Comment | Create 仅 `content`；`type/author/source_task/workspace` 由 Server 生成 | Append-only，无 Update/Delete |
| Task | User 只允许 Cancel；Daemon 只按 lifecycle 端点推进状态 | Complete/Fail/Cancel 幂等；终态不可回退 |

所有 Read DTO 使用数据库分册 1.3 节中的目标字段名，并遵守以下收口规则：

- 普通 Resource DTO 不返回 `*_hash`、验证码、Daemon/Task Token、Custom Env 或 MCP Secret 明文；
- `GET /api/agents/{agentId}/env` 仅 Agent Owner 或 Workspace Owner/Admin 可 Reveal 且必须先写审计；普通 Agent DTO 中的 Env/MCP 只返回 key 与 redacted 状态，MCP 配置没有明文 Reveal API；
- `TaskRun` 必须额外展开 `agent_name/runtime_name/squad_name` 只读快照或 display 字段，避免 UI 逐行 N+1；
- `Agent` 返回 `skills` 与 `invocation_targets` 的非 Secret 视图；`Squad` 返回 Leader 与 roster；
- `Issue` 返回可读 `identifier = issue_prefix-number`；`Comment` 返回可信 Actor display；
- 删除/归档的关联对象必须返回稳定 tombstone display，历史 Run 不因名称对象归档而无法渲染；
- 新增任何响应字段必须更新本节、TypeScript DTO、Go response type 和 contract snapshot。

### 18.12 稳定错误码

P0 至少冻结以下 `error.code`：

```text
invalid_argument unauthenticated forbidden not_found conflict invalid_cursor
database_identity_mismatch protocol_version_unsupported
agent_runtime_required agent_archived invocation_forbidden runtime_unavailable
active_tasks_exist idempotency_key_reused task_state_conflict
```

Handler 不得把数据库错误文本、SQL、路径或 Secret 写入 `message/details`。未知内部错误统一 `500 + internal_error`，并只在服务端日志关联 Request ID。
