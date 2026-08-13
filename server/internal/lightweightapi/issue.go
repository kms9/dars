package lightweightapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const (
	issueTitleMaxLength      = 500
	issueIdempotencyMaxBytes = 255
)

var issueStatuses = map[string]struct{}{
	"backlog": {}, "todo": {}, "in_progress": {}, "in_review": {},
	"done": {}, "blocked": {}, "cancelled": {},
}

type issueResponse struct {
	ID                 string          `json:"id"`
	WorkspaceID        string          `json:"workspace_id"`
	Identifier         string          `json:"identifier"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Status             string          `json:"status"`
	AssigneeType       string          `json:"assignee_type"`
	AssigneeID         string          `json:"assignee_id"`
	CreatorType        string          `json:"creator_type"`
	CreatorID          string          `json:"creator_id"`
	AcceptanceCriteria json.RawMessage `json:"acceptance_criteria"`
	ContextRefs        json.RawMessage `json:"context_refs"`
	Number             int32           `json:"number"`
	FirstExecutedAt    *string         `json:"first_executed_at"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

func issueDTO(issue lwdb.Issue, prefix string) issueResponse {
	acceptanceCriteria := json.RawMessage(issue.AcceptanceCriteria)
	if !json.Valid(acceptanceCriteria) {
		acceptanceCriteria = json.RawMessage(`[]`)
	}
	contextRefs := json.RawMessage(issue.ContextRefs)
	if !json.Valid(contextRefs) {
		contextRefs = json.RawMessage(`[]`)
	}
	return issueResponse{
		ID: uuidString(issue.ID), WorkspaceID: uuidString(issue.WorkspaceID),
		Identifier: fmt.Sprintf("%s-%d", prefix, issue.Number), Title: issue.Title, Description: issue.Description,
		Status: issue.Status, AssigneeType: issue.AssigneeType, AssigneeID: uuidString(issue.AssigneeID),
		CreatorType: issue.CreatorType, CreatorID: uuidString(issue.CreatorID),
		AcceptanceCriteria: acceptanceCriteria, ContextRefs: contextRefs, Number: issue.Number,
		FirstExecutedAt: timePointer(issue.FirstExecutedAt), CreatedAt: timeString(issue.CreatedAt), UpdatedAt: timeString(issue.UpdatedAt),
	}
}

type issueMutation struct {
	Title              json.RawMessage `json:"title"`
	Description        json.RawMessage `json:"description"`
	Status             json.RawMessage `json:"status"`
	AssigneeType       json.RawMessage `json:"assignee_type"`
	AssigneeID         json.RawMessage `json:"assignee_id"`
	AcceptanceCriteria json.RawMessage `json:"acceptance_criteria"`
	ContextRefs        json.RawMessage `json:"context_refs"`
}

type issueMutationValues struct {
	setTitle              bool
	title                 string
	setDescription        bool
	description           string
	setStatus             bool
	status                string
	setAssignee           bool
	assigneeType          string
	assigneeID            pgtype.UUID
	setAcceptanceCriteria bool
	acceptanceCriteria    []byte
	setContextRefs        bool
	contextRefs           []byte
}

func decodeIssueString(raw json.RawMessage, trim bool) (string, bool, bool) {
	if raw == nil {
		return "", false, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", true, false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", true, false
	}
	if trim {
		value = strings.TrimSpace(value)
	}
	return value, true, true
}

func decodeIssueArray(raw json.RawMessage) ([]byte, bool, bool) {
	if raw == nil {
		return nil, false, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true, false
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || items == nil {
		return nil, true, false
	}
	normalized, err := json.Marshal(items)
	return normalized, true, err == nil
}

func parseIssueMutation(r *http.Request, create bool) (issueMutationValues, bool) {
	var body issueMutation
	if decodeStrictTaskBody(r, &body) != nil {
		return issueMutationValues{}, false
	}
	var values issueMutationValues
	var ok bool
	values.title, values.setTitle, ok = decodeIssueString(body.Title, true)
	if !ok || (values.setTitle && (values.title == "" || len([]rune(values.title)) > issueTitleMaxLength)) {
		return issueMutationValues{}, false
	}
	values.description, values.setDescription, ok = decodeIssueString(body.Description, false)
	if !ok {
		return issueMutationValues{}, false
	}
	values.status, values.setStatus, ok = decodeIssueString(body.Status, true)
	if !ok {
		return issueMutationValues{}, false
	}
	if values.setStatus {
		if _, valid := issueStatuses[values.status]; !valid {
			return issueMutationValues{}, false
		}
	}
	assigneeType, setAssigneeType, ok := decodeIssueString(body.AssigneeType, true)
	if !ok {
		return issueMutationValues{}, false
	}
	assigneeID, setAssigneeID, ok := decodeIssueString(body.AssigneeID, true)
	if !ok || setAssigneeType != setAssigneeID {
		return issueMutationValues{}, false
	}
	if setAssigneeType {
		if assigneeType != "agent" && assigneeType != "squad" {
			return issueMutationValues{}, false
		}
		parsed, err := parseUUID(assigneeID)
		if err != nil {
			return issueMutationValues{}, false
		}
		values.setAssignee, values.assigneeType, values.assigneeID = true, assigneeType, parsed
	}
	values.acceptanceCriteria, values.setAcceptanceCriteria, ok = decodeIssueArray(body.AcceptanceCriteria)
	if !ok {
		return issueMutationValues{}, false
	}
	values.contextRefs, values.setContextRefs, ok = decodeIssueArray(body.ContextRefs)
	if !ok {
		return issueMutationValues{}, false
	}
	if create && (!values.setTitle || !values.setStatus || !values.setAssignee) {
		return issueMutationValues{}, false
	}
	if !create && !values.setTitle && !values.setDescription && !values.setStatus && !values.setAssignee && !values.setAcceptanceCriteria && !values.setContextRefs {
		return issueMutationValues{}, false
	}
	return values, true
}

func issueCreateDigest(values issueMutationValues) string {
	acceptanceCriteria := json.RawMessage(`[]`)
	if values.setAcceptanceCriteria {
		acceptanceCriteria = values.acceptanceCriteria
	}
	contextRefs := json.RawMessage(`[]`)
	if values.setContextRefs {
		contextRefs = values.contextRefs
	}
	description := ""
	if values.setDescription {
		description = values.description
	}
	return requestDigest(map[string]any{
		"title": values.title, "description": description, "status": values.status,
		"assignee_type": values.assigneeType, "assignee_id": uuidString(values.assigneeID),
		"acceptance_criteria": acceptanceCriteria, "context_refs": contextRefs,
	})
}

type issueAssigneeResolution struct {
	agentID     pgtype.UUID
	runtimeID   pgtype.UUID
	squadID     pgtype.UUID
	leader      bool
	displayType string
}

type issueResolutionError struct {
	status int
	code   string
}

func (err *issueResolutionError) Error() string { return err.code }

func issueResolutionFailure(status int, code string) error {
	return &issueResolutionError{status: status, code: code}
}

func writeIssueResolutionError(w http.ResponseWriter, err error) {
	var resolution *issueResolutionError
	if errors.As(err, &resolution) {
		writeCode(w, resolution.status, resolution.code)
		return
	}
	writeCode(w, http.StatusInternalServerError, "internal_error")
}

func resolveIssueAgent(ctx context.Context, queries *lwdb.Queries, workspaceID, userID, agentID pgtype.UUID) (issueAssigneeResolution, error) {
	initial, err := queries.GetAgent(ctx, lwdb.GetAgentParams{ID: agentID, WorkspaceID: workspaceID})
	if err != nil {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusNotFound, "not_found")
	}
	if initial.ArchivedAt.Valid {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "agent_archived")
	}
	if !initial.RuntimeID.Valid {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "agent_runtime_required")
	}
	runtime, err := queries.LockAgentRuntime(ctx, lwdb.LockAgentRuntimeParams{ID: initial.RuntimeID, WorkspaceID: workspaceID})
	if err != nil {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "agent_runtime_required")
	}
	agent, err := queries.LockAgent(ctx, lwdb.LockAgentParams{ID: agentID, WorkspaceID: workspaceID})
	if err != nil {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "conflict")
	}
	if agent.ArchivedAt.Valid {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "agent_archived")
	}
	if !agent.RuntimeID.Valid || agent.RuntimeID != runtime.ID {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "agent_runtime_required")
	}
	if !humanCanInvokeAgentWithQueries(ctx, queries, agent, userID) {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusForbidden, "invocation_forbidden")
	}
	return issueAssigneeResolution{agentID: agent.ID, runtimeID: runtime.ID, displayType: "agent"}, nil
}

// resolveIssueAssignee 将 Issue 的负责人解析为可入队的 Agent + Runtime。
// assignee_type=squad 时：锁小队、校验恰好一名 leader，并解析为 Leader Agent（leader=true）。
// assignee_type=agent 时：直接解析该 Agent。归档小队或 leader 不一致会返回冲突错误。
func resolveIssueAssignee(ctx context.Context, queries *lwdb.Queries, workspaceID, userID pgtype.UUID, assigneeType string, assigneeID pgtype.UUID) (issueAssigneeResolution, error) {
	if assigneeType == "agent" {
		return resolveIssueAgent(ctx, queries, workspaceID, userID, assigneeID)
	}
	if assigneeType != "squad" {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusBadRequest, "invalid_argument")
	}
	squad, err := queries.LockSquad(ctx, lwdb.LockSquadParams{ID: assigneeID, WorkspaceID: workspaceID})
	if err != nil {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusNotFound, "not_found")
	}
	if squad.ArchivedAt.Valid {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "squad_archived")
	}
	members, err := queries.ListSquadMembers(ctx, squad.ID)
	if err != nil {
		return issueAssigneeResolution{}, err
	}
	leaders := 0
	for _, member := range members {
		if member.Role == "leader" {
			leaders++
			if member.AgentID != squad.LeaderID {
				return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "conflict")
			}
		}
	}
	if leaders != 1 {
		return issueAssigneeResolution{}, issueResolutionFailure(http.StatusConflict, "conflict")
	}
	resolved, err := resolveIssueAgent(ctx, queries, workspaceID, userID, squad.LeaderID)
	if err != nil {
		return issueAssigneeResolution{}, err
	}
	resolved.squadID, resolved.leader, resolved.displayType = squad.ID, true, "squad"
	return resolved, nil
}

func issueTaskContext(issue lwdb.Issue, identifier string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"issue_id":            uuidString(issue.ID),
		"identifier":          identifier,
		"title":               issue.Title,
		"description":         issue.Description,
		"acceptance_criteria": json.RawMessage(issue.AcceptanceCriteria),
		"context_refs":        json.RawMessage(issue.ContextRefs),
	})
	return payload
}

// enqueueInitialIssueTask 为 Issue 写入首条 agent_task_queue，并标记 first_executed。
// 小队场景下 agent_id=Leader、squad_id 有值、is_leader_task=true；不会给每个成员各入一队。
func (h *Handler) enqueueInitialIssueTask(ctx context.Context, queries *lwdb.Queries, issue lwdb.Issue, prefix string, userID pgtype.UUID, source string, assignee issueAssigneeResolution) (lwdb.Issue, lwdb.AgentTaskQueue, error) {
	if err := h.requireGatewayProviderForNewTask(ctx, queries, issue.WorkspaceID, assignee.agentID, assignee.runtimeID); err != nil {
		return lwdb.Issue{}, lwdb.AgentTaskQueue{}, err
	}
	task, err := queries.CreateInitialIssueTask(ctx, lwdb.CreateInitialIssueTaskParams{
		WorkspaceID: issue.WorkspaceID, AgentID: assignee.agentID, RuntimeID: assignee.runtimeID, IssueID: issue.ID,
		SquadID: assignee.squadID, IsLeaderTask: assignee.leader, Context: issueTaskContext(issue, fmt.Sprintf("%s-%d", prefix, issue.Number)),
		UserID: userID, OriginatorSource: pgtype.Text{String: source, Valid: true},
	})
	if err != nil {
		return lwdb.Issue{}, lwdb.AgentTaskQueue{}, err
	}
	updated, err := queries.MarkIssueFirstExecuted(ctx, lwdb.MarkIssueFirstExecutedParams{ID: issue.ID, WorkspaceID: issue.WorkspaceID})
	if err != nil {
		return lwdb.Issue{}, lwdb.AgentTaskQueue{}, err
	}
	return updated, task, nil
}

func (h *Handler) issueScope(w http.ResponseWriter, r *http.Request) (lwdb.Issue, pgtype.UUID, Principal, bool) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return lwdb.Issue{}, pgtype.UUID{}, Principal{}, false
	}
	issueID, err := parseUUID(chi.URLParam(r, "issueId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.Issue{}, pgtype.UUID{}, Principal{}, false
	}
	issue, err := h.q.GetIssue(r.Context(), lwdb.GetIssueParams{ID: issueID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.Issue{}, pgtype.UUID{}, Principal{}, false
	}
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return lwdb.Issue{}, pgtype.UUID{}, Principal{}, false
	}
	if principal.Kind == principalTask {
		taskID, parseErr := parseUUID(principal.TaskID)
		if parseErr != nil {
			writeCode(w, http.StatusForbidden, "forbidden")
			return lwdb.Issue{}, pgtype.UUID{}, Principal{}, false
		}
		task, taskErr := h.q.GetAgentTask(r.Context(), lwdb.GetAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
		if taskErr != nil || !task.IssueID.Valid || task.IssueID != issue.ID {
			writeCode(w, http.StatusForbidden, "forbidden")
			return lwdb.Issue{}, pgtype.UUID{}, Principal{}, false
		}
	}
	return issue, workspaceID, principal, true
}

func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	allowedQuery := map[string]struct{}{"limit": {}, "cursor": {}, "status": {}, "assignee_type": {}, "assignee_id": {}, "creator_type": {}, "creator_id": {}}
	for key := range r.URL.Query() {
		if _, allowed := allowedQuery[key]; !allowed {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	limit, cursorAt, cursorID, err := parseChatPage(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	statuses := make([]string, 0, len(r.URL.Query()["status"]))
	seenStatuses := make(map[string]struct{}, len(r.URL.Query()["status"]))
	for _, rawStatus := range r.URL.Query()["status"] {
		status := strings.TrimSpace(rawStatus)
		if _, valid := issueStatuses[status]; !valid {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		if _, duplicate := seenStatuses[status]; !duplicate {
			statuses = append(statuses, status)
			seenStatuses[status] = struct{}{}
		}
	}
	if len(r.URL.Query()["assignee_type"]) > 1 || len(r.URL.Query()["assignee_id"]) > 1 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if len(r.URL.Query()["creator_type"]) > 1 || len(r.URL.Query()["creator_id"]) > 1 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	var assigneeType pgtype.Text
	if raw := strings.TrimSpace(r.URL.Query().Get("assignee_type")); raw != "" {
		if raw != "agent" && raw != "squad" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		assigneeType = pgtype.Text{String: raw, Valid: true}
	}
	var assigneeID pgtype.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("assignee_id")); raw != "" {
		if !assigneeType.Valid {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		assigneeID, err = parseUUID(raw)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	var creatorType pgtype.Text
	if raw := strings.TrimSpace(r.URL.Query().Get("creator_type")); raw != "" {
		if raw != "member" && raw != "agent" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		creatorType = pgtype.Text{String: raw, Valid: true}
	}
	var creatorID pgtype.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("creator_id")); raw != "" {
		if !creatorType.Valid {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		creatorID, err = parseUUID(raw)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	if creatorType.Valid && creatorType.String == "agent" {
		_, _, userID, ok := h.chatHumanScope(w, r)
		if !ok {
			return
		}
		agent, err := h.q.GetAgent(r.Context(), lwdb.GetAgentParams{ID: creatorID, WorkspaceID: workspaceID})
		if err != nil || !h.humanCanViewAgent(r, agent, userID) {
			writeCode(w, http.StatusForbidden, "forbidden")
			return
		}
	} else if creatorType.Valid && creatorType.String == "member" {
		if _, err := h.q.GetMemberByID(r.Context(), lwdb.GetMemberByIDParams{ID: creatorID, WorkspaceID: workspaceID}); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	rows, err := h.q.ListIssuesPage(r.Context(), lwdb.ListIssuesPageParams{
		WorkspaceID: workspaceID, CursorUpdatedAt: cursorAt, CursorID: cursorID, Statuses: statuses,
		AssigneeType: assigneeType, AssigneeID: assigneeID, CreatorType: creatorType, CreatorID: creatorID, PageLimit: limit + 1,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	workspace, err := h.q.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	items := make([]issueResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, issueDTO(row, workspace.IssuePrefix))
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		value := encodeChatCursor(rows[len(rows)-1].UpdatedAt, rows[len(rows)-1].ID)
		nextCursor = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextCursor})
}

// CreateIssue 创建 Issue（Run）。人类 Web/CLI 入口。
// 流程：幂等校验 → resolveIssueAssignee → 写 Issue；
// 当 status=todo 时 enqueueInitialIssueTask（小队则只入队 Leader）→
// commit 后 publish issue:created，若已入队则 announceQueuedTask 唤醒 Daemon。
func (h *Handler) CreateIssue(w http.ResponseWriter, r *http.Request) {
	principal, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return
	}
	values, valid := parseIssueMutation(r, true)
	if !valid {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > issueIdempotencyMaxBytes {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	requestHash := issueCreateDigest(values)
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	key := pgtype.Text{String: idempotencyKey, Valid: true}
	existing, err := qtx.GetIssueByIdempotencyKey(r.Context(), lwdb.GetIssueByIdempotencyKeyParams{
		WorkspaceID: workspaceID, CreatorType: "member", CreatorID: userID, IdempotencyKey: key,
	})
	if err == nil {
		if !existing.RequestHash.Valid || existing.RequestHash.String != requestHash {
			writeCode(w, http.StatusConflict, "idempotency_key_reused")
			return
		}
		workspace, workspaceErr := qtx.GetWorkspace(r.Context(), workspaceID)
		if workspaceErr != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		writeJSON(w, http.StatusCreated, issueDTO(existing, workspace.IssuePrefix))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	assignee, err := resolveIssueAssignee(r.Context(), qtx, workspaceID, userID, values.assigneeType, values.assigneeID)
	if err != nil {
		writeIssueResolutionError(w, err)
		return
	}
	counter, err := qtx.LockWorkspaceIssueCounter(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	// The Workspace counter lock is the serialization fence for concurrent
	// creates, including same-key requests that resolved different assignees.
	// Re-read after acquiring it so the loser replays/conflicts instead of
	// surfacing the partial unique index as a 500.
	existing, err = qtx.GetIssueByIdempotencyKey(r.Context(), lwdb.GetIssueByIdempotencyKeyParams{
		WorkspaceID: workspaceID, CreatorType: "member", CreatorID: userID, IdempotencyKey: key,
	})
	if err == nil {
		if !existing.RequestHash.Valid || existing.RequestHash.String != requestHash {
			writeCode(w, http.StatusConflict, "idempotency_key_reused")
			return
		}
		writeJSON(w, http.StatusCreated, issueDTO(existing, counter.IssuePrefix))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	description := ""
	if values.setDescription {
		description = values.description
	}
	acceptanceCriteria := []byte(`[]`)
	if values.setAcceptanceCriteria {
		acceptanceCriteria = values.acceptanceCriteria
	}
	contextRefs := []byte(`[]`)
	if values.setContextRefs {
		contextRefs = values.contextRefs
	}
	issue, err := qtx.CreateIssue(r.Context(), lwdb.CreateIssueParams{
		WorkspaceID: workspaceID, Title: values.title, Description: description, Status: values.status,
		AssigneeType: values.assigneeType, AssigneeID: values.assigneeID, CreatorType: "member", CreatorID: userID,
		AcceptanceCriteria: acceptanceCriteria, ContextRefs: contextRefs, Number: counter.IssueCounter,
		IdempotencyKey: key, RequestHash: pgtype.Text{String: requestHash, Valid: true},
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	var task *lwdb.AgentTaskQueue
	if issue.Status == "todo" {
		updated, queued, enqueueErr := h.enqueueInitialIssueTask(r.Context(), qtx, issue, counter.IssuePrefix, userID, "manual", assignee)
		if enqueueErr != nil {
			writeIssueResolutionError(w, enqueueErr)
			return
		}
		issue, task = updated, &queued
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := issueDTO(issue, counter.IssuePrefix)
	h.publish(protocol.EventIssueCreated, uuidString(workspaceID), "member", principal.UserID, map[string]any{"issue": response})
	if task != nil {
		h.announceQueuedTask(*task)
	}
	writeJSON(w, http.StatusCreated, response)
}

func (h *Handler) GetIssue(w http.ResponseWriter, r *http.Request) {
	issue, workspaceID, _, ok := h.issueScope(w, r)
	if !ok {
		return
	}
	workspace, err := h.q.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, issueDTO(issue, workspace.IssuePrefix))
}

func (h *Handler) UpdateIssue(w http.ResponseWriter, r *http.Request) {
	issue, workspaceID, principal, ok := h.issueScope(w, r)
	if !ok {
		return
	}
	values, valid := parseIssueMutation(r, false)
	if !valid {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if principal.Kind == principalTask && (values.setTitle || values.setDescription || values.setAcceptanceCriteria || values.setContextRefs) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	locked, err := qtx.LockIssue(r.Context(), lwdb.LockIssueParams{ID: issue.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	assigneeType, assigneeID := locked.AssigneeType, locked.AssigneeID
	if values.setAssignee {
		assigneeType, assigneeID = values.assigneeType, values.assigneeID
	}
	targetStatus := locked.Status
	if values.setStatus {
		targetStatus = values.status
	}
	needsInitialTask := targetStatus == "todo" && !locked.FirstExecutedAt.Valid
	var assignee issueAssigneeResolution
	if values.setAssignee || needsInitialTask {
		assignee, err = resolveIssueAssignee(r.Context(), qtx, workspaceID, userID, assigneeType, assigneeID)
		if err != nil {
			writeIssueResolutionError(w, err)
			return
		}
	}
	updated, err := qtx.UpdateIssue(r.Context(), lwdb.UpdateIssueParams{
		SetTitle: values.setTitle, Title: values.title, SetDescription: values.setDescription, Description: values.description,
		SetStatus: values.setStatus, Status: values.status, SetAssigneeType: values.setAssignee, AssigneeType: values.assigneeType,
		SetAssigneeID: values.setAssignee, AssigneeID: values.assigneeID,
		SetAcceptanceCriteria: values.setAcceptanceCriteria, AcceptanceCriteria: values.acceptanceCriteria,
		SetContextRefs: values.setContextRefs, ContextRefs: values.contextRefs, ID: issue.ID, WorkspaceID: workspaceID,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	workspace, err := qtx.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	var task *lwdb.AgentTaskQueue
	if needsInitialTask {
		source := "manual"
		if principal.Kind == principalTask {
			source = "agent"
		}
		executed, queued, enqueueErr := h.enqueueInitialIssueTask(r.Context(), qtx, updated, workspace.IssuePrefix, userID, source, assignee)
		if enqueueErr != nil {
			writeIssueResolutionError(w, enqueueErr)
			return
		}
		updated, task = executed, &queued
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	actorType, actorID := "member", principal.UserID
	if principal.Kind == principalTask {
		actorType, actorID = "agent", principal.AgentID
	}
	response := issueDTO(updated, workspace.IssuePrefix)
	h.publish(protocol.EventIssueUpdated, uuidString(workspaceID), actorType, actorID, map[string]any{
		"issue": response, "prev_status": locked.Status, "prev_assignee_type": locked.AssigneeType, "prev_assignee_id": uuidString(locked.AssigneeID),
	})
	if task != nil {
		h.announceQueuedTask(*task)
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) DeleteIssue(w http.ResponseWriter, r *http.Request) {
	issue, workspaceID, principal, ok := h.issueScope(w, r)
	if !ok {
		return
	}
	if principal.Kind == principalTask {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	locked, err := qtx.LockIssue(r.Context(), lwdb.LockIssueParams{ID: issue.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	active, err := qtx.CountActiveTasksByIssue(r.Context(), lwdb.CountActiveTasksByIssueParams{WorkspaceID: workspaceID, IssueID: locked.ID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if active > 0 {
		writeCode(w, http.StatusConflict, "active_tasks_exist")
		return
	}
	cleanupScope := lwdb.DeleteTaskTokensByIssueParams{TargetWorkspaceID: workspaceID, TargetIssueID: locked.ID}
	if _, err := qtx.DeleteTaskTokensByIssue(r.Context(), cleanupScope); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTaskMessagesByIssue(r.Context(), lwdb.DeleteTaskMessagesByIssueParams(cleanupScope)); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTaskUsageByIssue(r.Context(), lwdb.DeleteTaskUsageByIssueParams(cleanupScope)); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteCommentsByIssue(r.Context(), lwdb.DeleteCommentsByIssueParams{WorkspaceID: workspaceID, IssueID: locked.ID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteActivityByIssue(r.Context(), lwdb.DeleteActivityByIssueParams{WorkspaceID: workspaceID, IssueID: locked.ID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTerminalTasksByIssue(r.Context(), lwdb.DeleteTerminalTasksByIssueParams{WorkspaceID: workspaceID, IssueID: locked.ID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	deleted, err := qtx.DeleteIssue(r.Context(), lwdb.DeleteIssueParams{ID: locked.ID, WorkspaceID: workspaceID})
	if err != nil || deleted != 1 || tx.Commit(r.Context()) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventIssueDeleted, uuidString(workspaceID), "member", principal.UserID, map[string]string{"issue_id": uuidString(locked.ID)})
	w.WriteHeader(http.StatusNoContent)
}
