# Phase 2 前置：Lightweight Baseline Schema 与数据访问层

记录时间：2026-08-05（Asia/Shanghai）

## 验收边界

- 验证对象：`server/lightweight/migrations`、`server/lightweight/queries`、`server/pkg/lightweightdb` 与 Agent Secret codec。
- 数据库：本机 PostgreSQL 16.13 临时实例，trust 认证，临时 Unix socket，目标库名固定为 `multica_lightweight`。
- 隔离声明：本轮未使用用户提供的远程数据库凭据，也未连接或写入旧 `multica` 数据库。
- 运行时切换、真实业务闭环与旧库 checksum 复核仍属于第 12、14 组，不由本证据替代。

## 迁移与静态契约

- 迁移总数：77（4 个建表 migration、72 个单语句并发索引 migration、1 个后置唯一约束绑定 migration）。
- 表：严格匹配冻结 allowlist 的 26 张业务表；`schema_migrations` 只属于迁移工具记账。
- 索引：72 个业务索引逐项匹配冻结契约的表、键顺序、唯一性、predicate 和 `NULLS NOT DISTINCT`。
- 约束：冻结的 14 条 row check 均在指定表中；42 个非 partial unique index 通过后置 `USING INDEX` 绑定。
- 外键与级联：0 个；table DDL 不声明 `PRIMARY KEY` 或 `UNIQUE`，不隐式创建索引。

静态门禁：

```text
go test ./internal/lightweightmigrations -count=1
ok github.com/multica-ai/multica/server/internal/lightweightmigrations
```

## Fresh Install 与数据库审计

在空的 `multica_lightweight` 数据库执行：

```text
MULTICA_EDITION=lightweight EXPECTED_DATABASE_NAME=multica_lightweight go run ./cmd/migrate up
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> go test ./internal/lightweightmigrations -run TestLightweightFreshDatabaseAudit -count=1 -v
```

结果：

```text
77 migrations applied
application tables: 26
application indexes: 72
foreign keys: 0
schema marker: multica_lightweight / version 1
TestLightweightFreshDatabaseAudit: PASS
```

Fresh Database audit 同时逐列检查 `schema-columns.tsv` 的类型、可空性和默认值，并执行 Agent Secret 的真实事务内持久化探针：

- `custom_env` 的每个 value 独立保存为 `{v,kid,nonce,ciphertext}`。
- `mcp_config` 完整 JSON 文档保存为单个同形 envelope。
- JSONB 持久化值不包含测试明文；用对应 key ID 可完成解密回读。

## Database identity fail-closed

对同一临时实例中新建的空数据库 `multica`，以期望库名 `multica_lightweight` 执行迁移：

```text
exit: 1
error class: database_identity_mismatch
public tables after rejection: 0
```

该结果证明库名 guard 在 Goose 创建 `schema_migrations` 或执行其他 DDL 前失败。单元测试另覆盖非空库缺 marker、错误 edition 和不支持 schema version 的拒绝路径。

## sqlc 与加密组件

- Lightweight query 集：5 个 SQL 文件、141 个命名 query，只引用 26 表。
- 覆盖：稳定 cursor、claim `FOR UPDATE SKIP LOCKED`、prepare lease、deferred promotion/reclaim、三类 idempotency lookup、历史 tombstone display 和显式 Workspace cleanup。
- 生成产物：`server/pkg/lightweightdb`，26 个 model，是后续 Lightweight Core 的唯一目标数据访问层。
- append-only Comment 不暴露 update/delete query。

执行结果：

```text
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate --file sqlc.yaml
go test ./lightweight ./internal/lightweightmigrations ./internal/agentconfigsecret ./internal/util/secretbox ./pkg/lightweightdb -count=1
PASS
```

Agent Secret codec 的单元测试覆盖 round-trip、无明文、真实字节篡改、未知 key ID、未知 envelope version 和 redacted read。
