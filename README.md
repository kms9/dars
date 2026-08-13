<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/logo-light.svg">
  <img alt="DARS" src="docs/assets/logo-light.svg" width="50">
</picture>

# DARS

**Distributed Agent Runtime System.**

An open-source control plane and distributed runtime for coding agents.<br/>
Route work across machines, execute with local agent CLIs, and keep every run observable.

[![CI](https://github.com/kms9/dars/actions/workflows/ci.yml/badge.svg)](https://github.com/kms9/dars/actions/workflows/ci.yml)
[![GitHub stars](https://img.shields.io/github/stars/kms9/dars?style=flat)](https://github.com/kms9/dars/stargazers)
[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/W8gYBn226t)

[Website](https://dars.ai) · [Docs](https://dars.ai/docs/environment-variables#github-integration) · [Discord](https://discord.gg/W8gYBn226t) · [X](https://x.com/DARSAI) · [Self-Hosting](SELF_HOSTING.md) · [Contributing](CONTRIBUTING.md)

**English | [简体中文](README.zh-CN.md)**

</div>

## What is DARS?

DARS is a **Distributed Agent Runtime System**. Its Web control plane and Go server coordinate local Daemon/CLI workers, route runs to the right agent runtime, and stream execution progress back to one observable system.

Each task gets explicit runtime configuration, scoped credentials, an isolated work directory, durable events, and a recorded result. DARS is vendor-neutral, self-hosted infrastructure for human + AI teams. It works with **Claude Code**, **Codex**, **CodeBuddy**, **GitHub Copilot CLI**, **OpenCode**, **OpenClaw**, **Hermes**, **Pi**, **Cursor Agent**, **Kimi**, **Kiro CLI**, **Antigravity**, **Qoder CLI**, and **Trae CLI**.

For larger teams, Squads add a stable routing layer: assign work to a group led by an agent, and the leader delegates to the right member.

## Why "DARS"?

DARS stands for **Distributed Agent Runtime System**:

- **Distributed** — the control plane and execution workers can run on different machines.
- **Agent Runtime** — provider CLIs execute through explicit runtimes, workspaces, credentials, and lifecycle controls.
- **System** — routing, execution, events, collaboration, skills, and evidence share one durable model.

## Features

DARS manages the full distributed agent lifecycle: from task assignment and runtime routing to execution monitoring and skill reuse.

- **Agents as Teammates** — create agents with explicit runtime, model, skills, environment, and invocation permissions.
- **Squads** — group agents under a leader agent and assign work to the squad. The leader delegates through comments and mentions.
- **Autonomous Execution** — set it and forget it. Full task lifecycle management (enqueue, claim, start, complete/fail) with real-time progress streaming via WebSocket.
- **Direct Chat and Runs** — talk to an agent directly, or create a durable run with separate comments and task execution history.
- **Reusable Skills** — every solution becomes a reusable skill for the whole team. Deployments, migrations, code reviews — skills compound your team's capabilities over time.
- **Local Runtimes** — connect local daemons, auto-detect installed agent CLIs, and monitor runtime readiness.
- **Multi-Workspace** — isolate agents, squads, skills, chats, and runs by workspace.

---

## Quick Install

<details open>
<summary><b>macOS / Linux</b></summary>

<br/>

### Homebrew (recommended)

```bash
brew install kms9/tap/dars
```

Use `brew upgrade kms9/tap/dars` to keep the CLI current.

### Install script

```bash
curl -fsSL https://raw.githubusercontent.com/kms9/dars/main/scripts/install.sh | bash
```

Use this if Homebrew is not available. The script installs the DARS CLI on macOS and Linux by using Homebrew when it is on `PATH`, otherwise it downloads the binary directly.

Then configure your self-hosted server, authenticate, and start the daemon:

```bash
dars setup self-host
```

> To deploy the server on your machine, add `--with-server` when installing:
>
> ```bash
> curl -fsSL https://raw.githubusercontent.com/kms9/dars/main/scripts/install.sh | bash -s -- --with-server
> dars setup self-host
> ```
>
> This pulls the official DARS images from GHCR (latest stable by default). Requires Docker. See the [Self-Hosting Guide](SELF_HOSTING.md) for details.
> If the selected GHCR tag has not been published yet, fall back to `make selfhost-build` from a checkout.

</details>

<details>
<summary><b>Windows (PowerShell)</b></summary>

<br/>

### PowerShell

```powershell
irm https://raw.githubusercontent.com/kms9/dars/main/scripts/install.ps1 | iex
```

Then configure your self-hosted server, authenticate, and start the daemon:

```powershell
dars setup self-host
```

> **Self-hosting?** Set the `DARS_MODE` environment variable to `with-server` before running the installer to deploy a full DARS server on your machine:
>
> ```powershell
> $env:DARS_MODE="with-server"; irm https://raw.githubusercontent.com/kms9/dars/main/scripts/install.ps1 | iex
> dars setup self-host
> ```
>
> This pulls the official DARS images from GHCR (latest stable by default). Requires Docker. See the [Self-Hosting Guide](SELF_HOSTING.md) for details.

</details>

---

## Getting Started

### 1. Set up and start the daemon

```bash
dars setup self-host # Configure, authenticate, and start the daemon
```

The daemon runs in the background and auto-detects agent CLIs (`claude`, `codex`, `codebuddy`, `copilot`, `opencode`, `openclaw`, `hermes`, `pi`, `cursor-agent`, `kimi`, `kiro-cli`, `agy`, `qodercli`, `qoderclicn`, `traecli`) on your PATH.

### 2. Verify your runtime

Open your workspace in the DARS web app and navigate to **Runtimes**. Your machine should appear as an active runtime.

> **What is a Runtime?** A Runtime is a local compute environment connected through the daemon. It reports which agent CLIs are available so DARS knows where to route work.

### 3. Create an agent

Go to **Agents** and click **New Agent**. Pick the runtime you just connected, choose a provider, and configure the agent.

### 4. Assign your first task

Create a run from **Runs** (or via `dars issue create`) and assign it to an agent or squad. The assignee executes it on your runtime and reports progress through comments and task events.

---

## CLI

The `dars` CLI connects your local machine to DARS — authenticate, manage workspaces, and run the agent daemon.

| Command | Description |
|---------|-------------|
| `dars login` | Authenticate with an email code or PAT |
| `dars daemon start` | Start the local agent runtime |
| `dars daemon status` | Check daemon status |
| `dars setup self-host` | Configure a self-hosted Lightweight runtime |
| `dars workspace list` | List your workspaces (current is marked with `*`) |
| `dars workspace switch <id\|slug>` | Switch the default workspace for this profile |
| `dars agent list` | List agents |
| `dars issue list` | List runs |
| `dars issue create` | Create a run |

See the [CLI and Daemon Guide](CLI_AND_DAEMON.md) for the full command reference.

---

## Architecture

```
┌──────────────┐     ┌──────────────┐     ┌──────────────────┐
│   Next.js    │────>│  Go Backend  │────>│   PostgreSQL     │
│   Frontend   │<────│  (Chi + WS)  │<────│   (pgvector)     │
└──────────────┘     └──────┬───────┘     └──────────────────┘
                            │
                     ┌──────┴───────┐
                     │ Agent Daemon │  runs on your machine
                     └──────────────┘  (Claude Code, Codex, CodeBuddy, GitHub Copilot CLI,
                                        OpenCode, OpenClaw, Hermes, Pi, Cursor Agent,
                                        Kimi, Kiro CLI, Antigravity, Qoder CLI, Trae CLI)
```

| Layer | Stack |
|-------|-------|
| Frontend | Next.js 16 (App Router) |
| Backend | Go (Chi router, sqlc, gorilla/websocket) |
| Database | PostgreSQL 17 with pgvector |
| Agent Runtime | Local daemon executing Claude Code, Codex, CodeBuddy, GitHub Copilot CLI, OpenCode, OpenClaw, Hermes, Pi, Cursor Agent, Kimi, Kiro CLI, Antigravity, Qoder CLI, or Trae CLI |

## Development

For contributors working on the DARS codebase, see the [Contributing Guide](CONTRIBUTING.md).

**Prerequisites:** [Node.js](https://nodejs.org/) v20+, [pnpm](https://pnpm.io/) v10.28+, [Go](https://go.dev/) v1.26+, [Docker](https://www.docker.com/)

```bash
make dev
```

`make dev` auto-detects your environment (main checkout or worktree), creates the env file, installs dependencies, sets up the database, runs migrations, and starts all services.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development workflow, worktree support, testing, and troubleshooting.

An iOS mobile client lives in [`apps/mobile/`](apps/mobile/) — see its [README](apps/mobile/README.md) for how to build it onto your own iPhone.


## License

[Multica License](LICENSE) — the complete Apache License 2.0 text incorporated together with additional conditions — see [NOTICE](NOTICE) for attribution notices.

- Providing Multica as a hosted service to third parties, or embedding it in a commercially distributed product, requires a commercial license obtained from the producer (condition 1a).
- Unless the producer has granted a written branding waiver, the Multica LOGO, product name, and copyright information may not be removed or modified in a Multica user interface. The user interface is defined by derivation — including `apps/web/`, `apps/desktop/`, `apps/mobile/`, `packages/views/`, and `packages/ui/` — and covers raw source, the frontend container image, and compiled desktop and mobile binaries (condition 1b).
- Non-interface use (running only the `server/` backend, the daemon, or the CLI) is exempt from the branding condition, but must retain the source and [NOTICE](NOTICE) attribution and state that the product is built on Multica, with a link back to this repository (condition 1c).
- A branding waiver and a commercial license are separate grants; neither implies the other (condition 1d).
