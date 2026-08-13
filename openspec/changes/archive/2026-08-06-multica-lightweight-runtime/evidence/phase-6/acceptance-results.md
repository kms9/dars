# Phase 6：142 项验收结果索引说明

机器状态以 `contracts/acceptance-index.json` 为准。本文件说明每个 family 的运行证据来源；最终状态为 141 passed、1 blocked、0 failed、0 not_run。

| Family / IDs | 状态 | 主要证据 |
|---|---|---|
| AG-01..07 | passed | Fresh PostgreSQL live flow、Runtime/Agent/Skill contract、真实 Runtime Agent |
| API-01..10 | passed | 114-route/credential/strict DTO/lifecycle contracts、Fresh Checkout E2E |
| AUTH-01..04 | passed | identity live flow、production readiness/mail fail-closed tests、浏览器登录 |
| BLD-01..05 | passed | build-graph contract、Fresh Checkout `make check`、端口清理复验 |
| CH-01..11 | passed | Chat live flow、真实浏览器 streaming/final projection、removed attachment negative |
| CM-01..11 | passed | Comment/Mention live flow、append-only route matrix、LOC-3/LOC-4 |
| DB-01..06 | passed | Fresh migration/catalog、backup/restore、真实 attribution |
| DB-07 | blocked | Phase 3 最后一次 hash 一致；最终 13.8 无安全凭据，不能复读旧库 |
| DB-08..10 | passed | migration static contract、reset/name guard、constraint rejection tests |
| DR-01..11 | passed | 两个独立 Daemon、register/heartbeat/claim/token tests、ready/stop/restart observations |
| FLOW-01..07 | passed | LOC-3、LOC-4、mention/re-entry/reconciliation/failure/offline tests |
| IS-01..11 | passed | Run live flow、cursor/filter/status contracts、浏览器 Run 页面 |
| PRC-01..07 | passed | composition/Worker/event/control WS allowlists 与 fallback tests |
| RC-01..04 | passed | running Comment/Mention races、delivered/coalesced follow-up live flow |
| RP-01 | passed | Runtime Profile CRUD + Daemon registration live flow |
| SEC-01..12 | passed | 114-route credential matrix、Task/Daemon scope、Secret codec/backup/reveal audit |
| SK-01..03 | passed | Skill/File/Bundle/invocation gate live flow |
| SQ-01..11 | passed | Squad lifecycle live flow、browser E2E、LOC-3/LOC-4 |
| TQ-01..11 | passed | real PostgreSQL concurrent claim/lifecycle/replay/fence tests、真实 Task |
| WS-01..06 | passed | identity/workspace live flow、role/delete/invitation negative、browser context |

通用执行证据：

```text
CHECK_DATABASE_SUFFIX=physical_delete_final_4 make check -> PASS
Fresh detached checkout + candidate overlay, CHECK_DATABASE_SUFFIX=fresh_candidate_2 make check -> PASS
Target TS tests -> 119 passed
Target Go tests -> all passed, no database skip
Chromium E2E -> 1 passed
Real Direct Chat -> completed with visible streaming
Real FLOW-01 -> in_review
Real FLOW-02 -> in_review
```

DB-07 的 `blocked` 是外部证据缺口，不是数据库实现失败。其解除条件是使用不落盘到仓库/命令的受控凭据，以只读事务重复 Phase 0 的 revision、normalized schema dump、normalized data dump、canonical logical content 与 per-table manifest，并与 `evidence/baseline/old-database-baseline.json` 完全比较。

