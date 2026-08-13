# Phase 3：Runtime Profile、Agent 与 Skill（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 已实现边界

- Target Router 的 Runtime Profile CRUD、Runtime list/update/delete、Agent CRUD/archive/restore/env/skills 与 Skill/File CRUD 已切换到 `server/internal/lightweightapi` 和 26 表轻量查询包。
- Runtime models、local-skills inventory 与 local-skill import 的 9 条 human/daemon request-result Handler 已切换到 `server/internal/lightweightapi`；Target Router 不再把这些路径绑定到旧 `handler.Handler`。
- 临时 discovery/import request 使用单进程内存控制面，不新增数据库表或后台 Worker。状态为 `pending/running/completed/conflict/failed/timeout`，具有 pending/running timeout、5 分钟 retention、FIFO claim、最多 10 条 import batch 和终态回传幂等保护。
- HTTP 与 Daemon Control WS heartbeat 共享同一个 pending-action claim；新 Daemon 使用 batch import，未声明 batch 能力的 Daemon 每次只获取一条 import。
- Local Skill import 只允许 Runtime Owner 发起；Daemon result 必须匹配 Workspace、daemon、Runtime 和 request。导入 bundle 的相对路径经 traversal/duplicate 校验后，Skill 与 Skill Files 在一个事务内写入现有 26 表 baseline。
- Agent create/update 使用目标严格 DTO；Runtime 必须存在于同一 Workspace。Agent archive 在存在活动 Task 时返回冲突，restore 要求 Runtime 绑定仍有效。
- Agent 保留 model、thinking level、service tier、runtime config、custom args、max concurrency、permission mode 与 disabled runtime skills 契约。
- Skill File 更新使用事务执行完整集合替换；Agent-Skill 使用事务执行完整集合绑定；仍被 Agent 引用的 Skill 拒绝删除。
- Runtime 删除使用 Runtime/Agent 行锁：普通删除拒绝仍绑定的活动 Agent；显式解绑删除要求 `expected_active_agent_ids` 与锁内实际集合完全相等；Profile 派生 Runtime 拒绝删除。
- Agent create/update 在事务内先锁定目标 Runtime 行，再写入 Runtime binding；因此 Runtime delete 锁定该行后，不会有新的 Agent binding 越过集合确认并留下悬空引用。
- Agent `custom_env` 按键独立加密，`mcp_config` 整体加密。常规 Agent DTO 仅返回环境变量键名和 MCP 是否已配置；Env reveal/update 均写入审计，update 与审计在同一事务内提交。
- Daemon Claim 仅在通过 Workspace/daemon/runtime scope 后解密执行期 Env/MCP，并在同一事务签发 hash-only Task Token；解密失败会回滚 Claim。
- `private/public_to` 调用门禁统一由 Direct Chat 与 Squad wiring 复用；Workspace target 比较 Agent 所属 Workspace，Member target 先从当前 Workspace 的 `user_id` 解析 `member.id` 后再匹配 allowlist。Workspace Owner/Admin 只有资源管理权限，不会绕过调用门禁。

## PostgreSQL 16 真实流程

环境：全新隔离的本地 PostgreSQL 16 临时实例，数据库名 `multica_lightweight`，从空库执行完整 Lightweight migration。未连接或修改远程数据库，也未连接旧 `multica`。

```text
DATABASE_URL=<temporary-local-dsn> \
  MULTICA_EDITION=lightweight \
  EXPECTED_DATABASE_NAME=multica_lightweight \
  go run ./cmd/migrate up

LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run TestLightweightIdentityWorkspaceTokenLiveFlow -count=1 -v

Result: PASS

Fresh PostgreSQL evidence: migrations=77, target tables=26, PostgreSQL=16.13
```

该流程实际验证：

- Runtime Profile 创建、更新、被 Runtime 引用时删除冲突、解除引用后删除成功。
- Agent 创建响应不暴露 MCP 明文；数据库 JSONB 不包含 MCP 或 Env 明文，并可由配置的 Agent Secret key 正确解密。
- Env Update 响应只返回掩码，Env Reveal 只对授权 Owner/Admin 返回明文；两次操作产生对应 `activity_log` 审计记录。
- 普通 Member 不能修改他人 Agent、Runtime 或 Skill，也不能创建 Runtime Profile；Workspace Owner 可以执行这些管理动作。
- 跨 Workspace invocation target 被拒绝且事务回滚。
- Skill File 完整替换、Agent-Skill 绑定、绑定时删除冲突、解绑后删除及 Skill File 显式清理成功。
- Runtime 有绑定 Agent 时普通删除冲突；错误 expected set 冲突；正确集合确认后原子解绑 Agent 并删除 Runtime。
- 并发门禁先持有 Runtime 删除锁，再并发创建绑定该 Runtime 的 Agent；创建请求保持阻塞。删除提交后创建以 `agent_runtime_required` 失败，数据库不存在新 Agent 或悬空 Runtime ID。
- 空闲 Agent archive/restore 成功；活动 Task 存在时 archive 冲突；Task 终态后 archive/restore 成功。
- Owner 发起 model discovery、任意 Workspace Member 发起非敏感 local Skill/MCP inventory，heartbeat 原子领取三类 pending action；另一 Daemon 对目标 Runtime 回传 model result 返回 404。
- Model result 保存完整 thinking/service-tier DTO；重复失败回传不能把 completed 请求回退。
- Local Skill/MCP inventory 只返回非敏感摘要；结果可由 Workspace Member 查询。
- Runtime Owner 发起 Skill bundle import，Daemon 回传后事务创建一条 Skill 和两条 Skill File；重复回传不重复创建。普通 Member 发起读取 Owner 本机文件的 import 返回 403。
- Runtime 离线时 discovery 发起返回稳定 `runtime_offline`，不创建 pending request。
- `private` Agent 仅 Owner 可调用；Workspace Owner/Admin 不能调用或接入他人私有 Agent。`public_to` Workspace target 允许 Workspace Member，Member target 只允许被指定的 Member，另一普通 Member 返回 `403 invocation_forbidden`。
- 不存在的 Member target、跨 Workspace target 与跨 Workspace Squad Agent 均被拒绝且不产生部分写入；合法 Member target 同时通过 Direct Chat 和 Squad 间接调用。

## 静态与合同门禁

```text
go test ./internal/lightweightapi ./cmd/server ./pkg/lightweightdb \
  -run 'TestLightweight|TestRuntimeRequest|TestDaemon|TestTarget|TestBuild|TestTransport|TestListener' \
  -count=1
Result: PASS
```

`TestLightweightRuntimeRequestRoutesUseCore` 固定 9 条 request/result route 必须绑定 Lightweight Core；`TestLightweightCompositionDoesNotConstructExitedDependencies` 与冻结 114 路由集合共同证明 Target graph 不构造或暴露 Cloud Runtime、Runtime Usage、Connected App、Composio、Agent Template/Builder 和 Runtime Self-update 分支。旧完整产品源码的物理删除仍由 Phase 5 第 13 组门禁负责，不属于 7.2 的运行分支验收。

## 尚未宣称完成

- 历史 tombstone display 与完整 RP/SK/AG acceptance matrix 仍待后续任务完成。
- 因此 7.7、7.11 仍保持未完成；本证据不等同于完整 Phase 3 验收。
