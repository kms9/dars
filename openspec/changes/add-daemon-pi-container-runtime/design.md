## Context

See `proposal.md` for motivation and `specs/` for the observable contract. The current daemon already discovers Pi through `DARS_PI_PATH`, passes `DARS_PI_MODEL` to the Pi backend, registers runtimes through `lightweight-runtime-v1`, receives a 30-day `ddt_`, injects only a task-bound `dat_` as `DARS_TOKEN`, stores Pi sessions under `~/.dars/pi-sessions`, and reports `starting`/`running` on `127.0.0.1:19514/health`.

Three current constraints shape the design:

- `dars daemon start` loads its pairing credential only from `~/.dars/config.json`; an environment-only container cannot start today.
- `/api/daemon/register` accepts Human JWT/PAT and creates a `ddt_`, while `/api/me` deliberately rejects `ddt_`; a container auth probe cannot use `/api/me`.
- Pi 0.84.2 requires Node `>=22.19.0` and exposes `pi auth check --json`, `--list-models`, `--no-session`, `--no-tools`, and JSON print mode, which provide stronger preflight primitives than parsing config files alone.

The authoritative current code has 142 routes and 35 application tables; older 126-route/27-table prose and metadata are stale historical baselines. This change adds three dedicated routes (target 145) and only additive columns on the existing `daemon_token` table (still 35 tables). Desktop is not a target.

## Goals / Non-Goals

**Goals:**

- Produce one Linux image that contains only the DARS CLI/daemon and Pi runtime dependencies and can run from explicit, non-interactive configuration.
- Make the externally supplied credential a real `ddt_` with the existing daemon scope, not a Human PAT hidden behind a misleading variable name.
- Make configuration, Pi readiness, DARS auth, runtime registration, task execution, and result persistence independently observable and testable.
- Reuse the existing runtime protocol and Pi backend rather than creating a Docker-only execution path.

**Non-Goals:**

- No Pi SDK embedding, resident Pi process pool, generic sandbox, Kubernetes operator, image for other providers, or Desktop integration.
- No guarantee that arbitrary untrusted task code is contained beyond the normal Docker boundary.
- No default real-provider call in `make check`; quota-consuming acceptance remains an explicit gate.

## Decisions

### 1. Pre-provision the existing `ddt_` through dedicated Workspace routes

Three Workspace-admin routes manage machine credentials without changing the Human PAT surface:

```http
GET /api/workspaces/{workspaceId}/daemon-tokens
POST /api/workspaces/{workspaceId}/daemon-tokens
DELETE /api/workspaces/{workspaceId}/daemon-tokens/{tokenId}
```

`boundaryWorkspaceAdmin` makes the current Workspace owner/admin authoritative. POST accepts `{daemon_id,name,expires_at}`; repeating a Workspace/daemon pair is explicit rotation, returns plaintext once, and marks matching runtimes offline without deleting their rows. DELETE revokes by token ID and also marks matching runtimes offline. The default TTL is 90 days and an explicit expiry may not exceed 365 days.

The `daemon_token` table remains the authority and keeps its existing unique `(workspace_id, daemon_id)` invariant. Additive nullable columns record `user_id`, `name`, and `token_prefix`. A migration backfills `user_id` only when all matching runtimes identify one unique owner; orphan or ambiguous rows remain null and cannot use direct `ddt_` Register until a current admin rotates them. No foreign key, table, or new index is required. Route contracts move from the live 142-route baseline to 145.

`/api/daemon/register` receives a dedicated register-auth middleware:

```text
Human JWT/PAT ── register ──> upsert runtime + rotate/return ddt_

pre-provisioned ddt_
       │
       ├── scope == request workspace_id + daemon_id
       └── register ────────> upsert runtime + keep current ddt_
```

On the `ddt_` path, the token row's non-null `user_id` supplies the runtime owner, the handler does not call `CreateDaemonToken`, and the response omits a replacement token. The daemon client already retains its current token when a register response contains no new token. All later endpoints continue through the existing `DaemonAuth` scope.

`ddt_` is a Workspace machine credential, not continuing delegation from its issuing Human. It remains valid if that Human leaves. Current admins can list, rotate, or revoke it; rotation changes `user_id` to the current actor and becomes the explicit ownership handoff. Register does not re-check the old issuer's membership.

Graceful Deregister changes from "offline and delete token" to "offline only". Credential invalidation is limited to explicit rotate/revoke, Workspace deletion, and expiry. This is required for a clean SIGTERM followed by same-token container restart.

Alternatives rejected:

- Supplying a `dpat_` as `DARS_DAEMON_TOKEN` would make unattended startup easy but would place Human-wide authority on every node and contradict the variable and security model.
- A new one-time `den_` enrollment token would need a second persistent credential lifecycle or an additional token/table kind, while the existing scoped `ddt_` already has the required authority.
- Reusing `/api/tokens` would mix user-global PAT and Workspace machine authorization, break its current DTO assumptions, and was only attractive under the stale 126-route premise.

### 2. Resolve daemon auth from a fail-closed secret input without materializing it to config

One shared resolver will enforce:

```text
DARS_DAEMON_TOKEN_FILE contents (must be ddt_)
        >
DARS_DAEMON_TOKEN (must be ddt_)
        >
profile config token (existing Human PAT/JWT local flow)
        >
startup error
```

`DARS_DAEMON_TOKEN_FILE` is authoritative whenever set: missing/unreadable/empty/non-`ddt_` content fails immediately and never falls back. `requireDaemonAuth`, foreground `resolveAuth`, and a new non-mutating daemon auth-preflight command use the same resolver. Container startup requires file/environment `ddt_`; profile PAT/JWT fallback remains only for the existing local daemon flow. `DARS_WORKSPACE_ID` similarly overrides the profile config Workspace ID, while the existing `DARS_DAEMON_ID`, device name, runtime name, max concurrency, and workspaces-root environment paths remain authoritative.

The auth-preflight command calls `GET /api/daemon/workspaces` with the `ddt_` and verifies that the single returned Workspace matches `DARS_WORKSPACE_ID`; it does not call `/api/me` and does not register a Runtime. Register remains part of daemon startup so there is one owner of runtime lifecycle.

Calling `Client.SetToken` with a `ddt_` clears any stale Human `pairingToken`; Register then uses the current daemon token, so an old mounted config PAT cannot silently rotate the authoritative file/environment token.

The generic task child-environment merge already removes every inherited `DARS_*` value before overlaying task-specific values. It does not remove Provider variables because environment-supplied Provider credentials are an explicit supported Pi path. The ACP model-discovery path currently appends `os.Environ()` directly and will be changed to the same sanitized base. Tests cover both Pi task launch and ACP discovery.

### 3. Use a Debian Node 22 runtime image with an exact Pi package version

The build uses a Go 1.26.1 builder for a static `dars` binary and an exact Node 22 patch release satisfying Pi's `>=22.19.0` engine requirement on Debian slim. Debian avoids Alpine/musl surprises in npm extensions and supplies the required execution utilities from standard packages. The runtime installs `@earendil-works/pi-coding-agent@0.84.2` through an overridable `PI_VERSION` build argument with lifecycle scripts disabled, asserts Node and Pi versions during build, cleans package-manager caches, and records DARS/Pi versions as OCI labels.

The image creates an unprivileged `dars` user and these owned paths:

```text
/config/pi          native PI_CODING_AGENT_DIR mount
/home/dars/.dars    daemon identity, logs, runtime state, Pi sessions
/workspaces         repo cache and task workspaces
```

The image accepts build-time UID/GID inputs so a non-root user can read a mode-0600 host Pi config and write its owned state volumes. `tini` is the image entrypoint. The shell entrypoint finishes with `exec dars daemon start --foreground`, so `tini` only reaps children and forwards signals.

Alternatives rejected:

- Installing latest Pi makes an old checkout build different behavior on different days.
- Copying the host Pi installation or `/Users/logo/.pi` into the image leaks credentials and is architecture-dependent.
- Alpine is smaller but provides no material acceptance benefit and increases compatibility risk for user Pi extensions.

### 4. Make Pi basic and active preflight use Pi's public CLI

Model resolution is `DARS_PI_MODEL` first, otherwise `defaultProvider/defaultModel` from `$PI_CODING_AGENT_DIR/settings.json`. The resulting `provider/model` must identify exactly one row in the full Pi `--list-models` runtime. For built-in providers, basic mode runs `pi auth check --model <provider/model> --json` and accepts only `status:"ready"`; JSON is parsed without `--credentials` and logs only provider/model/auth type. A read-only config adds `--no-refresh`; an expired OAuth config therefore fails with an actionable request for a writable mount. Because Pi's auth-check runtime does not load extension/custom catalogs, a custom provider in basic mode returns a diagnostic requiring active mode instead of a false credential failure.

Active mode runs after basic in a freshly created temporary directory:

```text
pi --model <provider/model>
   --no-session --no-tools --no-context-files
   --mode json -p <fixed low-token probe>
```

It keeps extension discovery enabled because a mounted config may supply a legitimate custom provider/model through an extension. The probe has closed stdin, no tools, session, project context, or persistent workspace, and must finish its JSON event stream with the sentinel `DARS_PI_PREFLIGHT_OK`. `disabled` is accepted only when explicitly set.

### 5. Derive Docker readiness from the daemon's registered Pi runtime

The daemon health response gains an additive, non-secret mapping from runtime ID to provider for each Workspace. Existing fields and `starting`/`running` semantics remain compatible. A small `dars daemon container-health` command queries the loopback health endpoint and succeeds only when:

- status is `running`;
- the configured Workspace exists;
- at least one registered runtime in that Workspace has provider `pi`.

The provider mapping is an additive field; existing `workspaces[].runtimes []string` remains unchanged. The Docker `HEALTHCHECK` invokes this command and uses a `start_period` sized for active preflight plus first registration. It does not require curl, expose the health port, or duplicate registration logic in the entrypoint.

### 6. Keep Compose inputs explicit and state boundaries separate

`docker-compose.daemon-pi.yml` builds the image, requires `DARS_SERVER_URL`, `DARS_WORKSPACE_ID`, `DARS_DAEMON_ID`, one Daemon Token secret input, and `DARS_PI_CONFIG_DIR`, mounts the Pi directory at `/config/pi`, and uses separate named volumes for `/home/dars/.dars` and `/workspaces`. The example includes `host.docker.internal:host-gateway` so a locally hosted DARS Server works on Linux and Docker Desktop, but a normal HTTP(S) remote URL needs no special network mode. The preferred production example uses `DARS_DAEMON_TOKEN_FILE`; the environment variable remains supported for the required simple contract.

The Pi config mount is read-write by default because OAuth refresh can update `auth.json`; documentation calls out UID/GID handling and a read-only option only for non-refreshing API-key configurations. Entrypoint diagnostics include the container UID and failing path without reading Secret contents. Neither the whole host HOME nor Docker socket is mounted.

### 7. Split deterministic tests from the explicitly authorized real smoke

Default tests cover token management/scope, register auth branches, environment resolution, credential stripping, health semantics, entrypoint stage failures, and image contents with fake controlled executables. They never discover or execute a user-installed Pi.

The real script is gated by `DARS_RUN_REAL_PI_CONTAINER_SMOKE=1`. It requires an explicit Human control credential and Pi config path, provisions a unique daemon ID/`ddt_`, builds the candidate image, and starts it against an isolated DARS database/server with an empty-repos Workspace. The Agent is bound to the Pi runtime/model and has no ToolBundle. The harness requires a tool-result containing `DARS_PI_E2E_OK`, exactly one completed terminal task row, and a final persisted output equal to `DARS_PI_E2E_OK` after the Server's TrimSpace/redaction normalization. It then recreates the container with the same volumes, runs a second task, tests Server disconnect/reconnect, and tests failure-then-success. A permission-adjusted working copy of the user's Pi config is mode 0700, deleted after the run, and never included in evidence. Evidence is redacted JSON with image digest and version metadata.

## Risks / Trade-offs

- [A `ddt_` passed as an environment variable is visible to Docker administrators and process inspectors] → Prefer the fail-closed token-file input in production while retaining the required environment contract; never copy or log the value.
- [Three new API routes change the active router contract] → Update the current 142-route list/metadata/spec to exactly 145 and keep PAT `/api/tokens` byte-for-byte compatible.
- [Backfilling `user_id` for old Daemon Tokens can be ambiguous] → Backfill only one unique matching runtime owner, keep unresolved rows nullable, reject their direct `ddt_` Register, and let a current admin rotate them.
- [The one-token-per-daemon unique constraint means rotation immediately disconnects the old container] → Treat POST for an existing pair as explicit rotation, label it clearly, and verify old-token rejection/new-token success.
- [Graceful stop no longer revokes a machine credential] → Make decommissioning an explicit admin revoke/rotate operation; Deregister only marks runtime offline so ordinary restarts work.
- [The issuing Human may leave the Workspace] → Treat `ddt_` as Workspace-owned; current admins manage it and explicit rotation transfers the audit/runtime owner seed to the current actor.
- [A finite machine token expires] → Default to 90 days, cap at 365 days, surface expiry metadata, and document explicit rotation; never imply permanent unattended validity.
- [Active preflight consumes quota and adds cold-start latency] → Keep `basic` as the default, require `active` in real acceptance, use a fixed low-token no-tool probe, and report mode in health evidence.
- [Pi config permissions differ across macOS/Linux bind mounts] → Run an entrypoint writability check with an actionable UID/path error and offer a managed volume/import workflow in documentation.
- [Pi auth check does not load extension providers] → Require active mode for extension/custom providers after full catalog resolution instead of treating provider-not-found as bad credentials.
- [Provider extensions can execute during active preflight] → Run in an empty temporary workdir with tools/context/session disabled; this change trusts the explicitly mounted Pi config but no repository content.
- [A remote Server may use a private CA] → Rely on system CA by default and document an optional CA mount rather than disabling TLS verification.

## Migration Plan

1. Add nullable `daemon_token.user_id/name/token_prefix` columns and conservative backfill in reversible migrations, regenerate sqlc, and update the 35-table column contracts.
2. Add three Workspace-admin Daemon Token routes, update the router target from 142 to 145, and implement rotate/revoke-offline semantics while leaving PAT routes unchanged.
3. Add Human-or-matching-Daemon Register, change Deregister to offline-only, and deploy Server before distributing the image.
4. Add the fail-closed file/environment credential resolver, stale pairing-token clearing, Workspace env override, ACP environment sanitation, auth/health probe commands, and tests.
5. Add the pinned image, entrypoint, Compose file, deterministic container tests, built-in runtime-protocol skill update, documentation, and gated real smoke.
6. Run standard checks, isolated Docker negative tests, then the authorized real Pi cold-start/restart/reconnect/failure-recovery E2E and retain the evidence.

Rollback removes the image distribution and CLI environment path first, then reverts Server register/token behavior. The additive token metadata columns can remain harmlessly during application rollback; their down migrations are used only when intentionally rolling the schema back after all pre-provisioned container nodes are stopped or re-paired through the old Human flow.
