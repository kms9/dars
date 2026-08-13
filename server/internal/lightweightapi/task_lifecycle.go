package lightweightapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/util"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
	"github.com/kms9/dars/pkg/redact"
)

const chatNoResponseFallback = "The agent finished this turn without a text reply."

type taskLifecycleResponse struct {
	ID                    string          `json:"id"`
	WorkspaceID           string          `json:"workspace_id"`
	AgentID               string          `json:"agent_id"`
	RuntimeID             string          `json:"runtime_id"`
	IssueID               string          `json:"issue_id,omitempty"`
	ChatSessionID         string          `json:"chat_session_id,omitempty"`
	SquadID               string          `json:"squad_id,omitempty"`
	Status                string          `json:"status"`
	Priority              int32           `json:"priority"`
	Attempt               int32           `json:"attempt"`
	MaxAttempts           int32           `json:"max_attempts"`
	IsLeaderTask          bool            `json:"is_leader_task,omitempty"`
	SessionID             string          `json:"session_id,omitempty"`
	WorkDir               string          `json:"work_dir,omitempty"`
	Result                json.RawMessage `json:"result,omitempty"`
	Error                 string          `json:"error,omitempty"`
	FailureReason         string          `json:"failure_reason,omitempty"`
	TriggerSummary        string          `json:"trigger_summary,omitempty"`
	WaitReason            string          `json:"wait_reason,omitempty"`
	PrepareLeaseExpiresAt *string         `json:"prepare_lease_expires_at,omitempty"`
	DispatchedAt          *string         `json:"dispatched_at,omitempty"`
	StartedAt             *string         `json:"started_at,omitempty"`
	CompletedAt           *string         `json:"completed_at,omitempty"`
	CreatedAt             string          `json:"created_at"`
}

func taskLifecycleDTO(task lwdb.AgentTaskQueue) taskLifecycleResponse {
	result := json.RawMessage(nil)
	if len(task.Result) > 0 && json.Valid(task.Result) {
		result = append(json.RawMessage(nil), task.Result...)
	}
	return taskLifecycleResponse{
		ID: uuidString(task.ID), WorkspaceID: uuidString(task.WorkspaceID), AgentID: uuidString(task.AgentID), RuntimeID: uuidString(task.RuntimeID),
		IssueID: optionalUUIDString(task.IssueID), ChatSessionID: optionalUUIDString(task.ChatSessionID), SquadID: optionalUUIDString(task.SquadID),
		Status: task.Status, Priority: task.Priority, Attempt: task.Attempt, MaxAttempts: task.MaxAttempts, IsLeaderTask: task.IsLeaderTask,
		SessionID: optionalString(task.SessionID), WorkDir: optionalString(task.WorkDir), Result: result,
		Error: optionalString(task.Error), FailureReason: optionalString(task.FailureReason), TriggerSummary: optionalString(task.TriggerSummary),
		WaitReason: optionalString(task.WaitReason), PrepareLeaseExpiresAt: timePointer(task.PrepareLeaseExpiresAt),
		DispatchedAt: timePointer(task.DispatchedAt), StartedAt: timePointer(task.StartedAt), CompletedAt: timePointer(task.CompletedAt),
		CreatedAt: timeString(task.CreatedAt),
	}
}

func decodeStrictTaskBody(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func nullableTaskText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
}

func (h *Handler) daemonRuntimeScope(w http.ResponseWriter, r *http.Request, rawRuntimeID string) (Principal, pgtype.UUID, lwdb.AgentRuntime, bool) {
	principal, workspaceID, ok := daemonWorkspaceScope(w, r)
	if !ok {
		return Principal{}, pgtype.UUID{}, lwdb.AgentRuntime{}, false
	}
	runtimeID, err := parseUUID(rawRuntimeID)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return Principal{}, pgtype.UUID{}, lwdb.AgentRuntime{}, false
	}
	runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil || runtime.DaemonID != principal.DaemonID {
		writeCode(w, http.StatusNotFound, "not_found")
		return Principal{}, pgtype.UUID{}, lwdb.AgentRuntime{}, false
	}
	return principal, workspaceID, runtime, true
}

func (h *Handler) daemonTaskScope(w http.ResponseWriter, r *http.Request, rawTaskID string) (Principal, pgtype.UUID, lwdb.AgentTaskQueue, bool) {
	principal, workspaceID, ok := daemonWorkspaceScope(w, r)
	if !ok {
		return Principal{}, pgtype.UUID{}, lwdb.AgentTaskQueue{}, false
	}
	taskID, err := parseUUID(rawTaskID)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return Principal{}, pgtype.UUID{}, lwdb.AgentTaskQueue{}, false
	}
	task, err := h.q.GetAgentTask(r.Context(), lwdb.GetAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return Principal{}, pgtype.UUID{}, lwdb.AgentTaskQueue{}, false
	}
	runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: task.RuntimeID, WorkspaceID: workspaceID})
	if err != nil || runtime.DaemonID != principal.DaemonID {
		writeCode(w, http.StatusNotFound, "not_found")
		return Principal{}, pgtype.UUID{}, lwdb.AgentTaskQueue{}, false
	}
	return principal, workspaceID, task, true
}

func (h *Handler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": task.Status})
}

func (h *Handler) ExtendTaskPrepareLease(w http.ResponseWriter, r *http.Request) {
	rawRuntimeID := chi.URLParam(r, "runtimeId")
	_, workspaceID, runtime, ok := h.daemonRuntimeScope(w, r, rawRuntimeID)
	if !ok {
		return
	}
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	if task.RuntimeID != runtime.ID || task.WorkspaceID != workspaceID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if task.Status != "dispatched" && task.Status != "waiting_local_directory" {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	updated, err := h.q.ExtendTaskPrepareLease(r.Context(), lwdb.ExtendTaskPrepareLeaseParams{
		ID: task.ID, RuntimeID: runtime.ID,
		PrepareLeaseExpiresAt: pgtype.Timestamptz{Time: h.now().Add(prepareLeaseDuration), Valid: true},
	})
	if err != nil {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	writeJSON(w, http.StatusOK, taskLifecycleDTO(updated))
}

type taskWaitRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) MarkTaskWaitingLocalDirectory(w http.ResponseWriter, r *http.Request) {
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	request := taskWaitRequest{}
	if decodeStrictTaskBody(r, &request) != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if task.Status == "waiting_local_directory" {
		writeJSON(w, http.StatusOK, taskLifecycleDTO(task))
		return
	}
	if task.Status != "dispatched" {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	updated, err := h.q.MarkTaskWaitingLocalDirectory(r.Context(), lwdb.MarkTaskWaitingLocalDirectoryParams{
		ID: task.ID, RuntimeID: task.RuntimeID, WaitReason: nullableTaskText(redact.Text(request.Reason)),
		PrepareLeaseExpiresAt: pgtype.Timestamptz{Time: h.now().Add(prepareLeaseDuration), Valid: true},
	})
	if err != nil {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	h.publish(protocol.EventTaskWaitingLocalDirectory, uuidString(task.WorkspaceID), "agent", uuidString(task.AgentID), taskLifecycleDTO(updated))
	writeJSON(w, http.StatusOK, taskLifecycleDTO(updated))
}

// StartTask Daemon 在本机准备就绪后将任务标为 running（dispatched / waiting_local_directory → running）。
// 位于 ClaimTasks 与 Backend.Execute 之间，表示 Agent 进程即将或已经开始执行。
func (h *Handler) StartTask(w http.ResponseWriter, r *http.Request) {
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	if task.Status == "running" {
		writeJSON(w, http.StatusOK, taskLifecycleDTO(task))
		return
	}
	if task.Status != "dispatched" && task.Status != "waiting_local_directory" {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	updated, err := h.q.StartTask(r.Context(), lwdb.StartTaskParams{ID: task.ID, RuntimeID: task.RuntimeID})
	if err != nil {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	h.publish(protocol.EventTaskRunning, uuidString(task.WorkspaceID), "agent", uuidString(task.AgentID), taskLifecycleDTO(updated))
	writeJSON(w, http.StatusOK, taskLifecycleDTO(updated))
}

type taskProgressRequest struct {
	Summary string `json:"summary"`
	Step    int    `json:"step"`
	Total   int    `json:"total"`
}

func (h *Handler) ReportTaskProgress(w http.ResponseWriter, r *http.Request) {
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	var request taskProgressRequest
	if decodeStrictTaskBody(r, &request) != nil || request.Step < 0 || request.Total < 0 || (request.Total > 0 && request.Step > request.Total) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	request.Summary = strings.TrimSpace(redact.Text(request.Summary))
	if request.Summary == "" {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	updated, err := h.q.UpdateTaskProgress(r.Context(), lwdb.UpdateTaskProgressParams{
		TriggerSummary: nullableTaskText(request.Summary), ID: task.ID, RuntimeID: task.RuntimeID,
	})
	if err != nil {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	payload := protocol.TaskProgressPayload{TaskID: uuidString(task.ID), Summary: request.Summary, Step: request.Step, Total: request.Total}
	h.publish(protocol.EventTaskProgress, uuidString(task.WorkspaceID), "agent", uuidString(task.AgentID), payload)
	writeJSON(w, http.StatusOK, map[string]string{"status": updated.Status})
}

type taskMessageRequest struct {
	Seq     int            `json:"seq"`
	Type    string         `json:"type"`
	Tool    string         `json:"tool,omitempty"`
	Content string         `json:"content,omitempty"`
	Input   map[string]any `json:"input,omitempty"`
	Output  string         `json:"output,omitempty"`
}

type taskMessageBatchRequest struct {
	Messages []taskMessageRequest `json:"messages"`
}

func taskMessagePayload(message lwdb.TaskMessage, task lwdb.AgentTaskQueue) protocol.TaskMessagePayload {
	input := map[string]any(nil)
	if len(message.Input) > 0 {
		_ = json.Unmarshal(message.Input, &input)
	}
	return protocol.TaskMessagePayload{
		TaskID: uuidString(task.ID), IssueID: optionalUUIDString(task.IssueID), Seq: int(message.Seq), Type: message.Type,
		Tool: optionalString(message.Tool), Content: optionalString(message.Content), Input: input, Output: optionalString(message.Output),
		CreatedAt: timeString(message.CreatedAt),
	}
}

func (h *Handler) ReportTaskMessages(w http.ResponseWriter, r *http.Request) {
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	var request taskMessageBatchRequest
	if decodeStrictTaskBody(r, &request) != nil || len(request.Messages) > 1000 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	for _, message := range request.Messages {
		message.Type = strings.TrimSpace(message.Type)
		if message.Seq < 0 || message.Seq > math.MaxInt32 || message.Type == "" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	for _, message := range request.Messages {
		message.Content = redact.Text(message.Content)
		message.Output = redact.Text(message.Output)
		message.Input = redact.InputMap(message.Input)
		var input []byte
		if message.Input != nil {
			input, _ = json.Marshal(message.Input)
		}
		created, err := h.q.CreateTaskMessage(r.Context(), lwdb.CreateTaskMessageParams{
			TaskID: task.ID, Seq: int32(message.Seq), Type: strings.TrimSpace(message.Type), Tool: nullableTaskText(message.Tool),
			Content: nullableTaskText(message.Content), Input: input, Output: nullableTaskText(message.Output),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		h.publish(protocol.EventTaskMessage, uuidString(task.WorkspaceID), "agent", uuidString(task.AgentID), taskMessagePayload(created, task))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ListTaskMessages(w http.ResponseWriter, r *http.Request) {
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	after := int64(-1)
	if raw := r.URL.Query().Get("since"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value < -1 {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		after = value
	}
	messages, err := h.q.ListTaskMessagesSince(r.Context(), lwdb.ListTaskMessagesSinceParams{TaskID: task.ID, AfterSeq: int32(after)})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]protocol.TaskMessagePayload, 0, len(messages))
	for _, message := range messages {
		response = append(response, taskMessagePayload(message, task))
	}
	writeJSON(w, http.StatusOK, response)
}

type taskUsageEntry struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	CostUSDTicks     int64  `json:"cost_usd_ticks"`
}

type taskUsageRequest struct {
	Usage []taskUsageEntry `json:"usage"`
}

func (h *Handler) ReportTaskUsage(w http.ResponseWriter, r *http.Request) {
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	var request taskUsageRequest
	if decodeStrictTaskBody(r, &request) != nil || len(request.Usage) > 1000 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	for _, usage := range request.Usage {
		if strings.TrimSpace(usage.Provider) == "" || strings.TrimSpace(usage.Model) == "" || usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.CostUSDTicks < 0 {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	for _, usage := range request.Usage {
		cost := pgtype.Int8{}
		if usage.CostUSDTicks > 0 {
			cost = pgtype.Int8{Int64: usage.CostUSDTicks, Valid: true}
		}
		if _, err := h.q.UpsertTaskUsage(r.Context(), lwdb.UpsertTaskUsageParams{
			TaskID: task.ID, Provider: strings.ToLower(strings.TrimSpace(usage.Provider)), Model: strings.TrimSpace(usage.Model),
			InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, CacheReadTokens: usage.CacheReadTokens,
			CacheWriteTokens: usage.CacheWriteTokens, CostUsdTicks: cost,
		}); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type taskCompleteRequest struct {
	Output                string `json:"output"`
	SessionID             string `json:"session_id"`
	WorkDir               string `json:"work_dir"`
	SessionRolloutMissing bool   `json:"session_rollout_missing"`
	RetiredSessionID      string `json:"retired_session_id"`
}

type taskFailRequest struct {
	Error                 string `json:"error"`
	SessionID             string `json:"session_id"`
	WorkDir               string `json:"work_dir"`
	FailureReason         string `json:"failure_reason"`
	SessionRolloutMissing bool   `json:"session_rollout_missing"`
	RetiredSessionID      string `json:"retired_session_id"`
}

func requestDigest(value any) string {
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func terminalText(reported string, existing pgtype.Text) pgtype.Text {
	if strings.TrimSpace(reported) == "" {
		return existing
	}
	return nullableTaskText(reported)
}

func terminalElapsed(task lwdb.AgentTaskQueue, completedAt time.Time) pgtype.Int8 {
	start := task.CreatedAt.Time
	if task.StartedAt.Valid {
		start = task.StartedAt.Time
	}
	if start.IsZero() || completedAt.Before(start) {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: completedAt.Sub(start).Milliseconds(), Valid: true}
}

func retryDelay(failureReason string) (time.Duration, bool) {
	switch strings.TrimSpace(failureReason) {
	case "runtime_restart":
		return 0, true
	case "runtime_offline", "timeout", "provider_network", "agent_error.provider_network", "skill_bundle_unavailable":
		return 5 * time.Second, true
	default:
		return 0, false
	}
}

func (h *Handler) createRetryTask(ctx context.Context, q *lwdb.Queries, task lwdb.AgentTaskQueue, failureReason string) (*lwdb.AgentTaskQueue, error) {
	delay, retryable := retryDelay(failureReason)
	if !retryable || task.Attempt >= task.MaxAttempts {
		return nil, nil
	}
	status := "queued"
	fireAt := pgtype.Timestamptz{}
	if delay > 0 {
		status = "deferred"
		fireAt = pgtype.Timestamptz{Time: h.now().Add(delay), Valid: true}
	}
	if task.ToolBundleID.Valid {
		if err := h.requireGatewayProviderForPinnedTask(ctx, q, task.WorkspaceID, task.RuntimeID); err != nil {
			return nil, err
		}
	}
	retry, err := q.CreateRetryTask(ctx, lwdb.CreateRetryTaskParams{
		RetryStatus: status, ClearSession: task.SessionRolloutMissing,
		FireAt: fireAt, ParentTaskID: task.ID, WorkspaceID: task.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &retry, nil
}

// announceQueuedTask 任务入队后的副作用：向用户平面广播 task:queued，
// 并经 DaemonWakeupNotifier 唤醒对应 runtime（daemonws / Redis relay）。
func (h *Handler) announceQueuedTask(task lwdb.AgentTaskQueue) {
	h.publish(protocol.EventTaskQueued, uuidString(task.WorkspaceID), "system", "", taskLifecycleDTO(task))
	if h.cfg.Wakeup != nil {
		h.cfg.Wakeup.NotifyTaskAvailable(uuidString(task.RuntimeID), uuidString(task.ID))
	}
}

// writeCompletionProjection 将 Daemon complete 的 output 投影为持久化产物：
// Issue 任务 → CreateComment（agent 作者）；Chat 任务 → CreateChatMessage。
// 空 output 的 Issue 不写评论；Chat 则写 no_response 占位。
func (h *Handler) writeCompletionProjection(ctx context.Context, qtx *lwdb.Queries, task lwdb.AgentTaskQueue, request taskCompleteRequest) (*lwdb.Comment, *lwdb.ChatMessage, error) {
	body := strings.TrimSpace(util.UnescapeBackslashEscapes(redact.Text(request.Output)))
	idempotencyKey := pgtype.Text{String: "task-complete:" + uuidString(task.ID), Valid: true}
	requestHash := pgtype.Text{String: requestDigest(request), Valid: true}
	if task.IssueID.Valid {
		if body == "" {
			return nil, nil, nil
		}
		comment, err := qtx.CreateComment(ctx, lwdb.CreateCommentParams{
			WorkspaceID: task.WorkspaceID, IssueID: task.IssueID, AuthorType: "agent", AuthorID: task.AgentID,
			Content: body, SourceTaskID: task.ID, IdempotencyKey: idempotencyKey, RequestHash: requestHash,
		})
		if err != nil {
			return nil, nil, err
		}
		return &comment, nil, nil
	}
	messageKind := protocol.ChatMessageKindMessage
	if body == "" {
		body = chatNoResponseFallback
		messageKind = protocol.ChatMessageKindNoResponse
	}
	message, err := qtx.CreateChatMessage(ctx, lwdb.CreateChatMessageParams{
		ChatSessionID: task.ChatSessionID, Role: "assistant", Content: body, TaskID: task.ID,
		ElapsedMs: terminalElapsed(task, h.now()), MessageKind: messageKind,
		IdempotencyKey: idempotencyKey, RequestHash: requestHash,
	})
	return nil, &message, err
}

func (h *Handler) writeFailureProjection(ctx context.Context, qtx *lwdb.Queries, task lwdb.AgentTaskQueue, request taskFailRequest) (*lwdb.ChatMessage, error) {
	if !task.ChatSessionID.Valid {
		return nil, nil
	}
	body := strings.TrimSpace(redact.Text(request.Error))
	if body == "" {
		body = "The agent run failed without an error message."
	}
	idempotencyKey := pgtype.Text{String: "task-fail:" + uuidString(task.ID), Valid: true}
	message, err := qtx.CreateChatMessage(ctx, lwdb.CreateChatMessageParams{
		ChatSessionID: task.ChatSessionID, Role: "assistant", Content: body, TaskID: task.ID,
		FailureReason: nullableTaskText(request.FailureReason), ElapsedMs: terminalElapsed(task, h.now()),
		MessageKind: protocol.ChatMessageKindMessage, IdempotencyKey: idempotencyKey,
		RequestHash: pgtype.Text{String: requestDigest(request), Valid: true},
	})
	return &message, err
}

func updateChatResume(ctx context.Context, qtx *lwdb.Queries, task lwdb.AgentTaskQueue) error {
	if !task.ChatSessionID.Valid {
		return nil
	}
	_, err := qtx.UpdateChatSessionResume(ctx, lwdb.UpdateChatSessionResumeParams{
		SessionID: task.SessionID, WorkDir: task.WorkDir, ID: task.ChatSessionID, WorkspaceID: task.WorkspaceID,
	})
	return err
}

// CompleteTask Daemon 上报任务成功结束（ddt_）。
// 状态 running→completed → writeCompletionProjection →
// 对 Issue 评论 routeCreatedComment（成员结果会回传 Leader）→
// reconcileUndeliveredComments → 吊销 dat_ → announce 新队列 →
// publish comment:created / task:completed 供 Web 收敛。
func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	principal, workspaceID, scoped, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	var request taskCompleteRequest
	if decodeStrictTaskBody(r, &request) != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	var lockedIssue *lwdb.Issue
	if scoped.IssueID.Valid {
		issue, issueErr := qtx.LockIssue(r.Context(), lwdb.LockIssueParams{ID: scoped.IssueID, WorkspaceID: workspaceID})
		if issueErr != nil {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		lockedIssue = &issue
	}
	runtime, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: scoped.RuntimeID, WorkspaceID: workspaceID})
	if err != nil || runtime.DaemonID != principal.DaemonID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	locked, err := qtx.LockAgentTask(r.Context(), lwdb.LockAgentTaskParams{ID: scoped.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if locked.Status == "completed" {
		writeJSON(w, http.StatusOK, taskLifecycleDTO(locked))
		return
	}
	if locked.Status != "running" {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	result, _ := json.Marshal(request)
	session := terminalText(request.SessionID, locked.SessionID)
	missing := locked.SessionRolloutMissing || request.SessionRolloutMissing
	if missing {
		session = pgtype.Text{}
	}
	updated, err := qtx.CompleteTask(r.Context(), lwdb.CompleteTaskParams{
		Result: result, SessionRolloutMissing: missing, SessionID: session, WorkDir: terminalText(request.WorkDir, locked.WorkDir),
		RetiredSessionID: terminalText(request.RetiredSessionID, locked.RetiredSessionID), ID: locked.ID, RuntimeID: locked.RuntimeID,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	issueComment, chatMessage, err := h.writeCompletionProjection(r.Context(), qtx, updated, request)
	if err != nil || updateChatResume(r.Context(), qtx, updated) != nil {
		slog.Error("lightweight task completion projection failed", "task_id", uuidString(updated.ID), "error", err)
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	queuedTasks := make([]lwdb.AgentTaskQueue, 0, 2)
	if lockedIssue != nil {
		if issueComment != nil {
			routed, routeErr := h.routeCreatedComment(r.Context(), qtx, *lockedIssue, *issueComment, principal, &updated)
			if routeErr != nil {
				slog.Error("lightweight completion comment routing failed", "task_id", uuidString(updated.ID), "error", routeErr)
				writeIssueResolutionError(w, routeErr)
				return
			}
			queuedTasks = append(queuedTasks, routed...)
			if _, touchErr := qtx.TouchIssueForComment(r.Context(), lwdb.TouchIssueForCommentParams{ID: lockedIssue.ID, WorkspaceID: workspaceID}); touchErr != nil {
				slog.Error("lightweight completion issue touch failed", "task_id", uuidString(updated.ID), "error", touchErr)
				writeCode(w, http.StatusInternalServerError, "internal_error")
				return
			}
		}
		followup, reconcileErr := h.reconcileUndeliveredComments(r.Context(), qtx, *lockedIssue, updated)
		if reconcileErr != nil {
			slog.Error("lightweight comment reconciliation failed", "task_id", uuidString(updated.ID), "error", reconcileErr)
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if followup != nil {
			queuedTasks = append(queuedTasks, *followup)
		}
	}
	if _, err := qtx.DeleteTaskTokensByTask(r.Context(), updated.ID); err != nil {
		slog.Error("lightweight completion token cleanup failed", "task_id", uuidString(updated.ID), "error", err)
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("lightweight task completion commit failed", "task_id", uuidString(updated.ID), "error", err)
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	for _, task := range queuedTasks {
		h.announceQueuedTask(task)
	}
	if updated.IssueID.Valid && strings.TrimSpace(request.Output) != "" {
		h.publish(protocol.EventCommentCreated, uuidString(updated.WorkspaceID), "agent", uuidString(updated.AgentID), map[string]any{"issue_id": uuidString(updated.IssueID), "source_task_id": uuidString(updated.ID)})
	}
	if chatMessage != nil {
		h.publish(protocol.EventChatDone, uuidString(updated.WorkspaceID), "agent", uuidString(updated.AgentID), protocol.ChatDonePayload{
			ChatSessionID: uuidString(updated.ChatSessionID), TaskID: uuidString(updated.ID), MessageID: uuidString(chatMessage.ID),
			Content: chatMessage.Content, ElapsedMs: chatMessage.ElapsedMs.Int64, CreatedAt: timeString(chatMessage.CreatedAt), MessageKind: chatMessage.MessageKind,
		})
	}
	h.publish(protocol.EventTaskCompleted, uuidString(updated.WorkspaceID), "agent", uuidString(updated.AgentID), taskLifecycleDTO(updated))
	writeJSON(w, http.StatusOK, taskLifecycleDTO(updated))
}

func (h *Handler) FailTask(w http.ResponseWriter, r *http.Request) {
	principal, workspaceID, scoped, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	var request taskFailRequest
	if decodeStrictTaskBody(r, &request) != nil || strings.TrimSpace(request.Error) == "" {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	runtime, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: scoped.RuntimeID, WorkspaceID: workspaceID})
	if err != nil || runtime.DaemonID != principal.DaemonID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	locked, err := qtx.LockAgentTask(r.Context(), lwdb.LockAgentTaskParams{ID: scoped.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if locked.Status == "failed" {
		writeJSON(w, http.StatusOK, taskLifecycleDTO(locked))
		return
	}
	if locked.Status != "dispatched" && locked.Status != "waiting_local_directory" && locked.Status != "running" {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	session := terminalText(request.SessionID, locked.SessionID)
	missing := locked.SessionRolloutMissing || request.SessionRolloutMissing
	if missing {
		session = pgtype.Text{}
	}
	updated, err := qtx.FailTask(r.Context(), lwdb.FailTaskParams{
		Error: nullableTaskText(redact.Text(request.Error)), FailureReason: nullableTaskText(request.FailureReason),
		SessionRolloutMissing: missing, SessionID: session, WorkDir: terminalText(request.WorkDir, locked.WorkDir),
		RetiredSessionID: terminalText(request.RetiredSessionID, locked.RetiredSessionID), ID: locked.ID, RuntimeID: locked.RuntimeID,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	retry, err := h.createRetryTask(r.Context(), qtx, updated, request.FailureReason)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	var chatMessage *lwdb.ChatMessage
	if retry == nil {
		chatMessage, err = h.writeFailureProjection(r.Context(), qtx, updated, request)
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	if updateChatResume(r.Context(), qtx, updated) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTaskTokensByTask(r.Context(), updated.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if chatMessage != nil {
		h.publish(protocol.EventChatMessage, uuidString(updated.WorkspaceID), "agent", uuidString(updated.AgentID), protocol.ChatMessagePayload{
			ChatSessionID: uuidString(updated.ChatSessionID), MessageID: uuidString(chatMessage.ID), Role: chatMessage.Role,
			Content: chatMessage.Content, TaskID: uuidString(updated.ID), CreatedAt: timeString(chatMessage.CreatedAt),
		})
	}
	h.publish(protocol.EventTaskFailed, uuidString(updated.WorkspaceID), "agent", uuidString(updated.AgentID), taskLifecycleDTO(updated))
	if retry != nil && retry.Status == "queued" {
		h.announceQueuedTask(*retry)
	}
	writeJSON(w, http.StatusOK, taskLifecycleDTO(updated))
}

type pinTaskSessionRequest struct {
	SessionID string `json:"session_id"`
	WorkDir   string `json:"work_dir"`
}

func (h *Handler) PinTaskSession(w http.ResponseWriter, r *http.Request) {
	_, _, task, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	var request pinTaskSessionRequest
	if decodeStrictTaskBody(r, &request) != nil || (strings.TrimSpace(request.SessionID) == "" && strings.TrimSpace(request.WorkDir) == "") {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
		writeJSON(w, http.StatusOK, taskLifecycleDTO(task))
		return
	}
	updated, err := h.q.UpdateTaskSession(r.Context(), lwdb.UpdateTaskSessionParams{
		SetSessionID: strings.TrimSpace(request.SessionID) != "", SessionID: nullableTaskText(request.SessionID),
		SetWorkDir: strings.TrimSpace(request.WorkDir) != "", WorkDir: nullableTaskText(request.WorkDir), ID: task.ID, RuntimeID: task.RuntimeID,
	})
	if err != nil {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	writeJSON(w, http.StatusOK, taskLifecycleDTO(updated))
}

func cancelledChatInputTaskID(task lwdb.AgentTaskQueue) pgtype.UUID {
	if task.ChatInputTaskID.Valid {
		return task.ChatInputTaskID
	}
	return task.ID
}

func createCancelledChatDraft(ctx context.Context, q *lwdb.Queries, task lwdb.AgentTaskQueue) (bool, error) {
	if !task.ChatSessionID.Valid {
		return false, nil
	}
	message, err := q.GetChatMessageByTaskID(ctx, lwdb.GetChatMessageByTaskIDParams{
		ChatSessionID: task.ChatSessionID, TaskID: cancelledChatInputTaskID(task), Role: "user",
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := q.CreateChatDraftRestore(ctx, lwdb.CreateChatDraftRestoreParams{
		ChatSessionID: task.ChatSessionID, TaskID: task.ID, Content: message.Content,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func cancelledChatInitiator(ctx context.Context, q *lwdb.Queries, task lwdb.AgentTaskQueue) string {
	for _, candidate := range []pgtype.UUID{task.InitiatorUserID, task.OriginatorUserID, task.AccountableUserID} {
		if candidate.Valid {
			return uuidString(candidate)
		}
	}
	if !task.ChatSessionID.Valid {
		return ""
	}
	session, err := q.GetChatSession(ctx, lwdb.GetChatSessionParams{ID: task.ChatSessionID, WorkspaceID: task.WorkspaceID})
	if err != nil {
		return ""
	}
	return uuidString(session.CreatorID)
}

func (h *Handler) publishCancelledChatFinalized(task lwdb.AgentTaskQueue, initiatorUserID string) {
	if !task.ChatSessionID.Valid {
		return
	}
	h.publish(protocol.EventChatCancelFinalized, uuidString(task.WorkspaceID), "member", initiatorUserID, protocol.ChatCancelFinalizedPayload{
		Outcome: protocol.ChatCancelOutcomeRestored, ChatSessionID: uuidString(task.ChatSessionID),
		TaskID: uuidString(task.ID), InitiatorUserID: initiatorUserID,
	})
}

func (h *Handler) CancelTaskByUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || (principal.Kind != principalJWT && principal.Kind != principalPAT) {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	taskID, err := parseUUID(chi.URLParam(r, "taskId"))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	locked, err := qtx.LockAgentTask(r.Context(), lwdb.LockAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if locked.Status == "cancelled" {
		writeJSON(w, http.StatusOK, taskLifecycleDTO(locked))
		return
	}
	if locked.Status == "completed" || locked.Status == "failed" {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	if locked.ChatSessionID.Valid {
		session, err := qtx.GetChatSession(r.Context(), lwdb.GetChatSessionParams{ID: locked.ChatSessionID, WorkspaceID: workspaceID})
		if err != nil {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		if uuidString(session.CreatorID) != principal.UserID {
			writeCode(w, http.StatusForbidden, "forbidden")
			return
		}
	} else {
		agent, err := qtx.GetAgent(r.Context(), lwdb.GetAgentParams{ID: locked.AgentID, WorkspaceID: workspaceID})
		if err != nil {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		if agent.PermissionMode == "private" && !h.canManage(r, agent.OwnerID) {
			writeCode(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	wasActiveChat := locked.ChatSessionID.Valid && (locked.Status == "dispatched" || locked.Status == "waiting_local_directory" || locked.Status == "running")
	cancelled, err := qtx.CancelTask(r.Context(), lwdb.CancelTaskParams{ID: locked.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	draftCreated, err := createCancelledChatDraft(r.Context(), qtx, cancelled)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTaskTokensByTask(r.Context(), cancelled.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventTaskCancelled, uuidString(cancelled.WorkspaceID), "member", principal.UserID, taskLifecycleDTO(cancelled))
	if cancelled.ChatSessionID.Valid && !wasActiveChat && draftCreated {
		h.publishCancelledChatFinalized(cancelled, principal.UserID)
	}
	writeJSON(w, http.StatusOK, taskLifecycleDTO(cancelled))
}

func (h *Handler) AckTaskCancelled(w http.ResponseWriter, r *http.Request) {
	principal, workspaceID, scoped, ok := h.daemonTaskScope(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	runtime, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: scoped.RuntimeID, WorkspaceID: workspaceID})
	if err != nil || runtime.DaemonID != principal.DaemonID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	task, err := qtx.LockAgentTask(r.Context(), lwdb.LockAgentTaskParams{ID: scoped.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if task.Status != "cancelled" {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	wasDeferred := task.ChatFinalizeDeferredAt.Valid
	_, err = createCancelledChatDraft(r.Context(), qtx, task)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	finalized, err := qtx.FinalizeCancelledTask(r.Context(), lwdb.FinalizeCancelledTaskParams{ID: task.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTaskTokensByTask(r.Context(), task.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	initiator := cancelledChatInitiator(r.Context(), qtx, finalized)
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if wasDeferred {
		h.publishCancelledChatFinalized(finalized, initiator)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *Handler) ListPendingTasksByRuntime(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, runtime, ok := h.daemonRuntimeScope(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	tasks, err := h.q.ListPendingTasksByRuntime(r.Context(), lwdb.ListPendingTasksByRuntimeParams{WorkspaceID: workspaceID, RuntimeID: runtime.ID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]taskLifecycleResponse, 0, len(tasks))
	for _, task := range tasks {
		response = append(response, taskLifecycleDTO(task))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) RecoverOrphanedTasks(w http.ResponseWriter, r *http.Request) {
	principal, workspaceID, runtime, ok := h.daemonRuntimeScope(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	lockedRuntime, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: runtime.ID, WorkspaceID: workspaceID})
	if err != nil || lockedRuntime.DaemonID != principal.DaemonID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	tasks, err := qtx.RecoverOrphanedTasksForRuntime(r.Context(), lwdb.RecoverOrphanedTasksForRuntimeParams{WorkspaceID: workspaceID, RuntimeID: runtime.ID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	retries := make([]lwdb.AgentTaskQueue, 0, len(tasks))
	for _, task := range tasks {
		if _, err := qtx.DeleteTaskTokensByTask(r.Context(), task.ID); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		retry, err := h.createRetryTask(r.Context(), qtx, task, "runtime_restart")
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if retry != nil {
			retries = append(retries, *retry)
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	for _, task := range tasks {
		h.publish(protocol.EventTaskFailed, uuidString(task.WorkspaceID), "system", "", taskLifecycleDTO(task))
	}
	for _, retry := range retries {
		if retry.Status == "queued" {
			h.announceQueuedTask(retry)
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"orphaned": len(tasks), "retried": len(retries)})
}
