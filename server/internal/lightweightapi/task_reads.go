package lightweightapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

type taskUsageResponse struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	CostUSDTicks     *int64 `json:"cost_usd_ticks"`
}

func taskUsageDTO(usage lwdb.TaskUsage) taskUsageResponse {
	var cost *int64
	if usage.CostUsdTicks.Valid {
		value := usage.CostUsdTicks.Int64
		cost = &value
	}
	return taskUsageResponse{
		Provider: usage.Provider, Model: usage.Model, InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens, CostUSDTicks: cost,
	}
}

type taskRunData struct {
	id, workspaceID, agentID, runtimeID  pgtype.UUID
	issueID, squadID, parentTaskID       pgtype.UUID
	agentName, runtimeName, squadName    string
	isLeader                             bool
	status                               string
	priority, attempt, maxAttempts       int32
	sessionID, workDir                   pgtype.Text
	result                               []byte
	errorText, failureReason             pgtype.Text
	triggerSummary, waitReason           pgtype.Text
	dispatchedAt, startedAt, completedAt pgtype.Timestamptz
	createdAt                            pgtype.Timestamptz
}

func taskRunPageData(row lwdb.ListTaskRunsPageRow) taskRunData {
	return taskRunData{
		id: row.ID, workspaceID: row.WorkspaceID, agentID: row.AgentID, runtimeID: row.RuntimeID,
		issueID: row.IssueID, squadID: row.SquadID, parentTaskID: row.ParentTaskID,
		agentName: row.AgentName, runtimeName: row.RuntimeName, squadName: row.SquadName,
		isLeader: row.IsLeaderTask, status: row.Status, priority: row.Priority, attempt: row.Attempt, maxAttempts: row.MaxAttempts,
		sessionID: row.SessionID, workDir: row.WorkDir, result: row.Result, errorText: row.Error,
		failureReason: row.FailureReason, triggerSummary: row.TriggerSummary, waitReason: row.WaitReason,
		dispatchedAt: row.DispatchedAt, startedAt: row.StartedAt, completedAt: row.CompletedAt, createdAt: row.CreatedAt,
	}
}

func activeTaskRunData(row lwdb.ListActiveTaskRunsRow) taskRunData {
	return taskRunData{
		id: row.ID, workspaceID: row.WorkspaceID, agentID: row.AgentID, runtimeID: row.RuntimeID,
		issueID: row.IssueID, squadID: row.SquadID, parentTaskID: row.ParentTaskID,
		agentName: row.AgentName, runtimeName: row.RuntimeName, squadName: row.SquadName,
		isLeader: row.IsLeaderTask, status: row.Status, priority: row.Priority, attempt: row.Attempt, maxAttempts: row.MaxAttempts,
		sessionID: row.SessionID, workDir: row.WorkDir, result: row.Result, errorText: row.Error,
		failureReason: row.FailureReason, triggerSummary: row.TriggerSummary, waitReason: row.WaitReason,
		dispatchedAt: row.DispatchedAt, startedAt: row.StartedAt, completedAt: row.CompletedAt, createdAt: row.CreatedAt,
	}
}

type taskRunNamedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type taskRunResponse struct {
	ID             string              `json:"id"`
	WorkspaceID    string              `json:"workspace_id"`
	IssueID        string              `json:"issue_id"`
	Agent          taskRunNamedRef     `json:"agent"`
	Runtime        taskRunNamedRef     `json:"runtime"`
	Squad          *taskRunNamedRef    `json:"squad"`
	IsLeaderTask   bool                `json:"is_leader_task"`
	Status         string              `json:"status"`
	Priority       int32               `json:"priority"`
	Attempt        int32               `json:"attempt"`
	MaxAttempts    int32               `json:"max_attempts"`
	ParentTaskID   *string             `json:"parent_task_id"`
	SessionID      *string             `json:"session_id"`
	WorkDir        *string             `json:"work_dir"`
	Result         json.RawMessage     `json:"result"`
	Error          *string             `json:"error"`
	FailureReason  *string             `json:"failure_reason"`
	TriggerSummary *string             `json:"trigger_summary"`
	WaitReason     *string             `json:"wait_reason"`
	Usage          []taskUsageResponse `json:"usage"`
	DispatchedAt   *string             `json:"dispatched_at"`
	StartedAt      *string             `json:"started_at"`
	CompletedAt    *string             `json:"completed_at"`
	CreatedAt      string              `json:"created_at"`
}

func taskRunDTO(task taskRunData, usage []taskUsageResponse) taskRunResponse {
	result := json.RawMessage(nil)
	if len(task.result) > 0 && json.Valid(task.result) {
		result = append(json.RawMessage(nil), task.result...)
	}
	var squad *taskRunNamedRef
	if task.squadID.Valid {
		squad = &taskRunNamedRef{ID: uuidString(task.squadID), Name: task.squadName}
	}
	if usage == nil {
		usage = []taskUsageResponse{}
	}
	return taskRunResponse{
		ID: uuidString(task.id), WorkspaceID: uuidString(task.workspaceID), IssueID: uuidString(task.issueID),
		Agent:   taskRunNamedRef{ID: uuidString(task.agentID), Name: task.agentName},
		Runtime: taskRunNamedRef{ID: uuidString(task.runtimeID), Name: task.runtimeName}, Squad: squad,
		IsLeaderTask: task.isLeader, Status: task.status, Priority: task.priority, Attempt: task.attempt,
		MaxAttempts: task.maxAttempts, ParentTaskID: optionalUUIDPointer(task.parentTaskID),
		SessionID: textPointer(task.sessionID), WorkDir: textPointer(task.workDir), Result: result,
		Error: textPointer(task.errorText), FailureReason: textPointer(task.failureReason),
		TriggerSummary: textPointer(task.triggerSummary), WaitReason: textPointer(task.waitReason), Usage: usage,
		DispatchedAt: timePointer(task.dispatchedAt), StartedAt: timePointer(task.startedAt),
		CompletedAt: timePointer(task.completedAt), CreatedAt: timeString(task.createdAt),
	}
}

func loadTaskUsage(ctxRequest *http.Request, queries *lwdb.Queries, ids []pgtype.UUID) (map[string][]taskUsageResponse, error) {
	result := make(map[string][]taskUsageResponse, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := queries.ListTaskUsageForTasks(ctxRequest.Context(), ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		key := uuidString(row.TaskID)
		result[key] = append(result[key], taskUsageDTO(row))
	}
	return result, nil
}

func rejectTaskPrincipalForRunHistory(w http.ResponseWriter, principal Principal) bool {
	if principal.Kind != principalTask {
		return false
	}
	writeCode(w, http.StatusForbidden, "forbidden")
	return true
}

func (h *Handler) ListTaskRuns(w http.ResponseWriter, r *http.Request) {
	issue, workspaceID, principal, ok := h.issueScope(w, r)
	if !ok || rejectTaskPrincipalForRunHistory(w, principal) {
		return
	}
	for key := range r.URL.Query() {
		if key != "limit" && key != "cursor" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	limit, cursorAt, cursorID, err := parseChatPage(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	rows, err := h.q.ListTaskRunsPage(r.Context(), lwdb.ListTaskRunsPageParams{
		WorkspaceID: workspaceID, IssueID: issue.ID, CursorCreatedAt: cursorAt, CursorID: cursorID, PageLimit: limit + 1,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	ids := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	usageByTask, err := loadTaskUsage(r, h.q, ids)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	items := make([]taskRunResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, taskRunDTO(taskRunPageData(row), usageByTask[uuidString(row.ID)]))
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		value := encodeChatCursor(rows[len(rows)-1].CreatedAt, rows[len(rows)-1].ID)
		nextCursor = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextCursor})
}

func (h *Handler) GetActiveIssueTasks(w http.ResponseWriter, r *http.Request) {
	issue, workspaceID, principal, ok := h.issueScope(w, r)
	if !ok || rejectTaskPrincipalForRunHistory(w, principal) {
		return
	}
	rows, err := h.q.ListActiveTaskRuns(r.Context(), lwdb.ListActiveTaskRunsParams{WorkspaceID: workspaceID, IssueID: issue.ID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	ids := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	usageByTask, err := loadTaskUsage(r, h.q, ids)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	items := make([]taskRunResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, taskRunDTO(activeTaskRunData(row), usageByTask[uuidString(row.ID)]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items})
}

type taskMessageResponse struct {
	ID        string          `json:"id"`
	TaskID    string          `json:"task_id"`
	Seq       int32           `json:"seq"`
	Type      string          `json:"type"`
	Tool      *string         `json:"tool"`
	Content   *string         `json:"content"`
	Input     json.RawMessage `json:"input"`
	Output    *string         `json:"output"`
	CreatedAt string          `json:"created_at"`
}

func taskMessageReadDTO(message lwdb.TaskMessage) taskMessageResponse {
	input := json.RawMessage(nil)
	if len(message.Input) > 0 && json.Valid(message.Input) {
		input = append(json.RawMessage(nil), message.Input...)
	}
	return taskMessageResponse{
		ID: uuidString(message.ID), TaskID: uuidString(message.TaskID), Seq: message.Seq, Type: message.Type,
		Tool: textPointer(message.Tool), Content: textPointer(message.Content), Input: input,
		Output: textPointer(message.Output), CreatedAt: timeString(message.CreatedAt),
	}
}

func (h *Handler) taskMessageReadScope(w http.ResponseWriter, r *http.Request) (lwdb.AgentTaskQueue, bool) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return lwdb.AgentTaskQueue{}, false
	}
	taskID, err := parseUUID(chi.URLParam(r, "taskId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.AgentTaskQueue{}, false
	}
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return lwdb.AgentTaskQueue{}, false
	}
	if principal.Kind == principalTask && principal.TaskID != uuidString(taskID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return lwdb.AgentTaskQueue{}, false
	}
	task, err := h.q.GetAgentTask(r.Context(), lwdb.GetAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.AgentTaskQueue{}, false
	}
	return task, true
}

func (h *Handler) ListTaskMessagesForUser(w http.ResponseWriter, r *http.Request) {
	task, ok := h.taskMessageReadScope(w, r)
	if !ok {
		return
	}
	for key := range r.URL.Query() {
		if key != "limit" && key != "cursor" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	limit, cursorAt, cursorID, err := parseChatPage(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	rows, err := h.q.ListTaskMessagesPage(r.Context(), lwdb.ListTaskMessagesPageParams{
		TaskID: task.ID, CursorCreatedAt: cursorAt, CursorID: cursorID, PageLimit: limit + 1,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]taskMessageResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, taskMessageReadDTO(row))
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		value := encodeChatCursor(rows[len(rows)-1].CreatedAt, rows[len(rows)-1].ID)
		nextCursor = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextCursor})
}
