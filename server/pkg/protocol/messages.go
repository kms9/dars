package protocol

import "encoding/json"

const (
	DaemonProtocolVersion          = "lightweight-runtime-v1"
	DaemonCapabilitySkillBundlesV1 = "skill-bundles-v1"
	DaemonCapabilityRPCV1          = "rpc-v1"
)

type RPCRequestPayload struct {
	RequestID string          `json:"request_id"`
	Method    string          `json:"method"`
	Body      json.RawMessage `json:"body,omitempty"`
	TimeoutMs int64           `json:"timeout_ms,omitempty"`
}

type RPCResponsePayload struct {
	RequestID string          `json:"request_id"`
	Status    int             `json:"status"`
	Body      json.RawMessage `json:"body,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type TaskAvailablePayload struct {
	RuntimeID string `json:"runtime_id"`
	TaskID    string `json:"task_id,omitempty"`
}

type RuntimeProfilesChangedPayload struct {
	WorkspaceID      string `json:"workspace_id"`
	RuntimeProfileID string `json:"runtime_profile_id,omitempty"`
}

type WorkspacesChangedPayload struct{}

const PendingWorkKindModelList = "model_list"

type PendingWorkPayload struct {
	RuntimeID string `json:"runtime_id"`
	Kind      string `json:"kind,omitempty"`
}

type TaskProgressPayload struct {
	TaskID  string `json:"task_id"`
	Summary string `json:"summary"`
	Step    int    `json:"step,omitempty"`
	Total   int    `json:"total,omitempty"`
}

type TaskMessagePayload struct {
	TaskID    string         `json:"task_id"`
	IssueID   string         `json:"issue_id,omitempty"`
	Seq       int            `json:"seq"`
	Type      string         `json:"type"`
	Tool      string         `json:"tool,omitempty"`
	Content   string         `json:"content,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
}

type ChatMessagePayload struct {
	ChatSessionID string `json:"chat_session_id"`
	MessageID     string `json:"message_id"`
	Role          string `json:"role"`
	Content       string `json:"content"`
	TaskID        string `json:"task_id,omitempty"`
	CreatedAt     string `json:"created_at"`
}

const (
	ChatMessageKindMessage    = "message"
	ChatMessageKindNoResponse = "no_response"
)

type ChatDonePayload struct {
	ChatSessionID string `json:"chat_session_id"`
	TaskID        string `json:"task_id"`
	MessageID     string `json:"message_id,omitempty"`
	Content       string `json:"content,omitempty"`
	ElapsedMs     int64  `json:"elapsed_ms,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	MessageKind   string `json:"message_kind,omitempty"`
}

const (
	ChatCancelOutcomeStopped  = "stopped"
	ChatCancelOutcomeRestored = "restored"
)

type ChatCancelFinalizedPayload struct {
	Outcome         string `json:"outcome"`
	ChatSessionID   string `json:"chat_session_id"`
	TaskID          string `json:"task_id"`
	InitiatorUserID string `json:"initiator_user_id,omitempty"`
	MessageID       string `json:"message_id,omitempty"`
	Content         string `json:"content,omitempty"`
	MessageKind     string `json:"message_kind,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	ElapsedMs       int64  `json:"elapsed_ms,omitempty"`
}

type DaemonHeartbeatRequestPayload struct {
	RuntimeID           string `json:"runtime_id"`
	SupportsBatchImport bool   `json:"supports_batch_import,omitempty"`
}

type DaemonHeartbeatAckPayload struct {
	RuntimeID                string                                   `json:"runtime_id"`
	Status                   string                                   `json:"status"`
	ServerCapabilities       []string                                 `json:"server_capabilities,omitempty"`
	RuntimeGone              bool                                     `json:"runtime_gone,omitempty"`
	PendingModelList         *DaemonHeartbeatPendingModelList         `json:"pending_model_list,omitempty"`
	PendingLocalSkills       *DaemonHeartbeatPendingLocalSkills       `json:"pending_local_skills,omitempty"`
	PendingLocalSkillImport  *DaemonHeartbeatPendingLocalSkillImport  `json:"pending_local_skill_import,omitempty"`
	PendingLocalSkillImports []DaemonHeartbeatPendingLocalSkillImport `json:"pending_local_skill_imports,omitempty"`
}

const HeartbeatStatusRuntimeGone = "runtime_gone"

type DaemonHeartbeatPendingModelList struct {
	ID string `json:"id"`
}

type DaemonHeartbeatPendingLocalSkills struct {
	ID string `json:"id"`
}

type DaemonHeartbeatPendingLocalSkillImport struct {
	ID       string `json:"id"`
	SkillKey string `json:"skill_key"`
}
