# Old `multica` database baseline audit

Status: **COMPLETE — the original `multica` database was captured through a read-only connection.**

Task 1.8 is backed by the machine-readable record in [old-database-baseline.json](./old-database-baseline.json). The supplied password was used only in transient process memory and was not written to either evidence file, the repository, a command-line argument or a connection file.

## Captured database identity

- capture window: `2026-08-05T02:09:43.151026Z` to `2026-08-05T02:14:31Z`
- endpoint: `10.8.8.120:5412`
- database/user: `multica` / `postgres`
- SSL mode: `disable` (the connection was not protected by PostgreSQL TLS)
- PostgreSQL: `12.22` (`server_version_num=120022`)
- cluster system identifier: `6873377209962900392`
- database OID: `5132864`
- encoding/collation/ctype: `UTF8` / `zh_CN.UTF-8` / `zh_CN.UTF-8`
- database size at capture start: `10953583` bytes

Every SQL inspection ran with `default_transaction_read_only=on`; logical fingerprint queries additionally used `REPEATABLE READ READ ONLY`. No migration, DDL, DML, lock-taking maintenance command or restore was executed.

## Revision baseline

`public.schema_migrations` contains 106 rows:

- lexical version range: `001_init` through `084_squad`
- latest applied revision: `084_squad`
- latest applied at: `2026-06-15T06:20:49.515017Z`
- ordered `version + applied_at` manifest SHA-256: `d4737531c2fef014e958e5e24cfb719048402f5e8a87cd0820cd119d761960cf`

## Schema baseline

- 47 base tables
- 441 columns
- 141 indexes
- 47 primary keys
- 19 unique constraints
- 82 foreign keys
- 46 check constraints
- 8 triggers
- normalized schema-only dump SHA-256: `65a4a99585a781ce3eca69bd0fb4051a34b9b328e4e7e612331438cd9a322104`
- immediate repeat SHA-256: `65a4a99585a781ce3eca69bd0fb4051a34b9b328e4e7e612331438cd9a322104`

The schema dump used PostgreSQL client 18.3 with `--schema=public --schema-only --no-owner --no-privileges --no-comments --quote-all-identifiers`. PostgreSQL 18 random `\restrict` markers were removed before hashing so the checksum can be reproduced.

## Content baseline

- normalized data-only dump SHA-256: `40fbbf426607ac33a899b974fb4d1f0b4d78af1a5720c55f7a48ed0106f7b8c9`
- immediate repeat SHA-256: `40fbbf426607ac33a899b974fb4d1f0b4d78af1a5720c55f7a48ed0106f7b8c9`
- canonical logical content SHA-256: `8d728eaa430982b6609ddb392217e863b0e1553102a9b0f017f3985442f4b980`
- immediate repeat SHA-256: `8d728eaa430982b6609ddb392217e863b0e1553102a9b0f017f3985442f4b980`
- per-table count/digest manifest SHA-256: `5e7f5c1216117cb3501b049de5ed2a3a7fc7370e0573c63b88fb09db9894ac0b`

The canonical digest orders all non-system base tables by schema/name, emits a table sentinel, converts each row to canonical `jsonb` text and orders rows with `C` collation before SHA-256 hashing. Of 47 tables, 44 were empty. The three non-empty tables were:

- `public.schema_migrations`: 106 rows
- `public.task_usage_dashboard_rollup_state`: 1 row
- `public.task_usage_rollup_state`: 1 row

The data-only dump emitted circular foreign-key restore-order warnings for `issue`, `comment`, `agent_task_queue` and `autopilot_run`. Both dumps completed and produced identical hashes; the warning affects standalone restore ordering, not this read-only checksum.

## Pre-unblock discovery history

### Initial reachability checks

- `127.0.0.1:5432`: no response
- `127.0.0.1:5433`: no response
- Docker, Podman, Colima and Nerdctl CLIs: not installed
- Repository environment files: only examples/staging files; no local project `.env` or usable original-database DSN
- Indexed local backups matching `*multica*.dump`, `*multica*.backup` or `multica.sql*`: none found
- User-directory PostgreSQL cluster discovery found only unrelated PGlite stores.

### Second-pass runtime and configuration audit

A second read-only discovery pass expanded the search beyond commands visible on `PATH`:

- No Docker Desktop, OrbStack, Rancher Desktop, Podman Desktop or Colima application was found under the user or system application directories.
- No full-path Docker/Podman/Colima/Nerdctl executable, Docker VM disk image, container volume or `multica`/`pgdata` volume directory was found in their common user-library and home-directory locations.
- The only Docker container metadata directory was an empty macOS container stub; it contained no VM disk or database volume.
- Direct checks for repository `.env`, `.env.local` and `.env.worktree` files found none. This corrects for ignore-aware file searches that can omit local environment files.
- Adjacent repositories under `/Users/logo/self_repo/agent_team` contained no `.env` file supplying an original `multica` database connection.
- Redacted searches of the user's shell startup files, shell history and configuration directory found no reference to a `multica` database URL or host. No credential values were printed or saved.

The repository Compose files describe the expected service and volume names (`multica`, PostgreSQL on port 5432, and `pgdata`) but are configuration only; no matching live service or persisted volume is present in the inspected environment.

### Third-pass continuation audit (2026-08-05, Asia/Shanghai)

The apply workflow rechecked the current external state before deciding whether the gate was still blocked:

- None of `OLD_MULTICA_DATABASE_URL`, `MULTICA_OLD_DATABASE_URL`, `DATABASE_URL`, `POSTGRES_URL`, `PGHOST`, `PGPORT`, `PGDATABASE` or `PGSERVICE` was set in the execution environment.
- Repository `.env`, `.env.local`, `.env.worktree` and `.env.old-db` files were absent.
- PostgreSQL remained unreachable on `127.0.0.1:5432` and `127.0.0.1:5433`, with no matching listening process.
- A fresh filename scan of Downloads, Desktop, Documents and local source repositories found no `multica` SQL dump or backup.

The only Docker-related process was the installed macOS privileged networking helper; it did not provide a running Docker engine, PostgreSQL listener or recoverable database volume. These results repeat the same external blocking condition rather than proving task 1.8 complete.

### Resumed-goal blocking audit (2026-08-05, Asia/Shanghai)

After the apply goal was resumed, three consecutive apply turns repeated the current-state checks. At that time each turn found the same result: no supported database connection variable, no repository environment file, no PostgreSQL listener on ports 5432 or 5433, and no `multica` dump or backup in the inspected user directories. Phase 1 was not started out of order.

### Homebrew PostgreSQL audit

The only native PostgreSQL data directory is `/usr/local/var/postgresql@16`. It was shut down before inspection. To avoid mutating it, a filesystem clone was started on port 55433 and queried; the clone contained only `postgres` and `template1`, not `multica`. The clone was then stopped.

Source cluster facts:

- PostgreSQL format: 16
- system identifier: `7616284549460763462`
- state: `shut down`
- checkpoint LSN: `0/1513650`
- checkpoint WAL: `000000010000000000000001`
- source directory composite file SHA-256: `1aea662815101369171cd449b50cbc2b7274ded2a0f32cfaeecdb6081bcd99ba`

This fingerprint proves the inspected Homebrew cluster state at the Phase 0 boundary, but it cannot substitute for the required logical identity, migration revision, schema and content checksums of the actual old `multica` database.

## Future isolation comparison

Tasks 12.7 and 13.8 must repeat the same revision, normalized schema, normalized data dump, canonical logical content and per-table manifest methods against this endpoint. Any mismatch keeps the release gate failed until the difference is explained. The old database must remain read-only throughout; it must never be used as the Lightweight migrate, reset or runtime target.
