# Phase 3：Squad Leader Evaluation（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 实现边界

- `POST /api/issues/{issueId}/squad-evaluated` 已从 Legacy Handler 切换到 Lightweight Core，并继续受 Task-scoped transport allowlist 与严格 JSON schema 约束。
- 端点只接受有效 Task Token；Actor、Task、Workspace、Issue 与 Squad 均从可信 Principal 和数据库行派生，不读取客户端 Actor/Squad 字段。
- 事务按 Issue、Task、Squad 的顺序显式加锁，重新校验 Task 仍为 active、属于目标 Issue 和该 Squad、标记为 Leader Task，且 Agent 仍为 Squad 当前唯一 Leader。
- `outcome` 冻结为 `action/no_action/failed`；可选 `reason` 会 trim、脱敏并限制为 500 Unicode 字符。未知字段、非法 UTF-8、NUL 和超长 reason 返回 400。
- 同一 Task 只允许一条 `squad_leader_evaluated` Activity。并发同 Body 重放返回同一个持久化 ID；同一 Task 后续提交不同 outcome/reason 返回 `409 evaluation_already_recorded`。
- `no_action` 仅写 Activity，不 enqueue、coalesce 或更新任何 Task。
- 未分配 Squad 的 Issue 返回 `400 issue_not_squad`；Squad Member Task 或已不再是 Leader 的 Task 返回 403。
- 实现不新增表、索引、外键或级联；一次性约束由 Issue 行锁和 Task-scoped Activity 查询共同保证。

## Fresh PostgreSQL 真实验证

在全新隔离本地 PostgreSQL 16.13 `multica_lightweight` 空库应用全部 77 个 Lightweight migrations 后执行：

```text
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' -count=1 -v

Result: PASS
```

真实流程覆盖：

- 非法 outcome、伪造 Actor 字段与 Human Token 均不写 Activity；
- 两个并发 `no_action` 请求都返回 201 和同一个 Activity ID，数据库恰好新增一行；
- reason 以 trim 后值持久化，Actor ID 等于可信 Leader Agent ID；
- 不同结论重放返回 409；`no_action` 前后 Issue Task 总数不变；
- 同 Squad Member Task 被 403 拒绝；Direct Agent Issue 的 Task 被 400 拒绝；
- Fresh Database Audit 同时确认仍为 26 张业务表、72 个索引、0 个外键。

远程 PostgreSQL 未创建数据库、未迁移、未写入。
