---
name: dars-working-on-runs
description: "Use when reading, creating, updating, commenting on, or completing a Lightweight Run. Not for projects, boards, labels, properties, attachments, child issues, subscribers, reactions, or mixed timelines."
user-invocable: false
allowed-tools: Bash(dars *)
---

# Working on Lightweight Runs

A Run is the target Issue model. Its durable fields are title, description,
status, assignee, acceptance criteria, and context references. The valid status
set is `backlog`, `todo`, `in_progress`, `in_review`, `done`, `blocked`, and
`cancelled`.

## Read before writing

```bash
dars issue get <run-id>
dars issue comment list <run-id>
```

Comments and Task Runs are separate flat collections. Do not reconstruct an old
mixed timeline or issue hierarchy. A Human PAT may also use `dars issue runs
<run-id>` and `dars issue run-messages <task-id>`. A Task Token cannot list
Task Run history; a 403 is the intended boundary and must not be bypassed.

## Mutations

```bash
dars issue create --title "..." --status todo --assignee-type agent --assignee-id <agent-id>
dars issue update <run-id> --description "..."
dars issue status <run-id> in_review
dars issue comment add <run-id> --content-file ./reply.md
dars issue cancel-task <task-id>
```

Run create, title/description edits, history reads, and task cancellation are
Human operations. A Task Token is limited to its own Run: read it and its flat
comments, append a comment, and update only status or assignee. Never replace a
rejected Task Token with a Human PAT.

`backlog` is parked work. The transition to `todo` performs the one-time
dispatch decision. An assignee is always exactly one Agent or Squad; Members are
authors and operators, not Run assignees.

Use manual comments for delegation or immediate mid-task feedback. A non-empty
final assistant output is persisted exactly once as the attributed result
Comment when Task completion commits; do not manually post the same content.
Comments created by an agent are attributed from its Task Token; do not send
actor, workspace, source-task, or author fields. Mention targets use canonical links:
`[Agent name](mention://agent/<uuid>)` or
`[Squad name](mention://squad/<uuid>)`.

Before moving to `done`, verify the acceptance criteria and leave evidence in a
comment. Use `blocked` when progress needs external input; use `cancelled` only
when the work should stop permanently.
