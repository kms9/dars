---
name: dars-lightweight-resources
description: "Use when managing Lightweight Runtime Profiles, local Runtimes, Agents, encrypted Agent config, Skills (including URL/archive import and ClawHub search), or Agent-Skill bindings. Not for cloud runtimes, builders, templates, usage dashboards, or self-update."
user-invocable: false
allowed-tools: Bash(dars *)
---

# Lightweight resources

Follow the dependency order: Runtime Profile → registered local Runtime → Agent
→ Skill bindings → Run, Chat, or Squad.

## Runtime and profile

```bash
dars runtime profile list
dars runtime profile create --display-name "..." --protocol-family codex --command-name codex
dars runtime list
dars runtime rename <runtime-id> "Local Codex"
```

The Daemon registers local runtimes. A profile cannot be deleted while a
runtime derives from it. Runtime deletion is rejected while Agents are bound;
an explicit `--expected-active-agent-ids` set is required to confirm and unbind
the exact current set.

## Agent and secrets

```bash
dars agent create --name "..." --runtime-id <runtime-id>
dars agent env get <agent-id>
dars agent env set <agent-id> --custom-env-stdin
dars agent skills set <agent-id> --skills-json '[{"skill_id":"...","enabled":true}]'
```

List/get responses expose only secret keys. Env reveal and replace are audited;
prefer stdin so plaintext does not enter shell history. MCP config is encrypted
as a whole and must be supplied through the strict Agent mutation body. Never
put secrets in comments, task messages, logs, skill files, or Run context.

Agent invocation is `private` or `public_to` with explicit workspace/member
targets. Archive and restore preserve history; active tasks block archive.

## Skills

Skills are workspace-owned text plus an atomically replaced file set. Paths are
relative. `dars skill files replace` replaces the complete set; inspect it
first and include every file that must remain. A bound Skill cannot be deleted.

Create manually, import from a public URL, or upload a local archive:

```bash
dars skill create --name "..." --content $'---\nname: ...\n---\n'
dars skill import --url https://clawhub.ai/owner/slug --on-conflict fail
dars skill import --file ./review-helper.skill --on-conflict rename
dars skill search "code review"
```

`--on-conflict` accepts `fail`, `overwrite`, `rename`, or `skip`. Overwrite is
limited to the skill creator. URL hosts are limited to ClawHub, Skills.sh, and
GitHub. Runtime-local copy remains available in the Web Skills page for Runtime
Owners.

