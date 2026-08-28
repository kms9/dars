# lightweight-api-security Specification

## Purpose
定义 Lightweight P0 对外 API、认证、Token 权限、幂等、错误响应和实时协议的唯一契约，使 Web、Server 与 Daemon/CLI 可以同步完成 breaking cutover。
## Requirements
### Requirement: Skill Import 传输与体积边界
`POST /api/skills/import` SHALL 同时接受 `application/json`（URL 导入）与 `multipart/form-data`（archive 导入）。Archive 上传 MUST 强制压缩包大小上限与解压后的文件数/单文件/总大小上限；路径 MUST 拒绝 traversal 与绝对路径。JSON URL 导入 MUST 校验目标主机属于允许的公开 Skill 源，不得把任意内网 URL 当作导入源。

#### Scenario: 超限 archive 被拒绝
- **WHEN** 客户端上传超过压缩或解压上限的 archive
- **THEN** Server 返回 400 且不创建 Skill

#### Scenario: 非法路径被拒绝
- **WHEN** archive 或导入内容包含 `../` 或绝对路径文件条目
- **THEN** Server 拒绝导入

### Requirement: API 使用严格且一致的传输契约
API SHALL 使用 UUID 字符串、UTC RFC3339 时间与 `snake_case` JSON。列表无数据 MUST 返回 `[]`；分页 MUST 返回 `{items,next_cursor}`，默认 limit 30、范围 1..100，cursor 为服务端 opaque 字符串。JSON Mutation MUST 要求 `application/json`；Create 返回 201，Read/Update/Action 返回 200，无响应体的 Delete/Revoke 返回 204。

#### Scenario: 非法分页和 Mutation
- **WHEN** 客户端传入伪造 cursor、范围外 limit、错误 Content-Type、未知字段、只读字段或非法 null
- **THEN** Server 返回 400 且不发生部分写入

### Requirement: Workspace Context 只由 `X-Workspace-ID` 建立
Human Workspace-scoped 请求的 Workspace Context SHALL 只来自 `X-Workspace-ID` 请求头携带的 Workspace UUID，并由 middleware 经 Membership Guard 解析为不可被 header/body 覆盖的 typed context。Server MUST NOT 接受 slug 形式的 Workspace 标识、MUST NOT 在缺少该 header 时回退到 path 或 body 中的 Workspace ID、MUST NOT 用 Session 记忆的“当前 Workspace”替代它。

带 `{workspaceId}` 的 Human 路由 SHALL 要求 path 值与 header 解析结果一致，不一致时返回 404。非 Workspace-scoped 路由（Public/Auth、`/api/me`、`/api/tokens/**`、`GET /api/workspaces`、`POST /api/workspaces`）MUST NOT 要求该 header。Daemon Token 与 Task Token 的 Workspace 范围 SHALL 只来自服务端 Token 记录，这两类请求 MUST 忽略 `X-Workspace-ID`。

Web SHALL 通过 `GET /api/workspaces` 自行把 slug 路由解析为 UUID 后再发起请求；Server 不提供 slug 解析端点。

跨 Workspace 或不可见对象 SHALL 返回 404，存在但无操作权限 SHALL 返回 403，状态冲突 SHALL 返回 409。

#### Scenario: 跨 Workspace 资源访问
- **WHEN** Actor 使用 Workspace A 凭据请求 Workspace B 的资源并伪造 path/body 标识
- **THEN** 请求返回 404 且不泄露对象是否存在

#### Scenario: header 缺失、非 UUID 或与 path 不一致
- **WHEN** Workspace-scoped 请求缺少 `X-Workspace-ID`、传入 slug 或非 UUID 值
- **THEN** Server 返回 400 或 401，且不从 path/body 推断 Workspace
- **WHEN** `{workspaceId}` path 值与 header 解析出的 Workspace 不同
- **THEN** Server 返回 404

#### Scenario: Daemon 或 Task 凭据伪造 header
- **WHEN** Daemon Token 或 Task Token 请求携带指向其他 Workspace 的 `X-Workspace-ID`
- **THEN** Server 仅使用 Token 绑定范围，并对越界目标返回 403/404

### Requirement: 邮箱验证码是唯一 P0 登录方式
`send-code` SHALL 对存在和不存在邮箱统一返回 202，只保存验证码 hash，验证码 10 分钟过期、最多失败 5 次、验证成功后 single-use。首次验证成功 SHALL 只创建 User；之后 `POST /api/workspaces` MUST 在一个事务内创建 Workspace 和 Owner Member。生产 Session Cookie MUST 为 `HttpOnly + Secure + SameSite=Lax`，状态变更请求 MUST 校验 CSRF。

#### Scenario: 首次登录和建 Workspace
- **WHEN** 新邮箱在 10 分钟内成功验证并随后创建 Workspace
- **THEN** verify 阶段只创建 User
- **THEN** Workspace 与 Owner Member 在第二个事务中原子创建

#### Scenario: 验证码重放或穷举
- **WHEN** 验证码过期、已用、连续错误 5 次或成功后被重放
- **THEN** Server 统一拒绝且不泄露邮箱是否存在

### Requirement: 生产认证配置 fail closed
生产环境缺少邮件发送配置时 `/readyz` MUST 不 Ready；明文验证码只允许显式 development 配置，生产构建不得返回或记录。`/api/config` SHALL 只暴露 `edition/server_version/auth_mode/daemon_protocol/realtime_enabled` 和 Web 启动必需的非 Secret 值。

#### Scenario: 生产缺失邮件配置
- **WHEN** 以 production 模式启动且 Email Provider 未配置
- **THEN** 进程可以报告诊断但 `/readyz` 不 Ready
- **THEN** API、日志和 config 均无明文验证码或 Secret

### Requirement: PAT 生命周期使用一次可见明文
Human 用户 SHALL 能管理自己的 PAT。Token 明文 MUST 只在 Create/Renew 成功响应出现一次；数据库仅保存 hash 与 prefix，日志仅可记录 prefix。过期或撤销 PAT MUST 返回 401，PAT 权限不得超过所属 Human。

#### Scenario: 创建、续期与撤销 PAT
- **WHEN** 用户创建 PAT、使用它调用目标 User API、续期并撤销
- **THEN** 每次新明文只显示一次，旧 Token 在续期或撤销后失效

### Requirement: Daemon 使用唯一协议与专用 Token
Daemon 注册 MUST 声明 `lightweight-runtime-v1`，Server 不得降级旧协议。首次 Register SHALL 接受 Human JWT/PAT，并按 `workspace_id + daemon_id + runtime identity` 幂等 upsert；成功后只返回一次绑定 Workspace、daemon_id 与有效期的 Daemon Token。重配、Deregister 或 Workspace Delete SHALL 撤销旧 Token；过期后必须由 Human JWT/PAT 重新配对。

#### Scenario: 首次配对与后续请求
- **WHEN** Human JWT/PAT 以正确协议 Register Daemon
- **THEN** Server 签发一次可见的 Daemon Token
- **THEN** 后续 heartbeat、claim 与 lifecycle 请求只能用该 Token 访问绑定范围

#### Scenario: 协议错误或 Token 过期
- **WHEN** Daemon 使用非目标协议，或使用被撤销/过期 Token
- **THEN** Server 返回稳定拒绝且不创建 Runtime、不匿名降级、不跨 Workspace

### Requirement: Daemon Token 只访问绑定的 Daemon API
Daemon Token SHALL 只能调用 `/api/daemon/**`，并进一步限制为 Token 绑定的 Workspace、daemon_id、Runtime 与 Task；它 MUST NOT 访问 User API、其他 Daemon/Workspace 或签发 PAT/Task Token。

#### Scenario: Daemon Token 越权
- **WHEN** Daemon Token 请求 User API 或不属于其 Workspace/daemon 的 Runtime/Task
- **THEN** Server 返回 403 或 404 且不泄露或修改数据

### Requirement: Daemon 上下文查询只返回绑定范围
`GET /api/daemon/workspaces` SHALL 只返回 Daemon Token 绑定的那一个 Workspace，MUST NOT 保留按 Human 身份枚举该用户全部 Workspace 的分支。`GET /api/daemon/workspaces/{workspaceId}/repos` SHALL 在校验 Token 绑定后返回该 Workspace 的仓库配置，不返回 Secret 或非目标 settings 字段。

#### Scenario: Daemon 枚举 Workspace
- **WHEN** Daemon 使用 Daemon Token 请求 Workspace 列表
- **THEN** 响应恰好包含 Token 绑定的 Workspace
- **THEN** 即使同一 User 拥有多个 Workspace 也不返回其他 Workspace

### Requirement: GC 查询只暴露回收判定所需状态
四条 GC 查询 SHALL 供 Daemon 判定本地工作目录能否回收，只返回判定所需的最小状态：Issue 与 Chat Session 返回 `status` 与 `updated_at`，Task 返回 `status` 与 `completed_at`。对象不存在（含已硬删除）SHALL 返回 404，Daemon MAY 将其视为可立即回收信号。批量 Issue GC 查询 MUST 限制单次 `issue_ids` 数量与请求体大小，超限返回 400。全部四条 SHALL 校验 Daemon Token 绑定范围，跨 Workspace 目标返回 403/404，且 MUST NOT 返回标题、描述、Comment 或任何 Secret。

#### Scenario: Daemon 判定可回收
- **WHEN** Daemon 对已终态或已删除的 Issue、Chat Session 与 Task 发起 GC 查询
- **THEN** 终态对象返回最小状态字段，已删除对象返回 404
- **THEN** 响应不含业务正文或 Secret

#### Scenario: 越界或超限 GC 查询
- **WHEN** Daemon 查询其他 Workspace 的对象，或提交超过上限的 `issue_ids`
- **THEN** Server 分别返回 403/404 与 400，且不泄露对象是否存在

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

### Requirement: 资源 Owner 与 Workspace 角色权限明确
Member SHALL 能读取目标资源、创建 Chat/Run/Comment并调用获准 Agent；资源 Owner SHALL 能管理自己拥有的 Agent、Skill 或 Squad；Admin SHALL 能管理 Workspace 设置、Runtime Profile 与 Workspace 资源并 Reveal Agent Env；只有 Owner SHALL 能删除 Workspace。任何角色 MUST NOT 跨 Workspace。

#### Scenario: Member 修改他人资源
- **WHEN** Member 修改非本人 Agent/Skill/Squad 或 Workspace 设置
- **THEN** Server 返回 403；对应资源 Owner 或 Workspace Admin/Owner 按白名单成功

### Requirement: Mutation 幂等且生命周期受保护
Issue、Comment与 Chat Message Create MUST 要求 `Idempotency-Key`；同一 Actor+Workspace+route+key与相同 Body hash SHALL 返回同一结果，不同 hash MUST 返回409。Mention Enqueue、Task Complete/Fail/Cancel和 Builder finalize MUST 持久化防重。

Workspace、Runtime、Skill、Chat与 Issue的删除或归档 SHALL 在有 Active Task、在线 Daemon或引用冲突时返回冻结的403/409，并在允许时用事务显式清理。Agent Archive是明确例外：它 SHALL 在同一 operation幂等取消该 Agent active tasks后归档。Squad Archive是明确例外：它 SHALL 在同一事务把当前 Issue assignee转给 Leader并归档，同时保留既有 Task的 Squad attribution。任何例外步骤失败 MUST 整体回滚。

#### Scenario: 重放 Create
- **WHEN** 客户端用同一 key和相同 Body重放 Create
- **THEN** Server返回原资源而不重复写入
- **WHEN** 同一 key使用不同 Body
- **THEN** Server返回 `409 idempotency_key_reused`

#### Scenario: 删除被活动资源引用的对象
- **WHEN** 用户删除存在 Active Task、在线 Daemon或冻结引用冲突的 Runtime、Skill、Chat、Issue或 Workspace
- **THEN** Server返回稳定409且不发生部分清理

#### Scenario: 重放 Agent/Squad 特殊归档或 Builder 完成
- **WHEN** 客户端重放已成功的 Agent Archive、Squad Archive或 Builder finalize
- **THEN** 系统返回同一终态，不重复取消 Task、转移 Issue、创建 Agent或发布业务事件

### Requirement: 错误 envelope 与错误码稳定
所有错误 SHALL 使用 `{error:{code,message,details}}`；`code`为机器契约，至少包含 `invalid_argument/unauthenticated/forbidden/not_found/conflict/invalid_cursor/database_identity_mismatch/protocol_version_unsupported/agent_runtime_required/agent_archived/invocation_forbidden/runtime_unavailable/active_tasks_exist/idempotency_key_reused/task_state_conflict/builder_session_completed/builder_task_active/avatar_invalid/internal_error`。数据库错误、SQL、路径和 Secret MUST NOT 出现在响应中。

#### Scenario: 内部错误
- **WHEN** Handler发生未分类内部错误
- **THEN** 返回 `500 internal_error`和 Request ID关联信息
- **THEN** 响应不含数据库文本、SQL、文件路径或 Secret

#### Scenario: Builder 状态冲突
- **WHEN** 客户端完成已完成 Builder或在 active turn切换 Runtime
- **THEN** Server返回对应稳定 code，且不泄露 carrier或内部状态

### Requirement: Web Realtime 使用 Workspace 事件协议
认证后的 `/ws` SHALL 在升级前校验 Workspace Membership，并只广播冻结白名单事件。每个事件 MUST 包含 `type/event_id/workspace_id/occurred_at/actor_type/actor_id/payload`。连接内允许 at-least-once，客户端 MUST 按 event_id幂等；系统不提供历史 replay，重连后客户端 SHALL invalidate/refetch Run、Comment、Task、Chat、Agent snapshot/detail/tasks/skills、Squad detail/status、Skill和 Runtime查询。

Web事件白名单为：`workspace:updated workspace:deleted agent:created agent:updated agent:status agent:archived agent:restored skill:created skill:updated skill:deleted squad:created squad:updated squad:deleted issue:created issue:updated issue:deleted comment:created task:queued task:dispatch task:waiting_local_directory task:running task:progress task:message task:completed task:failed task:cancelled chat:message chat:done chat:cancel_finalized chat:session_updated chat:session_deleted daemon:register`。所有 payload MUST 为 non-Secret invalidation/summary数据；Builder draft和 Env明文不得广播。

#### Scenario: Web WS 重连
- **WHEN** Web连接断开、错过 Agent Instructions/Skills、roster、Task或 Settings事件后重新认证连接
- **THEN** 客户端不请求 event replay，而是 refetch当前 Workspace查询并最终与数据库一致

#### Scenario: Secret 更新事件
- **WHEN** Agent Manager更新 Env
- **THEN** event只标识资源和变更种类，payload不含 key value或其他 Secret

### Requirement: Daemon WS 使用独立控制协议
`/api/daemon/ws` SHALL 使用 `{type,request_id,payload}` control envelope，并只允许 `daemon:heartbeat/daemon:heartbeat_ack/daemon:task_available/daemon:runtime_profiles_changed/daemon:workspaces_changed/daemon:pending_work/daemon:rpc_request/daemon:rpc_response`。WS 仅提供 Wakeup/Control Hint，数据库与 HTTP lifecycle 仍是权威；重复或丢失 hint MUST NOT 导致重复 Claim，RPC 必须按 request_id 关联并可安全回退 HTTP。

#### Scenario: Hint 丢失或重复
- **WHEN** `daemon:task_available` 被重复投递或完全丢失
- **THEN** Daemon 通过幂等 Claim 与 HTTP fallback 最终处理 Task，且最多一个执行者获得任务

#### Scenario: RPC 超时
- **WHEN** Daemon RPC 请求在断线前未收到 response
- **THEN** 相同 Task/Idempotency 规则允许安全回退 HTTP，不将 Web Workspace event 当作 control response

### Requirement: Router 精确匹配批准的 145 条 manifest
Server SHALL 只注册 checked-in route manifest 中按 `HTTP method + path template` 归一化后的 145 条路由。当前 manifest 汇总此前已经批准的 142 条路由和以下 3 条 Workspace 管理员 Daemon Token 路由，且不得注册重复路径、旧 Desktop alias、MCP 管理、Integrations、Human Squad Member、Template/Composio/Attachment/Inbox/Channel 路由或其他业务路由；query string 不计入 path template。完整精确清单以 `openspec/contracts/routes.txt` 为准。

```http
GET /api/workspaces/{workspaceId}/daemon-tokens
POST /api/workspaces/{workspaceId}/daemon-tokens
DELETE /api/workspaces/{workspaceId}/daemon-tokens/{tokenId}
```

#### Scenario: Router snapshot 精确匹配
- **WHEN** contract test dump全部已注册路由并按 method+path排序
- **THEN** 结果与 checked-in 145 条 manifest 精确相等、无重复并包含上述 3 条 delta
- **THEN** 原批准的 Skills import/search、Tool Gateway control plane 和 Daemon lifecycle 路由仍全部存在

#### Scenario: 未批准恢复路由不可达
- **WHEN** 客户端访问 Agent MCP管理、Lark/Slack/其他 Integration、Human Squad Member、Agent Template、Composio、通用 Attachment、Inbox/Channel、旧 Desktop alias或其他未列出路径
- **THEN** Server返回404且不进入旧 Handler

### Requirement: Avatar 上传只接受受限图片
Agent/Squad avatar action SHALL 只接受 multipart单图片，限制 Content-Type、压缩字节数、解码后像素、尺寸与动画帧，并在解码后重新编码或验证安全格式。上传者必须能管理目标资源；media GET只返回当前仍绑定到 Agent/Squad的不可变对象，不提供目录枚举、任意文件名、通用下载或 Attachment API。

#### Scenario: 非图片或解码炸弹
- **WHEN** 客户端上传伪造 MIME、超限图片、异常像素/帧数或不可解码内容
- **THEN** Server返回400/413，资源 avatar_url不变且不保留可访问对象

#### Scenario: 越权头像上传
- **WHEN** Member为自己无管理权限的 Agent/Squad上传头像
- **THEN** Server返回403/404且不写对象或资源字段

### Requirement: MCP 与 REST 使用隔离的错误边界
Server SHALL 保持 `/bundles/{bundleId}/mcp` 的 MCP transport、JSON-RPC、streaming 和 method error wire contract，MUST NOT 使用 Lightweight REST error envelope 替换其 status、headers、content type 或 body。Tool Source/Bundle Control Plane SHALL 继续使用冻结的 Lightweight REST error envelope，不得将内部 parser、descriptor、network、SQL 或 Secret detail 原样返回。

#### Scenario: 两类 endpoint 同时失败
- **WHEN** MCP Facade 收到 protocol-invalid 请求且 Tool Source/Bundle Control Plane 收到 invalid payload
- **THEN** 前者返回 MCP 原生错误，后者返回 Lightweight REST error envelope，二者均不泄露 Secret 或内部错误
