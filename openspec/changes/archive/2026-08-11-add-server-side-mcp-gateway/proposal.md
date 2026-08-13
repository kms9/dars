## Why

Multica 当前已经能在 Task Claim 中向 Agent Runtime 下发 `mcp_config` 与 task-scoped `mat_` credential，但缺少一个由 Server 统一托管、按 Workspace/Agent/Task 授权的 Remote MCP 数据面。若继续让每个 Daemon 或 Agent 直接解析 OpenAPI、编译 Proto、连接 gRPC 或代理其他 MCP，不仅会复制执行逻辑和上游凭据，还会破坏 Lightweight 的 Server 权限主导与 Daemon 本地执行边界。

本变更将 MCP Gateway 冻结为 Web Server 后端能力，并把 Daemon 视为不可变兼容边界：Server 复用现有 Claim `agent.mcp_config` 形态动态注入 Gateway endpoint 与当前 Task Token，现有 Daemon 仅按既有行为启动 Runtime 和投影 MCP 配置。

## What Changes

- 在现有 Go Web Server 进程提供 task-token-only 的 Remote Streamable HTTP MCP Facade endpoint `POST /bundles/{bundleId}/mcp`，使用 stateless MCP 模式，并保持 MCP/JSON-RPC 响应不被普通 REST error envelope 改写。
- 在 Server 将 Agent 选中的 MCP/tools 与 Proto Tool Pack 物化为带唯一 ID 的不可变 ToolBundle。任何语义配置变更 SHALL 发布新 Bundle ID；Task 创建时固定 Bundle ID，后续 Claim、重试和重新 Claim 不跟随 Agent 当前配置漂移。
- 区分 Source catalog 的不可变 canonical tool name 与 Agent ToolBundle 的 exported tool name。上传时的 Source name 只决定 canonical namespace；Owner/Admin 在 Agent `Capabilities > Tools` 为每个已选 Definition 配置实际 MCP 调用名，修改别名 SHALL 发布新 Bundle ID，旧 Task 继续使用旧名称。
- 在 Server Task Claim 阶段为已固定 Bundle 的 Task 生成只含保留名称 `multica` 的标准 Remote MCP 配置；条目使用 `MULTICA_PUBLIC_URL + /bundles/{bundleId}/mcp` 与当前 `mat_` token。明文 token 不持久化到 `agent.mcp_config`、`runtime_mcp_overlay`、Bundle、日志、Realtime 或审计载荷。
- 在 Server 建立 Workspace-scoped Tool Source、Tool Definition、ToolBundle/Bundle Item、Agent current Bundle pointer 与审计模型，提供仅 Human Workspace Owner/Admin 可用的 Control Plane API，并在目标 Web 中增加 Tool Source 上传/管理与 Agent 工具选择入口。Web 只消费脱敏的控制面 API，不承担解析、编译、凭据、网络连接或调用执行。
- 在统一 Tool Registry 后提供三类 Server-side invoker：OpenAPI/HTTP、descriptor-driven dynamic gRPC，以及 Remote MCP proxy；build-time Proto 与 runtime-uploaded Proto 共享同一 dynamic gRPC runtime。
- `tools/list` 只投影 URL 与 Task Token 共同绑定的不可变 Bundle items 及其 exported names；`tools/call` 按同一 exported name 定位 item、从固定 invocation plan 路由，并逐次重新验证 task/workspace/agent/bundle/source/tool 的实时撤权状态。
- 增加 Server egress、SSRF/DNS rebinding、redirect、协议、TLS、超时、响应大小、并发、credential 隔离、审计与租户边界控制；Server 无法访问的 Daemon localhost、用户 VPN 或局域网目标明确不支持，也不增加 tunnel/local gateway fallback。
- 将支持范围限制为现有未修改 Daemon 与 Provider 已能消费的 Remote HTTP MCP 配置；不兼容 Provider fail closed，不以本变更修改 `server/internal/daemon`、`server/pkg/agent`、`server/cmd/multica` 或 Daemon wire protocol。
- 增加 Server-only unit/integration/conformance 验证，以及使用未修改 Daemon、真实目标 Runtime 和受控本地 upstream fixture 的发布验收矩阵；静态检查、mock 或单独 MCP HTTP 成功不能代替真实 Runtime 调用证据。验收只证明 Web Server 后端的物化、鉴权、投影和路由能力，不承诺外部依赖服务的可用性或行为稳定性。

## Capabilities

### New Capabilities

- `server-mcp-gateway`: Server Remote MCP 数据面、Task/Bundle Token 鉴权、按固定 Bundle 投影的 tool discovery/call、invoker dispatch、协议与错误边界。
- `server-tool-bundle`: Agent 工具选择的不可变快照、唯一 Bundle ID、Task 固定、Facade 寻址、Proto artifact 保留与实时撤权边界。
- `server-tool-source-management`: Workspace Tool Source/Definition 与 Agent Bundle publication 的 Server Control Plane、OpenAPI/Proto/Remote MCP 导入、credential 生命周期和 egress policy。
- `web-tool-source-management`: 目标 Web 中 Owner/Admin 可用的 Tool Source 目录、文件/URL 导入、校验预览、启停和 revision 管理入口。
- `web-agent-tool-bundle-management`: Agent `Capabilities > Tools` 中按 Source/Tool 选择、发布和清除不可变 ToolBundle 的控制面。

### Modified Capabilities

- `lightweight-product-boundary`: 为 Server-side Gateway 增加严格受限的目标 Web 管理面例外，同时继续禁止 raw Agent MCP 配置、Desktop 接线、Daemon Gateway 和本地网络 fallback。
- `lightweight-api-security`: 扩展 Task Token 的最小能力，使其只能调用自身固定 Bundle 的 Facade tools，并冻结 Bundle ID 非凭证、MCP 原生错误、租户鉴权与 fail-closed 行为。
- `lightweight-data-baseline`: 增加 Tool Source/Definition/Bundle/Task pin/Secret/Audit 持久化要求、显式应用层关系校验与清理，并保持无 Foreign Key/Cascade 和并发索引迁移规则。
- `lightweight-runtime-release`: 增加 Server MCP Gateway 的真实独立进程、未修改 Daemon、Provider 兼容矩阵、上游调用与安全负向发布证据。

## Impact

- Server composition/API：`server/cmd/server`、`server/internal/lightweightapi`、新增 `server/internal/mcpgateway`，以及 MCP 原生 transport/auth/error boundary。
- Claim contract：仅改变现有 `claim.agent.mcp_config` 的动态内容，使 Bundle Task 只获得 Gateway URL、Bundle ID 与 `mat_`；不增加或修改 Daemon DTO、协议版本或 Provider adapter。Daemon 既有 runtime-local MCP merge 行为保持不变。
- Data：Lightweight PostgreSQL 新增 ToolBundle/Bundle Item/current pointer/Task pin 及 Source/Secret/Audit 数据、queries/sqlc 生成物和显式清理顺序；Bundle Item 的既有 `public_name` 保存 Agent exported name，Definition snapshot 同时保存 canonical name，不新增可变 rename 表；所有新索引使用单语句 `CREATE [UNIQUE] INDEX CONCURRENTLY` migration。
- Dependencies：官方 Go MCP SDK、gRPC-Go、Protobuf dynamic descriptors/runtime compiler 与 OpenAPI parser；新增包只由 Server 入口导入，不进入 `multica` Daemon/CLI 执行图。
- Network/operations：Server 与目标 REST/gRPC/MCP 的网络、DNS、TLS、proxy、streaming、timeout、connection pool、rate/concurrency limit、metrics 与审计配置。
- Target Web：`apps/web` 仅增加 workspace route wiring，`packages/core` 增加经过 schema 校验的 Control Plane client/query，`packages/views` 增加共享业务页面与 Agent Tools 子视图；Source 页面区分 namespace/canonical/upstream name，Agent 页面提供 exported name 编辑、冲突校验与恢复默认，`packages/ui` 只在缺少必要原子组件时增补。
- Non-target：Desktop/Mobile/Docs、Daemon/CLI/Provider/协议源码、本机 MCP 管理和本机/VPN tunnel 均不在实现范围。
