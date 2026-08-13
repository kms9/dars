# Phase 2：Task 生命周期与终态回调（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 已实现边界

- Target Router 的用户 cancel、Task status、prepare lease、pending、waiting_local_directory、start、progress、message、usage、session、complete、fail、cancel-ack 与 orphan recovery 已切换到 `server/internal/lightweightapi` 和 26 表轻量查询包。
- 所有 Daemon Task 请求先验证 Daemon Token 的 Workspace/daemon_id，再验证 Task 绑定 Runtime 当前仍属于该 Daemon；越界统一返回 404。
- Complete/Fail 在事务内锁定 Runtime 与 Task，终态写入、Issue Comment 或 Chat Assistant Message 投影、Chat resume 更新和 Task Token 删除共同提交。
- Complete→completed、Fail→failed 只允许从冻结前置状态进入；同一种终态回调重复提交返回数据库中已保存的原结果，不覆盖结果、不重复 Comment/Chat Message，也不重复终态事件；相反终态回调返回 409。
- Issue Complete 在有输出时创建一条 `author_type=agent`、`source_task_id` 可信生成的 Comment。Direct Chat Complete 始终创建一条 Assistant Message，空输出使用 `message_kind=no_response`；Chat Fail 创建带 `failure_reason` 的 Assistant Message。
- Task Message 使用 `(task_id, seq)` 去重，重复 seq 不覆盖原消息且不重复发布事件；Usage 使用 `(task_id, provider, model)` 幂等写入并拒绝负数。
- Claim 提交后发布一次 `task:dispatch`；waiting、running、progress、message、completed、failed 在数据库写入成功后发布目标白名单事件。
- Orphan recovery 在 Runtime 行锁下将 dispatched/waiting/running Task 收敛为 failed，并在同一事务删除其 Task Token。
- 用户取消 queued Chat Task 时立即生成 Draft Restore 并完成收敛；取消运行中 Chat Task 后立即撤销 Task Token，Daemon status poll 得到 cancelled，cancel-ack 原子清除 deferred marker。取消输入以 Draft Restore 持久化，重复 cancel/ack 不重复事件或 Draft。
- Daemon Complete payload 已移除退出字段 `branch_name`，对应回归测试改为断言该字段不存在。

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
```

该流程实际验证：

- Claim → prepare lease → waiting_local_directory → running → progress → message/usage/session → complete 的真实 HTTP 与事务链路。
- 另一个同 Workspace Daemon 不能读取或修改不属于其 Runtime 的 Task。
- 重复 Message seq 保留首次内容且只发布一次事件；`since` catch-up 只返回更大的 seq。
- Complete 重放不覆盖首次输出；Complete 后 Fail 返回 409；Task Token 在终态后立即 401。
- 两个并发 Complete 请求均得到同一 completed 状态，数据库只存在一个终态 Comment。
- Chat 空输出 Complete 只生成一个 `no_response` Assistant Message；Chat Fail 重放只保留一个带 failure_reason 的 Assistant Message。
- Orphan recovery 将 Task 收敛为 `failed/runtime_restart` 并立即撤销 Task Token。
- 用户取消 running Chat Task、Daemon 轮询 cancelled、重复 cancel、两次 cancel-ack、Draft Restore 唯一写入与 `chat:cancel_finalized` 单次发布。
- 由 Chat Send API 创建的 queued Task 在 Claim 前取消后立即进入 cancelled、生成唯一 Draft Restore，并通过外部 List/Delete API 幂等消费。
- deferred cancellation sweeper 在 grace 到期后生成唯一 Draft Restore、清除 deferred marker 并完成收敛。
- 临时 PostgreSQL 实例在验证后停止，数据目录移入废纸篓，可恢复；远程目标数据库未被访问。

## 自动化门禁

```text
go test ./internal/lightweightapi ./pkg/lightweightdb -count=1
go test ./internal/daemon ./internal/daemonws -count=1
MULTICA_UNIT_TEST_ONLY=true go test ./cmd/server -run TestLightweight -count=1

Result: PASS
```

完整 Daemon 包首次回归发现旧测试仍要求 `branch_name`；该测试已按冻结 Lightweight DTO 修正为字段必须不存在，随后 `./internal/daemon` 与 `./internal/daemonws` 全量通过。

## 尚未宣称完成

- Complete/Fail/Cancel/Session/Usage/Message 的轻量终态链路已经闭合，因此 8.11 完成；Direct Chat Core、queued/running cancel、deferred cancel sweeper 与 Draft Restore 对外 CRUD 的完整证据见 `direct-chat-core.md`，因此 9.1–9.8 完成。
- Deferred promotion、有限 retry、batch claim、enqueue 与 Comment reconciliation 现已全部切到轻量 Task Core；fresh PostgreSQL live flow 覆盖 promotion、orphan→retry→claim→complete 和 Comment delivery fence，因此 8.10 已完成。Mention/re-entry 的新增证据见 `../phase-3/mention-reentry.md`。
- 本阶段覆盖 Task lifecycle 子链路，不等同于 TQ-01..11、CH-01..11 或发布验收全部通过。
