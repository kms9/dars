# Phase 2：Daemon 协议、控制通道与 Claim（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 协议切换

- Daemon Client 的 Register、Heartbeat 与 WebSocket handshake 均显式发送 `lightweight-runtime-v1`；Server 对缺失或不匹配版本 fail closed。
- Register 始终使用保留的 Human JWT/PAT pairing credential。成功响应含一次可见 Daemon Token 时，后续 Heartbeat、WS、Claim 和 callback 自动使用 Daemon Token；re-pair 仍使用原 Human credential。
- `/api/daemon/tasks/claim` 是唯一 Claim HTTP 路径。目标 Router 已对 `/api/daemon/claim` 与单 Runtime Claim 返回 404；Daemon Client 已删除单 Runtime Claim 方法和 404 legacy fallback。
- Daemon Client 已删除 Runtime Update Result 与 Autopilot GC 请求路径；Heartbeat 不再执行旧 `pending_update`。旧 Autopilot GC metadata 只按本地 orphan TTL 保守处理，不再访问 Server。
- Workspace 同步只访问 daemon-scoped `/api/daemon/workspaces`；目标端点不存在时 fail closed，不回退 Human `/api/workspaces`。

## Claim 原子边界

- Claim 在事务内验证 Token 的 Workspace/daemon_id/Runtime 集合与 Runtime online 状态，并使用 `FOR UPDATE SKIP LOCKED` 原子将 queued Task 切为 dispatched。
- Claim query 同时约束 Task/Agent Workspace、Agent 当前 Runtime 与未归档状态，避免无 FK baseline 下跨租户或陈旧绑定被领取。
- 同一事务删除旧 Task Token、签发 hash-only Task Token，并组装 Workspace Context、Repos、Agent 配置、完整启用 Skill Bundle。
- `custom_env` 和完整 `mcp_config` 只在授权 Daemon Claim 响应内解密；数据库仍保存 envelope 密文。任何解密或上下文装配失败都会回滚 Task dispatch 与 Token 写入。

## WS control 与故障语义

- Target WS upgrade 由 Daemon Token、固定协议 header/query、绑定 Workspace/daemon_id 和 Runtime 集合共同授权。
- WS Heartbeat 使用轻量查询更新目标 Runtime；Runtime 消失时返回 `runtime_gone`，不把数据库错误误报为删除。
- WS RPC 只接受 `tasks.claim`，检查 request Runtime 是 connection Runtime 集合的子集；Hub 保留 request_id 关联、事件去重与并发上限。
- 已有 WS-first tests 验证 sent-frame 断线的不确定结果不会立即 HTTP 双重 Claim，安全窗口后执行一次 HTTP batch fallback；canonical batch 404 不再调用 legacy path。

## 真实 PostgreSQL 16 证据

在全新本地 `multica_lightweight` 临时数据库执行完整 migration 后运行：

```text
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run TestLightweightIdentityWorkspaceTokenLiveFlow -count=1 -v

Result: PASS
```

真实流程创建 queued Task 后调用 `/api/daemon/tasks/claim`，验证：

- 返回唯一 Task、绑定 Runtime/Workspace、`mat_` Task Token、Workspace Context 与解密后的 Agent Env/MCP Secret。
- Task Token 数据库列不是明文；Task Token 可访问自身 Task probe，Task 进入终态后立即 401。
- Claim 后 Agent archive 因活动 Task 返回冲突，Task 终态后 archive/restore 成功。

同一流程还验证四条 Lightweight GC 路由：

- 批量 Issue GC 在一次响应中区分 found/not-found，只返回 `id/found/status/updated_at`；跨 Workspace path 返回 404。
- 单 Issue 与 Chat Session 只返回 `status/updated_at`，不返回 title、description 或其他内容。
- Task GC 只返回 `status/completed_at`，且另一个同 Workspace Daemon Token 查询不属于其 Runtime 的 Task 时返回 404。
- 直接 Handler tests 覆盖 500 个 `issue_ids` 上限、64 KiB body 上限、unknown field、非法 UUID 与跨 Workspace 拒绝。

## 自动化门禁

```text
go test ./internal/daemon ./internal/daemonws ./internal/lightweightapi -count=1
MULTICA_UNIT_TEST_ONLY=true go test ./cmd/server -run TestLightweight -count=1
Result: PASS
```

本轮 `go test ./internal/daemon -count=1` 实际通过（约 25 秒）。隔离 PostgreSQL 验证后实例已停止并移入废纸篓；未连接远程数据库或旧 `multica`。

尚未宣称完成：全部 callback 的轻量 Task Core、跨 Server/Daemon/Web 的旧 alias 总扫描与 DR-01..11 总验收仍待后续任务。Web WS 的独立闭环证据见 `web-realtime.md`。
