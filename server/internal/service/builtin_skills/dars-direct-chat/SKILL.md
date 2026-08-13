---
name: dars-direct-chat
description: "Use when operating a Lightweight Direct Chat session, sending a message, recovering a pending task, cancelling it, or consuming a draft restore. Not for channels, threads, attachments, pins, unread state, or project context."
user-invocable: false
allowed-tools: Bash(dars *)
---

# Direct Chat

Direct Chat is a human-owned session with one Agent and one Runtime. It does not
depend on a Run.

```bash
dars chat list
dars chat create --agent-id <agent-id> --title "..."
dars chat messages list <session-id>
dars chat messages send <session-id> --content "..."
dars chat pending-task <session-id>
dars chat cancel-task <task-id>
dars chat draft-restores list <session-id>
dars chat draft-restores delete <session-id> <restore-id>
```

A send is plain text, creates exactly one user Message and one Chat Task, and
uses an idempotency key. Runtime progress is transient; durable completion is
an assistant, failure, or no-response Message. After reconnect or refresh,
query `pending-task` and refetch messages instead of relying on replayed WebSocket
events.

The commands above are Human operations. Inside the claimed Chat Task, the
Task Token may read only that session's messages; return the assistant answer as
the runtime's final output. Do not call `messages send` to manufacture the
assistant response and never fall back to a Human PAT.

Archive with `dars chat update <session-id> --status archived`. Active tasks
block archive and delete. Deletion is allowed only after archive. Draft restores
are server-created recovery records; consume one only after the recovered text
has been copied back into the user's draft.
