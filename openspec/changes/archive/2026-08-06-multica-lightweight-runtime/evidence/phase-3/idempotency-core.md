# Phase 3：Create 幂等并发门禁（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 实现边界

- Issue、Comment、Chat Message Create 均强制 `Idempotency-Key`，并按冻结的 Actor + Workspace + route/resource scope 查询。
- 三类请求都对规范化 wire body 计算 request hash；同 key 同 Body 返回首次持久化结果，不重复 Issue、Comment、Message 或 Task；同 key 不同 Body 返回 `409 idempotency_key_reused`。
- Comment 以 Issue 行锁作为并发 fence；Chat Message 以 Chat Session 行锁作为 fence。
- Issue Create 在 Workspace issue counter 锁之后执行第二次幂等查询；这覆盖相同 key 请求在首次无记录检查后并发穿越的窗口，包括不同 Assignee/Runtime 的请求。竞态 loser 回放或 409，partial unique index 不再泄漏为 500。
- 幂等 replay 不再次执行 Comment Mention routing，因此不会重复 enqueue/coalesce。

## Fresh PostgreSQL 真实并发验证

在全新 PostgreSQL 16.13 `multica_lightweight` 空库应用 77 个 migrations 后，`TestLightweightIdentityWorkspaceTokenLiveFlow` 使用真实并发 goroutine/HTTP 请求验证：

- 两个同 Actor、同 Workspace、同 key、同 Body 的 Issue Create 均返回 201 和同一 Issue ID，数据库只有一行；
- 两个同 key 同 Body 的 Comment Create 均返回同一 Comment ID，数据库只有一行，随后不同 Body 返回 409；
- 两个同 key 同 Body 的 Chat Message Create 均返回同一 Message/Task 结果，数据库只有一条 user Message 与一个 Task；
- 已有顺序用例继续验证三类 Create 的不同 Body 409，并验证 replay 不重复 Mention Task。

```text
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' -count=1 -v
Result: PASS
```

远程 PostgreSQL 未创建数据库、未迁移、未写入。
