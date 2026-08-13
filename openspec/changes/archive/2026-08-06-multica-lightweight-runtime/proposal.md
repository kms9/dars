## Why

当前仓库仍承载完整项目管理产品的页面、API、后台任务和 293 个历史数据库迁移，无法以可验证的边界交付只面向本地智能体与小队协作的轻量运行时。现在需要把已在 `docs/prd/multica_lightweight_runtime_docs/` 冻结的产品、接口、数据和验收契约转成可执行的 OpenSpec 变更，以便在不维护 Full/Light 双模式的前提下直接替换当前复制版本。

## What Changes

- **BREAKING** 将仓库收缩为单一轻量产品，只交付 Web、Server 与 Local Daemon/CLI；Desktop 专属代码本阶段不修改，但从目标构建、CI 和发布验收中排除，Mobile 与 Docs 也不进入目标产物。
- **BREAKING** 移除 Cloud Runtime、附件、Human Squad Member、Workspace Invitation、Autopilot、Inbox、Channel、Agent Builder/Template、Runtime Self-update 及完整项目管理外围能力，不提供旧 API 或旧 Desktop 兼容层。
- 保留 Workspace 隔离、只读成员、Runtime Profile、Skill、Agent、Direct Chat、Agent-only Squad、Lightweight Issue、Flat Comments 与 Durable Task Queue，并以 Issue 作为 Squad Run 的持久化根对象。
- 固化 Agent/Squad 协作状态与可靠性协议，包括 `blocked`、Leader 委派、Member 结果回写、Leader 重入、`deferred`、`waiting_local_directory`、prepare lease、comment reconciliation 与取消收敛。
- **BREAKING** 以冻结的 114 条 method+path manifest 替换当前 API；同步切换 Web、Server、Daemon/CLI，并明确 Email Code、PAT、Daemon Token、Task Token、Web Realtime 和 Daemon Control WS 的权限边界。
- **BREAKING** 使用新数据库 `multica_lightweight` 与独立 26 表 baseline，不回放原产品 293 个 up migrations，不迁移、覆盖或复用原 `multica` 数据库。
- 对 Agent `custom_env` 和完整 `mcp_config` 实施版本化密文 envelope，并把生产密钥、数据库身份和 edition/version 校验纳入 readiness 与迁移保护。
- 将后台进程限制为 task/runtime sweeper、heartbeat scheduler、in-memory realtime hub，以及显式启用的 Redis relay 和 metrics listener。
- 以真实 PostgreSQL、独立 Daemon/Runtime、Web、Fresh Install、Backup/Restore、路由/Worker/表白名单和二进制、JS、RSS、启动耗时阈值作为发布硬门禁。

## Capabilities

### New Capabilities

- `lightweight-product-boundary`: 定义单一轻量产品的交付面、保留与移除功能、目标构建图及不兼容边界。
- `lightweight-collaboration-runtime`: 定义 Agent、Direct Chat、Agent-only Squad、Issue/Comment Run 与可靠任务队列的业务行为和状态机。
- `lightweight-api-security`: 定义 114 条目标 API manifest、Email Code/PAT/Daemon/Task Token 权限以及 Web 与 Daemon 实时协议。
- `lightweight-data-baseline`: 定义 `multica_lightweight` 的 26 表新库 baseline、数据库身份保护、迁移规则、备份恢复与敏感配置加密。
- `lightweight-runtime-release`: 定义允许的后台进程、启动与故障恢复行为、端到端证据以及规模和性能发布门禁。

### Modified Capabilities

无。`openspec/specs/` 当前没有既有 capability spec；本变更从冻结的 P0 文档建立首批规范。

## Impact

- 后端：`server/` 的 Router、鉴权、Handler、Service、Worker、SQL/sqlc、迁移与启动/readiness 逻辑将发生 breaking 收缩。
- 前端与共享包：`apps/web/`、`packages/core/`、`packages/views/` 和 `packages/ui/` 将移除完整项目管理表面并对齐新 API；继续遵守现有包边界和 React Query/Zustand 所有权规则。
- 本地执行端：Daemon/CLI 的配对、Token、批量 claim、heartbeat、Task Token 与控制消息协议将同步升级到 `lightweight-runtime-v1`。
- 构建发布：根 pnpm/Turbo、Makefile、CI、镜像/安装流程和验收脚本将只覆盖目标交付图；Desktop 源码留存但不作为本变更验收对象。
- 数据：新增独立 PostgreSQL baseline 和数据库保护；旧 `multica` 仅允许作为只读归档/回退数据源保留。
- 需求来源：`docs/prd/multica_lightweight_runtime_docs/00_multica_lightweight_runtime_prd.md` 至 `04_e2e_acceptance_matrix.md` 是本变更的冻结输入，冲突时以相应分册中的显式目标 manifest、表清单和验收阈值为准。
