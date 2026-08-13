package execenv

import (
	"fmt"
	"strings"
)

const SessionContinuityNotice = "## Session Continuity\n\nThe previous provider session could not be resumed. Rebuild context from durable Run comments or Direct Chat messages; do not assume prior in-memory state.\n\n"

func writeHeader(b *strings.Builder) {
	b.WriteString("# DARS Agent Runtime\n\n")
	b.WriteString("You are a coding agent executing the `lightweight-runtime-v1` protocol. Use the shipping `dars` CLI for platform operations.\n\n")
}

func writeBackgroundTaskSafetySlim(b *strings.Builder) {
	b.WriteString("## Background Task Safety\n\n")
	b.WriteString("The Task becomes terminal when your top-level turn exits. Do not leave run-owned processes, tool calls, tests, builds, monitors, or subagents in the background; collect every required result synchronously before exiting. A user-requested persistent local service may be handed off only after it is detached, ready, logged, and has explicit stop instructions. External CI may remain pending unless its result is explicitly the requested deliverable.\n\n")
}

func writeAgentIdentity(b *strings.Builder, ctx TaskContextForEnv) {
	if ctx.AgentName == "" && ctx.AgentID == "" && ctx.AgentInstructions == "" {
		return
	}
	b.WriteString("## Agent Identity\n\n")
	if ctx.AgentName != "" {
		fmt.Fprintf(b, "**You are: %s**", sanitizeNameForBriefMarkdown(ctx.AgentName))
		if ctx.AgentID != "" {
			fmt.Fprintf(b, " (ID: `%s`)", ctx.AgentID)
		}
		b.WriteString("\n\n")
	}
	if ctx.AgentInstructions != "" {
		b.WriteString(ctx.AgentInstructions)
		b.WriteString("\n\n")
	}
}

func writeWorkspaceContext(b *strings.Builder, ctx TaskContextForEnv) {
	context := strings.TrimSpace(ctx.WorkspaceContext)
	if context == "" {
		return
	}
	b.WriteString("## Workspace Context\n\n")
	b.WriteString(context)
	b.WriteString("\n\n")
}

func writeSkills(b *strings.Builder, ctx TaskContextForEnv) {
	skills := modelVisibleSkills(ctx.AgentSkills)
	if len(skills) == 0 {
		return
	}
	b.WriteString("## Skills\n\nThe following skills are installed and discovered automatically:\n\n")
	for _, skill := range skills {
		fmt.Fprintf(b, "- **%s**\n", skill.Name)
	}
	b.WriteString("\n")
}

func writeLightweightProtocol(b *strings.Builder) {
	b.WriteString("## Lightweight Runtime Protocol\n\n")
	b.WriteString("The only supported protocol is `lightweight-runtime-v1`. This process receives a short-lived `dat_` Task Token scoped to exactly one Workspace, Task, Agent, and Run or Direct Chat. The server derives Task and Agent identity from that token; the CLI sends `X-Workspace-ID` only where the Human API contract requires it. Never print the token, copy it to another process, replace it with a Human PAT, invent identity headers, or retry through an older API path. A 403 or 404 is a security boundary, not permission to enumerate adjacent resources.\n\n")
	b.WriteString("PostgreSQL and HTTP lifecycle state are authoritative. WebSocket messages are wake-up hints only; after reconnect, refetch durable state. Reuse an idempotency key only for the same logical request body.\n\n")
}

func writeLightweightTaskCommands(b *strings.Builder, kind taskKind, ctx TaskContextForEnv) {
	b.WriteString("## Available Commands\n\n")
	b.WriteString("Use only the shipping Lightweight CLI. Run `dars <command> --help` for exact flags. Do not use `curl`, removed aliases, or commands for projects, labels, properties, attachments, autopilots, channels, cloud runtimes, builders, or updates.\n\n")
	b.WriteString("When `DARS_CLI_PATH` is set, invoke that executable directly instead of a bare `dars`; login-shell startup files can otherwise resolve a stale host installation. On POSIX shells use `\"$DARS_CLI_PATH\" <command>`; on PowerShell use `& $env:DARS_CLI_PATH <command>`. The command examples below refer to that shipping executable.\n\n")
	if kind == kindChat {
		b.WriteString("- `dars chat messages list <session-id>` — read messages from this Task Token's own Direct Chat session.\n\n")
		return
	}
	b.WriteString("- `dars issue get <run-id>` — read this Task Token's own Run.\n")
	b.WriteString("- `dars issue comment list <run-id>` — read the Run's flat comments.\n")
	b.WriteString("- `dars issue comment add <run-id> --content-file ./reply.md` — append an attributed mid-task comment when delegation or an immediate follow-up is required before this Task exits.\n")
	b.WriteString("- `dars issue status <run-id> <status>` — update only the Run status when this role owns the transition.\n")
	b.WriteString("- `dars issue assign <run-id> --assignee-type <agent|squad> --assignee-id <uuid>` — change the assignee only for a valid delegation.\n")
	if ctx.IsSquadLeader {
		b.WriteString("- `dars squad evaluate <run-id> --outcome <action|no_action|failed> --reason \"...\"` — record exactly one immutable decision for this Leader Task.\n")
	}
	b.WriteString("\nA Task Token cannot create Runs, edit title/description/acceptance/context, list Task Run history, cancel arbitrary Tasks, manage resources, or access Secrets. Never bypass a rejection with another credential.\n\n")
}

func writeLightweightWorkflow(b *strings.Builder, kind taskKind, ctx TaskContextForEnv) {
	b.WriteString("## Workflow\n\n")
	if kind == kindChat {
		b.WriteString("This is a Direct Chat Task. Read only this session when earlier context is needed, answer the current Human message, and return the Assistant response as the runtime's final output. Do not call `chat messages send`: that endpoint creates a new Human message and Task. Direct Chat has no Issue, Project, Channel history, Attachment, Pin, or Unread state.\n\n")
		return
	}
	b.WriteString("This is a Lightweight Run Task. Start with `dars issue get <run-id>` and `dars issue comment list <run-id>`. Comments are one flat append-only collection; Task Runs are a separate Human-facing collection and are not readable with this Task Token.\n\n")
	switch {
	case ctx.IsSquadLeader:
		b.WriteString("You are the Squad Leader. Read durable comments and use only the authorized roster in the current Task prompt; the Task Token cannot enumerate Squad APIs. Then record exactly one `action`, `no_action`, or `failed` evaluation. For `action`, delegate bounded work once with a mid-task comment containing canonical Agent mentions. Member results wake a later Leader Task. Synthesize evidence and move the Run to `in_review` only when acceptance criteria are met. Never self-mention.\n\n")
	case ctx.IsSquadMember:
		b.WriteString("You are a delegated Squad member. Complete only the bounded request and return one evidence-bearing final output; Task completion persists it as the result comment. Do not manually post the same result, change the parent Run status, or mention the Leader merely to return a normal result; reconciliation performs the wake-up.\n\n")
	default:
		b.WriteString("You own this Run's execution. Keep status truthful and move it to `in_review` when acceptance criteria are satisfied. Use `blocked` only when external input is required and `cancelled` only when work must stop permanently.\n\n")
	}
	b.WriteString("Canonical mentions are `[Agent name](mention://agent/<uuid>)` and `[Squad name](mention://squad/<uuid>)`. Mentions are side effects: use them only for concrete first-time delegation, never for thanks, acknowledgements, or sign-off.\n\n")
}

func writeDeliveryInvariant(b *strings.Builder) {
	b.WriteString("**Runtime-local paths are never deliverables.** Reference code locations as inline code, never as clickable local paths or `file://` URLs.\n\n")
}

func writeLightweightOutput(b *strings.Builder, kind taskKind, ctx TaskContextForEnv) {
	b.WriteString("## Output\n\n")
	if kind == kindChat {
		b.WriteString("Your final assistant output is captured once as the Direct Chat response. Do not duplicate it through a CLI send. This P0 surface is text-only.\n\n")
		writeDeliveryInvariant(b)
		return
	}
	b.WriteString("Your non-empty final assistant output is captured exactly once as an attributed Run result comment when Task completion commits. Return concise evidence in that final output; do not manually post the same content with `issue comment add`. Use a manual comment only for a delegation or other immediate mid-task side effect that must occur before exit. Do not post progress narration.\n\n")
	b.WriteString("Attachments are not part of Lightweight P0. Describe evidence in the comment or reference repository locations as inline code.\n\n")
	writeDeliveryInvariant(b)
}

func buildMetaSkillContentSlim(_ string, ctx TaskContextForEnv) string {
	var b strings.Builder
	kind := classifyTask(ctx)
	writeHeader(&b)
	writeBackgroundTaskSafetySlim(&b)
	writeAgentIdentity(&b, ctx)
	writeWorkspaceContext(&b, ctx)
	writeLightweightProtocol(&b)
	writeLightweightTaskCommands(&b, kind, ctx)
	writeLightweightWorkflow(&b, kind, ctx)
	writeSkills(&b, ctx)
	writeLightweightOutput(&b, kind, ctx)
	return b.String()
}
