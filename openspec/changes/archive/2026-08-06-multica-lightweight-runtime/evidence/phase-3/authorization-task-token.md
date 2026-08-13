# Phase 3：角色权限、Task Token 与 114 路由凭据合同（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## Human 角色与资源 Owner

- Workspace Member 可读目标资源、创建自己的 Agent/Skill/Squad、创建获准 Chat/Run/Comment；不能修改他人 Agent/Skill/Squad、Workspace Settings 或 Runtime Profile。
- `agent.owner_id`、`skill.created_by`、`squad.creator_id` 是资源 Owner；对应用户可以管理自身资源。
- Workspace Admin/Owner 可覆盖资源管理权限、管理 Workspace Settings 与 Runtime Profile，并 Reveal/Update Agent Env；Invocation Gate 仍独立生效，Admin/Owner 不会绕过其他用户 private Agent。
- Workspace Delete 在 Router 层是 Owner-only；Admin 即使资源空闲也先返回 403。
- 所有资源 lookup 和管理判断均使用当前 Workspace-scoped Member/ID，不接受 path/body Workspace 回退。

## Task Token 最小权限

- Task Token 仅由服务端 task-token 记录解析 `task_id/workspace_id/agent_id/user_id`，并在每次请求重新加载仍处于 `dispatched/waiting_local_directory/running` 的 Task。
- 允许：读取自身 Issue/Flat Comments、为自身 Issue 写可信 Agent Comment、仅更新自身 Issue `status/assignee`、读取自身 Chat Context、自身 Task Messages，以及自身 Leader Task 的 Squad Evaluation。
- 拒绝：其他 Issue、Chat、Task Messages、TaskRun history、人类 `/api/me`、Agent Env/Secret、资源创建/删除；对自身 Issue 的 `title/description/acceptance_criteria/context_refs` 返回 403 且不部分更新。
- 请求携带的 `X-Workspace-ID/X-Agent-ID/X-Task-ID` 会被认证层覆盖或忽略，归因只来自 Token 记录。
- Task 终态后 Token 行被删除，旧明文立即返回 401。

## 114 路由凭据矩阵

`TestLightweightRouteCredentialMatrix` 对唯一 114-route manifest 的每条 method+path，逐一验证以下六类凭据：anonymous、Human Member、Human Admin、Human Owner、Task Token、Daemon Token。矩阵独立冻结：

- Public；
- Human（JWT/PAT）；
- Workspace Member；
- Workspace Admin；
- Workspace Owner；
- Human-or-Task scoped；
- Daemon-only；
- WebSocket Human Membership。

这与实际 middleware 的 JWT/PAT、Task Token、Daemon Token Live Flow 正反例组合验证，防止新增路由落入宽松默认边界。

## Fresh PostgreSQL 验证

在全新 PostgreSQL 16.13 空库应用 77 个 migrations 后，完整 Live Flow 通过，新增覆盖：

- Member 修改 Workspace 403；Admin 修改 Workspace/Owner Agent、Reveal Env、创建删除 Runtime Profile 成功；Admin 删除 Workspace 403；
- Task Token 更新自身 Issue assignee 成功，读取其他 Chat 返回 404，读取 Agent Env 返回 403；
- 既有自身/他项 Issue、Comments、Task Messages、TaskRun、Squad Evaluation、terminal-token 用例继续通过。

```text
go test ./cmd/server -run 'TestLightweightRouteCredentialMatrix|TestLightweightTaskTokenIssueUpdateFieldBoundary' -count=1

LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' -count=1 -v

Result: PASS
```

远程 PostgreSQL 未创建数据库、未迁移、未写入。
