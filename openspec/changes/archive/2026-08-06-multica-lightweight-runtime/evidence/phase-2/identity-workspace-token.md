# Phase 2：Identity、Workspace 与 Token 安全（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 实现边界

- 新增 `server/internal/lightweightapi`，Identity、Workspace、PAT 与 Daemon 配对直接依赖 `server/pkg/lightweightdb`，不再通过旧 schema model。
- Target Router 已切换 Email Code、Me、Workspace、Member、PAT、Daemon Register/Deregister/Heartbeat/Workspace/Repos 路由和 Human/Workspace/Daemon/Task middleware。
- 未完成的资源 Owner、Task Token 具体对象动作和完整 114 Handler 正反矩阵继续留在第 7、8 组及 5.7、5.12、5.13、5.15、5.17，不在本证据中误报。

## 真实数据库流程

环境：全新本地 PostgreSQL 16.13 临时实例与 Fresh Lightweight migration；未连接远程数据库或旧 `multica`。

```text
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run TestLightweightIdentityWorkspaceTokenLiveFlow -count=1 -v

TestLightweightIdentityWorkspaceTokenLiveFlow: PASS
```

该流程实际验证：

- Email Code 只保存 keyed hash，10 分钟过期，错误 5 次锁定，成功 single-use，重放失败。
- `send-code` 对不允许注册的未知邮箱仍返回 202 且不创建状态；production 缺 Mail Provider 时 mutation fail closed。
- 首次 Verify 只创建 User；Workspace 数仍为 0。
- `POST /api/workspaces` 在事务内创建 Workspace、Owner Member，`issue_counter=0`。
- Workspace 请求缺 `X-Workspace-ID`、只给 slug、path/header 不一致分别失败；Member List 只读成功。
- PAT Create/Renew 明文一次可见，只存 hash/prefix；Renew 轮换后旧 Token 立即 401；Revoke 后新 Token 立即 401。目标路径不使用 Token cache，因此不存在撤销后的缓存窗口。
- Register 只接受 Human JWT/PAT 与 `lightweight-runtime-v1`；签发 hash-only Daemon Token。
- Re-pair 保留一条最新 Token，旧 Token 立即 401；Deregister 将 Runtime offline 并撤销 Token。
- Daemon Token 不能访问 Human API，Human Token 不能访问后续 Daemon lifecycle；Daemon Workspace 枚举恰好返回绑定 Workspace。第二个 Daemon 对不属于其 Runtime 的 Task status/GC 请求返回 404。
- Daemon Token 到期后 heartbeat 返回 401，且不回退到 Human PAT；Workspace Delete 的显式清理事务把包括已过期记录在内的 Daemon/Task Token 行清零。
- Task Token 从服务端记录恢复 `task/workspace/agent/user`，同时校验绑定任务与活动状态；终态任务 Token 返回 401。
- Workspace Delete 遇在线 Runtime 返回 409；允许删除后显式清理 Workspace 数据与 Daemon/Task Token，同时保留 User 与 PAT 记录。

## Cookie 与静态门禁

- production Cookie 强制 `Secure + HttpOnly + SameSite=Lax`；Cookie mutation 继续校验绑定 auth token 的 CSRF header。
- Router contract 明确将 Register 归入 Human pairing，其余 `/api/daemon/**` 归入 Daemon Token boundary。

```text
go test ./internal/lightweightapi ./internal/auth -count=1
MULTICA_UNIT_TEST_ONLY=true go test ./cmd/server -run TestLightweight -count=1
PASS
```
