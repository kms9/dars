## 1. Server Boundary and Dependencies

- [x] 1.1 Add the official Go MCP SDK, gRPC, Protobuf descriptor/runtime compiler, and OpenAPI parser as direct Server dependencies, then add an import-graph contract proving `cmd/multica` does not depend on them or `internal/mcpgateway`.
- [x] 1.2 Create the `server/internal/mcpgateway` package boundaries, narrow identity/store/projector interfaces, and a lifecycle object constructed and closed only by `server/cmd/server`.
- [x] 1.3 Refactor Chi registration into common middleware plus separate MCP and REST groups so `/bundles/{bundleId}/mcp` bypasses `lightweightErrorEnvelope` while existing REST routes retain their frozen transport behavior.
- [x] 1.4 Add Server configuration parsing and validation for `MULTICA_PUBLIC_URL`, egress allowlists, timeouts, size limits, redirects, and concurrency defaults without adding a Full/Light or Daemon feature switch.
- [x] 1.5 Revise scope contract tests so target Web/shared frontend may contain only the approved Tools control plane while Desktop/Mobile/Docs, `server/internal/daemon`, `server/pkg/agent`, `server/cmd/multica`, and Daemon protocol files remain free of Gateway execution changes/imports.

## 2. Tool Registry Data and Secret Foundation

- [x] 2.1 Add additive migrations for `tool_source`, `tool_source_revision`, `tool_definition`, `tool_bundle`, `tool_bundle_item`, `agent_tool_bundle_head`, `tool_source_artifact`, and `tool_source_secret`, plus nullable Task/Task Token Bundle pin columns, with Workspace scope, immutable/status checks, and no Foreign Key or Cascade.
- [x] 2.2 Add every primary/unique/query index in its own single-statement `CREATE [UNIQUE] INDEX CONCURRENTLY` migration, then bind required primary/unique constraints in later migrations.
- [x] 2.3 Add Workspace-scoped sqlc queries for Source/revision lifecycle, atomic Bundle publication and Agent-head switch, write-once Task pin, Task Token Bundle scope, authorized Bundle/item reads, artifacts, secrets, live revoke state, and Claim lookup.
- [x] 2.4 Extract a reusable versioned AEAD primitive while preserving the existing `agentconfigsecret` facade, and add a domain-separated Tool Source secret codec with ciphertext-swap and redaction tests.
- [x] 2.5 Implement explicit transactional Source/Bundle retention and Workspace delete cleanup for Agent heads, Task/Token pins, Bundle items, definitions, revisions, artifacts, secrets, activity records, and current revision state; retained Bundle artifacts must not be removed early.
- [x] 2.6 Update schema/table manifests and add Fresh DB migration tests for exact objects, no FK/Cascade, concurrent indexes, uniqueness, immutable Bundle writes, cross-Workspace rejection, Task pin stability, rollback compatibility, and cleanup order.

## 3. Tool Source Control Plane

- [x] 3.1 Add strict REST DTO schemas, stable redacted error codes, route-manifest entries, and Owner/Admin authorization for Tool Source and ToolBundle route families.
- [x] 3.2 Implement Source list/get/create/update with normalized kinds, stable names, disabled-by-default creation, immutable revision staging, and redacted endpoint/auth/artifact metadata.
- [x] 3.3 Implement bounded validation and atomic publish so successful revisions advance `current_revision` with a complete definition set while failed revisions leave the last ready revision untouched.
- [x] 3.4 Implement Source enable/disable/delete and tool listing so emergency disable immediately blocks affected Bundle discovery/call, while delete respects retained Bundle/artifact references and closes or retires revision-owned cache/connection state.
- [x] 3.5 Implement complete Agent tool-selection publication: normalize selected MCP/tools/Proto packs, validate same-Workspace ready revisions and Provider compatibility, create a new opaque Bundle and immutable items for every semantic change, and atomically switch the Agent head.
- [x] 3.6 Implement dedicated bounded artifact upload/storage and encrypted Source auth handling; ensure Control Plane responses, validation errors, logs, Realtime, metrics, and audit never return artifact contents or Secret plaintext.
- [x] 3.7 Add Control Plane integration tests covering role matrix, cross-Workspace probing, invalid IDs, strict request fields, revision conflicts, failed/partial Bundle publication, semantic change/revert new-ID behavior, no-op save, runtime incompatibility, revoke/disable/delete, retained artifacts, and redaction sentinels.
- [x] 3.8 Add read-only Bundle detail/summary and explicit Bundle revoke APIs; treat content hash as integrity/diagnostic metadata only and never as authority to reuse a historical Bundle ID after a semantic change.

## 4. MCP Data Plane and Claim Projection

- [x] 4.1 Construct the official MCP Server and stateless Streamable HTTP handler under `/bundles/{bundleId}/mcp` with request-cancellation propagation, tool-only capabilities, bounded bodies, and MCP-native status/header/body/stream tests.
- [x] 4.2 Wrap the Bundle route in Task-only authentication, resolve the opaque path ID, and require exact Token/Task/Workspace/Agent/Bundle-pin match without accepting client-supplied identity overrides or treating Bundle ID as a credential.
- [x] 4.3 Implement database-authoritative `tools/list` projection from immutable Task-pinned Bundle items with deterministic ordering, followed by live Task/Agent/Bundle/Source/tool revoke checks; never follow the Agent head or Source current revision for an existing Task.
- [x] 4.4 Implement `tools/call` lookup from the same Bundle item, input-schema validation, per-call live reauthorization, fixed invocation-plan dispatch, deadline/cancellation/concurrency enforcement, and no transparent retry for non-idempotent calls.
- [x] 4.5 Pin the Agent current Bundle into each Task at creation, keep the pin write-once across Claim/re-Claim, copy it into Task Token scope, and implement Claim projection that returns existing semantics without a pin or exactly one `mcpServers.multica` entry with the Bundle URL and current token for a Bundle Task.
- [x] 4.6 Add a code-owned Provider compatibility allowlist and enforce it at Bundle publication, Agent runtime change, Task creation, and Claim; unsupported Providers must fail before a task silently starts without Gateway tools.
- [x] 4.7 Add Claim and MCP integration tests for no-Bundle semantics, Gateway-only Server document, unchanged runtime-local Daemon merge, public URL failure, token hashing/non-persistence, Task creation/re-Claim pin stability, active-to-terminal revocation, cross-Bundle/Agent calls, runtime changes, and unchanged Claim DTO shape.
- [x] 4.8 Add a deterministic Server-local registry invoker to prove end-to-end Bundle discovery/call, multi-replica stateless requests, explicit revoke, cancellation, audit metadata, and REST/MCP error-boundary isolation before adapters are enabled.
- [x] 4.9 Add concurrency tests for Agent head switch versus Task creation/Claim so Bundle header/items, Agent head, Task pin, Token scope and Claim URL can never observe mixed IDs.

## 5. Shared Egress Policy and Remote MCP Adapter

- [x] 5.1 Implement canonical endpoint parsing and connect-time DNS/IP policy that denies userinfo, unsupported schemes/ports, loopback, link-local, multicast, unspecified, metadata, and non-allowlisted private/VPC ranges.
- [x] 5.2 Implement controlled HTTP transport/dialing with approved-IP pinning, original-host TLS verification, redirect revalidation, explicit proxy policy, body/response limits, deadlines, cancellation, and disabled automatic invocation retries.
- [x] 5.3 Implement the equivalent approved resolver/dialer contract for gRPC so Source validation and every new channel connection reapply DNS, IP, port, TLS identity, and allowlist decisions.
- [x] 5.4 Implement Remote Streamable HTTP MCP Source validation/discovery that snapshots tools into a staged Source revision with namespace-safe names, can publish selected definitions into immutable Bundle items, and rejects prompts/resources/roots/sampling/logging or persistent callback/session requirements.
- [x] 5.5 Implement the Remote MCP invoker using a bounded short-lived upstream client context per call, fixed upstream tool identity, Server-held credentials, cancellation, result limits, and unconditional cleanup.
- [x] 5.6 Add real local-network egress tests for DNS rebinding, redirects, mixed allowed/denied DNS answers, proxy bypass, TLS mismatch, metadata/private targets, timeout/cancellation, oversized responses, unsupported upstream capabilities, and Secret redaction.

## 6. Dynamic gRPC Adapter

- [x] 6.1 Implement a common descriptor registry that normalizes embedded and uploaded descriptor sets, resolves imports, rejects streaming methods, generates bounded deterministic MCP schemas for unary RPCs, and exposes immutable artifact IDs/digests for Bundle publication.
- [x] 6.2 Add build-time descriptor embedding and startup descriptor-registry registration, then materialize a selected embedded descriptor as a Workspace-scoped immutable Source artifact/revision when an authorized Admin binds its endpoint, without generated service clients or a separate Server process.
- [x] 6.3 Implement the replica-local gRPC channel manager keyed by Source revision, endpoint, TLS, and metadata identity, with shared egress dialing, idle retirement, revision invalidation, and Server shutdown cleanup.
- [x] 6.4 Implement strict ProtoJSON-to-`dynamicpb` request conversion, fixed full-method `ClientConn.Invoke`, dynamic response conversion, deadline/cancellation, response limits, redacted errors, and invocation audit.
- [x] 6.5 Implement bounded `.proto`, archive, and descriptor-set validation/storage using the runtime compiler; publish through the same descriptor registry, retain artifacts referenced by Bundles, and reject zip bombs, missing imports, conflicts, streaming-only services, and partial definition sets.
- [x] 6.6 Add descriptor/schema unit tests and real unary gRPC integration tests for embedded/uploaded equivalence, TLS/metadata, connection reuse/retirement, invalid arguments, unavailable endpoints, timeout/cancellation, and no credential leakage.

## 7. OpenAPI HTTP Adapter

- [x] 7.1 Implement bounded OpenAPI document and URL import with shared egress fetching, local/approved `$ref` resolution, depth/count/size limits, operation validation, and deterministic collision detection.
- [x] 7.2 Compile supported operations into immutable definitions and request plans that fix scheme/host/base path/method/auth behavior while mapping path/query/header/body fields to MCP JSON schemas.
- [x] 7.3 Implement the HTTP invoker with strict argument validation, protected header filtering, Server-held authentication, shared egress transport, bounded response mapping, cancellation, redacted errors, and audit.
- [x] 7.4 Add parser/schema unit tests and real HTTP integration tests for each parameter location, request/response bodies, auth, remote refs, redirects, endpoint/header override attempts, non-idempotent no-retry behavior, limits, and SSRF negatives.

## 8. Observability and Lifecycle Hardening

- [x] 8.1 Add structured MCP/Source/Bundle metrics and redacted activity audit for Task/Agent/Bundle/item/source/revision identity, outcome, latency and sizes without recording raw arguments, results, headers, tokens, artifacts, descriptors, or credentials.
- [x] 8.2 Add immutable Bundle/revision cache lifecycle, connection/client shutdown hooks, in-flight deadline draining, and tests proving revoked/disabled Bundles or revisions cannot authorize new calls while existing bounded calls terminate safely.
- [x] 8.3 Add per-Task and per-Source concurrency/size/deadline guards and abuse tests showing one tenant/source cannot exhaust unbounded goroutines, memory, connections, descriptor compilation, or response streaming.
- [x] 8.4 Update `.env.example` and operator documentation for public URL, egress allowlists, proxy/TLS behavior, limits, readiness diagnostics, unsupported local-network topology, external-availability non-goal, and rollback by disabling Agent Gateway access/revoking Bundles/Sources.

## 9. Verification and Release Evidence

- [x] 9.1 Run and record focused gateway/control/data/secret/egress/adapter tests plus MCP transport/conformance tests, keeping E2/build evidence separate from runtime evidence.
- [x] 9.2 Regenerate sqlc, run Fresh Lightweight migrations, and verify manifests, index statements, schema identity, encrypted storage, explicit cleanup, and additive rollback behavior.
- [x] 9.3 Run `gofmt`, `go vet`, `go test ./...`, target builds, `make test`, and `make check`; fix only Server-side scope regressions and record any skipped or unavailable gate.
- [x] 9.4 Verify the final diff contains no changes under frozen Daemon/Provider/CLI/protocol or Desktop/Mobile/Docs paths, target Web changes stay inside the approved control-plane surface, and the built `multica` dependency graph excludes Gateway/MCP/gRPC/OpenAPI/Proto compiler packages.
- [ ] 9.5 Re-run the authorized unchanged-Daemon E3 compatibility matrix for every Provider declared supported against the final `/bundles/{bundleId}/mcp` route, proving Bundle URL/header preservation, Gateway-only Server Claim document, unchanged runtime-local merge, Runtime `tools/list`, tool call, B1/B2 Task pin stability and terminal-token revocation; mark every untested Provider unsupported.
- [x] 9.6 Execute controlled local-fixture E3 flows for Remote MCP, embedded unary gRPC, uploaded Proto unary gRPC and OpenAPI HTTP, correlating fixture observations with Source revision, Bundle/item, Task pin, MCP result and redacted audit; do not treat this as proof of external dependency availability.
- [x] 9.7 Execute the full security negative matrix for tenant and Bundle boundaries, Bundle enumeration/token mismatch, immutable-write attempts, credentials, malformed protocols/schemas, endpoint override, SSRF/DNS/redirect/TLS, limits, cancellation, concurrency, no-retry semantics, retained artifact and deletion cleanup.
- [x] 9.8 Update route/table/release acceptance manifests and produce a final evidence report that explicitly separates completed Server checks, unchanged-Daemon/runtime proof, controlled local-fixture proof, unsupported Providers, external-availability non-goal and remaining production gaps.
- [x] 9.9 Replace the pre-release MCP Facade route with `/bundles/{bundleId}/mcp` across Claim projection, Chi routing, tests, specs and operator docs; do not keep a legacy alias, and rerun focused MCP/Claim tests, strict OpenSpec validation and the Fresh DB `make check` gate.

## 10. Web Tool Control Plane

- [x] 10.1 Add Owner/Admin-only Agent current ToolBundle read and clear endpoints with strict DTOs, same-Workspace authorization, retained Task pin semantics, redacted responses and malformed/cross-Workspace tests.
- [x] 10.2 Add bounded atomic multipart Tool Source import for OpenAPI/Swagger documents, `.proto`, Proto archives and descriptor sets; remove the effective JSON/base64 upload mismatch and test cancellation, oversized input, partial-write cleanup and redaction.
- [x] 10.3 Add explicit Swagger 2.0 JSON/YAML detection and Server-side normalization into the OpenAPI compiler, including fail-closed conversion error coverage and no browser-side parsing.
- [x] 10.4 Add `packages/core` Tool Source/Definition/Bundle zod schemas, camelCase DTOs, API methods, Workspace-scoped React Query keys/hooks and malformed-response tests.
- [x] 10.5 Add target Web Workspace `Tools` navigation, list/import/detail routes and shared views for Source lifecycle, redacted status, catalog preview, validate/enable/disable/update/delete actions and `update available` semantics.
- [x] 10.6 Add Agent `Capabilities > Tools` with current Bundle restore, Source/individual Tool selection, full-selection publish, clear-current behavior, no-op/new-ID feedback, Provider incompatibility and Task-pin explanation.
- [x] 10.7 Add frontend permission/direct-route guards, stable error UX and contract tests proving raw `agent.mcp_config`, runtime-local MCP, credentials, Gateway execution dependencies and non-target app wiring remain absent.
- [x] 10.8 Add focused component and browser E2E coverage for file/URL import, catalog preview, different Agent selections, refresh restore, Source revision warning, publish/clear and Member denial.

## 11. Revised Verification

- [x] 11.1 Run OpenSpec strict validation, target TypeScript typecheck/test/lint/build, focused Go tests and browser E2E for the Web control plane; record skipped gates separately.
- [x] 11.2 Run Fresh DB `make check`, re-verify unchanged Daemon/CLI dependency graph and update release evidence so Web E2 evidence remains separate from unchanged-Daemon E3/provider proof.

## 12. Agent Tool Exported Names

- [x] 12.1 Freeze Source canonical name, Agent exported name, immutable Bundle/new-ID and deferred Skill binding boundaries across proposal, design and delta specs; pass strict OpenSpec validation.
- [x] 12.2 Replace Agent Bundle publication with strict `items[{tool_definition_id, exported_name}]`, validate normalized MCP-safe uniqueness, snapshot canonical/exported identities, include aliases in manifest/audit/DTOs and cover no-op/new-ID/atomic-failure behavior.
- [x] 12.3 Project and resolve only Bundle exported names in Gateway `tools/list`/`tools/call`, retain fixed upstream invocation, and test that canonical/upstream names are not implicit call aliases.
- [x] 12.4 Update `packages/core` ToolBundle types, zod boundary schemas and API request contract for canonical/exported names, including malformed-response tests.
- [x] 12.5 Update Source import/detail naming copy and Agent `Capabilities > Tools` to edit, reset, validate, publish and refresh-restore per-item exported names without browser persistence or Source mutation.
- [x] 12.6 Add focused Go, shared-view and browser coverage for default/custom aliases, duplicate/invalid names, per-Agent isolation, alias-only new Bundle ID and historical Task pin behavior.
- [x] 12.7 Run focused Go and target frontend checks, strict OpenSpec validation and frozen Daemon/CLI/non-target dependency-boundary scans; record the unchanged-Daemon E3 Provider matrix separately.
