package lightweightapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	agentBuilderDraftNameMax        = 200
	agentBuilderDraftDescriptionMax = 2000
	agentBuilderInstructionsMax     = 64 * 1024
)

var agentBuilderForbiddenDraftFields = []string{
	"custom_env", "env", "mcp_config", "mcp", "integration", "integrations",
	"owner_id", "owner", "archived", "archived_at", "archived_by", "status",
	"history", "run_count", "last_active", "id", "workspace_id", "kind", "system_key",
	"runtime_config", "custom_args", "disabled_runtime_skills", "avatar_url",
	"thinking_level", "service_tier", "max_concurrent_tasks", "runtime_mode",
}

// agentBuilderDraft is the server-approved subset of Agent configuration that may
// be stored in agent_builder_draft or finalized into a user Agent.
type agentBuilderDraft struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Instructions    string   `json:"instructions"`
	Model           string   `json:"model"`
	SkillIDs        []string `json:"skill_ids"`
	PermissionScope string   `json:"permission_scope"`
	MemberIDs       []string `json:"member_ids"`
	RuntimeID       string   `json:"runtime_id,omitempty"`
	ThinkingLevel   string   `json:"thinking_level,omitempty"`
	ServiceTier     string   `json:"service_tier,omitempty"`
	MaxConcurrent   *int32   `json:"max_concurrent_tasks,omitempty"`
}

type agentBuilderDraftTombstone struct {
	FinalizedAgentID string `json:"finalized_agent_id"`
	IdempotencyKey   string `json:"idempotency_key,omitempty"`
}

const agentBuilderInstructions = `You are DARS Agent Builder. Help the user design one practical AI agent through a short conversation.

Your job is to propose and refine configuration, never to create resources yourself. Ask only questions that materially change behavior. Prefer making a reasonable draft immediately, then ask at most two focused questions per turn.

Every response MUST end with exactly one <agent_draft> JSON block using this shape:
<agent_draft>{"name":"","description":"","instructions":"","model":"","skill_ids":[],"permission_scope":"private","member_ids":[]}</agent_draft>

Rules:
- The JSON must be valid, compact JSON on one physical line. Do not wrap it in Markdown fences.
- Escape every line break inside instructions as \n. Never place a literal newline inside a JSON string.
- Preserve good existing draft fields supplied in the user's message unless the user asks to change them.
- name is concise and suitable for a workspace list.
- description is one sentence, at most 200 characters.
- instructions are a complete Markdown system prompt describing role, workflow, output, and constraints.
- model must be empty, preserve current_draft.model, or exactly match an id explicitly listed in AVAILABLE RUNTIME MODELS. Never use a model label as the id.
- When AVAILABLE RUNTIME MODELS is null or empty, preserve current_draft.model and never invent a model id.
- skill_ids may only contain IDs explicitly listed in AVAILABLE WORKSPACE SKILLS.
- permission_scope must be private, workspace, or members. Default to private unless the user explicitly requests sharing.
- member_ids may only contain IDs explicitly listed in AVAILABLE WORKSPACE MEMBERS, and only when permission_scope is members.
- Never request, expose, or place secrets, tokens, passwords, or environment-variable values in the draft.
- Do not claim that the agent has been created. The user must review and confirm the draft in the UI.`

func parseAgentBuilderDraft(raw json.RawMessage) (agentBuilderDraft, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return agentBuilderDraft{}, fmt.Errorf("invalid draft")
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return agentBuilderDraft{}, fmt.Errorf("invalid draft")
	}
	for _, forbidden := range agentBuilderForbiddenDraftFields {
		if _, ok := envelope[forbidden]; ok {
			return agentBuilderDraft{}, fmt.Errorf("forbidden field %s", forbidden)
		}
	}
	if meta, ok := envelope["_meta"]; ok {
		var tombstone agentBuilderDraftTombstone
		if json.Unmarshal(meta, &tombstone) == nil && tombstone.FinalizedAgentID != "" {
			return agentBuilderDraft{}, fmt.Errorf("session completed")
		}
	}
	var draft agentBuilderDraft
	if err := json.Unmarshal(raw, &draft); err != nil {
		return agentBuilderDraft{}, fmt.Errorf("invalid draft")
	}
	draft.Name = strings.TrimSpace(draft.Name)
	draft.Description = strings.TrimSpace(draft.Description)
	draft.Instructions = strings.TrimSpace(draft.Instructions)
	draft.Model = strings.TrimSpace(draft.Model)
	draft.PermissionScope = strings.TrimSpace(draft.PermissionScope)
	if draft.PermissionScope == "" {
		draft.PermissionScope = "private"
	}
	if draft.Name == "" {
		return agentBuilderDraft{}, fmt.Errorf("name is required")
	}
	if utf8.RuneCountInString(draft.Name) > agentBuilderDraftNameMax {
		return agentBuilderDraft{}, fmt.Errorf("name is too long")
	}
	if utf8.RuneCountInString(draft.Description) > agentBuilderDraftDescriptionMax {
		return agentBuilderDraft{}, fmt.Errorf("description is too long")
	}
	if draft.Instructions == "" {
		return agentBuilderDraft{}, fmt.Errorf("instructions are required")
	}
	if utf8.RuneCountInString(draft.Instructions) > agentBuilderInstructionsMax {
		return agentBuilderDraft{}, fmt.Errorf("instructions are too long")
	}
	switch draft.PermissionScope {
	case "private", "workspace", "members":
	default:
		return agentBuilderDraft{}, fmt.Errorf("invalid permission_scope")
	}
	if draft.PermissionScope != "members" {
		draft.MemberIDs = nil
	}
	if draft.MaxConcurrent != nil && *draft.MaxConcurrent < 1 {
		return agentBuilderDraft{}, fmt.Errorf("invalid max_concurrent_tasks")
	}
	return draft, nil
}

func parseAgentBuilderDraftTombstone(raw json.RawMessage) (agentBuilderDraftTombstone, bool) {
	if len(raw) == 0 || !json.Valid(raw) {
		return agentBuilderDraftTombstone{}, false
	}
	var envelope struct {
		Meta agentBuilderDraftTombstone `json:"_meta"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Meta.FinalizedAgentID == "" {
		return agentBuilderDraftTombstone{}, false
	}
	return envelope.Meta, true
}

func marshalAgentBuilderDraftTombstone(agentID, idempotencyKey string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"_meta": agentBuilderDraftTombstone{
			FinalizedAgentID: agentID,
			IdempotencyKey:   idempotencyKey,
		},
	})
	return payload
}
