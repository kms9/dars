package protocol

// Lightweight WebSocket event names.
const (
	EventWorkspaceUpdated = "workspace:updated"
	EventWorkspaceDeleted = "workspace:deleted"

	EventAgentStatus   = "agent:status"
	EventAgentCreated  = "agent:created"
	EventAgentUpdated  = "agent:updated"
	EventAgentArchived = "agent:archived"
	EventAgentRestored = "agent:restored"

	EventSkillCreated = "skill:created"
	EventSkillUpdated = "skill:updated"
	EventSkillDeleted = "skill:deleted"

	EventSquadCreated = "squad:created"
	EventSquadUpdated = "squad:updated"
	EventSquadDeleted = "squad:deleted"

	EventIssueCreated   = "issue:created"
	EventIssueUpdated   = "issue:updated"
	EventIssueDeleted   = "issue:deleted"
	EventCommentCreated = "comment:created"

	EventTaskQueued                = "task:queued"
	EventTaskDispatch              = "task:dispatch"
	EventTaskWaitingLocalDirectory = "task:waiting_local_directory"
	EventTaskRunning               = "task:running"
	EventTaskProgress              = "task:progress"
	EventTaskMessage               = "task:message"
	EventTaskCompleted             = "task:completed"
	EventTaskFailed                = "task:failed"
	EventTaskCancelled             = "task:cancelled"

	EventChatMessage         = "chat:message"
	EventChatDone            = "chat:done"
	EventChatCancelFinalized = "chat:cancel_finalized"
	EventChatSessionUpdated  = "chat:session_updated"
	EventChatSessionDeleted  = "chat:session_deleted"

	EventDaemonHeartbeat              = "daemon:heartbeat"
	EventDaemonHeartbeatAck           = "daemon:heartbeat_ack"
	EventDaemonRegister               = "daemon:register"
	EventDaemonTaskAvailable          = "daemon:task_available"
	EventDaemonRuntimeProfilesChanged = "daemon:runtime_profiles_changed"
	EventDaemonWorkspacesChanged      = "daemon:workspaces_changed"
	EventDaemonPendingWork            = "daemon:pending_work"
	EventDaemonRPCRequest             = "daemon:rpc_request"
	EventDaemonRPCResponse            = "daemon:rpc_response"
)
