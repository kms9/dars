## ADDED Requirements

### Requirement: Gateway 发布证明 Daemon 与 Provider 适配未修改
发布验收 SHALL 证明本变更未修改 `server/internal/daemon/**`、`server/internal/daemon/execenv/**`、`server/pkg/agent/**`、`server/cmd/multica/**` 或 Daemon wire protocol，并证明 `multica` Daemon/CLI 构建依赖图不包含 Server Gateway、MCP SDK、gRPC/OpenAPI/Proto compiler 新依赖。

兼容矩阵 SHALL 使用发布基线的未修改 Daemon 与每个声明支持 Remote HTTP MCP 的目标 Provider Runtime 完成真实 Bundle Claim、MCP discovery 和至少一次受控 tool call，并证明完整 Bundle path 与 Authorization header 被保留。不支持该 MCP config/transport 的 Provider SHALL 在 Server Bundle publication/dispatch 前 fail closed，MUST NOT 通过临时 Daemon patch、custom args 绕过或“配置已下发”自述标记为支持。

#### Scenario: 未修改 Daemon 完成调用
- **WHEN** 独立 Server/PostgreSQL 与发布基线未修改 Daemon 启动目标 Provider Task
- **THEN** Claim 仅通过既有 `agent.mcp_config` 投影 Gateway-only Server document，Runtime 实际完成 `/bundles/{bundleId}/mcp` discovery 和 tool call
- **THEN** Daemon diff、protocol DTO 与依赖图保持冻结

#### Scenario: Provider 不支持 Remote HTTP MCP
- **WHEN** Agent 绑定到现有 Provider adapter 明确无法消费 Remote HTTP MCP 配置的 Runtime
- **THEN** Server 阻止 Gateway Bundle publication/dispatch 并返回可诊断、无 Secret 的 unsupported result
- **THEN** 任务不得在缺少 tool 能力时静默运行，也不得修改 Daemon 扩大支持

### Requirement: 每类 Invoker 使用受控本地 fixture 闭环验收
发布矩阵 SHALL 分别使用测试环境控制的本地 HTTP service、unary gRPC service、uploaded Proto artifact 与 Remote Streamable HTTP MCP fixture，证明 Tool Source validation、revision publish、Bundle publication、Task pin、Claim projection、`tools/list`、`tools/call`、受控 side effect/result、audit 和禁用撤权闭环。仅 parser unit test、mock invoker、静态 Tool Definition、HTTP start event 或 MCP 自述 MUST NOT 计为闭环完成。

该矩阵 SHALL 只证明 Multica Web Server 后端的物化、协议、鉴权、路由和结果映射，不以任何非受控外部依赖服务的当前可用性作为 required gate，也不得把本地 fixture 通过表述为外部服务 uptime/behavior 证明。

#### Scenario: 三类 Invoker 正向闭环
- **WHEN** Active Agent Task 依次调用 ready OpenAPI、gRPC 与 Remote MCP tools
- **THEN** 对应本地 fixture 观察到唯一受控调用，MCP result、数据库 Source revision/Bundle/Task pin 与 redacted audit 一致

#### Scenario: 运行中撤销 Bundle 或 Source
- **WHEN** Admin 在 active Task 已完成一次调用后撤销 Bundle、Agent Gateway access、Source 或 tool
- **THEN** 后续 discovery/call 立即不再获授权，且本地 fixture 不收到第二次调用

### Requirement: Bundle 生命周期进入 unchanged-Daemon E3
E3 matrix SHALL 证明一次语义配置变更生成新 Bundle ID，配置变化前创建的 Task 在首次 Claim、Token 重签发和重新 Claim 后仍使用原 Bundle，而变化后创建的 Task 使用新 Bundle。矩阵 SHALL 同时证明 Bundle Task 的 Server-managed Claim document 只含 Gateway entry，Daemon 既有 runtime-local MCP merge 行为未被修改。

#### Scenario: 配置切换不改变运行中 Task
- **WHEN** T1 固定 B1 后 Agent 发布 B2，随后 T1 重新 Claim 且新建 T2
- **THEN** T1 的 Gateway URL、`tools/list` 与 `tools/call` 仍使用 B1，T2 使用 B2
- **THEN** 两个 Task 的 Token 不能交叉访问另一个 Bundle

### Requirement: Gateway 安全负向矩阵进入 required release gate
Required release gate SHALL 覆盖跨 Workspace/Agent/Task/Bundle 调用、Bundle ID 枚举与 Token/Task pin mismatch、终态/过期 Token、Control Plane 越权、不可变写入尝试、malformed MCP、schema invalid arguments、tool/endpoint override、SSRF、DNS rebinding、redirect、private/metadata IP、TLS identity、oversized artifact/request/response、timeout、cancellation、concurrency、Secret redaction、non-idempotent retry、retained artifact 与 Workspace delete cleanup。

#### Scenario: 安全矩阵执行
- **WHEN** 自动化对真实 Server 数据面与控制面执行全部冻结负向用例
- **THEN** 每个用例 fail closed、无跨租户泄露、无未授权上游连接/副作用、无 Secret 输出且无部分持久化

### Requirement: Gateway E3 证据与普通 Server 检查同时通过
变更发布前 SHALL 通过 Server unit/integration tests、MCP conformance/transport tests、Go build/vet/test、Fresh DB migrations、`make check` 目标流水线，以及独立 Server/Daemon/Runtime E3 matrix。E2/build 成功与 E3/runtime 证据 MUST 分开记录；任何一侧缺失都不得宣称本变更 production-ready。

#### Scenario: 汇总发布证据
- **WHEN** 候选版本准备发布
- **THEN** 报告分别列出静态/单测、Fresh DB/Server integration、未修改 Daemon/真实 Runtime 和受控本地 fixture 证据
- **THEN** 缺失 Provider、网络或安全证据的能力明确标记未完成，并明确外部依赖可用性不在本阶段证明范围
