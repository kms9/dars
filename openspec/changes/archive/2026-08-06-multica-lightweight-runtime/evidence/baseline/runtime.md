# Phase 0 runtime baseline

## Identity and isolation

- Source commit: `736fbc8a5f1b22d48354a0e55baa00661e9e4326`
- Detached worktree: `/tmp/dars-lightweight-baseline.plpoR9/worktree`
- PostgreSQL: Homebrew PostgreSQL `16.13`, independent cluster `/tmp/dars-baseline-postgres.qsJN4Y`, `127.0.0.1:55432`
- Database: `multica_baseline_736fbc8a`
- HTTP listener: `127.0.0.1:58080` probe against PID `78239`
- Optional Redis and metrics listeners were disabled. The development email service used a fixed non-production verification code.

The server was terminated after measurement with `SIGQUIT` so the Go runtime emitted a process-backed goroutine inventory. This was an isolated temporary process; it did not affect the current checkout or the user's original database.

## Ready time

The wall-clock interval from process launch to the first successful `GET /readyz` response was measured with Ruby's monotonic clock and a polling loop:

- Ready time: `0.134633 s`
- First ready response: HTTP `200`
- Independent post-warmup probes: `/health`, `/healthz`, and `/readyz` all returned HTTP `200`
- `lsof` confirmed the measured PID owned the TCP listener on port `58080`.

## Idle RSS

After a 10-second warmup, `ps -o rss` was sampled 10 times at one-second intervals:

```text
30672 30688 30688 30688 30688 30128 28176 28160 28160 28064
```

- Unit: KiB, as reported by macOS `ps`
- Mean: `29611.2 KiB` = `30321869 bytes` (rounded)
- Maximum: `30688 KiB`
- Lightweight acceptance ceiling derived from the frozen baseline mean: `24257495 bytes` (`80%`)

The separate macOS `sample` report showed a `13.3M` physical footprint, but that tool uses a different accounting model. Release comparisons must use the same `ps RSS` procedure above.

## Worker inventory

`workers-736fbc8a.tsv` records the actual active and inactive background components. Active business/runtime goroutines observed in the `SIGQUIT` dump were:

- in-memory realtime hub
- runtime sweeper
- batched heartbeat scheduler
- Autopilot failure monitor
- database stats logger
- Webhook delivery supervisor plus three worker loops
- channel inbound supervisor
- channel media reconciler
- scheduler manager, with task-usage rollup and scheduled-Autopilot jobs registered

The GitHub PR refresh manager was constructed but disabled, Redis relay was not configured, and metrics listener was not configured. HTTP serving and Go/pgx internal goroutines are transport/runtime infrastructure and are not counted as business Worker entries.

## Router snapshot

The sorted Chi router walk contains `351` unique method+path pairs. The raw fixture is `router-736fbc8a.txt`.

- SHA-256: `2c7a56f76b2c14fe157c84163c30615a973d7c765f69251b3f601d0941b523e7`
- The target contract has 114 routes, so the baseline contains substantial product surface outside the target. Task 1.4 freezes the target set separately; this file is intentionally the unfiltered current-state snapshot.

## PostgreSQL schema snapshot

All current migrations completed successfully in the isolated database. The migration ledger is `schema_migrations`; its latest applied version is `252_agent_builder_draft`.

- Database size after migration and idle runtime probe: `13622295 bytes`
- Public tables: `87`
- Public indexes: `308`
- Constraints: `97` CHECK, `106` FOREIGN KEY, `84` PRIMARY KEY, `36` UNIQUE
- Extensions: `pg_trgm`, `pgcrypto`, `plpgsql`

The frozen target explicitly requires 26 tables and no foreign keys/cascades. The 106 baseline foreign keys are evidence of the current state, not permission to carry them into the target schema.

Snapshot files and hashes:

| File | Rows | SHA-256 |
|---|---:|---|
| `schema-tables.txt` | 87 | `94fadedddba3c87134c636a01b0ca2a466c94b44e1a7ab3a1631e9152bb03b15` |
| `schema-columns.txt` | 927 | `c4cef15c303602f51d9b320907bb60230ea2f4562517f8d43ee45b9fbee7d176` |
| `schema-indexes.txt` | 308 | `90d03af5293066a436e576c615b9d1243f40e4a2e439aca2644190e9ee2b9293` |
| `schema-constraints.txt` | 1075 | `caffa68d5b4ec4941e7c83ab1328b0b013ce6d4ca0a13ce0480ae51e24d5489d` |
| `schema-migrations-tail.txt` | 10 | `8c41d1373b09086e21decd912c24f06db7c92df29a013af51ec6a773b7d7d4cb` |

The row counts above are line counts in canonical `psql -A -t` output. The database contains one scheduler execution row created by the baseline process; schema shape and migration identity are unaffected.
