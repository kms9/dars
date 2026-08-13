package daemon

import (
	"encoding/json"
)

// AgentEntry describes a single available agent CLI.
type AgentEntry struct {
	Path string // path to CLI binary (pinned at startup; symlink-resolved to a concrete, possibly versioned, path)
	// Command is the bare command name or DARS_*_PATH value that Path was
	// resolved from at startup. It is kept so the daemon can re-resolve Path
	// if the pinned executable later vanishes — e.g. a version manager
	// (Homebrew Cask, nvm/fnm) does an in-place upgrade that deletes the old
	// versioned directory Path points into. Empty for synthesized entries
	// (custom runtime profiles) that carry an absolute path directly. See
	// Daemon.resolveAgentEntry and MUL-4486.
	Command string
	Model   string // model override (optional)
}

// Runtime represents a registered daemon runtime.
type Runtime struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
	// ProfileID is non-empty when this runtime was registered from a
	// workspace custom runtime profile (MUL-3284). It links the runtime row
	// back to the profile so the daemon can resolve the profile's
	// command_name to the executable to launch. Built-in (provider-detected)
	// runtimes leave this empty.
	ProfileID string `json:"profile_id,omitempty"`
}

// RepoData holds repository information from the workspace.
type RepoData struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Ref         string `json:"ref,omitempty"`
}

// Task represents a claimed task from the server.
// Agent data (name, skills) is populated by the claim endpoint.
type Task struct {
	ID                string     `json:"id"`
	AgentID           string     `json:"agent_id"`
	RuntimeID         string     `json:"runtime_id"`
	IssueID           string     `json:"issue_id"`
	WorkspaceID       string     `json:"workspace_id"`
	WorkspaceContext  string     `json:"workspace_context,omitempty"`
	ThreadName        string     `json:"thread_name,omitempty"`
	Agent             *AgentData `json:"agent,omitempty"`
	Repos             []RepoData `json:"repos,omitempty"`
	SquadID           string     `json:"squad_id,omitempty"`
	Squad             *SquadData `json:"squad,omitempty"`
	IsLeaderTask      bool       `json:"is_leader_task,omitempty"`
	ForceFreshSession bool       `json:"force_fresh_session,omitempty"`
	HandoffNote       string     `json:"handoff_note,omitempty"`
	PriorSessionID    string     `json:"prior_session_id,omitempty"`
	PriorWorkDir      string     `json:"prior_work_dir,omitempty"`
	ChatSessionID     string     `json:"chat_session_id,omitempty"`
	ChatMessage       string     `json:"chat_message,omitempty"`
	CommentIDs        []string   `json:"comment_ids,omitempty"`

	// PriorSessionResumeUnavailable is daemon-local and records that a claimed
	// resume could not be honored. It is never accepted from the wire.
	PriorSessionResumeUnavailable bool `json:"-"`
	// AuthToken is the task-scoped credential the server mints at claim time.
	// The daemon injects it into the spawned agent as DARS_TOKEN so the
	// agent never sees the daemon's own (often workspace-owner) credential.
	// Empty or non-task-scoped values are fatal for writable agent tasks; the
	// daemon must not fall back to its own token. See MUL-3292.
	AuthToken string `json:"auth_token,omitempty"`
}

// SquadData is the non-secret, claim-authorized collaboration context for a
// Squad task. Task Tokens intentionally cannot enumerate Squad APIs, so the
// daemon must receive the roster with the claim rather than asking the spawned
// agent to cross that credential boundary.
type SquadData struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Instructions string            `json:"instructions,omitempty"`
	Members      []SquadMemberData `json:"members"`
}

type SquadMemberData struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

// AgentData holds agent details returned by the claim endpoint.
type AgentData struct {
	ID                    string                     `json:"id"`
	Name                  string                     `json:"name"`
	Instructions          string                     `json:"instructions"`
	Skills                []SkillData                `json:"skills,omitempty"`
	SkillRefs             []SkillRefData             `json:"skill_refs,omitempty"`
	CustomEnv             map[string]string          `json:"custom_env,omitempty"`
	CustomArgs            []string                   `json:"custom_args,omitempty"`
	McpConfig             json.RawMessage            `json:"mcp_config,omitempty"`
	Model                 string                     `json:"model,omitempty"`
	ThinkingLevel         string                     `json:"thinking_level,omitempty"`
	ServiceTier           string                     `json:"service_tier,omitempty"`
	DisabledRuntimeSkills []DisabledRuntimeSkillData `json:"disabled_runtime_skills,omitempty"`
	// RuntimeConfig is the per-provider runtime_config JSON as stored on
	// the agent record, forwarded verbatim by the claim endpoint. The
	// daemon decodes provider-specific fields (e.g. openclaw mode +
	// gateway endpoint, see issue #3260); other backends ignore it.
	RuntimeConfig json.RawMessage `json:"runtime_config,omitempty"`
}

// DisabledRuntimeSkillData is the task-wire identity of one runtime-local
// skill that must be hidden from this agent's provider process.
type DisabledRuntimeSkillData struct {
	RuntimeID string `json:"runtime_id"`
	Provider  string `json:"provider"`
	Root      string `json:"root"`
	Key       string `json:"key"`
	Name      string `json:"name,omitempty"`
	Plugin    string `json:"plugin,omitempty"`
}

// SkillData represents a structured skill for task execution.
type SkillData struct {
	ID          string          `json:"id"`
	Source      string          `json:"source,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Hash        string          `json:"hash,omitempty"`
	SizeBytes   int64           `json:"size_bytes,omitempty"`
	Content     string          `json:"content"`
	Files       []SkillFileData `json:"files,omitempty"`
}

// SkillFileData represents a supporting file within a skill.
type SkillFileData struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type SkillRefData struct {
	ID          string             `json:"id"`
	Source      string             `json:"source"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Hash        string             `json:"hash"`
	SizeBytes   int64              `json:"size_bytes"`
	FileCount   int                `json:"file_count"`
	Files       []SkillFileRefData `json:"files,omitempty"`
}

type SkillFileRefData struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

// TaskUsageEntry represents token usage for a single model during a task execution.
type TaskUsageEntry struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	// CostUSDTicks is the provider's own price for this usage, in 1e-10 USD.
	// Omitted when the agent reports no cost, which is the common case — the
	// server then leaves the column NULL and the client estimates from the
	// pricing table instead. See agent.TokenUsage.CostUSDTicks.
	CostUSDTicks int64 `json:"cost_usd_ticks,omitempty"`
}

// TaskResult is the outcome of executing a task.
type TaskResult struct {
	Status        string `json:"status"`
	Comment       string `json:"comment"`
	BranchName    string `json:"branch_name,omitempty"`
	EnvType       string `json:"env_type,omitempty"`
	SessionID     string `json:"session_id,omitempty"` // Claude session ID for future resumption
	WorkDir       string `json:"work_dir,omitempty"`   // working directory used during execution
	EnvRoot       string `json:"-"`                    // env root dir for writing GC metadata (not sent to server)
	FailureReason string `json:"-"`                    // classifier forwarded to FailTask on the blocked path; empty falls back to 'agent_error'
	// SessionRolloutMissing is set when the daemon withheld this task's Codex
	// session because its rollout was not in the store (MUL-5305). Forwarded to
	// the terminal report so the server clears the resume pointer and flags the
	// continuity gap for the next claim. Not part of the wire result itself.
	SessionRolloutMissing bool `json:"-"`
	// RetiredSessionID names a session this run was told to resume and then
	// abandoned as unresumable (GH #6066). Forwarded on every terminal path,
	// including the completed one: a fresh-session retry that SUCCEEDS is
	// precisely when the abandoned id would otherwise stay selectable.
	RetiredSessionID string           `json:"-"`
	Usage            []TaskUsageEntry `json:"usage,omitempty"` // per-model token usage
}
