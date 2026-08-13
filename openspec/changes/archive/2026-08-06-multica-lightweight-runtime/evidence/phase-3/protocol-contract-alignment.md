# Protocol contract alignment evidence

> 2026-08-06 终态更新：本文件末尾的 remaining gate 已由 Web/CLI 物理删除与最终 cross-tree scan 解除；见 `../phase-5/physical-delete.md`。

Date: 2026-08-05 (Asia/Shanghai)

Scope: the Server/Daemon and active Web realtime portions of task 6.11. The
task remains open until the target Web API client and retained CLI commands are
cut over in tasks 10.x/11.x.

## Implemented contract sources

- `server/pkg/protocol/lightweight_contract.go` owns the 30 Daemon HTTP
  method+path templates, the 31 Web event types, the 8 Daemon control event
  types, and parameter-safe client path expansion.
- `protocol.DaemonProtocolVersion` is the only Go protocol version. The
  Lightweight handler no longer duplicates its own string constant.
- The Lightweight Server route manifest and mutation DTO map reference the
  shared Daemon route constants.
- The Daemon client and WS handshake reference the same constants. Production
  client sources contain none of the removed batch-claim aliases, Runtime
  update-result route, or Autopilot GC route.
- `packages/core/types/lightweight-protocol.ts` freezes the target Web WS path,
  `workspace_id` query parameter, protocol version, and Web event union.
- `WSClient` rejects events outside that union before both typed and `onAny`
  consumers. This prevents retained legacy modules from making exited events
  part of the active Lightweight wire protocol while physical deletion is
  still pending.
- The old Issue GC compatibility probe/fan-out was removed. A missing batch
  endpoint is treated as an incompatible server and local data remains for a
  later retry.

## Frozen-fixture gates

- `TestLightweightDaemonProtocolMatchesFrozenManifest` compares the shared
  Daemon route list to the Daemon subset of `contracts/routes.txt`.
- `TestLightweightEventTypesMatchFrozenContracts` compares both Go event lists
  to `contracts/web-events.txt` and `contracts/daemon-events.txt`.
- `lightweight-protocol.test.ts` independently compares the TypeScript Web
  event union to `contracts/web-events.txt` and rejects representative exited
  events (`inbox:new`, `chat:quick_actions`).
- `WSClient` tests prove `workspace_id` replaces `workspace_slug`, exited
  events never reach consumers, and `event_id` at-least-once duplicates are
  suppressed.

## Verification

```text
go test ./pkg/protocol ./internal/lightweightapi ./internal/daemon ./internal/daemonws ./cmd/server \
  -run 'TestLightweight|TestClientLightweight|TestClient_ListWorkspaces|TestGCWorkspace_(BatchesAndDeduplicatesIssueChecks|DoesNotFallBackToOldServer|BatchFailureDoesNotFanOutOrClean)|TestClaimTasksWSFirst_DoesNotFallbackToLegacyClaim' -count=1
PASS

go test ./internal/daemon ./internal/daemonws ./internal/lightweightapi ./cmd/server -count=1
PASS (Daemon 27.269s; daemonws, lightweightapi and cmd/server pass)

pnpm -C packages/core typecheck
PASS

pnpm -C packages/core test -- api/ws-client.test.ts types/lightweight-protocol.test.ts
PASS (115 files, 1,238 tests)

git diff --check
PASS
```

Production-source negative scans passed for:

- removed Daemon routes `/api/daemon/claim`, single-Runtime claim,
  Runtime update result and Autopilot GC;
- Daemon path literals outside the shared contract in `client.go`, `wakeup.go`,
  `lightweight_composition.go` and `lightweight_transport.go`;
- GC legacy endpoint/protocol fallback state.

## Remaining gate

The existing full-product CLI and Web API client still contain exited paths.
They are not accepted as evidence for API-03 or task 6.11. Tasks 10.1-10.13 and
11.1-11.5 must cut those surfaces over and rerun the cross-tree scan before
6.11 can be checked.
