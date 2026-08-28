---
name: dars-runtime-protocol
description: "Use when a DARS task must reason about its Daemon, Task Token, claim lifecycle, workspace boundary, or retry behavior. Not for changing server configuration or inventing compatibility fallbacks."
user-invocable: false
allowed-tools: Bash(dars *)
---

# Lightweight runtime protocol

The only supported protocol is `lightweight-runtime-v1`. Never retry through an
older claim URL or change the credential type after an authorization failure.

## Credential boundary

- Human operations use a `dpat_` PAT or the Web session.
- A workspace owner/admin can pre-provision one `ddt_` Daemon Token for a
  stable daemon ID. Local Human-authenticated registration can also rotate and
  return that token. A matching `ddt_` registration reuses the token and never
  returns its plaintext. Heartbeats, claim, Daemon WebSocket, and result
  delivery use it.
- Container credentials resolve in the strict order
  `DARS_DAEMON_TOKEN_FILE` → `DARS_DAEMON_TOKEN`. If the file variable is set,
  an absent, unreadable, empty, or non-`ddt_` file fails closed; containers do
  not fall back to a Human profile. `DARS_WORKSPACE_ID` must match the token's
  server-side workspace scope.
- A claim returns a short-lived `dat_` Task Token. In a task process the daemon
  injects that token with `DARS_WORKSPACE_ID`, `DARS_AGENT_ID`, and
  `DARS_TASK_ID`. The CLI forwards the corresponding trusted
  `X-Workspace-ID`, `X-Agent-ID`, and `X-Task-ID` headers.
- Never replace a missing or rejected Task Token with a user PAT. Do not copy a
  task token into another task, workspace, terminal, or log.
- Never expose a Daemon Token or Human PAT to Pi, ACP discovery, or a task
  process. Those child environments remove inherited `DARS_*` variables and
  receive only the task-scoped `dat_` values required by the protocol.

Task Tokens expose only the current Run or Direct Chat context, append-only
comments/messages, task status, and the Leader evaluation endpoint. A 403 or
404 is a security boundary, not a signal to enumerate adjacent resources.

## Delivery and retry

WebSocket messages are hints. Durable state is PostgreSQL; reconnect performs
HTTP refetch. A Daemon claims only through the batch claim endpoint, and
duplicate hints must not produce duplicate work.

Run creation, comment creation, and Direct Chat sends require an
`Idempotency-Key`. The CLI generates one when omitted. For a manual retry of the
same logical write, reuse the original key and the identical body; changing the
body under the same key is a conflict.

Terminal task results are idempotent and cannot move backward. Cancellation is
always `dars issue cancel-task <task-id>` or
`dars chat cancel-task <task-id>`; both call the single canonical task
cancel endpoint.
