# Phase 3：历史 Tombstone 与 Runtime 解绑生命周期（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 已实现语义

- TaskRun 的 Agent、Runtime、Squad display 通过单次 `LEFT JOIN` 展开；关联行仍存在（含归档）时保留历史名称，真正删除时使用稳定 `[deleted ...]` tombstone，不逐行补查。
- Runtime 的普通删除在存在未归档 Agent 时返回 409，并同时返回最新 `active_agent_ids` 供客户端确认。
- `unbind-agents-and-delete` 严格解析、标准化并去重 `expected_active_agent_ids`；集合与事务锁内实际未归档 Agent 不一致时返回最新集合。
- Runtime 删除前显式清空所有 Agent 的 `runtime_id`，包括确认集合中刻意不出现的已归档 Agent。这样无 FK baseline 下也不会让 Agent Restore 复活悬空 Runtime 引用。
- 历史 Task 自身的 `runtime_id` 保留用于审计；Runtime 删除后 TaskRun 仍能返回原 Task/Agent ID 和 `[deleted runtime]` display。

## Fresh PostgreSQL 真实流程

在全新 PostgreSQL 16.13 空库应用 77 个 migrations 后，`TestLightweightIdentityWorkspaceTokenLiveFlow` 验证：

- 有未归档 Agent 的 Runtime 删除返回 409、当前 Agent ID 与 `active_agent_ids`；错误确认集合返回同一最新集合；非法 UUID 返回 400；
- 正确确认集合在一个事务内解绑 Agent 并删除 Runtime；
- 专用 Agent 先产生终态历史 Task，再归档 Agent；仅剩归档绑定时普通 Runtime 删除成功并显式清空该绑定；
- 该历史 Issue 的 TaskRun 继续显示归档 Agent 原名与 `[deleted runtime]`，不会因关联 Runtime 删除而无法渲染；
- 尝试 Restore 已失去 Runtime 的 Agent 返回 `409 agent_runtime_required`。

```text
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' -count=1 -v

Result: PASS
```

远程 PostgreSQL 未创建数据库、未迁移、未写入。
