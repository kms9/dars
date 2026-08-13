## Context

See [proposal.md](./proposal.md) for motivation and scope. The current Server already mints a task-scoped `mat_` credential during Claim, decrypts the Agent's stored `mcp_config`, and returns both through the existing Claim DTO. The current Daemon already merges that canonical config with runtime-local MCP entries and adapts URL/header entries for supported Provider runtimes. `agent_task_queue.runtime_mcp_overlay` exists in PostgreSQL but the current Claim path does not consume it.

The shipping topology remains one Go Web Server process, PostgreSQL, optional Redis, Web, and a separately built local `multica` Daemon/CLI. The new gateway must share the Server lifecycle and database without entering the Daemon/CLI import graph. The current Chi router also installs a REST error envelope globally; that envelope cannot wrap MCP protocol errors or streamed responses.

The revised Lightweight product boundary includes a targeted Web Human control plane for Tool Source import/management and Agent ToolBundle selection. The browser remains an untrusted REST client: it never parses/compiles tool artifacts, holds upstream credentials, connects to upstream tools, or implements MCP data-plane behavior. The change still does not restore raw Agent MCP management.

The added ToolBundle requirement makes the Server-owned selection an immutable publication rather than a live query over an Agent's current bindings. The Server snapshots the catalog contract it controls; availability and behavioral stability of external HTTP/gRPC/MCP dependencies remain outside the snapshot guarantee and release claim.

## Goals / Non-Goals

**Goals:**

- Make the Web Server the only MCP Server, Tool Registry, policy authority and upstream invoker.
- Reuse the existing Claim `agent.mcp_config` field so the current Daemon and Provider adapters remain unchanged.
- Keep registry state, upstream credentials and audit authority in the Server/PostgreSQL trust boundary.
- Materialize every semantic Agent tool-selection change as a new immutable ToolBundle and pin each Task to one Bundle before Claim.
- Support stateless, horizontally scalable MCP discovery/call plus OpenAPI, unary dynamic gRPC and Remote MCP tool sources.
- Enforce the same Workspace/Agent/Task/Bundle identity at authentication, discovery and every invocation.
- Keep configuration snapshots stable across Task Claim/re-Claim while making explicit Bundle/Agent/Source/tool revocation effective for later discovery/calls.
- Keep migrations additive and permit rollback by disabling Agent Gateway access/revoking Bundles before deploying the prior Server.
- Provide Owner/Admin-only Workspace Tools pages and Agent `Capabilities > Tools`, backed by schema-validated shared queries and redacted Control Plane responses.
- Preserve immutable Source catalog identity while allowing each Agent Bundle to expose a stable, independently chosen MCP tool name.

**Non-Goals:**

- Modifying Daemon source, DTOs, protocol version, Provider adapters, CLI commands or release binary behavior.
- Adding Desktop/Mobile/Docs flow, generic Integration model or generic Attachment system.
- Exposing raw `agent.mcp_config`, runtime-local MCP entries, upstream credential values or Gateway execution logic in Web.
- Supporting Daemon-local, user-VPN or LAN-only targets, tunnels, reverse callbacks or Agent direct upstream access.
- Proxying MCP prompts/resources/roots/sampling/logging, state that must survive multiple tool calls, or legacy stdio/SSE upstreams.
- Mapping streaming gRPC methods to tools in the first delivery.
- Generating `pb.go`, dynamically rebuilding the Server, or restarting Server/Daemon for uploaded Proto.
- Guaranteeing reachability, uptime, latency or behavioral immutability of external dependency services; controlled local fixtures prove only the Web Server backend contract.
- Adding a formal Skill requirement manifest, automatically binding Skill requirements to Definitions, or projecting arguments between incompatible HTTP/gRPC schemas; those build on Bundle aliases in a later change.

## Decisions

### 1. Keep one shipping Server process and add a Server-owned gateway graph

`server/cmd/server` remains the only composition root. It constructs a `mcpgateway.Gateway`, registry store, secret codec, egress clients and invokers, injects a narrow Claim projector interface into `lightweightapi.Handler`, mounts the MCP handler, and closes connection managers during the existing Server shutdown sequence.

The implementation lives under:

```text
server/internal/mcpgateway/
├── gateway.go              # MCP server, discovery and dispatch
├── claimconfig.go          # canonical multica Claim entry
├── identity.go             # narrow task identity contract
├── registry.go             # authorized DB-backed catalog reads
├── policy.go               # invocation policy and limits
├── audit.go
├── secret.go
├── egress/
├── openapi/
├── grpcdynamic/
└── remote/
```

`lightweightapi` depends only on a small `ClaimConfigProjector` interface; the gateway receives an identity resolver adapter from the composition root instead of importing Handler internals. This prevents an import cycle and keeps authentication owned by the existing Server API layer.

Alternative: start an independent MCP service or Worker. Rejected because it would add a second lifecycle, configuration surface and authorization/cache consistency problem without solving a current scaling need. The stateless handler and replica-local connection pools already fit the Web Server process.

### 2. Give Bundle-scoped MCP paths a route-specific transport boundary

Common request ID, logging, metrics and recovery middleware remain outer middleware. REST routes are registered in a Chi group that additionally applies `lightweightErrorEnvelope`, strict JSON and REST CORS behavior. `/bundles/{bundleId}/mcp` is mounted outside that REST group, wrapped only by task authentication, Bundle authorization, MCP-specific limits/metrics and the official Go MCP SDK handler.

The SDK transport uses stateless Streamable HTTP and request-cancellation propagation. Chi resolves the opaque Bundle ID into request context before the shared SDK handler processes the request. The handler lets the SDK return protocol-correct method rejection, content types, headers, JSON-RPC errors and streams. Task authentication happens before MCP decoding; an invalid token, unknown Bundle, cross-Workspace Bundle or token/Task/Bundle mismatch returns a non-enumerating authentication/authorization failure before discovery or invocation.

Alternative: special-case the `/mcp/**` prefix inside `lightweightErrorEnvelope`. Rejected because future MCP paths/methods could silently regain buffering; structural route separation is easier to verify.

Alternative: hand-write JSON-RPC and protocol negotiation. Rejected because the official SDK already implements current and negotiated legacy MCP transport behavior and conformance fixes.

### 3. Pin a Bundle to the Task and project one transient Gateway entry

Saving a semantic Agent tool selection publishes a new opaque Bundle ID and atomically advances the Agent's current Bundle pointer. A no-op save MAY retain the current ID, but a changed selection, Source revision, public contract, invocation plan, policy or Proto artifact digest MUST create a new ID; a content hash is diagnostic and MUST NOT cause a previous ID to be reused after a semantic change or revert.

Task creation copies the Agent's current Bundle ID into a write-once Task pin. Claim and re-Claim always use that Task pin, so queued or retried Tasks do not drift to a newer Agent configuration. After the task token is minted and stored as a hash with the same Bundle scope, `buildClaimedTask` calls the injected projector within the Claim transaction. The projector:

1. Resolves the Task's pinned Bundle and verifies Workspace, Agent, non-revoked Bundle state and the runtime Provider's code-owned compatibility allowlist.
2. Returns the existing stored MCP semantics unchanged when the Task has no Bundle pin.
3. Parses `MULTICA_PUBLIC_URL` from Server configuration; it never derives a public endpoint from the request Host or forwarded headers.
4. For a Bundle Task, returns a canonical document containing only `mcpServers.multica`, with URL `MULTICA_PUBLIC_URL + /bundles/{bundleId}/mcp` and the current `mat_` token. Other Server-stored Agent MCP entries do not enter that Claim document.
5. Returns only the effective JSON for the Claim response. It performs no write of the token-bearing document.

Bundle publication, Agent runtime changes, Task creation and the final Claim path enforce the Provider allowlist. The initial allowlist contains only Provider/runtime combinations proven by the E3 compatibility matrix; adding another Provider requires release evidence, not a user override. This makes unsupported integrations fail before a task silently starts without required tools.

The Daemon's existing merge of runtime-local MCP entries remains unchanged. “Claim contains only the Gateway” applies to the Server-managed `claim.agent.mcp_config`; it does not suppress or redefine locally owned Runtime MCP entries.

`runtime_mcp_overlay` is not used for this feature. Persisting a token-free template there adds another source of configuration truth, while persisting the effective entry would violate token handling. A future cleanup may retire that field separately.

Alternative: permanently save the Gateway entry in `agent.mcp_config`. Rejected because Task Token identity and lifetime are per Claim and a stored token would be stale and recoverable from backups.

Alternative: add a new Claim field interpreted by Daemon. Rejected because the explicit requirement is zero Daemon/protocol modification.

### 4. Use immutable ToolBundles plus live revocation state as authorization truth

The data model is additive:

```text
tool_source
  id, workspace_id, name, kind, enabled, current_revision,
  created_by, created_at, updated_at

tool_source_revision
  id, workspace_id, source_id, revision, status,
  endpoint, transport_config, artifact_id, secret_id,
  validation_code, created_by, created_at, published_at

tool_definition
  id, workspace_id, source_id, source_revision_id,
  public_name, upstream_name, description,
  input_schema, output_schema, operation_metadata, enabled

tool_bundle
  id, workspace_id, manifest_hash, status,
  created_by, created_at, revoked_at

tool_bundle_item
  id, workspace_id, bundle_id, ordinal, public_name (Agent exported name),
  source_id, source_revision_id, tool_definition_id, artifact_id,
  definition_snapshot (exported and canonical names), invocation_plan, created_at

agent_tool_bundle_head
  id, workspace_id, agent_id, bundle_id,
  updated_by, updated_at

tool_source_artifact
  id, workspace_id, source_id, sha256, media_type,
  size_bytes, content, created_at

tool_source_secret
  id, workspace_id, source_id, envelope, key_id,
  created_at, updated_at
```

`agent_task_queue.tool_bundle_id` is nullable and write-once after Task creation; `task_token.tool_bundle_id` copies the same scope when the `mat_` hash is stored. These columns are Server authorization metadata and do not enter a new Claim/Daemon DTO field.

There are no Foreign Keys or cascades. Queries always carry `workspace_id`; services validate every relationship. A Source publish transaction inserts the validated revision/definitions and atomically advances `tool_source.current_revision`. A Bundle publish transaction copies the selected definitions and kind-specific invocation plans, resolves public-name collisions, records immutable Source revision and Proto artifact digests, inserts all items, then atomically advances the Agent head. Failed publications never expose a partial Bundle.

`tools/list` reads only the Task-pinned Bundle items; a later Agent configuration or Source revision does not rewrite that list. `tools/call` selects only the pinned item's persisted invocation plan, then checks live Task, Agent Gateway access, Bundle revocation, Source/tool emergency disable and deployment policy before connecting. A Bundle ID is an opaque resource name, never a capability; URL possession without the matching active `mat_` and Task pin grants nothing.

Definitions, Bundle manifests and parsed descriptors may be cached per replica by immutable IDs. Cache keys cannot authorize access: every request first resolves Task/Token/Bundle match and live revocation state from the database. Old connection/cache objects close asynchronously after explicit revoke or in-flight completion.

Tool Definition canonical public names use a persisted deterministic namespace derived from a normalized Source slug and stable upstream operation key. Import rejects canonical collisions before publish. Published Definitions and Source revisions cannot be renamed in place.

Bundle publication accepts an `exported_name` for each selected Definition. The exported name defaults to the Definition canonical name, is normalized by trimming surrounding whitespace, and must match the existing MCP-safe `^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$` contract. It must be unique within the complete Bundle. `tool_bundle_item.public_name` stores this exported name so the existing database uniqueness and MCP lookup remain authoritative; `definition_snapshot.name` is also the exported name used by `tools/list`, while `definition_snapshot.canonicalName` preserves the immutable catalog identity for control-plane display and audit. The invocation plan continues to store the fixed upstream name/operation independently from either display name.

An exported-name change participates in the canonical manifest hash and therefore creates a new Bundle ID. The same Definition may use different exported names in different Agent Bundles. A historical Task remains pinned to both the old Definition contract and old exported name. This provides the stable semantic name a Skill can document, such as `skill.kratos-user.get_user`, without adding a Skill-to-Tool relation or argument transformation in this phase.

Alternative: rename Tool Definitions after upload. Rejected because one mutable global rename would affect every Agent and break retained Source revision/Bundle identity.

Alternative: treat the custom name as a browser-only label. Rejected because MCP `tools/list` and `tools/call` must agree on the same name and old Task pins must remain reproducible.

Artifacts use a dedicated bounded byte store rather than the exited generic Attachment model. A Bundle may reference immutable content-addressed Proto/descriptors without copying bytes, but referenced artifacts MUST remain retained for the Bundle retention lifetime. Each index is built by its own concurrent migration and later bound to a constraint where required. Workspace delete, Source delete and Bundle retention use explicit transactional cleanup order.

Alternative: store only a mutable `tool_source` row and replace definitions in place. Rejected because callers could observe mixed revisions and failed validation would destroy the last ready catalog.

Alternative: resolve the Agent's current Source bindings on every MCP request. Rejected because an active or re-Claimed Task would silently change tools after an Agent edit and a Source revision switch.

### 5. Use a shared Server secret primitive with domain-separated associated data

The existing versioned AEAD envelope implementation is extracted behind an internal generic secret primitive while preserving the current `agentconfigsecret` facade and behavior. Tool Source secrets use a separate facade and associated data containing domain, Workspace, Source and revision identity, preventing ciphertext swapping across tenants or uses. The existing production secret key/readiness contract is reused unless key separation can be added without changing operational semantics.

Control Plane DTOs return only `configured`, auth kind, key identifiers and update timestamps. Invocation code decrypts into the smallest scope, applies credentials directly to the outbound client and never attaches them to MCP results, Claim logs or audit data. Audit stores identity, source/tool/revision, outcome, latency, sizes and redacted error code—not raw arguments, results or headers.

Alternative: place upstream credentials in the Agent MCP config. Rejected because it hands enterprise credentials to Agent Runtime and Daemon and bypasses Server policy.

### 6. Centralize egress validation in connect-time dialers

All import fetches and invokers use a shared egress policy and controlled clients. The policy parses endpoints without userinfo, allows only configured schemes/ports, resolves DNS, rejects disallowed address classes, and pins each actual dial to an approved resolved IP while retaining the original hostname for TLS verification. HTTP redirects repeat the full check; environment proxy use is disabled unless an explicit deployment proxy is validated. gRPC uses the same resolver/dial policy through a custom dialer.

Loopback, link-local, multicast, unspecified and cloud metadata targets are always denied. RFC1918/VPC ranges are denied unless present in a deployment-level allowlist; there is no per-Workspace bypass that an Admin can use to pivot through the Server. Validation-time checks improve feedback, but connect-time checks are authoritative against DNS rebinding and changed endpoints.

All clients apply configurable safe defaults for artifact/body/response sizes, connect and total deadlines, redirect count and per-replica Source/Task concurrency. Client cancellation propagates to HTTP, gRPC and upstream MCP calls. Automatic retries are disabled for tool invocation; any future retry must be explicitly safe for a known idempotent operation.

### 7. Normalize all source kinds into immutable invocation plans

Each published `tool_definition` points to an immutable, kind-specific invocation plan:

- **OpenAPI**: parse and resolve bounded local/approved refs at revision validation; map operations to JSON schemas and fixed HTTP request plans. Runtime calls only validate arguments and execute the compiled plan.
- **gRPC**: normalize embedded and uploaded sources to descriptor sets; reject streaming methods; use `dynamicpb`, strict ProtoJSON and a fixed full method name. A replica-local channel manager is keyed by source revision, endpoint, TLS and metadata identity and closes on revision retirement/shutdown.
- **Remote MCP**: validation creates a short-lived upstream client session, negotiates Streamable HTTP and snapshots tools. Each call uses a bounded short-lived client context to invoke one fixed upstream tool and closes it. Only tools are exposed; endpoints requiring persistent cross-call state or client callbacks fail validation.

The registry dispatches by persisted Source kind. Adapters cannot accept a caller-provided endpoint, credential or upstream operation selector.

Build-time descriptors are compiled and registered in the process-owned descriptor registry during Server composition. They are not inserted as tenant-owned database rows at startup because no Workspace, Human actor, or upstream endpoint exists at that boundary. When an authorized Admin stages a gRPC Source with a code-owned descriptor name and fixed endpoint, the Server materializes those registered bytes as a Workspace-scoped immutable artifact and Source revision in the same transaction. Uploaded Proto follows the same artifact, validation, definition, Bundle, and invocation path after upload.

Alternative: generate and compile Go gRPC clients for uploaded Proto. Rejected because runtime code generation/build/restart is unnecessary, increases supply-chain risk and duplicates the descriptor runtime already needed for uploads.

Alternative: keep long-lived upstream MCP sessions across replicas. Rejected for the first delivery because it reintroduces affinity and distributed session storage. Short-lived sessions trade latency for a simpler, stateless trust model; unsupported session-dependent tools are explicit.

### 8. Expose a targeted Web Control Plane and use explicit status transitions

The Human-admin REST surface is registered in the existing route manifest and strict transport map. The initial route families are:

```text
GET/POST   /api/tool-sources
GET/PUT/DELETE /api/tool-sources/{sourceId}
POST       /api/tool-sources/{sourceId}/validate
POST       /api/tool-sources/{sourceId}/enable
POST       /api/tool-sources/{sourceId}/disable
GET        /api/tool-sources/{sourceId}/tools
POST       /api/tool-sources/import
GET        /api/tool-bundles/{bundleId}
GET/DELETE /api/agents/{agentId}/tool-bundle
PUT        /api/agents/{agentId}/tool-bundle
POST       /api/tool-bundles/{bundleId}/revoke
```

Source create/update records a validating revision; validation/publish may run in the request for bounded inputs or through an in-process Server job owned by the same lifecycle. Upload imports use bounded multipart so metadata, artifact, staged revision and validation are committed atomically; this avoids the generic JSON/base64 body limit and interrupted multi-request orphan state. URL sources retain strict JSON contracts. Swagger 2.0 is detected and normalized to OpenAPI 3 inside the Server before the existing compiler path.

Bundle publication accepts a complete normalized `items[{tool_definition_id, exported_name}]` selection, validates same-Workspace/current ready revisions, exported-name syntax/uniqueness and runtime compatibility, creates all immutable items, and advances the Agent head in one transaction. `GET /api/agents/{agentId}/tool-bundle` restores Definition identity plus canonical/exported names without requiring a known Bundle ID; `DELETE` clears only the Agent head and leaves Task pins/retained Bundles untouched. No independent worker is introduced. Every response uses stable redacted error codes such as `tool_source_invalid`, `tool_source_unreachable`, `tool_bundle_invalid`, `tool_bundle_revoked`, `tool_name_conflict`, `provider_mcp_unsupported` and `egress_forbidden` rather than parser/network internals.

The target Web adds Workspace `Tools` list/detail/import routes and Agent `Capabilities > Tools`. `packages/core` owns zod response schemas, API methods and Workspace-scoped React Query keys; `packages/views` owns business pages and dialogs; `apps/web` only wires App Router pages. React Query owns all Source/Definition/Bundle server state. Zustand is not used for persisted selection. Owner/Admin navigation and controls are hidden from Members, while direct routes and every API operation remain server-authorized.

Desktop, Mobile and Docs do not wire these pages. The frontend does not import MCP SDK, OpenAPI/Proto compiler, gRPC client or Gateway packages and never directly contacts upstream endpoints.

## Risks / Trade-offs

- **[Provider advertises MCP differently from static expectations]** → Start with a minimal code-owned allowlist backed by real unchanged-Daemon E3 evidence; validate again at Bundle publication, runtime change, Task creation and Claim; unsupported providers fail closed.
- **[Claim projection takes ownership of a reserved MCP name]** → For Bundle Tasks return exactly one Server-managed `multica` entry, reject attempts to manage that name through stored Agent config paths, and prove the unchanged Daemon still merges separately owned runtime-local entries.
- **[MCP streaming is buffered or errors are rewritten]** → Mount `/bundles/{bundleId}/mcp` outside the REST error group and run transport/conformance tests for status, headers, streaming, cancellation and error bodies.
- **[SSRF or DNS rebinding pivots through Server networks]** → Reuse one connect-time egress dialer across import, HTTP, gRPC and MCP; deny sensitive ranges by default; revalidate redirects and every dial; add real negative tests.
- **[Dynamic schemas consume excessive CPU/memory]** → Bound upload, archive expansion, ref count/depth, descriptor count, generated schema size, request/response size and validation concurrency; never compile in Daemon.
- **[Replica-local caches or pools serve stale authorization]** → Cache only immutable Bundle/revision data, resolve Token/Task/Bundle match and live revocation from PostgreSQL, and close old pools asynchronously.
- **[Agent edits tools while a Task is queued or retried]** → Pin the Bundle at Task creation, copy it into Task Token scope, and reject Claim/request mismatches instead of following the Agent head.
- **[Bundle snapshot is mistaken for an upstream availability guarantee]** → Define immutability only for Server-owned manifest, schemas, revisions, artifacts and invocation plans; use controlled local fixtures and make no external uptime claim.
- **[Short-lived Remote MCP sessions add latency or exclude stateful tools]** → Accept the latency for the first stateless delivery, pool only transport-safe resources, and reject tools requiring cross-call state rather than adding hidden affinity.
- **[A Source update partially publishes definitions]** → Stage artifacts/secrets/revision/definitions and atomically advance `current_revision` only after complete validation.
- **[Secrets leak through validation errors, audit or MCP results]** → Domain-separated encryption, structured redacted errors, audit allowlists and tests that scan database/log/metrics/Realtime/MCP outputs for sentinels.
- **[Browser upload leaves partial Sources or hits JSON/base64 limits]** → Use one bounded multipart import transaction and return only redacted revision/catalog summaries.
- **[UI accidentally becomes a second registry or parser]** → Keep all Source/Bundle server state in React Query, prohibit frontend artifact parsing and require every network response to pass a schema boundary.
- **[Custom names hide the upstream identity or collide]** → Always display exported, canonical and upstream names in their owning contexts; validate syntax and complete-Bundle uniqueness on Server; include both identities in redacted audit without arguments/results.

## Migration Plan

1. Add official Server-only dependencies, the route-group separation, secret primitive and empty gateway lifecycle; verify `cmd/multica` dependency graph and Daemon paths remain unchanged.
2. Apply additive Tool Registry and ToolBundle migrations and generated queries, including Task/Token pin columns, explicit Workspace/Source/Bundle cleanup and concurrent indexes. No existing row is backfilled and no Agent Bundle head exists after migration.
3. Add Control Plane CRUD/validation, atomic multipart artifact import and Agent current-head read/clear with Sources disabled by default. Deploy Server and validate sources without exposing tools to any Agent.
4. Add schema-validated shared client/query code, Workspace Tools list/detail/import routes and Agent `Capabilities > Tools`, with Owner/Admin authorization and browser acceptance coverage.
5. Add immutable Bundle publication, Agent head, Task creation pinning, Task Token scope and `/bundles/{bundleId}/mcp` with a deterministic Server-local tool fixture. Prove new-ID semantics, Claim/re-Claim stability, stateless transport, token lifetime, provider gating and unchanged-Daemon projection.
6. Enable Remote MCP, build-time unary gRPC, OpenAPI/Swagger, then uploaded Proto adapters in that order, with each phase gated by controlled local upstream fixtures and its security matrix; external dependency availability is not a release gate.
7. Enable selected Agent Bundle heads only after their Provider combination passes the unchanged-Daemon E3 matrix. Observe latency, errors, egress denials, pool usage and audit redaction.
8. Run full Fresh DB, Server, target Web build/tests, browser E2E, `make check`, MCP conformance, security negative and real Runtime release gates using controlled local fixtures.

Rollback is data-driven and recoverable: disable Agent Gateway access and revoke active Bundles/Sources, wait for in-flight calls to reach their bounded deadline, close replica-local pools, then deploy the prior Server. The old Server ignores additive tables; no persistent token-bearing config or Daemon migration needs reversal. Schema removal, if ever desired, is a separate destructive change after retention/export review.
