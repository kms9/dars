# MCP Gateway 发布证据

> Change：`add-server-side-mcp-gateway`  
> 记录日期：2026-08-11  
> 结论：Server 实现、目标 Web Control Plane、Fresh DB、受控本地 fixture E3 和安全负向矩阵已通过。Codex 的 unchanged-Daemon 真实 Runtime E3 是路径迁移前证据，最终 `/bundles/{bundleId}/mcp` 路径的真实 Runtime E3 仍需重跑；OpenSpec 当前完成 73/74，唯一未完成项是 9.5，因此该发布门禁保持打开。代码 allowlist 只声明 `codex` 支持，其他未测试 Provider 继续 fail closed。本报告证明受控本地环境下的实现与运行时兼容性，不等同于生产部署完成或任意外部依赖可用。

## 范围结论

- Gateway 执行能力只存在于 Go Web Server 后端，包括 Tool Source registry、不可变 ToolBundle、Task pin、Claim Gateway 投影、MCP Facade、Remote MCP/gRPC/OpenAPI adapter、egress policy、Secret、metrics 与 audit。
- 目标 Web/shared frontend 新增且只新增 Owner/Admin Human Control Plane：Workspace `Tools` 导入/管理页面和 Agent `Capabilities > Tools` 选择/发布页面。浏览器不解析、编译或执行 Swagger/OpenAPI/Proto，不读取 raw `agent.mcp_config`，也不管理 runtime-local MCP。
- `server/internal/daemon`、`server/pkg/agent`、`server/cmd/dars` 与 Daemon protocol 没有 Gateway 实现或依赖；Desktop、Mobile 与 Docs app 没有新增产品能力。
- Daemon 现有 runtime-local MCP merge 保持原状。本 change 不要求 Server 看见、代理或禁用本机 MCP，本机 MCP 条目可以继续存在。
- 外部 HTTP、gRPC 或 MCP 依赖服务的当前 uptime、网络稳定性与兼容性不是本阶段正确性或发布完成度证据；本报告只记录 Server 和受控本地 fixture 能力。

## ToolBundle 与 MCP Facade 结论

- Agent 选中的 MCP/tools/Proto pack 会物化为带 opaque `tb_` ID 的不可变 Bundle。no-op 保存可保留当前 ID；任何语义变化以及 revert 都会创建新 ID，并原子切换 Agent head。
- Tool Source 上传时配置的 Name 是不可变 canonical namespace；每个 Agent 在发布 Bundle 时可以为选中的 Tool 配置独立 `exported_name`。只改 exported name 也属于语义变化并生成新 Bundle ID，不修改 Tool Definition、Source 或 upstream invocation plan。
- Bundle publication 使用严格的 `items[{tool_definition_id, exported_name}]` 合约；Server trim 后校验 MCP-safe 语法与 Bundle 内唯一性。`tools/list` 只公开 exported name，`tools/call` 也只按 exported name 解析，canonical/upstream name 不作为隐式别名。
- Task 创建时写入一次 Bundle pin，Task Token 同时绑定 Task、Workspace、Agent 与 Bundle；Claim/re-Claim 不会跟随后续 Agent head 或 Source current revision。
- Bundle Task 的 Claim 只生成一个 `mcpServers.dars`，URL 指向 `DARS_PUBLIC_URL/bundles/{bundleId}/mcp`，header 使用当前 `dat_` Task Token。无 Bundle Task 保持既有 Claim 语义。
- `tools/list` 与 `tools/call` 只以数据库中的 Task pin 和不可变 Bundle item 为权威，并在每次操作前检查 Task/Agent/Bundle/Source/tool 的实时撤权状态。Bundle ID 不是 credential。
- Token 只持久化 hash，Source credential 只持久化 domain-separated AEAD envelope；audit、metrics、错误与响应不记录明文 token、credential、请求参数、结果或 artifact 内容。

## 最终验证记录

以下命令在 2026-08-11 当前工作区实际返回 0：

| 命令 | 证据范围 |
| --- | --- |
| `make sqlc` | sqlc 重新生成成功，Tool Registry query/generated output 保持一致。 |
| `gofmt`、相关包 `go vet`、focused tests、`go test ./... -count=1`、target builds | Go 格式、静态检查、非 DB 分支、依赖边界与构建。 |
| 隔离 DB 上的 `make test ...` | 使用 `dars_lightweight_check_make_test_94405` 执行 migrations 与全包 `go test -race`；全部通过后数据库删除。 |
| `make check` | 使用自动生成的 Fresh DB 完整执行 migrations、4 个 target TypeScript typecheck、Web production build、core 73/views 49/Web 71 个单测、全包 Go tests、专用 backend/frontend 与 Playwright E2E；1 个综合 E2E 通过，结束后服务停止且数据库删除。 |
| `openspec validate add-server-side-mcp-gateway --type change --strict --no-interactive` | OpenSpec schema 与 change artifact 严格校验。 |

`make check` 中的 Fresh DB tests 没有 skip，实际覆盖：

- Claim 精确生成 `/bundles/{bundleId}/mcp`，Chi 只挂载该新路由，旧 `/mcp/bundles/{bundleId}` 返回 404；MCP endpoint 继续绕过 REST error envelope。
- 精确 schema/table/constraint manifest、无 Foreign Key/Cascade、每索引独立 `CREATE [UNIQUE] INDEX CONCURRENTLY` migration、唯一性与 schema identity。
- Bundle immutable trigger、跨 Workspace 拒绝、Task pin write-once、Source/Bundle retention、显式 Workspace cleanup 与 additive rollback。
- Source Secret ciphertext scope/swap/redaction，以及 Task Token 不落明文。
- Control Plane role/strict body、revision publish、partial rollback、Bundle no-op/change/revert、disable/delete/revoke 与 retained artifact。
- multipart Swagger 原子创建/更新、失败时零 Source 残留、Agent current Bundle read/clear 与历史 Bundle/Task pin retention。
- Claim、MCP official client、多副本 stateless projection、Agent head/Task pin 并发与终态撤权。

`make test` 使用单独的隔离数据库补充了 race detector 证据；未对原始 `dars` 或 `dars_lightweight` 数据库执行迁移或测试。

## Web Control Plane E2

- Workspace sidebar 仅向 Owner/Admin 展示 `Tools`；直接访问 `/tools`、`/tools/{id}` 或 Agent `cap=tools` 的 Member 会得到权限错误，且不会调用 Tool Source API。
- Tools 页面支持 OpenAPI 3/Swagger 2 JSON/YAML 文件或 URL、`.proto`/ZIP/descriptor set，以及 HTTP(S) Streamable Remote MCP endpoint。上传走 Server bounded multipart 原子导入，URL 走 strict JSON；页面只展示文件元数据、revision/status、脱敏 endpoint/credential 状态和发现的 catalog。
- Swagger 2 JSON/YAML 在 Server 内识别并转换；只有能通过统一 OpenAPI 3 校验/编译的结果才发布，不能确认无损转换时 fail closed。浏览器依赖图 contract 排除 MCP SDK、kin-openapi、gRPC 与 Proto compiler。
- Agent `Capabilities > Tools` 恢复当前 Bundle，支持整 Source/单 Tool 选择、不同 Agent 独立快照、no-op/new-ID 反馈、Source revision update warning、清除 head 与显式 revoke；页面说明旧 Task 继续使用创建时 pin 的 Bundle。
- Agent 选中 Tool 后可编辑或重置 per-item exported name；页面展示 canonical MCP name 与 upstream operation，阻止非法或重复名称。发布、刷新恢复、不同 Agent 隔离和 alias-only 新 Bundle ID 均有 shared-view/Go/browser 自动化覆盖。
- Playwright 从默认账号/默认 Workspace 进入 `/demo/tools`，通过页面上传 Swagger 2 文件、启用 Source，为两个 Agent 选择不同 Tools 并得到不同 `tb_` ID，刷新后恢复第一个选择，再清除其 current head。测试同时断言 multipart 请求保持在隔离前端 origin，由 Next rewrite 到 Fresh DB 后端。

这些是 Web 产品操作链和 Server API 的 E2/browser 证据，不替代 unchanged-Daemon/Provider 的最终真实 Runtime E3。

## Agent exported name 增量验收

本轮在不修改 Daemon/CLI/Provider 的前提下完成并验证了 Agent 级 Tool 名称映射：

- Go control-plane tests 覆盖默认 canonical name、自定义 alias、alias-only 新 Bundle、重复/非法名称拒绝、失败发布不切换 head 与 no-op 保存；registry integration 使用 MCP official client 证明 `tools/list` 只返回 exported name、`tools/call` 只接受 exported name，并在 metadata-only audit 中分别记录 exported/canonical/upstream identities。
- `packages/core` schema/API tests 覆盖严格 publication DTO 与 malformed response；`packages/views` tests 覆盖默认、编辑、重置、重复/非法名称阻断、刷新恢复和发布 payload；Playwright 覆盖页面配置 alias、发布、刷新恢复、不同 Agent 独立 Bundle 与清除 head。
- `go test ./cmd/server ./internal/mcpgateway ./internal/lightweightapi`、`make test`、core/views 单测、`pnpm typecheck`、`pnpm lint`、`pnpm build`、独立 Playwright、最终隔离 `make check` 与 strict OpenSpec validation 均通过。
- 冻结路径扫描确认 `server/internal/daemon`、`server/pkg/agent`、`server/cmd/dars`、Desktop、Mobile 与 Docs 无本轮改动；旧 `tool_definition_ids` publication DTO 已从目标 Server/Web/shared/E2E/change 范围移除。

上述证据关闭 OpenSpec 12.7，但不关闭 9.5。最终 `/bundles/{bundleId}/mcp` 的 unchanged-Daemon 真实 Provider E3 矩阵仍必须单独执行，不能由 Server integration、controlled fixture 或浏览器 E2E 代替。

## 受控本地 fixture E3

`TestDatabaseRegistryControlledAdapterE3OnFreshCheckDatabase` 在 Fresh DB 全量门禁中通过。该测试通过真实 Control Plane、数据库、TaskAuth、MCP official client 与 adapter 闭合以下四条链：

1. Remote Streamable HTTP MCP。
2. Server embedded descriptor unary gRPC。
3. 上传 `.proto` 编译后的 unary gRPC。
4. URL 导入 OpenAPI document 后的 HTTP operation。

同一个不可变四项 Bundle 被 pin 到 Task，随后通过 Bundle URL 执行 `tools/list` 和四次 `tools/call`。断言关联了 ready/current Source revision、Bundle item/artifact、Task/Token pin、MCP result、上游精确调用次数与认证观测，以及四条 metadata-only audit。fixture payload/auth sentinel 不出现在 audit，Token 与 Source Secret 不以明文持久化。

这只证明当前 DARS Server 对受控本地依赖的 contract，不证明任意外部依赖服务可用。

## 安全负向矩阵

Fresh DB 的 `make check` 已执行完整矩阵：

- tenant、Workspace、Agent、Task、Bundle 边界；同 Token 跨 Bundle、错误 Token、语法合法但不存在的 Bundle 都 fail closed，不泄露 Bundle pin。
- Bundle/revision immutable-write、跨 Workspace 写入、partial publication 与 cleanup/retention 约束。
- Source credential ciphertext、ciphertext swap、protected headers、Token hash 与各层 redaction sentinel。
- MCP transport/body/auth、非法 ID/未知字段、Proto/OpenAPI schema、streaming/unsupported capability 与 artifact/archive limits。
- endpoint/header override、DNS rebinding、混合 DNS answer、private/metadata/loopback、redirect、proxy bypass、TLS hostname 与 gRPC resolver/dial policy。
- deadline、cancellation、body/response size、Task/Source concurrency、shutdown drain、connection retirement。
- Remote MCP、gRPC 与 OpenAPI 非幂等调用均不做透明 retry，并校验上游精确调用次数。
- retained artifact、Source disable/delete、Bundle revoke、Task terminal token 与 Workspace 显式删除顺序。

## 未修改 Daemon 与 Codex Runtime E3（路径迁移前证据）

冻结目录 contract 和 `cmd/dars` dependency graph 已通过，证明当前源码没有把 Gateway/MCP/gRPC/OpenAPI/Proto compiler 引入 Daemon/CLI。在此基础上，本次使用 `make build` 生成的 Server 与 `dars` 发布二进制、Fresh DB `dars_lightweight_check_provider_e3`、未修改的 `/usr/local/bin/codex` 0.147.0 和受控本地 OpenAPI fixture 完成了真实 Runtime E3。该次执行发生在 Facade 路径由 `/mcp/bundles/{bundleId}` 迁移为 `/bundles/{bundleId}/mcp` 之前，因此只能证明旧路径下的 Daemon/Provider 投影行为，不能替代最终路径的 exact-path 重跑。验证代理只记录 Claim/Gateway 路径、JSON-RPC method 和 Authorization SHA-256；它不保存或返回明文 token。该代理不实现 WebSocket upgrade，因此 Daemon 控制连接按既有逻辑降级为 HTTP polling；Task Claim、心跳和结果上报仍走真实 Server API。

唯一声明支持的 Provider 是 `codex`，证据如下：

- 语义配置变化生成不同 Bundle：B1 `tb_01KZPR552D9308BHBGEX5D2RVR` 只含 `runtimeEcho`，B2 `tb_01KZPR554G9DMEZB5CRJTTFF8X` 增加第二个工具。T1 在 B2 发布前固定 B1，但在 B2 发布后才 Claim，最终仍从 B1 URL 完成调用；T2 固定并使用 B2。
- 两次 Server Claim 记录的 `mcpServers` key 都精确为 `dars`，URL 分别保留当时旧路由的完整 B1/B2 path。各 Claim 的 Authorization 摘要与后续同 Bundle `initialize`、`tools/list`、`tools/call` 摘要一致，证明 Provider 在该次旧路由实验中保留了 URL 与 header。
- 两个 Task 的 per-task Codex config 同时包含 `dars` 与既有 runtime-local `computer-use`、`node_repl`、`openaiDeveloperDocs`；这证明 Daemon 原有本机 MCP merge 保持工作，不要求隐藏本机 MCP。
- 真实 Codex 分别返回 `TOOL_OK B1_RUNTIME_E3` 与 `TOOL_OK B2_RUNTIME_E3`。fixture 精确观察到 `runtimeEcho=2`、参数依次为 `B1_RUNTIME_E3`、`B2_RUNTIME_E3`，新增的 `runtimeSecond` 未被误调用。
- 两条 `mcp_gateway_tool_invoked` audit 均记录正确 Task、Agent、Bundle、item、Source/revision、latency 与 sizes，未记录参数、结果或凭据。两个 Task 终态后 `task_token` 行数为 0，使用捕获的原 token 重放 B1/B2 Gateway 请求都返回 401。
- 同一 Task 的 token 重签和 re-Claim 由 Fresh DB `TestClaimBundleProjectionAndReclaimOnFreshCheckDatabase` 覆盖：第二次 Claim 仍投影原 B1、生成不同 token、删除旧 token hash，并把新 hash 绑定同一 Task/Bundle。真实 Runtime E3 负责证明未修改 Daemon/Codex 能消费这一投影；报告不把 Server 集成测试伪装成第二次 Provider 进程执行。

代码 allowlist 仍只启用 `codex`。Claude、Cursor、OpenCode 及其他 Provider 未执行该矩阵，继续在 Bundle publication、Agent runtime change、Task creation 与 Claim 阶段返回 `provider_mcp_unsupported`。在最终 `/bundles/{bundleId}/mcp` 路由完成 unchanged-Daemon exact-path 重跑前，本 change 不恢复 production-ready 结论。受控 fixture 不证明外部 OpenAPI/MCP/gRPC 服务的 uptime、网络或生产兼容性；真实生产部署、跨网络连通性、容量与长期稳定性仍需独立上线验收。

## 验证基础设施修正

为让现有 release gate 真实反映结果，本 change 同时修正了验证 harness：

- `scripts/check.sh` cleanup 保留失败退出码，避免前置失败被误报为成功。
- Web production build 显式绑定本次 check backend 的 same-origin proxy，并通过 Turbo 声明 `REMOTE_API_URL` build env。
- `CHECK_BACKEND_PORT` / `CHECK_FRONTEND_PORT` 允许验收服务避开已运行的本地应用；覆盖时同步 `PLAYWRIGHT_BASE_URL`，并把浏览器 API/WS 固定到隔离前端 origin，防止误测 `.env` 中的既有站点。
- Playwright 使用刚构建的 `next start` production artifact；Squad E2E selector 对齐当前 Create Squad modal。

这些 harness 改动只修复构建/E2E 运行方式；Web 产品能力限定在上述 Control Plane，Daemon 行为没有改变。

## 回滚

先清除 Agent current Bundle head，撤销活跃 Bundle 或禁用 Source，等待有界在途调用结束并关闭 replica-local channel，再部署上一版 Server。旧 Server 会忽略新增表；schema 删除与 retained artifact 清理属于后续独立的破坏性变更。
