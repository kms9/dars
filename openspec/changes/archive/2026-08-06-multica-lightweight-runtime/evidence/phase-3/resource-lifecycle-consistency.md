# Phase 3：资源生命周期与跨表一致性（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## Active / Idle 资源语义

| 资源 | Active / 引用冲突 | Idle / 允许路径 |
| --- | --- | --- |
| Workspace | Active Task 或 online Runtime 返回 409；仅 Owner 可删 | 按依赖顺序显式清理 Workspace 业务数据，保留 User/PAT/其他 Workspace |
| Runtime | Active Task、Profile 派生实例或未确认的未归档 Agent 返回 409 | 确认集合一致后清空全部 Agent 绑定并删除；仅剩归档 Agent 时普通删除也清空绑定 |
| Agent | 任一 active Task 返回 409 | 归档；Restore 前重新锁定并验证 Runtime 仍存在 |
| Skill | 任一 Agent binding 返回 409 | 同一事务删除 Skill Files 后删除 Skill |
| Chat | Active Task 时不能 archive/delete；未 archive 不能 delete | archive 后显式删除 token/message/usage/draft/task/session |
| Squad | Active Task 返回 409；归档后禁止新成员和新 Issue 分配 | 无 Active Task 时写 archive tombstone，不提供 Restore |
| Issue | Active Task 返回 409 | 显式删除 token/message/usage/comment/activity/terminal task/Issue |

## 无 FK 的显式锁与事务

- Runtime mutation 使用 `Runtime → Agent rows` 锁序，并在锁内重新枚举未归档 Agent 与确认集合；所有 Agent binding（含 archived）在 Runtime delete 前显式清空。
- Agent 更新/归档/恢复现在在事务内锁 Agent；切换或恢复 Runtime 使用 `Runtime → Agent` 锁序。归档在同一锁内重新检查 Active Task。
- Issue/Chat/Squad mutation 在创建 Task 或删除/归档前锁对应 Issue/Session/Squad；Comment 与 Complete 共享 Issue fence。
- Skill Delete、Skill File replace/delete 与 Agent-Skill 完整集合绑定共享 Skill 行锁；Agent-Skill 集合先锁 Agent，再以排序后的 Skill ID 锁定所有目标，避免并发删除留下无主 binding。
- 所有跨表 lookup 都带 Workspace scope；Task Token、Mention、Invocation target、Runtime/Agent、Squad/Leader 与 Task root 在 Service 层重新校验，不依赖 FK。

## Fresh PostgreSQL 并发与租户验证

在全新 PostgreSQL 16.13 空库应用全部 77 个 migrations 后，真实 Live Flow 通过：

- 持有 Runtime 行锁期间并发创建 Agent 必须阻塞；Runtime 删除提交后创建返回 `agent_runtime_required` 且无 Agent 残行；
- 持有 Skill 行锁期间并发 Agent-Skill bind 必须阻塞；Skill 删除提交后 bind 返回 404 且无 `agent_skill` 孤儿；
- 持有 Agent 行锁期间并发创建 `todo` Run 必须阻塞；Agent 归档提交后创建返回 `agent_archived`，Issue 与 Task 均未部分写入；
- Comment/Complete 竞态、同 key Issue/Comment/Chat Create 竞态、双 Daemon Claim 与 Runtime/Agent 确认集合竞态继续通过；
- 跨 Workspace Runtime、Agent、Squad、Mention、Task Token、Daemon 和 GC 请求均 fail closed；失败路径的目标 Workspace 行数不变；
- Workspace、Runtime、Agent、Skill、Chat、Squad、Issue 的 Active/Idle 正反例均在同一 Live Flow 验证。

```text
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' -count=1 -v

Result: PASS
```

远程 PostgreSQL 未创建数据库、未迁移、未写入。
