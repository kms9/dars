package lightweightapi

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

type agentScopeCounts struct {
	Mine     int `json:"mine"`
	All      int `json:"all"`
	Archived int `json:"archived"`
}

type agentSnapshotItem struct {
	ID                 string  `json:"id"`
	WorkspaceID        string  `json:"workspace_id"`
	OwnerID            string  `json:"owner_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	AvatarURL          *string `json:"avatar_url"`
	RuntimeID          *string `json:"runtime_id"`
	Status             string  `json:"status"`
	PermissionMode     string  `json:"permission_mode"`
	Model              *string `json:"model"`
	ThinkingLevel      *string `json:"thinking_level"`
	ServiceTier        *string `json:"service_tier"`
	MaxConcurrentTasks int32   `json:"max_concurrent_tasks"`
	ArchivedAt         *string `json:"archived_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type agentSnapshotTask struct {
	ID            string  `json:"id"`
	AgentID       string  `json:"agent_id"`
	Status        string  `json:"status"`
	IssueID       *string `json:"issue_id,omitempty"`
	ChatSessionID *string `json:"chat_session_id,omitempty"`
	FailureReason *string `json:"failure_reason,omitempty"`
	StartedAt     *string `json:"started_at,omitempty"`
	CompletedAt   *string `json:"completed_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

type agentRunCountItem struct {
	AgentID  string `json:"agent_id"`
	RunCount int32  `json:"run_count"`
}

type agentActivityBucket struct {
	AgentID     string `json:"agent_id"`
	BucketAt    string `json:"bucket_at"`
	TaskCount   int32  `json:"task_count"`
	FailedCount int32  `json:"failed_count"`
}

type agentFilterOwner struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type agentFilterRuntime struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type agentFilterMetadata struct {
	Owners   []agentFilterOwner   `json:"owners"`
	Runtimes []agentFilterRuntime `json:"runtimes"`
}

type agentSnapshotResponse struct {
	ScopeCounts    agentScopeCounts      `json:"scope_counts"`
	Agents         []agentSnapshotItem   `json:"agents"`
	Tasks          []agentSnapshotTask   `json:"tasks"`
	RunCounts      []agentRunCountItem   `json:"run_counts"`
	Activity       []agentActivityBucket `json:"activity"`
	FilterMetadata agentFilterMetadata   `json:"filter_metadata"`
}

type agentTaskSummary30d struct {
	RunCount      int32   `json:"run_count"`
	SuccessCount  int32   `json:"success_count"`
	FailCount     int32   `json:"fail_count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

type agentTasksPageResponse struct {
	Items      []taskLifecycleResponse `json:"items"`
	NextCursor *string                 `json:"next_cursor"`
	Summary30d agentTaskSummary30d     `json:"summary_30d"`
}

type cancelAgentTasksResponse struct {
	Cancelled int `json:"cancelled"`
}

func (h *Handler) ListAgentSnapshot(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return
	}
	agents, err := h.q.ListAgents(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	visible := make([]lwdb.Agent, 0, len(agents))
	counts := agentScopeCounts{}
	ownerSeen := make(map[string]struct{})
	runtimeSeen := make(map[string]struct{})
	filterOwners := make([]agentFilterOwner, 0)
	filterRuntimes := make([]agentFilterRuntime, 0)
	for _, agent := range agents {
		if !h.humanCanViewAgent(r, agent, userID) {
			continue
		}
		visible = append(visible, agent)
		ownerKey := uuidString(agent.OwnerID)
		if _, seen := ownerSeen[ownerKey]; !seen {
			ownerSeen[ownerKey] = struct{}{}
			if user, userErr := h.q.GetUserByID(r.Context(), agent.OwnerID); userErr == nil {
				filterOwners = append(filterOwners, agentFilterOwner{ID: ownerKey, Name: user.Name})
			}
		}
		if agent.RuntimeID.Valid {
			runtimeKey := uuidString(agent.RuntimeID)
			if _, seen := runtimeSeen[runtimeKey]; !seen {
				runtimeSeen[runtimeKey] = struct{}{}
				runtime, runtimeErr := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: agent.RuntimeID, WorkspaceID: workspaceID})
				if runtimeErr == nil {
					name := runtime.Name
					if runtime.CustomName.Valid && runtime.CustomName.String != "" {
						name = runtime.CustomName.String
					}
					filterRuntimes = append(filterRuntimes, agentFilterRuntime{ID: runtimeKey, Name: name, Status: runtime.Status})
				}
			}
		}
		if agent.ArchivedAt.Valid {
			counts.Archived++
			continue
		}
		counts.All++
		if agent.OwnerID == userID {
			counts.Mine++
		}
	}
	allowedIDs := make(map[string]struct{}, len(visible))
	agentItems := make([]agentSnapshotItem, 0, len(visible))
	for _, agent := range visible {
		allowedIDs[uuidString(agent.ID)] = struct{}{}
		agentItems = append(agentItems, agentSnapshotDTO(agent))
	}
	tasks, err := h.q.ListWorkspaceAgentTaskSnapshot(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	taskItems := make([]agentSnapshotTask, 0, len(tasks))
	for _, task := range tasks {
		if _, ok := allowedIDs[uuidString(task.AgentID)]; !ok {
			continue
		}
		taskItems = append(taskItems, agentSnapshotTaskDTO(task))
	}
	runRows, err := h.q.GetWorkspaceAgentRunCounts(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	runCounts := make([]agentRunCountItem, 0, len(runRows))
	for _, row := range runRows {
		if _, ok := allowedIDs[uuidString(row.AgentID)]; !ok {
			continue
		}
		runCounts = append(runCounts, agentRunCountItem{AgentID: uuidString(row.AgentID), RunCount: row.RunCount})
	}
	activityRows, err := h.q.GetWorkspaceAgentActivity30d(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	activity := make([]agentActivityBucket, 0, len(activityRows))
	for _, row := range activityRows {
		if _, ok := allowedIDs[uuidString(row.AgentID)]; !ok {
			continue
		}
		activity = append(activity, agentActivityBucket{
			AgentID: uuidString(row.AgentID), BucketAt: timeString(row.Bucket),
			TaskCount: row.TaskCount, FailedCount: row.FailedCount,
		})
	}
	writeJSON(w, http.StatusOK, agentSnapshotResponse{
		ScopeCounts: counts, Agents: agentItems, Tasks: taskItems, RunCounts: runCounts, Activity: activity,
		FilterMetadata: agentFilterMetadata{Owners: filterOwners, Runtimes: filterRuntimes},
	})
}

func agentSnapshotDTO(agent lwdb.Agent) agentSnapshotItem {
	return agentSnapshotItem{
		ID: uuidString(agent.ID), WorkspaceID: uuidString(agent.WorkspaceID), OwnerID: uuidString(agent.OwnerID),
		Name: agent.Name, Description: agent.Description, AvatarURL: textPointer(agent.AvatarUrl),
		RuntimeID: optionalUUIDPointer(agent.RuntimeID), Status: agent.Status, PermissionMode: agent.PermissionMode,
		Model: textPointer(agent.Model), ThinkingLevel: textPointer(agent.ThinkingLevel), ServiceTier: textPointer(agent.ServiceTier),
		MaxConcurrentTasks: agent.MaxConcurrentTasks, ArchivedAt: timePointer(agent.ArchivedAt), UpdatedAt: timeString(agent.UpdatedAt),
	}
}

func agentSnapshotTaskDTO(task lwdb.AgentTaskQueue) agentSnapshotTask {
	return agentSnapshotTask{
		ID: uuidString(task.ID), AgentID: uuidString(task.AgentID), Status: task.Status,
		IssueID: optionalUUIDPointer(task.IssueID), ChatSessionID: optionalUUIDPointer(task.ChatSessionID),
		FailureReason: optionalStringPointer(task.FailureReason), StartedAt: timePointer(task.StartedAt),
		CompletedAt: timePointer(task.CompletedAt), CreatedAt: timeString(task.CreatedAt),
	}
}

func optionalStringPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func (h *Handler) ListAgentTasks(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	_, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return
	}
	if !h.humanCanViewAgent(r, agent, userID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	limit, cursorAt, cursorID, err := parseChatPage(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	rows, err := h.q.ListAgentTasksPage(r.Context(), lwdb.ListAgentTasksPageParams{
		WorkspaceID: workspaceID, AgentID: agent.ID, CursorCreatedAt: cursorAt, CursorID: cursorID, PageLimit: limit + 1,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]taskLifecycleResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, taskLifecycleDTO(row))
	}
	summaryRow, err := h.q.GetAgentTaskSummary30d(r.Context(), lwdb.GetAgentTaskSummary30dParams{WorkspaceID: workspaceID, AgentID: agent.ID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	successRate := 0.0
	if summaryRow.RunCount > 0 {
		successRate = float64(summaryRow.SuccessCount) / float64(summaryRow.RunCount)
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		value := encodeChatCursor(rows[len(rows)-1].CreatedAt, rows[len(rows)-1].ID)
		nextCursor = &value
	}
	writeJSON(w, http.StatusOK, agentTasksPageResponse{
		Items: items, NextCursor: nextCursor,
		Summary30d: agentTaskSummary30d{
			RunCount: summaryRow.RunCount, SuccessCount: summaryRow.SuccessCount, FailCount: summaryRow.FailCount,
			SuccessRate: successRate, AvgDurationMs: summaryRow.AvgDurationMs,
		},
	})
}

func (h *Handler) CancelAgentTasks(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, agent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	principal, _ := principalFromContext(r.Context())
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	cancelled, err := h.cancelAgentTasksTx(r.Context(), qtx, agent.WorkspaceID, agent.ID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	for _, task := range cancelled {
		h.publish(protocol.EventTaskCancelled, uuidString(task.WorkspaceID), "member", principal.UserID, taskLifecycleDTO(task))
		if initiator := cancelledChatInitiator(r.Context(), h.q, task); initiator != "" {
			h.publishCancelledChatFinalized(task, initiator)
		}
	}
	writeJSON(w, http.StatusOK, cancelAgentTasksResponse{Cancelled: len(cancelled)})
}

func (h *Handler) cancelAgentTasksTx(ctx context.Context, qtx *lwdb.Queries, workspaceID, agentID pgtype.UUID) ([]lwdb.AgentTaskQueue, error) {
	cancelled, err := qtx.CancelAgentTasksByAgent(ctx, lwdb.CancelAgentTasksByAgentParams{WorkspaceID: workspaceID, AgentID: agentID})
	if err != nil {
		return nil, err
	}
	for _, task := range cancelled {
		if _, err := createCancelledChatDraft(ctx, qtx, task); err != nil {
			return nil, err
		}
		if _, err := qtx.DeleteTaskTokensByTask(ctx, task.ID); err != nil {
			return nil, err
		}
	}
	return cancelled, nil
}

func broadcastAgentSummary(response agentResponse) map[string]any {
	return map[string]any{
		"id": response.ID, "workspace_id": response.WorkspaceID, "owner_id": response.OwnerID,
		"name": response.Name, "description": response.Description, "instructions": response.Instructions,
		"avatar_url": response.AvatarURL, "runtime_id": response.RuntimeID, "status": response.Status, "permission_mode": response.PermissionMode,
		"max_concurrent_tasks": response.MaxConcurrentTasks, "model": response.Model,
		"thinking_level": response.ThinkingLevel, "service_tier": response.ServiceTier,
		"archived_at": response.ArchivedAt, "archived_by": response.ArchivedBy,
		"skills": response.Skills, "invocation_targets": response.InvocationTargets,
		"custom_env_keys": response.CustomEnvKeys, "mcp_configured": response.MCPConfigured,
		"disabled_runtime_skills": response.DisabledRuntimeSkills, "created_at": response.CreatedAt, "updated_at": response.UpdatedAt,
	}
}
