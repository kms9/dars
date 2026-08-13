package lightweightapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const (
	chatTitleMaxLength      = 200
	chatContentMaxLength    = 1 << 20
	chatIdempotencyMaxBytes = 255
	chatDefaultPageLimit    = 30
	chatMaxPageLimit        = 100
)

type chatSessionResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	AgentID     string  `json:"agent_id"`
	CreatorID   string  `json:"creator_id"`
	RuntimeID   string  `json:"runtime_id"`
	Title       string  `json:"title"`
	SessionID   *string `json:"session_id"`
	WorkDir     *string `json:"work_dir"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func chatSessionDTO(session lwdb.ChatSession) chatSessionResponse {
	return chatSessionResponse{
		ID: uuidString(session.ID), WorkspaceID: uuidString(session.WorkspaceID), AgentID: uuidString(session.AgentID),
		CreatorID: uuidString(session.CreatorID), RuntimeID: uuidString(session.RuntimeID), Title: session.Title,
		SessionID: textPointer(session.SessionID), WorkDir: textPointer(session.WorkDir), Status: session.Status,
		CreatedAt: timeString(session.CreatedAt), UpdatedAt: timeString(session.UpdatedAt),
	}
}

type chatMessageResponse struct {
	ID            string  `json:"id"`
	ChatSessionID string  `json:"chat_session_id"`
	Role          string  `json:"role"`
	Content       string  `json:"content"`
	TaskID        *string `json:"task_id"`
	FailureReason *string `json:"failure_reason"`
	ElapsedMS     *int64  `json:"elapsed_ms"`
	MessageKind   string  `json:"message_kind"`
	CreatedAt     string  `json:"created_at"`
}

func chatMessageDTO(message lwdb.ChatMessage) chatMessageResponse {
	var taskID *string
	if message.TaskID.Valid {
		value := uuidString(message.TaskID)
		taskID = &value
	}
	var elapsed *int64
	if message.ElapsedMs.Valid {
		value := message.ElapsedMs.Int64
		elapsed = &value
	}
	return chatMessageResponse{
		ID: uuidString(message.ID), ChatSessionID: uuidString(message.ChatSessionID), Role: message.Role,
		Content: message.Content, TaskID: taskID, FailureReason: textPointer(message.FailureReason),
		ElapsedMS: elapsed, MessageKind: message.MessageKind, CreatedAt: timeString(message.CreatedAt),
	}
}

type chatDraftRestoreResponse struct {
	ID            string `json:"id"`
	ChatSessionID string `json:"chat_session_id"`
	TaskID        string `json:"task_id"`
	Content       string `json:"content"`
	CreatedAt     string `json:"created_at"`
}

func chatDraftRestoreDTO(restore lwdb.ChatDraftRestore) chatDraftRestoreResponse {
	return chatDraftRestoreResponse{
		ID: uuidString(restore.ID), ChatSessionID: uuidString(restore.ChatSessionID), TaskID: uuidString(restore.TaskID),
		Content: restore.Content, CreatedAt: timeString(restore.CreatedAt),
	}
}

type chatPageCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

func parseChatPage(r *http.Request) (int32, pgtype.Timestamptz, pgtype.UUID, error) {
	limit := chatDefaultPageLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > chatMaxPageLimit {
			return 0, pgtype.Timestamptz{}, pgtype.UUID{}, errors.New("invalid limit")
		}
		limit = parsed
	}
	rawCursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if rawCursor == "" {
		return int32(limit), pgtype.Timestamptz{}, pgtype.UUID{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(rawCursor)
	if err != nil || len(payload) == 0 {
		return 0, pgtype.Timestamptz{}, pgtype.UUID{}, errors.New("invalid cursor")
	}
	var cursor chatPageCursor
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return 0, pgtype.Timestamptz{}, pgtype.UUID{}, errors.New("invalid cursor")
	}
	at, err := parseRFC3339(cursor.At)
	if err != nil {
		return 0, pgtype.Timestamptz{}, pgtype.UUID{}, errors.New("invalid cursor")
	}
	id, err := parseUUID(cursor.ID)
	if err != nil {
		return 0, pgtype.Timestamptz{}, pgtype.UUID{}, errors.New("invalid cursor")
	}
	return int32(limit), at, id, nil
}

func encodeChatCursor(at pgtype.Timestamptz, id pgtype.UUID) string {
	payload, _ := json.Marshal(chatPageCursor{At: timeString(at), ID: uuidString(id)})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func parseRFC3339(value string) (pgtype.Timestamptz, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return pgtype.Timestamptz{}, errors.New("invalid time")
	}
	return pgtype.Timestamptz{Time: parsed, Valid: true}, nil
}

func (h *Handler) chatHumanScope(w http.ResponseWriter, r *http.Request) (Principal, pgtype.UUID, pgtype.UUID, bool) {
	principal, ok := principalFromContext(r.Context())
	if !ok || (principal.Kind != principalJWT && principal.Kind != principalPAT) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return Principal{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return Principal{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return Principal{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	return principal, workspaceID, userID, true
}

func (h *Handler) humanCanInvokeAgent(ctx context.Context, agent lwdb.Agent, userID pgtype.UUID) bool {
	return humanCanInvokeAgentWithQueries(ctx, h.q, agent, userID)
}

func humanCanInvokeAgentWithQueries(ctx context.Context, queries *lwdb.Queries, agent lwdb.Agent, userID pgtype.UUID) bool {
	if agent.OwnerID == userID {
		return true
	}
	if agent.PermissionMode != "public_to" {
		return false
	}
	targets, err := queries.ListAgentInvocationTargets(ctx, agent.ID)
	if err != nil {
		return false
	}
	var memberID pgtype.UUID
	memberResolved := false
	for _, target := range targets {
		if target.TargetType == "workspace" && target.TargetID == agent.WorkspaceID {
			return true
		}
		if target.TargetType != "member" {
			continue
		}
		if !memberResolved {
			member, err := queries.GetMember(ctx, lwdb.GetMemberParams{WorkspaceID: agent.WorkspaceID, UserID: userID})
			if err != nil {
				return false
			}
			memberID, memberResolved = member.ID, true
		}
		if target.TargetID == memberID {
			return true
		}
	}
	return false
}

func (h *Handler) humanCanViewAgent(r *http.Request, agent lwdb.Agent, userID pgtype.UUID) bool {
	if agent.OwnerID == userID {
		return true
	}
	if member, ok := MemberFromContext(r.Context()); ok && (member.Role == "owner" || member.Role == "admin") {
		return true
	}
	return h.humanCanInvokeAgent(r.Context(), agent, userID)
}

func (h *Handler) loadOwnedChatSession(w http.ResponseWriter, r *http.Request, allowWithoutAgentAccess bool) (lwdb.ChatSession, pgtype.UUID, pgtype.UUID, bool) {
	_, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return lwdb.ChatSession{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	sessionID, err := parseUUID(chi.URLParam(r, "sessionId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.ChatSession{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	session, err := h.q.GetChatSession(r.Context(), lwdb.GetChatSessionParams{ID: sessionID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.ChatSession{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	if session.CreatorID != userID {
		writeCode(w, http.StatusForbidden, "forbidden")
		return lwdb.ChatSession{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	if !allowWithoutAgentAccess {
		agent, err := h.q.GetAgentRow(r.Context(), lwdb.GetAgentRowParams{ID: session.AgentID, WorkspaceID: workspaceID})
		if err != nil {
			writeCode(w, http.StatusNotFound, "not_found")
			return lwdb.ChatSession{}, pgtype.UUID{}, pgtype.UUID{}, false
		}
		if isAgentBuilderCarrier(agent) {
			return session, workspaceID, userID, true
		}
		if !h.humanCanViewAgent(r, agent, userID) {
			writeCode(w, http.StatusForbidden, "forbidden")
			return lwdb.ChatSession{}, pgtype.UUID{}, pgtype.UUID{}, false
		}
	}
	return session, workspaceID, userID, true
}

func (h *Handler) loadReadableChatSession(w http.ResponseWriter, r *http.Request) (lwdb.ChatSession, bool) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return lwdb.ChatSession{}, false
	}
	if principal.Kind != principalTask {
		session, _, _, ok := h.loadOwnedChatSession(w, r, false)
		return session, ok
	}
	workspaceID, err := parseUUID(principal.WorkspaceID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return lwdb.ChatSession{}, false
	}
	sessionID, err := parseUUID(chi.URLParam(r, "sessionId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.ChatSession{}, false
	}
	taskID, err := parseUUID(principal.TaskID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return lwdb.ChatSession{}, false
	}
	task, err := h.q.GetAgentTask(r.Context(), lwdb.GetAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
	if err != nil || !task.ChatSessionID.Valid || task.ChatSessionID != sessionID || uuidString(task.AgentID) != principal.AgentID {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.ChatSession{}, false
	}
	session, err := h.q.GetChatSession(r.Context(), lwdb.GetChatSessionParams{ID: sessionID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.ChatSession{}, false
	}
	return session, true
}

type createChatSessionRequest struct {
	AgentID string `json:"agent_id"`
	Title   string `json:"title"`
}

func (h *Handler) CreateChatSession(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return
	}
	var request createChatSessionRequest
	if decodeStrictTaskBody(r, &request) != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	agentID, err := parseUUID(request.AgentID)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	agent, err := h.q.GetAgent(r.Context(), lwdb.GetAgentParams{ID: agentID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if agent.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "agent_archived")
		return
	}
	if !h.humanCanInvokeAgent(r.Context(), agent, userID) {
		writeCode(w, http.StatusForbidden, "invocation_forbidden")
		return
	}
	if !agent.RuntimeID.Valid {
		writeCode(w, http.StatusConflict, "agent_runtime_required")
		return
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = agent.Name
	}
	if title == "" || len([]rune(title)) > chatTitleMaxLength {
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
	runtime, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: agent.RuntimeID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusConflict, "agent_runtime_required")
		return
	}
	lockedAgent, err := qtx.LockAgent(r.Context(), lwdb.LockAgentParams{ID: agent.ID, WorkspaceID: workspaceID})
	if err != nil || lockedAgent.RuntimeID != runtime.ID || lockedAgent.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	if runtime.Status != "online" {
		writeCode(w, http.StatusServiceUnavailable, "runtime_unavailable")
		return
	}
	session, err := qtx.CreateChatSession(r.Context(), lwdb.CreateChatSessionParams{
		WorkspaceID: workspaceID, AgentID: agentID, CreatorID: userID, RuntimeID: runtime.ID, Title: title,
	})
	if err != nil || tx.Commit(r.Context()) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, chatSessionDTO(session))
}

func (h *Handler) ListChatSessions(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return
	}
	limit, cursorAt, cursorID, err := parseChatPage(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	rows, err := h.q.ListChatSessionsPage(r.Context(), lwdb.ListChatSessionsPageParams{
		WorkspaceID: workspaceID, CreatorID: userID, CursorUpdatedAt: cursorAt, CursorID: cursorID, PageLimit: limit + 1,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]chatSessionResponse, 0, len(rows))
	for _, row := range rows {
		agent, err := h.q.GetAgentRow(r.Context(), lwdb.GetAgentRowParams{ID: row.AgentID, WorkspaceID: workspaceID})
		if err != nil || isAgentBuilderCarrier(agent) {
			continue
		}
		if !h.humanCanViewAgent(r, agent, userID) {
			continue
		}
		items = append(items, chatSessionDTO(row))
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		value := encodeChatCursor(rows[len(rows)-1].UpdatedAt, rows[len(rows)-1].ID)
		nextCursor = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextCursor})
}

func (h *Handler) GetChatSession(w http.ResponseWriter, r *http.Request) {
	session, _, _, ok := h.loadOwnedChatSession(w, r, false)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, chatSessionDTO(session))
}

type updateChatSessionRequest struct {
	Title  *string `json:"title"`
	Status *string `json:"status"`
}

func (h *Handler) UpdateChatSession(w http.ResponseWriter, r *http.Request) {
	session, workspaceID, userID, ok := h.loadOwnedChatSession(w, r, false)
	if !ok {
		return
	}
	var request updateChatSessionRequest
	if decodeStrictTaskBody(r, &request) != nil || (request.Title == nil && request.Status == nil) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	title := session.Title
	if request.Title != nil {
		title = strings.TrimSpace(*request.Title)
		if title == "" || len([]rune(title)) > chatTitleMaxLength {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	status := session.Status
	if request.Status != nil {
		status = strings.TrimSpace(*request.Status)
		if status != "active" && status != "archived" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	locked, err := qtx.LockChatSession(r.Context(), lwdb.LockChatSessionParams{ID: session.ID, WorkspaceID: workspaceID})
	if err != nil || locked.CreatorID != userID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if status == "archived" {
		active, err := qtx.CountActiveTasksByChatSession(r.Context(), lwdb.CountActiveTasksByChatSessionParams{WorkspaceID: workspaceID, ChatSessionID: locked.ID})
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if active > 0 {
			writeCode(w, http.StatusConflict, "active_tasks_exist")
			return
		}
	}
	updated, err := qtx.UpdateChatSession(r.Context(), lwdb.UpdateChatSessionParams{
		SetTitle: request.Title != nil, Title: title, SetStatus: request.Status != nil, Status: status,
		ID: locked.ID, WorkspaceID: workspaceID,
	})
	if err != nil || tx.Commit(r.Context()) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventChatSessionUpdated, uuidString(workspaceID), "member", uuidString(userID), chatSessionDTO(updated))
	writeJSON(w, http.StatusOK, chatSessionDTO(updated))
}

func (h *Handler) DeleteChatSession(w http.ResponseWriter, r *http.Request) {
	session, workspaceID, userID, ok := h.loadOwnedChatSession(w, r, true)
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
	locked, err := qtx.LockChatSession(r.Context(), lwdb.LockChatSessionParams{ID: session.ID, WorkspaceID: workspaceID})
	if err != nil || locked.CreatorID != userID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if locked.Status != "archived" {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	active, err := qtx.CountActiveTasksByChatSession(r.Context(), lwdb.CountActiveTasksByChatSessionParams{WorkspaceID: workspaceID, ChatSessionID: locked.ID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if active > 0 {
		writeCode(w, http.StatusConflict, "active_tasks_exist")
		return
	}
	cleanupScope := lwdb.DeleteTaskTokensByChatSessionParams{TargetWorkspaceID: workspaceID, TargetChatSessionID: locked.ID}
	if _, err := qtx.DeleteTaskTokensByChatSession(r.Context(), cleanupScope); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTaskMessagesByChatSession(r.Context(), lwdb.DeleteTaskMessagesByChatSessionParams(cleanupScope)); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTaskUsageByChatSession(r.Context(), lwdb.DeleteTaskUsageByChatSessionParams(cleanupScope)); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteChatDraftRestoresBySession(r.Context(), locked.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := qtx.DeleteAgentBuilderDraft(r.Context(), locked.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteChatMessagesBySession(r.Context(), locked.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteTerminalTasksByChatSession(r.Context(), lwdb.DeleteTerminalTasksByChatSessionParams{WorkspaceID: workspaceID, ChatSessionID: locked.ID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	deleted, err := qtx.DeleteChatSession(r.Context(), lwdb.DeleteChatSessionParams{ID: locked.ID, WorkspaceID: workspaceID})
	if err != nil || deleted != 1 || tx.Commit(r.Context()) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventChatSessionDeleted, uuidString(workspaceID), "member", uuidString(userID), map[string]string{"chat_session_id": uuidString(locked.ID)})
	w.WriteHeader(http.StatusNoContent)
}

type sendChatMessageRequest struct {
	Content string `json:"content"`
}

type sendChatMessageResponse struct {
	MessageID string `json:"message_id"`
	TaskID    string `json:"task_id"`
	CreatedAt string `json:"created_at"`
}

func (h *Handler) SendChatMessage(w http.ResponseWriter, r *http.Request) {
	session, workspaceID, userID, ok := h.loadOwnedChatSession(w, r, false)
	if !ok {
		return
	}
	var request sendChatMessageRequest
	if decodeStrictTaskBody(r, &request) != nil || strings.TrimSpace(request.Content) == "" || len(request.Content) > chatContentMaxLength {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > chatIdempotencyMaxBytes {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	requestHash := requestDigest(request)
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	lockedSession, err := qtx.LockChatSession(r.Context(), lwdb.LockChatSessionParams{ID: session.ID, WorkspaceID: workspaceID})
	if err != nil || lockedSession.CreatorID != userID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if lockedSession.Status != "active" {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	key := pgtype.Text{String: idempotencyKey, Valid: true}
	existing, err := qtx.GetChatMessageByIdempotencyKey(r.Context(), lwdb.GetChatMessageByIdempotencyKeyParams{ChatSessionID: lockedSession.ID, IdempotencyKey: key})
	if err == nil {
		if !existing.RequestHash.Valid || existing.RequestHash.String != requestHash {
			writeCode(w, http.StatusConflict, "idempotency_key_reused")
			return
		}
		if !existing.TaskID.Valid {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		writeJSON(w, http.StatusCreated, sendChatMessageResponse{MessageID: uuidString(existing.ID), TaskID: uuidString(existing.TaskID), CreatedAt: timeString(existing.CreatedAt)})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	agent, err := qtx.GetAgentRow(r.Context(), lwdb.GetAgentRowParams{ID: lockedSession.AgentID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	builderCarrier := isAgentBuilderCarrier(agent)
	if !builderCarrier {
		if agent.ArchivedAt.Valid {
			writeCode(w, http.StatusConflict, "agent_archived")
			return
		}
		if !h.humanCanInvokeAgent(r.Context(), agent, userID) {
			writeCode(w, http.StatusForbidden, "invocation_forbidden")
			return
		}
	}
	if !agent.RuntimeID.Valid {
		writeCode(w, http.StatusConflict, "agent_runtime_required")
		return
	}
	runtime, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: agent.RuntimeID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusConflict, "agent_runtime_required")
		return
	}
	if builderCarrier {
		if _, err := qtx.LockAgentBuilderCarrier(r.Context(), lwdb.LockAgentBuilderCarrierParams{ID: agent.ID, WorkspaceID: workspaceID}); err != nil {
			writeCode(w, http.StatusConflict, "conflict")
			return
		}
	} else {
		lockedAgent, err := qtx.LockAgent(r.Context(), lwdb.LockAgentParams{ID: agent.ID, WorkspaceID: workspaceID})
		if err != nil || lockedAgent.RuntimeID != runtime.ID || lockedAgent.ArchivedAt.Valid {
			writeCode(w, http.StatusConflict, "conflict")
			return
		}
	}
	if runtime.Status != "online" {
		writeCode(w, http.StatusServiceUnavailable, "runtime_unavailable")
		return
	}
	if err := h.requireGatewayProviderForNewTask(r.Context(), qtx, workspaceID, agent.ID, runtime.ID); err != nil {
		writeGatewayProviderError(w, err)
		return
	}
	lockedSession, err = qtx.RebindChatSessionRuntime(r.Context(), lwdb.RebindChatSessionRuntimeParams{RuntimeID: runtime.ID, ID: lockedSession.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	task, err := qtx.CreateChatTask(r.Context(), lwdb.CreateChatTaskParams{
		WorkspaceID: workspaceID, AgentID: agent.ID, RuntimeID: runtime.ID, ChatSessionID: lockedSession.ID,
		SessionID: lockedSession.SessionID, WorkDir: lockedSession.WorkDir, UserID: userID,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	message, err := qtx.CreateChatMessage(r.Context(), lwdb.CreateChatMessageParams{
		ChatSessionID: lockedSession.ID, Role: "user", Content: request.Content, TaskID: task.ID,
		MessageKind: protocol.ChatMessageKindMessage, IdempotencyKey: key,
		RequestHash: pgtype.Text{String: requestHash, Valid: true},
	})
	if err != nil || tx.Commit(r.Context()) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventChatMessage, uuidString(workspaceID), "member", uuidString(userID), protocol.ChatMessagePayload{
		ChatSessionID: uuidString(lockedSession.ID), MessageID: uuidString(message.ID), Role: "user",
		Content: message.Content, TaskID: uuidString(task.ID), CreatedAt: timeString(message.CreatedAt),
	})
	h.announceQueuedTask(task)
	writeJSON(w, http.StatusCreated, sendChatMessageResponse{MessageID: uuidString(message.ID), TaskID: uuidString(task.ID), CreatedAt: timeString(message.CreatedAt)})
}

func (h *Handler) ListChatMessages(w http.ResponseWriter, r *http.Request) {
	session, ok := h.loadReadableChatSession(w, r)
	if !ok {
		return
	}
	limit, cursorAt, cursorID, err := parseChatPage(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	rows, err := h.q.ListChatMessagesPage(r.Context(), lwdb.ListChatMessagesPageParams{
		ChatSessionID: session.ID, CursorCreatedAt: cursorAt, CursorID: cursorID, PageLimit: limit + 1,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]chatMessageResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, chatMessageDTO(row))
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		value := encodeChatCursor(rows[len(rows)-1].CreatedAt, rows[len(rows)-1].ID)
		nextCursor = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextCursor})
}

func (h *Handler) GetPendingChatTask(w http.ResponseWriter, r *http.Request) {
	session, _, _, ok := h.loadOwnedChatSession(w, r, false)
	if !ok {
		return
	}
	task, err := h.q.GetPendingChatTask(r.Context(), lwdb.GetPendingChatTaskParams{WorkspaceID: session.WorkspaceID, ChatSessionID: session.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"task_id": uuidString(task.ID), "status": task.Status, "created_at": timeString(task.CreatedAt)})
}

func (h *Handler) ListChatDraftRestores(w http.ResponseWriter, r *http.Request) {
	session, _, _, ok := h.loadOwnedChatSession(w, r, true)
	if !ok {
		return
	}
	rows, err := h.q.ListChatDraftRestores(r.Context(), session.ID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]chatDraftRestoreResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, chatDraftRestoreDTO(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) ConsumeChatDraftRestore(w http.ResponseWriter, r *http.Request) {
	session, _, _, ok := h.loadOwnedChatSession(w, r, true)
	if !ok {
		return
	}
	restoreID, err := parseUUID(chi.URLParam(r, "restoreId"))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if _, err := h.q.DeleteChatDraftRestore(r.Context(), lwdb.DeleteChatDraftRestoreParams{ID: restoreID, ChatSessionID: session.ID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
