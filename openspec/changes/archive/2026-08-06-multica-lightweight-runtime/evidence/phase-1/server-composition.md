# Phase 1 Lightweight Server composition evidence

Evidence time: 2026-08-05 11:53 Asia/Shanghai

Scope: OpenSpec tasks 3.1-3.13 and acceptance IDs API-01, API-02,
PRC-01, PRC-02 and PRC-06.

## Implemented contract

- `NewLightweightComposition` is the sole process composition root used by
  `main`; it has no Full/Light or edition switch.
- One manifest registers exactly 114 target method/path pairs under explicit
  Public, Human, Workspace, Workspace Admin/Owner, Task, Daemon and WebSocket
  authentication boundaries.
- The target handler graph does not construct Storage, Cloud Runtime,
  Autopilot, Webhook, Channel, Slack, Lark, Composio, VCS or PR refresh
  dependencies.
- `main` starts only the in-memory realtime hub, runtime/task sweeper, batched
  heartbeat scheduler, optional sharded Redis relay and optional metrics
  listener.
- The existing sweeper also deletes expired/used/locked verification codes and
  expired Daemon/Task tokens. Its cleanup interface contains no PAT deletion.
- The metrics listener uses only Go/process/build/HTTP/realtime/Daemon WS
  collectors.
- `/api/config`, `/readyz`, the error envelope, strict target Mutation field
  schemas, Content-Type validation and the six paginated route guards are
  covered by unit contracts.

## Commands and results

The `cmd/server` package's legacy `TestMain` normally exits early when its
legacy integration database is unavailable. The command below deliberately
sets `MULTICA_UNIT_TEST_ONLY=true`; this skips only that database fixture setup
and runs the in-memory/static contracts rather than reporting a false pass.

```text
cd server
MULTICA_UNIT_TEST_ONLY=true go test ./cmd/server \
  -run 'TestLightweight(RouterMatchesFrozenManifest|RemovedRoutesReturnNotFound|CompositionDoesNotConstructExitedDependencies|MainWorkerAllowlist|MutationSchemasCoverTargetRoutes|StrictJSONRejectsBadTransportBeforeHandler|StrictJSONPreservesMergeAndNullableFields|TaskTokenIssueUpdateFieldBoundary|PaginationNormalizesLimitAndValidatesCursor)$' \
  -count=1 -v
```

Result: PASS. The output showed every selected test and subtest executing.

```text
MULTICA_UNIT_TEST_ONLY=true go test ./cmd/server \
  -run 'TestLightweight|TestSweepExpiredCredentialsUsesFrozenAllowlist' -count=1
go test ./internal/metrics -run TestRuntimeRegistryContainsOnlyBasicCollectors -count=1
```

Result: PASS.

## Acceptance mapping

| ID | Evidence | Result |
|---|---|---|
| API-01 | `TestLightweightRouterMatchesFrozenManifest` compares the Chi walk to `contracts/routes.txt` and asserts 114 unique entries | PASS |
| API-02 | `TestLightweightRemovedRoutesReturnNotFound` probes representative removed route families and proves 404 | PASS |
| PRC-01 | `TestLightweightCompositionDoesNotConstructExitedDependencies` checks the target handler graph; `TestLightweightMainWorkerAllowlist` rejects exited constructors/start calls | PASS |
| PRC-02 | `TestLightweightMainWorkerAllowlist` proves no scheduler manager or removed job registration remains in `main` | PASS |
| PRC-06 | `TestLightweightMainWorkerAllowlist` checks the exact approved process lifecycle markers and forbidden worker markers | PASS (composition contract) |

The live PostgreSQL/process/goroutine repetition is intentionally deferred to
the isolated `multica_lightweight` database gates and final runtime acceptance;
the old `multica` database was not used or written for this phase.
