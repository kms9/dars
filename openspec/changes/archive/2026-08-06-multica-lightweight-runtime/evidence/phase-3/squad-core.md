# Phase 3：Agent-only Squad Core（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 已实现边界

- Target Router 的 10 条 Squad 路由全部绑定 `server/internal/lightweightapi`，不再进入旧完整产品 Handler。
- Squad create/update/member mutation 只接受 `agent_id`；Human member 旧字段会被严格 DTO 拒绝。
- Leader 与所有新增/升职 Agent 必须来自同一 Workspace，并通过发起人的 `private/public_to` invocation gate；Workspace Owner/Admin 的管理权限不会绕过调用权限。
- 创建 Squad 与首个 Leader roster 在同一事务提交。Leader 更换先锁定 Squad，再原子 upsert 新 Leader、降级旧 Leader并同步 `squad.leader_id`；当前 Leader 不允许直接删除或降级。
- Squad archive 先锁定 Squad，并检查 `deferred/queued/dispatched/waiting_local_directory/running` Task。存在活动 Task 时返回 `409 active_tasks_exist` 且不改变 Squad 或 Task；仅有终态 Task 后才写入 archive tombstone。
- 归档 Squad 拒绝新的 member mutation，不提供 Restore 路由；已有 Issue 的 `assignee_type=squad` 与 `assignee_id` 原样保留。
- Member status 由 Agent archive、活动 Task 与 Runtime liveness 派生为 `archived/working/idle/unstable/offline`，不新增缓存表或 Worker。

## PostgreSQL 16 真实流程

环境：每次从空目录初始化的隔离本地 PostgreSQL 16.13，数据库名 `multica_lightweight`。远程 PostgreSQL 仅做过先前只读连通性检查，本阶段未创建数据库、未迁移、未写入。

```text
DATABASE_URL=<temporary-local-dsn> \
  MULTICA_EDITION=lightweight \
  EXPECTED_DATABASE_NAME=multica_lightweight \
  go run ./cmd/migrate up

LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' -count=1 -v

Result: PASS
Migrations: 77
Target tables excluding schema_migrations: 26
```

真实流程覆盖：

- 普通 Member 用自己的私有 Agent 创建 Squad；Workspace Owner/Admin 无法调用或接入该 Agent。
- Human member body、跨 Workspace Agent、不可调用私有 Agent 均被拒绝。
- Workspace target 与 Member target Agent 可被授权 Member 接入 Squad；非目标 Member 无法调用 Member-target Agent。
- Leader 更换后数据库中恰好一条 `role=leader`，且 `squad.leader_id` 同步；直接 remove 当前 Leader 返回 `409 leader_required`。
- Leader 活动 Task 使 member status 返回 `working`；归档返回 `409 active_tasks_exist`，Squad 保持未归档且 Task 保持 queued。
- Task 进入终态后归档成功；归档后新 member mutation 返回 `409 squad_archived`，历史 Issue 的 Squad assignee 保留。

## 静态合同门禁

```text
go test ./internal/lightweightapi ./cmd/server ./pkg/lightweightdb \
  -run 'TestLightweight|TestRuntimeRequest|TestDaemon|TestTarget|TestBuild|TestTransport|TestListener' \
  -count=1
Result: PASS
```

`TestLightweightSquadRoutesUseCore` 固定 10 条 Squad route 必须绑定 Lightweight Core，并禁止重新绑定旧 Handler。冻结 114 路由 contract 中不存在 Squad Restore。

## 尚未宣称完成

- 本证据只完成 7.6、8.4、8.5 与 8.6，不代表完整 Squad/Run/Comment/Task Core 已完成。
- Run 创建后的 archived Squad 分配门禁、全资源 Active/Idle lifecycle、跨表并发锁矩阵和 SQ/IS/CM/FLOW/RC/TQ 全量 acceptance matrix 仍由 8.1–8.3、8.7–8.18 后续任务覆盖。
