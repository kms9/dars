## ADDED Requirements

### Requirement: Tool Registry 使用 Workspace-scoped 专用数据模型
Lightweight PostgreSQL SHALL 以专用 Tool Source、Tool Definition、ToolBundle、ToolBundle Item、Agent current Bundle pointer、Task/Task Token Bundle pin、Tool Source Artifact 与 Tool Source Secret 数据保存 Server-side Gateway 状态。每行 SHALL 带有直接或可验证的 Workspace scope；Source revision、Definition public name、Bundle identity/item、Agent pointer、Task pin 与 Secret/Artifact identity MUST 具有阻止同 Workspace 重复或跨 Workspace 关联的唯一约束。

数据库 MUST NOT 添加 Foreign Key、Cascade 或通用 Attachment/Integration 表。Server SHALL 在应用层验证 Source/Definition/Bundle/Bundle Item/Artifact/Secret、Agent、Task、Token 与 Workspace 关系；需要原子性的 Source revision publish、Bundle publish、Agent pointer switch、Task pin、disable/revoke/delete 与 Workspace cleanup MUST 使用事务、必要锁和显式依赖顺序。

#### Scenario: 创建 Source 并发布 Bundle
- **WHEN** Admin 在 Workspace 创建 ready Source 并为同 Workspace Agent 发布选择快照
- **THEN** 专用表保存一致的 Workspace scope、revision、definitions、不可变 Bundle/items 与 Agent pointer，且不会构造通用 Integration/Attachment 数据

#### Scenario: 跨 Workspace 或悬空关系
- **WHEN** 请求尝试把其他 Workspace Definition 放入 Bundle、把 Bundle 固定到其他 Workspace Task，或删除 retained Bundle 仍引用的 Source/Artifact/Secret
- **THEN** Server 在应用层事务中拒绝或按冻结顺序显式清理，且不产生部分写入

### Requirement: ToolBundle 与 Task pin 具有数据库级不可变契约
ToolBundle header/item SHALL 只通过 publish transaction 创建，普通 update query MUST NOT 修改 manifest、item exported name、Definition canonical name、Source revision、schema、invocation plan 或 artifact digest。Agent current Bundle pointer MAY 原子切换；`agent_task_queue.tool_bundle_id` 在 Task 创建后 MUST NOT 随 Agent pointer、Claim 或重新 Claim 改变；Task Token 记录 SHALL 保存或可权威解析同一 Bundle scope。

Bundle content hash SHALL 用于完整性检查和诊断，不得作为历史 Bundle ID 复用键。Bundle revoke、Source/tool disable、Secret rotation 与运行 policy SHALL 使用独立状态，MUST NOT 通过改写 Bundle items 实现。

#### Scenario: 配置变化与 Task 并发
- **WHEN** Agent 发布 B2 的同时已有 Task 固定 B1，或同一 Task 重新 Claim
- **THEN** 事务和必要锁保证 Agent head、Task pin、Token scope 与 Claim URL 各自一致，不出现 B1/B2 混合

#### Scenario: 尝试更新 Bundle item
- **WHEN** 任何 query 或服务尝试就地修改已发布 Bundle item
- **THEN** 写入路径不存在或被拒绝，变更必须发布新 Bundle ID

#### Scenario: 只修改 Agent exported name
- **WHEN** Admin 保持 Definition selection 不变但修改任一 exported name
- **THEN** 新 Bundle item 保存新 exported name 与同一 canonical identity，历史 Bundle item 和 Task pin 不被改写

### Requirement: Tool Source Secret 使用版本化密文 envelope
Tool Source 的 auth headers、tokens、gRPC metadata、TLS private material 与 Remote MCP credential SHALL 使用 `{v,kid,nonce,ciphertext}` authenticated encryption envelope 持久化，密钥来自生产 readiness 已校验的部署 Secret。数据库、备份、普通 DTO、MCP payload、Claim、日志、metrics、Realtime、validation error 与 activity audit MUST NOT 保存或返回 credential 明文。

Server SHALL 只在授权 Source validation 或 invocation scope 内解密，并在使用结束后释放明文引用。Task Claim 只能包含 Server `/bundles/{bundleId}/mcp` 所需的当前 task-scoped `mat_` token，MUST NOT 包含任何 upstream credential。

#### Scenario: Secret at rest 与调用时解密
- **WHEN** Admin 保存 Source credential 后检查持久化与观测输出，并由授权 Task 调用 tool
- **THEN** 静态输出仅可见版本化密文或 redacted metadata，只有 Server outbound invocation 临时获得明文

#### Scenario: 生产加密配置缺失
- **WHEN** production 缺少合法 Tool Source secret encryption key material
- **THEN** Server readiness 不通过，且不以明文、弱加密或禁用审计方式降级

### Requirement: Task Token 与动态 MCP 配置不得持久化明文
Task Token SHALL 继续只持久化 hash 与非 Secret scope metadata，其中 MAY 包含 Task 固定 Bundle ID。Claim 动态生成的 `Authorization: Bearer mat_...` Gateway entry MUST 只存在于当前 Claim response 和受保护的 Runtime task config 生命周期，MUST NOT 写回 `agent.mcp_config`、`runtime_mcp_overlay`、ToolBundle、Tool Source Secret、Tool Definition、activity log、Realtime 或数据库备份。

#### Scenario: Claim 后检查数据库与日志
- **WHEN** Server 为绑定 Agent Claim 动态注入 Gateway MCP entry 后检查所有相关表、日志、Realtime 与审计
- **THEN** 只能找到 Task Token hash、Task/Agent/Workspace/Bundle scope 和不含 token 的配置 metadata，不能恢复 Claim 中的明文 token

### Requirement: Tool Registry 索引使用独立并发迁移
Tool Source、Definition、Bundle、Bundle Item、Agent pointer、Task/Token pin、Artifact 与 Secret 的每个普通或唯一索引 SHALL 使用独立单语句 `CREATE INDEX CONCURRENTLY` 或 `CREATE UNIQUE INDEX CONCURRENTLY` migration，并在后续 migration 中绑定需要的 unique/primary constraint。Schema manifest、query contract、Workspace delete/Bundle retention 顺序与迁移验收 SHALL 同步更新。

#### Scenario: 验证新迁移
- **WHEN** 在 Fresh Lightweight PostgreSQL 执行全部 up migrations 并扫描新增 schema
- **THEN** 不存在 Foreign Key/Cascade，所有新增索引均来自独立 concurrent migration，manifest 与实际 schema 一致
