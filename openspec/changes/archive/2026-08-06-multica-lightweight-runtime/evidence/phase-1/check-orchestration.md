# Phase 1 `make check` orchestration evidence

Captured on 2026-08-05 (Asia/Shanghai) for task 2.3.

## Isolation setup

- PostgreSQL 16 ran in a disposable local cluster on a loopback-only listener.
- `make check` created `multica_lightweight_check_full_contract_retry_20260805` from an empty database.
- The supplied original `multica` database was never used by this run.
- Backend, Web, uploads and workspaces used dedicated ports or disposable directories.

## Pipeline result

The updated entrypoint reached every declared stage:

1. target TypeScript typecheck: passed;
2. target Web production build: passed;
3. target TypeScript/Web contract tests: passed (Web 188, Core 1234, Views 3485);
4. current migration chain and all Go packages: passed, including `server/pkg/agent`;
5. dedicated backend and Web startup: passed;
6. Playwright browser E2E: executed all 30 tests, 22 passed and 8 failed.

The eight browser failures are current Full-product assertions in Agent MCP, Issue Table, legacy onboarding locale and Settings/Composio coverage. They are not evidence that the final Lightweight product passes BLD-03. Target browser coverage and removal of these obsolete Full-product tests remain part of tasks 10.14, 2.6 and the final release gates.

## Cleanup result

On failure the check entrypoint stopped both processes and dropped the fresh check database. The outer harness then stopped the disposable PostgreSQL cluster and moved its exact runtime directory to Trash. No listeners remained on the test ports.

Conclusion: task 2.3's orchestration and isolation behavior are implemented. BLD-03 remains **not passed** until the final Lightweight schema and target browser suite pass end to end.
> 2026-08-06 终态更新：下述失败是 Phase 1 历史记录。目标 E2E 与 Fresh Checkout `make check` 已通过，见 `../phase-6/final-release.md`。
