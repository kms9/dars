<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/logo-light.svg">
  <img alt="DARS" src="docs/assets/logo-light.svg" width="50">
</picture>

# DARS

**分布式 Agent Runtime 系统。**

面向编码 Agent 的开源控制面与分布式运行时。<br/>
跨机器路由工作，通过本地 Agent CLI 执行，并让每次运行都有可观测记录。

[![CI](https://github.com/kms9/dars/actions/workflows/ci.yml/badge.svg)](https://github.com/kms9/dars/actions/workflows/ci.yml)
[![GitHub stars](https://img.shields.io/github/stars/kms9/dars?style=flat)](https://github.com/kms9/dars/stargazers)
[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/W8gYBn226t)

[官网](https://dars.ai) · [Discord](https://discord.gg/W8gYBn226t) · [X](https://x.com/DARSAI) · [自部署指南](SELF_HOSTING.md) · [参与贡献](CONTRIBUTING.md)

**[English](README.md) | 简体中文**

</div>

## DARS 是什么？

DARS 是 **Distributed Agent Runtime System（分布式 Agent Runtime 系统）**。Web 控制面与 Go Server 协同本地 Daemon/CLI Worker，把 Run 路由到合适的 Agent Runtime，并将执行进度持续汇总到一个可观测系统中。

每个 task 都有明确的运行时配置、作用域凭证、隔离工作目录、持久化事件与结果记录。DARS 是厂商中立、可自部署、面向人类与 AI 团队的开源基础设施。支持 **Claude Code**、**Codex**、**CodeBuddy**、**GitHub Copilot CLI**、**OpenCode**、**OpenClaw**、**Hermes**、**Pi**、**Cursor Agent**、**Kimi**、**Kiro CLI**、**Antigravity**、**Qoder CLI** 与 **Trae CLI**。

面向更大的团队，Squads（小队）提供稳定的路由层：把任务分给由 Agent 带队的小队，由队长判断谁最适合接手。

## 为什么叫 "DARS"？

DARS 是 **Distributed Agent Runtime System** 的缩写：

- **Distributed**：控制面与执行 Worker 可以运行在不同机器上。
- **Agent Runtime**：Provider CLI 通过明确的 Runtime、工作区、凭证与生命周期控制来执行。
- **System**：路由、执行、事件、协作、Skill 与证据使用同一套持久化模型。

## 功能特性

DARS 管理完整的分布式 Agent 生命周期：从任务分配、Runtime 路由到执行监控与 Skill 复用。

- **Agent 即队友** — 使用明确的 Runtime、Model、Skill、环境变量和调用权限创建 Agent。
- **Squad（小队）** — 把多个 Agent 组合成由 Leader Agent 带队的小队，Leader 通过评论和 Mention 完成委派。
- **自主执行** — 设置后无需管理。完整的任务生命周期管理（排队、认领、执行、完成/失败），通过 WebSocket 实时推送进度。
- **Direct Chat 与 Run** — 可以直接与 Agent 对话，也可以创建包含独立评论区和任务执行历史的持久化 Run。
- **可复用技能** — 每个解决方案都成为全团队可复用的技能。部署、数据库迁移、代码审查——技能让团队能力随时间持续增长。
- **本地 Runtime** — 连接本地 daemon，自动检测已安装的 Agent CLI，并监控运行状态。
- **多工作区** — 按 Workspace 隔离 Agent、Squad、Skill、Chat 和 Run。

---

## 快速安装

### macOS / Linux（推荐 Homebrew）

```bash
brew install kms9/tap/dars
```

后续可用 `brew upgrade kms9/tap/dars` 更新 CLI。

### macOS / Linux（安装脚本）

```bash
curl -fsSL https://raw.githubusercontent.com/kms9/dars/main/scripts/install.sh | bash
```

如果没有 Homebrew，可以使用安装脚本。脚本会安装 DARS CLI：检测到 `brew` 时通过 Homebrew 安装，否则直接下载二进制。

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/kms9/dars/main/scripts/install.ps1 | iex
```

安装完成后，一条命令完成配置、认证和启动：

```bash
dars setup self-host
```

> **自部署？** 加上 `--with-server` 在本地部署完整的 DARS 服务：
>
> ```bash
> curl -fsSL https://raw.githubusercontent.com/kms9/dars/main/scripts/install.sh | bash -s -- --with-server
> dars setup self-host
> ```
>
> 需要 Docker。详见 [自部署指南](SELF_HOSTING.md)。

---

## 快速上手

安装好 CLI 并部署服务后，按以下步骤将第一个任务分配给 Agent：

### 1. 配置并启动 daemon

```bash
dars setup self-host # 配置、认证、启动 daemon（一条命令搞定）
```

daemon 在后台运行，保持你的机器与 DARS 的连接。它会自动检测 PATH 中可用的 Agent CLI（`claude`、`codex`、`codebuddy`、`copilot`、`opencode`、`openclaw`、`hermes`、`pi`、`cursor-agent`、`kimi`、`kiro-cli`、`agy`、`qodercli`、`qoderclicn`、`traecli`）。

### 2. 确认运行时已连接

在 DARS Web 端打开 Workspace，进入 **Runtimes**，你应该能看到当前机器已作为活跃 Runtime 出现在列表中。

> **什么是 Runtime（运行时）？** Runtime 是通过 daemon 连接的本地执行环境。它会上报可用的 Agent CLI，DARS 据此决定将任务路由到哪里执行。

### 3. 创建 Agent

进入 **Agents**，点击 **新建 Agent**，选择刚连接的 Runtime 和 Provider，并完成 Agent 配置。

### 4. 分配你的第一个任务

在 **Runs** 中创建一次 Run（或执行 `dars issue create`），然后将其分配给 Agent 或 Squad。执行方会在 Runtime 上运行，并通过评论和任务事件持续汇报进度。

大功告成！你的 Agent 现在是团队的一员了。 🎉

---

## 架构

```
┌──────────────┐     ┌──────────────┐     ┌──────────────────┐
│   Next.js    │────>│  Go 后端     │────>│   PostgreSQL     │
│   前端       │<────│  (Chi + WS)  │<────│   (pgvector)     │
└──────────────┘     └──────┬───────┘     └──────────────────┘
                            │
                     ┌──────┴───────┐
                     │ Agent Daemon │  运行在你的机器上
                     └──────────────┘  （Claude Code、Codex、CodeBuddy、GitHub Copilot CLI、
                                        OpenCode、OpenClaw、Hermes、Pi、Cursor Agent、
                                        Kimi、Kiro CLI、Antigravity、Qoder CLI、Trae CLI）
```

| 层级 | 技术栈 |
|------|--------|
| 前端 | Next.js 16 (App Router) |
| 后端 | Go (Chi router, sqlc, gorilla/websocket) |
| 数据库 | PostgreSQL 17 with pgvector |
| Agent 运行时 | 本地 daemon 执行 Claude Code、Codex、CodeBuddy、GitHub Copilot CLI、OpenCode、OpenClaw、Hermes、Pi、Cursor Agent、Kimi、Kiro CLI、Antigravity、Qoder CLI 或 Trae CLI |

## 开发

参与 DARS 代码贡献，请参阅 [贡献指南](CONTRIBUTING.md)。

**环境要求：** [Node.js](https://nodejs.org/) v20+, [pnpm](https://pnpm.io/) v10.28+, [Go](https://go.dev/) v1.26+, [Docker](https://www.docker.com/)

```bash
pnpm install
cp .env.example .env
make setup
make start
```

完整的开发流程、worktree 支持、测试和问题排查请参阅 [CONTRIBUTING.md](CONTRIBUTING.md)。

iOS 移动端代码位于 [`apps/mobile/`](apps/mobile/)，自己编译装到手机的方法见 [README](apps/mobile/README.md)。

## 开源协议

[Multica License](LICENSE)（Apache License 2.0 全文并入 + 附加条件），署名信息见 [NOTICE](NOTICE)。

- 面向第三方的托管服务或嵌入式商业分发，需向 producer 取得商业授权（条款 1a）。
- 除非取得 producer 的书面品牌豁免，不得移除或修改 Multica 界面中的 LOGO、产品名与署名信息。「界面」按派生关系界定——包含 `apps/web/`、`apps/desktop/`、`apps/mobile/`、`packages/views/`、`packages/ui/`——并覆盖源码、前端容器镜像与编译后的桌面 / 移动端二进制（条款 1b）。
- 不使用 Multica 界面的用法（只跑 `server/` 后端、daemon 或 CLI）不受品牌限制，但需保留源码与 [NOTICE](NOTICE) 中的署名，并在产品文档中声明基于 Multica 构建且回链本仓库（条款 1c）。
- 品牌豁免与商业授权是两个独立的授权，互不蕴含（条款 1d）。
