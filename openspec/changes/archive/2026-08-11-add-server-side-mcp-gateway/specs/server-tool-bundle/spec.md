## Purpose

定义 Web Server 后端如何把 Agent 选择的 MCP tools、OpenAPI/gRPC tools 与 Proto Tool Pack 物化为带唯一 ID 的不可变快照，并把同一快照稳定绑定到 Task、Task Token 与 MCP Facade 请求，同时保持实时安全撤权和零 Daemon 修改边界。

## ADDED Requirements

### Requirement: Agent 工具配置发布为不可变 ToolBundle
Server SHALL 将一次完整、合法的 Agent 工具选择发布为不可变 ToolBundle。Bundle SHALL 使用 Server 生成的 opaque unique ID，并保存 canonical manifest 与 content hash；发布完成后，任何 API、后台任务或调用路径 MUST NOT 就地增加、删除、替换或重命名 Bundle item。

Agent 的 MCP/tool/Proto Tool Pack 选择、固定 Source revision、公开 tool contract、invocation plan、Bundle policy 或 Proto artifact digest 发生语义变化时，Server MUST 发布新 Bundle ID，再原子切换 Agent current Bundle pointer。无语义变化的幂等保存 MAY 保留当前 ID；从配置 B 改回历史配置 A 仍 SHALL 发布新 ID，content hash MUST NOT 被用来复用历史 Bundle ID。

#### Scenario: 修改 Agent 工具选择
- **WHEN** Admin 为 Agent 添加、删除或替换一个 tool，或选择同一 Source 的新 revision
- **THEN** Server 发布完整的新 Bundle ID 并原子更新 Agent pointer
- **THEN** 旧 Bundle 内容和 ID 保持不变且仍可供已固定的 Task 解析

#### Scenario: Bundle 发布中途失败
- **WHEN** 选择中存在跨 Workspace Definition、名称冲突、未 ready revision、缺失 artifact 或无法规范化的 invocation plan
- **THEN** Server 不创建可见的部分 Bundle，也不改变 Agent 当前 Bundle pointer

### Requirement: Bundle 物化 Server 掌握的完整工具契约
每个 Bundle item SHALL 固定 Agent exported name、Definition canonical public name、description、input/output schema、Source/Source revision、Tool Definition identity、kind-specific invocation plan、policy metadata，以及相关 Proto/descriptor artifact ID 与 digest。`tools/list` MUST 从这些 Bundle items 投影确定且稳定的 exported-name catalog；`tools/call` MUST 只使用对应 exported name 定位 item，并只使用该 item 固定的 operation、upstream tool name 或 gRPC full method。

Proto/descriptor bytes MAY 以不可变 content-addressed artifact 共享存储而不在每个 Bundle 内复制，但 Server MUST 在任何 retained Bundle 引用期间保留并验证其 digest。Bundle item 或调用参数 MUST NOT 改写 endpoint、credential、method、Source revision、artifact 或其他 Workspace identity。

#### Scenario: Bundle 包含 Proto Tool Pack
- **WHEN** Agent 选择由 build-time 或 uploaded descriptor 产生的 unary gRPC tools
- **THEN** Bundle 固定对应 descriptor artifact digest、method、schema 与 invocation plan
- **THEN** 后续上传同名 Proto 或发布新 Source revision 不改变旧 Bundle

#### Scenario: 读取 Bundle catalog
- **WHEN** Gateway 为同一 Bundle 重复执行 `tools/list`
- **THEN** 在没有显式撤权的情况下返回相同且确定排序的 names/schemas/descriptions

### Requirement: Agent 为每个 Bundle item 配置 MCP exported name
Bundle publication SHALL 接受完整的 `items[{tool_definition_id, exported_name}]` 配置。`exported_name` 为空或省略时 MUST NOT 被 Server 猜测；Web SHALL 显式提交 canonical public name 作为默认值。Server SHALL trim 名称并要求其匹配 `^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`，且在同一 Bundle 内唯一。

Exported name 只改变该 Agent Bundle 的 MCP 调用身份，不得改变 Tool Definition、Source namespace、upstream name、schema、endpoint、credential、method 或 invocation plan。修改任一 exported name SHALL 视为语义配置变化并发布新 Bundle ID；相同 Definition selection 与相同 normalized exported names 的幂等保存 MAY 保持当前 ID。

#### Scenario: Agent 使用自定义调用名
- **WHEN** Admin 将 canonical tool `kratos-demo-http.User_GetUser` 以 exported name `skill.kratos-user.get_user` 发布给 Agent
- **THEN** 新 Bundle item 同时冻结两个名称，Gateway `tools/list` 只暴露 `skill.kratos-user.get_user`
- **THEN** `tools/call` 通过该 exported name 调用固定的原 Definition/upstream operation

#### Scenario: 修改别名生成新 Bundle
- **WHEN** Admin 只把一个已选 Tool 的 exported name 从 `get_user` 改为 `skill.kratos-user.get_user`
- **THEN** Server 发布新 Bundle ID，旧 Task 继续使用旧 Bundle 与 `get_user`，新 Task 使用新名称

#### Scenario: exported name 冲突或不合法
- **WHEN** 完整 selection 包含重复、空白、超长或 MCP-unsafe exported name
- **THEN** Server 原子拒绝整个 publication，返回稳定 `tool_name_conflict` 或 `tool_bundle_invalid`，且 Agent head 保持不变

### Requirement: Agent head 与 Task 使用不同生命周期
Server SHALL 维护 Agent current Bundle pointer 作为新 Task 的配置 head。Task 创建时 SHALL 在同一受控事务中复制该 pointer 到 write-once Task Bundle pin；后续 Claim、重新 Claim、Task Token 重签发或 Agent current Bundle 变化 MUST NOT 改写该 Task pin。

Claim 为 Bundle Task 签发 `mat_` 时 SHALL 将相同 Bundle ID 绑定到 Task Token scope，并将该 ID 写入 Gateway URL。Agent 没有 current Bundle 时，新 Task SHALL 保持既有非 Gateway Claim 语义，不得隐式创建空 Bundle 或要求不支持 MCP 的 Provider 建立连接。

#### Scenario: Agent 在 Task 排队后修改配置
- **WHEN** Task T1 已固定 Bundle B1，随后 Agent 发布并切换到 Bundle B2
- **THEN** T1 的首次 Claim、重新 Claim 和 MCP 请求继续使用 B1
- **THEN** 变更后创建的新 Task 使用 B2

#### Scenario: Task Token 重新签发
- **WHEN** 同一非终态 Task 因合法恢复路径获得新的 `mat_`
- **THEN** 新 Token 仍绑定 Task 原有 Bundle ID，旧 Token 按现有生命周期撤销

### Requirement: Bundle ID 是寻址标识而不是权限凭证
MCP Facade SHALL 通过 `/bundles/{bundleId}/mcp` 从标准 MCP endpoint URL 获得 Bundle ID，不要求 Daemon、Provider adapter 或 MCP client 为 `tools/list`/`tools/call` 增加非标准参数。Gateway SHALL 将 URL Bundle ID 与 `mat_` 解析出的 active Task、Workspace、Agent 和 Task Bundle pin 逐项匹配；仅持有或猜中 Bundle ID MUST NOT 获得 catalog 或调用权限。

未知 Bundle、跨 Workspace Bundle、其他 Agent/Task 的 Bundle、Task pin 不匹配、终态 Task 或无效 Token SHALL 在 tool discovery 和任何 outbound side effect 前 fail closed，并使用不泄露资源是否存在的错误。

#### Scenario: Token 与 URL Bundle 不一致
- **WHEN** Active Task Token 请求另一个 Bundle ID，或客户端伪造 Bundle/Workspace/Agent header
- **THEN** Gateway 拒绝请求，不返回 tools、不建立上游连接且不泄露目标 Bundle 是否存在

### Requirement: 不可变快照与实时撤权分离
普通 Agent 配置变更和 current pointer 切换 MUST NOT 改变已创建 Task 的 Bundle catalog。Server SHALL 另行维护并在每次 discovery/call 检查 Task/Token 状态、Agent Gateway access、Bundle revoke、Source/tool emergency disable 与部署安全 policy；这些运行控制状态 MAY 阻止旧 Bundle 的后续使用，但 MUST NOT 就地改写其 manifest。

Secret rotation、全局限流、Source/Bundle 紧急禁用等运行控制 SHALL NOT 因自身变化创建新 Bundle ID。若 Admin 需要终止既有 Task 对某工具的访问，必须使用显式撤权/禁用操作，而不是仅切换 Agent current Bundle。

#### Scenario: 仅修改 Agent current Bundle
- **WHEN** Agent 从 B1 切换到 B2 且没有显式撤销 B1、Source 或 tool
- **THEN** 已固定 B1 的 active Task 继续看到 B1，新 Task 使用 B2

#### Scenario: 运行中紧急撤权
- **WHEN** Admin 显式撤销 Bundle、禁用 Agent Gateway access、Source 或 tool
- **THEN** 受影响 Task 的后续 discovery/call 立即 fail closed，但已存 Bundle manifest 保持不可变

### Requirement: Bundle 保证范围限制在 Web Server 后端
ToolBundle 的不可变保证 SHALL 只覆盖 Multica Web Server 持久化并校验的 manifest、Tool contract、Source revision、Proto artifact digest 与 invocation plan。它 MUST NOT 被描述为对外部 HTTP、gRPC 或 MCP 依赖服务的 reachability、uptime、latency、返回内容或行为稳定性的保证。

本变更的自动化验收 SHALL 使用由测试环境控制的本地 HTTP/gRPC/MCP fixtures 验证 Server 物化、Facade 路由、鉴权、结果映射和失败处理。外部依赖服务当前是否可用 MUST NOT 作为 ToolBundle 正确性或本阶段发布完成度的判断依据。

#### Scenario: 外部依赖不可用
- **WHEN** Bundle manifest 完整有效，但其指向的非受控外部服务暂时不可达
- **THEN** Bundle 仍是有效不可变快照，调用按有界 transport error 返回
- **THEN** Server 不修改 Bundle、不自动切换 Source revision，也不据此声称外部服务可用

### Requirement: Control Plane 可读取和清除 Agent 当前 Bundle head
Server SHALL 为授权 Owner/Admin 提供 Workspace-scoped Agent current Bundle 读取契约，返回 `bundle: null` 或当前 Bundle header、items 与 Source revision summary，使 Web 在刷新后恢复已发布选择。读取 MUST 按 Agent 与 Workspace 授权，不得要求客户端已知 Bundle ID，不得返回 credential、artifact content 或 invocation Secret。

Owner/Admin SHALL 能显式清除 Agent current Bundle head。清除只影响清除后创建的新 Task；已固定 Bundle 的 Task 和 retained Bundle manifest 保持不变。若 selection 与当前 Bundle 语义相同，重复发布 SHALL 返回当前 Bundle；若 selection、Definition revision 或公开 contract 变化则 SHALL 创建新 ID。

#### Scenario: 刷新 Agent Tools 页面
- **WHEN** Admin 为 Agent 发布 Bundle 后重新打开或刷新 `Capabilities > Tools`
- **THEN** Web 通过 Agent current Bundle API 恢复当前 items 与 Bundle ID，无需枚举或猜测历史 Bundle

#### Scenario: 清除 Agent 当前工具
- **WHEN** Admin 显式清除 Agent current Bundle head
- **THEN** 后续新 Task 不固定 Gateway Bundle，既有 Task 继续使用原 pin，Server 不撤销或改写旧 Bundle
