---
name: dars-squad-collaboration
description: "Use when a Run is assigned to a Squad, a Leader delegates to member Agents, or member results must be reconciled. Not for member assignees, nested squads, or autonomous self-trigger loops."
user-invocable: false
allowed-tools: Bash(dars *)
---

# Squad collaboration

A Squad contains Agents only and has exactly one Leader. The Leader coordinates
the Run; members execute bounded delegated work. Humans can inspect live status
with `dars squad member status <squad-id>`.

## Leader protocol

1. Read the Run and flat comments. Use only the authorized Squad roster in the
   current Task prompt; a Task Token cannot enumerate Squad or Task Run APIs.
2. Decide whether delegation is useful. Record exactly one immutable decision:
   the allowed outcomes are `action`, `no_action`, and `failed`.

```bash
dars squad evaluate <run-id> --outcome action --reason "delegating independent checks"
dars squad evaluate <run-id> --outcome no_action --reason "single-agent work"
dars squad evaluate <run-id> --outcome failed --reason "required member unavailable"
```

This command is Task-Token-only. The server derives the Leader, Task, Squad,
Agent, and Workspace; never include forged attribution.

3. Delegate by appending a Run comment with canonical Agent mentions. Give each
   member a bounded request and the evidence expected in its reply.
4. Re-read comments after wake-up. Member results wake the Leader
   and may be coalesced while it is already running.
5. Synthesize the final result, then move the Run to `in_review` or the correct
   terminal/blocking state. Return the synthesis as final output; Task completion
   persists that output once as the attributed result Comment, so do not post it
   manually a second time.

Members return their evidence as final output. Task completion persists that
output once as their result Comment and triggers reconciliation; members must
not manually post the same result before exiting.

## Loop and loss guards

- A Leader mention of itself must not enqueue another Leader task.
- A member must not mention the Leader merely to return normal results; the
  reconciliation path performs the wake-up.
- Repeated or delayed result events may be delivered more than once. Read
  durable comments before acting and avoid repeating an already recorded
  delegation.
- A running Leader can receive supplemental comments. If they cannot be added
  to the active task, the server creates one follow-up rather than losing them.
