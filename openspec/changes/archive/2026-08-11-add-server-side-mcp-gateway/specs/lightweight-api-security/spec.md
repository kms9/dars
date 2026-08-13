## MODIFIED Requirements

### Requirement: Task Token 使用最小任务能力
Task Token SHALL 从服务端 Token 记录解析 `task_id/workspace_id/agent_id` 及关联 Issue 或 Chat，不接受客户端伪造 Actor/Task。它只能读取自身 Task 的 Issue/Comments 或 Chat Context、自身 Task Messages，为自身 Issue 创建 Comment，仅更新自身 Issue 的 `status/assignee`，并记录自身 Issue 的 Squad Evaluation；它 MUST NOT 访问其他资源、管理 Token/Runtime Profile、读取 Agent Env/MCP Secret、创建或删除资源。

Task Token SHALL 额外只能访问 Server Remote MCP 数据面 `/bundles/{bundleId}/mcp`，且 `tools/list`/`tools/call` 能力必须由同一 Token 的 active Task、Workspace、Agent、Task-pinned ToolBundle、Bundle item 与实时撤权 policy 联合确定。Bundle ID 是不具授权能力的资源标识，必须与 Token/Task pin 匹配；Task Token MUST NOT 访问 Tool Source/Bundle Control Plane、Reveal upstream credentials、枚举 Workspace catalog、选择任意 Bundle/endpoint/operation 或调用其他 Task/Agent 的 tools。MCP 权限扩展 MUST NOT 扩大 Task Token 对普通 `/api/**` 路由的既有白名单。

Squad Task 的 Daemon Claim SHALL 同时返回只读、非 Secret 的授权 Squad Context `id/name/instructions/members[{agent_id,name,role}]`，供 Leader 使用 canonical Agent Mention。Server SHALL 在 Claim 时验证 Squad 未归档、Task Agent 属于 roster、roster 恰好一个 Leader，且 Leader Task Agent 与该 Leader 一致；Task Token 本身仍 MUST NOT 枚举 Squad 或 Task Run API。

#### Scenario: Task 更新自己的 Issue
- **WHEN** Task Token 仅提交允许的 `status/assignee` 字段
- **THEN** Server 在同一 Task/Workspace/Issue 范围内执行更新

#### Scenario: Task Token 夹带字段或访问他项
- **WHEN** Body 包含 `title`、`description` 或其他非白名单字段，或路径指向其他 Issue/Task/Chat
- **THEN** Server 返回 403/404 且不发生部分更新

#### Scenario: Leader 通过 Claim 获得最小 roster
- **WHEN** Daemon Claim 一个合法的 Squad Leader Task
- **THEN** Claim 只附带该 Task 所属 Squad 的非 Secret roster 和 instructions
- **THEN** Leader 可以形成 canonical Agent Mention，且 Task Token 对 Squad 枚举 API 仍返回 403

#### Scenario: Task Token 发现并调用自己的 tools
- **WHEN** Active Task Token 调用其 `/bundles/{pinnedBundleId}/mcp` 的 `tools/list` 或 Bundle 内 tool
- **THEN** Server 只在 Token/Task/Bundle 绑定范围内返回或执行该 tool，并忽略客户端伪造的 Workspace/Agent/Task/Bundle identity

#### Scenario: Task Token 访问 Control Plane 或他人 tool
- **WHEN** Task Token 请求 `/api/tool-sources/**`、Bundle 管理 API、Reveal credential、枚举整个 Workspace catalog 或调用其他 Task/Agent Bundle 的 tool
- **THEN** Server 返回 403/404 或 MCP authorization error，且不泄露目标是否存在或产生上游副作用

#### Scenario: Task 终态后再次调用 MCP
- **WHEN** Task 已完成、失败、取消或 Token 已过期/撤销后再次请求 MCP Facade
- **THEN** Server 返回认证失败，且不复用此前授权或上游连接继续执行新调用

## ADDED Requirements

### Requirement: MCP 与 REST 使用隔离的错误边界
Server SHALL 保持 `/bundles/{bundleId}/mcp` 的 MCP transport、JSON-RPC、streaming 和 method error wire contract，MUST NOT 使用 Lightweight REST error envelope 替换其 status、headers、content type 或 body。Tool Source/Bundle Control Plane SHALL 继续使用冻结的 Lightweight REST error envelope，不得将内部 parser、descriptor、network、SQL 或 Secret detail 原样返回。

#### Scenario: 两类 endpoint 同时失败
- **WHEN** MCP Facade 收到 protocol-invalid 请求且 Tool Source/Bundle Control Plane 收到 invalid payload
- **THEN** 前者返回 MCP 原生错误，后者返回 Lightweight REST error envelope，二者均不泄露 Secret 或内部错误
