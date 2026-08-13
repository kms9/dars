# Phase 3 database runtime cutover evidence

验证窗口：2026-08-05 17:12–17:23 Asia/Shanghai。

## 结论

- Lightweight 运行时、migrate、Make/Compose/CI/worktree/check/Helm 配置已统一到独立数据库名 `multica_lightweight`，并在 DDL、Worker、监听和业务写入前执行 PostgreSQL 版本、expected name、edition 与 schema version guard。
- 用户提供的连接池意图映射为 `DATABASE_MAX_CONNS=25`、`DATABASE_MAX_IDLE_CONNS=5`、`DATABASE_MAX_CONN_LIFETIME=300s`。项目使用 `pgxpool`，其没有原生的最大空闲连接数配置；实现通过并发安全的 acquire/release/close 连接集合强制 5 个空闲连接硬上限。保留 `DATABASE_MIN_CONNS=5` 作为预热下限。
- 远端 `10.8.8.120:5412` 当前可连接和认证，但服务端版本为 PostgreSQL 12.22。目标 baseline 的 `lw_133_unique_agent_runtime_identity` 使用 PostgreSQL 15 引入的 `NULLS NOT DISTINCT`，因此当前远端服务不兼容。必须先升级为 PostgreSQL 15+（推荐项目默认的 17）或提供新的 15+ 实例，才能创建并迁移 `multica_lightweight`。
- 远端没有创建 `multica_lightweight`，没有执行 migration/DDL/DML。原 `multica` 继续只读保留。
- `sslmode=disable` 可连接，但数据库链路未使用 PostgreSQL TLS；跨不可信网络部署前应提供证书并改为 `verify-full`（至少 `require`）。

项目运行配置等价形式（密码由 Secret Store 注入，不进入仓库）：

```text
DATABASE_URL=postgres://postgres:<secret>@10.8.8.120:5412/multica_lightweight?sslmode=disable
EXPECTED_DATABASE_NAME=multica_lightweight
MULTICA_EDITION=lightweight
DATABASE_MAX_CONNS=25
DATABASE_MIN_CONNS=5
DATABASE_MAX_IDLE_CONNS=5
DATABASE_MAX_CONN_LIFETIME=300s
MULTICA_AGENT_SECRET_KEY=<external-base64-32-byte-key>
```

## 本地真实 PostgreSQL 门禁

环境：一次性本地 PostgreSQL 16.13，loopback-only、trust 认证；验证后进程停止且数据目录移入废纸篓。两次从空库执行目标 migrate，均使用 77 个 Lightweight migration。

执行并通过：

```text
go test ./internal/lightweightmigrations -run '^TestLightweightFreshDatabaseAudit$' -count=1
go test ./internal/lightweightapi -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' -count=1
LIGHTWEIGHT_SECRET_PROBE_MODE=seed   go test ./internal/agentconfigsecret -run '^TestBackupRestoreSecretProbe$' -count=1
pg_dump -Fc multica_lightweight
pg_restore --no-owner --no-privileges ... multica_lightweight_restore
LIGHTWEIGHT_SECRET_PROBE_MODE=verify go test ./internal/agentconfigsecret -run '^TestBackupRestoreSecretProbe$' -count=1
```

结果：

```text
postgres_version=16.13
migration_count=77
application_table_count=26
total_index_count=72
foreign_key_count=0
fresh_catalog_audit=pass
constraint_rejections=pass
concurrent_claim=pass
workspace_delete=pass
cross_table_flow=pass
backup_restore=pass
secret_decrypt=pass
normalized_schema_match=pass
restored_server_ready=ok:ok:ok:ok
restored_email_auth=pass
restored_workspace_create=pass
```

Backup/Restore 使用随机外部 Agent Secret key：备份前写入 `{v,kid,nonce,ciphertext}`，数据库内容不含探针明文；恢复库用同一外部 key 重新注入后成功解密。key 未写入数据库、dump、日志或证据文件。

负向门禁也在真实 PostgreSQL 上通过：

```text
empty_guard=blocked empty_tables=0
wrong_name_guard=blocked
make db-reset POSTGRES_DB=multica -> rejected before database command
allowed-name DATABASE_URL/POSTGRES_DB mismatch -> rejected before database command
PostgreSQL 12 unit preflight -> rejected before DDL
```

## 原数据库隔离复核

远端复核使用 `default_transaction_read_only=on` 与只读事务，凭据仅经交互式 stdin 注入，未写入命令、环境文件或证据：

```text
server_version=12.22
system_identifier=6873377209962900392
schema_migrations=106
latest_revision=084_squad
public_tables=47
normalized_schema_dump_sha256=unchanged
normalized_data_dump_sha256=unchanged
nonempty_counts=schema_migrations:106,task_usage_dashboard_rollup_state:1,task_usage_rollup_state:1
remote_multica_lightweight_exists=false
```

复核 hash 与 Phase 0 `old-database-baseline.json` 完全一致。因此本阶段验证没有写入原库。

## 未完成门禁

- 本机没有 Helm CLI，`scripts/helm-config.test.sh` 未运行；Chart 的静态配置断言已更新，CI 会安装 Helm 后执行。
- DB-01 的最终真实 Runtime/Daemon/Web P0、DB-06 的完整 attribution 证据，以及 DB-01..10/SEC-12 的最终发布索引仍留在 12.8、14.4–14.10；本文件不能替代这些发布门禁。
- 当前远端 PostgreSQL 12.22 不得用于目标建库。远端创建动作只有在提供 PostgreSQL 15+ 端点并再次确认授权后才可执行。
