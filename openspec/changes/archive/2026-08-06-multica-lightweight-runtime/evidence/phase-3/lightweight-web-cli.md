# Phase 3 Lightweight Web and CLI evidence

> 2026-08-06 终态更新：本文件的未完成边界已由 Phase 5/6 解除；见 `../phase-5/physical-delete.md` 与 `../phase-6/final-release.md`。

验证窗口：2026-08-05 18:41–18:49 Asia/Shanghai。

## 结论

- Web production route tree 只包含 Login、Workspace create、Run、Chat、Agent、Squad、Skill、Runtime 与 Settings；Projects、Autopilots、Inbox、Billing、Usage 等深链返回 404。
- Lightweight API client 使用 `X-Workspace-ID`、冻结 DTO、`lightweight-runtime-v1`，并能解析 `{error:{code,message,details}}` 错误包。
- Run、Comment 与 Direct Chat 的浏览器 mutation 使用 `Idempotency-Key`；Server CORS allowlist 已包含该请求头，真实浏览器不再在 preflight 后阻断。
- Web 不再挂载 Client Usage reporter，因此不会调用已退出的 `/api/client-usage`。
- CLI 根命令仅暴露目标命令树；Email Code 与 PAT 是唯一 Login 路径，Daemon/Runtime、Agent、Chat、Squad、Run/Comment/Task、Workspace、Runtime Profile 与 Skill 命令已对齐目标路径。
- 内置 Skill 固定为 `multica-runtime-protocol`、`multica-working-on-runs`、`multica-squad-collaboration`、`multica-direct-chat`、`multica-lightweight-resources` 五项，并区分 Human Token、Daemon Token 与 Task Token 权限。

## 自动化验证

执行并通过：

```text
pnpm --filter @multica/core test
  Test Files 116 passed
  Tests 1244 passed

pnpm --filter @multica/core typecheck
  pass

pnpm --filter @multica/web test
  Test Files 22 passed
  Tests 159 passed

pnpm --filter @multica/web typecheck
  pass

pnpm --filter @multica/web build
  compiled successfully
  production route tree contains 17 product/auth routes and no exited routes

go test ./cmd/server -run 'CORSAllowedHeaders|Lightweight'
  pass

go test ./cmd/multica -run Lightweight
go test ./internal/daemon/execenv -run Lightweight
go test ./internal/daemon -run Lightweight
go test ./internal/service -run Lightweight
  pass
```

## 真实浏览器闭环

环境：Chromium headless、Web `localhost:23000`、目标 Server `localhost:28080`、一次性 Docker PostgreSQL 16、独立 `multica_lightweight` 数据库。测试凭据、Agent Secret 与固定验证码仅用于该隔离环境。

命令与结果：

```text
PLAYWRIGHT_BASE_URL=http://localhost:23000 \
MULTICA_DEV_VERIFICATION_CODE=<test-code> \
pnpm exec playwright test e2e/lightweight.spec.ts --project=chromium

1 passed (12.8s)
```

同一浏览器上下文完成：

1. Email Code 登录并创建 Workspace；
2. 验证 Sidebar 只含目标入口；
3. Daemon register、Agent create、Squad create；
4. 创建 Run、追加 Flat Comment、更新为 blocked，并展示独立 Task Runs 分区；
5. 创建 Direct Chat、发送文本、观察 Task queued 并调用统一 Cancel；
6. 查看 Runtime 与 Settings/Member；
7. 建立真实 WebSocket Workspace 连接；
8. 验证 Projects、Autopilots、Inbox、Billing、Usage 深链全部返回 404。

首次运行暴露并修复两个只会在真实浏览器出现的偏差：CORS 未放行 `Idempotency-Key`，以及 Workspace create 页面在 HttpOnly Cookie 登录后的 auth hydration race。修复后同一流程通过。

## 未完成边界

- Server/Daemon/Web/CLI 的退出源码、旧 DTO、旧测试与依赖仍待 Phase 5 物理删除，因此 6.11、11.4 和 13.x 尚不能视为完成。
- 本文件证明目标 Web/CLI 产品表面与浏览器闭环，不替代真实安装 Runtime 的 14.5–14.7 发布门禁。
