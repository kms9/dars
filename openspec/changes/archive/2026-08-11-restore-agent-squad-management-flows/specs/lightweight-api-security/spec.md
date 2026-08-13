## REMOVED Requirements

### Requirement: Router 精确匹配 116 条目标路由
**Reason**: Agent/Squad完整流程需要新增 Builder、运行聚合/取消和头像路由；继续冻结116会与批准产品边界冲突。

**Migration**: 以新增的“Router精确匹配批准的126条 manifest”替代；实现阶段把原116条和本 change的10条 delta合并成 checked-in排序 manifest。

## ADDED Requirements

### Requirement: Router 精确匹配批准的 126 条 manifest
Server SHALL 只注册 checked-in route manifest中按 `HTTP method + path template`归一化后的126条路由。新 manifest SHALL 包含原批准116条路由和以下10条恢复路由，且不得注册重复路径、旧 Desktop alias、MCP管理、Integrations、Human Squad Member、Template/Composio/Attachment/Inbox/Channel路由或其他业务路由；query string不计入 path template。

```http
GET /api/agents/snapshot
GET /api/agents/{agentId}/tasks
POST /api/agents/{agentId}/tasks/cancel

GET /api/agent-builder/sessions
POST /api/agent-builder/sessions
PATCH /api/agent-builder/sessions/{sessionId}/runtime
PUT /api/agent-builder/sessions/{sessionId}/draft

POST /api/agents/{agentId}/avatar
POST /api/squads/{squadId}/avatar
GET /media/avatars/{avatarId}
```

#### Scenario: Router snapshot 精确匹配
- **WHEN** contract test dump全部已注册路由并按 method+path排序
- **THEN** 结果与 checked-in 126条 manifest精确相等、无重复并包含上述10条 delta
- **THEN** 原批准的 Skills import/search和 Daemon lifecycle路由仍全部存在

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

## MODIFIED Requirements

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
