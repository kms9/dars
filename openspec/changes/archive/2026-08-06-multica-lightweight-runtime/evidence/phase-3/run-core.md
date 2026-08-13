# Phase 3：Lightweight Run Core（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 已实现边界

- Target Router 的 Issue List/Create/Get/Update/Delete 5 条路由已绑定 `server/internal/lightweightapi`，不再进入旧完整产品 Issue Handler。
- Create/Update 只接受 `title/description/status/assignee_type/assignee_id/acceptance_criteria/context_refs`；Create 要求 title、status 和完整 assignee pair，拒绝未知字段、非法 null、非法状态和不完整 assignee。
- Run 原生支持 `backlog/todo/in_progress/in_review/done/blocked/cancelled` 与 `agent/squad` Assignee；DTO 返回 `identifier=issue_prefix-number`，不暴露 idempotency/request hash。
- Agent/Squad Assignee 必须来自同一 Workspace、未归档且满足发起人的 invocation gate。Squad 必须恰好一个与 `squad.leader_id` 一致的 Leader；初始 Task 固定派给该 Leader。
- `todo` Create 在 Issue、Task 和 `first_executed_at` 同一事务中创建一个 Leader/Agent Task；`backlog` Create 不触发，首次更新为 `todo` 才触发。Issue 行锁与 `first_executed_at IS NULL` 门禁阻止重复触发。
- Runtime offline 不丢任务：合法 Assignee 仍创建 queued Task，由在线后的 Daemon claim；缺失 Runtime 或归档 Agent 使用稳定冲突码拒绝。
- Task Token 只可读取自身 Issue，并只可更新 `status/assignee`；不能读其他 Issue或修改标题、描述、验收条件、上下文。
- Issue Create 强制 `Idempotency-Key`；相同 Actor/Workspace/key 与相同规范化 Body 返回原 Issue，不同 Body 返回 `409 idempotency_key_reused`。
- Run List 默认/范围沿用 30 与 1..100，按 `updated_at DESC,id DESC`，支持重复 `status` 以及 assignee type/id filter，返回 `{items,next_cursor}`。
- 修复共享 opaque cursor 的 RFC3339 round-trip：原实现错误地用 pgx PostgreSQL 文本 scanner 解析服务端生成的 RFC3339；现在使用 `time.RFC3339Nano`，并由 `TestOpaqueCursorRoundTripUsesRFC3339` 固定。
- Issue Delete 先锁 Issue，存在 Active Task 时返回 `409 active_tasks_exist`；否则在一个事务中显式删除 Task Token、Message、Usage、Comment、Activity、终态 Task 和 Issue，不依赖 FK/CASCADE。

## PostgreSQL 16 真实流程

环境：全新隔离本地 PostgreSQL 16.13，数据库名 `multica_lightweight`，从空库应用全部 Lightweight migrations。远程 PostgreSQL 未创建数据库、未迁移、未写入。

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

- Archived Squad 创建 Run 返回冲突且 Issue 行数不变；活动 Squad 和可调用直接 Agent 均可作为 Assignee。
- Backlog 创建无 Task；backlog→todo 生成恰好一个 Leader Task，重复 todo update 不重复触发。
- Todo Create 立即生成一条 Leader Task并设置 `first_executed_at`；Blocked Create/Get/Update 原生保留 blocked 且不触发 Task。
- Daemon claim/start/complete Leader Task 后，Task Token 可把自身 Issue 推进到 `in_review`，不能越权读取其他 Issue或修改禁用字段；Task complete 不把 Issue 自动改成 done/cancelled。
- Active Task 下 Delete 返回 409；Task 终态后 Delete 返回 204，关联 Comment、Task 与附属数据均清理。
- Create 同 key 同 Body 返回同一 ID，不同 Body 返回 409。
- 三条相同 `updated_at` 的 blocked Run 以两页返回，3 个 ID 无重复、无遗漏；组合 status/assignee filter 正确。
- 额外注入另一 Workspace 且 assignee_id 指向当前 Workspace Squad 的无 FK 脏引用；当前 Workspace Run List 不泄漏该行。

## 静态合同门禁

```text
go test ./internal/lightweightapi ./cmd/server ./pkg/lightweightdb \
  -run 'TestLightweight|TestOpaqueCursor|TestRuntimeRequest|TestDaemon|TestTarget|TestBuild|TestTransport|TestListener' \
  -count=1
Result: PASS
```

`TestLightweightIssueRoutesUseCore` 固定 5 条 Issue route 必须绑定 Lightweight Core；`TestOpaqueCursorRoundTripUsesRFC3339` 固定 cursor 可由服务端生成后读回。

## 尚未宣称完成

- 本证据完成 8.1、8.2、8.3；Comment/Mention、Task Run 展开、Squad Evaluation 与全量 IS/SQ/FLOW/RC/TQ acceptance matrix 仍待 8.7–8.18。
- Issue lifecycle 已具备目标 Delete 语义，但 8.14 要求的全资源 lifecycle 矩阵、8.15 要求的全跨表并发矩阵尚未完成，因此两项继续保持未完成。
