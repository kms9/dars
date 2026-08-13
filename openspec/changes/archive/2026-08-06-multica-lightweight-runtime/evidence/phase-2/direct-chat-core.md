# Phase 2：Direct Chat Core（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 实现边界

- Target composition 的 10 条 Chat Session/Message/Pending/Draft Restore 路由、用户 Task Cancel 与 Daemon Cancel Ack 均直接绑定 `server/internal/lightweightapi`，不再进入旧聚合 Handler。
- Chat Session 支持 creator-scoped create/list/get/patch/delete；归档前检查 Active Task，删除只允许 archived + idle，并在事务内显式清理 Message、Draft Restore、终态 Task、Task Token、Task Message 与 Task Usage。
- Send 只接受纯文本 `content` 和必填 `Idempotency-Key`。同 key/同 body 返回原 Message/Task，同 key/不同 body 返回 `409 idempotency_key_reused`；未知 Attachment 字段返回 400 且零写入。
- 每次成功 Send 在一个事务中创建一条 User Message 和一个 queued Chat Task；Task 只有 `chat_session_id`，`issue_id` 为空。Claim 使用该 Task 绑定的输入 Message，并返回 thread、prior Session/WorkDir 和纯文本输入。
- Complete 生成 Assistant Message，空输出生成 `no_response`；Fail 生成带 `failure_reason` 的可见 Assistant Message。终态重放不会重复投影。
- queued cancel 立即生成 Draft Restore 并完成收敛；dispatched/waiting/running cancel 设置 deferred marker，Daemon ack 或 sweeper 负责最终收敛。重复 cancel、ack 与 Draft consume 均幂等。
- Pending Task 返回当前未终态 Task 的最小字段；终态后返回 `{}`。第二轮 Claim 只复用服务端持久化的安全 Session/WorkDir。
- Agent Runtime 在已有 Session 后被解绑时，Send 返回 `409 agent_runtime_required`，不会创建 Message 或 Task；归档 Session 拒绝继续 Send。
- `humanCanInvokeAgent` 对 workspace invocation target 同时校验 `target_id == agent.workspace_id`，避免异常数据把 Workspace 授权扩大为任意 workspace target。

## Fresh PostgreSQL 16 验证

环境：全新隔离的本机 PostgreSQL 16.13 临时实例，数据库名 `multica_lightweight`，从空库执行完整 Lightweight migration。验证后实例已停止，数据目录移入废纸篓。未连接或修改远程 PostgreSQL，也未连接旧 `multica`。

```text
DATABASE_URL=<temporary-local-dsn> \
  MULTICA_EDITION=lightweight \
  EXPECTED_DATABASE_NAME=multica_lightweight \
  go run ./cmd/migrate up

LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' \
  -count=1 -v

Result:
77 migrations applied
TestLightweightIdentityWorkspaceTokenLiveFlow: PASS (0.32s)
```

同一真实流程覆盖：

- Session create/list/get、creator privacy、title/status patch、Active Task archive guard、archived send rejection 与显式删除清理。
- Idempotency-Key 缺失、同 body 重放、不同 body 冲突，以及 removed Attachment 字段 400/零写入。
- Runtime missing 409/零写入；正常 Send 只生成唯一 Message/Chat Task，Claim 明确显示 `issue_id` 为空。
- Task Token 只能读取自身 Session 历史，伪造 `X-Workspace-ID` 不改变服务端绑定范围。
- task message stream、usage、Complete Assistant Message、Fail Failure Message、空输出 no-response 与终态重放。
- 两轮 Chat 的 Session/WorkDir resume，以及 Message cursor 和 Pending Task 刷新恢复。
- API 创建的 queued Chat Task cancel、Draft Restore list/consume；running cancel、Daemon status/ack；超时 deferred cancel sweeper。

静态回归：

```text
go test ./internal/lightweightapi ./cmd/server ./pkg/lightweightdb \
  -run 'TestLightweight|TestGC|TestDaemon|TestTarget|TestBuild|TestRuntime|TestTransport|TestListener' \
  -count=1

Result: PASS
```

## 任务判定

- 9.1–9.8：上述代码、目标 composition、严格 DTO 与 Fresh PostgreSQL 流程逐项提供直接证据，可标记完成。
- 9.9：保持未完成。Server 侧已覆盖 CH-01、CH-02、CH-04..11 的对应协议行为，并产生 streaming event；但 CH-03 要求真实 Web UI 实时展示，尚未运行目标 Web/浏览器闭环，不能把 Handler 测试当作 CH-01..11 全部通过。
- 本证据不代表独立 Daemon/真实 Runtime Handler、浏览器、远端 PostgreSQL、Phase 4 P0 或发布门禁已经完成。
