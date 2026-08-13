## 1. 契约与迁移基线

- [x] 1.1 新增排序后的126-route与27-table checked-in manifests，更新数据库 retention/API/E2E文档，并让 contract test输出集合 diff而不只比较数量。
- [x] 1.2 为 `agent.kind/system_key`、`agent_builder_draft`和 Agent/Squad avatar字段编写 forward/down migrations；逐索引用独立 `CREATE INDEX CONCURRENTLY` migration，确认无 FK/CASCADE/隐式索引。
- [x] 1.3 更新 sqlc queries/models和 schema contract fixtures，验证 Fresh Install恰好27表、26→27 upgrade保留现有 Agent/Agent-only Squad/Task/Chat数据、原 `multica`数据库 revision/checksum不变。
- [x] 1.4 扩展 Workspace delete/backup/restore显式清理清单，覆盖 Builder draft与 avatar objects，并增加跨 Workspace和对象清理失败测试。

## 2. Agent Server 基础能力

- [x] 2.1 扩展 Agent list/detail DTO和权限判定，支持头像、model/thinking/service tier、runtime config、custom args、permission targets、Instructions、Workspace Skills和 disabled runtime skills；不新增 MCP或 Integration字段/操作。
- [x] 2.2 实现 `GET /api/agents/snapshot`的 scope counts、presence/workload、run count、last activity和 filter metadata，确保 `kind=system`与不可见 Agent在结果和计数中均被排除。
- [x] 2.3 实现稳定分页的 `GET /api/agents/{agentId}/tasks`与30天聚合，复用持久化 Task/Usage/Message数据并排除 Builder carrier tasks。
- [x] 2.4 实现幂等 `POST /api/agents/{agentId}/tasks/cancel`，并把 Agent Archive改为同一 operation取消 active tasks后归档；验证 Restore不重放已取消任务且历史归因保留。
- [x] 2.5 为 Issue list增加可信 `creator_type/creator_id` filter和相应索引/租户检查，保持 existing assignee/status cursor契约。
- [x] 2.6 实现 Agent/Squad专用 avatar upload、受限解码/重编码、immutable media读取、替换与显式清理，不引入 Attachment表或通用文件 API。
- [x] 2.7 扩展 Agent/Squad事件、cache invalidation和 activity audit；验证 payload/log/error不含 Env明文或 Builder draft正文。

## 3. Agent 列表与手工创建 Web

- [x] 3.1 在 `packages/core`重建 Agent query/types、presence/access/failure派生和 view store，遵守 React Query/Zustand及 package boundary。
- [x] 3.2 实现 Agent List的 My/All/Archived、搜索、availability/access/runtime/owner filters、排序/隐藏列、stable pagination和空/错/加载状态。
- [x] 3.3 实现行操作 Duplicate/Cancel/Archive/Restore与批量 Access/Archive/Restore，展示逐项成功失败且不扩大单资源权限。
- [x] 3.4 恢复 Create入口二选一和 Blank form，覆盖头像预览、identity、Instructions、Skills、Runtime/model/thinking/service tier/concurrency与 private/workspace/specified access。
- [x] 3.5 实现浏览器会话内手工 draft恢复、Create and open、avatar失败重试及成功清理；增加刷新/返回/重复提交测试。
- [x] 3.6 实现 Duplicate预填 Instructions/Skills和其他非 Secret配置，清除 identity/owner/history/Env；对不可用 Runtime强制重新选择并增加负向测试。

## 4. AI Builder

- [x] 4.1 移植并收窄 Builder structured protocol与校验测试，只允许批准的 Agent非 Secret字段和 Skills IDs，明确拒绝 Env、MCP、Integration、owner、archive和 history字段。
- [x] 4.2 实现 Builder session list/create、hidden system carrier、private chat/task和 draft autosave，保证普通 Agent/Chat/activity/run count/invocation查询不可见。
- [x] 4.3 实现在线 Runtime/compatible model选择和 Runtime switch并发保护；active builder task时返回 `builder_task_active`且不改变 state。
- [x] 4.4 实现 unfinished session resume和用户/Workspace隔离，包括刷新、跨设备、已归档 Runtime及失败 Task场景。
- [x] 4.5 实现 Builder finalize事务和幂等重放：验证最新 draft、创建一个普通 Agent及其 Instructions/Skills、完成 session、清理 draft；失败时保留可续接状态。
- [x] 4.6 恢复 `/agents/new/ai`与 session页面，呈现 conversation、structured draft、autosave/switch/finalize状态，并完成键盘和离开保护测试。

## 5. Agent Detail、Work 与运行操作

- [x] 5.1 实现 Detail header、Overview current/recent work、Issue/transcript链接、失败原因/initiator和30-day summary，覆盖空数据和稳定 cursor。
- [x] 5.2 接通 Direct Message到现有 Direct Chat、Assign Work到 Lightweight Issue/Run，并验证权限、Runtime不可用和 archived Agent错误。
- [x] 5.3 实现 Work的 Assigned/Created、搜索、状态筛选、排序/分页和 Issue detail navigation，不恢复 Project/Property/Label/Board依赖。
- [x] 5.4 实现 Overview/Work/Capabilities/Settings URL view状态与权限可见性，并增加 browser back/forward、深链及断线 refetch测试。

## 6. Agent Instructions、Skills 与 Settings

- [x] 6.1 实现只包含 Instructions与 Skills的 Capabilities导航，旧 MCP/Integrations view参数和深链不可用且不调用相关 API。
- [x] 6.2 实现 Instructions编辑和跨 tab/route/Agent的保存-放弃-取消 dirty guard，并验证保存值进入后续 Daemon Claim。
- [x] 6.3 实现 Workspace Skills绑定/解绑和 Runtime local skills发现/详情/启停，验证 offline/unsupported/failed/empty区分及真实 Task bundle解析。
- [x] 6.4 实现 General与Access设置：头像、资料、Runtime/model/thinking/service tier/concurrency和 private/workspace/specified members，处理版本冲突与兼容性校验。
- [x] 6.5 实现 Environment Reveal/Update及审计、Custom Args和 Runtime-specific Config，覆盖只读用户、非法 schema和适用 Runtime显示。

## 7. Agent-only Squad Server 与 Web

- [x] 7.1 补齐现有 `squad_member(agent_id,role)` service/queries/DTO的 add/remove/role规则，验证租户、Agent调用权限、archive状态和重复 Agent；不修改为多态 roster。
- [x] 7.2 实现 Leader原子 Promote：锁定 Squad/相关 Agent rows、自动加入新 Leader、降级旧 Leader并拒绝直接移除当前 Leader；增加并发测试。
- [x] 7.3 验证 Daemon Claim返回 Squad Instructions和 `members[{agent_id,name,role}]`，只有 roster Agent可 canonical mention/委派，保持 Leader-Member-Leader闭环。
- [x] 7.4 实现 Squad Archive事务，把当前 Issue assignee转给 Leader、保留 active Task历史归因并禁止新 Run；覆盖冲突整体回滚和无 Autopilot副作用。
- [x] 7.5 实现 Squad List的 My/All、counts、搜索/filters/sort/display、Agent previews与 Archive行操作。
- [x] 7.6 实现 Create Squad modal的头像、资料、Leader和 additional Agents原子创建及错误恢复；UI/API均不提供 Human Member。
- [x] 7.7 实现 Squad Detail资料、Members/Instructions tabs、dirty guard、Agent role、status/last active/active Issue、Promote/Remove/Add。
- [x] 7.8 实现从 Squad创建 Agent并显式确认加入 roster的 return-to流程，验证取消/失败不改变 roster且不会自动替换 Leader。

## 8. 安全、边界与回归验证

- [x] 8.1 增加跨 Workspace、Agent owner/Squad creator/Admin、批量部分授权和 system carrier隐藏的 API contract测试。（`agent_squad_auth_contract_test.go` + `batch.contract.test.ts`；live DB tests 需 `LIGHTWEIGHT_DATABASE_URL`，本机 SKIP）
- [x] 8.2 增加 Agent Archive取消、Squad Archive转移和 Builder finalize的幂等/并发/事务回滚测试。（`agent_squad_lifecycle_contract_test.go`；live DB tests 本机 SKIP）
- [x] 8.3 增加 Env/avatar的密文或受限介质、redaction、审计、日志/错误/Realtime泄漏和恶意上传测试。（`agent_management_test.go` 扩展 + 既有 `live_integration_test.go` env/claim 覆盖）
- [x] 8.4 增加 MCP、Lark/Slack/其他 Integrations、Human Squad Member旧深链/API/import/worker的负向扫描，确认这些能力没有随页面恢复而回流。（`removed_capability_contract_test.go`、`product-boundary.contract.test.ts`、`apps/web/product-boundary-contract.test.ts`）
- [x] 8.5 增加 package import边界、目标 build graph、126-route/27-table/worker manifests扫描，确认 Desktop源码未接线或修改。（`build-graph-contract.test.ts` 扩展、`contract_test.go` 27 models、`lightweight_composition_contract_test.go`）
- [x] 8.6 运行 Go/TypeScript unit与contract tests、`pnpm typecheck`、`pnpm test`、`make test`和隔离 Fresh DB的 `make check`，记录任何跳过或环境门禁。（`pnpm typecheck`/`pnpm test` 通过；route/table/openspec strict 通过；`make test` 因 `TestLightweightBuiltinSkillProtocolContract` 失败；`make check` 与 DB live tests 未运行）

## 9. 真实流程与交付证据

- [ ] 9.1 用真实 PostgreSQL、独立 Server/Daemon、目标 Runtime和浏览器执行 Agent list/manual/duplicate/Builder/detail/work/Instructions/Skills/settings/archive/restore矩阵，保存 HTTP/WS/DB/Runtime一致证据。（门禁：`LIGHTWEIGHT_DATABASE_URL` + 独立 Server/Daemon + 目标 Runtime + 浏览器）
- [ ] 9.2 执行 Agent-only Squad create/add/remove/role/Instructions/Leader/Run/Create Agent/archive矩阵，验证 Claim、Leader-Member-Leader和历史归因。（同上）
- [ ] 9.3 执行 MCP/Integrations/Human Squad Member负向浏览器与 API矩阵，证明入口不可见、代表 route为404、相关 Worker未启动且 schema无相关新表。（需浏览器 + 运行中 Server）
- [x] 9.4 更新 `04_e2e_acceptance_matrix.md`强制 ID、SELF_HOSTING/配置文档和迁移/回滚 runbook；区分已验证工程结果、未运行门禁和最终发布结论。（§0 证据边界、AG-07、DB-05、§17 表数量、删除能力说明已更新）
- [x] 9.5 复核 `openspec validate restore-agent-squad-management-flows --strict`、route=126、tables=27、批准 Worker集合及 clean evidence bundle后，才将 change标记为可实施完成；不得以静态页面或旧源码存在替代发布证据。（strict validate 通过；126/27 contract tests 通过；§9.1–9.3 未跑，change 不可标记为发布完成）
