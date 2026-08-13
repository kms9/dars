package daemon

import (
	"fmt"
	"strings"

	"github.com/kms9/dars/internal/daemon/execenv"
)

// freshSessionRetryPrompt makes provider-session loss explicit before the
// daemon retries with a fresh session. Durable Run comments or Direct Chat
// messages, not provider memory, are the source of truth.
func freshSessionRetryPrompt(prompt string) string {
	const notice = "⚠️ Note: a previous provider session for this task could not be resumed, so this is a brand-new session. None of the earlier provider conversation context is available. Do not assume prior in-memory state or uncommitted work carried over; rebuild context from the current Lightweight Run comments or Direct Chat messages before acting.\n\n"
	return notice + prompt
}

// perTurnContextBlocks appends the claim-specific identity and continuity
// context after the stable runtime brief so resumed sessions retain their
// provider prompt-cache prefix.
func perTurnContextBlocks(task Task) string {
	var b strings.Builder
	if task.PriorSessionResumeUnavailable {
		b.WriteString(execenv.SessionContinuityNotice)
	}
	return b.String()
}

// BuildPrompt 构造 Daemon 交给 Agent CLI 的唯一两类 prompt：Run 或 Direct Chat。
// 小队 Leader（IsLeaderTask）会附带花名册，并要求执行 dars squad evaluate。
// 详细协作规则由 execenv 注入的 runtime brief / builtin skill 补充。
// BuildPrompt constructs the only two prompts supported by
// lightweight-runtime-v1: a Run task or a Direct Chat task. Detailed rules
// live in the runtime brief injected by execenv.
func BuildPrompt(task Task, _ string) string {
	body := buildPromptBody(task)
	if blocks := perTurnContextBlocks(task); blocks != "" {
		if !strings.HasSuffix(body, "\n\n") {
			body += "\n"
		}
		body += blocks
	}
	return body
}

func buildPromptBody(task Task) string {
	if task.ChatSessionID != "" {
		return buildLightweightChatPrompt(task)
	}
	return buildLightweightRunPrompt(task)
}

func buildLightweightRunPrompt(task Task) string {
	var b strings.Builder
	b.WriteString("You are executing one Lightweight Run with a task-scoped credential.\n\n")
	fmt.Fprintf(&b, "Run ID: `%s`\n", task.IssueID)
	if task.ID != "" {
		fmt.Fprintf(&b, "Task ID: `%s`\n", task.ID)
	}
	switch {
	case task.IsLeaderTask:
		b.WriteString("Role: Squad Leader\n")
	case task.SquadID != "":
		b.WriteString("Role: delegated Squad member\n")
	default:
		b.WriteString("Role: assigned Agent\n")
	}
	b.WriteString("\n")
	if task.Squad != nil {
		fmt.Fprintf(&b, "Authorized Squad context: `%s` (`%s`)\n", promptLine(task.Squad.Name), task.Squad.ID)
		if strings.TrimSpace(task.Squad.Instructions) != "" {
			fmt.Fprintf(&b, "Squad instructions: %s\n", promptLine(task.Squad.Instructions))
		}
		b.WriteString("Authorized roster (use these IDs for canonical mentions; do not enumerate Squad APIs with the Task Token):\n")
		for _, member := range task.Squad.Members {
			fmt.Fprintf(&b, "- %s (`%s`, role: `%s`)\n", promptLine(member.Name), member.AgentID, promptLine(member.Role))
		}
		b.WriteString("\n")
	}
	if task.HandoffNote != "" {
		b.WriteString("Handoff scope:\n\n")
		fmt.Fprintf(&b, "> %s\n\n", strings.ReplaceAll(strings.TrimSpace(task.HandoffNote), "\n", "\n> "))
	}
	if len(task.CommentIDs) > 0 {
		fmt.Fprintf(&b, "This Task includes these durable flat comment IDs: %s. Resolve their current content from the Run comment list.\n\n", strings.Join(task.CommentIDs, ", "))
	}
	fmt.Fprintf(&b, "Start with `dars issue get %s` and `dars issue comment list %s`. Follow the role-specific workflow in the injected runtime brief.\n", task.IssueID, task.IssueID)
	if task.IsLeaderTask {
		fmt.Fprintf(&b, "Record exactly one current-Task decision with `dars squad evaluate %s --outcome <action|no_action|failed> --reason \"...\"`.\n", task.IssueID)
	}
	return b.String()
}

func promptLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func buildLightweightChatPrompt(task Task) string {
	var b strings.Builder
	b.WriteString("You are executing one Lightweight Direct Chat Task with a task-scoped credential.\n\n")
	fmt.Fprintf(&b, "Chat session ID: `%s`\n", task.ChatSessionID)
	if task.ID != "" {
		fmt.Fprintf(&b, "Task ID: `%s`\n", task.ID)
	}
	b.WriteString("\n")
	if task.ChatMessage != "" {
		b.WriteString("Current user message:\n\n")
		b.WriteString(task.ChatMessage)
		b.WriteString("\n\n")
	}
	b.WriteString("If earlier context is needed, use the Task Token only with `dars chat messages list <session-id>`. Return the answer as your final assistant output; do not call `chat messages send`, and do not use Issue, channel, project, or attachment commands.\n")
	return b.String()
}
