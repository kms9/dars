# Phase 5：物理删除与依赖清理证据

验证日期：2026-08-06（Asia/Shanghai）。

## 结论

- Lightweight Server composition、114-route Router、CLI、Daemon、Web、Core、Views 与 26-table sqlc 层均已切换为目标实现。
- 退出能力的 Go Handler/Service/queries/generated models、旧 293 migration 启动链、Web/Core/Views 页面与 API、CLI 命令、i18n/assets 和目标外测试已从候选源码物理删除。
- `apps/desktop/**` 没有候选修改；Desktop 仍明确处于当前范围外。
- 空目录可能因当前工作树删除尚未提交或包管理器目录仍在磁盘上存在；验收以源码文件、import、build graph 与 Git 删除状态为准，而不是目录 inode 是否存在。

## 机器门禁

最终 Fresh Checkout 候选执行：

```text
Target TypeScript typecheck: 4/4 packages pass
Target Web production build: pass, only Lightweight routes emitted
Core unit/contract: 38 pass
Views unit/contract: 23 pass
Web unit/contract: 58 pass
Go packages: all pass on fresh Lightweight database
Chromium E2E: 1 pass
Router manifest lines: 114
Application tables: 26
Foreign keys: 0
Tracked Desktop changes: 0
```

`TestLightweightRouterMatchesFrozenManifest`、`TestLightweightMainWorkerAllowlist`、`TestTargetQueriesUseOnlyLightweightSchema`、`TestGeneratedModelsMatchTwentySixTables`、Web route/build-graph contract、Core API path snapshot 与 CLI command contract 同时通过。

生产目标树的负向 import 扫描未发现对以下退出包的引用：

```text
server/internal/{cloudruntime,handler,integrations,scheduler,storage}
@multica/core/{autopilots,billing,inbox,projects,slack,vcs}
```

这些目录中的候选源码文件数均为 `0`；删除后的构建不再依赖它们。

## 删除后真实复验

- 删除后使用 Docker PostgreSQL 17 重跑 `make check`，并完成真实双 Daemon Direct Chat、FLOW-01 与 FLOW-02。
- Router 精确集合、Worker allowlist、26 表、removed HTTP route 与 removed Web deep link 负向矩阵全部通过。
- 新数据库与本轮所有应用进程的 DSN 均固定为 loopback `multica_lightweight` 或一次性 `multica_lightweight_check_*`。

## 未完成外部门槛

旧数据库端点当前 TCP 可达，但本机没有 `~/.pgpass`、pg service 或受支持的旧库凭据环境变量；无密码连接按预期被 PostgreSQL 拒绝。因此本阶段不能执行任务 13.8 要求的最终 revision/schema/data checksum 再比较。

Phase 3 最后一次只读比较曾与基线完全一致；此后所有候选进程都使用本地独立 DSN，未向旧端点发起 migrate/runtime 写入。但这不能替代 13.8 明确要求的最终外部状态复读，因此该项保持未完成，DB-07 在发布索引中标记为 `blocked`。

