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
- Daemon registration returns an `ddt_` token bound to one workspace and
  daemon. Heartbeats, claim, Daemon WebSocket, and result delivery use it.
- A claim returns a short-lived `dat_` Task Token. In a task process the daemon
  injects that token with `DARS_WORKSPACE_ID`, `DARS_AGENT_ID`, and
  `DARS_TASK_ID`. The CLI forwards the corresponding trusted
  `X-Workspace-ID`, `X-Agent-ID`, and `X-Task-ID` headers.
- Never replace a missing or rejected Task Token with a user PAT. Do not copy a
  task token into another task, workspace, terminal, or log.

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
