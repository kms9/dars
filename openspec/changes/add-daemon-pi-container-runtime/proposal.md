## Why

DARS already executes Pi tasks through the local daemon, but it has no reproducible, unattended Linux runtime-node artifact. Operators therefore cannot start a Pi execution node from a server URL, credential, and existing Pi configuration and prove that DARS can discover it, dispatch work, and persist the returned result.

## What Changes

- Add a dedicated, multi-stage `Dockerfile.daemon-pi` that builds the DARS CLI and installs a pinned Pi CLI plus the minimum execution tools, runs as a non-root user, and uses `tini` for signal forwarding.
- Add a container entrypoint and Compose example that validate configuration and writable state, perform Pi basic or active preflight, then `exec dars daemon start --foreground` without an interactive login.
- Add Workspace-admin Daemon Token management on a dedicated API/CLI surface, allow a matching pre-provisioned `ddt_` to register without rotation, and resolve it non-interactively from a fail-closed secret file/environment input while preserving task-scoped `DARS_TOKEN` isolation.
- Reuse the daemon's local `/health` readiness contract so Docker becomes healthy only after initial pairing, runtime registration, and heartbeat-ready state complete.
- Support a mounted native Pi config such as `/Users/logo/.pi/agent`, a separately persisted DARS state directory, and a separately persisted task-workspace root.
- Add deterministic negative tests and a repeatable real-process smoke harness that builds the image, starts the container, observes the Pi runtime in DARS, dispatches a Pi task, and verifies the persisted terminal result `DARS_PI_E2E_OK`.
- Update the source PRD and operator documentation with the exact credential semantics, Pi mount path, image version-pinning policy, startup stages, failure diagnostics, and reproducible acceptance commands.

## Capabilities

### New Capabilities

- `daemon-pi-container-runtime`: Defines the image contents, startup/configuration contract, Pi preflight, readiness, persistent volumes, failure behavior, and black-box dispatch/result acceptance for a containerized Pi runtime node.

### Modified Capabilities

- `lightweight-api-security`: Clarifies unattended daemon bootstrap credential resolution and guarantees that bootstrap/Daemon credentials remain outside task processes while task-scoped `dat_` credentials retain their current authority.
- `lightweight-runtime-release`: Adds the daemon-pi image and its real container-to-Server-to-Pi-to-Server smoke evidence to the runtime release gate.

## Impact

- Affects the DARS CLI/daemon authentication bootstrap, three Workspace-scoped API routes, additive `daemon_token` metadata columns, daemon health/readiness consumption, Pi runtime probing, Docker build/runtime assets, Compose examples, documentation, and acceptance tooling.
- Preserves the current `lightweight-runtime-v1` registration, claim, heartbeat, task lifecycle, and Pi JSON-event protocol; no Desktop wiring is introduced.
- Updates the authoritative live contract from 142 to 145 routes and retains the current 35-table schema; no new table, foreign key, or compatibility route is introduced.
- Introduces a pinned Node/Pi supply-chain dependency in the runtime image and a real-agent acceptance path that can consume provider quota only when explicitly enabled.
