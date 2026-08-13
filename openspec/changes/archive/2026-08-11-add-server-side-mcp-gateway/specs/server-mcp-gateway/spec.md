## Purpose

定义由 Multica Web Server 承担的 Remote MCP 数据面、Task-scoped 鉴权、Tool 可见性与调用行为，使 Agent Runtime 通过现有 Daemon 配置投影安全调用统一工具，而不把 Gateway 逻辑或上游凭据下沉到本地执行面。

## ADDED Requirements

### Requirement: Server 提供唯一 Remote MCP 数据面
Multica Web Server SHALL 在单一 route family `/bundles/{bundleId}/mcp` 提供 Streamable HTTP MCP Server。对 `2026-07-28` 协议请求，数据面 MUST 使用 stateless 模式，使每个 JSON-RPC 请求通过独立 HTTP POST 完成，MUST NOT 要求 MCP sticky session、Daemon affinity 或固定 Server replica。

该 route family SHALL 只接受有效且仍处于 active Task 状态的 `mat_` Bearer Token，并要求 URL Bundle ID 与 Token/Task 固定 Bundle 完全一致。Human JWT/PAT、Daemon Token、过期或非当前 Task Token、未知或不匹配 Bundle MUST fail closed，且客户端提供的 Workspace/Agent/Task/Bundle header 或 arguments MUST NOT 覆盖服务端解析出的身份。

#### Scenario: Active Task 建立 MCP 调用
- **WHEN** Agent Runtime 使用当前 active Task 的 `mat_` token 向其固定 `/bundles/{bundleId}/mcp` 发送合法 MCP POST
- **THEN** Server 以 Token 绑定的 `task_id/workspace_id/agent_id/user_id/bundle_id` 处理请求
- **THEN** 请求不依赖协议级 Session、固定 Server replica 或 Daemon affinity

#### Scenario: 非 Task 身份访问 MCP
- **WHEN** 请求使用 Human、PAT、Daemon、过期、已撤销、终态 Task、伪造 Token 或不匹配 Bundle 访问 MCP Facade
- **THEN** Server 返回认证失败，且不执行 tool discovery、上游连接或业务副作用

### Requirement: Claim 通过既有 MCP 配置投影 Gateway
当 Task 已固定 ToolBundle 时，Server SHALL 在 Claim 签发当前 `mat_` token 后，通过既有 `claim.agent.mcp_config` 返回只含名为 `multica` 的 canonical Remote HTTP MCP document。该条目 SHALL 使用合法的 `MULTICA_PUBLIC_URL + /bundles/{task-bundle-id}/mcp` 和 `Authorization: Bearer <current-task-token>`；其他 Server-stored Agent MCP entries MUST NOT 进入该 Bundle Task 的 Claim document。

Server MUST NOT 为该能力新增 Claim/Daemon DTO field、修改 Daemon protocol version 或要求 Daemon/Provider adapter 新行为。动态条目与明文 Task Token MUST NOT 持久化到 `agent.mcp_config`、`runtime_mcp_overlay`、ToolBundle 或其他数据库字段。Daemon 既有 runtime-local MCP merge 行为 SHALL 保持不变，因此本要求不禁止 Runtime 继续看到本机配置的其他 MCP entries。

#### Scenario: Bound Agent 被未修改 Daemon Claim
- **WHEN** 未修改的 Daemon Claim 一个固定 ToolBundle 的 Agent Task
- **THEN** Claim 仍通过既有 `agent.mcp_config` 字段携带唯一 `multica` URL、Task Bundle ID 与当前 Task Token
- **THEN** Daemon 按既有行为把该配置投影到 Runtime，无需识别 Tool Source、OpenAPI、gRPC 或 MCP proxy 类型

#### Scenario: Task 没有 Bundle pin
- **WHEN** Task 创建时 Agent 没有 current ToolBundle
- **THEN** Server 不为本能力注入 `multica` 条目，且既有 MCP 配置语义保持不变

#### Scenario: Daemon 继续合并本机 MCP
- **WHEN** 未修改 Daemon 所在 Runtime 还配置了本机 MCP servers
- **THEN** Daemon 按既有 precedence 合并本机 entries 与 Claim 中唯一的 `multica` entry
- **THEN** Server 不读取、不快照也不代理这些 runtime-local entries

#### Scenario: Public URL 不可用
- **WHEN** Gateway 需要投影但 Server 没有合法绝对 HTTP(S) `MULTICA_PUBLIC_URL`
- **THEN** Server fail closed，且不下发不可达、由请求 Host 推导或缺少鉴权的 MCP endpoint

### Requirement: Tool discovery 投影 Task 固定 Bundle
`tools/list` SHALL 只返回 URL/Token/Task 共同固定的不可变 Bundle items，并在返回前应用实时撤权 policy。Server MUST NOT 改为查询 Agent current Bundle、Source current revision、整个 Workspace catalog、其他 Bundle/Agent tools、revoked source/tool 或上游 credential；返回名称、schema、description 与顺序 SHALL 由 Bundle manifest 确定且稳定。

#### Scenario: Agent 仅看到 Task Bundle 工具
- **WHEN** 一个 Workspace 存在多个 Bundle 且不同 Task 固定不同 Bundle
- **THEN** 每个 Task 的 `tools/list` 只包含自身 Bundle items
- **THEN** Tool schema 与 description 不包含 endpoint credential 或其他 Agent/Workspace metadata

#### Scenario: Bundle 工具在运行中被撤销
- **WHEN** Admin 在 Task 运行期间撤销 Bundle、Agent Gateway access、Tool Definition 或 Tool Source
- **THEN** 后续 `tools/list` 不再返回该 tool，后续 `tools/call` fail closed

### Requirement: Tool call 每次重新授权并由 Server 执行
`tools/call` SHALL 从 Token identity 与 Task-pinned Bundle 中解析 tool，逐次验证 Workspace、Agent、Task/Token/Bundle match、Bundle/Source/Definition revoke 状态与 deployment policy，再由 Bundle item 的不可变 invocation plan 选择 OpenAPI、gRPC 或 Remote MCP invoker。Tool arguments MUST NOT 选择任意 Bundle、endpoint、credential、Workspace、Agent、Task、Source revision、Proto artifact、upstream method 或未注册 operation。

Server SHALL 为每次调用应用 deadline、request/response size、concurrency 与 cancellation policy，记录不含 Secret 的审计结果。非幂等调用 MUST NOT 因 transport error 被透明重试；调用超时、客户端取消或授权在开始前失效 SHALL fail closed。

#### Scenario: 调用已授权工具
- **WHEN** Active Task 调用其固定 Bundle 中的 tool 并提供符合 input schema 的 arguments
- **THEN** Server 使用 Bundle 固定的 source revision、operation 与 server-held credential 发起一次有界上游调用
- **THEN** Server 返回 MCP tool result 并记录不含 Secret 的 invocation audit

#### Scenario: 伪造 tool 或 endpoint
- **WHEN** arguments、MCP headers、Bundle URL 或 tool name 尝试选择其他 Bundle/Workspace tool、替换 upstream endpoint/operation 或引用 Bundle 外 tool
- **THEN** Server 返回 MCP authorization/validation error，且不建立目标连接

### Requirement: MCP Facade 以 Bundle exported name 发现和定位工具
Gateway SHALL 把 Bundle item 的 exported name 作为标准 MCP Tool `name` 返回，并以同一名称解析 `tools/call`。Definition canonical public name 与 upstream name MAY 出现在授权 Human Control Plane 和脱敏 audit 中，但 MUST NOT 作为同一 Bundle 的隐式调用别名；客户端调用未发布的 canonical/upstream name SHALL fail closed。

#### Scenario: 只允许 Bundle 导出名
- **WHEN** Bundle 把 canonical tool `kratos-demo-http.User_GetUser` 导出为 `skill.kratos-user.get_user`
- **THEN** `tools/list` 返回后者，使用后者的 `tools/call` 命中固定 item
- **THEN** 使用 canonical 或 upstream name 的调用不产生上游副作用，除非该名称本身也作为另一个 item 的 exported name 被显式发布

### Requirement: MCP 原生协议错误不被 REST 包装
`/bundles/{bundleId}/mcp` 的 MCP success、JSON-RPC error、protocol negotiation error、streaming response 与 method rejection SHALL 保留 MCP transport 定义的 status、headers、content type 与 body。普通 Lightweight REST error envelope MUST NOT 缓冲、替换或重编码 MCP 响应；认证与 Bundle scope 失败仍 SHALL 使用非枚举的标准 HTTP failure，且不得泄露 Token 或 Bundle 是否存在。

#### Scenario: MCP 请求参数非法
- **WHEN** MCP client 发送 malformed JSON-RPC、unsupported protocol version 或 schema-invalid `tools/call`
- **THEN** 客户端收到 MCP transport/JSON-RPC 定义的错误而不是 Lightweight REST error envelope

#### Scenario: Tool call 流被取消
- **WHEN** 客户端取消一个尚未完成的 MCP POST
- **THEN** Server 将 cancellation 传播到有界 tool handler 与上游调用，并保留 MCP transport 的连接行为
