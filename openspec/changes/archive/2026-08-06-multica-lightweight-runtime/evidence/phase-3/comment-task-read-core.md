# Phase 3：Comment 与 TaskRun 读取面（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 已实现边界

- Target Router 的 Issue Comment List/Create、Issue TaskRun List、Active Task 和 Task Messages 5 条路由已绑定 `server/internal/lightweightapi`，不再进入旧完整产品 Handler。
- Comment 只允许 Issue 下 List/Create；冻结路由中没有 Update/Delete/Resolve/Reaction，保持 append-only。
- Comment Create wire DTO 只接受 `content`；`workspace_id/issue_id/author_type/author_id/type/source_task_id` 全部来自服务端认证上下文与目标 Issue/Task，不接受客户端伪造。
- Human Comment 固定归因为 `member + user_id`；Task Token Comment 固定归因为 `agent + token.agent_id + token.task_id`，并二次验证 Task、Agent、Issue 和 Workspace 一致。
- Comment Create 强制 `Idempotency-Key`；Issue 行锁串行化同一 Issue 的创建，相同 Actor/Workspace/Issue/key 与相同 Body 返回同一 Comment，不同 Body 返回 `409 idempotency_key_reused`。
- Comment 内容拒绝空值、NUL、非法 UTF-8 与超限 payload；持久化类型固定为 `message`。响应不暴露 `idempotency_key/request_hash`。
- Flat Comments 使用 `created_at ASC,id ASC` opaque cursor，默认 30、范围 1..100，author display 在单条 SQL 中解析，不产生逐条 display 查询。
- TaskRun 使用 `created_at ASC,id ASC` opaque cursor；Agent/Runtime/Squad 名称由查询 JOIN 一次展开，删除关联时返回稳定 tombstone display。
- TaskRun DTO 包含 leader、attempt/max_attempts、failure、usage、session/workdir 与 dispatched/started/completed lifecycle；usage 对本页 Task ID 批量读取，不产生逐 Task N+1。
- Task Messages 使用独立 opaque cursor 查询。Human 只能读当前 Workspace Task；Task Token 只能读自身 Task Message，且不能读取 TaskRun 历史。
- Active Task 查询与历史 TaskRun 查询分离；终态 Task 不再出现在 Active Task 结果中。

## PostgreSQL 16 真实流程

环境：全新隔离本地 PostgreSQL 16.13，数据库名 `multica_lightweight`，从空库应用全部 Lightweight migrations。远程 PostgreSQL 未创建数据库、未迁移、未写入。

```text
DATABASE_URL=<temporary-local-dsn> \
  MULTICA_EDITION=lightweight \
  EXPECTED_DATABASE_NAME=multica_lightweight \
  go run ./cmd/migrate up

LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightmigrations ./internal/lightweightapi \
  -run 'TestLightweightFreshDatabaseAudit|TestLightweightIdentityWorkspaceTokenLiveFlow' \
  -count=1 -v

Result: PASS
PostgreSQL: 16.13
Migrations: 77
Target tables excluding schema_migrations: 26
Foreign keys: 0
```

真实流程覆盖：

- Member 创建 Comment 后，同 key 同 Body 返回同一 ID，不同 Body 返回 409；伪造 author 字段返回 400。
- Active Task Token 为自身 Issue 创建 Comment，存储的 author 为 Agent、`source_task_id` 为自身 Task；读取其他 Issue Comments 返回 403。
- 两条 Comment 以 `limit=1` 分两页读取，顺序稳定且无遗漏。
- Daemon 写入 Task Message 与 Usage 后，Task Token 可读自身 Message、其他 Task 返回 403。
- Member 读取 TaskRun 时得到 Agent/Runtime/Squad 名称、Leader 标记、usage 与 started lifecycle；Task Token 读取 TaskRun 历史返回 403。
- Task 完成前 Active Task 包含该 Task，完成后返回空数组。

## 静态合同门禁

```text
go test ./internal/lightweightapi ./cmd/server \
  -run 'TestLightweight|TestOpaqueCursor|TestBuild|TestTransport' -count=1
Result: PASS
```

`TestLightweightCommentAndTaskReadRoutesUseCore` 固定 5 条目标 route 必须绑定 Lightweight Core。

## 尚未宣称完成

- 本证据完成 8.7 与 8.13；后续 8.8、8.9 的闭环证据记录在 `mention-reentry.md`。
- 8.12 需要把 Issue、Comment、Chat Message 三类幂等的并发 race 一并验证后再完成；本阶段只新增并验证 Comment 的串行化幂等。
- 8.18 的完整 SQ/IS/CM/FLOW/RC/TQ/API 矩阵尚未执行。
