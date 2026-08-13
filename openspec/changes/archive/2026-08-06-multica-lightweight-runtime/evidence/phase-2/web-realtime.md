# Phase 2：Web Realtime（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 目标事件边界

- Shipping composition 使用独立 `registerLightweightListeners`，只转发冻结的 31 个 Workspace 事件；Inbox、Invitation、Member、Project、Autopilot、旧集成事件及无 Workspace 事件不再进入目标 Web `/ws`。
- 每个目标事件固定包含 `type/event_id/workspace_id/occurred_at/actor_type/actor_id/payload`；`event_id` 为 UUID，空 Actor ID 编码为 `null`。
- Web 客户端保留最多 2048 个近期 `event_id`，同一 at-least-once delivery 只进入具体事件 handler 与 `onAny` handler 一次。
- 本地 Hub 与 Redis relay 共用同一 envelope；relay 遇到已有 `event_id` 时保留原值，不生成第二个客户端身份。

## Upgrade 与恢复契约

- Target `/ws` 只接受 `workspace_id`，不解析 `workspace_slug`。
- JWT/Cookie 与 `mul_` PAT 在 upgrade 前解析为 User，并通过 Lightweight `member` 表验证 Workspace Membership；Daemon/Task Token 不会经 PAT resolver 穿透。
- Web 客户端 upgrade query 已改为 Workspace UUID。断线重连不请求历史 replay；认证恢复后沿用 React Query invalidation/refetch，覆盖 Workspace、Runtime、Agent、Skill、Squad、Issue/Comment/Task 与 Chat 数据。

## 自动化门禁

```text
go test ./internal/lightweightapi ./cmd/server \
  -run 'TestLightweight|TestLightweightListeners' -count=1
go test ./internal/lightweightapi ./internal/realtime -count=1
pnpm --filter @multica/core test -- ws-client.test.ts use-realtime-sync-ws-instance.test.tsx
pnpm --filter @multica/core typecheck
pnpm --filter @multica/core lint

Result: PASS
```

Core Vitest 命令实际运行并通过 114 个 test files、1235 个 tests。

## PostgreSQL 16 真实 WS 验证

在全新隔离的本地 PostgreSQL 16 `multica_lightweight` 临时数据库完成全部 migration 后，运行：

```text
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run TestLightweightIdentityWorkspaceTokenLiveFlow -count=1 -v

Result: PASS
```

该流程实际验证 Owner Membership resolver、跨 Workspace resolver 拒绝、JWT Cookie 成员 WS upgrade 成功、跨 Workspace upgrade 返回 403、legacy `workspace_slug` 返回 400，以及 PAT resolver 在 rotate 前成功、rotate 后拒绝旧凭据。

隔离实例在验证后已停止并移入废纸篓；未连接或修改远程数据库，也未连接旧 `multica`。
